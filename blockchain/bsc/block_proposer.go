package bsc

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/ctxutil"
	"github.com/bloXroute-Labs/gateway/v2/utils/ptr"
)

type sendingState uint8

const (
	sendingStateNotSent = sendingState(iota)
	sendingStateSent
	sendingStateReplacedWithHigherReward
)

const mevBlockBacklog = 100

var (
	errAlreadySent      = errors.New("block already sent")
	errLowerBlockReward = errors.New("block reward is lower than the one is in memory")

	errLowerBlockNumber  = errors.New("proposed block number is too low")
	errHigherBlockNumber = errors.New("proposed block number is too high")

	errBlockNotSent         = errors.New("no block was sent")
	errBlockReplacedWithNew = errors.New("block was replaced with higher reward")
)

var (
	inTurnDifficulty = big.NewInt(2)
)

// BlockProposer is responsible for proposing blocks to the gateway
type BlockProposer struct {
	//	-- dependencies --
	clock           utils.Clock
	txStore         *services.TxStore
	blocksToCache   int
	log             *log.Entry
	callerManager   caller.Manager
	sendingInterval time.Duration

	//	-- internal data --
	cancel context.CancelFunc

	state *atomic.Pointer[services.RunState]

	wg          *sync.WaitGroup
	wgProposing *sync.WaitGroup

	ctxSwitcher ctxutil.CtxSwitcher

	blockMx   *sync.RWMutex
	blockList []rpc.BlockNumber
	blockMap  map[rpc.BlockNumber]*proposedBlockRequest

	proposingTimer       *time.Timer
	startSendingTime     *atomic.Pointer[time.Time]
	nextStartSendingTime *atomic.Pointer[time.Time]

	blockNumberToProposeFor *atomic.Pointer[big.Int]

	blockReward *atomic.Pointer[big.Int]

	processedBlockSentState *atomic.Pointer[sendingState]

	blockCh chan services.ChainBlock
}

// NewBlockProposer creates a new block proposer
func NewBlockProposer(clock utils.Clock, txStore *services.TxStore, blocksToCache int, log *log.Entry, callerManager caller.Manager, sendingInterval time.Duration) *BlockProposer {
	b := &BlockProposer{
		clock:           clock,
		txStore:         txStore,
		blocksToCache:   blocksToCache,
		log:             log,
		callerManager:   callerManager,
		sendingInterval: sendingInterval,

		state: atomic.NewPointer(ptr.New(services.StateIdle)),

		wg:          new(sync.WaitGroup),
		wgProposing: new(sync.WaitGroup),

		blockMx: new(sync.RWMutex),
		blockCh: make(chan services.ChainBlock, mevBlockBacklog),
	}

	b.reset()

	return b
}

// Run starts the block proposer
func (b *BlockProposer) Run(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			b.log.Warnf("failed to start: %v", err)
		}
	}()

	if err = b.requireIdle(); err != nil {
		return
	}

	b.state.Store(ptr.New(services.StateBooting))

	b.reset()

	b.wg.Add(1)
	go b.runLoop(ctx)

	return
}

// ShortIDs returns short ids for the given tx hashes
func (b *BlockProposer) ShortIDs(ctx context.Context, req *pb.TxHashListRequest) (*pb.ShortIDListReply, error) {
	return services.ShortIDs(ctx, req, b.txStore, b.log)
}

