package eth

import (
	"fmt"
	"math"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
)

func handleGetBlockHeaders(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockHeadersPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	headers, err := answerGetBlockHeaders(backend, &query, peer)
	if err == ErrAncientHeaders {
		go func() {
			peer.Log().Debugf("requested ancient headers, fetching result from blockchain node: %v", query)
			headerCh := make(chan eth.Packet)
			err := peer.RequestBlockHeaderRaw(query.Origin, query.Amount, query.Skip, query.Reverse, headerCh)
			if err != nil {
				peer.Log().Errorf("could not request headers from peer: %v", err)
				return
			}
			headersResponse := (<-headerCh).(*eth.BlockHeadersPacket)
			peer.Log().Debugf("successfully fetched %v ancient headers from blockchain node", len(*headersResponse))

			err = peer.SendBlockHeaders(*headersResponse)
			if err != nil {
				peer.Log().Errorf("could not send headers to peer: %v", err)
			}
		}()
		return nil
	}

	if err != nil {
		return nil
	}
	return peer.SendBlockHeaders(headers)
}

func handleGetBlockHeaders66(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockHeadersPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	headers, err := answerGetBlockHeaders(backend, query.GetBlockHeadersPacket, peer)
	if err == ErrAncientHeaders {
		go func() {
			peer.Log().Debugf("requested (id: %v) ancient headers, fetching result from blockchain node: %v", query.RequestId, query.GetBlockHeadersPacket)
			headerCh := make(chan eth.Packet)

			err := peer.RequestBlockHeaderRaw(query.Origin, query.Amount, query.Skip, query.Reverse, headerCh)
			if err != nil {
				peer.Log().Errorf("could not request headers from peer: %v", err)
				return
			}

			headersResponse := (<-headerCh).(*eth.BlockHeadersPacket)
			peer.Log().Debugf("successfully fetched %v ancient headers from blockchain node (id: %v)", len(*headersResponse), query.RequestId)

			err = peer.ReplyBlockHeaders(query.RequestId, *headersResponse)
			if err != nil {
				peer.Log().Errorf("could not send headers to peer: %v", err)
			}
		}()
		return nil
	}
	if err != nil {
		return nil
	}
	return peer.ReplyBlockHeaders(query.RequestId, headers)
}

func answerGetBlockHeaders(backend Backend, query *eth.GetBlockHeadersPacket, peer *Peer) ([]*ethtypes.Header, error) {
	if !peer.checkpointPassed {
		peer.checkpointPassed = true
		return []*ethtypes.Header{}, nil
	}
	if query.Amount > math.MaxInt32 {
		peer.Log().Warnf("could not retrieve all %v headers, maximum query amount is %v", query.Amount, math.MaxInt32)
		return []*ethtypes.Header{}, nil
	}
	headers, err := backend.GetHeaders(query.Origin, int(query.Amount), int(query.Skip), query.Reverse)

	switch {
	case err == ErrInvalidRequest || err == ErrAncientHeaders:
		return nil, err
	case err == ErrFutureHeaders:
		return []*ethtypes.Header{}, nil
	case err != nil:
		peer.Log().Warnf("could not retrieve all %v headers starting at %v, err: %v", int(query.Amount), query.Origin, err)
		return []*ethtypes.Header{}, nil
	default:
		return headers, nil
	}
}

func handleGetBlockBodies(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockBodiesPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	bodies, err := answerGetBlockBodies(backend, query)
	if err != nil {
		log.Errorf("error retrieving block bodies hashes %v: %v", query, err)
		return err
	}
	return peer.SendBlockBodies(bodies)
}

func handleGetBlockBodies66(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockBodiesPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	bodies, err := answerGetBlockBodies(backend, query.GetBlockBodiesPacket)
	if err != nil {
		log.Errorf("error retrieving block bodies for request ID %v, hashes %v: %v", query.RequestId, query.GetBlockBodiesPacket, err)
		return err
	}
	return peer.ReplyBlockBodies(query.RequestId, bodies)
}

func answerGetBlockBodies(backend Backend, query eth.GetBlockBodiesPacket) ([]*eth.BlockBody, error) {
	bodies, err := backend.GetBodies(query)
	if err == ErrBodyNotFound {
		log.Debugf("could not find all block bodies: %v", query)
		return []*eth.BlockBody{}, nil
	} else if err != nil {
		return nil, err
	}

	blockBodies := make([]*eth.BlockBody, 0, len(bodies))
	for _, body := range bodies {
		blockBody := &eth.BlockBody{
			Transactions: body.Transactions,
			Uncles:       body.Uncles,
		}
		blockBodies = append(blockBodies, blockBody)
	}

	return blockBodies, nil
}

func handleNewBlockMsg(backend Backend, msg Decoder, peer *Peer) error {
	var blockPacket eth.NewBlockPacket
	if err := msg.Decode(&blockPacket); err != nil {
		return fmt.Errorf("could not decode message %v: %v", msg, err)
	}
	peer.UpdateHead(blockPacket.Block.NumberU64(), blockPacket.Block.Hash())
	return backend.Handle(peer, &blockPacket)
}

