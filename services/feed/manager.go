package feed

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/jsonrpc2"

	log "github.com/bloXroute-Labs/bxcommon-go/v2/logger"
	"github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/metrics"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	accountExpiredError           = "Account expired, unsubscribe feed"
	logSubscriptionInterval       = 2 * time.Hour
	logSubscriptionInitialDelay   = time.Minute
	subscriptionsSnapshotInterval = 5 * time.Minute
)

// fanOutShardQueueSize is the depth of a single fan-out shard queue. It only has to smooth out
// scheduling jitter: the dispatcher blocks when a shard is behind, so backpressure travels back to
// the feed type's own notification channel instead of creating a second place where notifications
// are dropped.
const fanOutShardQueueSize = 128

// sharedFeedChannelName labels the fallback channel in the depth metric, which is otherwise tagged
// by feed type. It is not a feed type: every unregistered type shares this one queue.
const sharedFeedChannelName = "shared"

// fanOutShard is one worker of a feed type's fan-out pool. It owns a disjoint subset of that feed
// type's subscribers, so it can deliver without contending with the other shards.
type fanOutShard struct {
	notifications chan types.Notification

	mu   sync.RWMutex
	subs map[string]ClientSubscription
	// subCount mirrors len(subs) so the dispatcher can skip empty shards without taking mu.
	subCount atomic.Int64
}

// Manager - feed manager fields
type Manager struct {
	// feed is the fallback notification channel, used for feed types that were not registered
	// with NewManager and therefore have no dedicated channel.
	feed chan types.Notification
	// feeds holds one dedicated notification channel per registered feed type, so a burst on a
	// high volume feed (newTxs/pendingTxs) cannot fill up the channel shared with low volume
	// feeds and get their notifications dropped as collateral. Written only by NewManager and
	// read-only afterwards, so it needs no locking.
	feeds map[types.FeedType]chan types.Notification
	// shardsByType holds the fan-out worker pool of every registered feed type: the subscribers of
	// that feed type are split across the shards by subscription ID, so several notifications can
	// be delivered concurrently while each subscriber still gets its own notifications in order
	// (one shard owns a subscriber, and one goroutine drains that shard in FIFO order). Empty when
	// the pool is disabled (fanOutWorkers <= 1), in which case the fan-out stays inline. Written
	// only by NewManager and read-only afterwards, so the map itself needs no locking.
	shardsByType           map[types.FeedType][]*fanOutShard
	subscriberCounts       map[types.FeedType]*atomic.Int64
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
	metricsExporter        metrics.Exporter
}

// NewManager - create a new feedManager. Every feed type in feedTypes gets its own notification
// channel and consumer, isolating it from bursts on the other feeds; notifications of any other
// type fall back to a single shared channel.
//
// fanOutWorkers is the number of fan-out shards per registered feed type. With more than one, that
// feed type's subscribers are split across the shards by subscription ID and delivered by that many
// goroutines in parallel, while each individual subscriber keeps receiving its notifications in
// order. Anything <= 1 keeps the fan-out inline on the feed type's consumer, exactly as before.
func NewManager(sdn sdnsdk.SDNHTTP,
	subscriptionServices services.SubscriptionServices,
	accountModel sdnmessage.Account,
	stats statistics.Stats,
	blockchainNum bxtypes.NetworkNum,
	sendNotifications bool,
	metricsExporter metrics.Exporter,
	feedTypes []types.FeedType,
	fanOutWorkers int,
) *Manager {
	logger := log.WithFields(log.Fields{
		"component": "feedManager",
	})

	feeds := make(map[types.FeedType]chan types.Notification, len(feedTypes))
	for _, feedType := range feedTypes {
		feeds[feedType] = make(chan types.Notification, bxgateway.BxFeedChannelSize)
	}

	subscriberCounts := make(map[types.FeedType]*atomic.Int64, len(feedTypes))
	for _, feedType := range feedTypes {
		subscriberCounts[feedType] = &atomic.Int64{}
	}

	shardsByType := make(map[types.FeedType][]*fanOutShard, len(feedTypes))
	if fanOutWorkers > 1 {
		for _, feedType := range feedTypes {
			shards := make([]*fanOutShard, fanOutWorkers)
			for i := range shards {
				shards[i] = &fanOutShard{
					notifications: make(chan types.Notification, fanOutShardQueueSize),
					subs:          make(map[string]ClientSubscription),
				}
			}
			shardsByType[feedType] = shards
		}
	}

	newServer := &Manager{
		feed:                   make(chan types.Notification, bxgateway.BxFeedChannelSize),
		feeds:                  feeds,
		shardsByType:           shardsByType,
		subscriberCounts:       subscriberCounts,
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
		metricsExporter:        metricsExporter,
	}

	return newServer
}

