package types

// FeedType types of feeds
type FeedType string

// FeedType enumeration
const (
	NewTxsFeed     FeedType = "newTxs"
	PendingTxsFeed FeedType = "pendingTxs"
	BDNBlocksFeed  FeedType = "bdnBlocks"
	NewBlocksFeed  FeedType = "newBlocks"
	OnBlockFeed    FeedType = "ethOnBlock"
	TxReceiptsFeed FeedType = "txReceipts"
)

// Exists - checks if a field exists in feedType list
func Exists(field FeedType, slice []FeedType) bool {
	for _, valid := range slice {
		if field == valid {
			return true
		}
	}
	return false
}
