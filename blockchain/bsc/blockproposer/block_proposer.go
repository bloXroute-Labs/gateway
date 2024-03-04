package blockproposer

import (
	"context"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/atomic"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/ctxutil"
	"github.com/bloXroute-Labs/gateway/v2/utils/ptr"
)

type sendingState uint8

const (
	sendingStateNotSent = sendingState(iota)
	sendingStateSent
	sendingStateReplacedWithHigherReward
)

const mevRelayID = "bloXroute-GW"

const mevBlockBacklog = 100

type loadType string

const (
	regularLoadType loadType = "REGULAR"
	highLoadType    loadType = "HIGHLOAD"
)

var (
	errAlreadySent      = errors.New("block already sent")
	errLowerBlockReward = errors.New("block reward is lower than the one is in memory")
	errBlockNotFound    = errors.New("block number not found")

	errLowerBlockNumber  = errors.New("proposed block number is too low")
	errHigherBlockNumber = errors.New("proposed block number is too high")

	errBlockNotSent         = errors.New("no block was sent")
	errBlockReplacedWithNew = errors.New("block was replaced with higher reward")

	errSendBlockToBlockProposer = errors.New("failed to send block to block proposer")
)

var (
	inTurnDifficulty = big.NewInt(2)
)

// BlockProposer is responsible for proposing blocks to the gateway
type BlockProposer struct {
	cfg *Config

	state *atomic.Pointer[services.RunState]

	wg          *sync.WaitGroup
	wgProposing *sync.WaitGroup

	blockCh chan bsc.ChainBlock

	blockMap ProposedBlockStore

	cancel      context.CancelFunc
	ctxSwitcher ctxutil.CtxSwitcher

	blockNumberToProposeFor *atomic.Pointer[big.Int]

	blockReward *atomic.Pointer[big.Int]
}

// New creates a new block proposer
func New(cfg *Config) *BlockProposer {
	b := &BlockProposer{
		cfg: cfg,

		state: atomic.NewPointer(ptr.New(services.StateIdle)),

		wg:          new(sync.WaitGroup),
		wgProposing: new(sync.WaitGroup),

		blockCh: make(chan bsc.ChainBlock, mevBlockBacklog),
	}

	b.reset()

	return b
}

// requireIdle returns an error if the block proposer is not in the idle state
func (b *BlockProposer) requireIdle() error {
	return services.RequireState(b.state.Load(), services.StateIdle)
}

// requireRunning returns an error if the block proposer is not in the running state
func (b *BlockProposer) requireRunning() error {
	return services.RequireState(b.state.Load(), services.StateRunning)
}

// reset resets the block proposer
func (b *BlockProposer) reset() {
	if b.cancel != nil {
		b.cancel()
		b.wg.Wait()
	}

	b.blockMap = NewProposedBlockMap(b.cfg.BlocksToCache, b.cfg.SendingInfo.HighLoadTxNumThreshold)

	b.blockNumberToProposeFor = atomic.NewPointer(big.NewInt(-1))
	b.blockReward = atomic.NewPointer(big.NewInt(0))
}

