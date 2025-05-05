package eth

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// UpgradeStatusMsg is a message overloaded in eth/66
// TODO: upgrade go-ethereum package
const UpgradeStatusMsg = 0x0b

const (
	maxMessageSize                  = 10 * 1024 * 1024
	responseTimeout                 = 5 * time.Minute
	fastBlockConfirmationInterval   = 200 * time.Millisecond
	fastBlockConfirmationAttempts   = 5
	slowBlockConfirmationInterval   = 1 * time.Second
	headChannelBacklog              = 10
	blockChannelBacklog             = 10
	blockConfirmationChannelBacklog = 10
	blockQueueMaxSize               = 50
	delayLimit                      = 1 * time.Second
)

// special error constants during peer message processing
var (
	ErrResponseTimeout  = errors.New("response timed out")
	ErrUnknownRequestID = errors.New("unknown request ID on message")
)

// Peer wraps an Ethereum peer structure
type Peer struct {
	p        *p2p.Peer
	rw       p2p.MsgReadWriter
	version  uint
	chainID  uint64
	endpoint types.NodeEndpoint
	clock    clock.Clock
	ctx      context.Context
	cancel   context.CancelFunc
	log      *log.Entry

	disconnected     bool
	checkpointPassed bool

	responseQueue *syncmap.SyncMap[uint64, chan eth.Packet]

	newHeadCh           chan blockRef
	newBlockCh          chan *NewBlockPacket
	blockConfirmationCh chan common.Hash
	confirmedHead       blockRef
	sentHead            blockRef
	queuedBlocks        []*NewBlockPacket
	mu                  sync.RWMutex

	RequestConfirmations bool
}

// NewPeer returns a wrapped Ethereum peer
func NewPeer(ctx context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, chainID uint64) *Peer {
	return newPeer(ctx, p, rw, version, clock.RealClock{}, chainID)
}

func newPeer(parent context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, clock clock.Clock, chainID uint64) *Peer {
	ctx, cancel := context.WithCancel(parent)
	peer := &Peer{
		p:                    p,
		rw:                   rw,
		version:              version,
		ctx:                  ctx,
		cancel:               cancel,
		clock:                clock,
		chainID:              chainID,
		newHeadCh:            make(chan blockRef, headChannelBacklog),
		newBlockCh:           make(chan *NewBlockPacket, blockChannelBacklog),
		blockConfirmationCh:  make(chan common.Hash, blockConfirmationChannelBacklog),
		queuedBlocks:         make([]*NewBlockPacket, 0),
		responseQueue:        syncmap.NewIntegerMapOf[uint64, chan eth.Packet](),
		RequestConfirmations: true,
	}
	peer.endpoint = types.NodeEndpoint{IP: p.Node().IP().String(), Port: p.Node().TCP(), PublicKey: p.Info().Enode, Dynamic: !p.Info().Network.Static, ID: p.ID().String()}
	peerID := p.ID()
	peer.log = log.WithFields(log.Fields{
		"connType":   "ETH",
		"remoteAddr": p.RemoteAddr().String(),
		"id":         fmt.Sprintf("%x", peerID[:8]),
	})
	return peer
}

// ID provides a unique identifier for each Ethereum peer
func (ep *Peer) ID() string {
	return ep.p.ID().String()
}

// String formats the peer for display
func (ep *Peer) String() string {
	id := ep.p.ID()
	return fmt.Sprintf("ETH/%x@%v", id[:8], ep.p.RemoteAddr())
}

// IPEndpoint provides the peer IP endpoint
func (ep *Peer) IPEndpoint() types.NodeEndpoint {
	return ep.endpoint
}

// Dynamic returns true if the peer is dynamic connection
func (ep *Peer) Dynamic() bool {
	return !ep.p.Info().Network.Static
}

