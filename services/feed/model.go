package feed

import (
	"io"
	"sync/atomic"
	"time"

	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// ClientSubscription contains client subscription feed and connection
type ClientSubscription struct {
	types.ClientInfo
	types.ReqOptions
	feed               chan types.Notification
	feedType           bxtypes.FeedType
	feedConnectionType bxtypes.FeedConnectionType
	connection         io.Closer
	network            bxtypes.NetworkNum
	timeOpenedFeed     time.Time
	messagesSent       uint64
	errMsgChan         chan string
	unsubscribing      *atomic.Bool
}

// ClientSubscriptionHandlingInfo contains all info needed by subscription handler
type ClientSubscriptionHandlingInfo struct {
	SubscriptionID     string
	FeedChan           chan types.Notification
	ErrMsgChan         chan string
	PermissionRespChan chan *sdnmessage.SubscriptionPermissionMessage
}

// ClientSubscriptionFullInfo contains full info about client subscription
type ClientSubscriptionFullInfo struct {
	AccountID    bxtypes.AccountID
	FeedName     bxtypes.FeedType
	Network      bxtypes.NetworkNum
	RemoteAddr   string
	Include      string
	Filter       string
	Age          uint64
	MessagesSent uint64
	ConnType     bxtypes.FeedConnectionType
}

// ErrorNotification info about error notification
type ErrorNotification struct {
	ErrorMsg string
	FeedType bxtypes.FeedType
}
