package servers

import (
	"github.com/bloXroute-Labs/gateway/v2/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/zhouzhuojie/conditions"
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
	calls    *map[string]*RPCCall
	MultiTxs bool
}

type subscriptionRequest struct {
	feed    types.FeedType
	options subscriptionOptions
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