// ProposedBlock proposes a block to the validator
func (b *BlockProposer) ProposedBlock(ctx context.Context, req *pb.ProposedBlockRequest) (reply *pb.ProposedBlockReply, err error) {
	msg := fmt.Sprintf("handling ProposedBlock request blockNum(%d) ID(%s) blockReward(%s) processBlocksOnGateway(%v)", req.BlockNumber, req.Id, req.BlockReward, req.ProcessBlocksOnGateway)

	defer func() {
		if err != nil {
			msg += fmt.Sprintf(" err(%v)", err)
		}

		b.log.Debugf(msg)
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	var blockReq *proposedBlockRequest

	if blockReq, err = b.prepareProposedBlockReq(ctx, req); err != nil {
		return
	}

	if !req.ProcessBlocksOnGateway {
		msg += fmt.Sprintf(" details(proxy) blockPreparingDuration(%s)", blockReq.preparingDuration)

		if reply, err = b.submitProposedBlock(ctx, blockReq); err != nil {
			return
		}

		msg += fmt.Sprintf(" validatorReply(%s) validatorReplyTime(%s)", reply.ValidatorReply, time.UnixMilli(reply.ValidatorReplyTime).Format(services.BlockProposingDateFormat))
		b.processedBlockSentState.Store(ptr.New(sendingStateReplacedWithHigherReward))

		return
	}

	reply, err = b.processProposedBlock(ctx, blockReq)
	return
}

// BlockInfo method for providing block info
func (b *BlockProposer) BlockInfo(_ context.Context, req *pb.BlockInfoRequest) (reply *pb.BlockInfoReply, err error) {
	var (
		newStartSendingTime = req.StartSendingTime.AsTime()

		msg = fmt.Sprintf("handling BlockInfo request blockNum(%d) newStartSendingTime(%s)", req.BlockNumber, newStartSendingTime.Format(services.BlockProposingDateFormat))
	)

	defer func() {
		if err != nil {
			msg += fmt.Sprintf(" err(%v)", err)
		}

		b.log.Debugf(msg)
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	// if block info is for lower block number than b.blockNumberToProposeFor return error
	// if block info is for higher block number than b.blockNumberToProposeFor and not for next one return error
	// if block info is for higher block number than b.blockNumberToProposeFor and for next one add/update to the map
	// if block info is for the same block number as b.blockNumberToProposeFor add/update to the map
	blockNumberToProposeFor := b.blockNumberToProposeFor.Load().Uint64()

	switch {
	case req.BlockNumber < blockNumberToProposeFor:
		err = errors.WithMessagef(errLowerBlockNumber, "proposed %d, current %d", req.BlockNumber, blockNumberToProposeFor)
	case blockNumberToProposeFor+1 < req.BlockNumber:
		err = errors.WithMessagef(errHigherBlockNumber, "proposed %d, current %d", req.BlockNumber, blockNumberToProposeFor)
	}

	if err != nil {
		return
	}

	if req.BlockNumber == blockNumberToProposeFor {
		startSendingTime := *b.startSendingTime.Load()

		if !newStartSendingTime.IsZero() && (startSendingTime.IsZero() || newStartSendingTime.Before(startSendingTime)) {
			b.proposingTimer.Reset(newStartSendingTime.Sub(b.clock.Now()))
			b.startSendingTime.Store(&newStartSendingTime)
		}
	} else {
		nextStartSendingTime := *b.nextStartSendingTime.Load()

		if !newStartSendingTime.IsZero() && (nextStartSendingTime.IsZero() || newStartSendingTime.Before(nextStartSendingTime)) {
			b.nextStartSendingTime.Store(&newStartSendingTime)
		}
	}

	num := rpc.BlockNumber(req.BlockNumber)

	reply = new(pb.BlockInfoReply)

	b.blockMx.Lock()
	defer b.blockMx.Unlock()

	_, exists := b.blockMap[num]
	if !exists {
		return
	}

	if len(b.blockList) == cap(b.blockList) {
		delete(b.blockMap, b.blockList[0])
		b.blockList = append(b.blockList[1:], num)
	}

	b.blockMap[num] = newProposedBlockRequest()

	return
}

// ProposedBlockStats method for getting block stats
func (b *BlockProposer) ProposedBlockStats(ctx context.Context, req *pb.ProposedBlockStatsRequest) (reply *pb.ProposedBlockStatsReply, err error) {
	msg := fmt.Sprintf("handling ProposedBlockStats request blockNum(%d)", req.BlockNumber)

	defer func() {
		if err != nil {
			msg += fmt.Sprintf(" err(%v)", err)
		}

		b.log.Debugf(msg)
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	if err = b.requireSent(); err != nil {
		return
	}

	b.blockMx.RLock()
	record := b.blockMap[rpc.BlockNumber(req.BlockNumber)]
	b.blockMx.RUnlock()

	if record == nil || !record.isSent() {
		err = fmt.Errorf("block number %d not found", req.BlockNumber)
		return
	}

	msg += fmt.Sprintf(
		" ID(%s) sendingDuration(%s) receivedTime(%s) sentTime(%s) validatorReply(%v) validatorReplyTime(%v)",
		record.id,
		record.sendingDuration,
		record.receivedTime.Format(services.BlockProposingDateFormat),
		record.sentTime.Format(services.BlockProposingDateFormat),
		record.validatorReply,
		time.UnixMilli(record.validatorReplyTime).Format(services.BlockProposingDateFormat),
	)

	// check if context is done before creating a new block
	select {
	default:
	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	reply = &pb.ProposedBlockStatsReply{
		Id:                 record.id,
		SendingDuration:    durationpb.New(record.sendingDuration),
		ReceivedTime:       timestamppb.New(record.receivedTime),
		SentTime:           timestamppb.New(record.sentTime),
		ValidatorReply:     record.validatorReply,
		ValidatorReplyTime: record.validatorReplyTime,
	}

	return
}

// OnBlock method for providing blocks
func (b *BlockProposer) OnBlock(ctx context.Context, chainBlock services.ChainBlock) (err error) {
	defer func() {
		if err != nil {
			b.log.Warnf("handling OnBlock request failed: %v", err)
		}
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	// check if context is done before creating a new block
	select {
	default:
		err = errors.New("failed to send block to block proposer")
	case <-ctx.Done():
		err = ctx.Err()
	case b.blockCh <- chainBlock:
	}

	return
}

func (b *BlockProposer) reset() {
	if b.cancel != nil {
		b.cancel()
		b.wg.Wait()
	}

	b.blockList = make([]rpc.BlockNumber, 0, b.blocksToCache)
	b.blockMap = make(map[rpc.BlockNumber]*proposedBlockRequest, b.blocksToCache)

	b.proposingTimer = time.NewTimer(0)
	<-b.proposingTimer.C // discard the initial tick

	b.startSendingTime = atomic.NewPointer(new(time.Time))
	b.nextStartSendingTime = atomic.NewPointer(new(time.Time))
	b.blockNumberToProposeFor = atomic.NewPointer(big.NewInt(-1))
	b.blockReward = atomic.NewPointer(big.NewInt(0))
	b.processedBlockSentState = atomic.NewPointer(ptr.New(sendingStateNotSent))
}

func (b *BlockProposer) runLoop(ctx context.Context) {
	ctx, b.cancel = context.WithCancel(ctx)
	b.ctxSwitcher = ctxutil.NewCtxSwitcher(ctx)

	defer func() {
		b.proposingTimer.Stop()
		b.ctxSwitcher.Cancel()
		b.cancel()
		b.state.Store(ptr.New(services.StateIdle))
		b.wg.Done()
	}()

	var (
		chainHeadBlockNumber     = big.NewInt(0)
		chainHeadBlockDifficulty = big.NewInt(0)
	)

	b.log.Infof("start service startTime(%s) blocksToCache(%d)", b.clock.Now().Format(services.BlockProposingDateFormat), b.blocksToCache)

	b.state.Store(ptr.New(services.StateRunning))

	for {
		select {
		case <-ctx.Done():
			b.close()
			b.log.Infof("stop service stopTime(%s): %v", b.clock.Now().Format(services.BlockProposingDateFormat), ctx.Err())
			return
		case blk := <-b.blockCh:
			func() {
				if blk == nil {
					return
				}

				newChainHeadBlockNumber := blk.Number()
				newChainHeadBlockDifficulty := blk.Difficulty()
				newBlockNumberToProposeFor := getBlockNumberToProposeFor(newChainHeadBlockNumber)

				switch chainHeadBlockNumber.Cmp(newChainHeadBlockNumber) {
				case 1: // chainHeadBlockNumber > newChainHeadBlockNumber
					return
				case 0: // chainHeadBlockNumber == newChainHeadBlockNumber
					if chainHeadBlockDifficulty.Cmp(inTurnDifficulty) == 0 || newChainHeadBlockDifficulty.Cmp(inTurnDifficulty) != 0 {
						return
					}
				}

				msg := fmt.Sprintf("stopBuildingForBlock(%s) startBuildingForBlock(%s)", newChainHeadBlockNumber, newBlockNumberToProposeFor)
				defer func() { b.log.Debugf(msg) }()

				b.ctxSwitcher.Switch()
				b.wgProposing.Wait()

				chainHeadBlockNumber.Set(newChainHeadBlockNumber)
				chainHeadBlockDifficulty.Set(newChainHeadBlockDifficulty)
				b.blockReward.Store(big.NewInt(0))
				b.startSendingTime.Store(new(time.Time))
				b.processedBlockSentState.Store(ptr.New(sendingStateNotSent))

				b.proposingTimer.Reset(0)
				<-b.proposingTimer.C // discard the initial tick

				blockNumberToProposeFor := b.blockNumberToProposeFor.Load()
				b.blockNumberToProposeFor.Store(newBlockNumberToProposeFor)

				if blockNumberToProposeFor.Int64()+1 == newBlockNumberToProposeFor.Int64() {
					nextStartSendingTime := b.nextStartSendingTime.Load()

					if !nextStartSendingTime.IsZero() {
						b.proposingTimer.Reset(nextStartSendingTime.Sub(b.clock.Now()))
						b.startSendingTime.Store(nextStartSendingTime)
						b.nextStartSendingTime.Store(new(time.Time))
					}
				}
			}()
		case <-b.proposingTimer.C:
			blockNumber := b.blockNumberToProposeFor.Load()
			if blockNumber == nil {
				b.log.Warnf("failed to get block number to propose for")
				continue
			}

			b.proposingTimer.Reset(b.sendingInterval)

			blockNumberToProposeFor := blockNumber.Int64()
			blockNum := rpc.BlockNumber(blockNumberToProposeFor)

			b.blockMx.RLock()
			proposedBlock := b.blockMap[blockNum]
			b.blockMx.RUnlock()

			if proposedBlock == nil {
				b.log.Debugf(errAlreadySent.Error())
				continue
			}

			b.wgProposing.Add(1)
			go func(ctx context.Context, blockNumberToProposeFor int64, proposedBlock *proposedBlockRequest) { //nolint:contextcheck
				defer b.wgProposing.Done()

				if err := func() error {
					msg := fmt.Sprintf("proposing blockNum(%d) ID(%s) blockReward(%s) blockPreparingDuration(%s)", blockNumberToProposeFor, proposedBlock.id, proposedBlock.args.BlockReward, proposedBlock.preparingDuration)
					defer func() { b.log.Debugf(msg) }()

					reply, err := b.submitProposedBlock(ctx, proposedBlock)
					switch {
					case err == nil:
					case errors.Is(err, errAlreadySent):
						return nil
					default:
						return err
					}

					msg += fmt.Sprintf(" submitted(true) validatorReply(%s) validatorReplyTime(%s)", reply.ValidatorReply, time.UnixMilli(reply.ValidatorReplyTime).Format(services.BlockProposingDateFormat))
					b.processedBlockSentState.Store(ptr.New(sendingStateSent))

					return nil
				}(); err != nil {
					b.log.Errorf("failed submitting proposed blockNum(%d): %v", blockNum, err)
				}
			}(b.ctxSwitcher.Context(), blockNumberToProposeFor, proposedBlock)
		}
	}
}

func (b *BlockProposer) close() {
	b.ctxSwitcher.Cancel()
	b.wgProposing.Wait()
	b.callerManager.Close()
}

func (b *BlockProposer) prepareProposedBlockReq(ctx context.Context, req *pb.ProposedBlockRequest) (*proposedBlockRequest, error) {
	receivedTime := b.clock.Now()

	blockNumber := b.blockNumberToProposeFor.Load()
	if blockNumber == nil {
		return nil, errors.New("failed to get block number to propose for")
	}

	blockNumberToProposeFor := blockNumber.Uint64()

	switch {
	case req.BlockNumber < blockNumberToProposeFor:
		return nil, errors.WithMessagef(errLowerBlockNumber, "proposed %d, current %d", req.BlockNumber, blockNumberToProposeFor)
	case req.BlockNumber > blockNumberToProposeFor:
		return nil, errors.WithMessagef(errHigherBlockNumber, "proposed %d, current %d", req.BlockNumber, blockNumberToProposeFor)
	}

	blockReward, ok := new(big.Int).SetString(req.BlockReward, 10)
	if !ok || blockReward == nil {
		return nil, errors.New("failed converting blockReward")
	}

	blockRewardPtr := b.blockReward.Load()
	if blockReward.Cmp(blockRewardPtr) != 1 {
		return nil, errors.WithMessagef(errLowerBlockReward, "blockReward(%s) is lower than the one is in memory(%s)", req.BlockReward, blockRewardPtr)
	}

	b.blockReward.Store(blockReward)

	if b.txStore == nil || *b.txStore == nil {
		return nil, errors.New("tx store is not initialized")
	}

	txStore := *b.txStore
	payload := req.GetPayload()

	unReverted := req.GetUnRevertedHashes()
	unRevertedHashes := make([]common.Hash, len(unReverted))

	for _, hash := range unReverted {
		unRevertedHashes = append(unRevertedHashes, common.BytesToHash(hash))
	}

	proposedBlockReq := proposedBlockRequest{
		sent: atomic.NewBool(false),

		id:           req.Id,
		addr:         req.ValidatorHttpAddress,
		receivedTime: receivedTime,
		method:       fmt.Sprintf("%s_proposedBlock", req.Namespace),
		args: proposedBlockArgs{
			MEVRelay:         "bloXroute",
			BlockNumber:      rpc.BlockNumber(req.BlockNumber),
			PrevBlockHash:    common.HexToHash(req.PrevBlockHash),
			BlockReward:      blockReward,
			GasLimit:         req.GasLimit,
			GasUsed:          req.GasUsed,
			Payload:          make([]hexutil.Bytes, len(payload)),
			UnRevertedHashes: unRevertedHashes,
		},
	}

	var (
		txStoreTx *types.BxTransaction
		err       error
	)

	for i, proposedBlockTx := range payload {
		select { // check if context is done
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if proposedBlockTx.GetShortId() == 0 {
			proposedBlockReq.args.Payload[i] = proposedBlockTx.GetRawData()
			continue
		}

		if txStoreTx, err = txStore.GetTxByShortID(types.ShortID(proposedBlockTx.GetShortId())); err != nil {
			return nil, errors.WithMessage(err, "failed decompressing tx")
		}

		var blockchainTx = new(ethtypes.Transaction)

		if err = blockchainTx.UnmarshalBinary(txStoreTx.Content()); err == nil {
			proposedBlockReq.args.Payload[i] = hexutil.Bytes(txStoreTx.Content())
			continue

		}

		if blockchainTx, err = eth.TransactionBDNToBlockchain(txStoreTx); err != nil {
			return nil, errors.WithMessage(err, "failed converting tx")
		}

		if proposedBlockReq.args.Payload[i], err = blockchainTx.MarshalBinary(); err != nil {
			return nil, errors.WithMessage(err, "failed serializing tx")
		}
	}

	proposedBlockReq.preparingDuration = b.clock.Now().Sub(receivedTime)

	return &proposedBlockReq, nil
}

func (b *BlockProposer) submitProposedBlock(ctx context.Context, req *proposedBlockRequest) (reply *pb.ProposedBlockReply, err error) {
	if err = req.submit(ctx, b.clock, b.callerManager); err != nil {
		b.log.Errorf(err.Error())
		return
	}

	reply = &pb.ProposedBlockReply{ValidatorReply: req.validatorReply, ValidatorReplyTime: req.validatorReplyTime}
	return
}

func (b *BlockProposer) processProposedBlock(ctx context.Context, req *proposedBlockRequest) (*pb.ProposedBlockReply, error) {
	var (
		resp = "block updated in memory"
		err  error
	)

	b.blockMx.Lock() // lock for writing

	data := b.blockMap[req.args.BlockNumber]

	if data == nil {
		resp = "block added to memory"
		data = newProposedBlockRequest()
		b.blockMap[req.args.BlockNumber] = data
	}

	err = data.update(req)

	b.blockMx.Unlock() // unlock for writing

	if err != nil {
		return nil, err
	}

	if _, err = b.callerManager.AddClient(ctx, req.addr); err != nil { // init client if not exists
		return nil, err
	}

	return &pb.ProposedBlockReply{ValidatorReply: resp, ValidatorReplyTime: b.clock.Now().UnixMilli()}, nil
}

func (b *BlockProposer) requireIdle() error {
	return services.RequireState(b.state.Load(), services.StateIdle)
}

func (b *BlockProposer) requireRunning() error {
	return services.RequireState(b.state.Load(), services.StateRunning)
}

func (b *BlockProposer) requireSent() error {
	switch *b.processedBlockSentState.Load() {
	case sendingStateNotSent:
		return errBlockNotSent
	case sendingStateReplacedWithHigherReward:
		return errBlockReplacedWithNew
	default:
		return nil
	}
}

func getBlockNumberToProposeFor(number *big.Int) *big.Int {
	return new(big.Int).Add(number, big.NewInt(1))
}
