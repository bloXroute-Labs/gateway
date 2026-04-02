package services

import (
	"math"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"

	"github.com/bloXroute-Labs/gateway/v2/types"
	baseutils "github.com/bloXroute-Labs/gateway/v2/utils"
)

// SubscriptionBlacklistTimeout is the duration that an account is prevented from requesting a subscription after a rejected attempt
const SubscriptionBlacklistTimeout = time.Second * 15

// SubscriptionManager manages and stores all subscription requests to SDN and permission responses from SDN
type SubscriptionManager struct {
	subscribeRequests *syncmap.SyncMap[string, chan *types.SubscriptionPermissionMessage]
	blacklistCache    *syncmap.SyncMap[bxtypes.AccountID, time.Time]
	clock             clock.Clock
}

// SubscriptionLimitsEnforced - checks if a feedType has subscription limits
func SubscriptionLimitsEnforced(field types.FeedType) bool {
	subscriptionLimitedFeeds := []types.FeedType{types.NewTxsFeed, types.PendingTxsFeed, types.BDNBlocksFeed, types.NewBlocksFeed, types.OnBlockFeed, types.TxReceiptsFeed, types.TraceBlocksFeed}
	for _, valid := range subscriptionLimitedFeeds {
		if field == valid {
			return true
		}
	}
	return false
}

// NewSubscriptionManager returns a new instance
func NewSubscriptionManager(clock clock.Clock) SubscriptionManager {
	return SubscriptionManager{
		subscribeRequests: syncmap.NewStringMapOf[chan *types.SubscriptionPermissionMessage](),
		blacklistCache:    syncmap.NewTypedMapOf[bxtypes.AccountID, time.Time](syncmap.AccountIDHasher),
		clock:             clock,
	}
}

// RecordSubscriptionRequest stores and returns a channel for permission responses for subscription ID
func (m SubscriptionManager) RecordSubscriptionRequest(subscriptionID string) chan *types.SubscriptionPermissionMessage {
	permissionResponseChannel := make(chan *types.SubscriptionPermissionMessage, 10)
	m.subscribeRequests.Store(subscriptionID, permissionResponseChannel)
	return permissionResponseChannel
}

// ForwardPermissionResponse writes the permission response to the subscription's response channel if applicable and returns success boolean
func (m SubscriptionManager) ForwardPermissionResponse(permissionMsg *types.SubscriptionPermissionMessage) {
	if !permissionMsg.Allowed {
		_, found := m.blacklistCache.Load(permissionMsg.AccountID)
		if !found {
			m.blacklistCache.Store(permissionMsg.AccountID, m.clock.Now())
			m.clock.AfterFunc(SubscriptionBlacklistTimeout, func() { m.clean(permissionMsg.AccountID) })
		}
	}
	permissionResponseCh, exists := m.subscribeRequests.Load(permissionMsg.SubscriptionID)
	if !exists {
		log.Errorf("received permission response for subscription ID with no open request or subscription: %+v", permissionMsg)
		return
	}
	select {
	case permissionResponseCh <- permissionMsg:
	default:
		log.Errorf("failed to write new permission message %+v to channel (process: %v)", permissionMsg, baseutils.GetGID())
	}
}

// EndSubscriptionManagement removes the subscription ID (and corresponding response channel) from storage
func (m SubscriptionManager) EndSubscriptionManagement(subscriptionID string) {
	m.subscribeRequests.Delete(subscriptionID)
}

// IsAccountBlacklisted indicates whether an account is on the blacklist
// Used to prevent SDN from become overloaded by incessant repeated requests from bots or bad actors
func (m SubscriptionManager) IsAccountBlacklisted(accountID bxtypes.AccountID) (bool, int) {
	timeBlacklisted, found := m.blacklistCache.Load(accountID)
	if !found {
		return false, 0
	}
	timeBlacklistClears := timeBlacklisted.Add(SubscriptionBlacklistTimeout)
	secondsRemainingOnBlacklist := timeBlacklistClears.Sub(m.clock.Now()).Seconds()
	return found, int(math.Ceil(secondsRemainingOnBlacklist))
}

func (m SubscriptionManager) clean(accountID bxtypes.AccountID) {
	m.blacklistCache.Delete(accountID)
}
