package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
)

const (
	solutionsForIntentExpiry = time.Second * 10
	expiredSolutionsCheck    = time.Second
)

// IntentsManager interface responsible for managing intents and solutions
type IntentsManager interface {
	IntentSubscriptionsManager
	IntentSolutionsManager
	IntentsStatsManager
}

// IntentSubscriptionsManager responsible for managing client subscriptions
type IntentSubscriptionsManager interface {
	SubscriptionMessages() []bxmessage.Message
	AddIntentsSubscription(solverAddr string, hash, signature []byte)
	RmIntentsSubscription(solverAddr string)
	IntentsSubscriptionExists(solverAddr string) bool
	AddSolutionsSubscription(dAppAddr string, hash, signature []byte)
	RmSolutionsSubscription(dAppAddr string)
	SolutionsSubscriptionExists(dAppAddr string) bool
	AddQuotesSubscription(dAppAddr string) uint64
	RmQuotesSubscription(dAppAddr string) (uint64, error)
}

// IntentSolutionsManager responsible for managing internal short-lived  gw <-> relay(s) subscription
type IntentSolutionsManager interface {
	AddIntentOfInterest(intentID string)
	AppendSolutionsForIntent(solutions *bxmessage.IntentSolutions) // receive from relays
	AppendSolutionForIntent(solution *bxmessage.IntentSolution)    // receive from relays
	SolutionsForIntent(intentID string) []bxmessage.IntentSolution
	CleanupExpiredSolutions(ctx context.Context)
}

// IntentsStatsManager keeps track of intent, solution and quote submissions
type IntentsStatsManager interface {
	IncIntentSubmissions()
	IncSolutionSubmissions()
	IncQuoteSubmissions()
	TotalIntentSubmissions() uint64
	TotalSolutionSubmissions() uint64
	TotalQuoteSubmissions() uint64
}

// IntentsManagerImpl is the implementation of IntentsManager
type IntentsManagerImpl struct {
	intentsSubscriptions                            map[string]*subscription
	isMx                                            *sync.RWMutex
	solutionsSubscriptions                          map[string]*subscription
	ssMx                                            *sync.RWMutex
	solutionsForIntent                              map[string]solutionsForIntentWExp // intentID -> solutions
	sfiMx                                           *sync.RWMutex
	quotesSubscriptions                             map[string]*quoteSubscription
	quotesMx                                        *sync.RWMutex
	intentsCounter, solutionsCounter, quotesCounter *atomic.Uint64
}

type quoteSubscription struct {
	addr    string
	counter uint64
}

type subscription struct {
	addr      string
	hash      []byte
	signature []byte
}

type solutionsForIntentWExp struct {
	solutions map[string]bxmessage.IntentSolution // solutionID -> solution
	expiry    int64
}

// NewIntentsManager creates a new IntentsManager
func NewIntentsManager() *IntentsManagerImpl {
	return &IntentsManagerImpl{
		intentsSubscriptions:   make(map[string]*subscription),
		isMx:                   new(sync.RWMutex),
		solutionsSubscriptions: make(map[string]*subscription),
		ssMx:                   new(sync.RWMutex),
		solutionsForIntent:     make(map[string]solutionsForIntentWExp),
		sfiMx:                  new(sync.RWMutex),
		quotesSubscriptions:    make(map[string]*quoteSubscription),
		quotesMx:               new(sync.RWMutex),
		intentsCounter:         &atomic.Uint64{},
		solutionsCounter:       &atomic.Uint64{},
		quotesCounter:          &atomic.Uint64{},
	}
}

// SubscriptionMessages returns all the subscription messages
func (i *IntentsManagerImpl) SubscriptionMessages() []bxmessage.Message {
	var m = make([]bxmessage.Message, 0)

	i.isMx.RLock()
	for _, v := range i.intentsSubscriptions {
		m = append(m, bxmessage.NewIntentsSubscription(v.addr, v.hash, v.signature))
	}
	i.isMx.RUnlock()

	i.ssMx.RLock()
	for _, v := range i.solutionsSubscriptions {
		m = append(m, bxmessage.NewSolutionsSubscription(v.addr, v.hash, v.signature))
	}
	i.ssMx.RUnlock()

	i.quotesMx.RLock()
	for _, v := range i.quotesSubscriptions {
		m = append(m, bxmessage.NewQuotesSubscription(v.addr))
	}
	i.quotesMx.RUnlock()

	return m
}

// AddIntentsSubscription adds an intent subscription
func (i *IntentsManagerImpl) AddIntentsSubscription(solverAddr string, hash, signature []byte) {
	i.isMx.Lock()
	defer i.isMx.Unlock()
	i.intentsSubscriptions[solverAddr] = &subscription{
		addr:      solverAddr,
		hash:      hash,
		signature: signature,
	}
}

// RmIntentsSubscription removes an intent subscription
func (i *IntentsManagerImpl) RmIntentsSubscription(solverAddr string) {
	i.isMx.Lock()
	defer i.isMx.Unlock()
	delete(i.intentsSubscriptions, solverAddr)
}

// IntentsSubscriptionExists checks if an intent subscription exists
func (i *IntentsManagerImpl) IntentsSubscriptionExists(solverAddr string) bool {
	i.isMx.RLock()
	defer i.isMx.RUnlock()
	_, ok := i.intentsSubscriptions[solverAddr]
	return ok
}

// AddQuotesSubscription adds a quote subscription, return how many subscriptions there are
func (i *IntentsManagerImpl) AddQuotesSubscription(dAppAddr string) uint64 {
	i.quotesMx.Lock()
	defer i.quotesMx.Unlock()
	if sub, exist := i.quotesSubscriptions[dAppAddr]; exist {
		sub.counter++
		return sub.counter
	}
	i.quotesSubscriptions[dAppAddr] = &quoteSubscription{
		addr:    dAppAddr,
		counter: 1,
	}
	return 1
}

