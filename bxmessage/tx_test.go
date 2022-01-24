package bxmessage

import (
	"bytes"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

var nullByteAccountID = bytes.Repeat([]byte("\x00"), 36)

func TestTx_AccountIDEmpty(t *testing.T) {
	tx := Tx{}
	assert.Equal(t, types.AccountID(""), tx.AccountID())
}

func TestTx_AccountIDNullBytes(t *testing.T) {
	accountID := [AccountIDLen]byte{}
	copy(accountID[:], nullByteAccountID)

	tx := Tx{
		accountID: accountID,
	}
	assert.Equal(t, types.AccountID(""), tx.AccountID())

	b, _ := tx.Pack(AccountProtocol)
	tx2 := Tx{}
	_ = tx2.Unpack(b, AccountProtocol)
	assert.Equal(t, types.AccountID(""), tx2.AccountID())
}

func TestTx_SourceIDValid(t *testing.T) {
	sourceID := "4c5df5f8-2fd9-4739-a319-8beeba554a88"
	tx := Tx{}
	err := tx.SetSourceID(types.NodeID(sourceID))
	assert.Nil(t, err)
	assert.Equal(t, types.NodeID(sourceID), tx.SourceID())

	newSourceID := "9ee4ec57-d189-428e-92e6-d496670b5022"
	err = tx.SetSourceID(types.NodeID(newSourceID))
	assert.Nil(t, err)
	assert.Equal(t, types.NodeID(newSourceID), tx.SourceID())

	newInvalidSourceID := "invalid-source-id"
	err = tx.SetSourceID(types.NodeID(newInvalidSourceID))
	assert.NotNil(t, err)
}
