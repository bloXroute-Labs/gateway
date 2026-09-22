package eth

import (
	"errors"
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
)

func handleGetBlockHeaders(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockHeadersPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	headers, err := answerGetBlockHeaders(backend, &query, peer)
	if errors.Is(err, core.ErrAncientHeaders) {
		go func() {
			peer.Log().Debugf("requested (id: %v) ancient headers, fetching result from blockchain node: %v", query.RequestId, query)
			headerCh := make(chan eth.Packet)

			err := peer.RequestBlockHeaderRaw(query.Origin, query.Amount, query.Skip, query.Reverse, headerCh)
			if err != nil {
				peer.Log().Errorf("could not request headers from peer: %v", err)
				return
			}

			headersResponse := (<-headerCh).(*eth.BlockHeadersRequest)
			peer.Log().Debugf("successfully fetched %v ancient headers from blockchain node (id: %v)", len(*headersResponse), query.RequestId)

			resp := make([]rlp.RawValue, len(*headersResponse))
			for i := range *headersResponse {
				rlpData, err := rlp.EncodeToBytes((*headersResponse)[i])
				if err != nil {
					peer.Log().Errorf("could not encode header to rlp: %v", err)
					return
				}
				resp[i] = rlpData
			}

			err = peer.ReplyBlockHeadersRLP(query.RequestId, resp)
			if err != nil {
				peer.Log().Errorf("could not send headers to peer: %v", err)
			}
		}()

		return nil
	}

	if err != nil {
		return nil
	}

	return peer.ReplyBlockHeadersRLP(query.RequestId, headers)
}

func answerGetBlockHeaders(backend Backend, query *eth.GetBlockHeadersPacket, peer *Peer) ([]rlp.RawValue, error) {
	if !peer.checkpointPassed {
		peer.checkpointPassed = true
		return []rlp.RawValue{}, nil
	}
	if query.Amount > math.MaxInt32 {
		peer.Log().Warnf("could not retrieve all %v headers, maximum query amount is %v", query.Amount, math.MaxInt32)
		return []rlp.RawValue{}, nil
	}

	headers, err := backend.Chain().GetHeaders(query.Origin, int(query.Amount), int(query.Skip), query.Reverse) //nolint:gosec
	switch {
	case errors.Is(err, core.ErrInvalidRequest) || errors.Is(err, core.ErrAncientHeaders):
		return nil, err
	case errors.Is(err, core.ErrFutureHeaders):
		return []rlp.RawValue{}, nil
	case err != nil:
		peer.Log().Warnf("could not retrieve all %v headers starting at %v, err: %v", int(query.Amount), query.Origin, err)
		return []rlp.RawValue{}, nil
	default:
		var res []rlp.RawValue
		for i := range headers {
			rlpData, err := rlp.EncodeToBytes(headers[i])
			if err != nil {
				return nil, err
			}
			res = append(res, rlpData)
		}

		return res, nil
	}
}

func handleGetBlockBodies(backend Backend, msg Decoder, peer *Peer) error {
	var query eth.GetBlockBodiesPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	bodies, err := answerGetBlockBodies(backend, query)
	if err != nil {
		log.Errorf("error retrieving block bodies for request ID %v, hashes %v: %v", query.RequestId, query, err)
		return err
	}
	return peer.ReplyBlockBodies(query.RequestId, bodies)
}

func answerGetBlockBodies(backend Backend, query eth.GetBlockBodiesPacket) ([]*BlockBody, error) {
	bodies, err := backend.Chain().GetBodies(query.GetBlockBodiesRequest)
	if errors.Is(err, core.ErrBodyNotFound) {
		log.Debugf("could not find all block bodies: %v", query)
		return []*BlockBody{}, nil
	} else if err != nil {
		return nil, err
	}

	blockBodies := make([]*BlockBody, 0, len(bodies))
	for _, body := range bodies {
		blockBody := &BlockBody{
			Transactions: body.Transactions,
			Uncles:       body.Uncles,
		}
		blockBodies = append(blockBodies, blockBody)
	}

	sidecars, err := backend.Chain().GetBlobSidecars(query.GetBlockBodiesRequest)
	if err == nil {
		for idx, sidecar := range sidecars {
			blockBodies[idx].Sidecars = sidecar
		}
	}

	return blockBodies, nil
}

func handleNewBlockMsgRaw(backend Backend, msg Decoder, peer *Peer) error {
	return backend.HandleRaw(peer, msg.(p2p.Msg))
}

func handleTransactions(backend Backend, msg Decoder, peer *Peer) error {
	var txs eth.TransactionsPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	return backend.Handle(peer, &txs)
}

func handlePooledTransactions(backend Backend, msg Decoder, peer *Peer) error {
	var pooledTxsResponse eth.PooledTransactionsPacket
	if err := msg.Decode(&pooledTxsResponse); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	return backend.Handle(peer, &pooledTxsResponse)
}

func handleGetPooledTransactions(backend Backend, msg Decoder, peer *Peer) error {
	// Decode the pooled transactions retrieval message
	var query eth.GetPooledTransactionsPacket
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	txs, err := backend.RequestTransactions(query.GetPooledTransactionsRequest)
	if err != nil {
		return fmt.Errorf("could not retrieve pooled transactions: %v", err)
	}

	return peer.ReplyPooledTransaction(query.RequestId, txs)
}

func handleNewPooledTransactionHashes(backend Backend, msg Decoder, peer *Peer) error {
	var txs eth.NewPooledTransactionHashesPacket
	if err := msg.Decode(&txs); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	log.Tracef("%v: received tx announcement of %v transactions", peer, len(txs.Hashes))

	return backend.Handle(peer, &txs)
}

func handleNewBlockHashes(backend Backend, msg Decoder, peer *Peer) error {
	var blockHashes NewBlockHashesPacket
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

	headers, err := blockHeaders.List.Items()
	if err != nil {
		return fmt.Errorf("BlockHeaders: %w", err)
	}

	UpdatePeerHeadFromHeaders(headers, peer)

	handled, err := peer.NotifyResponse(blockHeaders.RequestId, new(eth.BlockHeadersRequest(headers)))
	if err != nil {
		return err
	}

	if handled {
		return nil
	}

	return backend.Handle(peer, new(eth.BlockHeadersRequest(headers)))
}

func handleBlockBodies(_ Backend, msg Decoder, peer *Peer) error {
	var blockBodies BlockBodiesPacket
	if err := msg.Decode(&blockBodies); err != nil {
		return fmt.Errorf("could not decode message: %v: %v", msg, err)
	}

	_, err := peer.NotifyResponse(blockBodies.RequestID, &blockBodies)
	return err
}

// UpdatePeerHeadFromHeaders updates the peer's head based on the block headers received.
func UpdatePeerHeadFromHeaders(headers []*types.Header, peer *Peer) {
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

func updatePeerHeadFromNewHashes(newBlocks NewBlockHashesPacket, peer *Peer) {
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

func handleUnimplemented(_ Backend, _ Decoder, _ *Peer) error {
	return nil
}
