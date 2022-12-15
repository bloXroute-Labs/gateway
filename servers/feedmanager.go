package servers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	uuid "github.com/satori/go.uuid"
	"github.com/sourcegraph/jsonrpc2"
)

// ClientSubscription contains client subscription feed and websocket connection
type ClientSubscription struct {
	feed           chan *types.Notification
	feedType       types.FeedType
	connection     *jsonrpc2.Conn
	tier           sdnmessage.AccountTier
	network        types.NetworkNum
	accountID      types.AccountID
	remoteAddress  string
	filters        string
	includes       string
	project        string
	timeOpenedFeed time.Time
	messagesSent   uint64
}

// FeedManager - feed manager fields
type FeedManager struct {
	feedChan                chan types.Notification
	idToClientSubscription  map[uuid.UUID]ClientSubscription
	lock                    sync.RWMutex
	node                    connections.BxListener
	networkNum              types.NetworkNum
	chainID                 types.NetworkID
	nodeWSManager           blockchain.WSManager
	accountModel            sdnmessage.Account
	getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error)
	certFile                string
	keyFile                 string
	cfg                     config.Bx
	log                     *log.Entry
	nextValidatorMap        *orderedmap.OrderedMap

	context context.Context
	cancel  context.CancelFunc
	stats   statistics.Stats
}

// NewFeedManager - create a new feedManager
func NewFeedManager(parent context.Context, node connections.BxListener, feedChan chan types.Notification,
	networkNum types.NetworkNum, networkID types.NetworkID, wsManager blockchain.WSManager,
	accountModel sdnmessage.Account, getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error),
	certFile string, keyFile string, cfg config.Bx, stats statistics.Stats, nextValidatorMap *orderedmap.OrderedMap) *FeedManager {
	ctx, cancel := context.WithCancel(parent)
	logger := log.WithFields(log.Fields{
		"component": "feedManager",
	})

	newServer := &FeedManager{
		feedChan:                feedChan,
		idToClientSubscription:  make(map[uuid.UUID]ClientSubscription),
		node:                    node,
		networkNum:              networkNum,
		chainID:                 networkID,
		nodeWSManager:           wsManager,
		accountModel:            accountModel,
		getCustomerAccountModel: getCustomerAccountModel,
		nextValidatorMap:        nextValidatorMap,
		certFile:                certFile,
		keyFile:                 keyFile,
		cfg:                     cfg,
		context:                 ctx,
		cancel:                  cancel,
		stats:                   stats,
		log:                     logger,
	}
	return newServer
}

// Start - start feed manager
func (f *FeedManager) Start() error {
	go f.run()
	return nil
}

func (f *FeedManager) checkFeedsLimit(clientSubscription *ClientSubscription, remoteAddress string) error {
	// feeds check should not be tested for customer running local gateway
	if clientSubscription.accountID == f.accountModel.AccountID {
		return nil
	}

	allowFeeds := f.accountModel.NewTransactionStreaming.Feed.Limit
	remoteIP := strings.Split(remoteAddress, ":")[0] + ":"

	f.lock.RLock()
	defer f.lock.RUnlock()
	activeFeeds := 0
	for k, v := range f.idToClientSubscription {
		if v.accountID == clientSubscription.accountID {
			activeFeeds++
			if v.feedType == clientSubscription.feedType &&
				v.network == clientSubscription.network &&
				v.includes == clientSubscription.includes &&
				v.filters == clientSubscription.filters &&
				v.project == clientSubscription.project &&
				strings.HasPrefix(v.remoteAddress, remoteIP) {
				return fmt.Errorf("duplicate feed request - account %v tier %v ip %v previous subscription ID %v", clientSubscription.accountID, clientSubscription.tier, remoteAddress, k)
			}
		}
	}
	if activeFeeds >= allowFeeds {
		return fmt.Errorf("account %v tier %v exceeded max number of allowed feeds %v", clientSubscription.accountID, clientSubscription.tier, allowFeeds)
	}
	return nil
}

// Subscribe - subscribe a client to a desired feed
func (f *FeedManager) Subscribe(feedName types.FeedType, conn *jsonrpc2.Conn, accountTier sdnmessage.AccountTier, accountID types.AccountID, remoteAddress string, filters string, includes string, project string) (*uuid.UUID, *chan *types.Notification, error) {
	id := uuid.NewV4()
	clientSubscription := ClientSubscription{
		feed:           make(chan *types.Notification, bxgateway.BxNotificationChannelSize),
		feedType:       feedName,
		connection:     conn,
		tier:           accountTier,
		network:        f.networkNum,
		accountID:      accountID,
		remoteAddress:  remoteAddress,
		includes:       includes,
		filters:        filters,
		project:        project,
		timeOpenedFeed: time.Now(),
	}

	if err := f.checkFeedsLimit(&clientSubscription, remoteAddress); err != nil {
		f.log.Error(err)
		return nil, nil, err
	}

	f.lock.Lock()
	f.idToClientSubscription[id] = clientSubscription
	f.lock.Unlock()

	f.log.Infof("%v subscribed to %v id %v with includes [%v] and filter [%v]", remoteAddress, feedName, id, includes, filters)

	return &id, &clientSubscription.feed, nil
}

