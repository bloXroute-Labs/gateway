package statistics

import "github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"

// Record represents a bloxroute style stat type record
type Record struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type blockRecord struct {
	EventSubjectID   string           `json:"event_subject_id"`
	EventLogic       EventLogic       `json:"event_logic"`
	NodeID           types.NodeID     `json:"node_id"`
	EventName        string           `json:"event_name"`
	NetworkNum       types.NetworkNum `json:"network_num"`
	SourceID         types.NodeID     `json:"source_id"`
	StartDateTime    string           `json:"start_date_time"`
	ExtraData        blockExtraData   `json:"extra_data,omitempty"`
	EndDateTime      string           `json:"end_date_time"`
	SentGatewayPeers int              `json:"gateway_peers"`
}

type gatewayBlockRecord struct {
	EventSubjectID    string           `json:"event_subject_id"`
	EventLogic        EventLogic       `json:"event_logic"`
	NodeID            types.NodeID     `json:"node_id"`
	EventName         string           `json:"event_name"`
	NetworkNum        types.NetworkNum `json:"network_num"`
	SourceID          types.NodeID     `json:"source_id"`
	StartDateTime     string           `json:"start_date_time"`
	ExtraData         blockExtraData   `json:"extra_data,omitempty"`
	EndDateTime       string           `json:"end_date_time"`
	SentGatewayPeers  int              `json:"gateway_peers"`
	OriginalSize      int              `json:"original_size"`
	CompressSize      int              `json:"compress_size"`
	ShortIDsCount     int              `json:"short_ids_count"`
	TxsCount          int              `json:"txs_count"`
	RecoveredTxsCount int              `json:"recovered_txs_count"`
}

type blockContent struct {
	BlockHash    string           `json:"block_hash"`
	NetworkNum   types.NetworkNum `json:"network_num"`
	BlockContent string           `json:"block_content"`
}

type blockExtraData struct {
	MoreInfo string `json:"more_info,omitempty"`
}

// LogRecord represents a log message to be sent to FluentD
type LogRecord struct {
	Level     string       `json:"level"`
	Name      string       `json:"name"`
	Instance  types.NodeID `json:"instance"`
	Msg       interface{}  `json:"msg"`
	Timestamp string       `json:"timestamp"`
}

type txRecord struct {
	EventSubjectID   string           `json:"event_subject_id"`
	EventLogic       EventLogic       `json:"event_logic"`
	NodeID           types.NodeID     `json:"node_id"`
	EventName        string           `json:"event_name"`
	NetworkNum       types.NetworkNum `json:"network_num"`
	StartDateTime    string           `json:"start_date_time"`
	ExtraData        txExtraData      `json:"extra_data,omitempty"`
	EndDateTime      string           `json:"end_date_time"`
	SentGatewayPeers int              `json:"gateway_peers"`
}

type txExtraData struct {
	MoreInfo             string            `json:"more_info,omitempty"`
	ShortID              types.ShortID     `json:"short_id"`
	NetworkNum           types.NetworkNum  `json:"network_num"`
	SourceID             types.NodeID      `json:"source_id"`
	IsCompactTransaction bool              `json:"is_compact_transaction"`
	AlreadySeenContent   bool              `json:"already_seen_content"`
	ExistingShortIds     types.ShortIDList `json:"existing_short_ids"`
}
