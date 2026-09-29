package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// RPCRequestType represents the JSON-RPC methods that are callable
type RPCRequestType string

// Bloxroute RPCRequestType enumeration
const (
	RPCSubscribe                  RPCRequestType = "subscribe"
	RPCUnsubscribe                RPCRequestType = "unsubscribe"
	RPCTx                         RPCRequestType = "blxr_tx"
	RPCPing                       RPCRequestType = "ping"
	RPCBatchTx                    RPCRequestType = "blxr_batch_tx"
	RPCQuotaUsage                 RPCRequestType = "quota_usage"
	RPCChangeNewPendingTxFromNode RPCRequestType = "new_pending_txs_source_from_node"
	RPCEthSubscribe               RPCRequestType = "eth_subscribe"
	RPCEthSendRawTransaction      RPCRequestType = "eth_sendRawTransaction"
	RPCEthUnsubscribe             RPCRequestType = "eth_unsubscribe"
)

// RPCMethodToRPCRequestType maps gRPC methods to RPCRequestType
var RPCMethodToRPCRequestType = map[string]RPCRequestType{
	"/gateway.Gateway/BlxrTx": RPCTx,
}


// RPCTxPayload is the payload of blxr_tx requests
type RPCTxPayload struct {
	Transaction             string         `json:"transaction"`
	MevBundleTx             bool           `json:"mev_bundle_tx"`
	ValidatorsOnly          bool           `json:"validators_only"`
	BlockchainNetwork       string         `json:"blockchain_network"`
	OriginalSenderAccountID string         `json:"original_sender_account_id"` // deprecated, use BLXRMetadata instead
	OriginalRPCMethod       RPCRequestType `json:"original_rpc_method"`
	NodeValidation          bool           `json:"node_validation"`
	FrontRunningProtection  bool           `json:"front_running_protection"`
	BackrunmeRewardAddress  string         `json:"backrunme_reward_address,omitempty"`
	RPCMode                 bool           `json:"rpc_mode,omitempty"`
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
	Fallback                uint16         `json:"fall_back"`
	BlockchainNetwork       string         `json:"blockchain_network"`
	OriginalSenderAccountID string         `json:"original_sender_account_id"`
	OriginalRPCMethod       RPCRequestType `json:"original_rpc_method"`
	NodeValidation          bool           `json:"node_validation"`
	FrontRunningProtection  bool           `json:"front_running_protection"`
	BackrunmeRewardAddress  string         `json:"backrunme_reward_address,omitempty"`
	RPCMode                 bool           `json:"rpc_mode,omitempty"`
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
	p.OriginalRPCMethod = payload.OriginalRPCMethod
	p.NodeValidation = payload.NodeValidation
	p.FrontRunningProtection = payload.FrontRunningProtection
	p.MevBundleTx = payload.MevBundleTx
	p.BackrunmeRewardAddress = payload.BackrunmeRewardAddress
	p.RPCMode = payload.RPCMode

	return nil
}
