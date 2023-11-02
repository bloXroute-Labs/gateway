package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/sourcegraph/jsonrpc2"
)

var errInvalidTransactions = "all transactions are invalid"

type rpcBatchTxResponse struct {
	TxHashes []string `json:"txHashes"`
}

func (h *handlerObj) handleRPCBatchTx(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCBatchTx)
		h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		SendErrorMsg(ctx, jsonrpc.InvalidRequest, errDifferentAccAuth, conn, req.ID)
		return
	}
	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}
	var params jsonrpc.RPCBatchTxPayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCChangeNewPendingTxFromNode, err), conn, req.ID)
		return
	}

	var ws connections.RPCConn
	if h.connectionAccount.AccountID == types.BloxrouteAccountID {
		// Tx sent from cloud services, need to update account ID of the connection to be the origin sender
		ws = connections.NewRPCConn(types.AccountID(params.OriginalSenderAccountID), h.remoteAddress, h.FeedManager.networkNum, utils.CloudAPI)
	} else {
		ws = connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.FeedManager.networkNum, utils.Websocket)
	}

	var txHashes []string

	for _, transaction := range params.Transactions {
		txHash, ok, err := HandleSingleTransaction(h.FeedManager, transaction, nil, ws, params.ValidatorsOnly, false,
			false, false, 0, nil, nil)
		if err != nil {
			h.log.WithField("method", jsonrpc.RPCBatchTx).Errorf("failed to handle transaction: %v", err)
		}
		if !ok {
			continue
		}
		txHashes = append(txHashes, txHash)
	}

	if len(txHashes) == 0 {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errInvalidTransactions, conn, req.ID)
		return
	}

	if len(txHashes) != len(params.Transactions) {
		h.log.WithField("method", jsonrpc.RPCBatchTx).
			Errorf("failed to handle all transactions, successful: %d, total: %d", len(txHashes), len(params.Transactions))
	}

	response := rpcBatchTxResponse{
		TxHashes: txHashes,
	}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}

	h.log.Infof("%v: Hashes - %v", jsonrpc.RPCBatchTx, response.TxHashes)
}
