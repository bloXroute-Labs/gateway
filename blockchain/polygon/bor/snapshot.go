package bor

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	bxgateway "github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon/bor/valset"
)

const methodGetSnapshot = "bor_getSnapshot"

// Snapshot bor snapshot info
type Snapshot struct {
	Number       uint64                    `json:"number"`       // Block number where the snapshot was created
	Hash         common.Hash               `json:"hash"`         // Block hash where the snapshot was created
	ValidatorSet *valset.ValidatorSet      `json:"validatorSet"` // Validator set at this moment
	Recents      map[uint64]common.Address `json:"recents"`      // Set of recent signers for spam protections
}

// copy creates a deep copy of the snapshot, though not the individual votes.
func (s *Snapshot) copy() *Snapshot {
	snapshot := &Snapshot{
		Number:       s.Number,
		Hash:         s.Hash,
		ValidatorSet: s.ValidatorSet.Copy(),
		Recents:      make(map[uint64]common.Address),
	}
	for block, signer := range s.Recents {
		snapshot.Recents[block] = signer
	}

	return snapshot
}

// IncrementSprint changes state of Snapshot to the state on the start of next sprint
func (s *Snapshot) IncrementSprint(currentSpan *SpanInfo, nextSpan *SpanInfo) (*Snapshot, error) {
	snap := s.copy()
	sprintStart := SprintStart(snap.Number)
	signer := snap.ValidatorSet.GetProposer().Address

	snap.Number = sprintStart + SprintSize
	snap.Recents = make(map[uint64]common.Address, SprintSize)

	for i := sprintStart + 1; i < snap.Number; i++ {
		snap.Recents[i] = signer
	}

	var newVals []*valset.Validator

	if snap.Number > currentSpan.EndBlock {
		newVals = nextSpan.SelectedProducers
	} else {
		newVals = snap.ValidatorSet.Validators
	}

	var err error

	snap.ValidatorSet, err = snap.nextSprintValidatorSet(newVals)
	if err != nil {
		return nil, err
	}

	return snap, nil
}

func (s *Snapshot) nextSprintValidatorSet(newVals []*valset.Validator) (*valset.ValidatorSet, error) {
	validatorSet, err := getUpdatedValidatorSet(s.ValidatorSet.Copy(), newVals)
	if err != nil {
		return nil, err
	}

	validatorSet.IncrementProposerPriority(1)

	return validatorSet, nil
}

func validatorContains(a []*valset.Validator, x *valset.Validator) (*valset.Validator, bool) {
	for _, n := range a {
		if n.Address == x.Address {
			return n, true
		}
	}

	return nil, false
}

func getUpdatedValidatorSet(oldValidatorSet *valset.ValidatorSet, newVals []*valset.Validator) (*valset.ValidatorSet, error) {
	v := oldValidatorSet
	oldVals := v.Validators

	changes := make([]*valset.Validator, 0, len(oldVals))

	for _, ov := range oldVals {
		if f, ok := validatorContains(newVals, ov); ok {
			ov.VotingPower = f.VotingPower
		} else {
			ov.VotingPower = 0
		}

		changes = append(changes, ov)
	}

	for _, nv := range newVals {
		if _, ok := validatorContains(changes, nv); !ok {
			changes = append(changes, nv)
		}
	}

	if err := v.UpdateWithChangeSet(changes); err != nil {
		return nil, err
	}

	return v, nil
}

// GetLatestSnapshot returns latest Snapshot.
func GetLatestSnapshot(provider blockchain.WSProvider) (*Snapshot, error) {
	return GetSnapshot(provider, "latest")
}

// GetSnapshot perform call to RPC.
func GetSnapshot(provider blockchain.WSProvider, payload ...any) (*Snapshot, error) {
	resp, err := provider.CallRPC(methodGetSnapshot,
		payload,
		blockchain.RPCOptions{
			RetryAttempts: bxgateway.MaxEthOnBlockCallRetries,
			RetryInterval: bxgateway.EthOnBlockCallRetrySleepInterval,
		})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to call rpc")
	}

	marshal, err := json.Marshal(resp)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal response to bytes")
	}

	snapshot := new(Snapshot)

	if err = json.Unmarshal(marshal, snapshot); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal response to snapshot")
	}

	return snapshot, nil
}
