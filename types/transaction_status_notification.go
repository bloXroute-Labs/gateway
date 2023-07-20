package types

// TransactionStatusNotification - represents a transaction status notification
type TransactionStatusNotification struct {
	SubscriptionIDs []string `json:"subscription_ids,omitempty"`
	TransactionHash string   `json:"transaction_hash,omitempty"`
	Status          Status   `json:"status,omitempty"`
}

// Status types of transaction state
type Status string

// Unknown is transaction status for unknown state
const Unknown Status = "unknown_state"

// TxPool is transaction status for pending status
const TxPool Status = "tx_pool"

// Mined is transaction status for confirmed status
const Mined Status = "mined"

// Canceled is transaction status for canceled status
const Canceled Status = "canceled"

// Replaced is transaction status for replaced status
const Replaced Status = "replaced"

// UpdateSource is the source for updating transaction status
type UpdateSource string

// NewBlocks is the feed name of the source for transaction status update
const NewBlocks UpdateSource = "newBlocks"

// PendingTxs is the feed name of the source for transaction status update
const PendingTxs UpdateSource = "pendingTxs"

// WithFields -
func (tn *TransactionStatusNotification) WithFields(fields []string) Notification {
	txStatusNotification := TransactionStatusNotification{}
	for _, param := range fields {
		switch param {
		case "transaction_hash":
			txStatusNotification.TransactionHash = tn.TransactionHash
		case "status":
			txStatusNotification.Status = tn.Status
		case "subscription_id":
			txStatusNotification.SubscriptionIDs = tn.SubscriptionIDs
		}
	}
	return &txStatusNotification
}

// NotificationType - returns the feed name notification
func (tn *TransactionStatusNotification) NotificationType() FeedType {
	return TransactionStatusFeed
}

// Filters -
func (tn *TransactionStatusNotification) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (tn *TransactionStatusNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (tn *TransactionStatusNotification) GetHash() string {
	return tn.TransactionHash
}
