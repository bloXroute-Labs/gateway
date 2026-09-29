package feed

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/metrics"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// testNotification is a minimal types.Notification used to drive the manager's routing.
type testNotification struct {
	feedType bxtypes.FeedType
	hash     string
}

func (n testNotification) WithFields([]string) types.Notification   { return n }
func (n testNotification) Filters() (map[string]interface{}, error) { return nil, nil }
func (n testNotification) LocalRegion() bool                        { return false }
func (n testNotification) GetHash() string                          { return n.hash }
func (n testNotification) NotificationType() bxtypes.FeedType       { return n.feedType }

func newTestManager(t *testing.T, feedTypes []bxtypes.FeedType, fanOutWorkers int) *Manager {
	t.Helper()

	ctl := gomock.NewController(t)
	sdn := mock.NewMockSDNHTTP(ctl)
	sdn.EXPECT().NodeID().Return(bxtypes.NodeID("nodeID")).AnyTimes()

	return NewManager(sdn, services.NewNoOpSubscriptionServices(), sdnmessage.Account{},
		statistics.NoStats{}, bxtypes.NetworkNum(5), true, &metrics.NoOpExporter{}, feedTypes,
		fanOutWorkers)
}

// subscribeTestClient subscribes one WebSocket client to feedType. Every client gets its own
// account ID so that the duplicate-feed guard does not reject it.
func subscribeTestClient(t *testing.T, f *Manager, feedType bxtypes.FeedType, n int) (string, chan types.Notification) {
	t.Helper()

	info, err := f.Subscribe(feedType, bxtypes.WebSocketFeed, nil, types.ClientInfo{
		AccountID:     bxtypes.AccountID(fmt.Sprintf("account-%d", n)),
		RemoteAddress: fmt.Sprintf("10.0.0.%d:1234", n+1),
	}, types.ReqOptions{}, false)
	require.NoError(t, err)

	return info.SubscriptionID, info.FeedChan
}

// shardSubTotal counts the subscriptions held by feedType's fan-out shards.
func shardSubTotal(f *Manager, feedType bxtypes.FeedType) int64 {
	var total int64
	for _, shard := range f.shardsByType[feedType] {
		total += shard.subCount.Load()
	}

	return total
}

// TestNotifyRoutesPerFeedType verifies each registered feed type is queued on its own channel.
func TestNotifyRoutesPerFeedType(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed, bxtypes.PendingTxsFeed}, 1)

	// Notify skips feed types nobody is subscribed to, so each one needs an audience before its
	// routing can be observed
	subscribeTestClient(t, f, bxtypes.NewTxsFeed, 0)
	subscribeTestClient(t, f, bxtypes.PendingTxsFeed, 1)

	f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0x1"})
	f.Notify(testNotification{feedType: bxtypes.PendingTxsFeed, hash: "0x2"})
	f.Notify(testNotification{feedType: bxtypes.PendingTxsFeed, hash: "0x3"})

	assert.Len(t, f.feeds[bxtypes.NewTxsFeed], 1)
	assert.Len(t, f.feeds[bxtypes.PendingTxsFeed], 2)
	// nothing should have fallen back to the shared channel
	assert.Empty(t, f.feed)
}

// TestNotifyUnregisteredFeedTypeFallsBack makes sure a feed type without a dedicated channel is
// still queued (on the shared fallback channel) rather than silently dropped.
func TestNotifyUnregisteredFeedTypeFallsBack(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, 1)

	f.Notify(testNotification{feedType: bxtypes.TxReceiptsFeed, hash: "0x1"})

	require.NotContains(t, f.feeds, bxtypes.TxReceiptsFeed)
	assert.Len(t, f.feed, 1)
	assert.Empty(t, f.feeds[bxtypes.NewTxsFeed])
}

