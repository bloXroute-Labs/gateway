package bor

import (
	"context"
	"strings"
	"sync"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/cenkalti/backoff/v4"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	log "github.com/bloXroute-Labs/gateway/v2/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// SprintSize represent size of sprint for polygon bor module
const SprintSize = uint64(16)

// SprintSizeHeimdall represent size of sprint for polygon heimdall module
const SprintSizeHeimdall = SprintSize * 4

var errQuerySnapshot = errors.New("failed to query blockchain node for snapshot")

// SprintManager basic client for processing bor sprints.
type SprintManager struct {
	ctx context.Context

	mx *sync.RWMutex

	booting *atomic.Bool
	started *atomic.Bool

	// pointer to the interface to make it managed externally
	wsManager *blockchain.WSManager

	spanner Spanner

	syncCh chan struct{}

	sprintMap map[uint64]string
}

// NewSprintManager creates a new SprintManager.
func NewSprintManager(ctx context.Context, wsManager *blockchain.WSManager, spanner Spanner) *SprintManager {
	return &SprintManager{
		ctx:       ctx,
		spanner:   spanner,
		wsManager: wsManager,

		mx: new(sync.RWMutex),

		booting: new(atomic.Bool),
		started: new(atomic.Bool),

		sprintMap: make(map[uint64]string),

		syncCh: make(chan struct{}),
	}
}

func (m *SprintManager) bootstrap() error {
	err := m.ctx.Err()
	if err != nil {
		return err
	}

	backOff := backoff.WithContext(Retry(), m.ctx)

	if err = backoff.Retry(m.spanner.Run, backOff); err != nil {
		return err
	}

	backOff.Reset()
	if err = backoff.Retry(m.updateSprintMapFromLatest, backOff); err != nil {
		return err
	}

	return nil
}

func (m *SprintManager) updateSprintMapFromLatest() error {
	snapshot, err := m.getLatestSnapshot()
	if err != nil {
		return err
	}

	currentSpan, err := m.spanner.GetSpanForHeight(snapshot.Number)
	if err != nil {
		return err
	}

	nextSpan, err := m.spanner.GetSpanByID(currentSpan.SpanID + 1)
	if err != nil {
		return err
	}

	sprintMap, err := getSprintValidatorsMap(currentSpan, nextSpan, snapshot)
	if err != nil {
		return errors.WithMessage(err, "failed to generate sprint validators map")
	}

	m.mx.Lock()
	m.sprintMap = sprintMap
	m.mx.Unlock()

	return nil
}

func getSprintValidatorsMap(currentSpan *SpanInfo, nextSpan *SpanInfo, snapshot *Snapshot) (map[uint64]string, error) {
	var err error

	sprintMap := make(map[uint64]string)

	sprintMap[snapshot.Number/SprintSize] = strings.ToLower(snapshot.ValidatorSet.GetProposer().Address.String())

	for snapshot.Number+SprintSize < nextSpan.EndBlock {
		snapshot, err = snapshot.IncrementSprint(currentSpan, nextSpan)
		if err != nil {
			return nil, err
		}

		sprintMap[snapshot.Number/SprintSize] = strings.ToLower(snapshot.ValidatorSet.GetProposer().Address.String())
	}

	return sprintMap, nil
}

func (m *SprintManager) getLatestSnapshot() (*Snapshot, error) {
	if m.wsManager == nil {
		return nil, errors.WithMessage(errQuerySnapshot, "no ws manager")
	}

	for _, wsProvider := range (*m.wsManager).Providers() {
		if !wsProvider.IsOpen() {
			continue
		}

		snapshot, err := GetLatestSnapshot(wsProvider)
		if err != nil {
			wsProvider.Log().Tracef("Failed to fetch snapshot by height: %v", err)

			continue
		}

		return snapshot, nil
	}

	return nil, errors.WithMessage(errQuerySnapshot, "no open providers")
}

// Run bootstrap initial state and start goroutine for processing of changes.
func (m *SprintManager) Run() error {
	if m.started.Load() || m.booting.Load() {
		return nil
	}

	m.booting.Store(true)
	defer m.booting.Store(false)

	if err := m.bootstrap(); err != nil {
		return errors.WithMessage(err, "failed to bootstrap sprint manager")
	}

	// cleanup span notification channel after bootstrap
	select {
	default:
	case <-m.spanner.GetSpanNotificationCh():
	}

	m.started.Store(true)

	go func() {
		for {
			select {
			case <-m.ctx.Done():
				m.started.Store(false)

				return
			case <-m.spanner.GetSpanNotificationCh():
				if err := m.updateSprintMapFromLatest(); err != nil {
					log.Warnf("failed to update validators for the latest span: %v", err)
				}
			case <-m.syncCh:
				if err := m.updateSprintMapFromLatest(); err != nil {
					log.Warnf("failed to update validators for the latest span: %v", err)
				}
			}
		}
	}()

	return nil
}

// IsRunning returns current state of SprintManager.
func (m *SprintManager) IsRunning() bool { return m.started.Load() }

// FutureValidators returns next n+2 producers of block.
func (m *SprintManager) FutureValidators(header *ethtypes.Header) [2]*types.FutureValidatorInfo {
	height := header.Number.Uint64()

	signer, err := Ecrecover(header)
	if err != nil {
		log.WithField("blockHeight", height).Warnf("failed to recover signer from header: %v", err)

		return emptyFutureValidatorInfo(height)
	}

	validatorInfo := StaticFutureValidatorInfo(height, strings.ToLower(signer.String()))

	var (
		validator string
		exists    bool
	)

	m.mx.RLock()
	validator, exists = m.sprintMap[SprintNum(header.Number.Uint64())]
	m.mx.RUnlock()

	if exists && validator != strings.ToLower(signer.String()) {
		spanInfo, err := m.spanner.GetSpanByID(GetSpanIDByHeight(header.Number.Uint64()))
		if err != nil {
			log.WithField("blockHeight", height).Warnf("failed to get span info: %v", err)

			return validatorInfo
		}

		if header.Difficulty.Uint64() == spanInfo.Difficulty() {
			log.Debugf("invalid future validator prediction detected: predicted(%s) actual(%s)", validator, strings.ToLower(signer.String()))

			select {
			default:
			case m.syncCh <- struct{}{}:
			}

			return validatorInfo
		}
	}

	if validatorInfo[1].WalletID != "nil" {
		return validatorInfo
	}

	m.mx.RLock()
	for i, info := range validatorInfo {
		if validator, exists = m.sprintMap[SprintNum(info.BlockHeight)]; exists {
			validatorInfo[i].WalletID = validator
		}
	}
	m.mx.RUnlock()

	return validatorInfo
}

// StaticFutureValidatorInfo that can be recovered from block header.
func StaticFutureValidatorInfo(height uint64, producer string) [2]*types.FutureValidatorInfo {
	if IsSprintStart(height + 1) {
		return emptyFutureValidatorInfo(height)
	}

	if SprintStart(height+2) <= height {
		return [2]*types.FutureValidatorInfo{
			{BlockHeight: height + 1, WalletID: producer},
			{BlockHeight: height + 2, WalletID: producer},
		}
	}

	if IsSprintStart(height + 2) {
		return [2]*types.FutureValidatorInfo{
			{BlockHeight: height + 1, WalletID: producer},
			{BlockHeight: height + 2, WalletID: "nil"},
		}
	}

	return emptyFutureValidatorInfo(height)
}

func emptyFutureValidatorInfo(height uint64) [2]*types.FutureValidatorInfo {
	return [2]*types.FutureValidatorInfo{
		{BlockHeight: height + 1, WalletID: "nil"},
		{BlockHeight: height + 2, WalletID: "nil"},
	}
}

// SprintNum helper which returns sprint number for provided blockHeight.
func SprintNum(blockHeight uint64) uint64 {
	return blockHeight / SprintSize
}

// SprintStart helper which returns the closest sprint start for provided blockHeight.
func SprintStart(blockHeight uint64) uint64 {
	return blockHeight / SprintSize * SprintSize
}

// IsSprintStart helper which indicates if provided blockHeight is start of sprint.
func IsSprintStart(blockHeight uint64) bool {
	return blockHeight%SprintSize == 0
}
