package types

// FeedType types of feeds
type FeedType string

// FeedType enumeration
const (
	NewTxsFeed      FeedType = "newTxs"
	PendingTxsFeed  FeedType = "pendingTxs"
	BDNBlocksFeed   FeedType = "bdnBlocks"
	NewBlocksFeed   FeedType = "newBlocks"
	OnBlockFeed     FeedType = "ethOnBlock"
	TxReceiptsFeed  FeedType = "txReceipts"
	TraceBlocksFeed FeedType = "traceBlocks" // only for BSC
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

// AllFeedTypes lists every feed type declared in this package. Components pass it (optionally
// extended with their own feed types) to the feed manager, which gives every listed type its own
// notification channel so that a burst on one feed cannot starve the others.
var AllFeedTypes = []FeedType{
	NewTxsFeed,
	PendingTxsFeed,
	BDNBlocksFeed,
	NewBlocksFeed,
	OnBlockFeed,
	TxReceiptsFeed,
	TraceBlocksFeed,
	NewBeaconBlocksFeed,
	BDNBeaconBlocksFeed,
}

// Exists - checks if a field exists in feedType list
func Exists(field FeedType, slice []FeedType) bool {
	for _, valid := range slice {
		if field == valid {
			return true
		}
	}
	return false
}
