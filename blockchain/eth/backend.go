package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/datatype"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/bsc"
	eth2 "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const checkpointTimeout = 5 * time.Second

// ErrInvalidPacketType is returned when a packet type is not recognized
var ErrInvalidPacketType = errors.New("invalid packet type")

// handler is the Ethereum backend implementation. It passes transactions and blocks to the BDN bridge and tracks received blocks and transactions from peers.
type handler struct {
	chain            *core.Chain
	bridge           blockchain.Bridge
	statusBridge     blockchain.StatusSubscription
	peers            *peerSet
	config           *network.EthConfig
	wsManager        blockchain.WSManager
	recommendedPeers map[string]struct{}
	responseQueue    *syncmap.SyncMap[string, chan interface{}]
}

// newHandler returns a new handler and starts its processing go routines
func newHandler(ctx context.Context, config *network.EthConfig, chain *core.Chain, bridge blockchain.Bridge, wsManager blockchain.WSManager, recommendedPeers map[string]struct{}) *handler {
	h := &handler{
		config:           config,
		chain:            chain,
		bridge:           bridge,
		statusBridge:     bridge.SubscribeStatus(false),
		peers:            newPeerSet(),
		wsManager:        wsManager,
		recommendedPeers: recommendedPeers,
		responseQueue:    syncmap.NewStringMapOf[chan interface{}](),
	}
	go h.checkInitialBlockchainLiveliness(100 * time.Second)
	go h.handleBDNBridge(ctx)

	return h
}

func (h *handler) runEthPeer(peer *eth2.Peer, handler func(peer *eth2.Peer) error) error {
	l := peer.Log()

	bscExt, err := h.peers.waitBscExtension(peer)
	if err != nil {
		l.Error("BSC extension barrier failed", "err", err)
		return err
	}

	if bscExt != nil {
		l.Debugf("adding BSC extension for peer %v", peer.IPEndpoint().IPPort())
	}

	peerStatus, err := peer.Handshake(h.config.Network, h.config.TotalDifficulty, h.config.Head, h.config.Genesis, h.config.ExecutionLayerForks)
	if err != nil {
		l.Errorf("peer %v handshake failed with error: %v", peer.IPEndpoint(), err)
		return err
	}

	l.Infof("peer %v is starting", peer.IPEndpoint())

	err = h.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{PeerEndpoint: peer.IPEndpoint(), IsConnected: true, IsDynamic: peer.Dynamic()})
	if err != nil {
		l.Errorf("failed to send blockchain connect status for %v: %v", peer.IPEndpoint(), err)
		return err
	}

	// set initial total difficulty
	h.chain.InitializeDifficulty(peerStatus.Head, peerStatus.TD)

	endpoint := peer.IPEndpoint()

	wg := new(sync.WaitGroup)

	_, isRecommended := h.recommendedPeers[fmt.Sprintf("%s:%d", endpoint.IP, endpoint.Port)]
	if !peer.Dynamic() && !isRecommended {
		ok := h.wsManager.SetBlockchainPeer(peer)
		if !ok {
			l.Warnf("unable to set blockchain peer for %v: no corresponding websockets provider found (ok if websockets URI was omitted)", endpoint.IPPort())
		}
		if ws, ok := h.wsManager.Provider(&endpoint); ok {
			wg.Add(1)
			go h.runEthSub(peer.Context(), ws, wg)
		}
	}
	if err := h.peers.register(peer, bscExt); err != nil {
		return err
	}

	defer func() {
		if !peer.Dynamic() && !isRecommended {
			ok := h.wsManager.UnsetBlockchainPeer(endpoint)
			if !ok {
				l.Warnf("unable to unset blockchain peer for %v: no corresponding websockets provider found (ok if websockets URI was omitted)", endpoint.IPPort())
			}
		}
		// cancel context. Should stop the wsProvider
		peer.Stop()

		err = h.peers.unregister(peer.ID())
		if err != nil {
			l.Errorf("failed to unregister peer %v: %v", endpoint.IPPort(), err)
		}
	}()

	peer.Start()

	time.AfterFunc(checkpointTimeout, func() {
		peer.PassCheckpoint()
	})

	// start handling messages from the peer
	peerErr := handler(peer)
	if errors.Is(peerErr, context.Canceled) {
		peerErr = nil
	}

	err = h.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{PeerEndpoint: endpoint, IsConnected: false, IsDynamic: peer.Dynamic()})
	if err != nil {
		l.Errorf("failed to send blockchain disconnect status for %v - %v, peer error - %v", endpoint, err, peerErr)
		return err
	}
	// TODO Here we have disconnection, but that disconnection does not affect BDNPerformanceStats

	wg.Wait()

	l.Errorf("peer %v terminated with error: %v", endpoint, peerErr)

	return err
}