// runLoop runs the block proposer loop
func (b *BlockProposer) runLoop(ctx context.Context) {
	// reassign the context to the local context with cancel
	ctx, b.cancel = context.WithCancel(ctx)

	// set the context switcher that manages the context of the current block inside the loop
	// can be switched to the context of the next block
	// or canceled from the outside
	b.ctxSwitcher = ctxutil.NewCtxSwitcher(ctx)

	// define defers to be executed when the loop exits
	defer func() {
		b.ctxSwitcher.Cancel()
		b.cancel()
		b.state.Store(ptr.New(services.StateIdle))
		b.wg.Done()
	}()

	var (
		chainHeadBlockNumber     = big.NewInt(0)
		chainHeadBlockDifficulty = big.NewInt(0)
	)

	{ // start the ticker handlers
		go b.runTicker(ctx, b.cfg.RegularTicker, regularLoadType)
		go b.runTicker(ctx, b.cfg.HighLoadTicker, highLoadType)
	}

	b.cfg.Log.Info("start service startTime(" + b.cfg.Clock.Now().Format(bsc.BlockProposingDateFormat) + ") blocksToCache(" + strconv.FormatInt(b.cfg.BlocksToCache, 10) + ")")

	b.state.Store(ptr.New(services.StateRunning))

	// block listener loop
	for {
		select {
		case <-ctx.Done():
			b.close()
			b.cfg.Log.Info("stop service stopTime(" + b.cfg.Clock.Now().Format(bsc.BlockProposingDateFormat) + "): " + ctx.Err().Error())
			return
		case blk := <-b.blockCh:
			func() {
				if blk == nil {
					return
				}

				newChainHeadBlockNumber := blk.Number()
				newChainHeadBlockDifficulty := blk.Difficulty()
				newBlockNumberToProposeFor := getBlockNumberToProposeFor(newChainHeadBlockNumber)
				newBlockTime := timeFromTimestamp(blk.Time())

				switch chainHeadBlockNumber.Cmp(newChainHeadBlockNumber) {
				case 1: // chainHeadBlockNumber > newChainHeadBlockNumber
					return
				case 0: // chainHeadBlockNumber == newChainHeadBlockNumber
					if chainHeadBlockDifficulty.Cmp(inTurnDifficulty) == 0 || newChainHeadBlockDifficulty.Cmp(inTurnDifficulty) != 0 {
						return
					}
				}

				msg := "stopProposingForBlock(" + newChainHeadBlockNumber.String() + ") startProposingForBlock(" + newBlockNumberToProposeFor.String() + ")"
				defer func() { b.cfg.Log.Debug(msg) }()

				// TODO: (mk) add block number request to the validator to establish connection with keep-alive

				b.ctxSwitcher.Switch()
				b.wgProposing.Wait()

				b.blockMap.Shift(rpc.BlockNumber(newBlockNumberToProposeFor.Uint64()))
				chainHeadBlockNumber.Set(newChainHeadBlockNumber)
				chainHeadBlockDifficulty.Set(newChainHeadBlockDifficulty)

				b.blockReward.Store(big.NewInt(0))

				b.cfg.RegularTicker.Reset(newBlockTime)
				b.cfg.HighLoadTicker.Reset(newBlockTime)

				b.blockNumberToProposeFor.Store(newBlockNumberToProposeFor)
			}()
		}
	}
}

// runTicker runs the ticker handler
func (b *BlockProposer) runTicker(ctx context.Context, ticker bsc.Ticker, load loadType) {
	go func() {
		if err := ticker.Run(ctx); err != nil {
			b.cfg.Log.Error("failed running " + string(load) + " ticker: " + err.Error())
		}
	}()

	b.cfg.Log.Info("start " + string(load) + " ticker handler startTime(" + b.cfg.Clock.Now().Format(bsc.BlockProposingDateFormat) + ")")

	for {
		select {
		case <-ctx.Done():
			b.cfg.Log.Info("stop " + string(load) + " ticker handler stopTime(" + b.cfg.Clock.Now().Format(bsc.BlockProposingDateFormat) + "): " + ctx.Err().Error())
			return
		case <-ticker.Alert():
			blockNumber := b.blockNumberToProposeFor.Load()
			if blockNumber == nil {
				b.cfg.Log.Warn(string(load) + " failed to get block number to propose for")
				continue
			}

			blockNum := rpc.BlockNumber(blockNumber.Int64())

			// checks inside of the loop so to not fire goroutines no submitter is available
			submitter := b.blockMap.SubmitBlock(blockNum, load == highLoadType)
			if submitter == nil {
				continue
			}

			b.wgProposing.Add(1)
			go func(submitter Submitter, blockNum rpc.BlockNumber, load loadType) {
				defer b.wgProposing.Done()

				msg, err := submitter(b.ctxSwitcher.Context(), b.cfg.Clock, b.cfg.CallerManager)
				if err != nil {
					b.cfg.Log.Error(string(load) + " failed submitting " + msg + ": " + err.Error())
					return
				}

				b.blockMap.SetState(blockNum, sendingStateSent)
				b.cfg.Log.Debug(string(load) + " " + msg)
			}(submitter, blockNum, load)
		}
	}
}

// close closes the block proposer
func (b *BlockProposer) close() {
	b.ctxSwitcher.Cancel()
	b.wgProposing.Wait()
	b.cfg.CallerManager.Close()
}

