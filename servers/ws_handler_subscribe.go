package servers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
)

var (
	errReadingNotification = errors.New("error when reading new notification")
)

func (h *handlerObj) handleRPCSubscribe(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	request, err := h.createClientReq(req)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}
	feedName := request.Feed

	if len(h.FeedManager.nodeWSManager.Providers()) == 0 && feedName == types.NewBlocksFeed &&
		h.FeedManager.networkNum != bxgateway.MainnetNum && h.FeedManager.networkNum != bxgateway.HoleskyNum {
		errMsg := fmt.Sprintf("%v Feed requires a websockets endpoint to be specifed via either --eth-ws-uri or --multi-node startup parameter", feedName)
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, req.ID)
		return
	}

	var filters string
	if request.Expr != nil {
		filters = request.Expr.String()
	}
	ro := types.ReqOptions{
		Filters:  filters,
		Includes: strings.Join(request.Includes, ","),
	}
	ci := types.ClientInfo{
		RemoteAddress: h.remoteAddress,
		AccountID:     h.connectionAccount.AccountID,
		Tier:          string(h.connectionAccount.TierName),
		MetaInfo:      h.headers,
	}

	sub, errSubscribe := h.FeedManager.Subscribe(request.Feed, types.WebSocketFeed, conn, ci, ro, false)
	if errSubscribe != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errSubscribe.Error(), conn, req.ID)
		return
	}
	subscriptionID := sub.SubscriptionID

	defer h.FeedManager.Unsubscribe(subscriptionID, false, "")

	if err = conn.Reply(ctx, req.ID, subscriptionID); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}
	h.FeedManager.stats.LogSubscribeStats(subscriptionID,
		h.connectionAccount.AccountID,
		feedName,
		h.connectionAccount.TierName,
		h.remoteAddress,
		h.FeedManager.networkNum,
		request.Includes,
		filters,
		"")

	if request.MultiTxs {
		if feedName != types.NewTxsFeed && feedName != types.PendingTxsFeed {
			log.Debugf("multi tx support only in new txs or pending txs, subscription id %v, account id %v, remote addr %v", subscriptionID, h.connectionAccount.AccountID, h.remoteAddress)
			SendErrorMsg(ctx, jsonrpc.InvalidParams, "multi tx support only in new txs or pending txs", conn, req.ID)
			return
		}
		err = h.subscribeMultiTxs(ctx, sub.FeedChan, subscriptionID, request, conn, req, feedName)
		if err != nil {
			log.Errorf("error while processing %v (%v) with multi tx argument: %v", feedName, subscriptionID, err)
			return
		}
	}

	h.handleRPCSubscribeNotify(ctx, conn, req.ID, sub, subscriptionID, feedName, request)
}

func (h *handlerObj) handleRPCSubscribeNotify(ctx context.Context, conn *jsonrpc2.Conn,
	reqID jsonrpc2.ID, sub *ClientSubscriptionHandlingInfo, subscriptionID string, feedName types.FeedType, request *ClientReq) {

	for {
		select {
		case <-conn.DisconnectNotify():
			return
		case errMsg := <-sub.ErrMsgChan:
			SendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, reqID)
			return
		case notification, ok := <-(sub.FeedChan):
			if !ok {
				if h.FeedManager.SubscriptionExists(subscriptionID) {
					SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, reqID)
				}
				return
			}

			switch feedName {
			case types.NewTxsFeed:
				tx := (notification).(*types.NewTransactionNotification)
				if h.sendTxNotification(ctx, subscriptionID, request, conn, tx) != nil {
					return
				}
			case types.PendingTxsFeed:
				tx := (notification).(*types.PendingTransactionNotification)
				if h.sendTxNotification(ctx, subscriptionID, request, conn, &tx.NewTransactionNotification) != nil {
					return
				}
			case types.BDNBlocksFeed, types.NewBlocksFeed, types.NewBeaconBlocksFeed, types.BDNBeaconBlocksFeed:
				if h.sendNotification(ctx, subscriptionID, request, conn, notification) != nil {
					return
				}
			case types.TxReceiptsFeed:
				if h.sendTxReceiptNotification(ctx, subscriptionID, request, conn, notification) != nil {
					return
				}
			case types.OnBlockFeed:
				block := notification.(*types.EthBlockNotification)

				sendEthOnBlockWsNotification := func(notification *types.OnBlockNotification) error {
					return h.sendNotification(ctx, subscriptionID, request, conn, notification)
				}

				err := HandleEthOnBlock(h.FeedManager.nodeWSManager, h.FeedManager, block, *request.calls, sendEthOnBlockWsNotification)
				if err != nil {
					SendErrorMsg(ctx, jsonrpc.InvalidRequest, err.Error(), conn, reqID)
					return
				}
			}
		}
	}
}

