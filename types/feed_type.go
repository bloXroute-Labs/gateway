package types

import bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

// TraceBlocksFeed is the trace blocks feed type (only for BSC)
const TraceBlocksFeed bxtypes.FeedType = "traceBlocks"

// AllFeedTypes lists every feed type declared in this package. Components pass it (optionally
// extended with their own feed types) to the feed manager, which gives every listed type its own
// notification channel so that a burst on one feed cannot starve the others.
var AllFeedTypes = []bxtypes.FeedType{
	bxtypes.NewTxsFeed,
	bxtypes.PendingTxsFeed,
	bxtypes.BDNBlocksFeed,
	bxtypes.NewBlocksFeed,
	bxtypes.OnBlockFeed,
	bxtypes.TxReceiptsFeed,
	TraceBlocksFeed,
	bxtypes.NewBeaconBlocksFeed,
	bxtypes.BDNBeaconBlocksFeed,
}
