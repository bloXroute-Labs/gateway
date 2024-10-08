package ws

import (
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/zhouzhuojie/conditions"

	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// MultiTransactions - response for MultiTransactions subscription
type MultiTransactions struct {
	Subscription string     `json:"subscription"`
	Result       []TxResult `json:"result"`
}

// TxResponse - response of the jsonrpc params
type TxResponse struct {
	Subscription string   `json:"subscription"`
	Result       TxResult `json:"result"`
}

// TxResult - request of jsonrpc params
type TxResult struct {
	TxHash      *string     `json:"txHash,omitempty"`
	TxContents  interface{} `json:"txContents,omitempty"`
	LocalRegion *bool       `json:"localRegion,omitempty"`
	Time        *string     `json:"time,omitempty"`
	RawTx       *string     `json:"rawTx,omitempty"`
}

// TxResultWithEthTx - request of jsonrpc params with an eth type transaction
type TxResultWithEthTx struct {
	TxHash      *string               `json:"txHash,omitempty"`
	TxContents  *ethtypes.Transaction `json:"txContents,omitempty"`
	LocalRegion *bool                 `json:"localRegion,omitempty"`
	Time        *string               `json:"time,omitempty"`
	RawTx       *string               `json:"rawTx,omitempty"`
}

// BlockResponse - response of the jsonrpc params
type BlockResponse struct {
	Subscription string             `json:"subscription"`
	Result       types.Notification `json:"result"`
}

type txReceiptResponse struct {
	Subscription string           `json:"subscription"`
	Result       *types.TxReceipt `json:"result"`
}

// ClientReq represent client request
type ClientReq struct {
	Includes []string
	Feed     types.FeedType
	Expr     conditions.Expr
	calls    *map[string]*handler.RPCCall
	MultiTxs bool
}

type subscriptionRequest struct {
	feed    types.FeedType
	options subscriptionOptions
}

type subscriptionIntentParams struct {
	SolverAddress string `json:"solver_address"`
	Hash          []byte `json:"hash"`
	Signature     []byte `json:"signature"`
	DappAddress   string `json:"dapp_address"`
	Filters       string `json:"filters"`
}

type subscribeQuotesParams struct {
	DappAddress string `json:"dapp_address"`
}

// subscriptionOptions Includes subscription options
type subscriptionOptions struct {
	Include    []string            `json:"Include"`
	Filters    string              `json:"Filters"`
	CallParams []map[string]string `json:"Call-Params"`
	MultiTxs   bool                `json:"MultiTxs"`
}

type rpcPingResponse struct {
	Pong string `json:"pong"`
}

type userIntentResponse struct {
	Subscription string                 `json:"subscription"`
	Result       userIntentNotification `json:"result"`
}

type userIntentNotification struct {
	DappAddress   string `json:"dapp_address"`
	SenderAddress string `json:"sender_address"`
	IntentID      string `json:"intent_id"`
	Intent        []byte `json:"intent"`
	Timestamp     string `json:"timestamp"`
}

type userIntentSolutionResponse struct {
	Subscription string                         `json:"subscription"`
	Result       userIntentSolutionNotification `json:"result"`
}

type userIntentSolutionNotification struct {
	IntentID       string `json:"intent_id"`
	SolutionID     string `json:"solution_id"`
	IntentSolution []byte `json:"intent_solution"`
}

type quoteResponse struct {
	Subscription string            `json:"subscription"`
	Result       quoteNotification `json:"result"`
}

type quoteNotification struct {
	DappAddress   string `json:"dapp_address"`
	ID            string `json:"quote_id"`
	SolverAddress string `json:"solver_address"`
	Quote         []byte `json:"quote"`
	Timestamp     string `json:"timestamp"`
}

type rpcBatchTxResponse struct {
	TxHashes []string `json:"txHashes"`
}
