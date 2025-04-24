package feed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/sourcegraph/jsonrpc2"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/metrics"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const accountExpiredError = "Account expired, unsubscribe feed"

// Manager - feed manager fields
type Manager struct {
	feed                   chan types.Notification
	errFeed                chan ErrorNotification
	idToClientSubscription map[string]ClientSubscription
	subscriptionServices   services.SubscriptionServices
	lock                   sync.RWMutex
	networkNum             bxtypes.NetworkNum
	nodeID                 bxtypes.NodeID
	accountModel           sdnmessage.Account
	sdn                    sdnsdk.SDNHTTP
	log                    *log.Entry
	stats                  statistics.Stats
	sendNotifications      bool
}

// NewManager - create a new feedManager
func NewManager(sdn sdnsdk.SDNHTTP,
	subscriptionServices services.SubscriptionServices,
	accountModel sdnmessage.Account,
	stats statistics.Stats,
	blockchainNum bxtypes.NetworkNum,
	sendNotifications bool,
) *Manager {

	logger := log.WithFields(log.Fields{
		"component": "feedManager",
	})

	newServer := &Manager{
		feed:                   make(chan types.Notification, bxgateway.BxNotificationChannelSize),
		errFeed:                make(chan ErrorNotification, bxgateway.BxErrorNotificationChannelSize),
		idToClientSubscription: make(map[string]ClientSubscription),
		subscriptionServices:   subscriptionServices,
		networkNum:             blockchainNum,
		nodeID:                 sdn.NodeID(),
		accountModel:           accountModel,
		sdn:                    sdn,
		stats:                  stats,
		log:                    logger,
		sendNotifications:      sendNotifications,
	}

	return newServer
}

// Start - start feed manager
func (f *Manager) Start(ctx context.Context) {
	wg := sync.WaitGroup{}
	wg.Add(3)

	go func() {
		defer wg.Done()
		f.notifySubscribers(ctx)
	}()

	go func() {
		defer wg.Done()
		f.notifyErrors(ctx)
	}()

	go func() {
		defer wg.Done()
		f.midnightCleanup(ctx)
	}()

	wg.Wait()
}

// Notify sends a notification to the feed
func (f *Manager) Notify(notification types.Notification) {
	if !f.sendNotifications {
		return
	}

	metrics.IncrFeedNotification(uint32(f.networkNum), string(notification.NotificationType()), metrics.SdNotificationsCreated)
	select {
	case f.feed <- notification:
	default:
		f.log.Errorf("%v feed manager channel is full, ignoring %v notification type with hash %v",
			bxtypes.NetworkNumToBlockchainNetwork[f.networkNum], notification.NotificationType(), notification.GetHash())
	}
}

// NotifyError sends an error notification to the feed
func (f *Manager) NotifyError(notification ErrorNotification) {
	if !f.sendNotifications {
		return
	}

	select {
	case f.errFeed <- notification:
	default:
		f.log.Errorf("can't send error %v to feed channel without blocking. Ignored error %v", notification.FeedType, notification.ErrorMsg)
	}
}

// Subscribe - subscribe a client to a desired feed
func (f *Manager) Subscribe(feedName types.FeedType, feedConnectionType types.FeedConnectionType,
	conn *jsonrpc2.Conn, ci types.ClientInfo, ro types.ReqOptions, ethSubscribe bool) (*ClientSubscriptionHandlingInfo, error) {

	id := f.subscriptionServices.GenerateSubscriptionID(ethSubscribe)
	clientSubscription := ClientSubscription{
		feed:               make(chan types.Notification, bxgateway.BxNotificationChannelSize),
		feedType:           feedName,
		feedConnectionType: feedConnectionType,
		connection:         conn,
		network:            f.networkNum,
		timeOpenedFeed:     time.Now(),
		errMsgChan:         make(chan string, 1),
		ClientInfo:         ci,
		ReqOptions:         ro,
	}

	if err := f.checkForDuplicateFeed(&clientSubscription, ci.RemoteAddress); err != nil {
		f.log.Error(err)
		return nil, err
	}

	subscriptionModel := types.SubscriptionModel{
		SubscriptionID: id,
		SubscriberIP:   strings.Split(ci.RemoteAddress, ":")[0],
		NodeID:         string(f.nodeID),
		AccountID:      ci.AccountID,
		NetworkNum:     f.networkNum,
		FeedType:       feedName,
	}

	allowed, reason, permissionRespChannel := f.subscriptionServices.IsSubscriptionAllowed(&subscriptionModel)
	if !allowed {
		log.Debugf("subscription %v: allowed %v, reason %v", id, allowed, reason)
		return nil, fmt.Errorf(reason)
	}

	log.Tracef("subscription %v is allowed", id)

	f.lock.Lock()
	f.idToClientSubscription[id] = clientSubscription
	f.lock.Unlock()

	f.log.WithFields(log.Fields{
		"account_id":     ci.AccountID,
		"feed_name":      feedName,
		"remote_address": ci.RemoteAddress,
		"includes":       ro.Includes,
		"filters":        ro.Filters,
	}).Info("subscribing to feed")

	handlingInfo := ClientSubscriptionHandlingInfo{
		SubscriptionID:     id,
		FeedChan:           clientSubscription.feed,
		ErrMsgChan:         clientSubscription.errMsgChan,
		PermissionRespChan: permissionRespChannel,
	}

	return &handlingInfo, nil
}

