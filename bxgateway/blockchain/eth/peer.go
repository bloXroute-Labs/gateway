package eth

import (
	"context"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/utils"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"math/big"
	"math/rand"
	"strconv"
	"time"
)

const (
	maxMessageSize                  = 10 * 1024 * 1024
	responseQueueSize               = 10
	responseTimeout                 = 5 * time.Minute
	blockConfirmationInterval       = 100 * time.Millisecond
	blockConfirmationAttempts       = 5
	maxIntervalBetweenBlocks        = 1 * time.Second
	headChannelBacklog              = 10
	blockChannelBacklog             = 10
	blockConfirmationChannelBacklog = 10
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
	endpoint types.NodeEndpoint
	clock    utils.Clock
	ctx      context.Context
	cancel   context.CancelFunc
	log      *log.Entry

	disconnected     bool
	checkpointPassed bool

	responseQueue   chan chan eth.Packet // chan is used as a concurrency safe queue
	responseQueue66 cmap.ConcurrentMap

	newHeadCh           chan uint64
	newBlockCh          chan *eth.NewBlockPacket
	blockConfirmationCh chan common.Hash
	confirmedHead       uint64
	sentHead            uint64
	queuedBlocks        []*eth.NewBlockPacket
}

// NewPeer returns a wrapped Ethereum peer
func NewPeer(ctx context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint) *Peer {
	return newPeer(ctx, p, rw, version, utils.RealClock{})
}