// runEthSub is running as a go routine. Can sleep if needed
func (h *handler) runEthSub(wsCtx context.Context, nodeWS blockchain.WSProvider, wg *sync.WaitGroup) {
	var newPendingTxsRespCh chan ethcommon.Hash
	var newPendingTxsErrCh <-chan error

	var newHeadsRespCh chan *ethtypes.Header
	var newHeadsErrCh <-chan error

	nodeWS.Log().Debugf("starting runEthSub... process %v", utils.GetGID())
	defer wg.Done()
	defer nodeWS.Log().Debugf("runEthSub ends process %v", utils.GetGID())

	for {
		nodeWS.Dial()

		// In Ethereum PoS consensus layer is responsible for blocks and confirmations even though heads are still available from websocket of execution layer
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

// runBscExtension registers a `bsc` peer into the joint eth/bsc peerset and
// starts handling inbound messages. As `bsc` is only a satellite protocol to
// `eth`, all subsystem registrations and lifecycle management will be done by
// the main `eth` handler to prevent strange races.
func (h *handler) runBscExtension(peer *bsc.Peer, handler bsc.Handler) error {
	if err := peer.Handshake(); err != nil {
		// ensure that waitBscExtension receives the exit signal normally
		// otherwise, can't graceful shutdown
		ps := h.peers
		id := peer.ID()

		// ensure nobody can double-connect
		ps.lock.Lock()
		if wait, ok := ps.bscWait[id]; ok {
			delete(ps.bscWait, id)
			peer.Log().Errorf("BSC extension Handshake failed: %v", err)
			wait <- bsc.Peer{}
		}
		ps.lock.Unlock()

		return err
	}

	if err := h.peers.registerBscExtension(peer); err != nil {
		peer.Log().Errorf("BSC extension registration failed: %v", err)
		return err
	}

	peer.Log().Infof("starting BSC extension for peer %v", peer.ID())

	defer peer.Stop()

	return handler(peer)
}

// handleFeeds processes the heads and tx hashes feeds. exit with false on feed error and true when done
func (h *handler) handleFeeds(wsCtx context.Context, nodeWS blockchain.WSProvider, newHeadsRespCh chan *ethtypes.Header, newPendingTxsRespCh chan ethcommon.Hash, newHeadsErrCh <-chan error, newPendingTxsErrCh <-chan error) bool {
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
			h.confirmBlockFromWS(newHeader.Hash(), newHeader.Number, nodeWS.BlockchainPeer().(*eth2.Peer))
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
			// read all pending txs from the channel into the list
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
			var status blockchain.NodeSyncStatus
			if !activeFeeds {
				status = blockchain.Unsynced
			} else {
				status = blockchain.Synced
			}
			h.wsManager.UpdateNodeSyncStatus(nodeWS.BlockchainPeerEndpoint(), status)
			activeFeeds = false
		}
	}
}

func (h *handler) handleBDNBridge(ctx context.Context) {
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

func (h *handler) notifyResponse(requestID string, data interface{}) error {
	responseCh, ok := h.responseQueue.LoadAndDelete(requestID)
	if !ok {
		return eth2.ErrUnknownRequestID
	}

	responseCh <- data

	return nil
}

func (h *handler) processBDNTransactions(bdnTxs blockchain.Transactions) {
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
			// For some reason, blob tx with a missed sidecar passes the tx store check
			if ethTx.BlobTxSidecar() != nil {
				pooledTransactionHashes = append(pooledTransactionHashes, ethTx.Hash())
				pooledTypes = append(pooledTypes, ethTx.Type())
				pooledSizes = append(pooledSizes, uint32(ethTx.Size()))
			} else {
				log.Debugf("blob tx from BDN %s has no sidecar data", ethTx.Hash())
			}

			continue
		}

		// allow sending tx to inbound node only if it's paid tx or marked as deliver to node
		p.Add(ethTx, bdnTx.Flags().IsPaid() || bdnTx.Flags().ShouldDeliverToNode())
	}

	h.broadcastTransactions(p, bdnTxs.PeerEndpoint, bdnTxs.ConnectionType)

	if len(pooledTransactionHashes) > 0 {
		h.broadcastPooledTransactionHashes(pooledTransactionHashes, pooledTypes, pooledSizes)
	}
}

