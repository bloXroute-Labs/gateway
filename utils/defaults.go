package utils

const (
	// DefaultRegularBlockSendDelayInitialMS is a default initial delay for sending blocks in milliseconds under regular load.
	DefaultRegularBlockSendDelayInitialMS = 2500

	// DefaultRegularBlockSendDelaySecondMS is a default second delay for sending blocks in milliseconds under regular load.
	DefaultRegularBlockSendDelaySecondMS = 2800

	// DefaultRegularBlockSendDelayIntervalMS is a default interval delay for sending blocks in milliseconds under regular load.
	DefaultRegularBlockSendDelayIntervalMS = 100

	// DefaultHighLoadBlockSendDelayInitialMS is a default initial delay for sending blocks in milliseconds under high load.
	DefaultHighLoadBlockSendDelayInitialMS = 300

	// DefaultHighLoadBlockSendDelaySecondMS is a default second delay for sending blocks in milliseconds under high load.
	DefaultHighLoadBlockSendDelaySecondMS = 2000

	// DefaultHighLoadBlockSendDelayIntervalMS is a default interval delay for sending blocks in milliseconds under high load.
	DefaultHighLoadBlockSendDelayIntervalMS = 100

	// DefaultHighLoadTxNumThreshold is a default threshold for high load.
	DefaultHighLoadTxNumThreshold = 1500
)