// Unsubscribe - unsubscribe a client from feed and optionally closes the corresponding client ws connection
func (f *Manager) Unsubscribe(subscriptionID string, closeClientConnection bool, errMsg string) error {
	f.lock.Lock()

	clientSub, exists := f.idToClientSubscription[subscriptionID]
	if !exists {
		f.lock.Unlock()
		return fmt.Errorf("%w: %v", bxgateway.ErrSubscriptionNotFound, subscriptionID)
	}
	delete(f.idToClientSubscription, subscriptionID)
	f.lock.Unlock()

	f.log.WithFields(log.Fields{
		"account_id":     clientSub.AccountID,
		"feed_name":      clientSub.feedType,
		"remote_address": clientSub.RemoteAddress,
	}).Infof("unsubscribing from feed, closing the connection: %v", closeClientConnection)

	if errMsg != "" {
		clientSub.errMsgChan <- errMsg
	}

	subscription := types.SubscriptionModel{
		SubscriptionID: subscriptionID,
		SubscriberIP:   strings.Split(clientSub.RemoteAddress, ":")[0],
		NodeID:         string(f.nodeID),
		AccountID:      clientSub.AccountID,
		NetworkNum:     clientSub.network,
		FeedType:       clientSub.feedType,
	}
	f.subscriptionServices.SendUnsubscribeNotification(&subscription)

	// the gRPC feeds are logged by the interceptor
	if clientSub.MetaInfo[types.SDKVersionHeaderKey] != "" {
		f.stats.LogSDKInfo(
			clientSub.MetaInfo[types.SDKBlockchainHeaderKey],
			string(clientSub.feedType),
			clientSub.MetaInfo[types.SDKCodeLanguageHeaderKey],
			clientSub.MetaInfo[types.SDKVersionHeaderKey],
			clientSub.AccountID,
			types.WebSocketFeed,
			clientSub.timeOpenedFeed,
			time.Now(),
		)
	}

	f.stats.LogUnsubscribeStats(
		subscriptionID,
		clientSub.feedType,
		f.networkNum,
		clientSub.AccountID,
		sdnmessage.AccountTier(clientSub.Tier))
	close(clientSub.feed)
	if closeClientConnection && clientSub.connection != nil {
		// TODO: need to unsubscribe all other subscriptions on this connection.
		err := clientSub.connection.Close()
		if err != nil && !errors.Is(err, jsonrpc2.ErrClosed) {
			f.log.Warnf("failed to close connection for %v: %v", subscriptionID, err)
			return fmt.Errorf("encountered error closing websocket connection with ID %v", subscriptionID)
		}
	}

	return nil
}

// SubscriptionExists - check if subscription exists
func (f *Manager) SubscriptionExists(subscriptionID string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if _, exists := f.idToClientSubscription[subscriptionID]; exists {
		return true
	}
	return false
}

// SubscriptionTypeExists - check if subscription with specific type exists
func (f *Manager) SubscriptionTypeExists(feedType types.FeedType) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	for _, clientSub := range f.idToClientSubscription {
		if clientSub.feedType == feedType {
			return true
		}
	}
	return false
}

// NeedBlocks checks if feedManager should receive block notifications
func (f *Manager) NeedBlocks() bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	for _, clientSub := range f.idToClientSubscription {
		if clientSub.feedType != types.NewTxsFeed && clientSub.feedType != types.PendingTxsFeed {
			return true
		}
	}
	return false
}

