package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *handlerObj) handleRPCBundleSubmission(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCBundleSubmission)
		h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		SendErrorMsg(ctx, jsonrpc.AccountIDError, errDifferentAccAuth, conn, req.ID)
		return
	}

	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCBundleSubmissionPayload

	if err := json.Unmarshal(*req.Params, &params); err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCBundleSubmission, err), conn, req.ID)
		return
	}

	var ws connections.RPCConn
	if h.connectionAccount.AccountID == types.BloxrouteAccountID {
		// Bundle sent from cloud services, need to update account ID of the connection to be the origin sender
		ws = connections.NewRPCConn(types.AccountID(params.OriginalSenderAccountID), h.remoteAddress, h.FeedManager.networkNum, utils.CloudAPI)
	} else {
		ws = connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.FeedManager.networkNum, utils.Websocket)
	}

	result, errCode, err := HandleMEVBundle(h.FeedManager, ws, h.connectionAccount, &params)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.RPCErrorCode(errCode), err.Error(), conn, req.ID)
		return
	}
	if err = conn.Reply(ctx, req.ID, result); err != nil {
		log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
	}
}
