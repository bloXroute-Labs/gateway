package types

// FeedType types of feeds
type FeedType string

// FeedType enumeration
const (
	NewTxsFeed            FeedType = "newTxs"
	PendingTxsFeed        FeedType = "pendingTxs"
	BDNBlocksFeed         FeedType = "bdnBlocks"
	NewBlocksFeed         FeedType = "newBlocks"
	OnBlockFeed           FeedType = "ethOnBlock"
	TxReceiptsFeed        FeedType = "txReceipts"
	TransactionStatusFeed FeedType = "transactionStatus"
)

// FeedConnectionType types of feeds
type FeedConnectionType string

// FeedConnectionType enumeration
const (
	WebSocketFeed FeedConnectionType = "ws"
	GRPCFeed      FeedConnectionType = "grpc"
)

// Beacon blocks
const (
	NewBeaconBlocksFeed FeedType = "newBeaconBlocks"
	BDNBeaconBlocksFeed FeedType = "bdnBeaconBlocks"
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