// proposedBlockRequest is a request to propose a block
func (b *BlockProposer) prepareProposedBlockReq(ctx context.Context, req *pb.ProposedBlockRequest) (*proposedBlockRequest, error) {
	receivedTime := b.cfg.Clock.Now()

	blockNumber := b.blockNumberToProposeFor.Load()
	if blockNumber == nil {
		return nil, errors.New("failed to get block number to propose for")
	}

	blockNumberToProposeFor := blockNumber.Uint64()

	switch {
	case req.BlockNumber < blockNumberToProposeFor:
		return nil, errors.WithMessage(errLowerBlockNumber, "proposed "+strconv.FormatUint(req.BlockNumber, 10)+", current "+strconv.FormatUint(blockNumberToProposeFor, 10))
	case req.BlockNumber > blockNumberToProposeFor:
		return nil, errors.WithMessage(errHigherBlockNumber, "proposed "+strconv.FormatUint(req.BlockNumber, 10)+", current "+strconv.FormatUint(blockNumberToProposeFor, 10))
	}

	blockReward, ok := new(big.Int).SetString(req.BlockReward, 10)
	if !ok || blockReward == nil {
		return nil, errors.New("failed converting blockReward")
	}

	blockRewardPtr := b.blockReward.Load()
	if blockReward.Cmp(blockRewardPtr) != 1 {
		return nil, errors.WithMessage(errLowerBlockReward, "blockReward("+req.BlockReward+") is lower than the one is in memory("+blockRewardPtr.String()+")")
	}

	b.blockReward.Store(blockReward)

	if b.cfg.TxStore == nil || *b.cfg.TxStore == nil {
		return nil, errors.New("tx store is not initialized")
	}

	txStore := *b.cfg.TxStore

	unReverted := req.GetUnRevertedHashes()
	unRevertedHashes := make([]common.Hash, len(unReverted))

	for _, hash := range unReverted {
		unRevertedHashes = append(unRevertedHashes, common.BytesToHash(hash))
	}

	payload := req.GetPayload()

	proposedBlockReq := newProposedBlockRequest()
	proposedBlockReq.id = req.Id
	proposedBlockReq.addr = req.ValidatorHttpAddress
	proposedBlockReq.receivedTime = receivedTime
	proposedBlockReq.method = req.Namespace + "_proposedBlock"
	proposedBlockReq.args = proposedBlockArgs{
		MEVRelay:         mevRelayID,
		BlockNumber:      rpc.BlockNumber(req.BlockNumber),
		PrevBlockHash:    common.HexToHash(req.PrevBlockHash),
		BlockReward:      blockReward,
		GasLimit:         req.GasLimit,
		GasUsed:          req.GasUsed,
		Payload:          make([]hexutil.Bytes, len(payload)),
		UnRevertedHashes: unRevertedHashes,
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

	proposedBlockReq.preparingDuration = b.cfg.Clock.Now().Sub(receivedTime)

	return proposedBlockReq, nil
}

// submitProposedBlock submits the proposed block to the validator
func (b *BlockProposer) submitProposedBlock(ctx context.Context, req *proposedBlockRequest) (reply *pb.ProposedBlockReply, err error) {
	if err = req.submit(ctx, b.cfg.Clock, b.cfg.CallerManager); err != nil {
		b.cfg.Log.Error(err.Error())
		return
	}

	reply = &pb.ProposedBlockReply{ValidatorReply: req.validatorReply, ValidatorReplyTime: req.validatorReplyTime}
	return
}

// processProposedBlock processes the proposed block
func (b *BlockProposer) processProposedBlock(ctx context.Context, req *proposedBlockRequest) (*pb.ProposedBlockReply, error) {
	var (
		resp    = "block updated in memory"
		err     error
		created bool
	)

	created, err = b.blockMap.CreateOrUpdate(req.args.BlockNumber, req)

	switch {
	case err != nil:
		return nil, err
	case created:
		resp = "block added to memory"
	}

	if _, err = b.cfg.CallerManager.AddClient(ctx, req.addr); err != nil { // init client if not exists
		return nil, err
	}

	return &pb.ProposedBlockReply{ValidatorReply: resp, ValidatorReplyTime: b.cfg.Clock.Now().UnixMilli()}, nil
}

// Run starts the block proposer
// that proposes blocks to the validator
// provides short IDs for transactions to the builder
// and collects statistics about the proposed blocks
func (b *BlockProposer) Run(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			b.cfg.Log.Warn("failed to start: " + err.Error())
		}
	}()

	if err = b.requireIdle(); err != nil {
		return
	}

	if err = b.cfg.Validate(); err != nil {
		return
	}

	b.state.Store(ptr.New(services.StateBooting))

	b.reset()

	b.wg.Add(1)
	go func(ctx context.Context) { defer b.wg.Done(); b.runLoop(ctx) }(ctx)

	return
}

