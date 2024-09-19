package types

import "time"

// QuoteNotification describes quote notification
type QuoteNotification struct {
	ID            string    // UUIDv4 - quote id
	DappAddress   string    // ETH Address
	SolverAddress string    // ETH Address
	Quote         []byte    // Variable length
	Hash          []byte    // Keccak256
	Signature     []byte    // ECDSA Signature
	Timestamp     time.Time // Short timestamp
}

// WithFields implements Notification
func (q *QuoteNotification) WithFields(_ []string) Notification {
	return nil
}

// LocalRegion implements Notification
func (q *QuoteNotification) LocalRegion() bool {
	return true
}

// GetHash implements Notification
func (q *QuoteNotification) GetHash() string {
	return string(q.Hash)
}

// NotificationType implements Notification
func (q *QuoteNotification) NotificationType() FeedType {
	return QuotesFeed
}

// Filters return a map of key,value that can be used to filter transactions
func (q *QuoteNotification) Filters(_ []string) map[string]interface{} {
	return nil
}
