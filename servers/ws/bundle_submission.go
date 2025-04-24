package ws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sourcegraph/jsonrpc2"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
)

func (h *handlerObj) handleRPCBundleSubmission(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if h.serverAccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCBundleSubmission)
		h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.serverAccountID)
		sendErrorMsg(ctx, jsonrpc.AccountIDError, errDifferentAccAuth, conn, req.ID)
		return
	}

	if req.Params == nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCBundleSubmissionPayload

	if err := json.Unmarshal(*req.Params, &params); err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCBundleSubmission, err), conn, req.ID)
		return
	}

	var ws connections.RPCConn
	if h.connectionAccount.AccountID == bxtypes.BloxrouteAccountID {
		// reject bundle if sent from Bloxroute, but has empty original sender
		if params.OriginalSenderAccountID == "" {
			sendErrorMsg(ctx, jsonrpc.InvalidParams, "original sender account ID param is missing", conn, req.ID)
			return
		}
		// Bundle sent from cloud services, need to update account ID of the connection to be the origin sender
		ws = connections.NewRPCConn(bxtypes.AccountID(params.OriginalSenderAccountID), h.remoteAddress, h.networkNum, bxtypes.CloudAPI)
	} else {
		ws = connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.networkNum, bxtypes.Websocket)
	}

	result, errCode, err := handler.HandleMEVBundle(h.node, ws, h.connectionAccount, &params)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.RPCErrorCode(errCode), err.Error(), conn, req.ID)
		return
	}
	if err = conn.Reply(ctx, req.ID, result); err != nil {
		log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
	}
}
