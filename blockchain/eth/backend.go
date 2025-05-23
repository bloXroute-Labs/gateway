package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/google/uuid"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/datatype"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	checkpointTimeout    = 5 * time.Second
	maxFutureBlockNumber = 100
)

// Backend represents the interface to which any stateful message handling (e.g. looking up tx pool items or block headers) will be passed to for processing
type Backend interface {
	NetworkConfig() *network.EthConfig
	RunPeer(peer *Peer, wg *sync.WaitGroup, handler func(*Peer) error) error
	Handle(peer *Peer, packet eth.Packet) error
	GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethtypes.Header, error)
	GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error)
	GetBlobSidecars(hashes []ethcommon.Hash) ([]bxcommoneth.BlobSidecars, error)
	GetBridge() blockchain.Bridge
	RequestTransactions(hashes []ethcommon.Hash) ([]rlp.RawValue, error)
}

// Handler is the Ethereum backend implementation. It passes transactions and blocks to the BDN bridge and tracks received blocks and transactions from peers.
type Handler struct {
	chain            *Chain
	bridge           blockchain.Bridge
	statusBridge     blockchain.StatusSubscription
	peers            *peerSet
	cancel           context.CancelFunc
	config           *network.EthConfig
	wsManager        blockchain.WSManager
	recommendedPeers map[string]struct{}
	responseQueue    *syncmap.SyncMap[string, chan interface{}]
}

// NewHandler returns a new Handler and starts its processing go routines
func NewHandler(parent context.Context, config *network.EthConfig, chain *Chain, bridge blockchain.Bridge, wsManager blockchain.WSManager, recommendedPeers map[string]struct{}) *Handler {
	ctx, cancel := context.WithCancel(parent)
	h := &Handler{
		config:           config,
		chain:            chain,
		bridge:           bridge,
		statusBridge:     bridge.SubscribeStatus(false),
		peers:            newPeerSet(),
		cancel:           cancel,
		wsManager:        wsManager,
		recommendedPeers: recommendedPeers,
		responseQueue:    syncmap.NewStringMapOf[chan interface{}](),
	}
	go h.checkInitialBlockchainLiveliness(100 * time.Second)
	go h.handleBDNBridge(ctx)
	return h
}

// handleFeeds processes the heads and txhashes feeds. exit with false on feed error and true when done
func (h *Handler) handleFeeds(wsCtx context.Context, nodeWS blockchain.WSProvider, newHeadsRespCh chan *ethtypes.Header, newPendingTxsRespCh chan ethcommon.Hash, newHeadsErrCh <-chan error, newPendingTxsErrCh <-chan error) bool {
	activeFeeds := false
	activeFeedsCheckTicker := time.NewTicker(time.Second * 10)
	defer func() {
		activeFeedsCheckTicker.Stop()
		h.wsManager.UpdateNodeSyncStatus(nodeWS.BlockchainPeerEndpoint(), blockchain.Unsynced)
	}()
	for {
		select {
		case <-wsCtx.Done():
			nodeWS.Log().Debugf("Stopping handleFeeds ctx Done process %v", utils.GetGID())
			return true
		case err := <-newHeadsErrCh:
			nodeWS.Log().Errorf("failed to get notification from newHeads: %v  process %v", err, utils.GetGID())
			return false
		case newHeader := <-newHeadsRespCh:
			nodeWS.Log().Tracef("received header for block %v (height %v)", newHeader.Hash(), newHeader.Number)
			h.confirmBlockFromWS(newHeader.Hash(), newHeader.Number, nodeWS.BlockchainPeer().(*Peer))
		case err := <-newPendingTxsErrCh:
			nodeWS.Log().Errorf("failed to get notification from newPendingTransactions: %v  process %v", err, utils.GetGID())
			return false
		case newPendingTx := <-newPendingTxsRespCh:
			activeFeeds = true
			txHash, err := types.NewSHA256Hash(newPendingTx[:])
			if err != nil {
				nodeWS.Log().Errorf("got an error from NewSHA256Hash for %v - %v", newPendingTx, err)
				continue
			}
			hashes := types.SHA256HashList{txHash}
			// read all pending txs from channel into the list
			morePendingTxs := true
			for morePendingTxs && len(hashes) < bxgateway.MaxAnnouncementFromNode {
				select {
				case newPendingTx = <-newPendingTxsRespCh:
					txHash, err = types.NewSHA256Hash(newPendingTx[:])
					if err != nil {
						nodeWS.Log().Errorf("got an error from NewSHA256Hash for %v - %v", newPendingTx, err)
						continue
					}
					hashes = append(hashes, txHash)
				default:
					morePendingTxs = false
				}
			}

			nodeWS.Log().Tracef("received %v new pending txs. All pending %v, hashes %v", len(hashes), !morePendingTxs, hashes)

			err = h.bridge.AnnounceTransactionHashes(bxgateway.WSConnectionID, hashes, nodeWS.BlockchainPeerEndpoint())
			if err != nil {
				nodeWS.Log().Errorf("failed to send %v transactions to the gateway: %v  process %v", len(hashes), err, utils.GetGID())
			}
		case <-activeFeedsCheckTicker.C:
			if !activeFeeds {
				h.wsManager.UpdateNodeSyncStatus(nodeWS.BlockchainPeerEndpoint(), blockchain.Unsynced)
			} else {
				h.wsManager.UpdateNodeSyncStatus(nodeWS.BlockchainPeerEndpoint(), blockchain.Synced)
			}
			activeFeeds = false
		}
	}
}