// GetClientSubscriptionHandlingInfo returns all client subscriptions with channels
func (f *Manager) GetClientSubscriptionHandlingInfo() map[string]ClientSubscriptionHandlingInfo {
	f.lock.RLock()
	defer f.lock.RUnlock()

	subscriptions := make(map[string]ClientSubscriptionHandlingInfo)
	for id, clientSub := range f.idToClientSubscription {
		subscriptions[id] = ClientSubscriptionHandlingInfo{
			SubscriptionID: id,
			FeedChan:       clientSub.feed,
			ErrMsgChan:     clientSub.errMsgChan,
		}
	}

	return subscriptions
}

// GetGrpcSubscriptionReply - return gRPC subscription reply
func (f *Manager) GetGrpcSubscriptionReply() []ClientSubscriptionFullInfo {
	f.lock.RLock()
	defer f.lock.RUnlock()
	resp := make([]ClientSubscriptionFullInfo, 0, len(f.idToClientSubscription))
	for _, clientData := range f.idToClientSubscription {
		subscribe := ClientSubscriptionFullInfo{
			AccountID:    clientData.AccountID,
			Tier:         clientData.Tier,
			FeedName:     clientData.feedType,
			Network:      clientData.network,
			RemoteAddr:   clientData.RemoteAddress,
			Include:      clientData.Includes,
			Filter:       clientData.Filters,
			Age:          uint64(time.Since(clientData.timeOpenedFeed).Seconds()),
			MessagesSent: clientData.messagesSent,
			ConnType:     clientData.feedConnectionType,
		}
		resp = append(resp, subscribe)
	}

	return resp
}

// GetAllSubscriptions returns all subscriptions
func (f *Manager) GetAllSubscriptions() []types.SubscriptionModel {
	f.lock.RLock()
	defer f.lock.RUnlock()
	subscriptionModels := make([]types.SubscriptionModel, len(f.idToClientSubscription))
	i := 0
	for id, sub := range f.idToClientSubscription {
		subscriptionModel := types.SubscriptionModel{
			SubscriptionID: id,
			SubscriberIP:   strings.Split(sub.RemoteAddress, ":")[0],
			NodeID:         string(f.nodeID),
			AccountID:      sub.AccountID,
			NetworkNum:     sub.network,
			FeedType:       sub.feedType,
		}
		subscriptionModels[i] = subscriptionModel
		i++
	}
	return subscriptionModels
}

