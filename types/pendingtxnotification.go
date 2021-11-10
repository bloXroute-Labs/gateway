package types

// PendingTransactionNotification - contains BxTransaction which contains the local region of the ethereum transaction and all its fields.
type PendingTransactionNotification struct {
	NewTransactionNotification
}

// CreatePendingTransactionNotification -  creates PendingTransactionNotification object which contains bxTransaction and local region
func CreatePendingTransactionNotification(bxTx *BxTransaction) Notification {
	return &PendingTransactionNotification{
		NewTransactionNotification{
			bxTx,
			nil,
		},
	}
}

// NotificationType - returns the feed name notification
func (pendingTransactionNotification *PendingTransactionNotification) NotificationType() FeedType {
	return PendingTxsFeed
}