// Log returns the context logger for the peer connection
func (ep *Peer) Log() *log.Entry {
	ep.mu.RLock()
	confirmedHead := ep.confirmedHead
	ep.mu.RUnlock()

	return ep.log.WithField("head", confirmedHead)
}

// Disconnect closes the running peer with a protocol error
func (ep *Peer) Disconnect(reason p2p.DiscReason) {
	ep.p.Disconnect(reason)
	ep.mu.Lock()
	ep.disconnected = true
	ep.mu.Unlock()
}

// Start launches the block sending loop that queued blocks get sent in order to the peer
func (ep *Peer) Start() {
	go ep.blockLoop()
}

// Stop shuts down the running goroutines
func (ep *Peer) Stop() {
	ep.cancel()
}

func (ep *Peer) getConfirmedHead() blockRef {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.confirmedHead
}

func (ep *Peer) getSentHead() blockRef {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.sentHead
}

func (ep *Peer) getQueuedBlocks() []*NewBlockPacket {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.queuedBlocks
}

func (ep *Peer) isVersion66() bool {
	return ep.version >= ETH66
}

func (ep *Peer) blockLoop() {
	for {
		select {
		case newHead := <-ep.newHeadCh:
			if newHead.height < ep.getConfirmedHead().height {
				break
			}

			ep.mu.Lock()
			// update most recent blocks
			ep.confirmedHead = newHead
			if newHead.height >= ep.sentHead.height {
				ep.sentHead = newHead
			}
			ep.mu.Unlock()

			// check if any blocks need to be released and determine number of blocks to prune
			var (
				breakpoint   = -1
				releaseBlock = false
			)

			confirmedHead := ep.getConfirmedHead()

			for i, qb := range ep.getQueuedBlocks() {
				queuedBlock := qb.Block

				nextHeight := queuedBlock.NumberU64() == confirmedHead.height+1
				isNextBlock := nextHeight && queuedBlock.ParentHash() == confirmedHead.hash
				exceedsHeight := queuedBlock.NumberU64() > confirmedHead.height+1

				// next block: stop here and release
				if isNextBlock {
					breakpoint = i
					releaseBlock = true
					break
				}
				// next height: could be another block at same height, set breakpoint and continue
				if nextHeight {
					breakpoint = i
				}
				// exceeds height: set breakpoint if not set previously from block at "nextHeight" and break
				if exceedsHeight {
					if breakpoint == -1 {
						breakpoint = i
					}
					break
				}
			}

			// no blocks match, so all must be stale
			if breakpoint == -1 {
				breakpoint = len(ep.queuedBlocks)
			}

			// prune blocks
			for _, prunedBlock := range ep.getQueuedBlocks()[:breakpoint] {
				ep.Log().Debugf("pruning now stale block %v at height %v after confirming %v", prunedBlock.Block.Hash(), prunedBlock.Block.NumberU64(), newHead)
			}

			ep.mu.Lock()
			ep.queuedBlocks = ep.queuedBlocks[breakpoint:]
			ep.mu.Unlock()

			// release block if possible
			if releaseBlock {
				ep.sendTopBlock()
			}
		case newBlock := <-ep.newBlockCh:
			newBlockNumber := newBlock.Block.NumberU64()
			newBlockHash := newBlock.Block.Hash()

			// always ignore stales blocks
			if newBlockNumber <= ep.sentHead.height && ep.sentHead.height != 0 {
				ep.Log().Debugf("skipping queued stale block %v at height %v, already sent block of height %v", newBlockHash, newBlockNumber, ep.sentHead)
				break
			}

			confirmedHead := ep.getConfirmedHead()
			sentHead := ep.getSentHead()

			// release next block if ready
			nextBlock := newBlockNumber == confirmedHead.height+1 && newBlock.Block.ParentHash() == confirmedHead.hash
			initialBlock := confirmedHead.height == 0 && sentHead.height == 0 && newBlock.Block.ParentHash() == confirmedHead.hash
			if nextBlock || initialBlock {
				ep.Log().Debugf("queued block %v at height %v is next expected (%v), sending immediately", newBlockHash, newBlockNumber, confirmedHead.height+1)
				err := ep.sendNewBlock(newBlock)
				if err != nil {
					ep.Log().Errorf("could not send block %v to peer %v: %v", newBlockHash, ep, err)
				}
				ep.mu.Lock()
				ep.sentHead = blockRef{
					height: newBlockNumber,
					hash:   newBlockHash,
				}
				ep.mu.Unlock()
			} else {
				// find position in block queue that block should be inserted at
				queuedBlocks := ep.getQueuedBlocks()
				insertionPoint := len(queuedBlocks)
				for i, block := range queuedBlocks {
					if block.Block.NumberU64() > newBlock.Block.NumberU64() {
						insertionPoint = i
					}
				}

				ep.Log().Debugf("queuing block %v at height %v behind %v blocks", newBlockHash, newBlockNumber, insertionPoint)

				ep.mu.Lock()
				// insert block at insertion point
				ep.queuedBlocks = append(ep.queuedBlocks, &NewBlockPacket{})
				copy(ep.queuedBlocks[insertionPoint+1:], ep.queuedBlocks[insertionPoint:])
				ep.queuedBlocks[insertionPoint] = newBlock
				ep.mu.Unlock()

				if len(ep.getQueuedBlocks()) > blockQueueMaxSize {
					ep.mu.Lock()
					ep.queuedBlocks = ep.queuedBlocks[len(ep.queuedBlocks)-blockQueueMaxSize:]
					ep.mu.Unlock()
				}
			}
		case <-ep.ctx.Done():
			return
		}
	}
}

