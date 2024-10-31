package validator

import (
	"errors"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

// Manager manages the next validators and their status
type Manager struct {
	nextValidatorMap                    *orderedmap.OrderedMap[uint64, string]
	validatorStatusMap                  *syncmap.SyncMap[string, bool]
	validatorListMap                    *syncmap.SyncMap[uint64, []string]
	pendingBSCNextValidatorTxHashToInfo map[string]PendingNextValidatorTxInfo
	lock                                sync.Mutex
}

// PendingNextValidatorTxInfo holds info needed to reevaluate next validator tx when next block published
type PendingNextValidatorTxInfo struct {
	Tx            *bxmessage.Tx
	Fallback      uint16
	TimeOfRequest time.Time
	Source        connections.Conn
}

// NewManager creates a new Manager
func NewManager(nextValidatorMap *orderedmap.OrderedMap[uint64, string], validatorStatusMap *syncmap.SyncMap[string, bool], validatorListMap *syncmap.SyncMap[uint64, []string]) *Manager {
	return &Manager{
		nextValidatorMap:                    nextValidatorMap,
		validatorStatusMap:                  validatorStatusMap,
		validatorListMap:                    validatorListMap,
		pendingBSCNextValidatorTxHashToInfo: make(map[string]PendingNextValidatorTxInfo),
	}
}

// ProcessNextValidatorTx - sets next validator wallets if accessible and returns bool indicating if tx is pending reevaluation due to inaccessible first validator for BSC
func (m *Manager) ProcessNextValidatorTx(tx *bxmessage.Tx, fallback uint16, networkNum types.NetworkNum, source connections.Conn) (bool, error) {
	if networkNum != bxgateway.BSCMainnetNum && networkNum != bxgateway.PolygonMainnetNum {
		return false, errors.New("currently next_validator is only supported on BSC and Polygon networks, please contact bloXroute support")
	}

	if m == nil {
		log.Errorf("failed to process next validator tx, because next validator map is nil, tx %v", tx.Hash().String())
		return false, errors.New("failed to send next validator tx, please contact bloXroute support")
	}

	tx.SetFallback(fallback)

	// take the latest two blocks from the ordered map for updating txMsg walletID
	n2Validator := m.nextValidatorMap.Newest()
	if n2Validator == nil {
		return false, errors.New("can't send tx with next_validator because the gateway encountered an issue fetching the epoch block, please try again later or contact bloXroute support")
	}

	if networkNum == bxgateway.BSCMainnetNum {
		n1Validator := n2Validator.Prev()
		n1ValidatorAccessible := false
		n1Wallet := ""
		if n1Validator != nil {
			n1Wallet = n1Validator.Value
			accessible, exist := m.validatorStatusMap.Load(n1Wallet)
			if exist {
				n1ValidatorAccessible = accessible
			}
		}

		if n1ValidatorAccessible {
			tx.SetWalletID(0, n1Wallet)
		} else {
			blockIntervalBSC := bxgateway.NetworkToBlockDuration[bxgateway.BSCMainnet]
			if fallback != 0 && fallback < uint16(blockIntervalBSC.Milliseconds()) {
				return false, nil
			}
			m.pendingBSCNextValidatorTxHashToInfo[tx.Hash().String()] = PendingNextValidatorTxInfo{
				Tx:            tx,
				Fallback:      fallback,
				TimeOfRequest: time.Now(),
				Source:        source,
			}
			return true, nil
		}
	}

	if networkNum == bxgateway.PolygonMainnetNum || networkNum == bxgateway.PolygonMumbaiNum {
		n1Validator := n2Validator.Prev()
		if n1Validator != nil {
			tx.SetWalletID(0, n1Validator.Value)
			tx.SetWalletID(1, n2Validator.Value)
		} else {
			tx.SetWalletID(0, n2Validator.Value)
		}
	}

	return false, nil
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
