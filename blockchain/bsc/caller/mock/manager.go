package mock

import (
	"context"
	"errors"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

// Manager manages callers
type Manager struct {
	Hasher      Hasher
	AddrRespMap map[string]map[string][]byte
	callerMap   *syncmap.SyncMap[string, caller.Caller]
}

// NewManager creates a new caller manager
func NewManager(hasher Hasher, addrRespMap map[string]map[string][]byte) *Manager {
	return &Manager{
		Hasher:      hasher,
		AddrRespMap: addrRespMap,
		callerMap:   syncmap.NewStringMapOf[caller.Caller](),
	}
}

// AddClient adds a new client to the manager
func (m *Manager) AddClient(_ context.Context, addr string) (caller.Caller, error) {
	respMap, exists := m.AddrRespMap[addr]
	if !exists {
		return nil, errors.New("response map not found")
	}

	client := &Caller{Hasher: m.Hasher, RespMap: respMap}
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
