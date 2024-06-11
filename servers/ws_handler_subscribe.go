package servers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	feed, rpcParams, err := h.parseSubscriptionRequest(req)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	// check if the account has the right tier to access
	// if "allowIntroductoryTierAccess" == false, then this check was already done before creating the connection
	if h.allowIntroductoryTierAccess && feed != types.UserIntentsFeed && feed != types.UserIntentSolutionsFeed && !h.connectionAccount.TierName.IsEnterprise() {
		SendErrorMsg(ctx, jsonrpc.Blocked, "account must be enterprise / enterprise elite / ultra", conn, req.ID)
		conn.Close()
		return
	}

	if len(h.FeedManager.nodeWSManager.Providers()) == 0 && feed == types.NewBlocksFeed &&
		h.FeedManager.networkNum != bxgateway.MainnetNum && h.FeedManager.networkNum != bxgateway.HoleskyNum {
		errMsg := fmt.Sprintf("%v Feed requires a websockets endpoint to be specifed via either --eth-ws-uri or --multi-node startup parameter", feed)
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, req.ID)
		return
	}

	var request *ClientReq
	var postRun func()
	if feed == types.UserIntentsFeed || feed == types.UserIntentSolutionsFeed {
		request, postRun, err = h.createIntentClientReq(req, feed, rpcParams)
	} else {
		request, err = h.createClientReq(req, feed, rpcParams)
	}
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	if request.MultiTxs && feed != types.NewTxsFeed && feed != types.PendingTxsFeed {
		log.Debugf("multi tx support only in new txs or pending txs, account id %v, remote addr %v", h.connectionAccount.AccountID, h.remoteAddress)
		SendErrorMsg(ctx, jsonrpc.InvalidParams, "multi tx support only in new txs or pending txs", conn, req.ID)
		return
	}

	ci, ro := h.createClientInfoAndRequestOpts(feed, request)

	sub, errSubscribe := h.FeedManager.Subscribe(request.Feed, types.WebSocketFeed, conn, ci, ro, false)
	if errSubscribe != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errSubscribe.Error(), conn, req.ID)
		return
	}
	subscriptionID := sub.SubscriptionID

	defer func() {
		h.FeedManager.Unsubscribe(subscriptionID, false, "")

		if postRun != nil {
			postRun()
		}
	}()

	if err = conn.Reply(ctx, req.ID, subscriptionID); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		SendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}
	h.FeedManager.stats.LogSubscribeStats(subscriptionID,
		h.connectionAccount.AccountID,
		feed,
		h.connectionAccount.TierName,
		h.remoteAddress,
		h.FeedManager.networkNum,
		request.Includes,
		ro.Filters)

	if request.MultiTxs {
		err = h.subscribeMultiTxs(ctx, sub.FeedChan, subscriptionID, request, conn, req, feed)
		if err != nil {
			log.Errorf("error while processing %v (%v) with multi tx argument: %v", feed, subscriptionID, err)
			return
		}

		return
	}

	h.handleRPCSubscribeNotify(ctx, conn, req.ID, sub, subscriptionID, feed, request)
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
		case notification, ok := <-sub.FeedChan:
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
			case types.UserIntentsFeed:
				in := notification.(*types.UserIntentNotification)
				if h.sendIntentNotification(ctx, subscriptionID, request, conn, in) != nil {
					return
				}
			case types.UserIntentSolutionsFeed:
				in := notification.(*types.UserIntentSolutionNotification)
				if in.DappAddress != "" && len(request.Includes) == 1 && in.DappAddress == request.Includes[0] {
					if h.sendIntentSolutionNotification(ctx, subscriptionID, conn, in) != nil {
						return
					}
				}
			}
		}
	}
}

func (h *handlerObj) createClientInfoAndRequestOpts(feed types.FeedType, request *ClientReq) (types.ClientInfo, types.ReqOptions) {
	ci := types.ClientInfo{
		RemoteAddress: h.remoteAddress,
		AccountID:     h.connectionAccount.AccountID,
		Tier:          string(h.connectionAccount.TierName),
		MetaInfo:      h.headers,
	}

	if feed == types.UserIntentsFeed || feed == types.UserIntentSolutionsFeed {
		return ci, types.ReqOptions{}
	}

	var filters string
	if request.Expr != nil {
		filters = request.Expr.String()
	}
	ro := types.ReqOptions{
		Filters:  filters,
		Includes: strings.Join(request.Includes, ","),
	}

	return ci, ro
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

func (h *handlerObj) sendIntentNotification(ctx context.Context, subscriptionID string, clientReq *ClientReq, conn *jsonrpc2.Conn, in *types.UserIntentNotification) error {
	response := userIntentResponse{
		Subscription: subscriptionID,
		Result: userIntentNotification{
			DappAddress:   in.DappAddress,
			SenderAddress: in.SenderAddress,
			IntentID:      in.ID,
			Intent:        in.Intent,
			Timestamp:     in.Timestamp.Format(time.RFC3339),
		},
	}

	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		h.log.Errorf("error reply to subscriptionID %v: %v", subscriptionID, err.Error())
		return err
	}

	return nil
}

func (h *handlerObj) sendIntentSolutionNotification(ctx context.Context, subscriptionID string, conn *jsonrpc2.Conn, in *types.UserIntentSolutionNotification) error {
	response := userIntentSolutionResponse{
		Subscription: subscriptionID,
		Result: userIntentSolutionNotification{
			IntentID:       in.ID,
			IntentSolution: in.Solution,
		},
	}

	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		h.log.Errorf("error reply to subscriptionID %v: %v", subscriptionID, err.Error())
		return err
	}

	return nil
}