// runEthSub is running as a go routine. Can sleep if needed
func (h *Handler) runEthSub(wsCtx context.Context, nodeWS blockchain.WSProvider, wg *sync.WaitGroup) {
	var newPendingTxsRespCh chan ethcommon.Hash
	var newPendingTxsErrCh <-chan error

	var newHeadsRespCh chan *ethtypes.Header
	var newHeadsErrCh <-chan error

	nodeWS.Log().Debugf("starting runEthSub... process %v", utils.GetGID())
	defer wg.Done()
	defer nodeWS.Log().Debugf("runEthSub ends process %v", utils.GetGID())

	for {
		nodeWS.Dial()

		// In Ethereum PoS consensus layer is responsible for blocks and confirmations even though heads are still avaliable from websocket of execution layer
		// eth.Chain clean is not working well because it has no blocks
		if h.config.Network != network.EthMainnetChainID {
			newHeadsRespCh = make(chan *ethtypes.Header)

			newHeadsSub, err := nodeWS.Subscribe(newHeadsRespCh, "newHeads")
			if err != nil {
				nodeWS.Log().Errorf("unable to subscribe to newHeads - %v process %v", err, utils.GetGID())
				nodeWS.Close()
				continue
			}

			newHeadsErrCh = newHeadsSub.Sub.(*rpc.ClientSubscription).Err()
		}

		newPendingTxsRespCh = make(chan ethcommon.Hash)
		newPendingTxsSub, err := nodeWS.Subscribe(newPendingTxsRespCh, "newPendingTransactions")
		if err != nil {
			nodeWS.Log().Errorf("unable to subscribe to newPendingTransactions - %v  process %v", err, utils.GetGID())
			nodeWS.Close()
			continue
		}
		newPendingTxsErrCh = newPendingTxsSub.Sub.(*rpc.ClientSubscription).Err()

		done := h.handleFeeds(wsCtx, nodeWS, newHeadsRespCh, newPendingTxsRespCh, newHeadsErrCh, newPendingTxsErrCh)
		// we had a feed error. Reconnect and resubscribe
		nodeWS.Close()
		if done { // if closure is expected, just exit
			return
		}
	}
}

// Stop shutdowns down all the handler goroutines
func (h *Handler) Stop() {
	h.cancel()
}

// NetworkConfig returns the backend's network configuration
func (h *Handler) NetworkConfig() *network.EthConfig {
	return h.config
}

// RunPeer registers a peer within the peer set and starts handling all its messages
func (h *Handler) RunPeer(ep *Peer, wg *sync.WaitGroup, handler func(*Peer) error) error {
	_, isRecommended := h.recommendedPeers[fmt.Sprintf("%s:%d", ep.endpoint.IP, ep.endpoint.Port)]
	if !ep.Dynamic() && !isRecommended {
		ok := h.wsManager.SetBlockchainPeer(ep)
		if !ok {
			log.Warnf("unable to set blockchain peer for %v: no corresponding websockets provider found (ok if websockets URI was omitted)", ep.endpoint.IPPort())
		}
		if ws, ok := h.wsManager.Provider(&ep.endpoint); ok {
			wg.Add(1)
			go h.runEthSub(ep.ctx, ws, wg)
		}
	}
	if err := h.peers.register(ep); err != nil {
		return err
	}
	defer func() {
		if !ep.Dynamic() && !isRecommended {
			ok := h.wsManager.UnsetBlockchainPeer(ep.endpoint)
			if !ok {
				log.Warnf("unable to unset blockchain peer for %v: no corresponding websockets provider found (ok if websockets URI was omitted)", ep.endpoint.IPPort())
			}
		}
		// cancel context. Should stop the wsProvider
		ep.Stop()
		_ = h.peers.unregister(ep.ID())
	}()

	ep.Start()
	time.AfterFunc(checkpointTimeout, func() {
		ep.checkpointPassed = true
	})
	return handler(ep)
}

