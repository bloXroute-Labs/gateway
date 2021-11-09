package types

// Notification represents a generic notification that allows filtering its fields
type Notification interface {
	WithFields(fields []string) Notification
	Filters(filters []string) map[string]interface{}
	LocalRegion() bool
	GetHash() string
	NotificationType() FeedType
}
