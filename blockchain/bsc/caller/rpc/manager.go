package rpc

import (
	"context"
	"net/http"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/ethereum/go-ethereum/rpc"
)

// Manager manages callers
type Manager struct {
	callerMap *syncmap.SyncMap[string, caller.Caller]
}

// NewManager creates a new caller manager
func NewManager() *Manager { return &Manager{callerMap: syncmap.NewStringMapOf[caller.Caller]()} }

// AddClient adds a new client to the manager
func (m *Manager) AddClient(ctx context.Context, addr string) (caller.Caller, error) {
	client, err := rpc.DialOptions(ctx, addr, rpc.WithHTTPClient(&http.Client{
		Transport: &http.Transport{
			MaxConnsPerHost:     10,
			MaxIdleConnsPerHost: 10,
			MaxIdleConns:        10,
			IdleConnTimeout:     0, // no timeout
		},
		Timeout: 60 * time.Second,
	}))
	if err != nil {
		return nil, err
	}

	m.callerMap.Store(addr, client)

	return client, nil
}

// GetClient returns a client for the given address
func (m *Manager) GetClient(ctx context.Context, addr string) (caller.Caller, error) {
	client, exists := m.callerMap.Load(addr)
	if exists {
		return client, nil
	}

	return m.AddClient(ctx, addr)
}

// Close closes all callers
func (m *Manager) Close() {
	m.callerMap.Range(func(key string, value caller.Caller) bool {
		if value != nil {
			value.Close()
		}

		m.callerMap.Delete(key)

		return true
	})
}
