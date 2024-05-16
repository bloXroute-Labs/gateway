package blockproposer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	testDefaultRegularInitialDelay   = time.Duration(utils.DefaultRegularBlockSendDelayInitialMS) * time.Millisecond
	testDefaultRegularSecondDelay    = time.Duration(utils.DefaultRegularBlockSendDelaySecondMS) * time.Millisecond
	testDefaultRegularIntervalDelay  = time.Duration(utils.DefaultRegularBlockSendDelayIntervalMS) * time.Millisecond
	testDefaultHighLoadInitialDelay  = time.Duration(utils.DefaultHighLoadBlockSendDelayInitialMS) * time.Millisecond
	testDefaultHighLoadSecondDelay   = time.Duration(utils.DefaultHighLoadBlockSendDelaySecondMS) * time.Millisecond
	testDefaultHighLoadIntervalDelay = time.Duration(utils.DefaultHighLoadBlockSendDelayIntervalMS) * time.Millisecond
	testDefaultHighLoadThreshold     = utils.DefaultHighLoadTxNumThreshold
)

var (
	customInitialDelay   = 2 * time.Second
	customSecondDelay    = 3 * time.Second
	customIntervalDelay  = 1 * time.Second
	customTxNumThreshold = 100
)

func TestSendingConfig_Validate(t *testing.T) {
	log := logger.Discard()

	tests := []struct {
		name       string
		config     SendingConfig
		expectFunc func(*require.Assertions, *SendingConfig)
	}{
		{
			name:   "All testDefaults",
			config: SendingConfig{},
			expectFunc: func(r *require.Assertions, c *SendingConfig) {
				r.Equal(testDefaultRegularInitialDelay, c.RegularBlockSendDelayInitial)
				r.Equal(testDefaultRegularSecondDelay, c.RegularBlockSendDelaySecond)
				r.Equal(testDefaultRegularIntervalDelay, c.RegularBlockSendDelayInterval)
				r.Equal(testDefaultHighLoadInitialDelay, c.HighLoadBlockSendDelayInitial)
				r.Equal(testDefaultHighLoadSecondDelay, c.HighLoadBlockSendDelaySecond)
				r.Equal(testDefaultHighLoadIntervalDelay, c.HighLoadBlockSendDelayInterval)
				r.Equal(testDefaultHighLoadThreshold, c.HighLoadTxNumThreshold)
			},
		},
		{
			name: "Valid custom settings",
			config: SendingConfig{
				RegularBlockSendDelayInitial:   customInitialDelay,
				RegularBlockSendDelaySecond:    customSecondDelay,
				RegularBlockSendDelayInterval:  customIntervalDelay,
				HighLoadBlockSendDelayInitial:  customInitialDelay,
				HighLoadBlockSendDelaySecond:   customSecondDelay,
				HighLoadBlockSendDelayInterval: customIntervalDelay,
				HighLoadTxNumThreshold:         customTxNumThreshold,
			},
			expectFunc: func(r *require.Assertions, c *SendingConfig) {
				r.Equal(customInitialDelay, c.RegularBlockSendDelayInitial)
				r.Equal(customSecondDelay, c.RegularBlockSendDelaySecond)
				r.Equal(customIntervalDelay, c.RegularBlockSendDelayInterval)
				r.Equal(customInitialDelay, c.HighLoadBlockSendDelayInitial)
				r.Equal(customSecondDelay, c.HighLoadBlockSendDelaySecond)
				r.Equal(customIntervalDelay, c.HighLoadBlockSendDelayInterval)
				r.Equal(customTxNumThreshold, c.HighLoadTxNumThreshold)
			},
		},
		{
			name: "Invalid second delay less than initial",
			config: SendingConfig{
				RegularBlockSendDelayInitial:  customSecondDelay,  // Intentionally swapped to create an invalid setup
				RegularBlockSendDelaySecond:   customInitialDelay, // Invalid configuration
				HighLoadBlockSendDelayInitial: customSecondDelay,  // Intentionally swapped to create an invalid setup
				HighLoadBlockSendDelaySecond:  customInitialDelay, // Invalid configuration
			},
			expectFunc: func(r *require.Assertions, c *SendingConfig) {
				r.Equal(customSecondDelay, c.RegularBlockSendDelayInitial)
				r.Equal(testDefaultRegularSecondDelay, c.RegularBlockSendDelaySecond) // Should be set to default
				r.Equal(customSecondDelay, c.HighLoadBlockSendDelayInitial)
				r.Equal(testDefaultHighLoadSecondDelay, c.HighLoadBlockSendDelaySecond) // Should be set to default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Validate(log)
			tt.expectFunc(require.New(t), &tt.config)
		})
	}
}