func (h *Handler) handleBDNBridge(ctx context.Context) {
	for {
		select {
		case bdnTxs := <-h.bridge.ReceiveRequestedTransactionsFromBDN():
			if err := h.notifyResponse(bdnTxs.RequestID, bdnTxs.Transactions); err != nil {
				log.Warnf("could not send requested transactions to peer: %v", err)
			}
		case bdnTxs := <-h.bridge.ReceiveBDNTransactions():
			readMore := true
			endpointToTxs := make(map[types.NodeEndpoint]*blockchain.Transactions)
			endpointToTxs[bdnTxs.PeerEndpoint] = &bdnTxs
			for readMore {
				select {
				case moreBdnTxs := <-h.bridge.ReceiveBDNTransactions():
					if tx, ok := endpointToTxs[moreBdnTxs.PeerEndpoint]; !ok {
						endpointToTxs[moreBdnTxs.PeerEndpoint] = &moreBdnTxs
					} else {
						tx.Transactions = append(tx.Transactions, moreBdnTxs.Transactions...)
					}
				default:
					readMore = false
				}
			}

			for _, sendingTxs := range endpointToTxs {
				h.processBDNTransactions(*sendingTxs)
			}
		case request := <-h.bridge.ReceiveTransactionHashesRequest():
			h.processBDNTransactionRequests(request)
		case bdnBlock := <-h.bridge.ReceiveEthBlockFromBDN():
			h.processBDNBlock(bdnBlock)
		case config := <-h.bridge.ReceiveNetworkConfigUpdates():
			h.config.Update(config)
		case <-h.statusBridge.ReceiveBlockchainStatusRequest():
			h.processBlockchainStatusRequest()
		case <-h.bridge.ReceiveNodeConnectionCheckRequest():
			h.processNodeConnectionCheckRequest()
		case endpoint := <-h.bridge.ReceiveDisconnectEvent():
			h.processDisconnectEvent(endpoint)
		case <-ctx.Done():
			return
		}
	}
}

// RequestTransactions requests transactions from the BDN
func (h *Handler) RequestTransactions(hashes []ethcommon.Hash) ([]rlp.RawValue, error) {
	respChan := make(chan interface{})

	requestID := uuid.New().String()
	h.responseQueue.Store(requestID, respChan)

	bxHashes := make(types.SHA256HashList, len(hashes))
	for i, hash := range hashes {
		bxHashes[i] = types.SHA256Hash(hash)
	}

	if err := h.bridge.RequestTransactionsFromBDN(requestID, bxHashes); err != nil {
		return nil, fmt.Errorf("could not request transactions from BDN: %v", err)
	}

	bdnTxs := (<-respChan).([]*types.BxTransaction)
	txs := make([]rlp.RawValue, len(bdnTxs))
	for i, bdnTx := range bdnTxs {
		txs[i] = rlp.RawValue(bdnTx.Content())
	}

	return txs, nil
}

func (h *Handler) notifyResponse(requestID string, data interface{}) error {
	responseCh, ok := h.responseQueue.LoadAndDelete(requestID)
	if !ok {
		return ErrUnknownRequestID
	}

	responseCh <- data

	return nil
}

func (h *Handler) processBDNTransactions(bdnTxs blockchain.Transactions) {
	var (
		pooledTransactionHashes []ethcommon.Hash
		pooledTypes             []byte
		pooledSizes             []uint32
	)

	p := datatype.NewProcessingETHTransaction(len(bdnTxs.Transactions))
	for _, bdnTx := range bdnTxs.Transactions {
		blockchainTx, err := h.bridge.TransactionBDNToBlockchain(bdnTx)
		if err != nil {
			logTransactionConverterFailure(err, bdnTx)
			continue
		}

		ethTx, ok := blockchainTx.(*ethtypes.Transaction)
		if !ok {
			logTransactionConverterFailure(err, bdnTx)
			continue
		}

		// Blob transactions should be sent via PoolTransactions message
		if ethTx.Type() == ethtypes.BlobTxType {
			// For some reason blob tx with a missed sidecar passes the tx store check
			if ethTx.BlobTxSidecar() != nil {
				pooledTransactionHashes = append(pooledTransactionHashes, ethTx.Hash())
				pooledTypes = append(pooledTypes, ethTx.Type())
				pooledSizes = append(pooledSizes, uint32(ethTx.Size()))
			} else {
				log.Debugf("blob tx from BDN %s has no sidecar data", ethTx.Hash())
			}

			continue
		}

		// allow sending tx to inbound node only if it's paid tx or marked as deliver to node, but it cannot be validators only
		p.Add(ethTx, (bdnTx.Flags().IsPaidTx() || bdnTx.Flags().IsDeliverToNode()) && !bdnTx.Flags().IsValidatorsOnly())
	}

	h.broadcastTransactions(p, bdnTxs.PeerEndpoint, bdnTxs.ConnectionType)

	if len(pooledTransactionHashes) > 0 {
		h.broadcastPooledTransactionHashes(pooledTransactionHashes, pooledTypes, pooledSizes)
	}
}

