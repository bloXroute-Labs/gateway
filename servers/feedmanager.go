package servers

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"sync"
)

// FeedManager - feed manager fields
type FeedManager struct {
	feedChan                      chan types.Notification
	subscriptionIDToOutputChannel map[uuid.UUID]map[types.FeedType]*chan *types.Notification
	wg                            *sync.WaitGroup
	lock                          sync.RWMutex
	addr                          string
	node                          connections.BxListener
	networkNum                    types.NetworkNum
	accountModel                  sdnmessage.Account
	getCustomerAccountModel       func(types.AccountID) (sdnmessage.Account, error)
}

// NewFeedManager    - create a new feedManager
func NewFeedManager(node connections.BxListener, feedChan chan types.Notification,
	wg *sync.WaitGroup, addr string, networkNum types.NetworkNum,
	accountModel sdnmessage.Account, getCustomerAccountModel func(types.AccountID) (sdnmessage.Account, error)) *FeedManager {
	newServer := &FeedManager{
		feedChan:                      feedChan,
		subscriptionIDToOutputChannel: make(map[uuid.UUID]map[types.FeedType]*chan *types.Notification),
		wg:                            wg,
		addr:                          addr,
		node:                          node,
		networkNum:                    networkNum,
		accountModel:                  accountModel,
		getCustomerAccountModel:       getCustomerAccountModel,
	}
	return newServer
}

// Start - start feed manager
func (f *FeedManager) Start() error {
	log.Infof("starting feed provider on addr: %v", f.addr)
	ch := clientHandler{
		feedManager: f,
		wg:          f.wg,
	}
	go ch.runWSServer()
	f.run()
	return nil
}

// Subscribe    - subscribe a client to a desired feed
func (f *FeedManager) Subscribe(feedName types.FeedType) (*uuid.UUID, *chan *types.Notification, error) {
	if !types.Exists(feedName, availableFeeds) {
		err := fmt.Errorf("got unsupported feed name %v", feedName)
		log.Error(err.Error())
		return nil, nil, err
	}

	id := uuid.NewV1()
	subscriptionFeed := make(chan *types.Notification, bxgateway.BxNotificationChannelSize)

	f.lock.Lock()
	defer f.lock.Unlock()
	f.subscriptionIDToOutputChannel[id] = make(map[types.FeedType]*chan *types.Notification)
	f.subscriptionIDToOutputChannel[id][feedName] = &subscriptionFeed
	return &id, &subscriptionFeed, nil
}

// Unsubscribe    - unsubscribe a client from feed
func (f *FeedManager) Unsubscribe(subscriptionID uuid.UUID) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	if subscriptionChan, exists := f.subscriptionIDToOutputChannel[subscriptionID]; exists {
		for feedName := range subscriptionChan {
			delete(f.subscriptionIDToOutputChannel, subscriptionID)
			close(*subscriptionChan[feedName])

		}
		return nil
	}
	return fmt.Errorf("subscription %v was not found", subscriptionID)
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
		for uid, feeds := range f.subscriptionIDToOutputChannel {
			for feed, feedChan := range feeds {
				if feed == notification.NotificationType() {
					select {
					case *feedChan <- &notification:
					default:
						log.Warnf("can't send %v to channel %v without blocking. Ignored hash %v", feed, uid, notification.GetHash())
					}
				}
			}
		}
		f.lock.RUnlock()
	}
}

func (f *FeedManager) subscriptionExists(subscriptionID uuid.UUID) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if _, exists := f.subscriptionIDToOutputChannel[subscriptionID]; exists {
		return true
	}
	return false
}
