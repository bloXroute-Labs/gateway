package eth

import (
	"context"
	"crypto/rand"
	b64 "encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"

	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// UpgradeStatusMsg is a message overloaded in eth/66
const UpgradeStatusMsg = 0x0b

const (
	maxMessageSize                  = 10 * 1024 * 1024
	fastBlockConfirmationInterval   = 200 * time.Millisecond
	fastBlockConfirmationAttempts   = 5
	slowBlockConfirmationInterval   = 1 * time.Second
	headChannelBacklog              = 10
	blockChannelBacklog             = 10
	blockConfirmationChannelBacklog = 10
	blockQueueMaxSize               = 50
	delayLimit                      = time.Second
	readStatusTimeout               = 6 * time.Second
	sendQueueSize                   = 20
)

// special error constants during peer message processing
var (
	ErrResponseTimeout  = errors.New("response timed out")
	ErrUnknownRequestID = errors.New("unknown request ID on message")
)

// Peer wraps an Ethereum peer structure
type Peer struct {
	*p2p.Peer
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

	ResponseQueue *syncmap.SyncMap[uint64, chan eth.Packet]

	newHeadCh            chan core.BlockRef
	newBlockCh           chan NewBlockPacket
	blockConfirmationCh  chan common.Hash
	queuedBlocks         []NewBlockPacket
	mu                   sync.RWMutex
	RequestConfirmations bool

	ConfirmedHead atomic.Value // stores core.BlockRef
	sentHead      atomic.Value // stores core.BlockRef

	// send queue to avoid blocking callers on p2p.Send
	sendCh chan sendJob
}

type sendJob struct {
	code uint64
	data interface{}
}

// NewPeer returns a wrapped Ethereum peer
func NewPeer(ctx context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, chainID uint64) *Peer {
	return newPeer(ctx, p, rw, version, clock.RealClock{}, chainID)
}

func newPeer(parent context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, clock clock.Clock, chainID uint64) *Peer {
	ctx, cancel := context.WithCancel(parent)
	peer := &Peer{
		Peer:                 p,
		rw:                   rw,
		version:              version,
		ctx:                  ctx,
		cancel:               cancel,
		clock:                clock,
		chainID:              chainID,
		newHeadCh:            make(chan core.BlockRef, headChannelBacklog),
		newBlockCh:           make(chan NewBlockPacket, blockChannelBacklog),
		blockConfirmationCh:  make(chan common.Hash, blockConfirmationChannelBacklog),
		queuedBlocks:         make([]NewBlockPacket, 0),
		ResponseQueue:        syncmap.NewIntegerMapOf[uint64, chan eth.Packet](),
		RequestConfirmations: true,
		sendCh:               make(chan sendJob, sendQueueSize),
	}

	go peer.startSender()
	peer.endpoint = types.NodeEndpoint{IP: p.Node().IP().String(), Port: p.Node().TCP(), PublicKey: p.Info().Enode, Dynamic: !p.Info().Network.Static, ID: p.ID().String()}
	peer.ConfirmedHead.Store(core.BlockRef{})
	peer.sentHead.Store(core.BlockRef{})
	peerID := p.ID()
	peer.log = log.WithFields(log.Fields{
		"connType":   "ETH",
		"remoteAddr": p.RemoteAddr().String(),
		"id":         fmt.Sprintf("%x", peerID[:8]),
	})
	return peer
}

func (p *Peer) startSender() {
	for {
		select {
		case job, ok := <-p.sendCh:
			if !ok {
				return
			}
			err := p2p.Send(p.rw, job.code, job.data)
			if err != nil && errors.Is(err, p2p.ErrShuttingDown) {
				p.log.Warn("peer is shutting down, disconnecting")
				p.Disconnect(p2p.DiscQuitting)
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// PassCheckpoint marks the peer as having passed the checkpoint, which is used to determine if the peer is ready to receive blocks
func (p *Peer) PassCheckpoint() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.checkpointPassed = true
}

// Context returns the context of the peer, which is used to cancel operations
func (p *Peer) Context() context.Context {
	return p.ctx
}

// ReceiveNewBlock returns a channel that receives new blocks from the peer
func (p *Peer) ReceiveNewBlock() <-chan NewBlockPacket {
	return p.newBlockCh
}

// Version returns the negotiated Ethereum protocol version of the peer
func (p *Peer) Version() uint {
	return p.version
}

// ID provides a unique identifier for each Ethereum peer
func (p *Peer) ID() string {
	return p.Peer.ID().String()
}

// String formats the peer for display
func (p *Peer) String() string {
	return fmt.Sprintf("ETH/%x@%v", p.ID()[:8], p.Peer.RemoteAddr())
}

// IPEndpoint provides the peer IP endpoint
func (p *Peer) IPEndpoint() types.NodeEndpoint {
	return p.endpoint
}

// Dynamic returns true if the peer is dynamic connection
func (p *Peer) Dynamic() bool {
	return !p.Peer.Info().Network.Static
}

// Log returns the context logger for the peer connection
func (p *Peer) Log() *log.Entry {
	return p.log.WithField("head", p.ConfirmedHead.Load().(core.BlockRef))
}

// Disconnect closes the running peer with a protocol error
func (p *Peer) Disconnect(reason p2p.DiscReason) {
	p.Peer.Disconnect(reason)
	p.mu.Lock()
	p.disconnected = true
	p.mu.Unlock()
}

// Start launches the block sending loop that queued blocks get sent to the peer
func (p *Peer) Start() {
	go p.blockLoop()
}

// Stop shuts down the running goroutines
func (p *Peer) Stop() {
	p.cancel()
}

// GetConfirmedHead returns the most recent confirmed block on the peer
func (p *Peer) GetConfirmedHead() core.BlockRef {
	return p.ConfirmedHead.Load().(core.BlockRef)
}

func (p *Peer) getSentHead() core.BlockRef {
	return p.sentHead.Load().(core.BlockRef)
}

func (p *Peer) getQueuedBlocks() []NewBlockPacket {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.queuedBlocks
}

func (p *Peer) isVersion66() bool {
	return p.version >= ETH66
}

func (p *Peer) blockLoop() {
	for {
		select {
		case newHead := <-p.newHeadCh:
			if newHead.Height < p.GetConfirmedHead().Height {
				break
			}

			// update most recent blocks
			p.ConfirmedHead.Store(newHead)
			if newHead.Height >= p.getSentHead().Height {
				p.sentHead.Store(newHead)
			}

			// check if any blocks need to be released and determine a number of blocks to prune
			var (
				breakpoint   = -1
				releaseBlock = false
			)

			confirmedHead := p.GetConfirmedHead()

			queuedBlocks := p.getQueuedBlocks()

			for i := range queuedBlocks {
				queuedBlock := queuedBlocks[i].Block

				nextHeight := queuedBlock.NumberU64() == confirmedHead.Height+1
				isNextBlock := nextHeight && queuedBlock.ParentHash() == confirmedHead.Hash
				exceedsHeight := queuedBlock.NumberU64() > confirmedHead.Height+1

				// next block: stop here and release
				if isNextBlock {
					breakpoint = i
					releaseBlock = true
					break
				}
				// next height: could be another block at the same height, set breakpoint and continue
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
				breakpoint = len(p.queuedBlocks)
			}

			// prune blocks
			for _, prunedBlock := range p.getQueuedBlocks()[:breakpoint] {
				p.Log().Debugf("pruning now stale block %v at height %v after confirming %v", prunedBlock.Block.Hash(), prunedBlock.Block.NumberU64(), newHead)
			}

			p.mu.Lock()
			p.queuedBlocks = p.queuedBlocks[breakpoint:]
			p.mu.Unlock()

			// release block if possible
			if releaseBlock {
				p.sendTopBlock()
			}
		case newBlock := <-p.newBlockCh:
			newBlockNumber := newBlock.Block.NumberU64()
			newBlockHash := newBlock.Block.Hash()

			sentHeadHeight := p.getSentHead().Height

			// always ignore stales blocks
			if newBlockNumber <= sentHeadHeight && sentHeadHeight != 0 {
				p.Log().Debugf("skipping queued stale block %v at height %v, already sent block of height %v", newBlockHash, newBlockNumber, p.sentHead)
				break
			}

			confirmedHead := p.GetConfirmedHead()
			sentHead := p.getSentHead()

			// release next block if ready
			nextBlock := newBlockNumber == confirmedHead.Height+1 && newBlock.Block.ParentHash() == confirmedHead.Hash
			initialBlock := confirmedHead.Height == 0 && sentHead.Height == 0 && newBlock.Block.ParentHash() == confirmedHead.Hash
			if nextBlock || initialBlock {
				p.Log().Debugf("queued block %v at height %v is next expected (%v), sending immediately", newBlockHash, newBlockNumber, confirmedHead.Height+1)
				err := p.sendNewBlock(newBlock)
				if err != nil {
					p.Log().Errorf("could not send block %v to peer %v: %v", newBlockHash, p, err)
				}

				p.sentHead.Store(core.BlockRef{
					Height: newBlockNumber,
					Hash:   newBlockHash,
				})
			} else {
				// find position in block queue that block should be inserted at
				queuedBlocks := p.getQueuedBlocks()
				insertionPoint := len(queuedBlocks)
				for i, block := range queuedBlocks {
					if block.Block.NumberU64() > newBlock.Block.NumberU64() {
						insertionPoint = i
					}
				}

				p.Log().Debugf("queuing block %v at height %v behind %v blocks", newBlockHash, newBlockNumber, insertionPoint)

				p.mu.Lock()
				// insert block at insertion point
				p.queuedBlocks = append(p.queuedBlocks, NewBlockPacket{})
				copy(p.queuedBlocks[insertionPoint+1:], p.queuedBlocks[insertionPoint:])
				p.queuedBlocks[insertionPoint] = newBlock
				p.mu.Unlock()

				if len(p.getQueuedBlocks()) > blockQueueMaxSize {
					p.mu.Lock()
					p.queuedBlocks = p.queuedBlocks[len(p.queuedBlocks)-blockQueueMaxSize:]
					p.mu.Unlock()
				}
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// should only be called by blockLoop when then length of the queue has already been checked
func (p *Peer) sendTopBlock() {
	p.mu.Lock()
	block := p.queuedBlocks[0]
	p.queuedBlocks = p.queuedBlocks[1:]
	p.mu.Unlock()

	p.sentHead.Store(core.BlockRef{
		Height: block.Block.NumberU64(),
		Hash:   block.Block.Hash(),
	})

	p.Log().Debugf("after confirming height %v, immediately sending next block %v at height %v", p.GetConfirmedHead(), block.Block.Hash(), block.Block.NumberU64())
	err := p.sendNewBlock(block)
	if err != nil {
		p.Log().Errorf("could not send block %v: %v", block.Block.Hash(), err)
	}
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

// Name implements the eth.Packet interface.
func (*UpgradeStatusPacket) Name() string { return "UpgradeStatus" }

// Kind implements the eth.Packet interface.
func (*UpgradeStatusPacket) Kind() byte { return UpgradeStatusMsg }

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

// NotifyResponse informs any listeners dependent on a request/response call to this peer, indicating if any channels were waiting for the message
func (p *Peer) NotifyResponse(requestID uint64, packet eth.Packet) (bool, error) {
	responseCh, ok := p.ResponseQueue.LoadAndDelete(requestID)
	if !ok {
		return false, ErrUnknownRequestID
	}

	if responseCh != nil {
		responseCh <- packet
	}
	return responseCh != nil, nil
}

// UpdateHead sets the latest confirmed block on the peer. This may release or prune queued blocks on the peer connection.
func (p *Peer) UpdateHead(height uint64, hash common.Hash) {
	p.Log().Debugf("confirming new head (height=%v, hash=%v), newHeadCh len %v", height, hash, len(p.newHeadCh))
	select {
	case p.newHeadCh <- core.BlockRef{Height: height, Hash: hash}:
	default:
		p.Log().Errorf("newHeadCh channel is full(height=%v, hash=%v), dropping new head update", height, hash)
	}
}

// QueueNewBlock adds a new block to the queue to be sent to the peer in the order the peer is ready for.
func (p *Peer) QueueNewBlock(block *bxcommoneth.Block, td *big.Int) {
	packet := NewBlockPacket{
		Block:    &block.Block,
		TD:       td,
		Sidecars: block.Sidecars(),
	}
	p.newBlockCh <- packet
}

// AnnounceBlock pushes a new block announcement to the peer. This is used when the total difficult is unknown, and so a new block message would be invalid.
func (p *Peer) AnnounceBlock(hash common.Hash, number uint64) error {
	packet := eth.NewBlockHashesPacket{
		{Hash: hash, Number: number},
	}
	return p.send(eth.NewBlockHashesMsg, packet)
}

// SendBlockBodies sends a batch of block bodies to the peer
func (p *Peer) SendBlockBodies(bodies []*eth.BlockBody) error {
	return p.send(eth.BlockBodiesMsg, eth.BlockBodiesResponse(bodies))
}

// ReplyBlockBodies sends a batch of requested block bodies to the peer
func (p *Peer) ReplyBlockBodies(id uint64, bodies []*BlockBody) error {
	return p.send(eth.BlockBodiesMsg, BlockBodiesPacket{
		RequestID:           id,
		BlockBodiesResponse: bodies,
	})
}

// ReplyBlockHeaders sends batch of requested block headers to the peer
func (p *Peer) ReplyBlockHeaders(id uint64, headers []*ethtypes.Header) error {
	return p.send(eth.BlockHeadersMsg, eth.BlockHeadersPacket{
		RequestId:           id,
		BlockHeadersRequest: headers,
	})
}

// SendTransactions sends pushes a batch of transactions to the peer
func (p *Peer) SendTransactions(txs ethtypes.Transactions) error {
	return p.send(eth.TransactionsMsg, txs)
}

// RequestTransactions requests a batch of announced transactions from the peer
func (p *Peer) RequestTransactions(txHashes []common.Hash) error {
	packet := eth.GetPooledTransactionsRequest(txHashes)
	return p.send(eth.GetPooledTransactionsMsg, eth.GetPooledTransactionsPacket{
		RequestId:                    generateUint64(),
		GetPooledTransactionsRequest: packet,
	})
}

// SendPooledTransactionHashes67 sends transaction hashes to the peer
func (p *Peer) SendPooledTransactionHashes67(txHashes []common.Hash) error {
	return p.send(eth.NewPooledTransactionHashesMsg, NewPooledTransactionHashesPacket66(txHashes))
}

// SendPooledTransactionHashes sends transaction hashes to the peer
func (p *Peer) SendPooledTransactionHashes(txHashes []common.Hash, types []byte, sizes []uint32) error {
	return p.send(eth.NewPooledTransactionHashesMsg, eth.NewPooledTransactionHashesPacket{
		Hashes: txHashes,
		Types:  types,
		Sizes:  sizes,
	})
}

// ReplyPooledTransaction sends a batch of pooled transactions to the peer
func (p *Peer) ReplyPooledTransaction(id uint64, txs []rlp.RawValue) error {
	return p.send(eth.PooledTransactionsMsg, eth.PooledTransactionsRLPPacket{
		RequestId:                     id,
		PooledTransactionsRLPResponse: txs,
	})
}

// RequestBlock fetches the specified block from the ETH66 peer, pushing the components to the channels upon request/response completion.
func (p *Peer) RequestBlock(blockHash common.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) error {
	if !p.isVersion66() {
		panic("unexpected call to request block 66 for a <ETH66 peer")
	}

	getHeadersPacket := &eth.GetBlockHeadersRequest{
		Origin:  eth.HashOrNumber{Hash: blockHash},
		Amount:  1,
		Skip:    0,
		Reverse: false,
	}
	getBodiesPacket := eth.GetBlockBodiesRequest{blockHash}

	getHeadersRequestID := generateUint64()
	getHeadersPacket66 := eth.GetBlockHeadersPacket{
		RequestId:              getHeadersRequestID,
		GetBlockHeadersRequest: getHeadersPacket,
	}
	getBodiesRequestID := generateUint64()
	getBodiesPacket66 := eth.GetBlockBodiesPacket{
		RequestId:             getBodiesRequestID,
		GetBlockBodiesRequest: getBodiesPacket,
	}

	p.registerForResponse(getHeadersRequestID, headersCh)
	p.registerForResponse(getBodiesRequestID, bodiesCh)

	if err := p.send(eth.GetBlockHeadersMsg, getHeadersPacket66); err != nil {
		return err
	}
	if err := p.send(eth.GetBlockBodiesMsg, getBodiesPacket66); err != nil {
		return err
	}
	return nil
}

// RequestBlockHeader dispatches a request for the specified block header to the Ethereum peer with no expected handling on response
func (p *Peer) RequestBlockHeader(hash common.Hash) error {
	return p.RequestBlockHeaderRaw(eth.HashOrNumber{Hash: hash}, 1, 0, false, nil)
}

// RequestBlockHeaderRaw dispatches a generalized GetHeaders message to the Ethereum peer
func (p *Peer) RequestBlockHeaderRaw(origin eth.HashOrNumber, amount, skip uint64, reverse bool, responseCh chan eth.Packet) error {
	packet := eth.GetBlockHeadersRequest{
		Origin:  origin,
		Amount:  amount,
		Skip:    skip,
		Reverse: reverse,
	}

	requestID := generateUint64()
	p.registerForResponse(requestID, responseCh)

	return p.send(eth.GetBlockHeadersMsg, eth.GetBlockHeadersPacket{
		RequestId:              requestID,
		GetBlockHeadersRequest: &packet,
	})
}

func (p *Peer) sendNewBlock(packet NewBlockPacket) error {
	// For BSC, the timestamp in block header is an expected time provided by the validator, but sometimes it arrives at BDN before the timestamp provided
	if p.chainID == network.BSCMainnetChainID || p.chainID == network.BSCTestnetChainID {
		// delay if the header timestamp is later than the system timestamp
		blockHeaderTimestamp := int64(packet.Block.Time()) * time.Second.Nanoseconds() //nolint:gosec
		currentTimestamp := p.clock.Now().UnixNano()
		delayNanoseconds := blockHeaderTimestamp - currentTimestamp
		if delayNanoseconds > 0 {
			if delayNanoseconds > delayLimit.Nanoseconds() {
				p.log.Warnf("received future block from BDN that is too far ahead by %v ms, block hash %v, block height %v, - not sending to peer", packet.Block.Hash().String(), packet.Block.Number().String(), time.Duration(delayNanoseconds).Milliseconds())
				return nil
			}
			go p.clock.AfterFunc(time.Duration(delayNanoseconds), func() {
				if err := p.sendNewBlock(packet); err != nil {
					p.Log().Errorf("failed to send future block %v at height %v after %v ms delay: %v",
						packet.Block.Hash().String(), packet.Block.Number().String(), time.Duration(delayNanoseconds).Milliseconds(), err)
				}
			})
			p.log.Tracef("received future block from blockchain peer, the block will be sent with %vms delay, block hash %v, block height %v",
				packet.Block.Hash().String(), packet.Block.Number().String(), time.Duration(delayNanoseconds).Milliseconds())
			return nil
		}
	}

	err := p.send(eth.NewBlockMsg, packet)
	if err != nil {
		return err
	}

	if p.RequestConfirmations {
		go func() {
			blockNumber := packet.Block.NumberU64()
			blockHash := packet.Block.Hash()
			count := 0

			// check for fast confirmation
			ticker := p.clock.Ticker(fastBlockConfirmationInterval)
			defer ticker.Stop()
			for count < fastBlockConfirmationAttempts {
				<-ticker.Alert()
				if p.GetConfirmedHead().Height >= blockNumber {
					return
				}
				err := p.RequestBlockHeader(blockHash)
				if err != nil {
					return
				}
				count++
			}

			// check for slower confirmation
			ticker = p.clock.Ticker(slowBlockConfirmationInterval)
			for {
				<-ticker.Alert()
				if p.GetConfirmedHead().Height >= blockNumber {
					return
				}
				err := p.RequestBlockHeader(blockHash)
				if err != nil {
					return
				}
			}
		}()
	}
	return nil
}

func (p *Peer) registerForResponse(requestID uint64, responseCh chan eth.Packet) {
	p.ResponseQueue.Store(requestID, responseCh)
}

// IsDisconnected checks if the peer is disconnected
func (p *Peer) IsDisconnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.disconnected
}

func (p *Peer) send(msgCode uint64, data interface{}) error {
	if p.IsDisconnected() {
		return nil
	}

	job := sendJob{code: msgCode, data: data}
	select {
	case p.sendCh <- job:
		return nil
	default:
		p.log.Warnf("send queue full for %v msgCode %d", p.IPEndpoint(), msgCode)
		return fmt.Errorf("send enqueue full")
	}
}

// Handshake executes the Ethereum protocol Handshake. Unlike Geth, the gateway waits for the peer status message before sending its own, to replicate some peer status fields.
func (p *Peer) Handshake(networkChain uint64, totalDifficulty *big.Int, head common.Hash, genesis common.Hash, executionLayerForks []string) (*eth.StatusPacket, error) {
	var peerStatus eth.StatusPacket
	err := p.readStatusMessage(&peerStatus, eth.StatusMsg)
	if err != nil {
		return nil, err
	}

	p.ConfirmedHead.Store(core.BlockRef{Hash: head})

	// for ethereum override the TD and the Head as get from the peer
	// Nethermind checks reject connections that bring old value
	if networkChain == network.EthMainnetChainID {
		totalDifficulty = peerStatus.TD
		head = peerStatus.Head
	}
	// used the same fork ID as received from peer; gateway is expected to usually be compatible with Ethereum peer
	err = p.send(eth.StatusMsg, &eth.StatusPacket{
		ProtocolVersion: uint32(p.version),
		NetworkID:       networkChain,
		TD:              totalDifficulty,
		Head:            head,
		Genesis:         genesis,
		ForkID:          peerStatus.ForkID,
	})
	if err != nil {
		return nil, err
	}

	if p.version >= eth.ETH68 && (networkChain == network.BSCMainnetChainID || networkChain == network.BSCTestnetChainID) {
		_, err = p.upgradeStatus()
		if err != nil {
			p.Log().Errorf("failed to upgrade status: %v", err)
		}
	}

	if peerStatus.NetworkID != networkChain {
		if !p.Dynamic() {
			p.Log().Infof("network ID does not match: expected %v, but got %v", networkChain, peerStatus.NetworkID)
		} else {
			p.Log().Tracef("network ID does not match: expected %v, but got %v", networkChain, peerStatus.NetworkID)
		}
		return nil, fmt.Errorf("network ID does not match: expected %v, but got %v", networkChain, peerStatus.NetworkID)
	}

	if peerStatus.ProtocolVersion != uint32(p.version) {
		if !p.Dynamic() {
			p.Log().Infof("protocol version does not match: expected %v, but got %v", p.version, peerStatus.ProtocolVersion)
		} else {
			p.Log().Tracef("protocol version does not match: expected %v, but got %v", p.version, peerStatus.ProtocolVersion)
		}
		return nil, fmt.Errorf("protocol version does not match: expected %v, but got %v", p.version, peerStatus.ProtocolVersion)
	}

	if peerStatus.Genesis != genesis {
		if !p.Dynamic() {
			p.Log().Infof("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
		} else {
			p.Log().Tracef("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
		}
		return nil, fmt.Errorf("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
	}

	if len(executionLayerForks) > 0 {
		b64ForkID := b64.StdEncoding.EncodeToString(peerStatus.ForkID.Hash[:])

		if !utils.Exists(b64ForkID, executionLayerForks) {
			if !p.Dynamic() {
				p.Log().Infof("fork ID does not match: expected %v, but got %v", executionLayerForks, b64ForkID)
			} else {
				p.Log().Tracef("fork ID does not match: expected %v, but got %v", executionLayerForks, b64ForkID)
			}
			return nil, fmt.Errorf("fork ID does not match: expected %v, but got %v", executionLayerForks, b64ForkID)
		}
	}

	p.endpoint.Version = int(peerStatus.ProtocolVersion)
	p.endpoint.Name = p.Peer.Fullname()
	p.endpoint.ConnectedAt = time.Now().Format(time.RFC3339)

	// send Empty NewPooledTransactionHashes based the protocol
	var msg interface{}
	switch p.version {
	case eth.ETH68:
		msg = eth.NewPooledTransactionHashesPacket{}
	default:
		msg = NewPooledTransactionHashesPacket66{}
	}
	if err := p.send(eth.NewPooledTransactionHashesMsg, &msg); err != nil {
		p.Log().Errorf("error sending empty NewPooledTransactionHashesMsg message after handshake %v", err)
	}

	return &peerStatus, nil
}

func (p *Peer) upgradeStatus() (*UpgradeStatusExtension, error) {
	var upgradeStatus UpgradeStatusPacket
	err := p.readStatusMessage(&upgradeStatus, UpgradeStatusMsg)
	if err != nil {
		return nil, err
	}

	extension, err := upgradeStatus.GetExtension()
	if err != nil {
		return nil, err
	}

	if err := p.send(UpgradeStatusMsg, upgradeStatus); err != nil {
		return extension, err
	}

	return extension, nil
}

type messageChanResponse struct {
	msg p2p.Msg
	err error
}

func (p *Peer) readStatusMessage(status interface{}, code uint64) error {
	messageChan := make(chan messageChanResponse, 1)

	go func() {
		msg, err := p.rw.ReadMsg()
		messageChan <- messageChanResponse{msg, err}
	}()

	timeout := time.NewTicker(readStatusTimeout)
	var msg p2p.Msg
	select {
	case message := <-messageChan:
		if message.err != nil {
			return message.err
		}
		msg = message.msg
	case <-timeout.C:
		timeout.Stop()
		return fmt.Errorf("failed to get status message after %v from peer", readStatusTimeout)
	}
	defer func() {
		_ = msg.Discard() //nolint:errcheck
	}()

	if msg.Code != code {
		return fmt.Errorf("unexpected message: %v, should have been %v", msg.Code, code)
	}

	if msg.Size > maxMessageSize {
		return fmt.Errorf("message is too big: %v > %v", msg.Size, maxMessageSize)
	}

	if err := msg.Decode(status); err != nil {
		return fmt.Errorf("could not decode status message: %v", err)
	}

	return nil
}

// generateUint64 generates a cryptographically secure random uint64
func generateUint64() uint64 {
	var b [8]byte
	// it never returns an error, and always fills b entirely.
	_, _ = rand.Read(b[:]) //nolint:errcheck

	return binary.LittleEndian.Uint64(b[:])
}
