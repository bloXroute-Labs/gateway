package metrics

// Exporter is an interface for sending metrics
type Exporter interface {
	PushIncrFeedNotificationCreated(networkNum uint32, notificationType string)
	PushIncrFeedNotificationProcessed(networkNum uint32, notificationType string)
	PushIncrFeedNotificationDelivered(networkNum uint32, notificationType string, accountID string)
}

// NoOpExporter is a no-op implementation of the metrics Exporter interface
type NoOpExporter struct{}

// PushIncrFeedNotificationCreated does nothing
func (n *NoOpExporter) PushIncrFeedNotificationCreated(uint32, string) {}

// PushIncrFeedNotificationProcessed does nothing
func (n *NoOpExporter) PushIncrFeedNotificationProcessed(uint32, string) {
}

// PushIncrFeedNotificationDelivered does nothing
func (n *NoOpExporter) PushIncrFeedNotificationDelivered(uint32, string, string) {
}
