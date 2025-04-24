package datatype

import (
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// ProcessingETHTransaction structure of list for processing eth transactions
type ProcessingETHTransaction struct {
	txs                 ethtypes.Transactions
	isAllowedForInbound []bool
}

// NewProcessingETHTransaction return new list with given size
func NewProcessingETHTransaction(size int) *ProcessingETHTransaction {
	return &ProcessingETHTransaction{
		txs:                 make([]*ethtypes.Transaction, 0, size),
		isAllowedForInbound: make([]bool, 0, size),
	}
}

// Add new transaction to list
func (p *ProcessingETHTransaction) Add(tx *ethtypes.Transaction, isAllowedForInbound bool) {
	p.txs = append(p.txs, tx)
	p.isAllowedForInbound = append(p.isAllowedForInbound, isAllowedForInbound)
}

// Transactions return list of transactions based on input parameters
func (p *ProcessingETHTransaction) Transactions(connectionType bxtypes.NodeType, inbound bool) ethtypes.Transactions {
	if connectionType == bxtypes.Blockchain && inbound {
		var result ethtypes.Transactions
		for i, tx := range p.txs {
			if p.isAllowedForInbound[i] {
				result = append(result, tx)
			}
		}
		return result
	}
	return p.txs
}
