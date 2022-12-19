package types

// Notification represents a generic notification that allows filtering its fields
type Notification interface {
	WithFields(fields []string) Notification
	Filters(filters []string) map[string]interface{}
	LocalRegion() bool
	GetHash() string
	NotificationType() FeedType
}

// BlockNotification represents a generic block notification
type BlockNotification interface {
	Notification

	SetNotificationType(FeedType)
	SetSource(*NodeEndpoint)
	IsNil() bool
	Clone() BlockNotification
}
