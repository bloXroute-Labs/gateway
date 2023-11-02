package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *handlerObj) handleRPCUnsubscribe(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params []string
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCUnsubscribe, err), conn, req.ID)
		return
	}

	if len(params) != 1 {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("received invalid number of params: expected 1, got %v",
			len(params)), conn, req.ID)
		return
	}

	uid := params[0]
	if err = h.FeedManager.Unsubscribe(uid, false, ""); err != nil {
		h.log.Warnf("subscription id %v was not found", uid)

		if err = conn.Reply(ctx, req.ID, "false"); err != nil {
			h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
			SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
			return
		}

		return
	}

	if err = conn.Reply(ctx, req.ID, "true"); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}
}
