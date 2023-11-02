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

type rpcTxResponse struct {
	TxHash string `json:"txHash"`
}

func (h *handlerObj) handleRPCTx(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCTx)
		if h.FeedManager.accountModel.AccountID == types.BloxrouteAccountID {
			h.log.Infof("received a tx from user account %v, remoteAddr %v: %v", h.connectionAccount.AccountID, h.remoteAddress, errDifferentAccAuth)
		} else {
			h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		}

		SendErrorMsg(ctx, jsonrpc.InvalidRequest, errDifferentAccAuth, conn, req.ID)
		return
	}

	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCTxPayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	// if user tried to send transaction directly to the internal gateway, return error
	if h.FeedManager.accountModel.AccountID == types.BloxrouteAccountID && types.AccountID(params.OriginalSenderAccountID) == types.EmptyAccountID {
		h.log.Errorf("cannot send transaction to internal gateway directly")
		SendErrorMsg(ctx, jsonrpc.InvalidRequest, "failed to send transaction", conn, req.ID)
		return
	}

	var ws connections.RPCConn
	if h.connectionAccount.AccountID == types.BloxrouteAccountID {
		// Tx sent from cloud services, need to update account ID of the connection to be the origin sender
		ws = connections.NewRPCConn(types.AccountID(params.OriginalSenderAccountID), h.remoteAddress, h.FeedManager.networkNum, utils.CloudAPI)
	} else {
		ws = connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.FeedManager.networkNum, utils.Websocket)
	}

	txHash, ok, err := HandleSingleTransaction(h.FeedManager, params.Transaction, nil, ws, params.ValidatorsOnly,
		params.NextValidator, params.NodeValidation, params.FrontRunningProtection, params.Fallback,
		h.FeedManager.nextValidatorMap, h.FeedManager.validatorStatusMap)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
	}
	if !ok {
		return
	}

	response := rpcTxResponse{
		TxHash: txHash,
	}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}

	h.log.Infof("blxr_tx: hash - 0x%v", response.TxHash)
}
