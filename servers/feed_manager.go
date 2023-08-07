package servers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/sourcegraph/jsonrpc2"
)

const (
	pendingNextValidatorListInitCapacity = 100
)

// ClientSubscription contains client subscription feed and connection
type ClientSubscription struct {
	feed               chan types.Notification
	feedType           types.FeedType
	feedConnectionType types.FeedConnectionType
	connection         *jsonrpc2.Conn
	tier               sdnmessage.AccountTier
	network            types.NetworkNum
	accountID          types.AccountID
	remoteAddress      string
	filters            string
	includes           string
	project            string
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

// PendingNextValidatorTxInfo holds info needed to reevaluate next validator tx when next block published
type PendingNextValidatorTxInfo struct {
	Tx            *bxmessage.Tx
	Fallback      uint16
	TimeOfRequest time.Time
	Source        connections.Conn
}

// FeedManager - feed manager fields
type FeedManager struct {
	feed                                chan types.Notification
	idToClientSubscription              map[string]ClientSubscription
	subscriptionServices                services.SubscriptionServices
	lock                                sync.RWMutex
	node                                connections.BxListener
	networkNum                          types.NetworkNum
	chainID                             types.NetworkID
	nodeID                              types.NodeID
	nodeWSManager                       blockchain.WSManager
	accountModel                        sdnmessage.Account
	getCustomerAccountModel             func(types.AccountID) (sdnmessage.Account, error)
	certFile                            string
	keyFile                             string
	cfg                                 config.Bx
	log                                 *log.Entry
	nextValidatorMap                    *orderedmap.OrderedMap
	validatorStatusMap                  *syncmap.SyncMap[string, bool]
	pendingBSCNextValidatorTxHashToInfo map[string]PendingNextValidatorTxInfo
	pendingBSCNextValidatorTxsMapLock   sync.Mutex

	context context.Context
	cancel  context.CancelFunc
	stats   statistics.Stats
}

// NewFeedManager - create a new feedManager
func NewFeedManager(parent context.Context, node connections.BxListener, wsFeedChan chan types.Notification,
	subscriptionServices services.SubscriptionServices,
	networkNum types.NetworkNum, networkID types.NetworkID, nodeID types.NodeID,
	wsManager blockchain.WSManager,
	accountModel sdnmessage.Account, getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error),
	certFile string, keyFile string, cfg config.Bx, stats statistics.Stats,
	nextValidatorMap *orderedmap.OrderedMap, validatorStatusMap *syncmap.SyncMap[string, bool]) *FeedManager {
	ctx, cancel := context.WithCancel(parent)
	logger := log.WithFields(log.Fields{
		"component": "feedManager",
	})

	newServer := &FeedManager{
		feed:                                wsFeedChan,
		idToClientSubscription:              make(map[string]ClientSubscription),
		subscriptionServices:                subscriptionServices,
		node:                                node,
		networkNum:                          networkNum,
		chainID:                             networkID,
		nodeID:                              nodeID,
		nodeWSManager:                       wsManager,
		accountModel:                        accountModel,
		getCustomerAccountModel:             getCustomerAccountModel,
		nextValidatorMap:                    nextValidatorMap,
		validatorStatusMap:                  validatorStatusMap,
		certFile:                            certFile,
		keyFile:                             keyFile,
		cfg:                                 cfg,
		context:                             ctx,
		cancel:                              cancel,
		stats:                               stats,
		log:                                 logger,
		pendingBSCNextValidatorTxHashToInfo: make(map[string]PendingNextValidatorTxInfo),
	}
	return newServer
}

// Start - start feed manager
func (f *FeedManager) Start() error {
	go f.run()
	return nil
}

