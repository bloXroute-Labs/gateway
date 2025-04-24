package services

import (
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// AccountBurstLimiter represents a service for managing burst limiters for accounts
type AccountBurstLimiter interface {
	AllowTransaction(id bxtypes.AccountID, paid bool) (bool, sdnmessage.BDNServiceBehaviorType)
	Register(account *sdnmessage.Account)
	BurstLimit(id bxtypes.AccountID, paid bool) uint64
	TotalExcess() RateSnapshot
	AccountExcess(id bxtypes.AccountID, paid bool) RateSnapshot
	AccountTotal(id bxtypes.AccountID, paid bool) RateSnapshot
}

// NewAccountBurstLimiter returns a service for managing burst limiters for accounts with leaky bucket burst limiters
func NewAccountBurstLimiter(clock clock.Clock) AccountBurstLimiter {
	return &leakyBucketAccountBurstLimiter{
		accountToLimiter: syncmap.NewTypedMapOf[bxtypes.AccountID, accountLimiter](syncmap.AccountIDHasher),
		clock:            clock,
		totalExcess:      NewRateSnapshot(clock),
	}
}

type leakyBucketAccountBurstLimiter struct {
	accountToLimiter *syncmap.SyncMap[bxtypes.AccountID, accountLimiter]
	clock            clock.Clock
	totalExcess      RateSnapshot
}

type accountLimiter struct {
	unpaidBurstLimiter utils.RateLimiter
	paidBurstLimiter   utils.RateLimiter

	unpaidBehavior sdnmessage.BDNServiceBehaviorType
	paidBehavior   sdnmessage.BDNServiceBehaviorType

	unpaidExcess RateSnapshot
	paidExcess   RateSnapshot
	unpaidTotal  RateSnapshot
	paidTotal    RateSnapshot
}

// AllowTransaction updates burst limit usage of transactions from a given account. Accounts that have not yet been loaded will always be passed.
func (l *leakyBucketAccountBurstLimiter) AllowTransaction(id bxtypes.AccountID, paid bool) (bool, sdnmessage.BDNServiceBehaviorType) {
	al, ok := l.accountLimiter(id)
	if !ok {
		return true, sdnmessage.BehaviorNoAction
	}

	var allowed bool
	if paid {
		allowed, _ = al.paidBurstLimiter.Take()
	} else {
		allowed, _ = al.unpaidBurstLimiter.Take()
	}

	al.count(paid)

	if !allowed {
		al.countExcess(paid)
		l.totalExcess.Track()
	}
	return allowed, l.limitBehavior(id, paid)
}

// Register saves a new account model for use with the burst limiters. New burst limits are re-granted full buckets.
func (l *leakyBucketAccountBurstLimiter) Register(account *sdnmessage.Account) {
	unpaidBurstLimit := account.UnpaidTransactionBurstLimit.MsgQuota.Limit
	unpaidBehavior := account.UnpaidTransactionBurstLimit.MsgQuota.BehaviorLimitFail
	paidBurstLimit := account.PaidTransactionBurstLimit.MsgQuota.Limit
	paidBehavior := account.PaidTransactionBurstLimit.MsgQuota.BehaviorLimitFail

	l.accountToLimiter.Store(account.AccountID, accountLimiter{
		unpaidBurstLimiter: utils.NewLeakyBucketRateLimiter(l.clock, uint64(unpaidBurstLimit), 5*time.Second),
		unpaidBehavior:     unpaidBehavior,
		paidBurstLimiter:   utils.NewLeakyBucketRateLimiter(l.clock, uint64(paidBurstLimit), 5*time.Second),
		paidBehavior:       paidBehavior,
		unpaidExcess:       NewRateSnapshot(l.clock),
		paidExcess:         NewRateSnapshot(l.clock),
		unpaidTotal:        NewRateSnapshot(l.clock),
		paidTotal:          NewRateSnapshot(l.clock),
	})
}

func (l *leakyBucketAccountBurstLimiter) BurstLimit(id bxtypes.AccountID, paid bool) uint64 {
	al, ok := l.accountLimiter(id)
	if !ok {
		return 0
	}
	if paid {
		return al.paidBurstLimiter.Limit()
	}
	return al.unpaidBurstLimiter.Limit()
}

func (l *leakyBucketAccountBurstLimiter) TotalExcess() RateSnapshot {
	return l.totalExcess
}

func (l *leakyBucketAccountBurstLimiter) AccountExcess(id bxtypes.AccountID, paid bool) RateSnapshot {
	al, ok := l.accountLimiter(id)
	if !ok {
		return emptySnapshot
	}
	if paid {
		return al.paidExcess
	}
	return al.unpaidExcess
}

func (l *leakyBucketAccountBurstLimiter) AccountTotal(id bxtypes.AccountID, paid bool) RateSnapshot {
	al, ok := l.accountLimiter(id)
	if !ok {
		return emptySnapshot
	}
	if paid {
		return al.paidTotal
	}
	return al.unpaidTotal
}

func (l *leakyBucketAccountBurstLimiter) limitBehavior(id bxtypes.AccountID, paid bool) sdnmessage.BDNServiceBehaviorType {
	al, ok := l.accountLimiter(id)
	if !ok {
		return sdnmessage.BehaviorNoAction
	}
	if paid {
		return al.paidBehavior
	}
	return al.unpaidBehavior
}

func (l *leakyBucketAccountBurstLimiter) accountLimiter(id bxtypes.AccountID) (accountLimiter, bool) {
	rawAccountLimiter, ok := l.accountToLimiter.Load(id)
	if !ok {
		return accountLimiter{}, false
	}
	return rawAccountLimiter, true
}

func (al *accountLimiter) count(paid bool) {
	if paid {
		al.paidTotal.Track()
	} else {
		al.unpaidTotal.Track()
	}
}

func (al *accountLimiter) countExcess(paid bool) {
	if paid {
		al.paidExcess.Track()
	} else {
		al.unpaidExcess.Track()
	}
}
