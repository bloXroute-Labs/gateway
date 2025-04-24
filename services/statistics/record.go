package statistics

import (
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Record represents a bloxroute style stat type record
type Record struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type blockRecord struct {
	EventSubjectID   string             `json:"event_subject_id"`
	EventLogic       EventLogic         `json:"event_logic"`
	NodeID           bxtypes.NodeID     `json:"node_id"`
	EventName        string             `json:"event_name"`
	NetworkNum       bxtypes.NetworkNum `json:"network_num"`
	SourceID         bxtypes.NodeID     `json:"source_id"`
	StartDateTime    string             `json:"start_date_time"`
	ExtraData        blockExtraData     `json:"extra_data,omitempty"`
	EndDateTime      string             `json:"end_date_time"`
	SentGatewayPeers int                `json:"gateway_peers"`
	BeaconBlockHash  string             `json:"beacon_block_hash,omitempty"`
}

type gatewayBlockRecord struct {
	EventSubjectID    string             `json:"event_subject_id"`
	EventLogic        EventLogic         `json:"event_logic"`
	NodeID            bxtypes.NodeID     `json:"node_id"`
	EventName         string             `json:"event_name"`
	NetworkNum        bxtypes.NetworkNum `json:"network_num"`
	SourceID          bxtypes.NodeID     `json:"source_id"`
	StartDateTime     string             `json:"start_date_time"`
	ExtraData         blockExtraData     `json:"extra_data,omitempty"`
	EndDateTime       string             `json:"end_date_time"`
	SentGatewayPeers  int                `json:"gateway_peers"`
	OriginalSize      int                `json:"original_size"`
	CompressSize      int                `json:"compress_size"`
	ShortIDsCount     int                `json:"short_ids_count"`
	TxsCount          int                `json:"txs_count"`
	RecoveredTxsCount int                `json:"recovered_txs_count"`
	BeaconBlockHash   string             `json:"beacon_block_hash,omitempty"`
}

type bundleRecord struct {
	EventSubjectID   string             `json:"event_subject_id"`
	EventName        string             `json:"event_name"`
	AccountID        bxtypes.AccountID  `json:"account_id"`
	NodeID           bxtypes.NodeID     `json:"node_id"`
	StartDateTime    string             `json:"start_date_time"`
	EndDateTime      string             `json:"end_date_time"`
	NetworkNum       bxtypes.NetworkNum `json:"network_num"`
	MEVBuilderNames  []string           `json:"mev_builder_names"`
	UUID             string             `json:"uuid"`
	BlockNumber      int64              `json:"block_number"`
	MinTimestamp     int                `json:"min_timestamp"`
	MaxTimestamp     int                `json:"max_timestamp"`
	ExtraData        bundleExtraData    `json:"extra_data,omitempty"`
	SentPeers        int                `json:"sent_peers,omitempty"`
	SentGatewayPeers int                `json:"gateway_peers,omitempty"`
}

// BundleSentToBuilderRecord represents a record of a bundle sent to a builder
type BundleSentToBuilderRecord struct {
	EstimatedBundleReceivedTime string                 `json:"estimated_bundle_received_time"`
	BundleHash                  string                 `json:"bundle_hash"`
	BlockNumber                 int64                  `json:"block_number"`
	UUID                        string                 `json:"uuid"`
	UserSetUUID                 bool                   `json:"user_set_UUID"`
	BuilderName                 string                 `json:"builder_name"`
	NetworkNum                  bxtypes.NetworkNum     `json:"network_num"`
	AccountID                   bxtypes.AccountID      `json:"account_id"`
	AccountTier                 sdnmessage.AccountTier `json:"account_tier"`
	BuilderURL                  string                 `json:"builder_url"`
	StatusCode                  int                    `json:"status_code"`
	ErrorMessage                string                 `json:"error_message"`
}

type ethBlockContent struct {
	BlockHash  string             `json:"block_hash"`
	NetworkNum bxtypes.NetworkNum `json:"network_num"`
	Header     string             `json:"header"`
	Txs        string             `json:"txs"`
	Trailer    string             `json:"trailer"`
}

type blockExtraData struct {
	MoreInfo string `json:"more_info,omitempty"`
}

type bundleExtraData struct {
	MoreInfo string `json:"more_info,omitempty"`
}

// LogRecord represents a log message to be sent to FluentD
type LogRecord struct {
	Level     string         `json:"level"`
	Name      string         `json:"name"`
	Instance  bxtypes.NodeID `json:"instance"`
	Msg       interface{}    `json:"msg"`
	Timestamp string         `json:"timestamp"`
}

// LogRecordWithNetwork represents a log message to be sent to FluentD with network number
type LogRecordWithNetwork struct {
	LogRecord
	NetworkNum bxtypes.NetworkNum `json:"network_num"`
}

type txRecord struct {
	EventSubjectID   string             `json:"event_subject_id"`
	EventLogic       EventLogic         `json:"event_logic"`
	NodeID           bxtypes.NodeID     `json:"node_id"`
	EventName        string             `json:"event_name"`
	NetworkNum       bxtypes.NetworkNum `json:"network_num"`
	StartDateTime    string             `json:"start_date_time"`
	ExtraData        txExtraData        `json:"extra_data,omitempty"`
	EndDateTime      string             `json:"end_date_time"`
	SentGatewayPeers int                `json:"gateway_peers"`
}

type txExtraData struct {
	MoreInfo             string             `json:"more_info,omitempty"`
	ShortID              types.ShortID      `json:"short_id"`
	NetworkNum           bxtypes.NetworkNum `json:"network_num"`
	SourceID             bxtypes.NodeID     `json:"source_id"`
	IsCompactTransaction bool               `json:"is_compact_transaction"`
	AlreadySeenContent   bool               `json:"already_seen_content"`
	ExistingShortIDs     types.ShortIDList  `json:"existing_short_ids"`
}

type subscribeRecord struct {
	SubscriptionID string                 `json:"subscription_id"`
	Type           string                 `json:"type"`
	Event          string                 `json:"event"`
	FeedFilters    string                 `json:"feed_filters"`
	IP             string                 `json:"ip"`
	AccountID      bxtypes.AccountID      `json:"account_id"`
	Tier           sdnmessage.AccountTier `json:"tier"`
	FeedName       types.FeedType         `json:"feed_name"`
	FeedInclude    []string               `json:"feed_include"`
	NetworkNum     bxtypes.NetworkNum     `json:"network_num"`
}

type unsubscribeRecord struct {
	SubscriptionID string                 `json:"subscription_id"`
	Type           string                 `json:"type"`
	Event          string                 `json:"event"`
	FeedName       types.FeedType         `json:"feed_name"`
	NetworkNum     bxtypes.NetworkNum     `json:"network_num"`
	AccountID      bxtypes.AccountID      `json:"account_id"`
	Tier           sdnmessage.AccountTier `json:"tier"`
}

type sdkInfoRecord struct {
	Blockchain string            `json:"blockchain"`
	Method     string            `json:"method"`
	Feed       string            `json:"feed"`
	SourceCode string            `json:"source_code"`
	Version    string            `json:"version"`
	AccountID  bxtypes.AccountID `json:"account_id"`
	Start      string            `json:"start"`
	End        string            `json:"end"`
}

// BlobRecord represents a blob record
type BlobRecord struct {
	EventSubjectID string             `json:"event_subject_id"`
	EventName      string             `json:"event_name"`
	NodeID         bxtypes.NodeID     `json:"node_id"`
	SourceID       bxtypes.NodeID     `json:"source_id"`
	NetworkNum     bxtypes.NetworkNum `json:"network_num"`
	StartDateTime  string             `json:"start_date_time"`
	EndDateTime    string             `json:"end_date_time"`
	OriginalSize   int                `json:"original_size"`
	CompressSize   int                `json:"compress_size"`
	BlobIndex      uint32             `json:"blob_index"`
	BlockHash      string             `json:"block_hash"`
}
