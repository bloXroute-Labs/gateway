package services

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// BlockProposingDateFormat is the date format for block proposing.
var BlockProposingDateFormat = "2006-01-02T15:04:05.000000"

// ChainBlock is a block in the blockchain.
type ChainBlock interface {
	Number() *big.Int
	Difficulty() *big.Int
}

// BlockProposer is an interface for a block proposer.
type BlockProposer interface {
	Runner

	ShortIDs(context.Context, *pb.TxHashListRequest) (*pb.ShortIDListReply, error)
	ProposedBlock(context.Context, *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error)
	BlockInfo(context.Context, *pb.BlockInfoRequest) (*pb.BlockInfoReply, error)
	ProposedBlockStats(context.Context, *pb.ProposedBlockStatsRequest) (*pb.ProposedBlockStatsReply, error)
	OnBlock(context.Context, ChainBlock) error
}

// NoopBlockProposer is a block proposer that does nothing.
type NoopBlockProposer struct {
	txStore *TxStore
	log     *log.Entry
}

// NewNoopBlockProposer creates a new NoopBlockProposer.
func NewNoopBlockProposer(txStore *TxStore, log *log.Entry) *NoopBlockProposer {
	return &NoopBlockProposer{txStore: txStore, log: log}
}

// Run runs the block proposer.
func (n *NoopBlockProposer) Run(context.Context) error { return nil }

// ShortIDs returns empty short IDs.
func (n *NoopBlockProposer) ShortIDs(ctx context.Context, req *pb.TxHashListRequest) (*pb.ShortIDListReply, error) {
	return ShortIDs(ctx, req, n.txStore, n.log)
}

// ShortIDs returns empty short IDs.
func ShortIDs(ctx context.Context, req *pb.TxHashListRequest, txStorePtr *TxStore, log *log.Entry) (*pb.ShortIDListReply, error) {
	if txStorePtr == nil || *txStorePtr == nil {
		return nil, errors.New("tx store is not initialized")
	}

	txStore := *txStorePtr

	result := pb.ShortIDListReply{ShortIDs: make([]uint32, len(req.GetTxHashes()))}

	for i, txHashBytes := range req.GetTxHashes() {
		select {
		default:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		txHash, err := types.NewSHA256Hash(txHashBytes)
		if err != nil {
			log.Errorf("txHash from builder is not correct position %v, txHash %v err %v", i, hex.EncodeToString(txHashBytes), err)
			continue
		}

		tx, exists := txStore.Get(txHash)
		if exists && len(tx.ShortIDs()) > 0 {
			result.ShortIDs[i] = uint32(tx.ShortIDs()[0])
		}
	}

	return &result, nil
}

// ProposedBlock returns an empty proposed block.
func (n *NoopBlockProposer) ProposedBlock(context.Context, *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error) {
	return new(pb.ProposedBlockReply), nil
}

// BlockInfo returns an empty block info.
func (n *NoopBlockProposer) BlockInfo(context.Context, *pb.BlockInfoRequest) (*pb.BlockInfoReply, error) {
	return new(pb.BlockInfoReply), nil
}

// ProposedBlockStats returns empty proposed block stats.
func (n *NoopBlockProposer) ProposedBlockStats(context.Context, *pb.ProposedBlockStatsRequest) (*pb.ProposedBlockStatsReply, error) {
	return new(pb.ProposedBlockStatsReply), nil
}

// OnBlock does nothing.
func (n *NoopBlockProposer) OnBlock(context.Context, ChainBlock) error { return nil }
