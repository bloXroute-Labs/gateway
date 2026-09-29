package validator

import (
	"sync"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/v2/syncmap"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
)

// Manager manages the next validators and their status
type Manager struct {
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

// Lock activates mutex lock
func (m *Manager) Lock() {
	m.lock.Lock()
}

// Unlock activates mutex lock
func (m *Manager) Unlock() {
	m.lock.Unlock()
}
