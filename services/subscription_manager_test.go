package services

import (
	"testing"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/stretchr/testify/assert"
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
