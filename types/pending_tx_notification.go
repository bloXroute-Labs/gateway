package types

import bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

// PendingTransactionNotification represents a pending transaction notification.
type PendingTransactionNotification struct {
	NewTransactionNotification
}

// CreatePendingTransactionNotification creates PendingTransactionNotification.
// ethTx is the shared parsed transaction; it must not be nil.
func CreatePendingTransactionNotification(hash SHA256Hash, flags TxFlags, ethTx *EthTransaction) Notification {
	return &PendingTransactionNotification{
		NewTransactionNotification: NewTransactionNotification{
			EthTransaction: ethTx,
			hash:           hash,
			localRegion:    TFLocalRegion&flags != 0,
		},
	}
}

// NotificationType - returns the feed name notification
func (pendingTransactionNotification *PendingTransactionNotification) NotificationType() bxtypes.FeedType {
	return bxtypes.PendingTxsFeed
}
