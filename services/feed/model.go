package feed

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/sourcegraph/jsonrpc2"
)

// ClientSubscription contains client subscription feed and connection
type ClientSubscription struct {
	types.ClientInfo
	types.ReqOptions
	feed               chan types.Notification
	feedType           types.FeedType
	feedConnectionType types.FeedConnectionType
	connection         *jsonrpc2.Conn
	network            types.NetworkNum
	timeOpenedFeed     time.Time
	messagesSent       uint64
	errMsgChan         chan string
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
	AccountID    types.AccountID
	Tier         string
	FeedName     types.FeedType
	Network      types.NetworkNum
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