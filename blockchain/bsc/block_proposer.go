package bsc

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// BlockProposingDateFormat is the date format for block proposing.
var BlockProposingDateFormat = "2006-01-02T15:04:05.000000"

// ChainBlock is a block in the blockchain.
type ChainBlock interface {
	// Number returns the block number.
	Number() *big.Int

	// Difficulty returns the block difficulty.
	Difficulty() *big.Int

	// Time returns the block time.
	Time() uint64
}

// BlockProposer is an interface for a block proposer.
type BlockProposer interface {
	services.Runner

	// OnBlock is called when a new block is received to notify the block proposer.
	OnBlock(context.Context, ChainBlock) error

	// ShortIDs returns the short IDs for the given transaction hashes.
	ShortIDs(context.Context, *pb.ShortIDsRequest) (*pb.ShortIDsReply, error)

	// ProposedBlock accepts a proposed block request and returns a reply from the block proposer.
	ProposedBlock(context.Context, *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error)

	// ProposedBlockStats returns the proposed block stats.
	ProposedBlockStats(context.Context, *pb.ProposedBlockStatsRequest) (*pb.ProposedBlockStatsReply, error)
}

// NoopBlockProposer is a block proposer that does nothing.
type NoopBlockProposer struct {
	txStore services.TxStore
	log     *logger.Entry
}

// NewNoopBlockProposer creates a new NoopBlockProposer.
func NewNoopBlockProposer(txStore services.TxStore, log *logger.Entry) *NoopBlockProposer {
	return &NoopBlockProposer{txStore: txStore, log: log}
}

// Run runs the block proposer.
func (n *NoopBlockProposer) Run(context.Context) error { return nil }

// ShortIDs returns empty short IDs.
func (n *NoopBlockProposer) ShortIDs(ctx context.Context, req *pb.ShortIDsRequest) (*pb.ShortIDsReply, error) {
	return ShortIDs(ctx, req, n.txStore, n.log)
}

// ShortIDs returns empty short IDs.
func ShortIDs(ctx context.Context, req *pb.ShortIDsRequest, txStore services.TxStore, log *logger.Entry) (*pb.ShortIDsReply, error) {
	if txStore == nil {
		return nil, errors.New("tx store is not initialized")
	}

	result := pb.ShortIDsReply{ShortIds: make([]uint32, len(req.GetTxHashes()))}

	for i, txHashBytes := range req.GetTxHashes() {
		select {
		default:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		txHash, err := types.NewSHA256Hash(txHashBytes)
		if err != nil {
			log.Error("txHash from builder is not correct position " + strconv.FormatInt(int64(i), 10) + ", txHash " + hex.EncodeToString(txHashBytes) + " err " + err.Error())
			continue
		}

		tx, exists := txStore.Get(txHash)
		if exists && len(tx.ShortIDs()) > 0 {
			result.ShortIds[i] = uint32(tx.ShortIDs()[0])
		}
	}

	return &result, nil
}

// ProposedBlock returns an empty proposed block.
func (n *NoopBlockProposer) ProposedBlock(context.Context, *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error) {
	return new(pb.ProposedBlockReply), nil
}

// ProposedBlockStats returns empty proposed block stats.
func (n *NoopBlockProposer) ProposedBlockStats(context.Context, *pb.ProposedBlockStatsRequest) (*pb.ProposedBlockStatsReply, error) {
	return new(pb.ProposedBlockStatsReply), nil
}

// OnBlock does nothing.
func (n *NoopBlockProposer) OnBlock(context.Context, ChainBlock) error { return nil }
