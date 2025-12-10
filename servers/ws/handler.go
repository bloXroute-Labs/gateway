package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

var (
	errParamsValueIsMissing = "params is missing in the request"
	errFDifferentAccAuth    = "%s is not allowed when account authentication is different from the node account"
)

// include field constants
const (
	includeTransactions              = "transactions"
	includeTransactionsWithoutSender = "transactions_without_sender"
)

type handlerObj struct {
	sdn                      sdnsdk.SDNHTTP
	node                     connections.BxListener
	feedManager              *feed.Manager
	chainID                  bxtypes.NetworkID
	nodeWSManager            blockchain.WSManager
	log                      *log.Entry
	networkNum               bxtypes.NetworkNum
	remoteAddress            string
	connectionAccount        sdnmessage.Account
	serverAccountID          bxtypes.AccountID
	ethSubscribeIDToChanMap  map[string]chan bool
	headers                  map[string]string
	stats                    statistics.Stats
	pendingTxsSourceFromNode bool
	enableBlockchainRPC      bool
	txFromFieldIncludable    bool
	oFACList                 *types.OFACMap
	senderExtractor          *services.SenderExtractor
}

// Handle handling client requests
func (h *handlerObj) Handle(ctx context.Context, conn *conn, req Request) {
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
		response, err := h.sdn.GetQuotaUsage(string(h.connectionAccount.AccountID))
		if err != nil {
			sendErrorMsg(ctx, jsonrpc.MethodNotFound, fmt.Sprintf("failed to fetch quota usage: %v", err), conn, req.ID)
			return
		}
		if err = conn.Reply(ctx, req.ID, response); err != nil {
			h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		}
	case jsonrpc.RPCBundleSubmission:
	// Do nothing. Bundle propagation no longer accepted as of
	// https://bloxroute.atlassian.net/browse/BP-3153
	case jsonrpc.RPCChangeNewPendingTxFromNode:
		h.handleRPCNewPendingTxsSourceFromNode(ctx, conn, req)
	default:
		if !h.enableBlockchainRPC {
			err := fmt.Errorf("got unsupported method name: %v", req.Method)
			sendErrorMsg(ctx, jsonrpc.MethodNotFound, err.Error(), conn, req.ID)
			return
		}
		ws, synced := h.nodeWSManager.SyncedProvider()
		if !synced {
			sendErrorMsg(ctx, jsonrpc.MethodNotFound, fmt.Sprintf("your blockchain node is either not synced or the gateway does not "+
				"have an active websocket connection to the node - request %v was not sent in order to prevent errors", req.Method), conn, req.ID)
			return
		}

		// only unmarshal params if they are present in the request
		var rpcParams []interface{}
		if req.Params != nil {
			err := json.Unmarshal(*req.Params, &rpcParams)
			if err != nil {
				sendErrorMsg(ctx, jsonrpc.InvalidRequest, fmt.Sprintf("unable to forward RPC request %v to node, "+
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

func (h *handlerObj) buildNotificationContent(notification types.Notification, includes []string) types.Notification {
	// for block feeds, replace 'transactions' with 'transactions_without_sender' for WithFields
	switch notification.NotificationType() {
	case types.NewBlocksFeed, types.BDNBlocksFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed:
		newIncludes := replaceTransactionsWithourSenderIfNeeded(includes)
		content := notification.WithFields(newIncludes)
		if blockContent, ok := content.(*types.EthBlockNotification); ok {
			senders := h.senderExtractor.GetSendersFromBlockTxs(blockContent.Block)
			blockContent.GetTxs(senders)
			return blockContent
		}
		return content
	default:
		return notification.WithFields(includes)
	}
}

// sendNotification - build a response according to client request and notify client
func (h *handlerObj) sendNotification(ctx context.Context, subscriptionID string, clientReq *ClientReq, conn *conn, notification types.Notification) error {
	response := BlockResponse{
		Subscription: subscriptionID,
	}
	// prepare includes: if client requested raw transactions, map "transactions" -> "raw_transactions" for WithFields
	includes := make([]string, len(clientReq.Includes))
	copy(includes, clientReq.Includes)
	if !clientReq.ParsedTxs {
		for i, inc := range includes {
			if inc == "transactions" {
				includes[i] = "raw_transactions"
			}
		}
	}

	response.Result = h.buildNotificationContent(notification, includes)
	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		if !errors.Is(err, ErrClosed) {
			h.log.Errorf("error reply to subscriptionID %v: %v", subscriptionID, err.Error())
		}
		return err
	}
	return nil
}

func replaceTransactionsWithourSenderIfNeeded(includes []string) []string {
	if !slices.Contains(includes, includeTransactions) {
		return includes
	}
	newIncludes := make([]string, 0, len(includes))
	for _, inc := range includes {
		if inc == includeTransactions {
			continue
		}
		newIncludes = append(newIncludes, inc)
	}
	newIncludes = append(newIncludes, includeTransactionsWithoutSender)
	return newIncludes
}
