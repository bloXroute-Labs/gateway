package eth

import (
	"fmt"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	log "github.com/sirupsen/logrus"
)

func handleGetBlockHeaders(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockHeadersPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	headers, err := answerGetBlockHeaders(backend, &query, peer)
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
	headers, err := backend.GetHeaders(query.Origin, int(query.Amount), int(query.Skip), query.Reverse)
	if err == ErrInvalidRequest {
		return nil, err
	} else if err != nil {
		log.Warnf("blockchain node %v is not synced. Missing %v headers starting at %v", peer, int(query.Amount), query.Origin)
		return []*ethtypes.Header{}, nil
	}
	return headers, nil
}

func handleGetBlockBodies(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockBodiesPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	bodies, err := answerGetBlockBodies(backend, query)
	if err != nil {
		log.Errorf("failed to retrieve block bodies hashes: %v", query)
		return nil
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
		log.Errorf("failed to retrieve block bodies for request ID %v, hashes: %v", query.RequestId, query.GetBlockBodiesPacket)
		return nil
	}
	return peer.ReplyBlockBodies(query.RequestId, bodies)
}

func answerGetBlockBodies(backend Backend, query eth.GetBlockBodiesPacket) ([]*eth.BlockBody, error) {
	bodies, err := backend.GetBodies(query)
	if err != nil {
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
	peer.UpdateHead(blockPacket.Block.NumberU64())
	return backend.Handle(peer, &blockPacket)
}

func handleTransactions(backend Backend, msg Decoder, peer *Peer) error {
	var txs eth.TransactionsPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}
	for _, tx := range txs {
		log.Tracef("%v: receive tx %v", peer, tx.Hash())
	}

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

	for _, tx := range pooledTxsResponse.PooledTransactionsPacket {
		log.Tracef("%v: received pooled tx %v", peer, tx.Hash())
	}

	return backend.Handle(peer, &pooledTxsResponse.PooledTransactionsPacket)
}

func handleNewPooledTransactionHashes(backend Backend, msg Decoder, peer *Peer) error {
	var txHashes eth.NewPooledTransactionHashesPacket
	if err := msg.Decode(&txHashes); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	for _, txHash := range txHashes {
		log.Tracef("%v: received tx announcement %v", peer, txHash)
	}

	return backend.Handle(peer, &txHashes)
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
	peer.NotifyResponse(&blockHeaders)
	return nil
}

func handleBlockHeaders66(backend Backend, msg Decoder, peer *Peer) error {
	var blockHeaders eth.BlockHeadersPacket66
	if err := msg.Decode(&blockHeaders); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	updatePeerHeadFromHeaders(blockHeaders.BlockHeadersPacket, peer)
	return peer.NotifyResponse66(blockHeaders.RequestId, &blockHeaders.BlockHeadersPacket)
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

	return peer.NotifyResponse66(blockBodies.RequestId, &blockBodies.BlockBodiesPacket)
}

func updatePeerHeadFromHeaders(headers eth.BlockHeadersPacket, peer *Peer) {
	if len(headers) > 0 {
		maxHeight := headers[0].Number
		for _, header := range headers[1:] {
			number := header.Number
			if number.Cmp(maxHeight) == 1 {
				maxHeight = number
			}
		}
		peer.UpdateHead(maxHeight.Uint64())
	}
}

func updatePeerHeadFromNewHashes(newBlocks eth.NewBlockHashesPacket, peer *Peer) {
	if len(newBlocks) > 0 {
		for _, newBlock := range newBlocks[1:] {
			maxHeight := newBlocks[0].Number
			number := newBlock.Number
			if number > maxHeight {
				maxHeight = number
			}
			peer.UpdateHead(maxHeight)
		}
	}
}

func handleUnimplemented(backend Backend, msg Decoder, peer *Peer) error {
	return nil
}