// OnBlock receives a block from the blockchain or BDN
// and sends it into the block proposer for further processing
func (b *BlockProposer) OnBlock(ctx context.Context, chainBlock bsc.ChainBlock) (err error) {
	defer func() {
		if err != nil {
			b.cfg.Log.Warn("handling OnBlock request failed: " + err.Error())
		}
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	// check if context is done before creating a new block
	select {
	default:
		err = errSendBlockToBlockProposer
	case <-ctx.Done():
		err = ctx.Err()
	case b.blockCh <- chainBlock:
	}

	return
}

// ShortIDs returns short IDs for the given transaction hashes that were sent from the builder
// so builder can use them to build a compressed block and submit it to the gateway
func (b *BlockProposer) ShortIDs(ctx context.Context, req *pb.ShortIDsRequest) (reply *pb.ShortIDsReply, err error) {
	msg := "handling ShortIDs request txHashesLen(" + strconv.FormatInt(int64(len(req.TxHashes)), 10) + ")"

	defer func() {
		if err != nil {
			msg += " err(" + err.Error() + ")"
		}

		b.cfg.Log.Debug(msg)
	}()

	reply, err = bsc.ShortIDs(ctx, req, b.cfg.TxStore, b.cfg.Log)

	return
}

// ProposedBlock receives a compressed block from the builder and sends it to the validator
// either immediately or after a processing comparison with the block that was previously sent
// and collects statistics about the proposed blocks
func (b *BlockProposer) ProposedBlock(ctx context.Context, req *pb.ProposedBlockRequest) (reply *pb.ProposedBlockReply, err error) {
	msg := "handling ProposedBlock request blockNum(" + strconv.FormatUint(req.BlockNumber, 10) + ") ID(" + req.Id + ") blockReward(" + req.BlockReward + ") processBlocksOnGateway(" + strconv.FormatBool(req.ProcessBlocksOnGateway) + ")"

	defer func() {
		if err != nil {
			msg += " err(" + err.Error() + ")"
		}

		b.cfg.Log.Debug(msg)
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	var blockReq *proposedBlockRequest

	if blockReq, err = b.prepareProposedBlockReq(ctx, req); err != nil {
		return
	}

	if !req.ProcessBlocksOnGateway {
		msg += " details(proxy) blockPreparingDuration(" + blockReq.preparingDuration.String() + ")"

		if reply, err = b.submitProposedBlock(ctx, blockReq); err != nil {
			return
		}

		msg += " validatorReply(" + reply.ValidatorReply + ") validatorReplyTime(" + time.UnixMilli(reply.ValidatorReplyTime).Format(bsc.BlockProposingDateFormat) + ")"
		b.blockMap.SetState(blockReq.args.BlockNumber, sendingStateReplacedWithHigherReward)

		return
	}

	reply, err = b.processProposedBlock(ctx, blockReq)

	return
}

// ProposedBlockStats returns statistics about the proposed blocks
func (b *BlockProposer) ProposedBlockStats(ctx context.Context, req *pb.ProposedBlockStatsRequest) (reply *pb.ProposedBlockStatsReply, err error) {
	msg := "handling ProposedBlockStats request blockNum(" + strconv.FormatUint(req.BlockNumber, 10) + ")"

	defer func() {
		if err != nil {
			msg += " err(" + err.Error() + ")"
		}

		b.cfg.Log.Debug(msg)
	}()

	if err = b.requireRunning(); err != nil {
		return
	}

	records := b.blockMap.Get(rpc.BlockNumber(req.BlockNumber))
	if records == nil {
		err = errors.WithMessage(errBlockNotFound, "block number "+strconv.FormatUint(req.BlockNumber, 10))
		return
	}

	if err = records.requireSent(); err != nil {
		return
	}

	replyRecords := make([]*pb.ProposedBlockStatsRecord, 0, records.len())

	for i, record := range records.get() { // filter out records that were not sent or are nil
		if record == nil || !record.isSent() {
			continue
		}

		replyRecords = append(replyRecords, &pb.ProposedBlockStatsRecord{
			Id:                 record.id,
			SendingDuration:    durationpb.New(record.sendingDuration),
			ReceivedTime:       timestamppb.New(record.receivedTime),
			SentTime:           timestamppb.New(record.sentTime),
			ValidatorReply:     record.validatorReply,
			ValidatorReplyTime: record.validatorReplyTime,
		})

		msg += " record-" + strconv.FormatInt(int64(i), 10) + "(" + record.String() + ")"
	}

	if len(replyRecords) == 0 {
		err = errors.WithMessage(errBlockNotFound, "block number "+strconv.FormatUint(req.BlockNumber, 10))
		return
	}

	// check if context is done before creating a new block
	select {
	default:
	case <-ctx.Done():
		err = ctx.Err()
		return
	}

	reply = &pb.ProposedBlockStatsReply{Records: replyRecords}

	return
}
