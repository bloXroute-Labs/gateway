package ws

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
)

func (h *handlerObj) handleRPCEthUnsubscribe(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, rpcParams []interface{}) {
	if len(rpcParams) != 1 {
		err := fmt.Sprintf("unable to process %s RPC request: expected 1 param, got %v", jsonrpc.RPCEthUnsubscribe, len(rpcParams))
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err, conn, req.ID)
		return
	}

	sid := rpcParams[0].(string)
	cancel, exit := h.ethSubscribeIDToChanMap[sid]
	if !exit {
		// unsubscribe with Feed manager
		if err := h.feedManager.Unsubscribe(sid, false, ""); err != nil {
			h.log.Infof("subscription id %v was not found", sid)

			if err = conn.Reply(ctx, req.ID, "false"); err != nil {
				h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
				sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
				return
			}
			return
		}
	} else {
		// unsubscribe by finding generated sid, and send signal to cancel chan
		cancel <- true
	}

	if err := conn.Reply(ctx, req.ID, "true"); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}
}