func (h *Handler) processBDNTransactionRequests(request blockchain.TransactionAnnouncement) {
	ethTxHashes := make([]ethcommon.Hash, 0, len(request.Hashes))

	for _, txHash := range request.Hashes {
		ethTxHashes = append(ethTxHashes, ethcommon.BytesToHash(txHash[:]))
	}

	peer, ok := h.peers.get(request.PeerID)
	if !ok {
		log.Warnf("peer %v announced %v hashes, but is not available for querying", request.PeerID, len(ethTxHashes))
		return
	}

	err := peer.RequestTransactions(ethTxHashes)
	if err != nil {
		peer.Log().Errorf("could not request %v transactions: %v", len(ethTxHashes), err)
	}
}

func (h *Handler) processBDNBlock(bdnBlock *types.BxBlock) {
	ethBlockInfo, err := h.storeBDNBlock(bdnBlock)
	if err != nil {
		logBlockConverterFailure(err, bdnBlock)
		return
	}

	// nil if block is a duplicate and does not need processing
	if ethBlockInfo == nil {
		return
	}
	block := ethBlockInfo.Block

	// In PoS the block difficulty is 0
	if block.Difficulty() == nil || block.Difficulty().Cmp(big.NewInt(0)) == 0 {
		return
	}

	err = h.chain.SetTotalDifficulty(ethBlockInfo)
	if err != nil {
		log.Debugf("could not resolve difficulty for block %v, announcing instead", block.Hash().String())
		h.broadcastBlockAnnouncement(block)
	} else {
		h.broadcastBlock(block, ethBlockInfo.TotalDifficulty(), nil)
	}
}

func (h *Handler) processBlockchainStatusRequest() {
	nodes := make([]*types.NodeEndpoint, 0)

	for _, peer := range h.peers.getAll() {
		endpoint := peer.IPEndpoint()
		nodes = append(nodes, &endpoint)
	}

	err := h.statusBridge.SendBlockchainStatusResponse(blockchain.BxStatus{Endpoints: nodes})
	if err != nil {
		log.Errorf("send blockchain status response: %v", err)
	}
}

func (h *Handler) processNodeConnectionCheckRequest() {
	var endpoint types.NodeEndpoint
	for _, peer := range h.peers.getAll() {
		endpoint = peer.IPEndpoint()
		if endpoint.IP == "" || endpoint.Port == 0 {
			continue
		}
		break
	}

	if endpoint.IP == "" || endpoint.Port == 0 {
		log.Errorf("try to send blockchain status with invalid blockchain ip: %v or post: %v", endpoint.IP, endpoint.Port)
		return
	}
	err := h.bridge.SendNodeConnectionCheckResponse(endpoint)
	if err != nil {
		log.Errorf("send blockchain status response: %v", err)
	}
}

func (h *Handler) processDisconnectEvent(endpoint types.NodeEndpoint) {
	// check if the peer is in the connections
	for _, peer := range h.peers.getAll() {
		if (endpoint.PublicKey != "" && endpoint.PublicKey == peer.IPEndpoint().PublicKey) || (endpoint.PublicKey == "" && endpoint.IPPort() == peer.IPEndpoint().IPPort()) {
			peer.Disconnect(p2p.DiscUselessPeer)
			return
		}
	}

	log.Debugf("Peer was not found %v", endpoint)
}