func handleTransactions(backend Backend, msg Decoder, peer *Peer) error {
	var txs eth.TransactionsPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}
	hashes := make([]common.Hash, len(txs))
	for idx, tx := range txs {
		hashes[idx] = tx.Hash()
	}
	log.Tracef("%v: receive tx %v", peer, hashes)

	return backend.Handle(peer, &txs)
}

func handlePooledTransactions(backend Backend, msg Decoder, peer *Peer) error {
	var txs eth.PooledTransactionsPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	for _, tx := range txs {
		log.Tracef("%v: received pooled tx %v", peer, tx.Hash())
	}

	return backend.Handle(peer, &txs)
}

func handlePooledTransactions66(backend Backend, msg Decoder, peer *Peer) error {
	var pooledTxsResponse eth.PooledTransactionsPacket66
	if err := msg.Decode(&pooledTxsResponse); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}
	// TODO: check why we get empty
	if len(pooledTxsResponse.PooledTransactionsPacket) == 0 {
		return nil
	}
	hashes := make([]common.Hash, len(pooledTxsResponse.PooledTransactionsPacket))
	for idx, tx := range pooledTxsResponse.PooledTransactionsPacket {
		hashes[idx] = tx.Hash()
	}

	log.Tracef("%v: received pooled txs %v", peer, len(hashes))
	return backend.Handle(peer, &pooledTxsResponse.PooledTransactionsPacket)
}

func handleNewPooledTransactionHashes(backend Backend, msg Decoder, peer *Peer) error {
	var txHashes eth.NewPooledTransactionHashesPacket66
	if err := msg.Decode(&txHashes); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}
	log.Tracef("%v: received tx announcement of %v transactions", peer, len(txHashes))

	return backend.Handle(peer, &txHashes)
}

func handleNewPooledTransactionHashes68(backend Backend, msg Decoder, peer *Peer) error {
	var txs eth.NewPooledTransactionHashesPacket68
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	log.Tracef("%v: received tx announcement of %v transactions", peer, len(txs.Hashes))

	return backend.Handle(peer, &txs)
}

func handleNewBlockHashes(backend Backend, msg Decoder, peer *Peer) error {
	var blockHashes eth.NewBlockHashesPacket
	if err := msg.Decode(&blockHashes); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	updatePeerHeadFromNewHashes(blockHashes, peer)
	return backend.Handle(peer, &blockHashes)
}

func handleBlockHeaders(backend Backend, msg Decoder, peer *Peer) error {
	var blockHeaders eth.BlockHeadersPacket
	if err := msg.Decode(&blockHeaders); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	updatePeerHeadFromHeaders(blockHeaders, peer)
	handled := peer.NotifyResponse(&blockHeaders)

	if handled {
		return nil
	}
	return backend.Handle(peer, &blockHeaders)
}

func handleBlockHeaders66(backend Backend, msg Decoder, peer *Peer) error {
	var blockHeaders eth.BlockHeadersPacket66
	if err := msg.Decode(&blockHeaders); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	updatePeerHeadFromHeaders(blockHeaders.BlockHeadersPacket, peer)
	handled, err := peer.NotifyResponse66(blockHeaders.RequestId, &blockHeaders.BlockHeadersPacket)

	if err != nil {
		return err
	}

	if handled {
		return nil
	}

	return backend.Handle(peer, &blockHeaders.BlockHeadersPacket)
}

func handleBlockBodies(backend Backend, msg Decoder, peer *Peer) error {
	var blockBodies eth.BlockBodiesPacket
	if err := msg.Decode(&blockBodies); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	peer.NotifyResponse(&blockBodies)
	return nil
}

func handleBlockBodies66(backend Backend, msg Decoder, peer *Peer) error {
	var blockBodies eth.BlockBodiesPacket66
	if err := msg.Decode(&blockBodies); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	_, err := peer.NotifyResponse66(blockBodies.RequestId, &blockBodies.BlockBodiesPacket)
	return err
}

func updatePeerHeadFromHeaders(headers eth.BlockHeadersPacket, peer *Peer) {
	if len(headers) > 0 {
		maxHeight := headers[0].Number
		hash := headers[0].Hash()
		for _, header := range headers[1:] {
			number := header.Number
			if number.Cmp(maxHeight) == 1 {
				maxHeight = number
				hash = header.Hash()
			}
		}
		peer.UpdateHead(maxHeight.Uint64(), hash)
	}
}

func updatePeerHeadFromNewHashes(newBlocks eth.NewBlockHashesPacket, peer *Peer) {
	if len(newBlocks) > 0 {
		maxHeight := newBlocks[0].Number
		hash := newBlocks[0].Hash
		for _, newBlock := range newBlocks[1:] {
			number := newBlock.Number
			if number > maxHeight {
				maxHeight = number
				hash = newBlock.Hash
			}
		}
		peer.UpdateHead(maxHeight, hash)
	}
}

func handleUnimplemented(backend Backend, msg Decoder, peer *Peer) error {
	return nil
}