// sendTxNotification - build a response according to client request and notify client
func (h *handlerObj) sendTxNotification(ctx context.Context, subscriptionID string, clientReq *ClientReq, conn *jsonrpc2.Conn, tx *types.NewTransactionNotification) error {
	result := filterAndIncludeTx(clientReq, tx, h.remoteAddress, h.connectionAccount.AccountID)
	if result == nil {
		return nil
	}
	response := TxResponse{
		Subscription: subscriptionID,
		Result:       *result,
	}

	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		h.log.Errorf("error notifying subscriptionID %v: %v", subscriptionID, err)
		return err
	}

	return nil
}

func (h *handlerObj) sendTxReceiptNotification(ctx context.Context, subscriptionID string, clientReq *ClientReq, conn *jsonrpc2.Conn, notification types.Notification) error {
	response := txReceiptResponse{
		Subscription: subscriptionID,
	}
	content := notification.WithFields(clientReq.Includes).(*types.TxReceiptsNotification)
	for _, receipt := range content.Receipts {
		response.Result = receipt
		err := conn.Notify(ctx, "subscribe", response)
		if err != nil {
			h.log.Errorf("error reply to subscriptionID %v: %v", subscriptionID, err.Error())
			return err
		}
	}

	return nil
}

func (h *handlerObj) subscribeMultiTxs(ctx context.Context, feedChan chan types.Notification, subscriptionID string, clientReq *ClientReq, conn *jsonrpc2.Conn, req *jsonrpc2.Request, feedName types.FeedType) error {
	for {
		select {
		case <-conn.DisconnectNotify():
			return nil
		case notification, ok := <-feedChan:
			if !ok {
				if h.FeedManager.SubscriptionExists(subscriptionID) {
					SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
				}
				return errReadingNotification
			}

			continueProcessing := true
			multiTxsResponse := MultiTransactions{Subscription: subscriptionID}

			switch feedName {
			case types.NewTxsFeed:
				tx := (notification).(*types.NewTransactionNotification)
				response := filterAndIncludeTx(clientReq, tx, h.remoteAddress, h.connectionAccount.AccountID)
				if response != nil {
					multiTxsResponse.Result = append(multiTxsResponse.Result, *response)
				}
			case types.PendingTxsFeed:
				tx := (notification).(*types.PendingTransactionNotification)
				response := filterAndIncludeTx(clientReq, &tx.NewTransactionNotification, h.remoteAddress, h.connectionAccount.AccountID)
				if response != nil {
					multiTxsResponse.Result = append(multiTxsResponse.Result, *response)
				}
			}
			for continueProcessing {
				select {
				case <-conn.DisconnectNotify():
					return nil
				case notification, ok := <-feedChan:
					if !ok {
						if h.FeedManager.SubscriptionExists(subscriptionID) {
							SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
						}
						return errReadingNotification
					}
					switch feedName {
					case types.NewTxsFeed:
						tx := (notification).(*types.NewTransactionNotification)
						response := filterAndIncludeTx(clientReq, tx, h.remoteAddress, h.connectionAccount.AccountID)
						if response != nil {
							multiTxsResponse.Result = append(multiTxsResponse.Result, *response)
						}
					case types.PendingTxsFeed:
						tx := (notification).(*types.PendingTransactionNotification)
						response := filterAndIncludeTx(clientReq, &tx.NewTransactionNotification, h.remoteAddress, h.connectionAccount.AccountID)
						if response != nil {
							multiTxsResponse.Result = append(multiTxsResponse.Result, *response)
						}
					}
					if len(multiTxsResponse.Result) >= 50 {
						continueProcessing = false
					}
				default:
					continueProcessing = false
				}
			}
			if len(multiTxsResponse.Result) > 0 {
				err := conn.Notify(ctx, "subscribe", multiTxsResponse)
				if err != nil {
					h.log.Errorf("error notifying subscriptionID %v: %v", subscriptionID, err)
					return err
				}
			}
		}
	}
}
