package bxmessage

import (
	"bytes"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	uuid "github.com/satori/go.uuid"
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
	strSourceID := "4c5df5f8-2fd9-4739-a319-8beeba554a88"
	var sourceID [16]byte
	uSourceID, _ := uuid.FromString(strSourceID)
	copy(sourceID[:], uSourceID[:])
	tx := Tx{
		sourceID: sourceID,
	}
	assert.Equal(t, types.NodeID(strSourceID), tx.SourceID())

	newSourceID := "9ee4ec57-d189-428e-92e6-d496670b5022"
	tx.SetSourceID(types.NodeID(newSourceID))
	assert.Equal(t, types.NodeID(newSourceID), tx.SourceID())

	newInvalidSourceID := "invalid-source-id"
	tx.SetSourceID(types.NodeID(newInvalidSourceID))
	assert.Equal(t, types.NodeID(newSourceID), tx.SourceID())

}