func (h *handler) processBDNTransactionRequests(request blockchain.TransactionAnnouncement) {
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

func (h *handler) processBDNBlock(bdnBlock *types.BxBlock) {
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

func (h *handler) processBlockchainStatusRequest() {
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

func (h *handler) processNodeConnectionCheckRequest() {
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

func (h *handler) processDisconnectEvent(endpoint types.NodeEndpoint) {
	// check if the peer is in the connections
	for _, peer := range h.peers.getAll() {
		if (endpoint.PublicKey != "" && endpoint.PublicKey == peer.IPEndpoint().PublicKey) || (endpoint.PublicKey == "" && endpoint.IPPort() == peer.IPEndpoint().IPPort()) {

			h.disconnectPeer(peer.ID())

			return
		}
	}

	log.Debugf("peer '%v' was not found", endpoint)
}

func (h *handler) broadcastTransactions(p *datatype.ProcessingETHTransaction, sourceNode types.NodeEndpoint, connectionType bxtypes.NodeType) {
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

func (h *handler) broadcastPooledTransactionHashes(txHashes []ethcommon.Hash, types []byte, sizes []uint32) {
	for _, peer := range h.peers.getAll() {
		if peer.Version() >= eth.ETH68 {
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

func (h *handler) broadcastBlock(block *bxcommoneth.Block, totalDifficulty *big.Int, sourceBlockchainPeer *eth2.Peer) {
	source := "BDN"
	if sourceBlockchainPeer != nil {
		source = sourceBlockchainPeer.IPEndpoint().IPPort()
	}

	if totalDifficulty == nil || totalDifficulty.Cmp(h.config.TerminalTotalDifficulty) >= 0 || totalDifficulty.Cmp(big.NewInt(0)) == 0 {
		return
	}

	for _, peer := range h.peers.getAll() {
		if peer.Peer == sourceBlockchainPeer {
			continue
		}
		peer.QueueNewBlock(block, totalDifficulty)
		peer.Log().Debugf("queuing block %v from %v", block.Hash().String(), source)
	}
}

func (h *handler) broadcastBlockAnnouncement(block *bxcommoneth.Block) {
	blockHash := block.Hash()
	number := block.NumberU64()
	for _, peer := range h.peers.getAll() {
		if err := peer.AnnounceBlock(blockHash, number); err != nil {
			peer.Log().Errorf("could not announce block %v: %v", block.Hash().String(), err)
		}
	}
}

// confirmBlockFromWS is called when the websocket connection indicates that a block has been accepted
// - this function is a replacement for requesting manual confirmations.
func (h *handler) confirmBlockFromWS(hash ethcommon.Hash, height *big.Int, peer *eth2.Peer) {
	if peer != nil {
		h.confirmBlock(hash, peer.IPEndpoint())
		peer.UpdateHead(height.Uint64(), hash)
	}
}

func (h *handler) confirmBlock(hash ethcommon.Hash, peerEndpoint types.NodeEndpoint) {
	newHeads := h.chain.ConfirmBlock(hash)
	h.sendConfirmedBlocksToBDN(newHeads, peerEndpoint)
}

func (h *handler) sendConfirmedBlocksToBDN(count int, peerEndpoint types.NodeEndpoint) {
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

// storeBDNBlock will return a nil block and no error if block is a duplicate
func (h *handler) storeBDNBlock(bdnBlock *types.BxBlock) (*core.BlockInfo, error) {
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

	ethBlockInfo, ok := blockchainBlock.(*core.BlockInfo)
	if !ok {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to Ethereum block")
	}

	h.chain.AddBlock(ethBlockInfo, core.BSBDN)
	return ethBlockInfo, nil
}

func (h *handler) checkInitialBlockchainLiveliness(initialLivelinessCheckDelay time.Duration) {
	time.Sleep(initialLivelinessCheckDelay)
	if len(h.peers.getAll()) == 0 {
		err := h.bridge.SendNoActiveBlockchainPeersAlert()
		if errors.Is(err, blockchain.ErrChannelFull) {
			log.Warnf("no active blockchain peers alert channel is full")
		}
	}
}

func (h *handler) blockAtDepth(chainDepth int) (*types.BxBlock, error) {
	block, err := h.chain.BlockAtDepth(chainDepth)
	if err != nil {
		log.Debugf("cannot retrieve block with chain depth %v, %v", chainDepth, err)
		return nil, err
	}
	blockInfo := core.NewBlockInfo(block, block.Header().Difficulty)
	bxBlock, err := h.bridge.BlockBlockchainToBDN(blockInfo)
	if err != nil {
		log.Debugf("cannot convert eth block to BDN block at the chain depth %v with hash %v, %v", chainDepth, block.Hash(), err)
		return nil, err
	}
	return bxBlock, err
}

func (h *handler) disconnectPeer(peerID string) {
	peer, ok := h.peers.get(peerID)
	if !ok {
		return
	}

	if err := h.peers.unregister(peerID); err != nil {
		log.Errorf("could not unregister peer %v: %v", peer.IPEndpoint().IPPort(), err)
		return
	}

	if err := h.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{PeerEndpoint: peer.IPEndpoint(), IsConnected: false, IsDynamic: peer.Dynamic()}); err != nil {
		log.Errorf("failed to send blockchain disconnect status for %v: %v", peer.IPEndpoint(), err)
	}

	peer.Disconnect(p2p.DiscUselessPeer)

	if peer.bscExt != nil {
		peer.bscExt.Disconnect(p2p.DiscUselessPeer)
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
