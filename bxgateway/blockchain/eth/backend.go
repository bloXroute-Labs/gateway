package eth

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/blockchain"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/blockchain/network"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"math/big"
	"strconv"
	"sync"
	"time"
)

const (
	blockStoreMaxSize       = 100
	blockStoreCleanInterval = 30 * time.Minute
)

// special error constant types
var (
	ErrInvalidRequest    = errors.New("invalid request")
	ErrInvalidPacketType = errors.New("invalid packet type")
	ErrBodyNotFound      = errors.New("block body not stored")
)

// Backend represents the interface to which any stateful message handling (e.g. looking up tx pool items or block headers) will be passed to for processing
type Backend interface {
	NetworkConfig() *network.EthConfig
	RunPeer(peer *Peer, handler func(*Peer) error) error
	Handle(peer *Peer, packet eth.Packet) error
	GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethtypes.Header, error)
	GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error)
}

// blockRef represents block info used for storing best block
type blockRef struct {
	height int
	hash   ethcommon.Hash
}

// Handler is the Ethereum backend implementation. It passes transactions and blocks to the BDN bridge and tracks received blocks and transactions from peers.
type Handler struct {
	bridge blockchain.Bridge
	peers  *peerSet
	cancel context.CancelFunc
	config *network.EthConfig

	headerLock            sync.RWMutex // lock for block headers/heights
	blockLock             sync.Mutex   // lock for processing blocks from BDN/blockchain
	heightToBlockHeaders  cmap.ConcurrentMap
	blockHashToHeight     cmap.ConcurrentMap
	blockHashToBody       cmap.ConcurrentMap
	blockHashToDifficulty cmap.ConcurrentMap

	bestBlock         blockRef
	partialChainstate []blockRef
}

// NewHandler returns a new Handler and starts its processing go routines
func NewHandler(parent context.Context, bridge blockchain.Bridge, config *network.EthConfig, ethWSUri string) *Handler {
	return newHandler(parent, bridge, config, ethWSUri, blockStoreCleanInterval, blockStoreMaxSize)
}

func newHandler(parent context.Context, bridge blockchain.Bridge, config *network.EthConfig, ethWSUri string, cleanStorageInterval time.Duration, maxSize int) *Handler {
	ctx, cancel := context.WithCancel(parent)
	h := &Handler{
		bridge:                bridge,
		peers:                 newPeerSet(),
		cancel:                cancel,
		config:                config,
		heightToBlockHeaders:  cmap.New(),
		blockHashToHeight:     cmap.New(),
		blockHashToBody:       cmap.New(),
		blockHashToDifficulty: cmap.New(),
		bestBlock:             blockRef{0, ethcommon.Hash{}},
	}
	go h.checkInitialBlockchainLiveliness(100 * time.Second)
	go h.handleBDNBridge(ctx)
	go h.cleanBlockStorage(cleanStorageInterval, maxSize)
	if ethWSUri != "" {
		go h.runEthSub(ethWSUri)
	}
	return h
}

func (h *Handler) handleFeeds(wsLog *log.Entry, nhSub *rpc.ClientSubscription, nptSub *rpc.ClientSubscription, newHeadsRespCh chan *ethtypes.Header, nptResponseCh chan ethcommon.Hash) {
	for {
		select {
		case err := <-nhSub.Err():
			wsLog.Errorf("failed to get notification from newHeads: %v", err)
			return
		case newHeader := <-newHeadsRespCh:
			wsLog.Tracef("received header for block %v (height %v)", newHeader.Hash(), newHeader.Number)
			bxBlock, err := h.createBxBlockFromEthHeader(newHeader)
			if err == ErrBodyNotFound {
				wsLog.Debugf("could not locate body for block %v (height %v), skipping", newHeader.Hash(), newHeader.Number)
				continue
			}
			if err != nil {
				var headerByteString string
				b, e := rlp.EncodeToBytes(newHeader)
				if e == nil {
					headerByteString = hex.EncodeToString(b)
				} else {
					headerByteString = fmt.Sprint(newHeader)
				}

				wsLog.WithField("header", headerByteString).Errorf("failed to create BDN block from NewHeads notification: %v", err)
				continue
			}

			err = h.bridge.SendBlockToBDN(bxBlock, types.NodeEndpoint{})
			if err != nil {
				wsLog.Errorf("failed to send block from NewHeads to BDN %v: %v", newHeader, err)
			}
		case err := <-nptSub.Err():
			wsLog.Errorf("failed to get notification from newPendingTransactions: %v", err)
			return
		case newPendingTx := <-nptResponseCh:
			txHash, err := types.NewSHA256Hash(newPendingTx[:])
			wsLog.Tracef("received new pending tx %v", txHash)
			hashes := types.SHA256HashList{txHash}
			err = h.bridge.AnnounceTransactionHashes(bxgateway.WSConnectionID, hashes)
			if err != nil {
				wsLog.Errorf("failed to send transaction %v to the gateway: %v", txHash, err)
			}
		}
	}
}

