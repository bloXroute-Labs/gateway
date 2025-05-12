package validator

import (
	"sync"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/syncmap"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
)

// Manager manages the next validators and their status
type Manager struct {
	nextValidatorMap                    *orderedmap.OrderedMap[uint64, string]
	validatorStatusMap                  *syncmap.SyncMap[string, bool]
	validatorListMap                    *syncmap.SyncMap[uint64, List]
	pendingBSCNextValidatorTxHashToInfo map[string]PendingNextValidatorTxInfo
	lock                                sync.Mutex
}

// List holds a list of validators and turn length
type List struct {
	Validators []string
	TurnLength uint8
}

// PendingNextValidatorTxInfo holds info needed to reevaluate next validator tx when next block published
type PendingNextValidatorTxInfo struct {
	Tx            *bxmessage.Tx
	Fallback      uint16
	TimeOfRequest time.Time
	Source        connections.Conn
}

// NewManager creates a new Manager
func NewManager(validatorStatusMap *syncmap.SyncMap[string, bool], validatorListMap *syncmap.SyncMap[uint64, List]) *Manager {
	return &Manager{
		validatorStatusMap:                  validatorStatusMap,
		validatorListMap:                    validatorListMap,
		pendingBSCNextValidatorTxHashToInfo: make(map[string]PendingNextValidatorTxInfo),
	}
}

// GetPendingNextValidatorTxs returns map of pending next validator transactions
func (m *Manager) GetPendingNextValidatorTxs() map[string]PendingNextValidatorTxInfo {
	return m.pendingBSCNextValidatorTxHashToInfo
}

// GetNextValidatorMap returns an ordered map of next validators
func (m *Manager) GetNextValidatorMap() *orderedmap.OrderedMap[uint64, string] {
	return m.nextValidatorMap
}

// GetValidatorStatusMap returns a synced map validators status
func (m *Manager) GetValidatorStatusMap() *syncmap.SyncMap[string, bool] {
	return m.validatorStatusMap
}

// Lock activates mutex lock
func (m *Manager) Lock() {
	m.lock.Lock()
}

// Unlock activates mutex lock
func (m *Manager) Unlock() {
	m.lock.Unlock()
}
