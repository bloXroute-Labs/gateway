package statistics

import (
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

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
	BeaconBlockHash  string           `json:"beacon_block_hash,omitempty"`
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
	BeaconBlockHash   string           `json:"beacon_block_hash,omitempty"`
}

type bundleRecord struct {
	EventSubjectID   string           `json:"event_subject_id"`
	EventName        string           `json:"event_name"`
	AccountID        types.AccountID  `json:"account_id"`
	NodeID           types.NodeID     `json:"node_id"`
	StartDateTime    string           `json:"start_date_time"`
	EndDateTime      string           `json:"end_date_time"`
	NetworkNum       types.NetworkNum `json:"network_num"`
	MEVBuilderNames  []string         `json:"mev_builder_names"`
	FrontRunning     bool             `json:"frontrunning"`
	UUID             string           `json:"uuid"`
	BlockNumber      uint64           `json:"block_number"`
	MinTimestamp     int              `json:"min_timestamp"`
	MaxTimestamp     int              `json:"max_timestamp"`
	ExtraData        bundleExtraData  `json:"extra_data,omitempty"`
	BundlePrice      int64            `json:"bundlePrice,omitempty"` // in wei
	EnforcePayout    bool             `json:"enforcePayout,omitempty"`
	SentPeers        int              `json:"sent_peers,omitempty"`
	SentGatewayPeers int              `json:"gateway_peers,omitempty"`
}

type ethBlockContent struct {
	BlockHash  string           `json:"block_hash"`
	NetworkNum types.NetworkNum `json:"network_num"`
	Header     string           `json:"header"`
	Txs        string           `json:"txs"`
	Trailer    string           `json:"trailer"`
}

type blockExtraData struct {
	MoreInfo string `json:"more_info,omitempty"`
}

type bundleExtraData struct {
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

type subscribeRecord struct {
	SubscriptionID string                 `json:"subscription_id"`
	Type           string                 `json:"type"`
	Event          string                 `json:"event"`
	FeedFilters    string                 `json:"feed_filters"`
	IP             string                 `json:"ip"`
	AccountID      types.AccountID        `json:"account_id"`
	Tier           sdnmessage.AccountTier `json:"tier"`
	FeedName       types.FeedType         `json:"feed_name"`
	FeedInclude    []string               `json:"feed_include"`
	NetworkNum     types.NetworkNum       `json:"network_num"`
	FeedProject    string                 `json:"feed_project"`
}

type unsubscribeRecord struct {
	SubscriptionID string                 `json:"subscription_id"`
	Type           string                 `json:"type"`
	Event          string                 `json:"event"`
	FeedName       types.FeedType         `json:"feed_name"`
	NetworkNum     types.NetworkNum       `json:"network_num"`
	AccountID      types.AccountID        `json:"account_id"`
	Tier           sdnmessage.AccountTier `json:"tier"`
}