func (h *Handler) awaitBlockResponse(peer *Peer, blockHash ethcommon.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) {
	startTime := time.Now()
	headers, bodies, err := h.fetchBlockResponse(peer, blockHash, headersCh, bodiesCh)
	if err != nil {
		if peer.disconnected {
			peer.Log().Tracef("block %v response timed out for disconnected peer", blockHash.String())
			return
		}

		switch {
		case errors.Is(err, ErrResponseTimeout):
			peer.Log().Errorf("did not receive block header and body for block %v before timeout", blockHash.String())
		case errors.Is(err, ErrInvalidPacketType):
		default:
			peer.Log().Errorf("could not fetch block header and body for block %v: %v", blockHash.String(), err)
		}
		peer.Disconnect(p2p.DiscUselessPeer)
		return
	}

	elapsedTime := time.Since(startTime)
	peer.Log().Debugf("took %v to fetch block %v header and body", elapsedTime, blockHash.String())

	if err := h.processBlockComponents(peer, headers, bodies); err != nil {
		log.Errorf("error processing block components for hash %v: %v", blockHash.String(), err)
	}
}

func (h *Handler) fetchBlockResponse(peer *Peer, blockHash ethcommon.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) (*eth.BlockHeadersRequest, *BlockBodiesPacket, error) {
	var (
		headers *eth.BlockHeadersRequest
		bodies  *BlockBodiesPacket
		errCh   = make(chan error, 2)
	)

	go func() {
		var ok bool

		select {
		case rawHeaders := <-headersCh:
			headers, ok = rawHeaders.(*eth.BlockHeadersRequest)
			peer.Log().Debugf("received header for block %v", blockHash.String())
			if !ok {
				log.Errorf("could not convert headers for block %v to the expected packet type, got %T", blockHash.String(), rawHeaders)
				errCh <- ErrInvalidPacketType
			} else {
				errCh <- nil
			}
		case <-time.After(responseTimeout):
			errCh <- ErrResponseTimeout
		}
	}()

	go func() {
		var ok bool

		select {
		case rawBodies := <-bodiesCh:
			bodies, ok = rawBodies.(*BlockBodiesPacket)
			peer.Log().Debugf("received body for block %v", blockHash)
			if !ok {
				log.Errorf("could not convert bodies for block %v to the expected packet type, got %T", blockHash.String(), rawBodies)
				errCh <- ErrInvalidPacketType
			} else {
				errCh <- nil
			}
		case <-time.After(responseTimeout):
			errCh <- ErrResponseTimeout
		}
	}()

	for i := 0; i < 2; i++ {
		err := <-errCh
		if err != nil {
			return nil, nil, err
		}
	}

	if len(*headers) != 1 || len(bodies.BlockBodiesResponse) != 1 {
		return nil, nil, fmt.Errorf("received %v headers and %v bodies, instead of 1 of each", len(*headers), len(bodies.BlockBodiesResponse))
	}

	return headers, bodies, nil
}

func (h *Handler) processBlockComponents(peer *Peer, headers *eth.BlockHeadersRequest, bodies *BlockBodiesPacket) error {
	if len(*headers) != 1 || len(bodies.BlockBodiesResponse) != 1 {
		return fmt.Errorf("received %v headers and %v bodies, instead of 1 of each", len(*headers), len(bodies.BlockBodiesResponse))
	}

	header := (*headers)[0]
	body := bodies.BlockBodiesResponse[0]
	block := bxcommoneth.NewBlockWithHeader(header).WithBody(ethtypes.Body{Transactions: body.Transactions, Uncles: body.Uncles})
	if len(body.Sidecars) > 0 {
		block = block.WithSidecars(body.Sidecars)
	}

	blockInfo := NewBlockInfo(block, nil)
	_ = h.chain.SetTotalDifficulty(blockInfo)
	return h.processBlock(peer, blockInfo)
}

// Handle processes Ethereum message packets that update internal backend state. In general, messages that require responses should not reach this function.
func (h *Handler) Handle(peer *Peer, packet eth.Packet) error {
	switch p := packet.(type) {
	case *eth.StatusPacket:
		h.chain.InitializeDifficulty(p.Head, p.TD)
		// send Empty NewPooledTransactionHashes based the protocol
		var msg interface{}
		switch peer.version {
		case eth.ETH68:
			msg = eth.NewPooledTransactionHashesPacket{}
		default:
			msg = NewPooledTransactionHashesPacket66{}
		}
		if err := peer.send(eth.NewPooledTransactionHashesMsg, &msg); err != nil {
			peer.Log().Errorf("error sending empty NewPooledTransactionHashesMsg message after handshake %v", err)
		}
		return nil
	case *eth.TransactionsPacket:
		return h.processTransactions(peer, *p)
	case *eth.PooledTransactionsResponse:
		return h.processTransactions(peer, *p)
	case *NewPooledTransactionHashesPacket66:
		return h.processTransactionHashes(peer, *p)
	case *eth.NewPooledTransactionHashesPacket:
		return h.processTransactionHashes(peer, (*p).Hashes)
	case *NewBlockPacket:
		block := &bxcommoneth.Block{Block: *p.Block}
		if len(p.Sidecars) > 0 {
			block.SetBlobSidecars(p.Sidecars)
		}
		return h.processBlock(peer, NewBlockInfo(block, p.TD))
	case *eth.NewBlockHashesPacket:
		return h.processBlockAnnouncement(peer, *p)
	case *eth.BlockHeadersRequest:
		return h.processBlockHeaders(peer, *p)
	default:
		return fmt.Errorf("unexpected eth packet type: %v", packet)
	}
}

