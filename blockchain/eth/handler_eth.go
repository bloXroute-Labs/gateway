package eth

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/google/uuid"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/bsc"
	eth2 "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	responseTimeout = 5 * time.Minute
)

type ethHandler handler

func (h *ethHandler) RunPeer(peer *eth2.Peer, hand eth2.Handler) error {
	return (*handler)(h).runEthPeer(peer, hand)
}

func (h *ethHandler) Chain() *core.Chain {
	return (*handler)(h).chain
}

func (h *ethHandler) PeerInfo(id enode.ID) interface{} {
	peer, ok := h.peers.get(id.String())
	if ok {
		return peer.info()
	}

	return nil
}

// Handle processes Ethereum message packets that update internal backend state.
// In general, messages that require responses should not reach this function.
func (h *ethHandler) Handle(peer *eth2.Peer, packet eth2.Packet) error {
	switch p := packet.(type) {
	case *eth.StatusPacket:
		return nil
	case *eth.TransactionsPacket:
		return h.processTransactions(peer, *p)
	case *eth.PooledTransactionsResponse:
		return h.processTransactions(peer, *p)
	case *eth2.NewPooledTransactionHashesPacket66:
		return h.processTransactionHashes(peer, *p)
	case *eth.NewPooledTransactionHashesPacket:
		return h.processTransactionHashes(peer, p.Hashes)
	case *eth2.NewBlockPacket:
		block := &bxcommoneth.Block{Block: *p.Block} //nolint:govet
		if len(p.Sidecars) > 0 {
			block.SetBlobSidecars(p.Sidecars)
		}
		return h.processBlock(peer, core.NewBlockInfo(block, p.TD))
	case *eth.NewBlockHashesPacket:
		return h.processBlockAnnouncement(peer, *p)
	case *eth.BlockHeadersRequest:
		return h.processBlockHeaders(peer, *p)
	default:
		return fmt.Errorf("unexpected eth packet type: %v", packet)
	}
}

