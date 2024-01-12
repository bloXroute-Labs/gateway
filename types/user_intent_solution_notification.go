package types

import (
	"sync"
	"time"
)

// UserIntentSolution describes IntentSolution submission body
type UserIntentSolution struct {
	ID            string
	SolverAddress string
	IntentID      string
	Solution      []byte // The intent payload
	Hash          []byte
	Signature     []byte
	Timestamp     time.Time
}

// UserIntentSolutionNotification describes IntentSolution user notification
type UserIntentSolutionNotification struct {
	UserIntentSolution
	lock sync.Mutex
}

// NewUserIntentSolutionNotification constructor for UserIntentSolutionNotification
func NewUserIntentSolutionNotification(solution *UserIntentSolution) *UserIntentSolutionNotification {
	return &UserIntentSolutionNotification{
		UserIntentSolution: *solution,
		lock:               sync.Mutex{},
	}
}

// WithFields implements Notification
func (u *UserIntentSolutionNotification) WithFields(_ []string) Notification {
	return nil
}

// LocalRegion implements Notification
func (u *UserIntentSolutionNotification) LocalRegion() bool {
	return true
}

// GetHash implements Notification
func (u *UserIntentSolutionNotification) GetHash() string {
	return string(u.Hash)
}

// NotificationType implements Notification
func (u *UserIntentSolutionNotification) NotificationType() FeedType {
	return UserIntentSolutionsFeed
}

// Filters returns a map of key,value that can be used to filter transactions
func (u *UserIntentSolutionNotification) Filters(_ []string) map[string]interface{} {
	return nil
}
