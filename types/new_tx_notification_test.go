package types

import (
	"encoding/hex"
	"testing"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/assert"
)

func TestHandleInvalidTxNotification(t *testing.T) {
	invalidTxNotification := mockNewInvalidTxNotification()
	assert.Equal(t, invalidTxNotification.validationStatus, TxPendingValidation)
	notificationWithFields := invalidTxNotification.Fields([]string{"tx_contents.from"})
	assert.Nil(t, notificationWithFields)
	assert.Equal(t, invalidTxNotification.validationStatus, TxInvalid)
}

func TestHandleValidTxNotification(t *testing.T) {
	validTxNotification := mockNewValidTxNotification()
	assert.Equal(t, validTxNotification.validationStatus, TxPendingValidation)
	notificationWithFields := validTxNotification.Fields([]string{"tx_contents.from"})
	assert.NotNil(t, notificationWithFields)
	assert.Equal(t, validTxNotification.validationStatus, TxValid)
}

func mockNewValidTxNotification() *NewTransactionNotification {
	var hash SHA256Hash
	hashRes, _ := hex.DecodeString("d04a43269e0009dade74f47b8aa9f8b28e372360ba45cbd9c9e9535bb952f74d")
	copy(hash[:], hashRes)
	content1, _ := hex.DecodeString("f88a8301b7f8851bf08eb00083013880945444d5db68dbfe553afa40d83d056a1cfe281ef788050b32f9024860009ad0a1d0bbd0b0d0b2d0b020d0a3d0bad180d0b0d197d0bdd1962138a0d8cae72eef3771c029cb3c9f15c5a607f4afba32be75316e110093e47baaacbca0215f74222e026dbc7e5f696925f723a417fccedc80c67b9697e7a2764da286a1")

	tx := NewBxTransaction(hash, bxtypes.NetworkNum(5), TFPaidTx, time.Now())
	tx.SetContent(content1)

	return CreateNewTransactionNotification(tx)
}

func mockNewInvalidTxNotification() *NewTransactionNotification {
	var hash SHA256Hash
	// length too short
	hashRes, _ := hex.DecodeString("ed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade")
	copy(hash[:], hashRes)
	// length too short
	content, _ := hex.DecodeString("f8708301b7f8851bf08eb0008301388094b877c7e556d50b0027053336b90f36becf67b3dd88050b32f902486000801ca0aa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0ca040266f84a2ecd4719057b0633cc80e3e0b3666f6")

	tx := NewBxTransaction(hash, bxtypes.NetworkNum(5), TFPaidTx, time.Now())
	tx.SetContent(content)
	return CreateNewTransactionNotification(tx)
}
