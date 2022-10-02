package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// RPCRequestType represents the JSON-RPC methods that are callable
type RPCRequestType string

// RPCRequestType enumeration
const (
	RPCSubscribe            RPCRequestType = "subscribe"
	RPCUnsubscribe          RPCRequestType = "unsubscribe"
	RPCPrivateTxBalance     RPCRequestType = "private_tx_balance"
	RPCPrivateTx            RPCRequestType = "blxr_private_tx"
	RPCBackrunPrivateTx     RPCRequestType = "backrun_private_tx"
	RPCTx                   RPCRequestType = "blxr_tx"
	RPCPing                 RPCRequestType = "ping"
	RPCMEVSearcher          RPCRequestType = "blxr_mev_searcher"
	RPCEthSendBundle        RPCRequestType = "eth_sendBundle"
	RPCEthSendMegaBundle    RPCRequestType = "eth_sendMegabundle"
	RPCEthCallBundle        RPCRequestType = "eth_callBundle"
	RPCBatchTx              RPCRequestType = "blxr_batch_tx"
	RPCQuotaUsage           RPCRequestType = "quota_usage"
	RPCBundleSubmission     RPCRequestType = "blxr_submit_bundle"
	RPCBundleSimulation     RPCRequestType = "blxr_simulate_bundle"
	RPCMegaBundleSubmission RPCRequestType = "blxr_submit_mega_bundle"
)

// RPCTxPayload is the payload of blxr_tx requests
type RPCTxPayload struct {
	Transaction             string `json:"transaction"`
	ValidatorsOnly          bool   `json:"validators_only"`
	BlockchainNetwork       string `json:"blockchain_network"`
	OriginalSenderAccountID string `json:"original_sender_account_id"`
}

// RPCBatchTxPayload is the payload of blxr_batch_tx request
type RPCBatchTxPayload struct {
	Transactions            []string `json:"transactions"`
	ValidatorsOnly          bool     `json:"validators_only"`
	BlockchainNetwork       string   `json:"blockchain_network"`
	OriginalSenderAccountID string   `json:"original_sender_account_id"`
}

type rpcTxJSON struct {
	Transaction             string `json:"transaction"`
	ValidatorsOnly          bool   `json:"validators_only"`
	BlockchainNetwork       string `json:"blockchain_network"`
	OriginalSenderAccountID string `json:"original_sender_account_id"`
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
	return nil
}
