package eth

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
)

const (
	maxMessageSize                  = 10 * 1024 * 1024
	responseQueueSize               = 10
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
	clock    utils.Clock
	ctx      context.Context
	cancel   context.CancelFunc
	log      *log.Entry

	disconnected     bool
	checkpointPassed bool

	responseQueue   chan chan eth.Packet // chan is used as a concurrency safe queue
	responseQueue66 *syncmap.SyncMap[uint64, chan eth.Packet]

	newHeadCh           chan blockRef
	newBlockCh          chan *eth.NewBlockPacket
	blockConfirmationCh chan common.Hash
	confirmedHead       blockRef
	sentHead            blockRef
	queuedBlocks        []*eth.NewBlockPacket

	RequestConfirmations bool
}

// NewPeer returns a wrapped Ethereum peer
func NewPeer(ctx context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, chainID uint64) *Peer {
	return newPeer(ctx, p, rw, version, utils.RealClock{}, chainID)
}

func newPeer(parent context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, clock utils.Clock, chainID uint64) *Peer {
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
		newBlockCh:           make(chan *eth.NewBlockPacket, blockChannelBacklog),
		blockConfirmationCh:  make(chan common.Hash, blockConfirmationChannelBacklog),
		queuedBlocks:         make([]*eth.NewBlockPacket, 0),
		responseQueue:        make(chan chan eth.Packet, responseQueueSize),
		responseQueue66:      syncmap.NewIntegerMapOf[uint64, chan eth.Packet](),
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
	return ep.log.WithField("head", ep.confirmedHead)
}

// Disconnect closes the running peer with a protocol error
func (ep *Peer) Disconnect(reason p2p.DiscReason) {
	ep.p.Disconnect(reason)
	ep.disconnected = true
}

// Start launches the block sending loop that queued blocks get sent in order to the peer
func (ep *Peer) Start() {
	go ep.blockLoop()
}

// Stop shuts down the running goroutines
func (ep *Peer) Stop() {
	ep.cancel()
}

func (ep *Peer) isVersion66() bool {
	return ep.version >= eth.ETH66
}

