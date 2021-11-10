package services

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestFirewall_Add(t *testing.T) {
	clock := &bxmock.MockClock{}
	firewall := newFirewall(clock, 30*time.Minute)

	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 10})
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 20})
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 30})

	assert.Equal(t, 3, len(firewall.rules))
	clock.IncTime(25 * time.Second)
	assert.Equal(t, 2, firewall.clean())
	clock.IncTime(35 * time.Second)
	assert.Equal(t, 1, firewall.clean())

	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 20})
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 10})
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 50})
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 40})
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: generateRandAccountID(), PeerID: generateRandNodeID(), Duration: 30})

	assert.Equal(t, 5, len(firewall.rules))
	clock.IncTime(15 * time.Second)
	assert.Equal(t, 1, firewall.clean())

	assert.Equal(t, 4, len(firewall.rules))
	clock.IncTime(10 * time.Second)
	assert.Equal(t, 1, firewall.clean())

	assert.Equal(t, 3, len(firewall.rules))
	clock.IncTime(10 * time.Second)
	assert.Equal(t, 1, firewall.clean())
}

func TestFirewall_ConnectionAllowed(t *testing.T) {
	clock := &bxmock.MockClock{}
	firewall := newFirewall(clock, 30*time.Minute)

	accountID1 := types.AccountID("")
	nodeID1 := types.NodeID("")
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: accountID1, PeerID: nodeID1, Duration: 20})

	rejectConnection := firewall.Validate(accountID1, nodeID1)
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), "")
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate("", generateRandNodeID())
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), generateRandNodeID())
	assert.Nil(t, rejectConnection)

	accountID2 := generateRandAccountID()
	nodeID2 := generateRandNodeID()
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: accountID2, PeerID: nodeID2, Duration: 20})

	rejectConnection = firewall.Validate(accountID2, nodeID2)
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate("", "")
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), "")
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate("", generateRandNodeID())
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), generateRandNodeID())
	assert.Nil(t, rejectConnection)

	accountID3 := types.AccountID("*")
	nodeID3 := generateRandNodeID()
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: accountID3, PeerID: nodeID3, Duration: 20})

	rejectConnection = firewall.Validate("", nodeID3)
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate("", "")
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), "")
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate("", generateRandNodeID())
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), generateRandNodeID())
	assert.Nil(t, rejectConnection)

	accountID4 := generateRandAccountID()
	nodeID4 := types.NodeID("*")
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: accountID4, PeerID: nodeID4, Duration: 20})

	rejectConnection = firewall.Validate("", "")
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate("", nodeID4)
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), "")
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate("", generateRandNodeID())
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), generateRandNodeID())
	assert.Nil(t, rejectConnection)

	accountID5 := types.AccountID("*")
	nodeID5 := types.NodeID("*")
	firewall.AddRule(sdnmessage.FirewallRule{AccountID: accountID5, PeerID: nodeID5, Duration: 20})

	rejectConnection = firewall.Validate("", "")
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate(accountID5, nodeID5)
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), "")
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate("", generateRandNodeID())
	assert.NotNil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), generateRandNodeID())
	assert.NotNil(t, rejectConnection)

	clock.IncTime(35 * time.Second)

	rejectConnection = firewall.Validate("", "")
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(accountID2, nodeID2)
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate("", nodeID3)
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(accountID5, nodeID5)
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), "")
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate("", generateRandNodeID())
	assert.Nil(t, rejectConnection)
	rejectConnection = firewall.Validate(generateRandAccountID(), generateRandNodeID())
	assert.Nil(t, rejectConnection)

	assert.Equal(t, 5, firewall.clean())
}

func generateRandAccountID() types.AccountID {
	id := uuid.NewV1()
	accountID := types.AccountID(id.String())
	return accountID
}

func generateRandNodeID() types.NodeID {
	id := uuid.NewV1()
	nodeID := types.NodeID(id.String())
	return nodeID
}
