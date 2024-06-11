package services

import (
	"sync"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
)

// IntentsManager interface for mocking
type IntentsManager interface {
	SubscriptionMessages() []bxmessage.Message
	AddIntentsSubscription(solverAddr string, hash, signature []byte)
	RmIntentsSubscription(solverAddr string)
	IntentsSubscriptionExists(solverAddr string) bool
	AddSolutionsSubscription(dAppAddr string, hash, signature []byte)
	RmSolutionsSubscription(dAppAddr string)
	SolutionsSubscriptionExists(dAppAddr string) bool
}

// IntentsManagerImpl is the implementation of IntentsManager
type IntentsManagerImpl struct {
	intentsSubscriptions   map[string]*subscription
	isMx                   *sync.RWMutex
	solutionsSubscriptions map[string]*subscription
	ssMx                   *sync.RWMutex
}

type subscription struct {
	addr      string
	hash      []byte
	signature []byte
}

// NewIntentsManager creates a new IntentsManager
func NewIntentsManager() *IntentsManagerImpl {
	return &IntentsManagerImpl{
		intentsSubscriptions:   make(map[string]*subscription),
		isMx:                   new(sync.RWMutex),
		solutionsSubscriptions: make(map[string]*subscription),
		ssMx:                   new(sync.RWMutex),
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
