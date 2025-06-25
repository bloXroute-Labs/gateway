package ws

import (
	"context"
	"fmt"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
)

func (h *handlerObj) handleRPCEthSendTx(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, rpcParams []interface{}) {
	if len(rpcParams) != 1 {
		err := fmt.Sprintf("unable to process %v RPC request: expected 1, got %d", jsonrpc.RPCEthSendRawTransaction, len(rpcParams))
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err, conn, req.ID)
		return
	}

	rawTxParam := rpcParams[0]
	var rawTxStr string
	switch rawTxParam.(type) {
	case string:
		rawTxStr = rawTxParam.(string)
		if len(rawTxStr) < 3 {
			err := fmt.Sprintf("unable to process %s RPC request: raw transaction string is too short", jsonrpc.RPCEthSendRawTransaction)
			sendErrorMsg(ctx, jsonrpc.InvalidParams, err, conn, req.ID)
			return
		}
		if rawTxStr[0:2] != "0x" {
			err := fmt.Sprintf("unable to process %s RPC request: expected raw transaction string to begin with '0x'", jsonrpc.RPCEthSendRawTransaction)
			sendErrorMsg(ctx, jsonrpc.InvalidParams, err, conn, req.ID)
			return
		}
		rawTxStr = rawTxStr[2:]
	default:
		err := fmt.Sprintf("unable to process %s RPC request: param must be a raw transaction string", jsonrpc.RPCEthSendRawTransaction)
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err, conn, req.ID)
		return
	}

	reqWS := connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.networkNum, bxtypes.Websocket)
	txHash, ok, err := handler.HandleSingleTransaction(h.node, h.nodeWSManager, rawTxStr, nil, reqWS, false, h.chainID)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
	}
	if !ok {
		return
	}

	if err = conn.Reply(ctx, req.ID, "0x"+txHash); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
	}
}
