package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *handlerObj) handleRPCNewPendingTxsSourceFromNode(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCChangeNewPendingTxFromNode)
		h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		SendErrorMsg(ctx, jsonrpc.AccountIDError, errDifferentAccAuth, conn, req.ID)
		return
	}

	var update bool
	if err := json.Unmarshal(*req.Params, &update); err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCChangeNewPendingTxFromNode, err), conn, req.ID)
		return
	}

	h.log.Infof("received %v request, changing it from %v to %v ", jsonrpc.RPCChangeNewPendingTxFromNode, *h.pendingTxsSourceFromNode, update)

	*h.pendingTxsSourceFromNode = update
	if err := conn.Reply(ctx, req.ID, "succeed"); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
	}
}