func (f *FeedManager) checkForDuplicateFeed(clientSubscription *ClientSubscription, remoteAddress string) error {
	// feeds check should not be tested for customer running local gateway
	if clientSubscription.accountID == f.accountModel.AccountID {
		return nil
	}

	remoteIP := strings.Split(remoteAddress, ":")[0] + ":"
	f.lock.RLock()
	defer f.lock.RUnlock()
	for k, v := range f.idToClientSubscription {
		if v.accountID == clientSubscription.accountID {
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
	return nil
}

// Subscribe - subscribe a client to a desired feed
func (f *FeedManager) Subscribe(feedName types.FeedType, feedConnectionType types.FeedConnectionType, conn *jsonrpc2.Conn, accountTier sdnmessage.AccountTier, accountID types.AccountID, remoteAddress string, filters string, includes string, project string, ethSubscribe bool) (*ClientSubscriptionHandlingInfo, error) {
	id := f.subscriptionServices.GenerateSubscriptionID(ethSubscribe)
	clientSubscription := ClientSubscription{
		feed:               make(chan types.Notification, bxgateway.BxNotificationChannelSize),
		feedType:           feedName,
		feedConnectionType: feedConnectionType,
		connection:         conn,
		tier:               accountTier,
		network:            f.networkNum,
		accountID:          accountID,
		remoteAddress:      remoteAddress,
		includes:           includes,
		filters:            filters,
		project:            project,
		timeOpenedFeed:     time.Now(),
		errMsgChan:         make(chan string, 1),
	}

	if err := f.checkForDuplicateFeed(&clientSubscription, remoteAddress); err != nil {
		f.log.Error(err)
		return nil, err
	}

	subscriptionModel := sdnmessage.SubscriptionModel{
		SubscriptionID: id,
		SubscriberIP:   strings.Split(remoteAddress, ":")[0],
		NodeID:         string(f.nodeID),
		AccountID:      accountID,
		NetworkNum:     f.networkNum,
		FeedType:       feedName,
	}

	allowed, reason, permissionRespChannel := f.subscriptionServices.IsSubscriptionAllowed(&subscriptionModel)
	log.Debugf("subscription %v: allowed %v, reason %v", id, allowed, reason)
	if !allowed {
		return nil, fmt.Errorf(reason)
	}

	f.lock.Lock()
	f.idToClientSubscription[id] = clientSubscription
	f.lock.Unlock()

	f.log.Infof("%v subscribed to %v id %v with includes [%v] and filter [%v]", remoteAddress, feedName, id, includes, filters)

	handlingInfo := ClientSubscriptionHandlingInfo{
		SubscriptionID:     id,
		FeedChan:           clientSubscription.feed,
		ErrMsgChan:         clientSubscription.errMsgChan,
		PermissionRespChan: permissionRespChannel,
	}
	return &handlingInfo, nil
}

// Unsubscribe    - unsubscribe a client from feed and optionally closes the corresponding client ws connection
func (f *FeedManager) Unsubscribe(subscriptionID string, closeClientConnection bool, errMsg string) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	clientSub, exists := f.idToClientSubscription[subscriptionID]
	if !exists {
		f.log.Warnf("attempting to unsubscribe from %v failed: subscription not found", subscriptionID)
		return fmt.Errorf("subscription %v was not found", subscriptionID)
	}

	f.log.Infof("unsubscribing %v from %v, closing the connection: %v", subscriptionID, clientSub.remoteAddress, closeClientConnection)

	if errMsg != "" {
		clientSub.errMsgChan <- errMsg
	}

	subscription := sdnmessage.SubscriptionModel{
		SubscriptionID: subscriptionID,
		SubscriberIP:   strings.Split(clientSub.remoteAddress, ":")[0],
		NodeID:         string(f.nodeID),
		AccountID:      clientSub.accountID,
		NetworkNum:     clientSub.network,
		FeedType:       clientSub.feedType,
	}
	f.subscriptionServices.SendUnsubscribeNotification(&subscription)

	f.stats.LogUnsubscribeStats(
		subscriptionID,
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
	copyIDToClientSubscription := make(map[string]ClientSubscription)
	for k, v := range f.idToClientSubscription {
		copyIDToClientSubscription[k] = v
	}
	f.lock.Unlock()

	for subscriptionID := range copyIDToClientSubscription {
		_ = f.Unsubscribe(subscriptionID, true, "")
	}
}

// run - getting feed notification and pass to client via common channel
func (f *FeedManager) run() {
	defer f.cancel()
	f.log.Infof("feedManager is Starting for network %v", f.networkNum)
	defer f.log.Infof("feedManager stopped for network %v", f.networkNum)

	for {
		select {
		case notification, ok := <-f.feed:
			if !ok {
				f.log.Errorf("can't pull from ws feed channel. Terminating")
				break
			}
			f.lock.RLock()
			for uid, clientSub := range f.idToClientSubscription {
				if (clientSub.feedConnectionType == types.WebSocketFeed || clientSub.feedConnectionType == types.GRPCFeed) && clientSub.feedType == notification.NotificationType() {
					select {
					case clientSub.feed <- notification:
						// Offer: I took this out as we are locking the map in read and can't write.
						// also, do we need to update the map after we update the counter?
						// if entry, ok := f.idToClientSubscription[uid]; ok {
						//	entry.messagesSent++
						//	f.idToClientSubscription[uid] = entry
						// }
					default:
						f.log.Errorf("can't send %v to channel %v without blocking. Ignored hash %v and unsubscribing", clientSub.feedType, uid, notification.GetHash())
						go func(subscriptionID string) {
							// running as go-routine since we are holding the lock. Closing the connection since we can't write
							if err := f.Unsubscribe(subscriptionID, true, ""); err != nil {
								f.log.Debugf("unable to Unsubscribe %v - %v", subscriptionID, err)
							}
							// TODO: mark clientSub as "being closed" to prevent multiple Unsubscribe
						}(uid)
					}
				}
			}
			f.lock.RUnlock()
		}
	}
}

// SubscriptionExists - check if subscription exists
func (f *FeedManager) SubscriptionExists(subscriptionID string) bool {
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

// GetAllSubscriptions returns all subscriptions
func (f *FeedManager) GetAllSubscriptions() []sdnmessage.SubscriptionModel {
	f.lock.RLock()
	defer f.lock.RUnlock()
	subscriptionModels := make([]sdnmessage.SubscriptionModel, len(f.idToClientSubscription))
	i := 0
	for id, sub := range f.idToClientSubscription {
		subscriptionModel := sdnmessage.SubscriptionModel{
			SubscriptionID: id,
			SubscriberIP:   strings.Split(sub.remoteAddress, ":")[0],
			NodeID:         string(f.nodeID),
			AccountID:      sub.accountID,
			NetworkNum:     sub.network,
			FeedType:       sub.feedType,
		}
		subscriptionModels[i] = subscriptionModel
		i++
	}
	return subscriptionModels
}

// GetPendingNextValidatorTxs returns map of pending next validator transactions
func (f *FeedManager) GetPendingNextValidatorTxs() map[string]PendingNextValidatorTxInfo {
	return f.pendingBSCNextValidatorTxHashToInfo
}

// LockPendingNextValidatorTxs activates mutex lock for pendingBSCNextValidatorTxHashToInfo map
func (f *FeedManager) LockPendingNextValidatorTxs() {
	f.pendingBSCNextValidatorTxsMapLock.Lock()
}

// UnlockPendingNextValidatorTxs activates mutex lock for pendingBSCNextValidatorTxHashToInfo map
func (f *FeedManager) UnlockPendingNextValidatorTxs() {
	f.pendingBSCNextValidatorTxsMapLock.Unlock()
}

func (f *FeedManager) getSyncedWSProvider(preferredProviderEndpoint *types.NodeEndpoint) (blockchain.WSProvider, bool) {
	if !f.nodeWSManager.Synced() {
		return nil, false
	}
	nodeWS, ok := f.nodeWSManager.Provider(preferredProviderEndpoint)
	if !ok || nodeWS.SyncStatus() != blockchain.Synced {
		nodeWS, ok = f.nodeWSManager.SyncedProvider()
	}
	return nodeWS, ok
}