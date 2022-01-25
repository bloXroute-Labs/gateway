package eth

import (
	"context"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	"github.com/bloXroute-Labs/gateway/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	log "github.com/sirupsen/logrus"
	"math/big"
	"time"
)

const (
	checkpointTimeout    = 5 * time.Second
	maxFutureBlockNumber = 100
)

// Backend represents the interface to which any stateful message handling (e.g. looking up tx pool items or block headers) will be passed to for processing
type Backend interface {
	NetworkConfig() *network.EthConfig
	RunPeer(peer *Peer, handler func(*Peer) error) error
	Handle(peer *Peer, packet eth.Packet) error
	GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethtypes.Header, error)
	GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error)
}

// Handler is the Ethereum backend implementation. It passes transactions and blocks to the BDN bridge and tracks received blocks and transactions from peers.
type Handler struct {
	bridge blockchain.Bridge
	peers  *peerSet
	cancel context.CancelFunc
	config *network.EthConfig

	chain           *Chain
	activeWebsocket bool
}

// NewHandler returns a new Handler and starts its processing go routines
func NewHandler(parent context.Context, bridge blockchain.Bridge, config *network.EthConfig, wsProvider blockchain.WSProvider) *Handler {
	ctx, cancel := context.WithCancel(parent)
	h := &Handler{
		bridge: bridge,
		peers:  newPeerSet(),
		cancel: cancel,
		config: config,
		chain:  NewChain(ctx),
	}
	go h.checkInitialBlockchainLiveliness(100 * time.Second)
	go h.handleBDNBridge(ctx)
	if wsProvider != nil {
		go h.runEthSub(wsProvider)
	}
	return h
}

func (h *Handler) handleFeeds(ws blockchain.WSProvider, newHeadsRespCh chan *ethtypes.Header, nptResponseCh chan ethcommon.Hash, nhSub *rpc.ClientSubscription, nptSub *rpc.ClientSubscription) {
	activeFeeds := true
	activeFeedsCheckTicker := time.NewTicker(time.Second * 10)
	defer activeFeedsCheckTicker.Stop()
	for {
		select {
		case err := <-nhSub.Err():
			ws.Log().Errorf("failed to get notification from newHeads: %v", err)
			ws.UpdateNodeSyncStatus(blockchain.Unsynced)
			return
		case newHeader := <-newHeadsRespCh:
			ws.Log().Tracef("received header for block %v (height %v)", newHeader.Hash(), newHeader.Number)
			h.confirmBlockForAll(newHeader.Hash(), newHeader.Number)
		case err := <-nptSub.Err():
			ws.Log().Errorf("failed to get notification from newPendingTransactions: %v", err)
			ws.UpdateNodeSyncStatus(blockchain.Unsynced)
			return
		case newPendingTx := <-nptResponseCh:
			activeFeeds = true
			txHash, err := types.NewSHA256Hash(newPendingTx[:])
			ws.Log().Tracef("received new pending tx %v", txHash)
			hashes := types.SHA256HashList{txHash}
			err = h.bridge.AnnounceTransactionHashes(bxgateway.WSConnectionID, hashes)
			if err != nil {
				ws.Log().Errorf("failed to send transaction %v to the gateway: %v", txHash, err)
			}
		case <-activeFeedsCheckTicker.C:
			if !activeFeeds {
				ws.UpdateNodeSyncStatus(blockchain.Unsynced)
			} else {
				ws.UpdateNodeSyncStatus(blockchain.Synced)
			}
			activeFeeds = false
		}
	}
}