// CloseAllClientConnections - unsubscribes all client subscriptions and closes all client ws connections
func (f *Manager) CloseAllClientConnections() {
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

// notifySubscribers - getting feed notification and pass to client via common channel
func (f *Manager) notifySubscribers(ctx context.Context) {
	f.log.Infof("feedManager is starting for network %v", f.networkNum)

	for {
		select {
		case <-ctx.Done():
			f.log.Infof("feedManager stopped for network %v", f.networkNum)
			return
		case notification := <-f.feed:
			metrics.IncrFeedNotification(uint32(f.networkNum), string(notification.NotificationType()), metrics.SdNotificationsProcessed)

			f.lock.RLock()
			for uid, clientSub := range f.idToClientSubscription {
				if (clientSub.feedConnectionType == types.WebSocketFeed || clientSub.feedConnectionType == types.GRPCFeed) && clientSub.feedType == notification.NotificationType() {
					select {
					case clientSub.feed <- notification:
						metrics.IncrFeedNotificationDelivered(uint32(f.networkNum), string(notification.NotificationType()), string(clientSub.AccountID))
					default:
						f.log.Errorf("can't send %v to channel %v without blocking. Ignored hash %v and unsubscribing", clientSub.feedType, uid, notification.GetHash())
						go f.unsubscribeFromFeed(uid)
					}
				}
			}
			f.lock.RUnlock()
		}
	}
}

func (f *Manager) notifyErrors(ctx context.Context) {
	f.log.Infof("feedManager error channel is starting for network %v", f.networkNum)

	for {
		select {
		case <-ctx.Done():
			f.log.Infof("feedManager error channel stopped for network %v", f.networkNum)
			return
		case errNotification := <-f.errFeed:
			f.sendErrorMsgToClient(errNotification)
		}
	}
}

func (f *Manager) midnightCleanup(ctx context.Context) {
	f.log.Infof("midnight cleanup started for network %v", f.networkNum)

	// variables needed for daily account expiration check
	firstDailyCheckTriggered := true
	now := time.Now().UTC()
	durationUntilMidnight := now.Truncate(24 * time.Hour).Add(24 * time.Hour).Sub(now)
	dailyTicker := time.NewTicker(durationUntilMidnight)
	defer dailyTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			f.log.Infof("midnight cleanup stopped for network %v", f.networkNum)
			return
		case <-dailyTicker.C:
			timeNow := time.Now()
			// checks every 24 hours for all existing user subscription, if account expired close the subscription.
			if firstDailyCheckTriggered {
				firstDailyCheckTriggered = false
				dailyTicker.Reset(24 * time.Hour)
			}

			subToRemove := make([]string, 0, len(f.idToClientSubscription))

			f.lock.RLock()
			copySubscriptionsMap := make(map[string]ClientSubscription, len(f.idToClientSubscription))
			for key, value := range f.idToClientSubscription {
				copySubscriptionsMap[key] = value
			}
			f.lock.RUnlock()

			for subID, sub := range copySubscriptionsMap {
				accountModel, err := f.sdn.FetchCustomerAccountModel(sub.AccountID)
				if err != nil {
					log.Debugf("can't get account model for %v, while account has active feed subscription (%v), feed type: %v with %v since %s", sub.AccountID, subID, sub.feedType, sub.feedConnectionType, sub.timeOpenedFeed)
					continue
				}

				expireDateTime, err := time.Parse(bxgateway.TimeDateLayoutISO, accountModel.ExpireDate)
				if err != nil {
					log.Debugf("can't parse account model expiration date for %v, while account has active feed subscription (%v), feed type: %v with %v since %s", sub.AccountID, subID, sub.feedType, sub.feedConnectionType, sub.timeOpenedFeed)
					continue
				}

				if time.Now().UTC().After(expireDateTime.UTC()) {
					// if account expires, disconnect client connection
					log.Debugf("removing feed subscription for %v because account expires on %v, the feed subscription was (%v), feed type: %v with %v since %s", sub.AccountID, accountModel.ExpireDate, subID, sub.feedType, sub.feedConnectionType, sub.timeOpenedFeed)
					subToRemove = append(subToRemove, subID)
				}
			}

			for _, sid := range subToRemove {
				err := f.Unsubscribe(sid, true, accountExpiredError)
				if err != nil {
					log.Errorf("failed to remove feed subscription %v, %v", sid, err)
				}
			}
			log.Tracef("midnight subscription cleanup took %v", time.Since(timeNow))
		}
	}
}

func (f *Manager) sendErrorMsgToClient(errNotification ErrorNotification) {
	f.lock.RLock()
	defer f.lock.RUnlock()

	for uid, clientSub := range f.idToClientSubscription {
		if (clientSub.feedConnectionType == types.WebSocketFeed || clientSub.feedConnectionType == types.GRPCFeed) && clientSub.feedType == errNotification.FeedType {
			select {
			case clientSub.errMsgChan <- errNotification.ErrorMsg:
			default:
				f.log.Errorf("can't send error %v to channel %v without blocking. Ignored error %v and unsubscribing", clientSub.feedType, uid, errNotification.ErrorMsg)
				go f.unsubscribeFromFeed(uid)
			}
		}
	}
}

func (f *Manager) unsubscribeFromFeed(subscriptionID string) {
	// running as go-routine since we are holding the lock. Closing the connection since we can't write
	if err := f.Unsubscribe(subscriptionID, true, ""); err != nil {
		f.log.Debugf("unable to Unsubscribe %v - %v", subscriptionID, err)
	}
	// TODO: mark clientSub as "being closed" to prevent multiple Unsubscribe
}

func (f *Manager) checkForDuplicateFeed(clientSubscription *ClientSubscription, remoteAddress string) error {
	// feeds check should not be tested for customer running local gateway
	if clientSubscription.AccountID == f.accountModel.AccountID {
		return nil
	}

	remoteIP := strings.Split(remoteAddress, ":")[0] + ":"
	f.lock.RLock()
	defer f.lock.RUnlock()
	for k, v := range f.idToClientSubscription {
		if v.AccountID == clientSubscription.AccountID {
			if v.feedType == clientSubscription.feedType &&
				v.network == clientSubscription.network &&
				v.Includes == clientSubscription.Includes &&
				v.Filters == clientSubscription.Filters &&
				strings.HasPrefix(v.RemoteAddress, remoteIP) {
				return fmt.Errorf("duplicate feed request - account %v tier %v ip %v previous subscription ID %v", clientSubscription.AccountID, clientSubscription.Tier, remoteAddress, k)
			}
		}
	}
	return nil
}