func (h *Handler) runEthSub(ethWSUri string) {
	wsLog := log.WithFields(log.Fields{
		"connType":   "WS",
		"remoteAddr": ethWSUri,
	})
	var client *rpc.Client
	var err error

	for {
		// gateway should retry connecting to the ws url until it's successfully connected
		client, err = rpc.Dial(ethWSUri)
		if err != nil {
			time.Sleep(5 * time.Second)
			log.Warnf("Failed to dial %v, retrying..", ethWSUri)
			continue
		}

		log.Infof("Connection was successfully established with %v", ethWSUri)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		newHeadsRespCh := make(chan *ethtypes.Header)
		nptResponseCh := make(chan ethcommon.Hash)

		nhSub, err := client.EthSubscribe(ctx, newHeadsRespCh, "newHeads")
		if err != nil {
			wsLog.Errorf("failed to subscribe with newHeads: %v", err)
			client.Close()
			continue
		}
		nptSub, err := client.EthSubscribe(ctx, nptResponseCh, "newPendingTransactions")
		if err != nil {
			wsLog.Errorf("failed to subscribe with newPendingTransactions: %v", err)
			nhSub.Unsubscribe()
			client.Close()
			continue
		}

		// process feeds
		h.handleFeeds(wsLog, nhSub, nptSub, newHeadsRespCh, nptResponseCh)

		// closing connection and trying reconnection
		nhSub.Unsubscribe()
		nptSub.Unsubscribe()
		client.Close()
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
	if err := h.peers.register(ep); err != nil {
		return err
	}
	defer func() {
		ep.Stop()
		_ = h.peers.unregister(ep.ID())
	}()

	ep.Start()
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
		case request := <-h.bridge.ReceiveBlockRequest():
			h.processBDNBlockRequest(request)
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
	err = h.setTotalDifficulty(ethBlockInfo)
	if err != nil {
		log.Debugf("could not resolve difficulty for block %v, announcing instead", ethBlock.Hash())
		h.broadcastBlockAnnouncement(ethBlock)
	} else {
		h.broadcastBlock(ethBlock, ethBlockInfo.TotalDifficulty())
	}
}

func (h *Handler) setTotalDifficulty(info *BlockInfo) error {
	totalDifficulty := info.TotalDifficulty()
	if totalDifficulty != nil {
		return nil
	}

	parentHash := info.Block.ParentHash()
	parentDifficulty, ok := h.getBlockDifficulty(parentHash)
	if ok {
		info.SetTotalDifficulty(new(big.Int).Add(parentDifficulty, info.Block.Difficulty()))
		h.storeBlockDifficulty(info.Block.Hash(), info.TotalDifficulty())
		return nil
	}
	return errors.New("could not calculate difficulty")
}

func (h *Handler) processBDNBlockRequest(request blockchain.BlockAnnouncement) {
	ethBlockHash := ethcommon.BytesToHash(request.Hash[:])
	peer, ok := h.peers.get(request.PeerID)
	if !ok {
		log.Warnf("peer %v announced block %v, but is not available for querying", request.PeerID, ethBlockHash)
		return
	}

	headersCh := make(chan eth.Packet)
	bodiesCh := make(chan eth.Packet)

	if peer.isVersion66() {
		err := peer.RequestBlock66(ethBlockHash, headersCh, bodiesCh)
		if err != nil {
			peer.Log().Errorf("could not request block %v: %v", ethBlockHash, err)
			return
		}
		go h.awaitBlockResponse(peer, ethBlockHash, headersCh, bodiesCh, h.fetchBlockResponse66)
	} else {
		err := peer.RequestBlock(ethBlockHash, headersCh, bodiesCh)
		if err != nil {
			peer.Log().Errorf("could not request block %v: %v", ethBlockHash, err)
			return
		}
		go h.awaitBlockResponse(peer, ethBlockHash, headersCh, bodiesCh, h.fetchBlockResponse)
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
	_ = h.setTotalDifficulty(blockInfo)
	return h.processBlock(peer, blockInfo)
}

// Handle processes Ethereum message packets that update internal backend state. In general, messages that require responses should not reach this function.
func (h *Handler) Handle(peer *Peer, packet eth.Packet) error {
	switch p := packet.(type) {
	case *eth.StatusPacket:
		// store an initial difficulty if needed to start calculating total difficulties
		if h.blockHashToDifficulty.Count() == 0 {
			h.storeBlockDifficulty(p.Head, p.TD)
		}
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
	default:
		return fmt.Errorf("unexpected eth packet type: %v", packet)
	}
}

func (h *Handler) createBxBlockFromEthHeader(header *ethtypes.Header) (*types.BxBlock, error) {
	body, ok := h.getBlockBody(header.Hash())
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
	h.blockLock.Lock()
	defer h.blockLock.Unlock()

	block := blockInfo.Block
	blockHash := block.Hash()
	if h.hasBlock(blockHash) {
		peer.Log().Debugf("skipping duplicate block %v", blockHash)
		return nil
	}

	peer.Log().Debugf("processing new block %v (height %v)", blockHash, block.Number())

	h.storeBlock(block, blockInfo.TotalDifficulty())

	bdnBlock, err := h.bridge.BlockBlockchainToBDN(blockInfo)
	if err != nil {
		return err
	}
	return h.bridge.SendBlockToBDN(bdnBlock, peer.IPEndpoint())
}

func (h *Handler) processBlockAnnouncement(peer *Peer, blockHashes eth.NewBlockHashesPacket) error {
	sha256Hashes := make([]types.SHA256Hash, 0, len(blockHashes))
	for _, blockHash := range blockHashes {
		sha256Hashes = append(sha256Hashes, NewSHA256Hash(blockHash.Hash))
		peer.Log().Debugf("processing new block announcement %v (height %v)", blockHash.Hash, blockHash.Number)
	}

	err := h.bridge.AnnounceBlockHashes(peer.ID(), peer.endpoint, sha256Hashes)

	if err == blockchain.ErrChannelFull {
		log.Warnf("block announcement channel for sending to the BDN is full; dropping %v hashes...", len(blockHashes))
		return nil
	}

	return err
}

// GetBodies assembles and returns a set of block bodies
func (h *Handler) GetBodies(hashes []ethcommon.Hash) ([]*ethtypes.Body, error) {
	bodies := make([]*ethtypes.Body, 0, len(hashes))
	for _, hash := range hashes {
		body, ok := h.getBlockBody(hash)
		if !ok {
			return nil, fmt.Errorf("could not fetch block body: %v", hash)
		}
		bodies = append(bodies, body)
	}
	return bodies, nil
}

// GetHeaders assembles and returns a set of headers
func (h *Handler) GetHeaders(start eth.HashOrNumber, count int, skip int, reverse bool) ([]*ethtypes.Header, error) {
	requestedHeaders := make([]*ethtypes.Header, 0, count)

	var (
		originHash   ethcommon.Hash
		originHeight int
		ok           bool
	)

	// figure out query scheme, then initialize requested headers with the first entry
	if start.Number > 0 {
		originHeight = int(start.Number)
		headers, ok := h.getHeadersAtHeight(originHeight)
		if !ok {
			return nil, fmt.Errorf("no entries stored at height: %v", originHeight)
		}

		var (
			originHeader *ethtypes.Header
			err          error
		)
		if len(headers) > 1 {
			originHeader, err = h.resolveForkedHeaders(headers, originHeight)
			if err != nil {
				return nil, err
			}
		} else {
			originHeader = headers[0]
		}
		requestedHeaders = append(requestedHeaders, originHeader)
	} else if start.Hash != (ethcommon.Hash{}) {
		originHash = start.Hash
		originHeight, ok = h.getBlockHeight(originHash)
		if !ok {
			return nil, fmt.Errorf("could not retrieve a corresponding height for block: %v", originHash)
		}
		originHeader, ok := h.getBlockHeader(originHeight, originHash)
		if !ok {
			return nil, fmt.Errorf("no header was with height %v and hash %v", originHeight, originHash)
		}
		requestedHeaders = append(requestedHeaders, originHeader)
	} else {
		return nil, ErrInvalidRequest
	}

	directionalMultiplier := 1
	increment := skip + 1
	if reverse {
		directionalMultiplier = -1
	}
	increment *= directionalMultiplier

	// iterate through all requested headers and fetch results
	for height := originHeight + increment; len(requestedHeaders) < count; height += increment {
		headers, ok := h.getHeadersAtHeight(height)
		if !ok {
			if height > h.bestBlock.height {
				log.Tracef("requested height %v is beyond best height: ok", height)
				break
			}
			return nil, fmt.Errorf("no entries stored at height: %v", originHeight)
		}
		if len(headers) > 1 {
			log.Tracef("fork detected: consulting partial chainstate to determine correct header")
			correctHeader, err := h.resolveForkedHeaders(headers, height)
			if err != nil {
				return nil, err
			}
			requestedHeaders = append(requestedHeaders, correctHeader)
		} else {
			requestedHeaders = append(requestedHeaders, headers[0])
		}
	}

	return requestedHeaders, nil
}

func (h *Handler) getBlockHeight(hash ethcommon.Hash) (int, bool) {
	blockHeight, ok := h.blockHashToHeight.Get(hash.String())
	if !ok {
		return 0, ok
	}
	return blockHeight.(int), ok
}

func (h *Handler) storeBlockHeight(hash ethcommon.Hash, height int) {
	h.blockHashToHeight.Set(hash.String(), height)
}

func (h *Handler) removeBlockHeight(hash ethcommon.Hash) {
	h.blockHashToHeight.Remove(hash.String())
}

func (h *Handler) getHeadersAtHeight(height int) ([]*ethtypes.Header, bool) {
	headers, ok := h.heightToBlockHeaders.Get(strconv.Itoa(height))
	if !ok {
		return nil, ok
	}
	return headers.([]*ethtypes.Header), ok
}

func (h *Handler) storeHeaderAtHeight(height int, header *ethtypes.Header) {
	// concurrent calls to this function are ok, only needs to be exclusionary with clean
	h.headerLock.RLock()
	defer h.headerLock.RUnlock()

	heightStr := strconv.Itoa(height)
	ok := h.heightToBlockHeaders.SetIfAbsent(heightStr, []*ethtypes.Header{header})
	if !ok {
		headers, _ := h.getHeadersAtHeight(height)
		headers = append(headers, header)
		h.heightToBlockHeaders.Set(heightStr, headers)
	}
}

func (h *Handler) getBlockHeader(height int, hash ethcommon.Hash) (*ethtypes.Header, bool) {
	headers, ok := h.getHeadersAtHeight(height)
	if !ok {
		return nil, ok
	}
	for _, header := range headers {
		if bytes.Equal(header.Hash().Bytes(), hash.Bytes()) {
			return header, true
		}
	}
	return nil, false
}

func (h *Handler) hasHeader(hash ethcommon.Hash) bool {
	return h.blockHashToHeight.Has(hash.String())
}

func (h *Handler) storeBlockHeader(header *ethtypes.Header) {
	blockHash := header.Hash()
	height := int(header.Number.Int64())
	h.storeHeaderAtHeight(height, header)
	h.storeBlockHeight(blockHash, height)
	if height > h.bestBlock.height {
		h.bestBlock = blockRef{height, blockHash}
	}
}

func (h *Handler) storeBlockDifficulty(hash ethcommon.Hash, difficulty *big.Int) {
	if difficulty != nil {
		h.blockHashToDifficulty.Set(hash.String(), difficulty)
	}
}

func (h *Handler) getBlockDifficulty(hash ethcommon.Hash) (*big.Int, bool) {
	difficulty, ok := h.blockHashToDifficulty.Get(hash.String())
	if !ok {
		return nil, ok
	}
	return difficulty.(*big.Int), ok
}

func (h *Handler) removeBlockDifficulty(hash ethcommon.Hash) {
	h.blockHashToDifficulty.Remove(hash.String())
}

func (h *Handler) storeBlockBody(hash ethcommon.Hash, body *ethtypes.Body) {
	h.blockHashToBody.Set(hash.String(), body)
}

func (h *Handler) getBlockBody(hash ethcommon.Hash) (*ethtypes.Body, bool) {
	body, ok := h.blockHashToBody.Get(hash.String())
	if !ok {
		return nil, ok
	}
	return body.(*ethtypes.Body), ok
}

func (h *Handler) hasBody(hash ethcommon.Hash) bool {
	return h.blockHashToBody.Has(hash.String())
}

func (h *Handler) removeBlockBody(hash ethcommon.Hash) {
	h.blockHashToBody.Remove(hash.String())
}

// storeBDNBlock will return a nil block and no error if block is a duplicate
func (h *Handler) storeBDNBlock(bdnBlock *types.BxBlock) (*BlockInfo, error) {
	h.blockLock.Lock()
	defer h.blockLock.Unlock()

	blockHash := ethcommon.BytesToHash(bdnBlock.Hash().Bytes())
	if h.hasBlock(blockHash) {
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

	h.storeBlock(ethBlockInfo.Block, ethBlockInfo.TotalDifficulty())
	return ethBlockInfo, nil
}

func (h *Handler) storeBlock(block *ethtypes.Block, difficulty *big.Int) {
	h.storeBlockHeader(block.Header())
	h.storeBlockBody(block.Hash(), block.Body())
	h.storeBlockDifficulty(block.Hash(), difficulty)
}

func (h *Handler) hasBlock(hash ethcommon.Hash) bool {
	return h.hasHeader(hash) && h.hasBody(hash)
}

// selects the header in the provided list that's part of the chain state (headers are expected to all be at the same height)
func (h *Handler) resolveForkedHeaders(headers []*ethtypes.Header, blockNumber int) (*ethtypes.Header, error) {
	requiredLength := h.bestBlock.height - blockNumber + 1
	expectedIndex := h.bestBlock.height - blockNumber

	err := h.refreshPartialChainstate(requiredLength)
	if err != nil {
		return nil, err
	}

	chainstateHash := h.partialChainstate[expectedIndex].hash
	for _, candidateHeader := range headers {
		candidateHash := candidateHeader.Hash()
		if chainstateHash == candidateHash {
			return candidateHeader, nil
		}
	}

	// collect information for errors (this should be very rare)
	hashes := make([]ethcommon.Hash, 0, len(headers))
	for _, header := range headers {
		hashes = append(hashes, header.Hash())
	}

	return nil, fmt.Errorf("none of the headers were located in the chainstate: %v", hashes)
}

func (h *Handler) refreshPartialChainstate(requiredLength int) error {
	if len(h.partialChainstate) == 0 {
		h.partialChainstate = append(h.partialChainstate, blockRef{
			height: h.bestBlock.height,
			hash:   h.bestBlock.hash,
		})
	}

	// build partial chainstate forward from current head until reaching previous best height
	// e.g. suppose between two calls of this function, this best block has moved from 10 to 20.
	//		partial chainstate still has the best block as 10, so the entries from 11-20 need to prepended
	chainHead := h.partialChainstate[0]
	if chainHead.hash != h.bestBlock.hash {
		missingHeadEntries := make([]blockRef, 0, h.bestBlock.height-chainHead.height)
		missingHeadEntries = append(missingHeadEntries, blockRef{h.bestBlock.height, h.bestBlock.hash})
		headHeight := h.bestBlock.height
		headHash := h.bestBlock.hash
		for height := h.bestBlock.height; height > chainHead.height; height-- {
			headHeader, ok := h.getBlockHeader(headHeight, headHash)
			if !ok {
				break
			}
			headHash = headHeader.ParentHash
		}

		if headHeight == chainHead.height && headHash == chainHead.hash {
			h.partialChainstate = append(missingHeadEntries, h.partialChainstate...)
		} else {
			// reorganization is required, rebuild to expected length
			h.partialChainstate = missingHeadEntries
		}
	}

	chainHeadHeight := h.partialChainstate[0].height
	requiredLowestHeight := chainHeadHeight - requiredLength

	// build partial chainstate backward from tail until reach required length
	// e.g. suppose the current partial chainstate contains blocks 10-20 (20 = best block), but requiredLength = 15
	//		this will add blocks 5-9 to the end of partial chainstate
	chainTail := h.partialChainstate[len(h.partialChainstate)-1]
	tailHeight := chainTail.height
	tailHash := chainTail.hash
	tail, ok := h.getBlockHeader(tailHeight, tailHash)
	if !ok {
		return fmt.Errorf("unable to construct partial chainstate (requested %v=>%v): block header %v at height %v not found", chainHead.height, requiredLowestHeight, chainTail.hash, chainTail.height)
	}
	for len(h.partialChainstate) < requiredLength {
		tailHeight--
		tailHash = tail.ParentHash
		h.partialChainstate = append(h.partialChainstate, blockRef{tailHeight, tailHash})
		tail, ok = h.getBlockHeader(tailHeight, tailHash)
		if !ok {
			return fmt.Errorf("unable to construct partial chainstate (requested %v=>%v): block header %v at height %v not found", chainHead.height, requiredLowestHeight, chainTail.hash, chainTail.height)
		}
	}
	return nil
}

func (h *Handler) cleanBlockStorage(cleanupFreq time.Duration, maxSize int) {
	ticker := time.NewTicker(cleanupFreq)
	for {
		select {
		case <-ticker.C:
			h.clean(maxSize)
		}
	}
}

func (h *Handler) clean(maxSize int) (lowestCleaned int, highestCleaned int, numCleaned int) {
	h.headerLock.Lock()
	defer h.headerLock.Unlock()

	numCleaned = 0
	lowestCleaned = h.bestBlock.height
	highestCleaned = 0

	// minimum height to not be cleaned
	minHeight := h.bestBlock.height - maxSize + 1
	numHeadersStored := h.heightToBlockHeaders.Count()

	if numHeadersStored >= maxSize {
		for elem := range h.heightToBlockHeaders.IterBuffered() {
			heightStr := elem.Key
			height, err := strconv.Atoi(heightStr)
			if err != nil {
				log.Errorf("failed to convert height %v from string to integer: %v", heightStr, err)
				continue
			}
			if height < minHeight {
				headers := elem.Val.([]*ethtypes.Header)
				h.heightToBlockHeaders.Remove(heightStr)
				for _, header := range headers {
					hash := header.Hash()
					h.removeBlockHeight(hash)
					h.removeBlockBody(hash)
					h.removeBlockDifficulty(hash)
					numCleaned++
					if height < lowestCleaned {
						lowestCleaned = height
					}
					if height > highestCleaned {
						highestCleaned = height
					}
				}
			}
		}

		log.Debugf("cleaned block storage (previous size %v out of max %v): %v block headers from %v to %v", numHeadersStored, maxSize, numCleaned, lowestCleaned, highestCleaned)
	} else {
		log.Debugf("skipping block storage cleanup, only had %v block headers out of a limit of %v", numHeadersStored, maxSize)
	}
	return
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
