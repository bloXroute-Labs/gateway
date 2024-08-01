package sdnmessage

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// BlockchainAttributes represents blockchain network attributes
type BlockchainAttributes struct {
	BlockchainNetMagic int64 `json:"blockchain_net_magic,omitempty"`
	BlockchainPort     int64 `json:"blockchain_port,omitempty"`
	BlockchainServices int64 `json:"blockchain_services,omitempty"`
	BlockchainVersion  int64 `json:"blockchain_version,omitempty"`
	// string?? doesn't work...
	ChainDifficulty         interface{}     `json:"chain_difficulty,omitempty"`
	TerminalTotalDifficulty interface{}     `json:"terminal_total_difficulty"`
	GenesisHash             string          `json:"genesis_hash,omitempty"`
	NetworkID               types.NetworkID `json:"network_id,omitempty"`
	BootstrapNodes          []string        `json:"bootstrap_nodes"`
	ExecutionLayerForks     []string        `json:"execution_layer_forks,omitempty"`

	// ETHShanghaiMergeTimeUnix specifies Ethereum Shanghai merge time, after this time
	// relay will ignore beacon blocks from gateways if their protocol version is low
	// and won't propagate beacon blocks to such gateways but will still propagate transactions
	ETHShanghaiMergeTimeUnix int64 `json:"eth_shanghai_merge_time_unix"`
}

// BlockchainNetwork represents network config for a given blockchain network being routed by bloxroute
type BlockchainNetwork struct {
	AllowTimeReuseSenderNonce              float64              `json:"allowed_time_reuse_sender_nonce"`
	AllowGasPriceChangeReuseSenderNonce    float64              `json:"allowed_gas_price_change_reuse_sender_nonce"`
	BlockConfirmationsCount                int                  `json:"block_confirmations_count"`
	BlockHoldTimeoutS                      float64              `json:"block_hold_timeout_s"`
	BlockInterval                          int64                `json:"block_interval"`
	BlockRecoveryTimeoutS                  int64                `json:"block_recovery_timeout_s"`
	DefaultAttributes                      BlockchainAttributes `json:"default_attributes"`
	ProgramName                            string               `json:"program_name"`
	EnableBlockCompression                 bool                 `json:"enable_block_compression"`
	EnableCheckSenderNonce                 bool                 `json:"enable_check_sender_nonce"`
	EnableNetworkContentLogs               bool                 `json:"enable_network_content_logs"`
	EnableRecordingTxDetectionTimeLocation bool                 `json:"enable_recording_tx_detection_time_location"`
	Environment                            string               `json:"environment"`
	FinalTxConfirmationsCount              int64                `json:"final_tx_confirmations_count"`
	IgnoreBlockIntervalCount               int64                `json:"ignore_block_interval_count"`
	LogCompressedBlockDebugInfoOnRelay     bool                 `json:"log_compressed_block_debug_info_on_relay"`
	MaxBlockSizeBytes                      int64                `json:"max_block_size_bytes"`
	MaxTxSizeBytes                         int64                `json:"max_tx_size_bytes"`
	MaxTotalBlobTxSizeBytes                uint64               `json:"max_total_blob_tx_size_bytes"`
	MediumTxNetworkFee                     int64                `json:"medium_tx_network_fee"`
	MempoolExpectedTransactionsCount       int64                `json:"mempool_expected_transactions_count"`
	MinTxAgeSeconds                        float64              `json:"min_tx_age_seconds"`
	MaxTxAgeSeconds                        int64                `json:"max_tx_age_seconds"`
	MinTxNetworkFee                        float64              `json:"min_tx_network_fee"`
	Network                                string               `json:"network"`
	NetworkNum                             types.NetworkNum     `json:"network_num"`
	Protocol                               string               `json:"protocol"`
	RemovedTransactionsHistoryExpirationS  int64                `json:"removed_transactions_history_expiration_s"`
	SdnID                                  string               `json:"sdn_id"`
	SendCompressedTxsAfterBlock            bool                 `json:"send_compressed_txs_after_block"`
	TxContentsMemoryLimitBytes             int64                `json:"tx_contents_memory_limit_bytes"`
	TxPercentToLogByHash                   float64              `json:"tx_percent_to_log_by_hash"`
	TxPercentToLogBySid                    float64              `json:"tx_percent_to_log_by_sid"`
	TxSyncIntervalS                        float64              `json:"tx_sync_interval_s"`
	TxSyncSyncContent                      bool                 `json:"tx_sync_sync_content"`
	Type                                   string               `json:"type"`
	EnableTxTrace                          bool                 `json:"enable_tx_trace"`
	InjectPoa                              bool                 `json:"inject_poa"`
	AllowedFromTier                        AccountTier          `json:"allowed_from_tier"`
	SendCrossGeo                           bool                 `json:"send_cross_geo"`
	DeliverToNodePercent                   uint64               `json:"deliver_to_node_percent"`
}

// BlockchainNetworks represents the full message returned from bxapi
type BlockchainNetworks map[types.NetworkNum]*BlockchainNetwork

// UpdateFrom - sets updates from a network
func (bcn *BlockchainNetwork) UpdateFrom(network *BlockchainNetwork) {
	bcn.AllowTimeReuseSenderNonce = network.AllowTimeReuseSenderNonce
	bcn.AllowGasPriceChangeReuseSenderNonce = network.AllowGasPriceChangeReuseSenderNonce
	bcn.TxPercentToLogByHash = network.TxPercentToLogByHash
	bcn.EnableTxTrace = network.EnableTxTrace
	bcn.EnableCheckSenderNonce = network.EnableCheckSenderNonce
}

// FindNetwork finds a BlockchainNetwork instance by its number and allow update
func (bcns *BlockchainNetworks) FindNetwork(networkNum types.NetworkNum) (*BlockchainNetwork, error) {
	if network, exists := (*bcns)[networkNum]; exists {
		return network, nil
	}
	return nil, fmt.Errorf("can't find blockchain network with network number %v", networkNum)
}

// IsAllowedTier check if tier is allowed in blockchain network
func (bcn *BlockchainNetwork) IsAllowedTier(clientTier AccountTier) bool {
	switch bcn.AllowedFromTier {
	case ATierIntroductory:
		return true
	case ATierDeveloper:
		return clientTier != ATierIntroductory
	case ATierProfessional:
		return clientTier != ATierIntroductory && clientTier != ATierDeveloper
	case ATierEnterprise:
		return clientTier.IsEnterprise()
	case ATierElite:
		return clientTier.IsElite()
	case ATierUltra:
		return clientTier.IsUltra()
	default:
		return false
	}
}
