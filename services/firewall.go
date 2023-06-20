package services

import (
	"fmt"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// FirewallRulesCleanupInterval - is cleanup interval for firewall rules
const FirewallRulesCleanupInterval = 15 * time.Minute

// Firewall - is manager for FirewallRules messages
type Firewall struct {
	rules []sdnmessage.FirewallRule
	lock  sync.RWMutex
	clock utils.Clock
}

// NewFirewall returns new manager for FirewallRules
func NewFirewall(cleanupInterval time.Duration) *Firewall {
	return newFirewall(utils.RealClock{}, cleanupInterval)
}

func newFirewall(clock utils.Clock, cleanupInterval time.Duration) *Firewall {
	firewall := &Firewall{
		rules: []sdnmessage.FirewallRule{},
		clock: clock,
	}
	log.Tracef("starting new firewall")
	go firewall.cleanup(cleanupInterval)
	return firewall
}

// AddRule - add a new firewallRule
func (firewall *Firewall) AddRule(firewallRule sdnmessage.FirewallRule) {
	firewallRule.SetExpirationTime(firewall.clock.Now().Add(time.Duration(firewallRule.Duration) * time.Second))
	firewall.lock.Lock()
	defer firewall.lock.Unlock()
	for i, rule := range firewall.rules {
		if rule.AccountID == firewallRule.AccountID && rule.PeerID == firewallRule.PeerID {
			firewall.rules[i] = firewallRule
			log.Debugf("firewall rule for account ID %v, node ID %v has changed, old rule was %v seconds, new rule is %v seconds", rule.AccountID, rule.PeerID, rule.Duration, firewallRule.Duration)
			return
		}
	}
	firewall.rules = append(firewall.rules, firewallRule)
	log.Debugf("firewall: new rule %v added", firewallRule)
}

func (firewall *Firewall) cleanup(cleanupInterval time.Duration) {
	log.Debugf("starting firewall cleanup routine")
	ticker := firewall.clock.Ticker(cleanupInterval)
	for {
		select {
		case <-ticker.Alert():
			firewall.clean()
		}
	}
}

func (firewall *Firewall) clean() int {
	timeNow := firewall.clock.Now()
	remainedRules := make([]sdnmessage.FirewallRule, 0)
	firewall.lock.Lock()
	defer firewall.lock.Unlock()
	for _, rule := range firewall.rules {
		if timeNow.Before(rule.GetExpirationTime()) {
			remainedRules = append(remainedRules, rule)
		}
	}
	cleaned := len(firewall.rules) - len(remainedRules)
	firewall.rules = remainedRules
	log.Debugf("firewall: %v rules has been cleaned", cleaned)
	return cleaned
}

// Validate - return error message if connection should be rejected
func (firewall *Firewall) Validate(accountID types.AccountID, nodeID types.NodeID) error {
	firewall.lock.RLock()
	defer firewall.lock.RUnlock()
	for _, rule := range firewall.rules {
		if firewall.clock.Now().Before(rule.GetExpirationTime()) &&
			(rule.AccountID == "*" || rule.AccountID == accountID) &&
			(rule.PeerID == "*" || rule.PeerID == nodeID) {
			return fmt.Errorf("connection with accountID %v and peerID %v is forbidden by firewall - reason: %v",
				rule.AccountID, rule.PeerID, rule.Reason)
		}
	}
	return nil
}