func (h *Handler) broadcastTransactions(p *datatype.ProcessingETHTransaction, sourceNode types.NodeEndpoint, connectionType bxtypes.NodeType) {
	for _, peer := range h.peers.getAll() {
		if sourceNode.IPPort() == peer.IPEndpoint().IPPort() {
			continue
		}
		txs := p.Transactions(connectionType, peer.Dynamic())
		if err := peer.SendTransactions(txs); err != nil {
			peer.Log().Errorf("could not send %v transactions: %v", len(txs), err)
		}
	}
}

func (h *Handler) broadcastPooledTransactionHashes(txHashes []ethcommon.Hash, types []byte, sizes []uint32) {
	for _, peer := range h.peers.getAll() {
		if peer.version >= eth.ETH68 {
			if err := peer.SendPooledTransactionHashes(txHashes, types, sizes); err != nil {
				peer.Log().Errorf("could not announce %v transaction hashes: %v", len(txHashes), err)
			}
			continue
		}

		if err := peer.SendPooledTransactionHashes67(txHashes); err != nil {
			peer.Log().Errorf("could not announce %v transaction hashes: %v", len(txHashes), err)
		}
	}
}

func (h *Handler) broadcastBlock(block *bxcommoneth.Block, totalDifficulty *big.Int, sourceBlockchainPeer *Peer) {
	source := "BDN"
	if sourceBlockchainPeer != nil {
		source = sourceBlockchainPeer.endpoint.IPPort()
	}

	if totalDifficulty == nil || totalDifficulty.Cmp(h.config.TerminalTotalDifficulty) >= 0 || totalDifficulty.Cmp(big.NewInt(0)) == 0 {
		return
	}

	for _, peer := range h.peers.getAll() {
		if peer == sourceBlockchainPeer {
			continue
		}
		peer.QueueNewBlock(block, totalDifficulty)
		peer.Log().Debugf("queuing block %v from %v", block.Hash().String(), source)
	}
}

func (h *Handler) broadcastBlockAnnouncement(block *bxcommoneth.Block) {
	blockHash := block.Hash()
	number := block.NumberU64()
	for _, peer := range h.peers.getAll() {
		if err := peer.AnnounceBlock(blockHash, number); err != nil {
			peer.Log().Errorf("could not announce block %v: %v", block.Hash().String(), err)
		}
	}
}

func (h *Handler) isChainIDMatch(txChainID uint64) bool {
	// if chainID is 0 its legacy tx, so we want to propagate it;
	// if it's not 0, we need to check if its match to gw chain id
	if txChainID == 0 || txChainID == h.config.Network {
		return true
	}
	return false
}

func (h *Handler) processTransactions(peer *Peer, txs []*ethtypes.Transaction) error {
	bdnTxs := make([]*types.BxTransaction, 0, len(txs))
	for _, tx := range txs {
		if tx.Type() == ethtypes.BlobTxType && tx.BlobTxSidecar() == nil {
			log.Debugf("blob tx %v from blockchain peer %v has no sidecar data", tx.Hash().String(), peer.endpoint.IPPort())
			continue
		}

		// the transaction is considered invalid if the length of authorization_list is zero.
		if tx.Type() == ethtypes.SetCodeTxType && tx.SetCodeAuthorizations() == nil {
			log.Debugf("set code tx %v from blockchain peer %v has no authorization list", tx.Hash().String(), peer.endpoint.IPPort())
			continue
		}

		if !h.isChainIDMatch(tx.ChainId().Uint64()) {
			log.Debugf("tx %v from blockchain peer %v has invalid chain id", tx.Hash().String(), peer.endpoint.IPPort())
			continue
		}
		bdnTx, err := h.bridge.TransactionBlockchainToBDN(tx)
		if err != nil {
			return err
		}
		bdnTxs = append(bdnTxs, bdnTx)
	}
	err := h.bridge.SendTransactionsToBDN(bdnTxs, peer.IPEndpoint())
	if errors.Is(err, blockchain.ErrChannelFull) {
		log.Warnf("transaction channel for sending to the BDN is full; dropping %v transactions...", len(txs))
		return nil
	}

	return err
}

