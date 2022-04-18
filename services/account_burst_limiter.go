package services

import (
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

// AccountBurstLimiter represents a service for managing burst limiters for accounts
type AccountBurstLimiter interface {
	AllowTransaction(id types.AccountID, paid bool) (bool, sdnmessage.BDNServiceBehaviorType)
	Register(account *sdnmessage.Account)
	BurstLimit(id types.AccountID, paid bool) int
}

// NewAccountBurstLimiter returns a service for managing burst limiters for accounts with leaky bucket burst limiters
func NewAccountBurstLimiter(clock utils.Clock) AccountBurstLimiter {
	return &leakyBucketAccountBurstLimiter{
		accountToLimiter: cmap.New(),
		clock:            clock,
	}
}

type leakyBucketAccountBurstLimiter struct {
	accountToLimiter cmap.ConcurrentMap
	clock            utils.Clock
}

type accountLimiter struct {
	unpaidBurstLimiter utils.RateLimiter
	unpaidBehavior     sdnmessage.BDNServiceBehaviorType
	paidBurstLimiter   utils.RateLimiter
	paidBehavior       sdnmessage.BDNServiceBehaviorType
}

// AllowTransaction updates burst limit usage of transactions from a given account. Accounts that have not yet been loaded will always be passed.
func (l *leakyBucketAccountBurstLimiter) AllowTransaction(id types.AccountID, paid bool) (bool, sdnmessage.BDNServiceBehaviorType) {
	al, ok := l.accountLimiter(id)
	if !ok {
		return true, sdnmessage.BehaviorNoAction
	}

	if paid {
		allowed, _ := al.paidBurstLimiter.Take()
		return allowed, l.limitBehavior(id, paid)
	}
	allowed, _ := al.unpaidBurstLimiter.Take()
	return allowed, l.limitBehavior(id, paid)
}

// Register saves a new account model for use with the burst limiters. New burst limits are re-granted full buckets.
func (l *leakyBucketAccountBurstLimiter) Register(account *sdnmessage.Account) {
	unpaidBurstLimit := account.UnpaidTransactionBurstLimit.MsgQuota.Limit
	unpaidBehavior := account.UnpaidTransactionBurstLimit.MsgQuota.BehaviorLimitFail
	paidBurstLimit := account.PaidTransactionBurstLimit.MsgQuota.Limit
	paidBehavior := account.PaidTransactionBurstLimit.MsgQuota.BehaviorLimitFail

	l.accountToLimiter.Set(string(account.AccountID), accountLimiter{
		unpaidBurstLimiter: utils.NewLeakyBucketRateLimiter(l.clock, unpaidBurstLimit, 5*time.Second),
		unpaidBehavior:     unpaidBehavior,
		paidBurstLimiter:   utils.NewLeakyBucketRateLimiter(l.clock, paidBurstLimit, 5*time.Second),
		paidBehavior:       paidBehavior,
	})
}

func (l *leakyBucketAccountBurstLimiter) BurstLimit(id types.AccountID, paid bool) int {
	al, ok := l.accountLimiter(id)
	if !ok {
		return 0
	}
	if paid {
		return al.paidBurstLimiter.Limit()
	}
	return al.unpaidBurstLimiter.Limit()
}

func (l *leakyBucketAccountBurstLimiter) limitBehavior(id types.AccountID, paid bool) sdnmessage.BDNServiceBehaviorType {
	al, ok := l.accountLimiter(id)
	if !ok {
		return sdnmessage.BehaviorNoAction
	}
	if paid {
		return al.paidBehavior
	}
	return al.unpaidBehavior
}

func (l *leakyBucketAccountBurstLimiter) accountLimiter(id types.AccountID) (accountLimiter, bool) {
	rawAccountLimiter, ok := l.accountToLimiter.Get(string(id))
	if !ok {
		return accountLimiter{}, false
	}
	return rawAccountLimiter.(accountLimiter), true
}
