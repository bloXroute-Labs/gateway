package bsc

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller/mock"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/ptr"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockBlock struct {
	number     *big.Int
	difficulty *big.Int
}

func (m *mockBlock) Number() *big.Int     { return m.number }
func (m *mockBlock) Difficulty() *big.Int { return m.difficulty }

func TestBlockProposer(t *testing.T) {
	t.Skip("local test only, rot ready for CI")
	log.SetLevel(log.DebugLevel)

	mc := new(utils.MockClock)
	mc.SetTime(time.Now().UTC())

	blocksToCache := 3
	entry := log.TestEntry()
	addreses := []string{"http://127.0.0.1:8080"}
	callerManage := mock.NewManager(mock.DefaultHasher, map[string]map[string][]byte{
		addreses[0]: {},
	})
	var txStore services.TxStore = ptr.New(services.NewBxTxStore(time.Minute, time.Minute, time.Minute, services.NewEmptyShortIDAssigner(), services.NewHashHistory("seenTxs", time.Minute), nil, 30*time.Minute, services.NoOpBloomFilter{}))

	bp := NewBlockProposer(mc, &txStore, blocksToCache, entry, callerManage, 50)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, bp.Run(ctx))

	for *bp.state.Load() != services.StateRunning {
		time.Sleep(10 * time.Millisecond)
	}

	block := &mockBlock{number: big.NewInt(1), difficulty: inTurnDifficulty}

	blkCtx := bp.ctxSwitcher.Context()
	blkCtxTimeout, blkCtxCancel := context.WithTimeout(blkCtx, 10*time.Second)
	defer blkCtxCancel()

	require.NoError(t, bp.OnBlock(ctx, block))

	select {
	case <-blkCtx.Done():
	case <-blkCtxTimeout.Done():
		select {
		case <-blkCtx.Done():
		default: // context is not done, but we timed out
			t.Fatal("timed out waiting for block to be proposed")
		}
	}

	blockNumberToProposeFor := bp.blockNumberToProposeFor.Load()

	proposedBlockReq := &pb.ProposedBlockRequest{
		BlockNumber:            blockNumberToProposeFor.Uint64(),
		BlockReward:            "70000000000000000",
		ValidatorHttpAddress:   addreses[0],
		ProcessBlocksOnGateway: true,
	}
	repl, err := bp.ProposedBlock(bp.ctxSwitcher.Context(), proposedBlockReq)
	require.NoError(t, err)
	require.NotNil(t, repl)

	require.Equal(t, block.Number().Int64()+1, blockNumberToProposeFor.Int64())
	require.True(t, bp.startSendingTime.Load().IsZero())

	_, err = bp.BlockInfo(bp.ctxSwitcher.Context(), &pb.BlockInfoRequest{BlockNumber: 3, StartSendingTime: timestamppb.New(time.Now().Add(2 * time.Second))})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)
	require.NoError(t, bp.OnBlock(ctx, &mockBlock{number: big.NewInt(2), difficulty: inTurnDifficulty}))
	require.NoError(t, bp.OnBlock(ctx, &mockBlock{number: big.NewInt(3), difficulty: inTurnDifficulty}))

	startSendingTime := mc.Now().Add(2 * time.Second)
	blockInfoReq := &pb.BlockInfoRequest{BlockNumber: blockNumberToProposeFor.Uint64(), StartSendingTime: timestamppb.New(startSendingTime)}

	_, err = bp.BlockInfo(bp.ctxSwitcher.Context(), blockInfoReq)
	require.NoError(t, err)
	require.Equal(t, &startSendingTime, bp.startSendingTime.Load())

	t.Log(
		"startSendingTime", bp.startSendingTime.Load().Format(services.BlockProposingDateFormat),
		"now", mc.Now().Format(services.BlockProposingDateFormat),
	)

	t.Log(repl)

	time.Sleep(5 * time.Second)
}
