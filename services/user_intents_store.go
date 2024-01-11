package services

import (
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

// UserIntentStore is syncmap for intents
type UserIntentStore struct {
	Cache *syncmap.SyncMap[string, types.UserIntent]
}

// NewUserIntentsStore constructor for UserIntentStore
func NewUserIntentsStore() *UserIntentStore {
	return &UserIntentStore{
		Cache: syncmap.NewStringMapOf[types.UserIntent](),
	}
}
