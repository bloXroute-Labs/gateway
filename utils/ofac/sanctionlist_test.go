package ofac

import (
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	types "github.com/bloXroute-Labs/gateway/v2/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

func createTransaction(rawTx string) (*ethtypes.Transaction, error) {
	txBytes, err := types.DecodeHex(rawTx)
	if err != nil {
		return nil, err
	}
	var ethTx ethtypes.Transaction
	err = ethTx.UnmarshalBinary(txBytes)

	if err != nil {
		return nil, err
	}
	return &ethTx, nil
}

func TestSanctionList_ShouldBlockTransaction_True(t *testing.T) {
	blockedAddress := "0x8576acc5c05d6ce88f4e49bf65bdf0c62f91353c"
	sanctionedTx := "f85d808080948576acc5c05d6ce88f4e49bf65bdf0c62f91353c808029a06c2bef8b42311b2a8da478a469fd2620278a2a1bcce9d9d4d768dec6bb90efaba0147b687755458d6d8ef964b8d3080b6abd89a9900f59f6446b5126d2349dfa1f"

	tx, err := createTransaction(sanctionedTx)
	assert.Nil(t, err)

	addresses, shouldBlock := ShouldBlockTransaction(tx)
	assert.Contains(t, addresses, blockedAddress)
	assert.True(t, shouldBlock)
}

func TestSanctionList_ShouldBlockTransaction_False(t *testing.T) {
	tx, err := createTransaction(fixtures.LegacyTransaction)
	assert.Nil(t, err)

	addresses, shouldBlock := ShouldBlockTransaction(tx)
	assert.Empty(t, addresses)
	assert.False(t, shouldBlock)
}
