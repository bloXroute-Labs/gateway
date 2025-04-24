package types

import "github.com/bloXroute-Labs/bxcommon-go/types"

// SubscriptionNotificationType represents the available feed subscription notification types
type SubscriptionNotificationType string

// SubscriptionNotificationType enumeration
const (
	SubscriptionNotificationTypeSubscribe   SubscriptionNotificationType = "SUBSCRIBE"
	SubscriptionNotificationTypeUnsubscribe SubscriptionNotificationType = "UNSUBSCRIBE"
	SubscriptionNotificationTypeReset       SubscriptionNotificationType = "RESET"
)

// SubscriptionNotification represents a notification for a subscription event
type SubscriptionNotification struct {
	Type          SubscriptionNotificationType `json:"type"`
	NodeID        string                       `json:"node_id"`
	Subscriptions []SubscriptionModel          `json:"subscriptions"`
}

// SubscriptionModel represents a feed subscription
type SubscriptionModel struct {
	SubscriptionID string           `json:"subscription_id"`
	SubscriberIP   string           `json:"subscriber_ip"`
	NodeID         string           `json:"node_id"`
	AccountID      types.AccountID  `json:"account_id"`
	NetworkNum     types.NetworkNum `json:"blockchain_network_num"`
	FeedType       FeedType         `json:"feed_type"`
}

// SubscriptionPermissionMessage represents SDN response to subscription request
type SubscriptionPermissionMessage struct {
	SubscriptionID string          `json:"subscription_id"`
	AccountID      types.AccountID `json:"account_id"`
	Allowed        bool            `json:"allowed"`
	ErrorReason    string          `json:"error_reason"`
}

// SubscriptionRequestAllMessage represents SDN request for subscriptions
type SubscriptionRequestAllMessage struct{}