// TestStartDrainsEveryFeedChannel makes sure every per-feed-type channel gets its own live
// consumer, i.e. the consumer goroutines started by Start do not all end up reading from the same
// channel and leaving the others undrained.
func TestStartDrainsEveryFeedChannel(t *testing.T) {
	feedTypes := []bxtypes.FeedType{
		bxtypes.NewTxsFeed, bxtypes.PendingTxsFeed, bxtypes.BDNBlocksFeed, bxtypes.TxReceiptsFeed,
	}
	f := newTestManager(t, feedTypes, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Start(ctx)

	for i, feedType := range feedTypes {
		subscribeTestClient(t, f, feedType, i)
	}

	for _, feedType := range feedTypes {
		f.Notify(testNotification{feedType: feedType, hash: "0x1"})
	}

	require.Eventually(t, func() bool {
		for _, feedType := range feedTypes {
			if len(f.feeds[feedType]) != 0 {
				return false
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond, "every feed channel must be drained by its own consumer")
}

// TestBurstOnOneFeedDoesNotDropOtherFeeds is the regression test for DI-4076: overflowing a high
// volume feed must not consume the capacity of the other feeds. No consumers are started, so the
// per-type channels fill up deterministically.
func TestBurstOnOneFeedDoesNotDropOtherFeeds(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed, bxtypes.PendingTxsFeed}, 1)

	subscribeTestClient(t, f, bxtypes.NewTxsFeed, 0)
	subscribeTestClient(t, f, bxtypes.PendingTxsFeed, 1)

	// flood newTxs past its channel capacity
	burst := bxgateway.BxFeedChannelSize + 500
	for i := 0; i < burst; i++ {
		f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0xnew"})
	}
	require.Len(t, f.feeds[bxtypes.NewTxsFeed], bxgateway.BxFeedChannelSize,
		"newTxs channel should be saturated at its capacity")

	// the other feed still has its full capacity available
	for i := 0; i < bxgateway.BxFeedChannelSize; i++ {
		f.Notify(testNotification{feedType: bxtypes.PendingTxsFeed, hash: "0xpending"})
	}
	assert.Len(t, f.feeds[bxtypes.PendingTxsFeed], bxgateway.BxFeedChannelSize,
		"pendingTxs notifications must not be dropped because of the newTxs burst")
}

// TestFanOutPoolPreservesPerSubscriberOrder is the core guarantee of the fan-out pool: several
// shards deliver concurrently, but each subscriber still sees its notifications in the order they
// were produced.
func TestFanOutPoolPreservesPerSubscriberOrder(t *testing.T) {
	const (
		subscribers   = 6
		notifications = 200
		workers       = 4
	)

	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, workers)
	require.Len(t, f.shardsByType[bxtypes.NewTxsFeed], workers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Start(ctx)

	channels := make([]chan types.Notification, subscribers)
	for i := range channels {
		_, channels[i] = subscribeTestClient(t, f, bxtypes.NewTxsFeed, i)
	}

	for i := 0; i < notifications; i++ {
		f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: strconv.Itoa(i)})
	}

	for i, channel := range channels {
		for want := 0; want < notifications; want++ {
			select {
			case got := <-channel:
				require.Equalf(t, strconv.Itoa(want), got.GetHash(),
					"subscriber %d received its notifications out of order", i)
			case <-time.After(5 * time.Second):
				t.Fatalf("subscriber %d received only %d of %d notifications", i, want, notifications)
			}
		}
	}
}

// TestFanOutPoolDisabledKeepsInlineFanOut checks that fanOutWorkers <= 1 builds no shards and keeps
// delivering on the feed type's consumer, i.e. the pool is opt-in.
func TestFanOutPoolDisabledKeepsInlineFanOut(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, 1)
	require.Empty(t, f.shardsByType)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Start(ctx)

	_, channel := subscribeTestClient(t, f, bxtypes.NewTxsFeed, 0)
	f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0x1"})

	select {
	case got := <-channel:
		assert.Equal(t, "0x1", got.GetHash())
	case <-time.After(5 * time.Second):
		t.Fatal("notification was not delivered with the fan-out pool disabled")
	}
}

// TestFanOutShardMembershipFollowsSubscriptions makes sure a subscription is owned by exactly one
// shard and is removed from it on unsubscribe, so no shard keeps delivering to a gone client.
func TestFanOutShardMembershipFollowsSubscriptions(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, 4)

	subscriptionID, _ := subscribeTestClient(t, f, bxtypes.NewTxsFeed, 0)

	owner := f.shardFor(bxtypes.NewTxsFeed, subscriptionID)
	require.NotNil(t, owner)
	assert.Equal(t, int64(1), owner.subCount.Load())
	assert.Equal(t, int64(1), shardSubTotal(f, bxtypes.NewTxsFeed), "subscription must live in one shard only")

	require.NoError(t, f.Unsubscribe(subscriptionID, false, ""))
	assert.Zero(t, owner.subCount.Load())
	assert.Zero(t, shardSubTotal(f, bxtypes.NewTxsFeed))
}

// TestFanOutPoolSpreadsSubscribersAcrossShards verifies the sharding actually splits the
// subscribers, which is what lets the delivery run in parallel.
func TestFanOutPoolSpreadsSubscribersAcrossShards(t *testing.T) {
	const (
		workers     = 4
		subscribers = 40
	)

	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, workers)
	for i := 0; i < subscribers; i++ {
		subscribeTestClient(t, f, bxtypes.NewTxsFeed, i)
	}

	usedShards := 0
	for _, shard := range f.shardsByType[bxtypes.NewTxsFeed] {
		if shard.subCount.Load() > 0 {
			usedShards++
		}
	}

	assert.Greater(t, usedShards, 1, "subscribers should be spread over more than one shard")
	assert.Equal(t, int64(subscribers), shardSubTotal(f, bxtypes.NewTxsFeed))
}

