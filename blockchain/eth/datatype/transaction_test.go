package datatype

import (
	"testing"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
)

func TestProcessingETHTransaction(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	p := NewProcessingETHTransaction(10)
	p.Add(bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil), true)
	p.Add(bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, nil), false)

	txs := p.Transactions(bxtypes.Blockchain, false)
	assert.Equal(t, len(txs), 2)
	txs = p.Transactions(bxtypes.Blockchain, true)
	assert.Equal(t, len(txs), 1)
	txs = p.Transactions(bxtypes.Gateway, true)
	assert.Equal(t, len(txs), 2)
}
