package services

import (
	"github.com/bloXroute-Labs/gateway/v2/types"
	baseutils "github.com/bloXroute-Labs/gateway/v2/utils"
)

// SubscriptionServices provides interface to core subscription management functions
type SubscriptionServices interface {
	IsSubscriptionAllowed(*types.SubscriptionModel) (bool, string, chan *types.SubscriptionPermissionMessage)
	SendUnsubscribeNotification(*types.SubscriptionModel)
	SendSubscriptionResetNotification([]types.SubscriptionModel)
	GenerateSubscriptionID(bool) string
}

// NoOpSubscriptionServices no-op implementation of SubscriptionServices interface
type NoOpSubscriptionServices struct {
}

// NewNoOpSubscriptionServices returns no-op set of subscription services
func NewNoOpSubscriptionServices() SubscriptionServices {
	noOpSubscriptionServices := NoOpSubscriptionServices{}
	return noOpSubscriptionServices
}

// IsSubscriptionAllowed approves all requests
func (n NoOpSubscriptionServices) IsSubscriptionAllowed(*types.SubscriptionModel) (bool, string, chan *types.SubscriptionPermissionMessage) {
	return true, "", nil
}

// SendUnsubscribeNotification - no-op
func (n NoOpSubscriptionServices) SendUnsubscribeNotification(*types.SubscriptionModel) {
	return
}

// SendSubscriptionResetNotification - no-op
func (n NoOpSubscriptionServices) SendSubscriptionResetNotification([]types.SubscriptionModel) {
	return
}

// GenerateSubscriptionID generate uuid
func (n NoOpSubscriptionServices) GenerateSubscriptionID(ethSubscribe bool) string {
	if ethSubscribe {
		u128, _ := baseutils.GenerateU128()
		return u128
	}

	return baseutils.GenerateUUID()
}