func (ep *Peer) blockLoop() {
	for {
		select {
		case newHead := <-ep.newHeadCh:
			if newHead.height < ep.confirmedHead.height {
				break
			}

			// update most recent blocks
			ep.confirmedHead = newHead
			if newHead.height >= ep.sentHead.height {
				ep.sentHead = newHead
			}

			// check if any blocks need to be released and determine number of blocks to prune
			var (
				breakpoint   = -1
				releaseBlock = false
			)
			for i, qb := range ep.queuedBlocks {
				queuedBlock := qb.Block

				nextHeight := queuedBlock.NumberU64() == ep.confirmedHead.height+1
				isNextBlock := nextHeight && queuedBlock.ParentHash() == ep.confirmedHead.hash
				exceedsHeight := queuedBlock.NumberU64() > ep.confirmedHead.height+1

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
			for _, prunedBlock := range ep.queuedBlocks[:breakpoint] {
				ep.Log().Debugf("pruning now stale block %v at height %v after confirming %v", prunedBlock.Block.Hash(), prunedBlock.Block.NumberU64(), newHead)
			}
			ep.queuedBlocks = ep.queuedBlocks[breakpoint:]

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

			// release next block if ready
			nextBlock := newBlockNumber == ep.confirmedHead.height+1 && newBlock.Block.ParentHash() == ep.confirmedHead.hash
			initialBlock := ep.confirmedHead.height == 0 && ep.sentHead.height == 0 && newBlock.Block.ParentHash() == ep.confirmedHead.hash
			if nextBlock || initialBlock {
				ep.Log().Debugf("queued block %v at height %v is next expected (%v), sending immediately", newBlockHash, newBlockNumber, ep.confirmedHead.height+1)
				err := ep.sendNewBlock(newBlock)
				if err != nil {
					ep.Log().Errorf("could not send block %v to peer %v: %v", newBlockHash, ep, err)
				}
				ep.sentHead = blockRef{
					height: newBlockNumber,
					hash:   newBlockHash,
				}
			} else {
				// find position in block queue that block should be inserted at
				insertionPoint := len(ep.queuedBlocks)
				for i, block := range ep.queuedBlocks {
					if block.Block.NumberU64() > newBlock.Block.NumberU64() {
						insertionPoint = i
					}
				}

				ep.Log().Debugf("queuing block %v at height %v behind %v blocks", newBlockHash, newBlockNumber, insertionPoint)

				// insert block at insertion point
				ep.queuedBlocks = append(ep.queuedBlocks, &eth.NewBlockPacket{})
				copy(ep.queuedBlocks[insertionPoint+1:], ep.queuedBlocks[insertionPoint:])
				ep.queuedBlocks[insertionPoint] = newBlock

				if len(ep.queuedBlocks) > blockQueueMaxSize {
					ep.queuedBlocks = ep.queuedBlocks[len(ep.queuedBlocks)-blockQueueMaxSize:]
				}
			}
		case <-ep.ctx.Done():
			return
		}
	}
}

// should only be called by blockLoop when then length of the queue has already been checked
func (ep *Peer) sendTopBlock() {
	block := ep.queuedBlocks[0]
	ep.queuedBlocks = ep.queuedBlocks[1:]
	ep.sentHead = blockRef{
		height: block.Block.NumberU64(),
		hash:   block.Block.Hash(),
	}

	ep.Log().Debugf("after confirming height %v, immediately sending next block %v at height %v", ep.confirmedHead.height, block.Block.Hash(), block.Block.NumberU64())
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

	ep.confirmedHead = blockRef{hash: head}

	// for ethereum override the TD and the Head as get from the peer
	// Nethermind checks reject connections which brings old value
	// New: If polygon sees old head it sends header request for it and of course it fails. This causes gateway to be disconnected
	switch networkChain {
	case network.EthMainnetChainID, network.PolygonMainnetChainID:
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
func (ep *Peer) NotifyResponse(packet eth.Packet) bool {
	responseCh := <-ep.responseQueue
	if responseCh != nil {
		responseCh <- packet
	}
	return responseCh != nil
}

// NotifyResponse66 informs any listeners dependent on a request/response call to this ETH66 peer, indicating if any channels were waiting for the message
func (ep *Peer) NotifyResponse66(requestID uint64, packet eth.Packet) (bool, error) {
	responseCh, ok := ep.responseQueue66.LoadAndDelete(requestID)
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
	ep.Log().Debugf("confirming new head (height=%v, hash=%v)", height, hash)
	ep.newHeadCh <- blockRef{
		height: height,
		hash:   hash,
	}
}

// QueueNewBlock adds a new block to the queue to be sent to the peer in the order the peer is ready for.
func (ep *Peer) QueueNewBlock(block *ethtypes.Block, td *big.Int) {
	packet := eth.NewBlockPacket{
		Block: block,
		TD:    td,
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
	return ep.send(eth.BlockBodiesMsg, eth.BlockBodiesPacket(bodies))
}

// ReplyBlockBodies sends a batch of requested block bodies to the peer (ETH66)
func (ep *Peer) ReplyBlockBodies(id uint64, bodies []*eth.BlockBody) error {
	return ep.send(eth.BlockBodiesMsg, eth.BlockBodiesPacket66{
		RequestId:         id,
		BlockBodiesPacket: bodies,
	})
}

// SendBlockHeaders sends a batch of block headers to the peer
func (ep *Peer) SendBlockHeaders(headers []*ethtypes.Header) error {
	return ep.send(eth.BlockHeadersMsg, eth.BlockHeadersPacket(headers))
}

// ReplyBlockHeaders sends batch of requested block headers to the peer (ETH66)
func (ep *Peer) ReplyBlockHeaders(id uint64, headers []*ethtypes.Header) error {
	return ep.send(eth.BlockHeadersMsg, eth.BlockHeadersPacket66{
		RequestId:          id,
		BlockHeadersPacket: headers,
	})
}

// SendTransactions sends pushes a batch of transactions to the peer
func (ep *Peer) SendTransactions(txs ethtypes.Transactions) error {
	return ep.send(eth.TransactionsMsg, txs)
}

// RequestTransactions requests a batch of announced transactions from the peer
func (ep *Peer) RequestTransactions(txHashes []common.Hash) error {
	packet := eth.GetPooledTransactionsPacket(txHashes)
	if ep.isVersion66() {
		return ep.send(eth.GetPooledTransactionsMsg, eth.GetPooledTransactionsPacket66{
			RequestId:                   rand.Uint64(),
			GetPooledTransactionsPacket: packet,
		})
	}
	return ep.send(eth.GetPooledTransactionsMsg, packet)
}

// RequestBlock fetches the specified block from the peer, pushing the components to the channels upon request/response completion.
func (ep *Peer) RequestBlock(blockHash common.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) error {
	if ep.isVersion66() {
		panic("unexpected call to request block for a ETH66 peer")
	}

	getHeadersPacket := &eth.GetBlockHeadersPacket{
		Origin:  eth.HashOrNumber{Hash: blockHash},
		Amount:  1,
		Skip:    0,
		Reverse: false,
	}

	getBodiesPacket := eth.GetBlockBodiesPacket{blockHash}

	ep.registerForResponse(headersCh)
	ep.registerForResponse(bodiesCh)

	if err := ep.send(eth.GetBlockHeadersMsg, getHeadersPacket); err != nil {
		return err
	}

	return ep.send(eth.GetBlockBodiesMsg, getBodiesPacket)
}

// RequestBlock66 fetches the specified block from the ETH66 peer, pushing the components to the channels upon request/response completion.
func (ep *Peer) RequestBlock66(blockHash common.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) error {
	if !ep.isVersion66() {
		panic("unexpected call to request block 66 for a <ETH66 peer")
	}

	getHeadersPacket := &eth.GetBlockHeadersPacket{
		Origin:  eth.HashOrNumber{Hash: blockHash},
		Amount:  1,
		Skip:    0,
		Reverse: false,
	}
	getBodiesPacket := eth.GetBlockBodiesPacket{blockHash}

	getHeadersRequestID := rand.Uint64()
	getHeadersPacket66 := eth.GetBlockHeadersPacket66{
		RequestId:             getHeadersRequestID,
		GetBlockHeadersPacket: getHeadersPacket,
	}
	getBodiesRequestID := rand.Uint64()
	getBodiesPacket66 := eth.GetBlockBodiesPacket66{
		RequestId:            getBodiesRequestID,
		GetBlockBodiesPacket: getBodiesPacket,
	}

	ep.registerForResponse66(getHeadersRequestID, headersCh)
	ep.registerForResponse66(getBodiesRequestID, bodiesCh)

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
	packet := eth.GetBlockHeadersPacket{
		Origin:  origin,
		Amount:  amount,
		Skip:    skip,
		Reverse: reverse,
	}
	if ep.isVersion66() {
		requestID := rand.Uint64()
		ep.registerForResponse66(requestID, responseCh)

		return ep.send(eth.GetBlockHeadersMsg, eth.GetBlockHeadersPacket66{
			RequestId:             requestID,
			GetBlockHeadersPacket: &packet,
		})
	}

	// intercept and discard handler on response
	ep.registerForResponse(responseCh)
	return ep.send(eth.GetBlockHeadersMsg, packet)
}

func (ep *Peer) sendNewBlock(packet *eth.NewBlockPacket) error {
	// For BSC, the timestamp in block header is an expected time provided by the validator, but sometimes it arrives at BDN before the timestamp provided
	if ep.chainID == bxgateway.BSCChainID {
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
				if ep.confirmedHead.height >= blockNumber {
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
				if ep.confirmedHead.height >= blockNumber {
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

func (ep *Peer) registerForResponse(responseCh chan eth.Packet) {
	ep.responseQueue <- responseCh
}

func (ep *Peer) registerForResponse66(requestID uint64, responseCh chan eth.Packet) {
	ep.responseQueue66.Store(requestID, responseCh)
}

func (ep *Peer) send(msgCode uint64, data interface{}) error {
	if ep.disconnected {
		return nil
	}
	return p2p.Send(ep.rw, msgCode, data)
}
