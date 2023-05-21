package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// RPCRequestType represents the JSON-RPC methods that are callable
type RPCRequestType string

// Bloxroute RPCRequestType enumeration
const (
	RPCSubscribe            RPCRequestType = "subscribe"
	RPCUnsubscribe          RPCRequestType = "unsubscribe"
	RPCPrivateTxBalance     RPCRequestType = "private_tx_balance"
	RPCPrivateTx            RPCRequestType = "blxr_private_tx"
	RPCTx                   RPCRequestType = "blxr_tx"
	RPCPing                 RPCRequestType = "ping"
	RPCMEVSearcher          RPCRequestType = "blxr_mev_searcher"
	RPCBatchTx              RPCRequestType = "blxr_batch_tx"
	RPCQuotaUsage           RPCRequestType = "quota_usage"
	RPCBundleSubmission     RPCRequestType = "blxr_submit_bundle"
	RPCBundleSimulation     RPCRequestType = "blxr_simulate_bundle"
	RPCMegaBundleSubmission RPCRequestType = "blxr_submit_mega_bundle"
	RPCStartMonitoringTx    RPCRequestType = "start_monitor_transaction"
	RPCStopMonitoringTx     RPCRequestType = "stop_monitor_transaction"
	RPCFeeBumpTx            RPCRequestType = "blxr_tx_fee_bump"
)

// External RPCRequestType enumeration
const (
	RPCEthSendBundle     RPCRequestType = "eth_sendBundle"
	RPCEthSendMegaBundle RPCRequestType = "eth_sendMegabundle"
	RPCEthCallBundle     RPCRequestType = "eth_callBundle"
)

// RPCTxPayload is the payload of blxr_tx requests
type RPCTxPayload struct {
	Transaction             string         `json:"transaction"`
	MevBundleTx             bool           `json:"mev_bundle_tx"`
	ValidatorsOnly          bool           `json:"validators_only"`
	NextValidator           bool           `json:"next_validator"`
	Fallback                uint16         `json:"fall_back"`
	BlockchainNetwork       string         `json:"blockchain_network"`
	OriginalSenderAccountID string         `json:"original_sender_account_id"`
	OriginalRPCMethod       RPCRequestType `json:"original_rpc_method"`
	NodeValidation          bool           `json:"node_validation"`
	FrontRunningProtection  bool           `json:"front_running_protection"`
}

// RPCBatchTxPayload is the payload of blxr_batch_tx request
type RPCBatchTxPayload struct {
	Transactions            []string `json:"transactions"`
	ValidatorsOnly          bool     `json:"validators_only"`
	BlockchainNetwork       string   `json:"blockchain_network"`
	OriginalSenderAccountID string   `json:"original_sender_account_id"`
}

type rpcTxJSON struct {
	Transaction             string         `json:"transaction"`
	MevBundleTx             bool           `json:"mev_bundle_tx"`
	ValidatorsOnly          bool           `json:"validators_only"`
	NextValidator           bool           `json:"next_validator"`
	Fallback                uint16         `json:"fall_back"`
	BlockchainNetwork       string         `json:"blockchain_network"`
	OriginalSenderAccountID string         `json:"original_sender_account_id"`
	OriginalRPCMethod       RPCRequestType `json:"original_rpc_method"`
	NodeValidation          bool           `json:"node_validation"`
	FrontRunningProtection  bool           `json:"front_running_protection"`
}

// UnmarshalJSON provides a compatibility layer for go-ethereum style RPC calls, which are [object], instead of just object.
func (p *RPCTxPayload) UnmarshalJSON(b []byte) error {
	var payload rpcTxJSON

	err := json.Unmarshal(b, &payload)
	if err != nil {
		var compatPayload []rpcTxJSON
		err = json.Unmarshal(b, &compatPayload)
		if err != nil {
			return err
		}

		if len(compatPayload) != 1 {
			return fmt.Errorf("could not deserialize blxr_tx %v", string(b))
		}

		payload = compatPayload[0]
	}

	p.Transaction = payload.Transaction
	p.ValidatorsOnly = payload.ValidatorsOnly
	p.BlockchainNetwork = payload.BlockchainNetwork
	p.OriginalSenderAccountID = payload.OriginalSenderAccountID
	p.NextValidator = payload.NextValidator
	p.Fallback = payload.Fallback
	p.OriginalRPCMethod = payload.OriginalRPCMethod
	p.NodeValidation = payload.NodeValidation
	p.FrontRunningProtection = payload.FrontRunningProtection
	p.MevBundleTx = payload.MevBundleTx

	return nil
}