func (h *Handler) processTransactionHashes(peer *Peer, txHashes []ethcommon.Hash) error {
	sha256Hashes := make([]types.SHA256Hash, 0, len(txHashes))
	for _, hash := range txHashes {
		sha256Hashes = append(sha256Hashes, NewSHA256Hash(hash))
	}

	err := h.bridge.AnnounceTransactionHashes(peer.ID(), sha256Hashes, peer.endpoint)
	if errors.Is(err, blockchain.ErrChannelFull) {
		log.Warnf("transaction announcement channel for sending to the BDN is full; dropping %v hashes...", len(txHashes))
		return nil
	}

	return err
}

func (h *Handler) processBlock(peer *Peer, blockInfo *BlockInfo) error {
	block := blockInfo.Block
	blockHash := block.Hash()
	blockHeight := block.Number()

	switch h.config.Network {
	case network.EthMainnetChainID:
		peer.Log().Errorf("ignoring block[hash=%s,height=%d] from old node", blockHash.String(), blockHeight)
		return nil
	}

	if err := h.chain.ValidateBlock(block); err != nil {
		if errors.Is(err, ErrAlreadySeen) {
			peer.Log().Debugf("skipping block %v (height %v): %v", blockHash.String(), blockHeight, err)
		} else {
			peer.Log().Warnf("skipping block %v (height %v): %v", blockHash.String(), blockHeight, err)
		}
		return nil
	}

	peer.Log().Debugf("processing new block %v (height %v)", blockHash.String(), blockHeight)
	newHeadCount := h.chain.AddBlock(blockInfo, BSBlockchain)
	h.sendConfirmedBlocksToBDN(newHeadCount, peer.IPEndpoint())
	h.broadcastBlock(block, blockInfo.totalDifficulty, peer)
	return nil
}

func (h *Handler) processBlockAnnouncement(peer *Peer, newBlocks eth.NewBlockHashesPacket) error {
	for _, block := range newBlocks {
		peer.Log().Debugf("processing new block announcement %v (height %v)", block.Hash, block.Number)

		if !h.chain.HasBlock(block.Hash) {
			headersCh := make(chan eth.Packet)
			bodiesCh := make(chan eth.Packet)

			err := peer.RequestBlock(block.Hash, headersCh, bodiesCh)
			if err != nil {
				peer.Log().Errorf("could not request block %v: %v", block.Hash, err)
				return err
			}
			go h.awaitBlockResponse(peer, block.Hash, headersCh, bodiesCh)
		} else {
			h.confirmBlock(block.Hash, peer.IPEndpoint())
		}
	}

	return nil
}

func (h *Handler) processBlockHeaders(peer *Peer, blockHeaders eth.BlockHeadersRequest) error {
	// expected to only be called when header is not expected to be part of a get block headers in response to a new block hashes message
	for _, blockHeader := range blockHeaders {
		h.confirmBlock(blockHeader.Hash(), peer.IPEndpoint())
	}
	return nil
}

// confirmBlockFromWS is called when the websocket connection indicates that a block has been accepted
// - this function is a replacement for requesting manual confirmations.
func (h *Handler) confirmBlockFromWS(hash ethcommon.Hash, height *big.Int, peer *Peer) {
	if peer != nil {
		h.confirmBlock(hash, peer.endpoint)
		peer.UpdateHead(height.Uint64(), hash)
	}
}

func (h *Handler) confirmBlock(hash ethcommon.Hash, peerEndpoint types.NodeEndpoint) {
	newHeads := h.chain.ConfirmBlock(hash)
	h.sendConfirmedBlocksToBDN(newHeads, peerEndpoint)
}

