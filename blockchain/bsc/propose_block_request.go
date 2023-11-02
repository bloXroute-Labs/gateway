package bsc

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

type proposedBlockArgs struct {
	MEVRelay         string          `json:"mevRelay,omitempty"`
	BlockNumber      rpc.BlockNumber `json:"blockNumber"`
	PrevBlockHash    common.Hash     `json:"prevBlockHash"`
	BlockReward      *big.Int        `json:"blockReward"`
	GasLimit         uint64          `json:"gasLimit"`
	GasUsed          uint64          `json:"gasUsed"`
	Payload          []hexutil.Bytes `json:"payload"`
	UnRevertedHashes []common.Hash   `json:"unRevertedHashes,omitempty"`
}

type proposedBlockRequest struct {
	sent *atomic.Bool

	args               proposedBlockArgs
	sentTime           time.Time
	receivedTime       time.Time
	preparingDuration  time.Duration
	sendingDuration    time.Duration
	validatorReplyTime int64
	validatorReply     string
	id                 string
	addr               string
	method             string
}

func newProposedBlockRequest() *proposedBlockRequest {
	return &proposedBlockRequest{sent: atomic.NewBool(false)}
}

func (r *proposedBlockRequest) submit(ctx context.Context, clock utils.Clock, callerManager caller.Manager) error {
	if r.isSent() {
		return errAlreadySent
	}

	// setting to is sent to prevent updating the block in memory
	r.setIsSent()

	client, err := callerManager.GetClient(ctx, r.addr)
	if err != nil {
		return errors.WithMessagef(err, "error getting client for %s", r.addr)
	}

	var result any

	r.sentTime = clock.Now()
	err = client.CallContext(ctx, &result, r.method, r.args)
	r.sendingDuration = clock.Now().Sub(r.sentTime)

	if err != nil {
		return errors.WithMessage(err, "error calling validator")
	}

	var body []byte

	if body, err = json.Marshal(result); err != nil {
		return errors.WithMessagef(err, "failed serializing result, message %+v", result)
	}

	r.validatorReply = string(body)
	r.validatorReplyTime = clock.Now().UnixMilli()

	return nil
}

func (r *proposedBlockRequest) update(req *proposedBlockRequest) error {
	switch {
	case r.isSent():
		r.clearSendInfo()
	case req.cmp(r) != 1:
		return errors.WithMessagef(errLowerBlockReward, "blockReward(%s) is lower than the one is in memory(%s)", req.args.BlockReward, r.args.BlockReward)
	}

	r.args = req.args
	r.receivedTime = req.receivedTime
	r.id = req.id
	r.addr = req.addr
	r.method = req.method
	r.preparingDuration = req.preparingDuration

	return nil
}

func (r *proposedBlockRequest) clearSendInfo() {
	r.sentTime = time.Time{}
	r.sendingDuration = 0
	r.validatorReply = ""
	r.validatorReplyTime = 0

	r.sent.Store(false)
}

func (r *proposedBlockRequest) isSent() bool {
	return r.sent.Load()
}

func (r *proposedBlockRequest) setIsSent() {
	r.sent.Store(true)
}

// cmp compares r and req and returns:
//
//	-1 if r <  req
//	 0 if r == req
//	+1 if r >  req
func (r *proposedBlockRequest) cmp(req *proposedBlockRequest) int {
	var reqBR *big.Int

	if req != nil && req.args.BlockReward != nil {
		reqBR = req.args.BlockReward
	}

	return r.compareBlockReward(reqBR)
}

// compareBlockReward compares r.args.BlockReward and blockReward and returns:
//
//	-1 if r.args.BlockReward <  blockReward
//	 0 if r.args.BlockReward == blockReward
//	+1 if r.args.BlockReward >  blockReward
func (r *proposedBlockRequest) compareBlockReward(blockReward *big.Int) int {
	var requestBR *big.Int

	if r.args.BlockReward != nil {
		requestBR = r.args.BlockReward
	}

	return compareBlockReward(requestBR, blockReward)
}

// compareBlockReward compares x and y and returns:
//
//	-1 if x <  y
//	 0 if x == y
//	+1 if x >  y
func compareBlockReward(x, y *big.Int) int {
	switch {
	case x == nil && y == nil:
		return 0
	case x == nil:
		return -1
	case y == nil:
		return 1
	default:
		return x.Cmp(y)
	}
}