func (h *Handler) runEthSub(ws blockchain.WSProvider) {
	newHeadsRespCh := make(chan *ethtypes.Header)
	nptResponseCh := make(chan ethcommon.Hash)

	for {
		nhSub, err := ws.Subscribe(newHeadsRespCh, "newHeads")
		if err != nil {
			log.Error(err)
			ws.Close()
			ws.Connect()
			continue
		}

		nptSub, err := ws.Subscribe(nptResponseCh, "newPendingTransactions")
		if err != nil {
			log.Error(err)
			ws.Close()
			ws.Connect()
			continue
		}

		h.activeWebsocket = true

		// process feeds
		h.handleFeeds(ws, newHeadsRespCh, nptResponseCh, nhSub.Sub.(*rpc.ClientSubscription), nptSub.Sub.(*rpc.ClientSubscription))

		// force all peers to manually request block confirmations while disconnected
		for _, peer := range h.peers.getAll() {
			peer.RequestConfirmations = true
		}
		h.activeWebsocket = false

		// closing connection and trying reconnection
		ws.Close()
		ws.Connect()
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
func (h *Handler) RunPeer(ep *Peer, handler func(*Peer) error) error {
	ep.RequestConfirmations = !h.activeWebsocket
	if err := h.peers.register(ep); err != nil {
		return err
	}
	defer func() {
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
		case bdnTxs := <-h.bridge.ReceiveBDNTransactions():
			h.processBDNTransactions(bdnTxs)
		case request := <-h.bridge.ReceiveTransactionHashesRequest():
			h.processBDNTransactionRequests(request)
		case bdnBlock := <-h.bridge.ReceiveBlockFromBDN():
			h.processBDNBlock(bdnBlock)
		case config := <-h.bridge.ReceiveNetworkConfigUpdates():
			h.config.Update(config)
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) processBDNTransactions(bdnTxs []*types.BxTransaction) {
	ethTxs := make([]*ethtypes.Transaction, 0, len(bdnTxs))

	for _, bdnTx := range bdnTxs {
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

		ethTxs = append(ethTxs, ethTx)
	}

	h.broadcastTransactions(ethTxs)
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

	ethBlock := ethBlockInfo.Block
	err = h.chain.SetTotalDifficulty(ethBlockInfo)
	if err != nil {
		log.Debugf("could not resolve difficulty for block %v, announcing instead", ethBlock.Hash())
		h.broadcastBlockAnnouncement(ethBlock)
	} else {
		h.broadcastBlock(ethBlock, ethBlockInfo.TotalDifficulty())
	}
}

func (h *Handler) awaitBlockResponse(peer *Peer, blockHash ethcommon.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet, fetchResponse func(peer *Peer, blockHash ethcommon.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) (*eth.BlockHeadersPacket, *eth.BlockBodiesPacket, error)) {
	startTime := time.Now()
	headers, bodies, err := fetchResponse(peer, blockHash, headersCh, bodiesCh)

	if err != nil {
		if peer.disconnected {
			peer.Log().Tracef("block %v response timed out for disconnected peer", blockHash)
			return
		}

		if err == ErrInvalidPacketType {
			// message is already logged
		} else if err == ErrResponseTimeout {
			peer.Log().Errorf("did not receive block header and body for block %v before timeout", blockHash)
		} else {
			peer.Log().Errorf("could not fetch block header and body for block %v: %v", blockHash, err)
		}
		peer.Disconnect(p2p.DiscUselessPeer)
		return
	}

	elapsedTime := time.Now().Sub(startTime)
	peer.Log().Debugf("took %v to fetch block %v header and body", elapsedTime, blockHash)

	if err := h.processBlockComponents(peer, headers, bodies); err != nil {
		log.Errorf("error processing block components for hash %v: %v", blockHash, err)
	}
}

func (h *Handler) fetchBlockResponse66(peer *Peer, blockHash ethcommon.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) (*eth.BlockHeadersPacket, *eth.BlockBodiesPacket, error) {
	var (
		headers *eth.BlockHeadersPacket
		bodies  *eth.BlockBodiesPacket
		errCh   = make(chan error, 2)
	)

	go func() {
		var ok bool

		select {
		case rawHeaders := <-headersCh:
			headers, ok = rawHeaders.(*eth.BlockHeadersPacket)
			peer.Log().Debugf("received header for block %v", blockHash)
			if !ok {
				log.Errorf("could not convert headers for block %v to the expected packet type, got %T", blockHash, rawHeaders)
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
			bodies, ok = rawBodies.(*eth.BlockBodiesPacket)
			peer.Log().Debugf("received body for block %v", blockHash)
			if !ok {
				log.Errorf("could not convert bodies for block %v to the expected packet type, got %T", blockHash, rawBodies)
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

	return headers, bodies, nil
}

func (h *Handler) fetchBlockResponse(peer *Peer, blockHash ethcommon.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) (*eth.BlockHeadersPacket, *eth.BlockBodiesPacket, error) {
	var (
		headers *eth.BlockHeadersPacket
		bodies  *eth.BlockBodiesPacket
		ok      bool
	)

	select {
	case rawHeaders := <-headersCh:
		headers, ok = rawHeaders.(*eth.BlockHeadersPacket)
		if !ok {
			log.Errorf("could not convert headers for block %v to the expected packet type, got %T", blockHash, rawHeaders)
			return nil, nil, ErrInvalidPacketType
		}
	case <-time.After(responseTimeout):
		return nil, nil, ErrResponseTimeout
	}

	peer.Log().Debugf("received header for block %v", blockHash)

	select {
	case rawBodies := <-bodiesCh:
		bodies, ok = rawBodies.(*eth.BlockBodiesPacket)
		if !ok {
			log.Errorf("could not convert headers for block %v to the expected packet type, got %T", blockHash, rawBodies)
			return nil, nil, ErrInvalidPacketType
		}
	case <-time.After(responseTimeout):
		return nil, nil, ErrResponseTimeout
	}

	peer.Log().Debugf("received body for block %v", blockHash)

	return headers, bodies, nil
}

func (h *Handler) processBlockComponents(peer *Peer, headers *eth.BlockHeadersPacket, bodies *eth.BlockBodiesPacket) error {
	if len(*headers) != 1 && len(*bodies) != 1 {
		return fmt.Errorf("received %v headers and %v bodies, instead of 1 of each", len(*headers), len(*bodies))
	}

	header := (*headers)[0]
	body := (*bodies)[0]
	block := ethtypes.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)
	blockInfo := NewBlockInfo(block, nil)
	_ = h.chain.SetTotalDifficulty(blockInfo)
	return h.processBlock(peer, blockInfo)
}

// Handle processes Ethereum message packets that update internal backend state. In general, messages that require responses should not reach this function.
func (h *Handler) Handle(peer *Peer, packet eth.Packet) error {
	switch p := packet.(type) {
	case *eth.StatusPacket:
		h.chain.InitializeDifficulty(p.Head, p.TD)
		return nil
	case *eth.TransactionsPacket:
		return h.processTransactions(peer, *p)
	case *eth.PooledTransactionsPacket:
		return h.processTransactions(peer, *p)
	case *eth.NewPooledTransactionHashesPacket:
		return h.processTransactionHashes(peer, *p)
	case *eth.NewBlockPacket:
		return h.processBlock(peer, NewBlockInfo(p.Block, p.TD))
	case *eth.NewBlockHashesPacket:
		return h.processBlockAnnouncement(peer, *p)
	case *eth.BlockHeadersPacket:
		return h.processBlockHeaders(peer, *p)
	default:
		return fmt.Errorf("unexpected eth packet type: %v", packet)
	}
}

func (h *Handler) createBxBlockFromEthHeader(header *ethtypes.Header) (*types.BxBlock, error) {
	body, ok := h.chain.getBlockBody(header.Hash())
	if !ok {
		return nil, ErrBodyNotFound
	}
	ethBlock := ethtypes.NewBlockWithHeader(header).WithBody(body.Transactions, body.Uncles)
	blockInfo := BlockInfo{ethBlock, header.Difficulty}
	bxBlock, err := h.bridge.BlockBlockchainToBDN(&blockInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to convert block %v to BDN format: %v", header.Hash(), err)
	}
	return bxBlock, nil
}

func (h *Handler) broadcastTransactions(txs ethtypes.Transactions) {
	for _, peer := range h.peers.getAll() {
		if err := peer.SendTransactions(txs); err != nil {
			peer.Log().Errorf("could not send %v transactions: %v", len(txs), err)
		}
	}
}

func (h *Handler) broadcastBlock(block *ethtypes.Block, totalDifficulty *big.Int) {
	for _, peer := range h.peers.getAll() {
		peer.QueueNewBlock(block, totalDifficulty)
		peer.Log().Debugf("queuing block %v from BDN", block.Hash())
	}
}

func (h *Handler) broadcastBlockAnnouncement(block *ethtypes.Block) {
	blockHash := block.Hash()
	number := block.NumberU64()
	for _, peer := range h.peers.getAll() {
		if err := peer.AnnounceBlock(blockHash, number); err != nil {
			peer.Log().Errorf("could not announce block %v: %v", block.Hash(), err)
		}
	}
}

func (h *Handler) processTransactions(peer *Peer, txs []*ethtypes.Transaction) error {
	bdnTxs := make([]*types.BxTransaction, 0, len(txs))
	for _, tx := range txs {
		bdnTx, err := h.bridge.TransactionBlockchainToBDN(tx)
		if err != nil {
			return err
		}
		bdnTxs = append(bdnTxs, bdnTx)
	}
	err := h.bridge.SendTransactionsToBDN(bdnTxs, peer.IPEndpoint())

	if err == blockchain.ErrChannelFull {
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

	err := h.bridge.AnnounceTransactionHashes(peer.ID(), sha256Hashes)

	if err == blockchain.ErrChannelFull {
		log.Warnf("transaction announcement channel for sending to the BDN is full; dropping %v hashes...", len(txHashes))
		return nil
	}

	return err
}

func (h *Handler) processBlock(peer *Peer, blockInfo *BlockInfo) error {
	block := blockInfo.Block
	blockHash := block.Hash()
	blockHeight := block.Number()

	if err := h.validateBlock(blockHash, blockHeight.Int64(), int64(block.Time())); err != nil {
		if err == ErrAlreadySeen {
			peer.Log().Debugf("skipping block %v (height %v): %v", blockHash, blockHeight, err)
		} else {
			peer.Log().Warnf("skipping block %v (height %v): %v", blockHash, blockHeight, err)
		}
		return nil
	}

	peer.Log().Debugf("processing new block %v (height %v)", blockHash, blockHeight)
	newHeadCount := h.chain.AddBlock(blockInfo, BSBlockchain)
	h.sendConfirmedBlocksToBDN(newHeadCount, peer.IPEndpoint())
	return nil
}

func (h *Handler) validateBlock(blockHash ethcommon.Hash, blockHeight int64, blockTime int64) error {
	if err := h.chain.ValidateBlock(blockHash, blockHeight); err != nil {
		return err
	}

	minTimestamp := time.Now().Add(-h.config.IgnoreBlockTimeout)
	if minTimestamp.Unix() > blockTime {
		return errors.New("timestamp too old")
	}
	return nil
}

func (h *Handler) processBlockAnnouncement(peer *Peer, newBlocks eth.NewBlockHashesPacket) error {
	for _, block := range newBlocks {
		peer.Log().Debugf("processing new block announcement %v (height %v)", block.Hash, block.Number)

		if !h.chain.HasBlock(block.Hash) {
			headersCh := make(chan eth.Packet)
			bodiesCh := make(chan eth.Packet)

			if peer.isVersion66() {
				err := peer.RequestBlock66(block.Hash, headersCh, bodiesCh)
				if err != nil {
					peer.Log().Errorf("could not request block %v: %v", block.Hash, err)
					return err
				}
				go h.awaitBlockResponse(peer, block.Hash, headersCh, bodiesCh, h.fetchBlockResponse66)
			} else {
				err := peer.RequestBlock(block.Hash, headersCh, bodiesCh)
				if err != nil {
					peer.Log().Errorf("could not request block %v: %v", block.Hash, err)
					return err
				}
				go h.awaitBlockResponse(peer, block.Hash, headersCh, bodiesCh, h.fetchBlockResponse)
			}
		} else {
			h.confirmBlock(block.Hash, peer.IPEndpoint())
		}
	}

	return nil
}

func (h *Handler) processBlockHeaders(peer *Peer, blockHeaders eth.BlockHeadersPacket) error {
	// expected to only be called when header is not expected to be part of a get block headers in response to a new block hashes message
	for _, blockHeader := range blockHeaders {
		h.confirmBlock(blockHeader.Hash(), peer.IPEndpoint())
	}
	return nil
}

// confirmedBlockForAll is called when the websocket connection indicates that a block has been accepted
// - this function is a replacement for requesting manual confirmations.
// - this function will not work correct once multiple blockchain peers are connected, since the websocket connection only represents one of them
func (h *Handler) confirmBlockForAll(hash ethcommon.Hash, height *big.Int) {
	h.confirmBlock(hash, types.NodeEndpoint{})
	for _, peer := range h.peers.getAll() {
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

		bdnBlock, err := h.bridge.BlockBlockchainToBDN(newHead)
		if err != nil {
			log.Errorf("could not convert block: %v", err)
			continue
		}
		err = h.bridge.SendBlockToBDN(bdnBlock, peerEndpoint)
		if err != nil {
			log.Errorf("could not send block to BDN: %v", err)
			continue
		}
		h.chain.MarkSentToBDN(newHead.Block.Hash())
	}
}

// GetBodies assembles and returns a set of block bodies
func (h *Handler) GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error) {
	return h.chain.GetBodies(hashes)
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
	ticker := time.NewTicker(initialLivelinessCheckDelay)
	for {
		select {
		case <-ticker.C:
			if len(h.peers.getAll()) == 0 {
				err := h.bridge.SendNoActiveBlockchainPeersAlert()
				if err == blockchain.ErrChannelFull {
					log.Warnf("no active blockchain peers alert channel is full")
				}
			}
			ticker.Stop()
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
	blockHex := "<omitted>"
	if log.IsLevelEnabled(log.TraceLevel) {
		b, err := rlp.EncodeToBytes(bdnBlock)
		if err != nil {
			log.Error("bad block from BDN could not be encoded to RLP bytes")
			return
		}
		blockHex = hexutil.Encode(b)
	}
	log.Errorf("could not convert block (hash: %v) from BDN to Ethereum block: %v. contents: %v", bdnBlock.Hash(), err, blockHex)
}
