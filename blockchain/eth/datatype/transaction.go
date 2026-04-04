package datatype

import (
	"sync"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// ProcessingETHTransaction structure of list for processing eth transactions
type ProcessingETHTransaction struct {
	txs                 ethtypes.Transactions
	isAllowedForInbound []bool
	filteredInbound     ethtypes.Transactions
	filteredOnce        sync.Once
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
		p.filteredOnce.Do(func() {
			p.filteredInbound = make(ethtypes.Transactions, 0, len(p.txs))
			for i, tx := range p.txs {
				if p.isAllowedForInbound[i] {
					p.filteredInbound = append(p.filteredInbound, tx)
				}
			}
		})
		return p.filteredInbound
	}
	return p.txs
}