func newPeer(parent context.Context, p *p2p.Peer, rw p2p.MsgReadWriter, version uint, clock utils.Clock) *Peer {
	ctx, cancel := context.WithCancel(parent)
	peer := &Peer{
		p:                   p,
		rw:                  rw,
		version:             version,
		ctx:                 ctx,
		cancel:              cancel,
		endpoint:            types.NodeEndpoint{IP: p.Node().IP().String(), Port: p.Node().TCP(), PublicKey: p.Info().Enode},
		clock:               clock,
		newHeadCh:           make(chan uint64, headChannelBacklog),
		newBlockCh:          make(chan *eth.NewBlockPacket, blockChannelBacklog),
		blockConfirmationCh: make(chan common.Hash, blockConfirmationChannelBacklog),
		queuedBlocks:        make([]*eth.NewBlockPacket, 0),
		responseQueue:       make(chan chan eth.Packet, responseQueueSize),
		responseQueue66:     cmap.New(),
	}
	peerID := p.ID()
	peer.log = log.WithFields(log.Fields{
		"connType":   "ETH",
		"remoteAddr": p.RemoteAddr(),
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

// Log returns the context logger for the peer connection
func (ep *Peer) Log() *log.Entry {
	return ep.log
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
	timer := ep.clock.Timer(maxIntervalBetweenBlocks)
	for {
		select {
		case newHead := <-ep.newHeadCh:
			if newHead < ep.confirmedHead {
				break
			}

			// update most recent blocks
			ep.confirmedHead = newHead
			if newHead > ep.sentHead {
				ep.sentHead = newHead
			}

			// prune entries on confirmedHead
			for i, block := range ep.queuedBlocks {
				if block.Block.NumberU64() > ep.confirmedHead {
					for _, prunedBlock := range ep.queuedBlocks[i:] {
						ep.Log().Debugf("pruning now stale block %v at height %v after confirming %v", prunedBlock.Block.Hash(), prunedBlock.Block.NumberU64(), newHead)
					}
					ep.queuedBlocks = ep.queuedBlocks[i:]
					break
				}
			}

			// release next block if ready
			if len(ep.queuedBlocks) > 0 && ep.queuedBlocks[0].Block.NumberU64() == ep.confirmedHead+1 {
				ep.Log().Debugf("after confirming height %v, immediately sending next block %v at height %v", newHead, ep.queuedBlocks[0].Block.Hash(), ep.queuedBlocks[0].Block.NumberU64())
				ep.sendTopBlock()
			}

			// reset timer if blocks are still waiting
			if len(ep.queuedBlocks) > 0 {
				timer.Reset(maxIntervalBetweenBlocks)
			} else {
				timer.Stop()
			}
		case newBlock := <-ep.newBlockCh:
			newBlockNumber := newBlock.Block.NumberU64()
			newBlockHash := newBlock.Block.Hash()

			// always ignore stales blocks
			if newBlockNumber <= ep.sentHead && ep.sentHead != 0 {
				ep.Log().Debugf("skipping queued stale block %v at height %v, already sent or confirmed block of height %v", newBlockHash, newBlockNumber, ep.sentHead)
				break
			}

			// release next block if ready
			if newBlockNumber == ep.confirmedHead+1 || ep.confirmedHead == 0 {
				ep.Log().Debugf("queued block %v at height %v is next expected (%v), sending immediately", newBlockHash, newBlockNumber, ep.confirmedHead+1)
				err := ep.sendNewBlock(newBlock)
				if err != nil {
					ep.Log().Errorf("could not send block %v to peer %v: %v", newBlockHash, ep, err)
				}
				ep.sentHead = newBlockNumber
				timer.Reset(maxIntervalBetweenBlocks)
			} else {
				// find position in block queue that block should be inserted at
				insertionPoint := len(ep.queuedBlocks)
				for i, block := range ep.queuedBlocks {
					// never send multiple blocks at same height
					if block.Block.NumberU64() == newBlock.Block.NumberU64() {
						insertionPoint = -1
						break
					}

					if block.Block.NumberU64() > newBlock.Block.NumberU64() {
						insertionPoint = i
					}
				}

				ep.Log().Debugf("queuing block %v at height %v behind %v blocks", newBlockHash, newBlockNumber, insertionPoint)

				if insertionPoint != -1 {
					// insert block at insertion point
					ep.queuedBlocks = append(ep.queuedBlocks, &eth.NewBlockPacket{})
					copy(ep.queuedBlocks[insertionPoint+1:], ep.queuedBlocks[insertionPoint:])
					ep.queuedBlocks[insertionPoint] = newBlock

					if len(ep.queuedBlocks) == 1 {
						timer.Reset(maxIntervalBetweenBlocks)
					}
				}
			}
		case <-timer.Alert():
			if len(ep.queuedBlocks) > 0 {
				block := ep.sendTopBlock()
				ep.Log().Debugf("next block %v at height %v has been released after timeout", block.Block.Hash(), block.Block.NumberU64())
				timer.Reset(maxIntervalBetweenBlocks)
			}
		case <-ep.ctx.Done():
			return
		}
	}
}

// should only be called by blockLoop when then length of the queue has already been checked
func (ep *Peer) sendTopBlock() *eth.NewBlockPacket {
	block := ep.queuedBlocks[0]
	ep.queuedBlocks = ep.queuedBlocks[1:]
	ep.sentHead = block.Block.NumberU64()

	err := ep.sendNewBlock(block)
	if err != nil {
		ep.Log().Errorf("could not send block %v to peer %v: %v", block.Block.Hash(), ep, err)
	}
	return block
}

// Handshake executes the Ethereum protocol Handshake. Unlike Geth, the gateway waits for the peer status message before sending its own, in order to replicate some peer status fields.
func (ep *Peer) Handshake(version uint32, network uint64, td *big.Int, head common.Hash, genesis common.Hash) (*eth.StatusPacket, error) {
	peerStatus, err := ep.readStatus()
	if err != nil {
		return peerStatus, err
	}

	// used same fork ID as received from peer; gateway is expected to usually be compatible with Ethereum peer
	err = ep.send(eth.StatusMsg, &eth.StatusPacket{
		ProtocolVersion: version,
		NetworkID:       network,
		TD:              td,
		Head:            head,
		Genesis:         genesis,
		ForkID:          peerStatus.ForkID,
	})

	if peerStatus.NetworkID != network {
		return peerStatus, fmt.Errorf("network ID does not match: expected %v, but got %v", network, peerStatus.NetworkID)
	}

	if peerStatus.ProtocolVersion != version {
		return peerStatus, fmt.Errorf("protocol version does not match: expected %v, but got %v", version, peerStatus.ProtocolVersion)
	}

	if peerStatus.Genesis != genesis {
		return peerStatus, fmt.Errorf("genesis block does not match: expected %v, but got %v", genesis, peerStatus.Genesis)
	}

	return peerStatus, nil
}

func (ep *Peer) readStatus() (*eth.StatusPacket, error) {
	var status eth.StatusPacket

	msg, err := ep.rw.ReadMsg()
	if err != nil {
		return nil, err
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

	if err = msg.Decode(&status); err != nil {
		return &status, fmt.Errorf("could not decode status message: %v", err)
	}

	return &status, nil
}

// NotifyResponse informs any listeners dependent on a request/response call to this peer
func (ep *Peer) NotifyResponse(packet eth.Packet) {
	responseCh := <-ep.responseQueue
	if responseCh != nil {
		responseCh <- packet
	}
}

// NotifyResponse66 informs any listeners dependent on a request/response call to this ETH66 peer
func (ep *Peer) NotifyResponse66(requestID uint64, packet eth.Packet) error {
	rawResponseCh, ok := ep.responseQueue66.Pop(convertRequestIDKey(requestID))
	if !ok {
		return ErrUnknownRequestID
	}

	responseCh := rawResponseCh.(chan eth.Packet)
	if responseCh != nil {
		responseCh <- packet
	}
	return nil
}

// UpdateHead sets the latest confirmed block on the peer. This may release or prune queued blocks on the peer connection.
func (ep *Peer) UpdateHead(height uint64) {
	ep.Log().Debugf("confirming height %v", height)
	ep.newHeadCh <- height
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

// RequestBlockHeader dispatches a request for the specified block header to the Ethereum peer.
func (ep *Peer) RequestBlockHeader(hash common.Hash) error {
	packet := eth.GetBlockHeadersPacket{
		Origin:  eth.HashOrNumber{Hash: hash},
		Amount:  1,
		Skip:    0,
		Reverse: false,
	}
	if ep.isVersion66() {
		requestID := rand.Uint64()
		ep.registerForResponse66(requestID, nil)

		return ep.send(eth.GetBlockHeadersMsg, eth.GetBlockHeadersPacket66{
			RequestId:             requestID,
			GetBlockHeadersPacket: &packet,
		})
	}

	// intercept and discard handler on response
	ep.registerForResponse(nil)
	return ep.send(eth.GetBlockHeadersMsg, packet)
}

func (ep *Peer) sendNewBlock(packet *eth.NewBlockPacket) error {
	err := ep.send(eth.NewBlockMsg, packet)
	if err != nil {
		return err
	}
	go func() {
		blockNumber := packet.Block.NumberU64()
		blockHash := packet.Block.Hash()

		timer := ep.clock.Timer(blockConfirmationInterval)
		count := 0

		for count < blockConfirmationAttempts {
			<-timer.Alert()

			if ep.confirmedHead >= blockNumber {
				return
			}

			err := ep.RequestBlockHeader(blockHash)
			if err != nil {
				return
			}

			// TODO: implement ticker
			timer.Reset(blockConfirmationInterval)
			count++
		}
	}()
	return nil
}

func (ep *Peer) registerForResponse(responseCh chan eth.Packet) {
	ep.responseQueue <- responseCh
}

func (ep *Peer) registerForResponse66(requestID uint64, responseCh chan eth.Packet) {
	ep.responseQueue66.Set(convertRequestIDKey(requestID), responseCh)
}

func (ep *Peer) send(msgCode uint64, data interface{}) error {
	if ep.disconnected {
		return nil
	}
	return p2p.Send(ep.rw, msgCode, data)
}

func convertRequestIDKey(requestID uint64) string {
	return strconv.FormatUint(requestID, 10)
}
