package blockproposer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// Submitter is a function that submits a block to the network
type Submitter func(ctx context.Context, clock utils.Clock, callerManager caller.Manager) (msg string, err error)

// ProposedBlockStore stores the proposed blocks
type ProposedBlockStore interface {
	Get(num rpc.BlockNumber) *ProposedBlockRequests
	CreateOrUpdate(num rpc.BlockNumber, req *proposedBlockRequest) (created bool, err error)
	SubmitBlock(num rpc.BlockNumber, fromHighLoad bool) Submitter
	Shift(num rpc.BlockNumber)
	SetState(num rpc.BlockNumber, state sendingState)
}

// ProposedBlockMap stores the proposed blocks. Actual implementation of ProposedBlockStore
type ProposedBlockMap struct {
	mx *sync.RWMutex
	l  []rpc.BlockNumber
	m  map[rpc.BlockNumber]*ProposedBlockRequests

	highLoadTxNumThreshold int
}

// NewProposedBlockMap creates a new ProposedBlockMap
func NewProposedBlockMap(size int64, highLoadTxNumThreshold int) *ProposedBlockMap {
	return &ProposedBlockMap{
		mx: new(sync.RWMutex),
		m:  make(map[rpc.BlockNumber]*ProposedBlockRequests, size),
		l:  make([]rpc.BlockNumber, 0, size),

		highLoadTxNumThreshold: highLoadTxNumThreshold,
	}
}

// Get the request in the map for the given block number
func (s *ProposedBlockMap) Get(num rpc.BlockNumber) *ProposedBlockRequests {
	s.mx.RLock()
	defer s.mx.RUnlock()

	return s.unsafeGet(num)
}

// CreateOrUpdate creates or updates the request in the map for the given block number
func (s *ProposedBlockMap) CreateOrUpdate(num rpc.BlockNumber, req *proposedBlockRequest) (created bool, err error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	return s.unsafeCreateOrUpdate(num, req)
}

// SubmitBlock submits the request in the map for the given block number
func (s *ProposedBlockMap) SubmitBlock(num rpc.BlockNumber, fromHighLoad bool) Submitter {
	s.mx.Lock()
	defer s.mx.Unlock()

	return s.unsafeSubmitBlock(num, fromHighLoad)
}

// Shift shifts the map by removing the oldest block number
func (s *ProposedBlockMap) Shift(num rpc.BlockNumber) {
	s.mx.Lock()
	s.unsafeShift(num)
	s.mx.Unlock()
}

// SetState sets the sending state of the request in the map for the given block number
func (s *ProposedBlockMap) SetState(num rpc.BlockNumber, state sendingState) {
	s.mx.Lock()
	s.unsafeSetState(num, state)
	s.mx.Unlock()
}

// --- unsafe methods ---

// unsafeExists checks if the block number exists in the map (without locking)
func (s *ProposedBlockMap) unsafeExists(num rpc.BlockNumber) bool {
	_, exists := s.m[num]
	return exists
}

// unsafeDelete deletes the block number from the map (without locking)
func (s *ProposedBlockMap) unsafeDelete(num rpc.BlockNumber) { delete(s.m, num) }

// unsafeGet gets the request in the map for the given block number (without locking)
func (s *ProposedBlockMap) unsafeGet(num rpc.BlockNumber) *ProposedBlockRequests { return s.m[num] }

// unsafeCreateOrUpdate creates or updates the request in the map for the given block number (without locking)
func (s *ProposedBlockMap) unsafeCreateOrUpdate(num rpc.BlockNumber, req *proposedBlockRequest) (created bool, err error) {
	// ensure the block number exists
	if !s.unsafeExists(num) {
		created = true
		s.m[num] = NewProposedBlockRequests(newProposedBlockRequest())
	}

	return created, s.unsafeUpdate(num, req)
}

// unsafeUpdate updates the request in the map for the given block number (without locking)
func (s *ProposedBlockMap) unsafeUpdate(num rpc.BlockNumber, req *proposedBlockRequest) error {
	requests := s.m[num]
	if requests == nil {
		return errBlockNotFound
	}

	// Get the most recent request for this block number
	latestReq := requests.tail()
	if latestReq == nil {
		return errBlockNotFound
	}

	// If the latest request hasn't been sent, update it
	if !latestReq.isSent() {
		return latestReq.update(req)
	}

	// Check if the new request has a higher block reward than the latest one
	if req.cmp(latestReq) != 1 {
		return fmt.Errorf("blockReward (%s) is lower than the one in memory (%s)", req.args.BlockReward, latestReq.args.BlockReward)
	}

	// Append the new request to the list
	s.m[num].add(req)

	return nil
}

// unsafeCreate creates the request in the map for the given block number (without locking)
func (s *ProposedBlockMap) unsafeCreate(num rpc.BlockNumber) {
	if s.unsafeExists(num) {
		return
	}

	s.m[num] = NewProposedBlockRequests(newProposedBlockRequest())
}

// unsafeSubmitBlock submits the request in the map for the given block number (without locking)
func (s *ProposedBlockMap) unsafeSubmitBlock(num rpc.BlockNumber, fromHighLoad bool) Submitter {
	requests := s.m[num]
	if requests == nil {
		return nil
	}

	requestsLen := requests.len()
	if requestsLen == 0 {
		return nil
	}

	latestReq := requests.tail()
	if latestReq == nil {
		return nil
	}

	// if the block is from high load and the number of transactions is LESS than the threshold, don't send it
	if fromHighLoad && len(latestReq.args.Payload) < s.highLoadTxNumThreshold {
		return nil
	}

	// if the block is NOT from high load and the number of transactions is MORE than the threshold, don't send it
	if !fromHighLoad && len(latestReq.args.Payload) > s.highLoadTxNumThreshold {
		return nil
	}

	if latestReq.compareAndSetIsSent() != nil {
		return nil
	}

	return func(ctx context.Context, clock utils.Clock, callerManager caller.Manager) (msg string, err error) {
		msg = fmt.Sprintf("proposing blockNum(%d) ID(%s) blockReward(%s) blockPreparingDuration(%s)", num, latestReq.id, latestReq.args.BlockReward, latestReq.preparingDuration)

		err = latestReq.send(ctx, clock, callerManager)
		switch {
		case err == nil:
			msg += fmt.Sprintf(" submitted(true) validatorReply(%s) validatorReplyTime(%s)", latestReq.validatorReply, time.UnixMilli(latestReq.validatorReplyTime).Format(bsc.BlockProposingDateFormat))
		case errors.Is(err, errAlreadySent):
			err = nil
		}

		return
	}
}

// unsafeShift shifts the map by removing the oldest block number (without locking)
func (s *ProposedBlockMap) unsafeShift(num rpc.BlockNumber) {
	if !s.unsafeExists(num) {
		return
	}

	if len(s.l) == cap(s.l) {
		s.unsafeDelete(s.l[0])
		s.l = append(s.l[1:], num)
	}

	s.unsafeCreate(num)
}

// unsafeSetState sets the sending state of the request in the map for the given block number (without locking)
func (s *ProposedBlockMap) unsafeSetState(num rpc.BlockNumber, state sendingState) {
	if !s.unsafeExists(num) {
		return
	}

	s.m[num].setSendingState(state)
}
