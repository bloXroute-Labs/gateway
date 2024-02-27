package blockproposer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

type mockClock struct{}

func (m *mockClock) Now() (r time.Time)                              { return }
func (m *mockClock) Timer(time.Duration) (r utils.Timer)             { return }
func (m *mockClock) Sleep(time.Duration)                             {}
func (m *mockClock) AfterFunc(time.Duration, func()) (r *time.Timer) { return }
func (m *mockClock) Ticker(time.Duration) (r utils.Ticker)           { return }

type mockTxStore struct{}

func (m mockTxStore) Start() (r error) { return }
func (m mockTxStore) Stop()            {}
func (m mockTxStore) Add(types.SHA256Hash, types.TxContent, types.ShortID, types.NetworkNum, bool, types.TxFlags, time.Time, int64, types.Sender) (r services.TransactionResult) {
	return
}
func (m mockTxStore) Get(types.SHA256Hash) (r *types.BxTransaction, e bool) { return }
func (m mockTxStore) Known(types.SHA256Hash) (r bool)                       { return }
func (m mockTxStore) HasContent(types.SHA256Hash) (r bool)                  { return }
func (m mockTxStore) RemoveShortIDs(*types.ShortIDList, services.ReEntryProtectionFlags, string) {
}
func (m mockTxStore) RemoveHashes(*types.SHA256HashList, services.ReEntryProtectionFlags, string) {}
func (m mockTxStore) GetTxByShortID(types.ShortID) (r *types.BxTransaction, e error)              { return }
func (m mockTxStore) Clear()                                                                      {}
func (m mockTxStore) Iter() (r <-chan *types.BxTransaction)                                       { return }
func (m mockTxStore) Count() (r int)                                                              { return }
func (m mockTxStore) Summarize() (r *pb.TxStoreReply)                                             { return }
func (m mockTxStore) CleanNow()                                                                   {}

type mockCallerManager struct{}

func (m mockCallerManager) AddClient(context.Context, string) (r caller.Caller, e error) { return }
func (m mockCallerManager) GetClient(context.Context, string) (r caller.Caller, e error) { return }
func (m mockCallerManager) Close()                                                       {}

type mockTicker struct{}

func (m mockTicker) Run(context.Context) (r error) { return }
func (m mockTicker) Alert() (r <-chan struct{})    { return }
func (m mockTicker) Reset(time.Time) (r bool)      { return }

// helper function to create a valid config for tests
func validConfig() *Config {
	sendingConfig := new(SendingConfig)
	sendingConfig.Enabled = true
	return &Config{
		Clock:          new(mockClock),
		TxStore:        func() *services.TxStore { var s services.TxStore = new(mockTxStore); return &s }(),
		Log:            logger.TestEntry(),
		CallerManager:  new(mockCallerManager),
		SendingInfo:    sendingConfig,
		RegularTicker:  new(mockTicker),
		HighLoadTicker: new(mockTicker),
		BlocksToCache:  10,
	}
}

func TestConfigValidate(t *testing.T) {
	t.Run("Valid configuration", func(t *testing.T) {
		config := validConfig()
		config.SendingInfo = &SendingConfig{Enabled: true}

		require.NoError(t, config.Validate())

	})

	t.Run("Valid configuration", func(t *testing.T) {
		config := validConfig()
		require.NoError(t, config.Validate())
	})

	t.Run("Clock is not initialized", func(t *testing.T) {
		config := validConfig()
		config.Clock = nil
		require.EqualError(t, config.Validate(), errClockNotInitialized.Error())
	})

	t.Run("TxStore is not initialized", func(t *testing.T) {
		config := validConfig()
		config.TxStore = nil
		require.EqualError(t, config.Validate(), errTxStoreNotInitialized.Error())
	})

	t.Run("Log is not initialized", func(t *testing.T) {
		config := validConfig()
		config.Log = nil
		require.EqualError(t, config.Validate(), errLogNotInitialized.Error())
	})

	t.Run("CallerManager is not initialized", func(t *testing.T) {
		config := validConfig()
		config.CallerManager = nil
		require.EqualError(t, config.Validate(), errCallerManager.Error())
	})

	t.Run("RegularTicker is not initialized", func(t *testing.T) {
		config := validConfig()
		config.RegularTicker = nil
		require.ErrorIs(t, config.Validate(), errTickerNotInitialized)
	})

	t.Run("HighLoadTicker is not initialized", func(t *testing.T) {
		config := validConfig()
		config.HighLoadTicker = nil
		require.ErrorIs(t, config.Validate(), errTickerNotInitialized)
	})
}
