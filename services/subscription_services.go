package services

import (
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"

	baseutils "github.com/bloXroute-Labs/gateway/v2/utils"
)

// SubscriptionServices provides interface to core subscription management functions
type SubscriptionServices interface {
	SendSubscribeNotification(*sdnmessage.SubscriptionModel) (bool, string, chan *sdnmessage.SubscriptionPermissionMessage)
	SendUnsubscribeNotification(*sdnmessage.SubscriptionModel)
	SendSubscriptionResetNotification([]sdnmessage.SubscriptionModel)
	GenerateSubscriptionID(bool) string
	Close(reason string) error
}

// NoOpSubscriptionServices no-op implementation of SubscriptionServices interface
type NoOpSubscriptionServices struct{}

// NewNoOpSubscriptionServices returns no-op set of subscription services
func NewNoOpSubscriptionServices() SubscriptionServices {
	noOpSubscriptionServices := NoOpSubscriptionServices{}
	return noOpSubscriptionServices
}

// SendSubscribeNotification approves all requests
func (n NoOpSubscriptionServices) SendSubscribeNotification(*sdnmessage.SubscriptionModel) (bool, string, chan *sdnmessage.SubscriptionPermissionMessage) {
	return true, "", nil
}

// SendUnsubscribeNotification - no-op
func (n NoOpSubscriptionServices) SendUnsubscribeNotification(*sdnmessage.SubscriptionModel) {
	return
}

// SendSubscriptionResetNotification - no-op
func (n NoOpSubscriptionServices) SendSubscriptionResetNotification([]sdnmessage.SubscriptionModel) {
	return
}

// GenerateSubscriptionID generate uuid
func (n NoOpSubscriptionServices) GenerateSubscriptionID(ethSubscribe bool) string {
	return generateSubscriptionID(ethSubscribe)
}

// Close - no-op
func (n NoOpSubscriptionServices) Close(string) error {
	return nil
}

func generateSubscriptionID(ethSubscribe bool) string {
	if ethSubscribe {
		u128, _ := baseutils.GenerateU128()
		return u128
	}

	return baseutils.GenerateUUID()
}
