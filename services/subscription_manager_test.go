package services

import (
	"testing"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	"github.com/stretchr/testify/assert"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

func TestSubscriptionManager_RequestSubscriptionLifecycle(t *testing.T) {
	manager := NewSubscriptionManager(&clock.MockClock{})
	responseChannel := manager.RecordSubscriptionRequest("sub1")
	assert.NotNil(t, responseChannel)
	permissionMsg := types.SubscriptionPermissionMessage{
		SubscriptionID: "sub1",
		AccountID:      "account1",
		Allowed:        true,
		ErrorReason:    "",
	}
	manager.ForwardPermissionResponse(&permissionMsg)

	select {
	case permissionResponse := <-responseChannel:
		assert.Equal(t, *permissionResponse, permissionMsg)
	default:
		assert.Fail(t, "permission response channel unexpectedly empty")
	}

	manager.EndSubscriptionManagement("sub1")
	responseChannel = manager.RecordSubscriptionRequest("sub1")
	assert.NotNil(t, responseChannel)
}

func TestSubscriptionManager_BlacklistLifecycle(t *testing.T) {
	mc := clock.MockClock{}
	manager := NewSubscriptionManager(&mc)
	responseChannel := manager.RecordSubscriptionRequest("sub1")
	assert.NotNil(t, responseChannel)
	permissionMsg := types.SubscriptionPermissionMessage{
		SubscriptionID: "sub1",
		AccountID:      "account1",
		Allowed:        false,
		ErrorReason:    "not allowed",
	}
	manager.ForwardPermissionResponse(&permissionMsg)
	blacklisted, secondsLeft := manager.IsAccountBlacklisted("account1")
	assert.True(t, blacklisted)
	assert.Equal(t, 15, secondsLeft)

	time.Sleep(time.Millisecond)
	mc.IncTime(SubscriptionBlacklistTimeout + time.Second)
	time.Sleep(time.Millisecond)

	blacklisted, _ = manager.IsAccountBlacklisted("account1")
	assert.False(t, blacklisted)
}

func TestSubscriptionManager_UnsubscribeLifecycle(t *testing.T) {
	mc := clock.MockClock{}
	manager := NewSubscriptionManager(&mc)

	sub1 := &types.SubscriptionModel{SubscriptionID: "sub1"}
	sub2 := &types.SubscriptionModel{SubscriptionID: "sub2"}

	manager.RecordUnsubscribeRequest(sub1)
	manager.RecordUnsubscribeRequest(sub2)

	// nothing is older than 10s yet
	expired := manager.UnsubscribeEventsOlderThan(10 * time.Second)
	assert.Empty(t, expired)

	// advance time so both events are older than 10s
	mc.IncTime(11 * time.Second)

	expired = manager.UnsubscribeEventsOlderThan(10 * time.Second)
	assert.Len(t, expired, 2)

	// confirm one unsubscribe and verify only the other remains
	manager.ConfirmUnsubscribe(sub1.SubscriptionID)

	expired = manager.UnsubscribeEventsOlderThan(10 * time.Second)
	assert.Len(t, expired, 1)
	assert.Equal(t, sub2.SubscriptionID, expired[0].SubscriptionID)

	manager.ConfirmUnsubscribe(sub2.SubscriptionID)

	expired = manager.UnsubscribeEventsOlderThan(10 * time.Second)
	assert.Empty(t, expired)
}

func TestSubscriptionManager_BlacklistExpiredAndTimeReset(t *testing.T) {
	mc := clock.MockClock{}
	manager := NewSubscriptionManager(&mc)
	responseChannel := manager.RecordSubscriptionRequest("sub1")
	assert.NotNil(t, responseChannel)
	permissionMsg := types.SubscriptionPermissionMessage{
		SubscriptionID: "sub1",
		AccountID:      "account1",
		Allowed:        false,
		ErrorReason:    "not allowed",
	}
	manager.ForwardPermissionResponse(&permissionMsg)
	blacklisted, secondsLeft := manager.IsAccountBlacklisted("account1")
	assert.True(t, blacklisted)
	assert.Equal(t, 15, secondsLeft)

	time.Sleep(time.Millisecond)
	mc.IncTime(SubscriptionBlacklistTimeout + time.Second)
	time.Sleep(time.Millisecond)

	blacklisted, _ = manager.IsAccountBlacklisted("account1")
	assert.False(t, blacklisted)

	responseChannel = manager.RecordSubscriptionRequest("sub2")
	assert.NotNil(t, responseChannel)
	permissionMsg = types.SubscriptionPermissionMessage{
		SubscriptionID: "sub2",
		AccountID:      "account1",
		Allowed:        false,
		ErrorReason:    "not allowed",
	}
	manager.ForwardPermissionResponse(&permissionMsg)
	mc.IncTime(5 * time.Second)
	blacklisted, secondsLeft = manager.IsAccountBlacklisted("account1")
	assert.True(t, blacklisted)
	assert.Equal(t, 10, secondsLeft)
}
