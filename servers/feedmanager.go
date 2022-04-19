package servers

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/config"
	"github.com/bloXroute-Labs/gateway/connections"
	log "github.com/bloXroute-Labs/gateway/logger"
	pb "github.com/bloXroute-Labs/gateway/protobuf"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/services/statistics"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/satori/go.uuid"
	"github.com/sourcegraph/jsonrpc2"
	"sync"
)

// ClientSubscription contains client subscription feed and websocket connection
type ClientSubscription struct {
	feed       chan *types.Notification
	feedType   types.FeedType
	connection *jsonrpc2.Conn
	tier       sdnmessage.AccountTier
	network    types.NetworkNum
	accountID  types.AccountID
}

// FeedManager - feed manager fields
type FeedManager struct {
	feedChan                chan types.Notification
	idToClientSubscription  map[uuid.UUID]ClientSubscription
	lock                    sync.RWMutex
	node                    connections.BxListener
	networkNum              types.NetworkNum
	nodeWSManager           blockchain.WSManager
	accountModel            sdnmessage.Account
	getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error)
	certFile                string
	keyFile                 string
	cfg                     config.Bx

	context context.Context
	cancel  context.CancelFunc
	stats   statistics.Stats
}

// NewFeedManager    - create a new feedManager
func NewFeedManager(parent context.Context, node connections.BxListener, feedChan chan types.Notification,
	networkNum types.NetworkNum, wsManager blockchain.WSManager,
	accountModel sdnmessage.Account, getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error),
	certFile string, keyFile string, cfg config.Bx, stats statistics.Stats) *FeedManager {
	ctx, cancel := context.WithCancel(parent)
	newServer := &FeedManager{
		feedChan:                feedChan,
		idToClientSubscription:  make(map[uuid.UUID]ClientSubscription),
		node:                    node,
		networkNum:              networkNum,
		nodeWSManager:           wsManager,
		accountModel:            accountModel,
		getCustomerAccountModel: getCustomerAccountModel,
		certFile:                certFile,
		keyFile:                 keyFile,
		cfg:                     cfg,
		context:                 ctx,
		cancel:                  cancel,
		stats:                   stats,
	}
	return newServer
}

// Start - start feed manager
func (f *FeedManager) Start() error {
	defer f.cancel()

	ch := clientHandler{
		feedManager:     f,
		websocketServer: nil,
		httpServer:      NewHTTPServer(f, f.cfg.HTTPPort),
	}
	go ch.manageWSServer(f.cfg.ManageWSServer)
	go ch.manageHTTPServer(f.context)

	f.run()
	return nil
}

// Subscribe    - subscribe a client to a desired feed
func (f *FeedManager) Subscribe(feedName types.FeedType, conn *jsonrpc2.Conn, accountTier sdnmessage.AccountTier, accountID types.AccountID) (*uuid.UUID, *chan *types.Notification, error) {
	if !types.Exists(feedName, availableFeeds) {
		err := fmt.Errorf("got unsupported feed name %v", feedName)
		log.Error(err.Error())
		return nil, nil, err
	}

	id := uuid.NewV4()
	clientSubscription := ClientSubscription{
		feed:       make(chan *types.Notification, bxgateway.BxNotificationChannelSize),
		feedType:   feedName,
		connection: conn,
		tier:       accountTier,
		network:    f.networkNum,
		accountID:  accountID,
	}

	f.lock.Lock()
	f.idToClientSubscription[id] = clientSubscription
	f.lock.Unlock()

	return &id, &clientSubscription.feed, nil
}

// Unsubscribe    - unsubscribe a client from feed and optionally closes the corresponding client ws connection
func (f *FeedManager) Unsubscribe(subscriptionID uuid.UUID, closeClientConnection bool) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	clientSub, exists := f.idToClientSubscription[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %v was not found", subscriptionID)
	}
	f.stats.LogUnsubscribeStats(
		&subscriptionID,
		clientSub.feedType,
		f.networkNum,
		clientSub.accountID,
		clientSub.tier)
	close(clientSub.feed)
	delete(f.idToClientSubscription, subscriptionID)
	if closeClientConnection {
		err := clientSub.connection.Close()
		if err != nil && err != jsonrpc2.ErrClosed {
			return fmt.Errorf("encountered error closing websocket connection with ID %v", subscriptionID)
		}
	}
	return nil
}

// CloseAllClientConnections - unsubscribes all client subscriptions and closes all client ws connections
func (f *FeedManager) CloseAllClientConnections() {
	for subscriptionID := range f.idToClientSubscription {
		err := f.Unsubscribe(subscriptionID, true)
		if err != nil {
			log.Errorf("failed to unsubscribe subscription with ID %v: %v", subscriptionID, err)
		}
	}
}

// run - getting newTx or pendingTx and pass to client via common channel
func (f *FeedManager) run() {
	for {
		notification, ok := <-f.feedChan
		if !ok {
			log.Errorf("feed manager can not pull from feed channel")
			break
		}
		f.lock.RLock()
		for uid, clientSub := range f.idToClientSubscription {
			if clientSub.feedType == notification.NotificationType() {
				select {
				case clientSub.feed <- &notification:
				default:
					log.Warnf("can't send %v to channel %v without blocking. Ignored hash %v", clientSub.feedType, uid, notification.GetHash())
				}
			}

		}
		f.lock.RUnlock()
	}
}

// subscriptionExists - check if subscription exists
func (f *FeedManager) subscriptionExists(subscriptionID uuid.UUID) bool {
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
			AccountId: string(clientData.accountID),
			Tier:      string(clientData.tier),
			FeedName:  string(clientData.feedType),
			Network:   uint32(clientData.network),
		}
		resp.Subscriptions = append(resp.Subscriptions, subscribe)
	}
	return resp
}
