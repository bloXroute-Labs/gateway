package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	uuid "github.com/satori/go.uuid"
)

// RPCRequestType represents the JSON-RPC methods that are callable
type RPCRequestType string

// Bloxroute RPCRequestType enumeration
const (
	RPCSubscribe                  RPCRequestType = "subscribe"
	RPCUnsubscribe                RPCRequestType = "unsubscribe"
	RPCPrivateTxBalance           RPCRequestType = "private_tx_balance"
	RPCPrivateTx                  RPCRequestType = "blxr_private_tx"
	RPCTx                         RPCRequestType = "blxr_tx"
	RPCPing                       RPCRequestType = "ping"
	RPCBatchTx                    RPCRequestType = "blxr_batch_tx"
	RPCQuotaUsage                 RPCRequestType = "quota_usage"
	RPCBundleSubmission           RPCRequestType = "blxr_submit_bundle"
	RPCEOBBundleSubmission        RPCRequestType = "blxr_submit_eob_bundle"
	RPCBundleSimulation           RPCRequestType = "blxr_simulate_bundle"
	RPCStartMonitoringTx          RPCRequestType = "start_monitor_transaction"
	RPCStopMonitoringTx           RPCRequestType = "stop_monitor_transaction"
	RPCFeeBumpTx                  RPCRequestType = "blxr_tx_fee_bump"
	RPCChangeNewPendingTxFromNode RPCRequestType = "new_pending_txs_source_from_node"
	RPCEthSubscribe               RPCRequestType = "eth_subscribe"
	RPCEthSendRawTransaction      RPCRequestType = "eth_sendRawTransaction"
	RPCEthUnsubscribe             RPCRequestType = "eth_unsubscribe"
	RPCSubmitIntent               RPCRequestType = "blxr_submit_intent"
	RPCSubmitIntentSolution       RPCRequestType = "blxr_submit_intent_solution"
	RPCGetIntentSolutions         RPCRequestType = "blxr_get_intent_solutions"
	RPCSubmitQuote                RPCRequestType = "blxr_submit_quote"
)

// External RPCRequestType enumeration
const (
	RPCEthSendBundle          RPCRequestType = "eth_sendBundle"
	RPCEthCallBundle          RPCRequestType = "eth_callBundle"
	RPCEthCancelBundle        RPCRequestType = "eth_cancelBundle"
	RPCEthSendExclusiveBundle RPCRequestType = "eth_sendExclusiveBundle"
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

// RPCBundleSubmissionPayload is the payload of blxr_submit_bundle request
type RPCBundleSubmissionPayload struct {
	BlockchainNetwork       string            `json:"blockchain_network"`
	MEVBuilders             map[string]string `json:"mev_builders"`
	Transaction             []string          `json:"transaction"`
	BlockNumber             string            `json:"block_number"`
	MinTimestamp            int               `json:"min_timestamp"`
	MaxTimestamp            int               `json:"max_timestamp"`
	RevertingHashes         []string          `json:"reverting_hashes"`
	UUID                    string            `json:"uuid"`
	AvoidMixedBundles       bool              `json:"avoid_mixed_bundles,omitempty"`
	OriginalSenderAccountID string            `json:"original_sender_account_id"`
	PriorityFeeRefund       bool              `json:"priority_fee_refund"`
	IncomingRefundRecipient string            `json:"refund_recipient,omitempty"`
	BlocksCount             int               `json:"blocks_count,omitempty"`
	DroppingTxHashes        []string          `json:"dropping_tx_hashes,omitempty"`
}

// Validate doing validation for blxr_submit_bundle payload
func (p RPCBundleSubmissionPayload) Validate() error {
	if len(p.Transaction) == 0 && p.UUID == "" {
		return errors.New("bundle missing txs")
	}

	if p.BlockNumber == "" && p.UUID == "" {
		return errors.New("bundle missing blockNumber")
	}

	if p.MinTimestamp < 0 {
		return errors.New("min timestamp must be greater than or equal to 0")
	}
	if p.MaxTimestamp < 0 {
		return errors.New("max timestamp must be greater than or equal to 0")
	}

	if p.UUID != "" {
		_, err := uuid.FromString(p.UUID)
		if err != nil {
			return fmt.Errorf("invalid UUID, %v", err)
		}
	}

	_, err := hexutil.DecodeUint64(p.BlockNumber)
	if err != nil {
		return fmt.Errorf("blockNumber must be hex, %v", err)
	}

	if p.IncomingRefundRecipient != "" {
		if !ethcommon.IsHexAddress(p.IncomingRefundRecipient) {
			return fmt.Errorf("refund recipient must be a hex address, %v", err)
		}
	}

	if len(p.DroppingTxHashes) > len(p.Transaction) {
		return fmt.Errorf("dropping txs have too many hashes, %v", p.DroppingTxHashes)
	}

	return nil
}

// RPCSendBundle MEVBundle payload to be used
type RPCSendBundle struct {
	Txs               []string `json:"txs"`
	UUID              string   `json:"uuid,omitempty"` // TODO: this should be called `replacementUuid` according to eth_sendBundle spec
	BlockNumber       string   `json:"blockNumber"`
	MinTimestamp      int      `json:"minTimestamp,omitempty"`
	MaxTimestamp      int      `json:"maxTimestamp,omitempty"`
	RevertingTxHashes []string `json:"revertingTxHashes,omitempty"`
	AvoidMixedBundles bool     `json:"avoidMixedBundles,omitempty"`
	RefundRecipient   string   `json:"refundRecipient,omitempty"`
	Boost             bool     `json:"boost"` // 'beaverbuild' builder specific
	AccountID         string   `json:"accountId,omitempty"`
	BlocksCount       int      `json:"blocksCount,omitempty"`
	DroppingTxHashes  []string `json:"droppingTxHashes,omitempty"`
}

// RPCCancelBundlePayload custom json-rpc required to cancel flashbots bundle
type RPCCancelBundlePayload struct {
	ReplacementUUID string `json:"replacementUuid"`
}

// RPCSubmitIntentPayload is the payload of blxr_submit_intent request
type RPCSubmitIntentPayload struct {
	DappAddress   string `json:"dapp_address"`
	SenderAddress string `json:"sender_address"`
	Intent        []byte `json:"intent"`
	Hash          []byte `json:"hash"`
	Signature     []byte `json:"signature"`
}

// RPCSubmitQuotePayload is the payload of blxr_submit_quote request
type RPCSubmitQuotePayload struct {
	DappAddress   string `json:"dapp_address"`
	SolverAddress string `json:"solver_address"`
	Quote         []byte `json:"quote"`
	Hash          []byte `json:"hash"`
	Signature     []byte `json:"signature"`
}

// RPCSubmitIntentSolutionPayload is the payload of blxr_submit_intent_solution request
type RPCSubmitIntentSolutionPayload struct {
	SolverAddress  string `json:"solver_address"`
	IntentID       string `json:"intent_id"`
	IntentSolution []byte `json:"intent_solution"`
	Hash           []byte `json:"hash"`
	Signature      []byte `json:"signature"`
}

// RPCGetIntentSolutionsPayload is the payload of blxr_get_intent_solutions request
type RPCGetIntentSolutionsPayload struct {
	IntentID            string `json:"intent_id"`
	DappOrSenderAddress string `json:"dapp_or_sender_address"`
	Hash                []byte `json:"hash"`
	Signature           []byte `json:"signature"`
}