// Unsubscribe    - unsubscribe a client from feed and optionally closes the corresponding client ws connection
func (f *FeedManager) Unsubscribe(subscriptionID uuid.UUID, closeClientConnection bool) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	clientSub, exists := f.idToClientSubscription[subscriptionID]
	if !exists {
		f.log.Warnf("attempting to unsubscribe from %v failed - not found", subscriptionID)
		return fmt.Errorf("subscription %v was not found", subscriptionID)
	}
	f.log.Infof("unsubscribing %v from %v. close connection %v", subscriptionID, clientSub.remoteAddress, closeClientConnection)

	f.stats.LogUnsubscribeStats(
		&subscriptionID,
		clientSub.feedType,
		f.networkNum,
		clientSub.accountID,
		clientSub.tier)
	close(clientSub.feed)
	delete(f.idToClientSubscription, subscriptionID)
	if closeClientConnection && clientSub.connection != nil {
		// TODO: need to unsubscribe all other subscriptions on this connection.
		err := clientSub.connection.Close()
		if err != nil && err != jsonrpc2.ErrClosed {
			f.log.Warnf("failed to close connection for %v - %v", subscriptionID, err)
			return fmt.Errorf("encountered error closing websocket connection with ID %v", subscriptionID)
		}
	}

	return nil
}

// CloseAllClientConnections - unsubscribes all client subscriptions and closes all client ws connections
func (f *FeedManager) CloseAllClientConnections() {
	// copy the map, since Unsubscribe has a lock inside
	f.lock.Lock()
	copyIDToClientSubscription := make(map[uuid.UUID]ClientSubscription)
	for k, v := range f.idToClientSubscription {
		copyIDToClientSubscription[k] = v
	}
	f.lock.Unlock()

	for subscriptionID := range copyIDToClientSubscription {
		_ = f.Unsubscribe(subscriptionID, true)
	}
}

// run - getting newTx or pendingTx and pass to client via common channel
func (f *FeedManager) run() {
	defer f.cancel()
	f.log.Infof("feedManager is Starting for network %v", f.networkNum)
	for {
		notification, ok := <-f.feedChan
		if !ok {
			f.log.Errorf("can't pull from feed channel. Terminating")
			break
		}
		f.lock.RLock()
		for uid, clientSub := range f.idToClientSubscription {
			if clientSub.feedType == notification.NotificationType() {
				select {
				case clientSub.feed <- &notification:
					// Offer: I took this out as we are locking the map in read and can't write.
					// also, do we need to update the map after we update the counter?
					//if entry, ok := f.idToClientSubscription[uid]; ok {
					//	entry.messagesSent++
					//	f.idToClientSubscription[uid] = entry
					//}
				default:
					f.log.Errorf("can't send %v to channel %v without blocking. Ignored hash %v and unsubscribing", clientSub.feedType, uid, notification.GetHash())
					go func(subscriptionID uuid.UUID) {
						// running as go-routine since we are holding the lock. Closing the connection since we can't write
						if err := f.Unsubscribe(subscriptionID, true); err != nil {
							f.log.Debugf("unable to Unsubscribe %v - %v", subscriptionID, err)
						}
						// TODO: mark clientSub as "being closed" to prevent multiple Unsubscribe
					}(uid)
				}
			}

		}
		f.lock.RUnlock()
	}
	f.log.Infof("feedManager stopped for network %v", f.networkNum)
}

// SubscriptionExists - check if subscription exists
func (f *FeedManager) SubscriptionExists(subscriptionID uuid.UUID) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if _, exists := f.idToClientSubscription[subscriptionID]; exists {
		return true
	}
	return false
}

// NeedBlocks checks if feedManager should receive block notifications
func (f *FeedManager) NeedBlocks() bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	for _, clientSub := range f.idToClientSubscription {
		if clientSub.feedType != types.NewTxsFeed && clientSub.feedType != types.PendingTxsFeed {
			return true
		}
	}
	return false
}

// GetGrpcSubscriptionReply - return gRPC subscription reply
func (f *FeedManager) GetGrpcSubscriptionReply() *pb.SubscriptionsReply {
	resp := &pb.SubscriptionsReply{}
	f.lock.RLock()
	defer f.lock.RUnlock()
	for _, clientData := range f.idToClientSubscription {
		subscribe := &pb.Subscription{
			AccountId:    string(clientData.accountID),
			Tier:         string(clientData.tier),
			FeedName:     string(clientData.feedType),
			Network:      uint32(clientData.network),
			RemoteAddr:   clientData.remoteAddress,
			Include:      clientData.includes,
			Filter:       clientData.filters,
			Age:          uint64(time.Since(clientData.timeOpenedFeed).Seconds()),
			MessagesSent: clientData.messagesSent,
		}
		resp.Subscriptions = append(resp.Subscriptions, subscribe)
	}
	return resp
}

// GetNumberOfSubscriptionsForAccount - returns the number of existing subscriptions for an account
func (f *FeedManager) GetNumberOfSubscriptionsForAccount(account types.AccountID) int {
	f.lock.RLock()
	defer f.lock.RUnlock()
	numberOfSubscriptions := 0
	for _, clientSub := range f.idToClientSubscription {
		if clientSub.accountID == account {
			numberOfSubscriptions++
		}
	}
	return numberOfSubscriptions
}

// GetFeedsForAccount - returns a list of types.FeedType for the existing subscriptions of an account
func (f *FeedManager) GetFeedsForAccount(account types.AccountID) []types.FeedType {
	f.lock.RLock()
	defer f.lock.RUnlock()
	subscriptions := make([]types.FeedType, 0)
	for _, clientSub := range f.idToClientSubscription {
		if clientSub.accountID == account {
			subscriptions = append(subscriptions, clientSub.feedType)
		}
	}
	return subscriptions
}
