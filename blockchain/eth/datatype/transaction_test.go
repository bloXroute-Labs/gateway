package datatype

import (
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/utils"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

func TestProcessingETHTransaction(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	p := NewProcessingETHTransaction(10)
	p.Add(bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil), true)
	p.Add(bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, nil), false)

	txs := p.Transactions(utils.Blockchain, false)
	assert.Equal(t, len(txs), 2)
	txs = p.Transactions(utils.Blockchain, true)
	assert.Equal(t, len(txs), 1)
	txs = p.Transactions(utils.Gateway, true)
	assert.Equal(t, len(txs), 2)
}