// feedDepthSampleInterval is how often the depth of the feed channels is reported. The queues drain
// in microseconds when healthy, so this is a sampled gauge: it is meant to show whether a channel
// runs steadily full (the consumer cannot keep up, and a deeper buffer would only delay the
// overflow) or spikes from near-empty (a transient ingest burst, which is what a deeper buffer
// absorbs), not to catch every peak.
const feedDepthSampleInterval = 10 * time.Second

// Start - start feed manager
func (f *Manager) Start(ctx context.Context) {
	f.log.Infof("feedManager is starting for network %v", f.networkNum)

	shardCount := 0
	for _, shards := range f.shardsByType {
		shardCount += len(shards)
	}

	wg := sync.WaitGroup{}
	// the 4 loops below, the feed channel depth sampler, one consumer per dedicated per-feed-type
	// channel, and one worker per fan-out shard
	wg.Add(5 + len(f.feeds) + shardCount)

	go func() {
		defer wg.Done()
		f.sampleFeedChannelDepth(ctx)
	}()

	for _, shards := range f.shardsByType {
		for _, shard := range shards {
			go func() {
				defer wg.Done()
				f.runFanOutShard(ctx, shard)
			}()
		}
	}

	go func() {
		defer wg.Done()
		f.notifySubscribers(ctx, f.feed)
	}()

	for _, feedChan := range f.feeds {
		go func() {
			defer wg.Done()
			f.notifySubscribers(ctx, feedChan)
		}()
	}

	go func() {
		defer wg.Done()
		f.notifyErrors(ctx)
	}()

	go func() {
		defer wg.Done()
		f.midnightCleanup(ctx)
	}()

	go func() {
		defer wg.Done()
		f.logCurrentSubscriptions(ctx)
	}()

	wg.Wait()

	f.log.Infof("feedManager stopped for network %v", f.networkNum)
}

// Notify sends a notification to the feed
func (f *Manager) Notify(notification types.Notification) {
	if !f.sendNotifications {
		return
	}

	f.metricsExporter.PushIncrFeedNotificationCreated(uint32(f.networkNum), string(notification.NotificationType()))

	// if nobody is subscribed to this feed type - drop it
	if subscribers, tracked := f.subscriberCounts[notification.NotificationType()]; tracked && subscribers.Load() == 0 {
		return
	}

	// block feeds are low rate and latency sensitive, so they are delivered to the subscribers on
	// the caller's goroutine: the channel hop and the consumer wake-up cost more than the fan-out
	if isBlockFeed(notification.NotificationType()) {
		f.fanOut(notification)
		return
	}

	// f.feeds is immutable after NewManager, so this lookup needs no locking. Feed types without a
	// dedicated channel keep using the shared one, so an unregistered type is never silently lost.
	feedChan, ok := f.feeds[notification.NotificationType()]
	if !ok {
		feedChan = f.feed
	}

	select {
	case feedChan <- notification:
	default:
		f.log.Errorf("%v feed manager channel is full, ignoring %v notification type with hash %v",
			bxtypes.NetworkNumToBlockchainNetwork[f.networkNum], notification.NotificationType(), notification.GetHash())
	}
}