// AddSolutionsSubscription adds a solutions subscription
func (i *IntentsManagerImpl) AddSolutionsSubscription(dappAddr string, hash, signature []byte) {
	i.ssMx.Lock()
	defer i.ssMx.Unlock()
	i.solutionsSubscriptions[dappAddr] = &subscription{
		addr:      dappAddr,
		hash:      hash,
		signature: signature,
	}
}

// RmQuotesSubscription removes a quote subscription
func (i *IntentsManagerImpl) RmQuotesSubscription(dAppAddr string) (uint64, error) {
	i.quotesMx.Lock()
	defer i.quotesMx.Unlock()
	if sub, exist := i.quotesSubscriptions[dAppAddr]; exist {
		sub.counter--
		if sub.counter == 0 {
			delete(i.quotesSubscriptions, dAppAddr)
		}
		return sub.counter, nil
	}
	return 0, ErrQuoteNotExist
}

// RmSolutionsSubscription removes a solutions subscription
func (i *IntentsManagerImpl) RmSolutionsSubscription(dAppAddr string) {
	i.ssMx.Lock()
	defer i.ssMx.Unlock()
	delete(i.solutionsSubscriptions, dAppAddr)
}

// SolutionsSubscriptionExists checks if a solutions subscription exists
func (i *IntentsManagerImpl) SolutionsSubscriptionExists(dAppAddr string) bool {
	i.ssMx.Lock()
	defer i.ssMx.Unlock()
	_, ok := i.solutionsSubscriptions[dAppAddr]
	return ok
}

// AddIntentOfInterest adds a record that we are interested in solutions for an intent from the relay(s)
func (i *IntentsManagerImpl) AddIntentOfInterest(intentID string) {
	i.sfiMx.Lock()
	defer i.sfiMx.Unlock()

	sol, ok := i.solutionsForIntent[intentID] // check if we already have a record
	if ok {
		sol.expiry = time.Now().Unix() + int64(solutionsForIntentExpiry.Seconds()) // reset expiry
		i.solutionsForIntent[intentID] = sol
		return
	}

	i.solutionsForIntent[intentID] = solutionsForIntentWExp{
		solutions: make(map[string]bxmessage.IntentSolution),
		expiry:    time.Now().Unix() + int64(solutionsForIntentExpiry.Seconds()),
	}
}

// AppendSolutionsForIntent adds solutions for an intent from the relay(s).
// Expiration time is not updated here, since if there is no interest in the intent, we should not keep the solutions.
func (i *IntentsManagerImpl) AppendSolutionsForIntent(message *bxmessage.IntentSolutions) {
	i.sfiMx.Lock()
	defer i.sfiMx.Unlock()

	for _, s := range message.Solutions() {
		sol, ok := i.solutionsForIntent[s.IntentID]
		if !ok {
			continue // ignore solutions for intents we are not interested in
		}

		sol.solutions[s.ID] = s                // add solution
		i.solutionsForIntent[s.IntentID] = sol // update
	}
}

// AppendSolutionForIntent adds a solution for an intent from the relay(s)
// Expiration time is not updated here, since if there is no interest in the intent, we should not keep the solutions.
func (i *IntentsManagerImpl) AppendSolutionForIntent(message *bxmessage.IntentSolution) {
	i.sfiMx.Lock()
	defer i.sfiMx.Unlock()

	sol, ok := i.solutionsForIntent[message.IntentID]
	if !ok {
		return
	}

	sol.solutions[message.ID] = *message
	i.solutionsForIntent[message.IntentID] = sol
}

// SolutionsForIntent returns solutions for an intent
func (i *IntentsManagerImpl) SolutionsForIntent(intentID string) []bxmessage.IntentSolution {
	i.sfiMx.Lock()
	defer i.sfiMx.Unlock()

	sol, ok := i.solutionsForIntent[intentID]
	if !ok {
		return nil
	}

	solutions := make([]bxmessage.IntentSolution, 0, len(sol.solutions))
	for _, s := range sol.solutions {
		solutions = append(solutions, s)
	}

	return solutions
}

// CleanupExpiredSolutions removes expired solutions from memory
func (i *IntentsManagerImpl) CleanupExpiredSolutions(ctx context.Context) {
	ticker := time.NewTicker(expiredSolutionsCheck)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.sfiMx.Lock()
			for intentID, v := range i.solutionsForIntent {
				if time.Now().Unix() > v.expiry {
					delete(i.solutionsForIntent, intentID)
				}
			}
			i.sfiMx.Unlock()
		}
	}
}

// IncIntentSubmissions increments the intent submissions counter by 1
func (i *IntentsManagerImpl) IncIntentSubmissions() {
	i.intentsCounter.Add(1)
}

// IncSolutionSubmissions increments the solution submissions counter by 1
func (i *IntentsManagerImpl) IncSolutionSubmissions() {
	i.solutionsCounter.Add(1)
}

// IncQuoteSubmissions increments the quote submissions counter by 1
func (i *IntentsManagerImpl) IncQuoteSubmissions() {
	i.quotesCounter.Add(1)
}

// TotalIntentSubmissions returns the total number of intent submissions
func (i *IntentsManagerImpl) TotalIntentSubmissions() uint64 {
	return i.intentsCounter.Load()
}

// TotalSolutionSubmissions returns the total number of solution submissions
func (i *IntentsManagerImpl) TotalSolutionSubmissions() uint64 {
	return i.solutionsCounter.Load()
}

// TotalQuoteSubmissions returns the total number of quote submissions
func (i *IntentsManagerImpl) TotalQuoteSubmissions() uint64 {
	return i.quotesCounter.Load()
}
