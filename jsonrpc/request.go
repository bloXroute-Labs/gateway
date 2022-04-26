package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// RPCRequestType represents the JSON-RPC methods that are callable
type RPCRequestType string

// RPCRequestType enumeration
const (
	RPCSubscribe         RPCRequestType = "subscribe"
	RPCUnsubscribe       RPCRequestType = "unsubscribe"
	RPCTx                RPCRequestType = "blxr_tx"
	RPCPing              RPCRequestType = "ping"
	RPCMEVSearcher       RPCRequestType = "blxr_mev_searcher"
	RPCEthSendBundle     RPCRequestType = "eth_sendBundle"
	RPCEthSendMegaBundle RPCRequestType = "eth_sendMegabundle"
	RPCBatchTx           RPCRequestType = "blxr_batch_tx"
)

// RPCTxPayload is the payload of blxr_tx requests
type RPCTxPayload struct {
	Transaction    string
	ValidatorsOnly bool
}

// RPCBatchTxPayload is the payload of blxr_batch_tx request
type RPCBatchTxPayload struct {
	Transactions   []string
	ValidatorsOnly bool
}

type rpcTxJSON struct {
	Transaction    string `json:"transaction"`
	ValidatorsOnly bool   `json:"validators_only"`
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
	return nil
}