// should only be called by blockLoop when then length of the queue has already been checked
func (ep *Peer) sendTopBlock() {
	ep.mu.Lock()
	block := ep.queuedBlocks[0]
	ep.queuedBlocks = ep.queuedBlocks[1:]
	ep.sentHead = blockRef{
		height: block.Block.NumberU64(),
		hash:   block.Block.Hash(),
	}
	ep.mu.Unlock()

	ep.Log().Debugf("after confirming height %v, immediately sending next block %v at height %v", ep.getConfirmedHead(), block.Block.Hash(), block.Block.NumberU64())
	err := ep.sendNewBlock(block)
	if err != nil {
		ep.Log().Errorf("could not send block %v: %v", block.Block.Hash(), err)
	}
}

// Handshake executes the Ethereum protocol Handshake. Unlike Geth, the gateway waits for the peer status message before sending its own, in order to replicate some peer status fields.
func (ep *Peer) Handshake(version uint32, networkChain uint64, td *big.Int, head common.Hash, genesis common.Hash, executionLayerForks []string) (*eth.StatusPacket, error) {
	peerStatus, err := ep.readStatus()
	if err != nil {
		return peerStatus, err
	}

	ep.mu.Lock()
	ep.confirmedHead = blockRef{hash: head}
	ep.mu.Unlock()

	// for ethereum override the TD and the Head as get from the peer
	// Nethermind checks reject connections which brings old value
	if networkChain == network.EthMainnetChainID {
		td = peerStatus.TD
		head = peerStatus.Head
	}
	// used same fork ID as received from peer; gateway is expected to usually be compatible with Ethereum peer
	err = ep.send(eth.StatusMsg, &eth.StatusPacket{
		ProtocolVersion: version,
		NetworkID:       networkChain,
		TD:              td,
		Head:            head,
		Genesis:         genesis,
		ForkID:          peerStatus.ForkID,
	})

	if err != nil {
		return peerStatus, err
	}

	if version >= ETH67 {
		// Relevant only for BSC
		switch networkChain {
		case network.BSCMainnetChainID, network.BSCTestnetChainID:
			_, err = ep.upgradeStatus()
			if err != nil {
				ep.Log().Errorf("failed to upgrade status: %v", err)
			}
		}
	}

	if peerStatus.NetworkID != networkChain {
		if !ep.Dynamic() {
			ep.Log().Infof("network ID does not match: expected %v, but got %v", networkChain, peerStatus.NetworkID)
		} else {
			ep.Log().Tracef("network ID does not match: expected %v, but got %v", networkChain, peerStatus.NetworkID)
		}
		return peerStatus, fmt.Errorf("network ID does not match: expected %v, but got %v", networkChain, peerStatus.NetworkID)
	}

	if peerStatus.ProtocolVersion != version {
		if !ep.Dynamic() {
			ep.Log().Infof("protocol version does not match: expected %v, but got %v", version, peerStatus.ProtocolVersion)
		} else {
			ep.Log().Tracef("protocol version does not match: expected %v, but got %v", version, peerStatus.ProtocolVersion)
		}
		return peerStatus, fmt.Errorf("protocol version does not match: expected %v, but got %v", version, peerStatus.ProtocolVersion)
	}

	if peerStatus.Genesis != genesis {
		if !ep.Dynamic() {
			ep.Log().Infof("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
		} else {
			ep.Log().Tracef("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
		}
		return peerStatus, fmt.Errorf("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
	}

	if len(executionLayerForks) > 0 {
		b64ForkID := b64.StdEncoding.EncodeToString(peerStatus.ForkID.Hash[:])

		if !utils.Exists(b64ForkID, executionLayerForks) {
			if !ep.Dynamic() {
				ep.Log().Infof("fork ID does not match: expected %v, but got %v", executionLayerForks, b64ForkID)
			} else {
				ep.Log().Tracef("fork ID does not match: expected %v, but got %v", executionLayerForks, b64ForkID)
			}
			return peerStatus, fmt.Errorf("fork ID does not match: expected %v, but got %v", executionLayerForks, b64ForkID)
		}
	}

	return peerStatus, nil
}

// UpgradeStatusExtension is the extension data for the UpgradeStatusPacket
type UpgradeStatusExtension struct {
	DisablePeerTxBroadcast bool
}

// Encode encodes the extension data
func (e *UpgradeStatusExtension) Encode() (*rlp.RawValue, error) {
	rawBytes, err := rlp.EncodeToBytes(e)
	if err != nil {
		return nil, err
	}
	raw := rlp.RawValue(rawBytes)
	return &raw, nil
}

// UpgradeStatusPacket is the packet used to upgrade the status message
type UpgradeStatusPacket struct {
	Extension *rlp.RawValue `rlp:"nil"`
}

// GetExtension decodes the extension data
func (p *UpgradeStatusPacket) GetExtension() (*UpgradeStatusExtension, error) {
	extension := &UpgradeStatusExtension{}
	if p.Extension == nil {
		return extension, nil
	}
	err := rlp.DecodeBytes(*p.Extension, extension)
	if err != nil {
		return nil, err
	}
	return extension, nil
}

func (ep *Peer) upgradeStatus() (*UpgradeStatusExtension, error) {
	upgradeStatus, err := ep.readUpgradeStatus()
	if err != nil {
		return nil, err
	}

	extension, err := upgradeStatus.GetExtension()
	if err != nil {
		return nil, err
	}

	if err := ep.send(UpgradeStatusMsg, upgradeStatus); err != nil {
		return extension, err
	}

	return extension, nil
}

func (ep *Peer) readUpgradeStatus() (*UpgradeStatusPacket, error) {
	var upgradeStatus UpgradeStatusPacket // safe to read after two values have been received from errc
	type messageChanResponse struct {
		msg p2p.Msg
		err error
	}
	messageChan := make(chan messageChanResponse, 1)

	go func() {
		msg, err := ep.rw.ReadMsg()
		messageChan <- messageChanResponse{msg, err}

	}()
	timeout := time.NewTicker(time.Second * 6)
	var msg p2p.Msg
	select {
	case message := <-messageChan:
		if message.err != nil {
			return nil, message.err
		}
		msg = message.msg
	case <-timeout.C:
		timeout.Stop()
		return nil, errors.New("failed to get message from peer, deadline exceeded")
	}
	defer func() {
		_ = msg.Discard()
	}()

	if msg.Code != UpgradeStatusMsg {
		return &upgradeStatus, fmt.Errorf("unexpected message: %v, should have been %v", msg.Code, UpgradeStatusMsg)
	}

	if msg.Size > maxMessageSize {
		return &upgradeStatus, fmt.Errorf("message is too big: %v > %v", msg.Size, maxMessageSize)
	}

	if err := msg.Decode(&upgradeStatus); err != nil {
		return &upgradeStatus, fmt.Errorf("could not decode upgrade status message: %v", err)
	}
	return &upgradeStatus, nil
}

func (ep *Peer) readStatus() (*eth.StatusPacket, error) {
	var status eth.StatusPacket
	type messageChanResponse struct {
		msg p2p.Msg
		err error
	}
	messageChan := make(chan messageChanResponse, 1)

	go func() {
		msg, err := ep.rw.ReadMsg()
		messageChan <- messageChanResponse{msg, err}
	}()

	timeout := time.NewTicker(time.Second * 6)
	var msg p2p.Msg
	select {
	case message := <-messageChan:
		if message.err != nil {
			return nil, message.err
		}
		msg = message.msg
	case <-timeout.C:
		timeout.Stop()
		return nil, errors.New("failed to get message from peer, deadline exceeded")
	}
	defer func() {
		_ = msg.Discard()
	}()

	if msg.Code != eth.StatusMsg {
		return &status, fmt.Errorf("unexpected first message: %v, should have been %v", msg.Code, eth.StatusMsg)
	}

	if msg.Size > maxMessageSize {
		return &status, fmt.Errorf("message is too big: %v > %v", msg.Size, maxMessageSize)
	}

	if err := msg.Decode(&status); err != nil {
		return &status, fmt.Errorf("could not decode status message: %v", err)
	}

	return &status, nil
}

// NotifyResponse informs any listeners dependent on a request/response call to this peer, indicating if any channels were waiting for the message
func (ep *Peer) NotifyResponse(requestID uint64, packet eth.Packet) (bool, error) {
	responseCh, ok := ep.responseQueue.LoadAndDelete(requestID)
	if !ok {
		return false, ErrUnknownRequestID
	}

	if responseCh != nil {
		responseCh <- packet
	}
	return responseCh != nil, nil
}

// UpdateHead sets the latest confirmed block on the peer. This may release or prune queued blocks on the peer connection.
func (ep *Peer) UpdateHead(height uint64, hash common.Hash) {
	ep.Log().Debugf("confirming new head (height=%v, hash=%v), newHeadCh len %v", height, hash, len(ep.newHeadCh))
	select {
	case ep.newHeadCh <- blockRef{height: height, hash: hash}:
	default:
		ep.Log().Errorf("newHeadCh channel is full(height=%v, hash=%v), dropping new head update", height, hash)
	}
}

// QueueNewBlock adds a new block to the queue to be sent to the peer in the order the peer is ready for.
func (ep *Peer) QueueNewBlock(block *bxcommoneth.Block, td *big.Int) {
	packet := NewBlockPacket{
		Block:    &block.Block,
		TD:       td,
		Sidecars: block.Sidecars(),
	}
	ep.newBlockCh <- &packet
}

// AnnounceBlock pushes a new block announcement to the peer. This is used when the total difficult is unknown, and so a new block message would be invalid.
func (ep *Peer) AnnounceBlock(hash common.Hash, number uint64) error {
	packet := eth.NewBlockHashesPacket{
		{Hash: hash, Number: number},
	}
	return ep.send(eth.NewBlockHashesMsg, packet)
}

// SendBlockBodies sends a batch of block bodies to the peer
func (ep *Peer) SendBlockBodies(bodies []*eth.BlockBody) error {
	return ep.send(eth.BlockBodiesMsg, eth.BlockBodiesResponse(bodies))
}

// ReplyBlockBodies sends a batch of requested block bodies to the peer
func (ep *Peer) ReplyBlockBodies(id uint64, bodies []*BlockBody) error {
	return ep.send(eth.BlockBodiesMsg, BlockBodiesPacket{
		RequestID:           id,
		BlockBodiesResponse: bodies,
	})
}

// ReplyBlockHeaders sends batch of requested block headers to the peer
func (ep *Peer) ReplyBlockHeaders(id uint64, headers []*ethtypes.Header) error {
	return ep.send(eth.BlockHeadersMsg, eth.BlockHeadersPacket{
		RequestId:           id,
		BlockHeadersRequest: headers,
	})
}

// SendTransactions sends pushes a batch of transactions to the peer
func (ep *Peer) SendTransactions(txs ethtypes.Transactions) error {
	return ep.send(eth.TransactionsMsg, txs)
}

// RequestTransactions requests a batch of announced transactions from the peer
func (ep *Peer) RequestTransactions(txHashes []common.Hash) error {
	packet := eth.GetPooledTransactionsRequest(txHashes)
	return ep.send(eth.GetPooledTransactionsMsg, eth.GetPooledTransactionsPacket{
		RequestId:                    rand.Uint64(),
		GetPooledTransactionsRequest: packet,
	})
}

// SendPooledTransactionHashes67 sends transaction hashes to the peer
func (ep *Peer) SendPooledTransactionHashes67(txHashes []common.Hash) error {
	return ep.send(eth.NewPooledTransactionHashesMsg, NewPooledTransactionHashesPacket66(txHashes))
}

// SendPooledTransactionHashes sends transaction hashes to the peer
func (ep *Peer) SendPooledTransactionHashes(txHashes []common.Hash, types []byte, sizes []uint32) error {
	return ep.send(eth.NewPooledTransactionHashesMsg, eth.NewPooledTransactionHashesPacket{
		Hashes: txHashes,
		Types:  types,
		Sizes:  sizes,
	})
}

// ReplyPooledTransaction sends a batch of pooled transactions to the peer
func (ep *Peer) ReplyPooledTransaction(id uint64, txs []rlp.RawValue) error {
	return ep.send(eth.PooledTransactionsMsg, eth.PooledTransactionsRLPPacket{
		RequestId:                     id,
		PooledTransactionsRLPResponse: txs,
	})
}

// RequestBlock fetches the specified block from the ETH66 peer, pushing the components to the channels upon request/response completion.
func (ep *Peer) RequestBlock(blockHash common.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) error {
	if !ep.isVersion66() {
		panic("unexpected call to request block 66 for a <ETH66 peer")
	}

	getHeadersPacket := &eth.GetBlockHeadersRequest{
		Origin:  eth.HashOrNumber{Hash: blockHash},
		Amount:  1,
		Skip:    0,
		Reverse: false,
	}
	getBodiesPacket := eth.GetBlockBodiesRequest{blockHash}

	getHeadersRequestID := rand.Uint64()
	getHeadersPacket66 := eth.GetBlockHeadersPacket{
		RequestId:              getHeadersRequestID,
		GetBlockHeadersRequest: getHeadersPacket,
	}
	getBodiesRequestID := rand.Uint64()
	getBodiesPacket66 := eth.GetBlockBodiesPacket{
		RequestId:             getBodiesRequestID,
		GetBlockBodiesRequest: getBodiesPacket,
	}

	ep.registerForResponse(getHeadersRequestID, headersCh)
	ep.registerForResponse(getBodiesRequestID, bodiesCh)

	if err := ep.send(eth.GetBlockHeadersMsg, getHeadersPacket66); err != nil {
		return err
	}
	if err := ep.send(eth.GetBlockBodiesMsg, getBodiesPacket66); err != nil {
		return err
	}
	return nil
}

// RequestBlockHeader dispatches a request for the specified block header to the Ethereum peer with no expected handling on response
func (ep *Peer) RequestBlockHeader(hash common.Hash) error {
	return ep.RequestBlockHeaderRaw(eth.HashOrNumber{Hash: hash}, 1, 0, false, nil)
}

// RequestBlockHeaderRaw dispatches a generalized GetHeaders message to the Ethereum peer
func (ep *Peer) RequestBlockHeaderRaw(origin eth.HashOrNumber, amount, skip uint64, reverse bool, responseCh chan eth.Packet) error {
	packet := eth.GetBlockHeadersRequest{
		Origin:  origin,
		Amount:  amount,
		Skip:    skip,
		Reverse: reverse,
	}

	requestID := rand.Uint64()
	ep.registerForResponse(requestID, responseCh)

	return ep.send(eth.GetBlockHeadersMsg, eth.GetBlockHeadersPacket{
		RequestId:              requestID,
		GetBlockHeadersRequest: &packet,
	})
}

func (ep *Peer) sendNewBlock(packet *NewBlockPacket) error {
	// For BSC, the timestamp in block header is an expected time provided by the validator, but sometimes it arrives at BDN before the timestamp provided
	if ep.chainID == bxtypes.BSCChainID {
		// delay if the header timestamp is later than the system timestamp
		blockHeaderTimestamp := int64(packet.Block.Time()) * time.Second.Nanoseconds()
		currentTimestamp := ep.clock.Now().UnixNano()
		delayNanoseconds := blockHeaderTimestamp - currentTimestamp
		if delayNanoseconds > 0 {
			if delayNanoseconds > delayLimit.Nanoseconds() {
				ep.log.Warnf("received future block from BDN that is too far ahead by %v ms, block hash %v, block height %v, - not sending to peer", packet.Block.Hash().String(), packet.Block.Number().String(), time.Duration(delayNanoseconds).Milliseconds())
				return nil
			}
			go ep.clock.AfterFunc(time.Duration(delayNanoseconds), func() {
				_ = ep.sendNewBlock(packet)
			})
			ep.log.Tracef("received future block from blockchain peer, the block will be sent with %vms delay, block hash %v, block height %v", packet.Block.Hash().String(), packet.Block.Number().String(), time.Duration(delayNanoseconds).Milliseconds())
			return nil
		}
	}

	err := ep.send(eth.NewBlockMsg, packet)
	if err != nil {
		return err
	}

	if ep.RequestConfirmations {
		go func() {
			blockNumber := packet.Block.NumberU64()
			blockHash := packet.Block.Hash()
			count := 0

			// check for fast confirmation
			ticker := ep.clock.Ticker(fastBlockConfirmationInterval)
			defer ticker.Stop()
			for count < fastBlockConfirmationAttempts {
				<-ticker.Alert()
				if ep.getConfirmedHead().height >= blockNumber {
					return
				}
				err := ep.RequestBlockHeader(blockHash)
				if err != nil {
					return
				}
				count++
			}

			// check for slower confirmation
			ticker = ep.clock.Ticker(slowBlockConfirmationInterval)
			for {
				<-ticker.Alert()
				if ep.getConfirmedHead().height >= blockNumber {
					return
				}
				err := ep.RequestBlockHeader(blockHash)
				if err != nil {
					return
				}
			}
		}()
	}
	return nil
}

func (ep *Peer) registerForResponse(requestID uint64, responseCh chan eth.Packet) {
	ep.responseQueue.Store(requestID, responseCh)
}

func (ep *Peer) isDisconnected() bool {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.disconnected
}

func (ep *Peer) send(msgCode uint64, data interface{}) error {
	if ep.isDisconnected() {
		return nil
	}
	return p2p.Send(ep.rw, msgCode, data)
}