func (h *ethHandler) RequestTransactions(hashes []common.Hash) ([]rlp.RawValue, error) {
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

func (h *ethHandler) processTransactions(peer *eth2.Peer, txs []*ethtypes.Transaction) error {
	bdnTxs := make([]*types.BxTransaction, 0, len(txs))
	for _, tx := range txs {
		if tx.Type() == ethtypes.BlobTxType && tx.BlobTxSidecar() == nil {
			log.Debugf("blob tx %v from blockchain peer %v has no sidecar data", tx.Hash().String(), peer.IPEndpoint().IPPort())
			continue
		}

		// the transaction is considered invalid if the length of authorization_list is zero.
		if tx.Type() == ethtypes.SetCodeTxType && tx.SetCodeAuthorizations() == nil {
			log.Debugf("set code tx %v from blockchain peer %v has no authorization list", tx.Hash().String(), peer.IPEndpoint().IPPort())
			continue
		}

		if !h.isChainIDMatch(tx.ChainId().Uint64()) {
			log.Debugf("tx %v from blockchain peer %v has invalid chain id", tx.Hash().String(), peer.IPEndpoint().IPPort())
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

func (h *ethHandler) processTransactionHashes(peer *eth2.Peer, txHashes []common.Hash) error {
	sha256Hashes := make([]types.SHA256Hash, 0, len(txHashes))
	for _, hash := range txHashes {
		sha256Hashes = append(sha256Hashes, NewSHA256Hash(hash))
	}

	err := h.bridge.AnnounceTransactionHashes(peer.ID(), sha256Hashes, peer.IPEndpoint())
	if errors.Is(err, blockchain.ErrChannelFull) {
		log.Warnf("transaction announcement channel for sending to the BDN is full; dropping %v hashes...", len(txHashes))
		return nil
	}

	return err
}

func (h *ethHandler) processBlock(peer *eth2.Peer, blockInfo *core.BlockInfo) error {
	block := blockInfo.Block
	blockHash := block.Hash()
	blockHeight := block.Number()

	if h.config.Network == network.EthMainnetChainID {
		peer.Log().Errorf("ignoring block [hash=%s, height=%d] from old node", blockHash.String(), blockHeight)
		return nil
	}

	if err := h.chain.ValidateBlock(block); err != nil {
		if errors.Is(err, core.ErrAlreadySeen) {
			peer.Log().Debugf("skipping block %v (height %v): %v", blockHash.String(), blockHeight, err)
		} else {
			peer.Log().Warnf("skipping block %v (height %v): %v", blockHash.String(), blockHeight, err)
		}
		return nil
	}

	peer.Log().Debugf("processing new block %v (height %v)", blockHash.String(), blockHeight)
	newHeadCount := h.chain.AddBlock(blockInfo, core.BSBlockchain)
	h.sendConfirmedBlocksToBDN(newHeadCount, peer.IPEndpoint())
	h.broadcastBlock(block, blockInfo.TotalDifficulty(), peer)
	return nil
}

func (h *ethHandler) processBlockAnnouncement(peer *eth2.Peer, newBlocks eth.NewBlockHashesPacket) error {
	for _, block := range newBlocks {
		peer.Log().Debugf("processing new block announcement %v (height %v)", block.Hash, block.Number)

		if !h.chain.HasBlock(block.Hash) {
			p, ok := h.peers.get(peer.ID())
			if ok && p.bscExt != nil && p.bscExt.Version() == bsc.Bsc2 {
				if err := h.requestBlock(p, block.Hash, block.Number); err != nil {
					peer.Log().Errorf("error requesting block %v (height %v): %v", block.Hash, block.Number, err)
					return err
				}
			} else {
				if err := h.requestHeadersAndBodies(peer, block.Hash); err != nil {
					peer.Log().Errorf("error requesting block %v (height %v): %v", block.Hash, block.Number, err)
					return err
				}
			}
		} else {
			h.confirmBlock(block.Hash, peer.IPEndpoint())
		}
	}

	return nil
}

func (h *ethHandler) requestHeadersAndBodies(peer *eth2.Peer, blockHash common.Hash) error {
	headersCh := make(chan eth.Packet)
	bodiesCh := make(chan eth.Packet)

	err := peer.RequestBlock(blockHash, headersCh, bodiesCh)
	if err != nil {
		h.disconnectPeer(peer.ID())
		return fmt.Errorf("could not request block %v: %v", blockHash.String(), err)
	}

	go h.awaitBlockResponse(peer, blockHash, headersCh, bodiesCh)
	return nil
}

func (h *ethHandler) requestBlock(peer *ethPeer, blockHash common.Hash, blockHeight uint64) error {
	peer.Log().Debugf("requesting block %v (height %v) from BSC extension", blockHash.String(), blockHeight)

	res, err := peer.bscExt.RequestBlocksByRange(blockHeight, blockHash, 1)
	if err != nil {
		h.disconnectPeer(peer.ID())

		return fmt.Errorf("could not request block %v: %v", blockHash.String(), err)
	}

	if len(res) != 1 {
		return fmt.Errorf("could not request block %v: got %d blocks", blockHash.String(), len(res))
	}

	header := res[0].Header
	body := &eth2.BlockBody{
		Transactions: res[0].Txs,
		Uncles:       res[0].Uncles,
		Withdrawals:  res[0].Withdrawals,
		Sidecars:     res[0].Sidecars,
	}

	if err = h.processBlockComponents(peer.Peer, header, body); err != nil {
		return fmt.Errorf("could not process block components for hash %v: %v", blockHash.String(), err)
	}
	return nil
}

func (h *ethHandler) processBlockHeaders(peer *eth2.Peer, blockHeaders eth.BlockHeadersRequest) error {
	// expected to only be called when header is not expected to be part of a get block headers in response to a new block hashes message
	for _, blockHeader := range blockHeaders {
		h.confirmBlock(blockHeader.Hash(), peer.IPEndpoint())
	}
	return nil
}

func (h *ethHandler) isChainIDMatch(txChainID uint64) bool {
	// if chainID is 0 its legacy tx, so we want to propagate it;
	// if it's not 0, we need to check if its match to gw chain id
	if txChainID == 0 || txChainID == h.config.Network {
		return true
	}

	return false
}

func (h *ethHandler) sendConfirmedBlocksToBDN(count int, peerEndpoint types.NodeEndpoint) {
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
	blockHash := common.BytesToHash(b.Hash().Bytes())

	if h.chain.HasConfirmationSendToBDN(blockHash) {
		log.Tracef("block %v has already been sent in a block confirm message to gateway", b.Hash())
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

func (h *ethHandler) blockAtDepth(chainDepth int) (*types.BxBlock, error) {
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

func (h *ethHandler) broadcastBlock(block *bxcommoneth.Block, totalDifficulty *big.Int, sourceBlockchainPeer *eth2.Peer) {
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

func (h *ethHandler) awaitBlockResponse(peer *eth2.Peer, blockHash common.Hash, headersCh chan eth.Packet, bodiesCh chan eth.Packet) {
	startTime := time.Now()
	header, body, err := h.fetchBlockResponse(peer, blockHash, headersCh, bodiesCh)
	if err != nil {
		if peer.IsDisconnected() {
			peer.Log().Tracef("block %v response timed out for disconnected peer", blockHash.String())
			return
		}

		switch {
		case errors.Is(err, eth2.ErrResponseTimeout):
			peer.Log().Errorf("did not receive block header and body for block %v before timeout", blockHash.String())
		case errors.Is(err, ErrInvalidPacketType):
		default:
			peer.Log().Errorf("could not fetch block header and body for block %v: %v", blockHash.String(), err)
		}

		peer.Disconnect(p2p.DiscUselessPeer)
		h.disconnectPeer(peer.ID())
		return
	}

	elapsedTime := time.Since(startTime)
	peer.Log().Debugf("took %v to fetch block %v header and body", elapsedTime, blockHash.String())

	if err = h.processBlockComponents(peer, header, body); err != nil {
		log.Errorf("error processing block components for hash %v: %v", blockHash.String(), err)
	}
}

func (h *ethHandler) fetchBlockResponse(peer *eth2.Peer, blockHash common.Hash, headersCh, bodiesCh chan eth.Packet) (*ethtypes.Header, *eth2.BlockBody, error) {
	var (
		headers *eth.BlockHeadersRequest
		bodies  *eth2.BlockBodiesPacket
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
			errCh <- eth2.ErrResponseTimeout
		}
	}()

	go func() {
		var ok bool

		select {
		case rawBodies := <-bodiesCh:
			bodies, ok = rawBodies.(*eth2.BlockBodiesPacket)
			peer.Log().Debugf("received body for block %v", blockHash)
			if !ok {
				log.Errorf("could not convert bodies for block %v to the expected packet type, got %T", blockHash.String(), rawBodies)
				errCh <- ErrInvalidPacketType
			} else {
				errCh <- nil
			}
		case <-time.After(responseTimeout):
			errCh <- eth2.ErrResponseTimeout
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

	return (*headers)[0], bodies.BlockBodiesResponse[0], nil
}

func (h *ethHandler) confirmBlock(hash common.Hash, peerEndpoint types.NodeEndpoint) {
	newHeads := h.chain.ConfirmBlock(hash)
	h.sendConfirmedBlocksToBDN(newHeads, peerEndpoint)
}

func (h *ethHandler) processBlockComponents(peer *eth2.Peer, header *ethtypes.Header, body *eth2.BlockBody) error {
	block := bxcommoneth.NewBlockWithHeader(header).WithBody(ethtypes.Body{Transactions: body.Transactions, Uncles: body.Uncles})
	if len(body.Sidecars) > 0 {
		block = block.WithSidecars(body.Sidecars)
	}

	bi := core.NewBlockInfo(block, nil)
	err := h.chain.SetTotalDifficulty(bi)
	if err != nil {
		log.Errorf("could not set total difficulty for block %v: %v", block.Hash().String(), err)
	}

	return h.processBlock(peer, bi)
}

func (h *ethHandler) disconnectPeer(peerID string) {
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
