package valset

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Validator represets Volatile state for each Validator
type Validator struct {
	ID               uint64         `json:"ID"`
	Address          common.Address `json:"signer"`
	VotingPower      int64          `json:"power"`
	ProposerPriority int64          `json:"accum"`
}

// NewValidator creates new validator
func NewValidator(address common.Address, votingPower int64) *Validator {
	return &Validator{
		Address:          address,
		VotingPower:      votingPower,
		ProposerPriority: 0,
	}
}

// Copy creates a new copy of the validator so we can mutate ProposerPriority.
// Panics if the validator is nil.
func (v *Validator) Copy() *Validator {
	vCopy := *v
	return &vCopy
}

// Cmp returns the one validator with a higher ProposerPriority.
// If ProposerPriority is same, it returns the validator with lexicographically smaller address
func (v *Validator) Cmp(other *Validator) *Validator {
	// if both of v and other are nil, nil will be returned and that could possibly lead to nil pointer dereference bubbling up the stack
	if v == nil {
		return other
	}

	if other == nil {
		return v
	}

	if v.ProposerPriority > other.ProposerPriority {
		return v
	}

	if v.ProposerPriority < other.ProposerPriority {
		return other
	}

	result := bytes.Compare(v.Address.Bytes(), other.Address.Bytes())

	if result == 0 {
		panic("Cannot compare identical validators")
	}

	if result < 0 {
		return v
	}

	// result > 0
	return other
}

func (v *Validator) String() string {
	if v == nil {
		return "nil-Validator"
	}

	return fmt.Sprintf("Validator{%v Power:%v Priority:%v}",
		v.Address.Hex(),
		v.VotingPower,
		v.ProposerPriority)
}

// HeaderBytes return header bytes
func (v *Validator) HeaderBytes() []byte {
	result := make([]byte, 40)
	copy(result[:20], v.Address.Bytes())
	copy(result[20:], v.PowerBytes())

	return result
}

// PowerBytes return power bytes
func (v *Validator) PowerBytes() []byte {
	powerBytes := big.NewInt(0).SetInt64(v.VotingPower).Bytes()
	result := make([]byte, 20)
	copy(result[20-len(powerBytes):], powerBytes)

	return result
}
