package types

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleInvalidTxNotification(t *testing.T) {
	invalidTxNotification := mockNewInvalidTxNotification()
	notificationWithFields, err := invalidTxNotification.Fields([]string{"tx_contents.from"})
	assert.Error(t, err)
	assert.Nil(t, notificationWithFields)
}

func TestHandleValidTxNotification(t *testing.T) {
	validTxNotification := mockNewValidTxNotification()
	notificationWithFields, err := validTxNotification.Fields([]string{"tx_contents.from"})
	assert.NoError(t, err)
	assert.NotNil(t, notificationWithFields)
	assert.Equal(t, "0x5444d5db68dbfe553afa40d83d056a1cfe281ef7", notificationWithFields["from"])
}

func mockNewValidTxNotification() *NewTransactionNotification {
	var hash SHA256Hash
	hashRes, _ := hex.DecodeString("d04a43269e0009dade74f47b8aa9f8b28e372360ba45cbd9c9e9535bb952f74d")
	copy(hash[:], hashRes)
	content1, _ := hex.DecodeString("f88a8301b7f8851bf08eb00083013880945444d5db68dbfe553afa40d83d056a1cfe281ef788050b32f9024860009ad0a1d0bbd0b0d0b2d0b020d0a3d0bad180d0b0d197d0bdd1962138a0d8cae72eef3771c029cb3c9f15c5a607f4afba32be75316e110093e47baaacbca0215f74222e026dbc7e5f696925f723a417fccedc80c67b9697e7a2764da286a1")

	ethTx := NewEthTransactionFromBytes(content1, EmptySender)

	return CreateNewTransactionNotification(hash, TFPaidTx, ethTx)
}

func mockNewInvalidTxNotification() *NewTransactionNotification {
	var hash SHA256Hash
	hashRes, _ := hex.DecodeString("ed2b4580a766bc9d81c73c35a8496f0461e9c261621cb9f4565ae52ade")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8708301b7f8851bf08eb0008301388094b877c7e556d50b0027053336b90f36becf67b3dd88050b32f902486000801ca0aa803263146bda76a58ebf9f54be589280e920616bc57e7bd68248821f46fd0ca040266f84a2ecd4719057b0633cc80e3e0b3666f6")

	ethTx := NewEthTransactionFromBytes(content, EmptySender)
	return CreateNewTransactionNotification(hash, TFPaidTx, ethTx)
}