func TestShardMembershipNeverOutlivesSubscription(t *testing.T) {
	const (
		workers     = 4
		rounds      = 100
		subscribers = 8
	)

	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, workers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Start(ctx)

	var wg sync.WaitGroup

	// keep notifications flowing, so the fan-out shards are actively delivering into the client
	// channels that CloseAllClientConnections is closing underneath them
	notifierDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-notifierDone:
				return
			default:
				f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: strconv.Itoa(i)})
				time.Sleep(time.Millisecond)
			}
		}
	}()

	for round := 0; round < rounds; round++ {
		var subscribeWg sync.WaitGroup
		for i := 0; i < subscribers; i++ {
			subscribeWg.Add(1)
			go func(n int) {
				defer subscribeWg.Done()
				_, _ = f.Subscribe(bxtypes.NewTxsFeed, bxtypes.WebSocketFeed, nil, types.ClientInfo{
					AccountID:     bxtypes.AccountID(fmt.Sprintf("account-%d-%d", round, n)),
					RemoteAddress: fmt.Sprintf("10.%d.%d.%d:1234", round%256, n, n+1),
				}, types.ReqOptions{}, false)
			}(i)
		}

		// races the subscribers above: it snapshots the canonical map and unsubscribes every id it
		// saw, closing each client channel on the way out
		go f.CloseAllClientConnections()

		subscribeWg.Wait()
	}

	close(notifierDone)
	wg.Wait()

	f.CloseAllClientConnections()

	f.lock.RLock()
	defer f.lock.RUnlock()

	for _, shard := range f.shardsByType[bxtypes.NewTxsFeed] {
		shard.mu.RLock()
		for id := range shard.subs {
			_, live := f.idToClientSubscription[id]
			assert.Truef(t, live, "subscription %v is still owned by a shard after being unsubscribed; "+
				"its channel is closed, so the next delivery to that shard would panic", id)
		}
		assert.Equal(t, int64(len(shard.subs)), shard.subCount.Load(), "subCount drifted from the shard's membership")
		shard.mu.RUnlock()
	}
}

// TestNotifySkipsFeedTypesWithoutSubscribers is the fix for the manager-channel overflow seen on
// gateways with no clients: newTxs is notified for every transaction whether or not anybody asked
// for it, so without this gate the firehose fills the feed channel and every notification past its
// capacity is counted (and logged) as a drop.
func TestNotifySkipsFeedTypesWithoutSubscribers(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed, bxtypes.PendingTxsFeed}, 1)

	for i := 0; i < bxgateway.BxFeedChannelSize+500; i++ {
		f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0xnew"})
	}

	assert.Empty(t, f.feeds[bxtypes.NewTxsFeed], "a feed type nobody subscribed to must not be queued")
	assert.Empty(t, f.feed, "and must not fall back to the shared channel either")

	// once a client is listening, the very next notification is queued again
	subscriptionID, _ := subscribeTestClient(t, f, bxtypes.NewTxsFeed, 0)
	f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0x1"})
	assert.Len(t, f.feeds[bxtypes.NewTxsFeed], 1)

	// ...and stops being queued as soon as the last subscriber leaves
	require.NoError(t, f.Unsubscribe(subscriptionID, false, ""))
	f.Notify(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0x2"})
	assert.Len(t, f.feeds[bxtypes.NewTxsFeed], 1, "no notification should have been added after the unsubscribe")
}

// TestNotifyStillQueuesUnregisteredFeedTypes guards the fallback: a feed type that was not
// registered with NewManager has no subscriber counter, and skipping it would silently lose
// notifications that the shared channel is supposed to carry.
func TestNotifyStillQueuesUnregisteredFeedTypes(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, 1)

	f.Notify(testNotification{feedType: bxtypes.TxReceiptsFeed, hash: "0x1"})

	assert.Len(t, f.feed, 1)
}

// TestFullClientChannelUnsubscribesOnce covers the second half of the fix: while a client's channel
// is full it is still in the subscription map, so every notification of the burst used to log an
// error and spawn another unsubscribe for the same client - on the goroutine that is supposed to be
// draining the feed.
func TestFullClientChannelUnsubscribesOnce(t *testing.T) {
	f := newTestManager(t, []bxtypes.FeedType{bxtypes.NewTxsFeed}, 1)

	subscriptionID, channel := subscribeTestClient(t, f, bxtypes.NewTxsFeed, 0)

	// nobody is reading this client's channel, so it saturates and every further delivery is a drop
	for i := 0; i < bxgateway.BxNotificationChannelSize; i++ {
		f.fanOut(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0xfill"})
	}
	require.Len(t, channel, bxgateway.BxNotificationChannelSize)

	f.lock.RLock()
	clientSub := f.idToClientSubscription[subscriptionID]
	f.lock.RUnlock()
	require.NotNil(t, clientSub.unsubscribing)
	require.False(t, clientSub.unsubscribing.Load(), "no drop has happened yet")

	for i := 0; i < 100; i++ {
		f.fanOut(testNotification{feedType: bxtypes.NewTxsFeed, hash: "0xdropped"})
	}

	assert.True(t, clientSub.unsubscribing.Load(), "the first drop must claim the unsubscribe")

	require.Eventually(t, func() bool {
		return !f.SubscriptionExists(subscriptionID)
	}, 5*time.Second, 5*time.Millisecond, "the dropped client must be unsubscribed")
}
