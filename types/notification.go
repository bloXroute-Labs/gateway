package types

// Notification represents a generic notification that allows filtering its fields
type Notification interface {
	WithFields(fields []string) Notification
	Filters() map[string]interface{}
	LocalRegion() bool
	GetHash() string
	NotificationType() FeedType
}

// CustomNotification represents a notification that can apply account-specific logic
// This interface extends Notification with the purpose of producing
// tailored notifications based on the subscriber's account.
type CustomNotification interface {
	Notification
	ApplyAccountLogic(account string) CustomNotification
}

// BlockNotification represents a generic block notification
type BlockNotification interface {
	Notification

	SetNotificationType(FeedType)
	SetSource(*NodeEndpoint)
	IsNil() bool
	Clone() BlockNotification
}
