package statistics

// EventLogic that instruct the log_agent how to treat this stat record
type EventLogic int

const (
	// EventLogicBlockInfo - represent block info
	EventLogicBlockInfo EventLogic = 1 << iota

	// EventLogicMatch - represent a stat records that have several hashes and used to match between them
	EventLogicMatch

	// EventLogicSummary - represent a summary record
	EventLogicSummary

	// EventLogicPropagationStart - represent start propagation
	EventLogicPropagationStart

	// EventLogicPropagationEnd - represent end propagation
	EventLogicPropagationEnd

	// EventLogicNone - represent no special logic
	EventLogicNone EventLogic = 0
)
