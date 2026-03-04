package services

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	"github.com/bloXroute-Labs/gateway/v2/types"
	baseutils "github.com/bloXroute-Labs/gateway/v2/utils"
)

// SubscriptionServices provides interface to core subscription management functions
type SubscriptionServices interface {
	SendSubscribeNotification(*types.SubscriptionModel) (bool, string, chan *types.SubscriptionPermissionMessage)
	SendUnsubscribeNotification(*types.SubscriptionModel)
	SendSubscriptionResetNotification([]types.SubscriptionModel)
	GenerateSubscriptionID(bool) string
}

// NoOpSubscriptionServices no-op implementation of SubscriptionServices interface
type NoOpSubscriptionServices struct {
}

// NewNoOpSubscriptionServices returns no-op set of subscription services
func NewNoOpSubscriptionServices() SubscriptionServices {
	noOpSubscriptionServices := NoOpSubscriptionServices{}
	return noOpSubscriptionServices
}

// SendSubscribeNotification approves all requests
func (n NoOpSubscriptionServices) SendSubscribeNotification(*types.SubscriptionModel) (bool, string, chan *types.SubscriptionPermissionMessage) {
	return true, "", nil
}

// SendUnsubscribeNotification - no-op
func (n NoOpSubscriptionServices) SendUnsubscribeNotification(*types.SubscriptionModel) {
	return
}

// SendSubscriptionResetNotification - no-op
func (n NoOpSubscriptionServices) SendSubscriptionResetNotification([]types.SubscriptionModel) {
	return
}

// GenerateSubscriptionID generate uuid
func (n NoOpSubscriptionServices) GenerateSubscriptionID(ethSubscribe bool) string {
	return generateSubscriptionID(ethSubscribe)
}

// SDNSubscriptionTracker tracks account subscriptions in coordination with SDN.
type SDNSubscriptionTracker struct {
	sdn   sdnsdk.SDNHTTP
	mutex sync.Mutex
	state message.InternalGateway
}

// NewSDNSubscriptionTracker creates a new SDNSubscriptionTracker instance.
func NewSDNSubscriptionTracker(sdn sdnsdk.SDNHTTP) *SDNSubscriptionTracker {
	return &SDNSubscriptionTracker{
		sdn: sdn,
		state: message.InternalGateway{
			AccountSubscriptionCounts: make(map[string]uint),
		},
	}
}

// Run periodically sends account subscription counts to SND.
func (t *SDNSubscriptionTracker) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	setInternalGateway := func() {
		t.mutex.Lock()
		state := cloneInternalGateway(&t.state)
		t.mutex.Unlock()

		if err := t.sdn.SetInternalGateway(&state); err != nil {
			log.Errorf("failed to send internal gateway state to SDN: %v", err)
		}
	}
	setInternalGateway()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			setInternalGateway()
		}
	}
}

// SendSubscribeNotification tries to increment number of subscriptions checking the account quota.
func (t *SDNSubscriptionTracker) SendSubscribeNotification(sub *types.SubscriptionModel) (bool, string, chan *types.SubscriptionPermissionMessage) {
	if err := t.sdn.AddInternalGatewaySubscription(sub.AccountID); err != nil {
		return false, fmt.Sprintf("cannot subscribe account %v: %v", sub.AccountID, err), nil
	}

	t.mutex.Lock()
	t.state.AccountSubscriptionCounts[string(sub.AccountID)]++
	t.mutex.Unlock()

	return true, "", nil
}

// SendUnsubscribeNotification decrements number of the account subscriptions.
func (t *SDNSubscriptionTracker) SendUnsubscribeNotification(sub *types.SubscriptionModel) {
	if err := t.sdn.RemoveInternalGatewaySubscription(sub.AccountID); err != nil {
		log.Errorf("failed to remove internal gateway subscription: %v", err)
		return
	}

	t.mutex.Lock()
	t.decrementSubscriptionCount(string(sub.AccountID))
	t.mutex.Unlock()
}

func (t *SDNSubscriptionTracker) decrementSubscriptionCount(accountID string) {
	counts := t.state.AccountSubscriptionCounts
	if count, ok := counts[accountID]; ok {
		count--
		if count == 0 {
			delete(counts, accountID)
		} else {
			counts[accountID] = count
		}
	}
}

// SendSubscriptionResetNotification resets subscriptions for the given accounts.
func (t *SDNSubscriptionTracker) SendSubscriptionResetNotification(subs []types.SubscriptionModel) {
	t.mutex.Lock()
	if len(subs) == 0 {
		for k := range t.state.AccountSubscriptionCounts {
			delete(t.state.AccountSubscriptionCounts, k)
		}
	} else {
		for _, s := range subs {
			t.decrementSubscriptionCount(string(s.AccountID))
		}
	}
	state := cloneInternalGateway(&t.state)
	t.mutex.Unlock()

	if err := t.sdn.SetInternalGateway(&state); err != nil {
		log.Errorf("failed to reset subscriptions in SDN: %v", err)
	}
}

// GenerateSubscriptionID generate uuid
func (t *SDNSubscriptionTracker) GenerateSubscriptionID(ethSubscribe bool) string {
	return generateSubscriptionID(ethSubscribe)
}

func generateSubscriptionID(ethSubscribe bool) string {
	if ethSubscribe {
		u128, _ := baseutils.GenerateU128()
		return u128
	}

	return baseutils.GenerateUUID()
}

func cloneInternalGateway(state *message.InternalGateway) message.InternalGateway {
	return message.InternalGateway{
		AccountSubscriptionCounts: maps.Clone(state.AccountSubscriptionCounts),
	}
}