func (h *Handler) sendConfirmedBlocksToBDN(count int, peerEndpoint types.NodeEndpoint) {
	newHeads, err := h.chain.GetNewHeadsForBDN(count)
	if err != nil {
		log.Errorf("could not fetch chainstate: %v", err)
	}

	// iterate in reverse to send all new heads in ascending order to BDN
	for i := len(newHeads) - 1; i >= 0; i-- {
		newHead := newHeads[i]
		hash := newHead.Block.Hash()

		bdnBlock, err := h.bridge.BlockBlockchainToBDN(newHead)
		if err != nil {
			log.Errorf("could not convert block %v: %v", hash, err)
			continue
		}
		err = h.bridge.SendBlockToBDN(bdnBlock, peerEndpoint)
		if err != nil {
			log.Errorf("could not send block %v to BDN: %v", hash, err)
			continue
		}
		h.chain.MarkSentToBDN(hash)
	}

	b, err := h.blockAtDepth(h.config.BlockConfirmationsCount)
	if err != nil {
		log.Debugf("cannot retrieve bxblock at depth %v, %v", h.config.BlockConfirmationsCount, err)
		return
	}
	blockHash := ethcommon.BytesToHash(b.Hash().Bytes())

	if h.chain.HasConfirmationSendToBDN(blockHash) {
		log.Debugf("block %v has already been sent in a block confirm message to gateway", b.Hash())
		return
	}
	log.Tracef("sending block (%v) confirm message to gateway from backend", b.Hash())
	err = h.bridge.SendConfirmedBlockToGateway(b, peerEndpoint)
	if err != nil {
		log.Debugf("failed sending block(%v) confirmation message to gateway, %v", b.Hash(), err)
		return
	}
	h.chain.MarkConfirmationSentToBDN(blockHash)
}

// GetBridge return bridge
func (h *Handler) GetBridge() blockchain.Bridge {
	return h.bridge
}

// GetBodies assembles and returns a set of block bodies
func (h *Handler) GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error) {
	return h.chain.GetBodies(hashes)
}

// GetBlobSidecars assembles and returns a set of blob sidecars
func (h *Handler) GetBlobSidecars(hashes []ethcommon.Hash) ([]bxcommoneth.BlobSidecars, error) {
	return h.chain.GetBlobSidecars(hashes)
}

// GetHeaders assembles and returns a set of headers
func (h *Handler) GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethtypes.Header, error) {
	return h.chain.GetHeaders(start, count, skip, reverse)
}

// storeBDNBlock will return a nil block and no error if block is a duplicate
func (h *Handler) storeBDNBlock(bdnBlock *types.BxBlock) (*BlockInfo, error) {
	blockHash := ethcommon.BytesToHash(bdnBlock.Hash().Bytes())
	if h.chain.HasBlock(blockHash) {
		log.Debugf("duplicate block %v from BDN, skipping", blockHash)
		return nil, nil
	}

	blockchainBlock, err := h.bridge.BlockBDNtoBlockchain(bdnBlock)
	if err != nil {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to Ethereum block")
	}

	ethBlockInfo, ok := blockchainBlock.(*BlockInfo)
	if !ok {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to Ethereum block")
	}

	h.chain.AddBlock(ethBlockInfo, BSBDN)
	return ethBlockInfo, nil
}

func (h *Handler) checkInitialBlockchainLiveliness(initialLivelinessCheckDelay time.Duration) {
	time.Sleep(initialLivelinessCheckDelay)
	if len(h.peers.getAll()) == 0 {
		err := h.bridge.SendNoActiveBlockchainPeersAlert()
		if errors.Is(err, blockchain.ErrChannelFull) {
			log.Warnf("no active blockchain peers alert channel is full")
		}
	}
}

func logTransactionConverterFailure(err error, bdnTx *types.BxTransaction) {
	transactionHex := "<omitted>"
	if log.IsLevelEnabled(log.TraceLevel) {
		transactionHex = hexutil.Encode(bdnTx.Content())
	}
	log.Errorf("could not convert transaction (hash: %v) from BDN to Ethereum transaction: %v. contents: %v", bdnTx.Hash(), err, transactionHex)
}

func logBlockConverterFailure(err error, bdnBlock *types.BxBlock) {
	var blockHex string
	if log.IsLevelEnabled(log.TraceLevel) {
		b, err := rlp.EncodeToBytes(bdnBlock)
		if err != nil {
			blockHex = fmt.Sprintf("bad block from BDN could not be encoded to RLP bytes: %v", err)
		} else {
			blockHex = hexutil.Encode(b)
		}
	}
	log.Errorf("could not convert block (hash: %v) from BDN to Ethereum block: %v. contents: %v", bdnBlock.Hash(), err, blockHex)
}

func (h *Handler) blockAtDepth(chainDepth int) (*types.BxBlock, error) {
	block, err := h.chain.BlockAtDepth(chainDepth)
	if err != nil {
		log.Debugf("cannot retrieve block with chain depth %v, %v", chainDepth, err)
		return nil, err
	}
	blockInfo := NewBlockInfo(block, block.Header().Difficulty)
	bxBlock, err := h.bridge.BlockBlockchainToBDN(blockInfo)
	if err != nil {
		log.Debugf("cannot convert eth block to BDN block at the chain depth %v with hash %v, %v", chainDepth, block.Hash(), err)
		return nil, err
	}
	return bxBlock, err
}
