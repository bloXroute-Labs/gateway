package types

import (
	"sync"
	"time"
)

// UserIntent describes user request for submitting intent
type UserIntent struct {
	ID            string
	DappAddress   string
	SenderAddress string
	Intent        []byte // The intent payload
	Hash          []byte
	Signature     []byte
	Timestamp     time.Time
}

// UserIntentNotification describes user intent notification
type UserIntentNotification struct {
	UserIntent
	lock sync.Mutex
}

// NewUserIntentNotification constructor for UserIntentNotification
func NewUserIntentNotification(intent *UserIntent) *UserIntentNotification {
	return &UserIntentNotification{
		UserIntent: *intent,
		lock:       sync.Mutex{},
	}
}

// WithFields not implemented
func (i *UserIntentNotification) WithFields(_ []string) Notification {
	return nil
}

// LocalRegion implements Notification
func (i *UserIntentNotification) LocalRegion() bool {
	return true
}

// GetHash implements Notification
func (i *UserIntentNotification) GetHash() string {
	return string(i.Hash)
}

// NotificationType implements Notification
func (i *UserIntentNotification) NotificationType() FeedType {
	return UserIntentsFeed
}

// Filters returns a map of key,value that can be used to filter transactions
func (i *UserIntentNotification) Filters(_ []string) map[string]interface{} {
	return nil
}
