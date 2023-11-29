package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/sourcegraph/jsonrpc2"
)

var (
	errParamsValueIsMissing = "params is missing in the request"
	errFDifferentAccAuth    = "%s is not allowed when account authentication is different from the node account"
)

type handlerObj struct {
	FeedManager              *FeedManager
	ClientReq                *clientReq
	remoteAddress            string
	connectionAccount        sdnmessage.Account
	getQuotaUsage            func(accountID string) (*connections.QuotaResponseBody, error)
	enableBlockchainRPC      bool
	pendingTxsSourceFromNode *bool
	log                      *log.Entry
	ethSubscribeIDToChanMap  map[string]chan bool
	headers                  map[string]string
	stats                    statistics.Stats
	txFromFieldIncludable    bool
}

// Handle handling client requests
func (h *handlerObj) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	start := time.Now()
	defer func() {
		h.log.Debugf("websocket handling for method %v ended. Duration %v", jsonrpc.RPCRequestType(req.Method), time.Since(start))
	}()

	switch jsonrpc.RPCRequestType(req.Method) {
	case jsonrpc.RPCSubscribe:
		h.handleRPCSubscribe(ctx, conn, req)
	case jsonrpc.RPCUnsubscribe:
		h.handleRPCUnsubscribe(ctx, conn, req)
	case jsonrpc.RPCTx:
		h.handleRPCTx(ctx, conn, req)
	case jsonrpc.RPCBatchTx:
		h.handleRPCBatchTx(ctx, conn, req)
	case jsonrpc.RPCPing:
		response := rpcPingResponse{
			Pong: time.Now().UTC().Format(bxgateway.MicroSecTimeFormat),
		}
		if err := conn.Reply(ctx, req.ID, response); err != nil {
			h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		}
	case jsonrpc.RPCQuotaUsage:
		response, err := h.getQuotaUsage(string(h.connectionAccount.AccountID))
		if err != nil {
			SendErrorMsg(ctx, jsonrpc.MethodNotFound, fmt.Sprintf("failed to fetch quota usage: %v", err), conn, req.ID)
			return
		}
		if err = conn.Reply(ctx, req.ID, response); err != nil {
			h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		}
	case jsonrpc.RPCMEVSearcher:
		h.handleRPCMevSearcher(ctx, conn, req)
	case jsonrpc.RPCBundleSubmission:
		h.handleRPCBundleSubmission(ctx, conn, req)
	case jsonrpc.RPCChangeNewPendingTxFromNode:
		h.handleRPCNewPendingTxsSourceFromNode(ctx, conn, req)
	default:
		if !h.enableBlockchainRPC {
			err := fmt.Errorf("got unsupported method name: %v", req.Method)
			SendErrorMsg(ctx, jsonrpc.MethodNotFound, err.Error(), conn, req.ID)
			return
		}
		ws, synced := h.FeedManager.nodeWSManager.SyncedProvider()
		if !synced {
			SendErrorMsg(ctx, jsonrpc.MethodNotFound, fmt.Sprintf("your blockchain node is either not synced or the gateway does not "+
				"have an active websocket connection to the node - request %v was not sent in order to prevent errors", req.Method), conn, req.ID)
			return
		}

		// only unmarshal params if they are present in the request
		var rpcParams []interface{}
		if req.Params != nil {
			err := json.Unmarshal(*req.Params, &rpcParams)
			if err != nil {
				SendErrorMsg(ctx, jsonrpc.InvalidRequest, fmt.Sprintf("unable to forward RPC request %v to node, "+
					"failed to unmarshal params %v: %v", req.Method, req.Params, err), conn, req.ID)
				return
			}
		}

		switch jsonrpc.RPCRequestType(req.Method) {
		case jsonrpc.RPCEthSendRawTransaction:
			h.handleRPCEthSendTx(ctx, conn, req, rpcParams)
		case jsonrpc.RPCEthSubscribe:
			h.handleRPCEthSubscribe(ctx, conn, req, ws, rpcParams)
		case jsonrpc.RPCEthUnsubscribe:
			h.handleRPCEthUnsubscribe(ctx, conn, req, rpcParams)
		default:
			response, nodeErr := ws.CallRPC(req.Method, rpcParams, blockchain.DefaultRPCOptions)
			if nodeErr != nil {
				if err := conn.Reply(ctx, req.ID, nodeErr); err != nil {
					h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
				}
				return
			}

			if err := conn.Reply(ctx, req.ID, response); err != nil {
				h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
				return
			}
		}
	}
}

// sendNotification - build a response according to client request and notify client
func (h *handlerObj) sendNotification(ctx context.Context, subscriptionID string, clientReq *clientReq, conn *jsonrpc2.Conn, notification types.Notification) error {
	response := BlockResponse{
		Subscription: subscriptionID,
	}
	content := notification.WithFields(clientReq.includes)
	response.Result = content
	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		h.log.Errorf("error reply to subscriptionID %v: %v", subscriptionID, err.Error())
		return err
	}
	return nil
}
