package blockproposer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/atomic"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/ptr"
)

// proposedBlockArgs is the arguments for the proposed block
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

// proposedBlockRequest is a request to propose a block
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

// newProposedBlockRequest creates a new proposedBlockRequest
func newProposedBlockRequest() *proposedBlockRequest {
	return &proposedBlockRequest{sent: atomic.NewBool(false)}
}

// submit submits the request to the validator
func (r *proposedBlockRequest) submit(ctx context.Context, clock utils.Clock, callerManager caller.Manager) error {
	if err := r.compareAndSetIsSent(); err != nil {
		return err
	}

	return r.send(ctx, clock, callerManager)
}

// send sends the request to the validator
func (r *proposedBlockRequest) send(ctx context.Context, clock utils.Clock, callerManager caller.Manager) error {
	client, err := callerManager.GetClient(ctx, r.addr)
	if err != nil {
		return fmt.Errorf("failed to get client for %s: %w", r.addr, err)
	}

	var result any

	r.sentTime = clock.Now()
	err = client.CallContext(ctx, &result, r.method, r.args)
	r.sendingDuration = clock.Now().Sub(r.sentTime)

	if err != nil {
		return fmt.Errorf("failed to call validator with %s: %w", r.method, err)
	}

	var body []byte

	if body, err = json.Marshal(result); err != nil {
		return fmt.Errorf("failed serializing result, message %+v: %w", result, err)
	}

	r.validatorReply = string(body)
	r.validatorReplyTime = clock.Now().UnixMilli()

	return nil
}

// update updates the request with the new one
func (r *proposedBlockRequest) update(req *proposedBlockRequest) error {
	if req.cmp(r) != 1 {
		return fmt.Errorf("blockReward(%s) is lower than the one is in memory(%s)", req.args.BlockReward, r.args.BlockReward)
	}

	r.args = req.args
	r.receivedTime = req.receivedTime
	r.id = req.id
	r.addr = req.addr
	r.method = req.method
	r.preparingDuration = req.preparingDuration

	return nil
}

// isSent returns true if the block was sent
func (r *proposedBlockRequest) isSent() bool {
	return r.sent.Load()
}

func (r *proposedBlockRequest) compareAndSetIsSent() (err error) {
	if !r.sent.CompareAndSwap(false, true) {
		err = errAlreadySent
	}

	return
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

// String returns a string representation of the proposedBlockRequest
func (r *proposedBlockRequest) String() string {
	return "{" +
		"ID: " + r.id + ", " +
		"sendingDuration: " + r.sendingDuration.String() + ", " +
		"receivedTime: " + r.receivedTime.Format(bsc.BlockProposingDateFormat) + ", " +
		"sentTime: " + r.sentTime.Format(bsc.BlockProposingDateFormat) + ", " +
		"validatorReply: " + r.validatorReply + ", " +
		"validatorReplyTime: " + time.UnixMilli(r.validatorReplyTime).Format(bsc.BlockProposingDateFormat) + ", " +
		"}"
}

// ProposedBlockRequests is a list of proposedBlockRequest
type ProposedBlockRequests struct {
	sendingState *atomic.Pointer[sendingState]
	requests     []*proposedBlockRequest
}

// NewProposedBlockRequests creates a new ProposedBlockRequests
func NewProposedBlockRequests(requests ...*proposedBlockRequest) *ProposedBlockRequests {
	r := &ProposedBlockRequests{
		sendingState: atomic.NewPointer(ptr.New(sendingStateNotSent)),
		requests:     make([]*proposedBlockRequest, len(requests)),
	}

	for _, req := range requests {
		r.add(req)
	}

	return r
}

// len returns the length of the list
func (r *ProposedBlockRequests) len() int { return len(r.requests) }

// get returns the requests
func (r *ProposedBlockRequests) get() []*proposedBlockRequest { return r.requests }

// tail returns the last request in the list
func (r *ProposedBlockRequests) tail() *proposedBlockRequest {
	return r.requests[len(r.requests)-1]
}

// add adds a new request to the list
func (r *ProposedBlockRequests) add(req *proposedBlockRequest) {
	r.requests = append(r.requests, req)
}

// setSendingState sets the sending state
func (r *ProposedBlockRequests) setSendingState(state sendingState) {
	r.sendingState.Store(ptr.New(state))
}

// requireSent returns an error if the block was not sent or was replaced with a higher reward
func (r *ProposedBlockRequests) requireSent() error {
	switch *r.sendingState.Load() {
	case sendingStateNotSent:
		return errBlockNotSent
	case sendingStateReplacedWithHigherReward:
		return errBlockReplacedWithNew
	default:
		return nil
	}
}
