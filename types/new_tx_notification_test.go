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
	hashRes, _ := hex.DecodeString("ed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade56056d")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8708301b7f8851bf08eb0008301388094b877c7e556d50b0027053336b90f36becf67b3dd88050b32f902486000801ca0aa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0ca040266f84a2ecd4719057b0633cc80e3e0b3666f6f6ec1890a920239634ec6531")

	tx := NewBxTransaction(hash, bxtypes.NetworkNum(5), TFPaidTx, time.Now())
	tx.SetContent(content)
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
