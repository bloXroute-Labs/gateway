package servers

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/jsonrpc2"
	"sync"
)

// ClientSubscription contains client subscription feed and websocket connection
type ClientSubscription struct {
	feed       chan *types.Notification
	feedType   types.FeedType
	connection *jsonrpc2.Conn
}

// FeedManager - feed manager fields
type FeedManager struct {
	feedChan                chan types.Notification
	idToClientSubscription  map[uuid.UUID]ClientSubscription
	lock                    sync.RWMutex
	addr                    string
	node                    connections.BxListener
	networkNum              types.NetworkNum
	blockchainWS            blockchain.WSProvider
	manageWSServer          bool
	accountModel            sdnmessage.Account
	getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error)
	websocketTLSEnabled     bool
	certFile                string
	keyFile                 string

	context context.Context
	cancel  context.CancelFunc
}

// NewFeedManager    - create a new feedManager
func NewFeedManager(parent context.Context, node connections.BxListener, feedChan chan types.Notification,
	addr string, networkNum types.NetworkNum, ws blockchain.WSProvider, manageWSServer bool,
	accountModel sdnmessage.Account, getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error),
	websocketTLSEnabled bool, certFile string, keyFile string) *FeedManager {
	ctx, cancel := context.WithCancel(parent)
	newServer := &FeedManager{
		feedChan:                feedChan,
		idToClientSubscription:  make(map[uuid.UUID]ClientSubscription),
		addr:                    addr,
		node:                    node,
		networkNum:              networkNum,
		blockchainWS:            ws,
		manageWSServer:          manageWSServer,
		accountModel:            accountModel,
		getCustomerAccountModel: getCustomerAccountModel,
		websocketTLSEnabled:     websocketTLSEnabled,
		certFile:                certFile,
		keyFile:                 keyFile,
		context:                 ctx,
		cancel:                  cancel,
	}
	return newServer
}

// Start - start feed manager
func (f *FeedManager) Start() error {
	log.Infof("starting feed provider on addr: %v", f.addr)
	defer f.cancel()

	ch := clientHandler{
		feedManager: f,
		server:      NewWSServer(f),
	}
	go ch.runWSServer()
	if f.manageWSServer {
		go ch.manageWSServer()
	}
	f.run()
	return nil
}

// Subscribe    - subscribe a client to a desired feed
func (f *FeedManager) Subscribe(feedName types.FeedType, conn *jsonrpc2.Conn) (*uuid.UUID, *chan *types.Notification, error) {
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
	}

	f.lock.Lock()
	f.idToClientSubscription[id] = clientSubscription
	f.lock.Unlock()

	return &id, &clientSubscription.feed, nil
}

// Unsubscribe    - unsubscribe a client from feed
func (f *FeedManager) Unsubscribe(subscriptionID uuid.UUID) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	clientSub, exists := f.idToClientSubscription[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription %v was not found", subscriptionID)
	}
	close(clientSub.feed)
	delete(f.idToClientSubscription, subscriptionID)
	err := clientSub.connection.Close()
	if err != nil && err != jsonrpc2.ErrClosed {
		return fmt.Errorf("encountered error closing websocket connection with ID %v", subscriptionID)
	}
	return nil
}

// UnsubscribeAll    - unsubscribes all client subscriptions
func (f *FeedManager) UnsubscribeAll() {
	for subscriptionID := range f.idToClientSubscription {
		err := f.Unsubscribe(subscriptionID)
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

func (f *FeedManager) subscriptionExists(subscriptionID uuid.UUID) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if _, exists := f.idToClientSubscription[subscriptionID]; exists {
		return true
	}
	return false
}