func isBlockFeed(feedType types.FeedType) bool {
	switch feedType {
	case types.NewBlocksFeed, types.BDNBlocksFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed:
		return true
	default:
		return false
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
	conn io.Closer, ci types.ClientInfo, ro types.ReqOptions, ethSubscribe bool,
) (*ClientSubscriptionHandlingInfo, error) {
	id := f.subscriptionServices.GenerateSubscriptionID(ethSubscribe)
	clientSubscription := ClientSubscription{
		unsubscribing:      &atomic.Bool{},
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

	var permissionRespChannel chan *types.SubscriptionPermissionMessage
	if ci.AccountID != bxtypes.BloxrouteAccountID {
		allowed, reason, ch := f.subscriptionServices.SendSubscribeNotification(&subscriptionModel)
		if !allowed {
			log.Debugf("subscription %v: allowed %v, reason %v", id, allowed, reason)
			return nil, errors.New(reason)
		}
		permissionRespChannel = ch
	}

	log.Tracef("subscription %v is allowed", id)

	f.lock.Lock()
	f.idToClientSubscription[id] = clientSubscription
	f.trackSubscriber(feedName, 1)
	f.addToShard(id, clientSubscription)
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
		// nothing to do, subscription does not exist
		f.lock.Unlock()
		return nil
	}
	delete(f.idToClientSubscription, subscriptionID)
	f.trackSubscriber(clientSub.feedType, -1)
	f.removeFromShard(subscriptionID, clientSub.feedType)
	f.lock.Unlock()

	f.log.WithFields(log.Fields{
		"account_id":     clientSub.AccountID,
		"feed_name":      clientSub.feedType,
		"remote_address": clientSub.RemoteAddress,
	}).Infof("unsubscribing from feed, closing the connection: %v", closeClientConnection)

	if errMsg != "" {
		clientSub.errMsgChan <- errMsg
	}

	if clientSub.AccountID != bxtypes.BloxrouteAccountID {
		subscription := types.SubscriptionModel{
			SubscriptionID: subscriptionID,
			SubscriberIP:   strings.Split(clientSub.RemoteAddress, ":")[0],
			NodeID:         string(f.nodeID),
			AccountID:      clientSub.AccountID,
			NetworkNum:     clientSub.network,
			FeedType:       clientSub.feedType,
		}
		f.subscriptionServices.SendUnsubscribeNotification(&subscription)
	}

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
		clientSub.AccountID)
	close(clientSub.feed)

	if closeClientConnection && clientSub.connection != nil {
		// TODO: need to unsubscribe all other subscriptions on this connection.
		err := clientSub.connection.Close()
		if err != nil && !errors.Is(err, jsonrpc2.ErrClosed) {
			f.log.Warnf("failed to close connection for %v: %v", subscriptionID, err)
			return fmt.Errorf("failed to close connection for %v: %w", subscriptionID, err)
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
	maps.Copy(copyIDToClientSubscription, f.idToClientSubscription)
	f.lock.Unlock()

	for subscriptionID := range copyIDToClientSubscription {
		_ = f.Unsubscribe(subscriptionID, true, "")
	}
}

// Close closes the subscription services
func (f *Manager) Close() error {
	return f.subscriptionServices.Close("stop feed manager")
}

// notifySubscribers - getting feed notification from feedChan and pass to client via common channel
func (f *Manager) notifySubscribers(ctx context.Context, feedChan <-chan types.Notification) {
	for {
		select {
		case <-ctx.Done():
			return
		case notification := <-feedChan:
			shards, ok := f.shardsByType[notification.NotificationType()]
			if !ok || len(shards) == 0 {
				f.fanOut(notification)
				continue
			}

			f.metricsExporter.PushIncrFeedNotificationProcessed(uint32(f.networkNum), string(notification.NotificationType()))

			for _, shard := range shards {
				// a shard without subscribers would only cost a channel hop and an empty iteration
				if shard.subCount.Load() == 0 {
					continue
				}

				// blocking on purpose: when a shard falls behind, the backpressure travels back to
				// this feed type's own channel, where an overflow is already counted and logged,
				// instead of silently dropping the notification here
				select {
				case shard.notifications <- notification:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// sampleFeedChannelDepth periodically reports how full each feed channel is. An overflow is already
// counted when it happens; this is what shows the queue filling up beforehand, and tells apart a
// channel that runs steadily full (the consumer is too slow - a deeper buffer only postpones the
// drops) from one that spikes from empty (an ingest burst, which a deeper buffer does absorb).
func (f *Manager) sampleFeedChannelDepth(ctx context.Context) {
	ticker := time.NewTicker(feedDepthSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for feedType, feedChan := range f.feeds {
				f.metricsExporter.PushFeedChannelDepth(uint32(f.networkNum), string(feedType), len(feedChan), cap(feedChan))
			}

			f.metricsExporter.PushFeedChannelDepth(uint32(f.networkNum), sharedFeedChannelName, len(f.feed), cap(f.feed))
		}
	}
}

// runFanOutShard drains one fan-out shard, delivering every notification to the subscribers that
// shard owns. A single goroutine per shard is what keeps each subscriber's notifications ordered.
func (f *Manager) runFanOutShard(ctx context.Context, shard *fanOutShard) {
	for {
		select {
		case <-ctx.Done():
			return
		case notification := <-shard.notifications:
			shard.mu.RLock()
			f.deliver(notification, shard.subs)
			shard.mu.RUnlock()
		}
	}
}

// shardFor returns the shard owning subscriptionID inside feedType's pool, or nil when that feed
// type has no pool. Hashing the subscription ID pins a subscriber to one shard, which is what
// preserves the delivery order for that subscriber.
func (f *Manager) shardFor(feedType types.FeedType, subscriptionID string) *fanOutShard {
	shards, ok := f.shardsByType[feedType]
	if !ok || len(shards) == 0 {
		return nil
	}

	hash := fnv.New32a()
	_, _ = hash.Write([]byte(subscriptionID))

	// masking the sign bit keeps the index non-negative even where int is 32 bits wide
	return shards[int(hash.Sum32()&math.MaxInt32)%len(shards)]
}

// trackSubscriber moves feedType's subscriber count by delta.
func (f *Manager) trackSubscriber(feedType types.FeedType, delta int64) {
	if subscribers, tracked := f.subscriberCounts[feedType]; tracked {
		subscribers.Add(delta)
	}
}

// addToShard registers a subscription with the shard that owns it, if its feed type has a pool.
func (f *Manager) addToShard(subscriptionID string, clientSub ClientSubscription) {
	shard := f.shardFor(clientSub.feedType, subscriptionID)
	if shard == nil {
		return
	}

	shard.mu.Lock()
	shard.subs[subscriptionID] = clientSub
	shard.subCount.Store(int64(len(shard.subs)))
	shard.mu.Unlock()
}

// removeFromShard drops a subscription from the shard that owns it, if its feed type has a pool.
func (f *Manager) removeFromShard(subscriptionID string, feedType types.FeedType) {
	shard := f.shardFor(feedType, subscriptionID)
	if shard == nil {
		return
	}

	shard.mu.Lock()
	delete(shard.subs, subscriptionID)
	shard.subCount.Store(int64(len(shard.subs)))
	shard.mu.Unlock()
}

func (f *Manager) fanOut(notification types.Notification) {
	f.metricsExporter.PushIncrFeedNotificationProcessed(uint32(f.networkNum), string(notification.NotificationType()))
	f.lock.RLock()
	f.deliver(notification, f.idToClientSubscription)
	f.lock.RUnlock()
}

// deliver passes notification to every subscription in subs that asked for this feed type. The
// caller must hold the lock guarding subs.
func (f *Manager) deliver(notification types.Notification, subs map[string]ClientSubscription) {
	customNotification, isCustom := notification.(types.CustomNotification)
	for uid, clientSub := range subs {
		notificationToSend := notification
		if isCustom {
			notificationToSend = customNotification.ApplyAccountLogic(string(clientSub.AccountID))
		}

		if (clientSub.feedConnectionType == types.WebSocketFeed || clientSub.feedConnectionType == types.GRPCFeed) && clientSub.feedType == notification.NotificationType() {
			select {
			case clientSub.feed <- notificationToSend:
				f.metricsExporter.PushIncrFeedNotificationDelivered(uint32(f.networkNum), string(notification.NotificationType()), string(clientSub.AccountID))
			default:
				if clientSub.unsubscribing.CompareAndSwap(false, true) {
					f.log.Errorf("can't send %v to channel %v without blocking. Ignored hash %v and unsubscribing", clientSub.feedType, uid, notification.GetHash())
					go f.unsubscribeFromFeed(uid)
				}
			}
		}
	}
}

func (f *Manager) notifyErrors(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case errNotification := <-f.errFeed:
			f.sendErrorMsgToClient(errNotification)
		}
	}
}

func (f *Manager) midnightCleanup(ctx context.Context) {
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
			maps.Copy(copySubscriptionsMap, f.idToClientSubscription)
			f.lock.RUnlock()

			for subID, sub := range copySubscriptionsMap {
				accountModel, err := f.sdn.FetchCustomerAccountModel(sub.AccountID)
				if err != nil {
					log.Debugf("can't get account model for %v, while account has active feed subscription (%v), feed type: %v with %v since %s", sub.AccountID, subID, sub.feedType, sub.feedConnectionType, sub.timeOpenedFeed)
					continue
				}

				if accountModel.IsExpired() {
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
				return fmt.Errorf("duplicate feed request - account %v ip %v previous subscription ID %v", clientSubscription.AccountID, remoteAddress, k)
			}
		}
	}
	return nil
}

func (f *Manager) logCurrentSubscriptions(ctx context.Context) {
	ticker := time.NewTicker(logSubscriptionInterval)
	defer ticker.Stop()

	// wait initial delay so clients have time to resubscribe
	select {
	case <-ctx.Done():
		return
	case <-time.After(logSubscriptionInitialDelay):
	}

	for {
		subs := f.currentSubscriptions()
		for accountID, feeds := range subs {
			for feedType, count := range feeds {
				f.stats.LogSubscriptionsStats(accountID, feedType, count, f.networkNum)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SubscriptionsSnapshotLoop periodically logs a snapshot of active subscriptions.
func (f *Manager) SubscriptionsSnapshotLoop(ctx context.Context) {
	ticker := time.NewTicker(subscriptionsSnapshotInterval)
	defer ticker.Stop()

	network := bxtypes.NetworkNumToBlockchainNetwork[f.networkNum]

	for {
		subs := f.currentSubscriptions()
		for accountID, feeds := range subs {
			if accountID == bxtypes.BloxrouteAccountID {
				continue
			}
			for feedType, count := range feeds {
				f.stats.LogSubscriptionsSnapshot(accountID, feedType, count, f.networkNum, network)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (f *Manager) currentSubscriptions() map[bxtypes.AccountID]map[types.FeedType]int {
	subscriptions := make(map[bxtypes.AccountID]map[types.FeedType]int)

	f.lock.RLock()
	defer f.lock.RUnlock()

	for i := range f.idToClientSubscription {
		infos, exists := subscriptions[f.idToClientSubscription[i].AccountID]
		if !exists {
			subscriptions[f.idToClientSubscription[i].AccountID] = map[types.FeedType]int{
				f.idToClientSubscription[i].feedType: 1,
			}
			continue
		}
		infos[f.idToClientSubscription[i].feedType]++
	}

	return subscriptions
}
