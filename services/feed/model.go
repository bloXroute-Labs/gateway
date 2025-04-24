package feed

import (
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// ClientSubscription contains client subscription feed and connection
type ClientSubscription struct {
	types.ClientInfo
	types.ReqOptions
	feed               chan types.Notification
	feedType           types.FeedType
	feedConnectionType types.FeedConnectionType
	connection         *jsonrpc2.Conn
	network            bxtypes.NetworkNum
	timeOpenedFeed     time.Time
	messagesSent       uint64
	errMsgChan         chan string
}

// ClientSubscriptionHandlingInfo contains all info needed by subscription handler
type ClientSubscriptionHandlingInfo struct {
	SubscriptionID     string
	FeedChan           chan types.Notification
	ErrMsgChan         chan string
	PermissionRespChan chan *types.SubscriptionPermissionMessage
}

// ClientSubscriptionFullInfo contains full info about client subscription
type ClientSubscriptionFullInfo struct {
	AccountID    bxtypes.AccountID
	Tier         string
	FeedName     types.FeedType
	Network      bxtypes.NetworkNum
	RemoteAddr   string
	Include      string
	Filter       string
	Age          uint64
	MessagesSent uint64
	ConnType     types.FeedConnectionType
}

// ErrorNotification info about error notification
type ErrorNotification struct {
	ErrorMsg string
	FeedType types.FeedType
}
