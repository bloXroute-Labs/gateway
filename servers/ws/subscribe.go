package ws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/zhouzhuojie/conditions"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/filter"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

var errReadingNotification = errors.New("error when reading new notification")

func (h *handlerObj) handleRPCSubscribe(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	feed, rpcParams, err := h.parseSubscriptionRequest(req)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	if len(h.nodeWSManager.Providers()) == 0 && feed == types.NewBlocksFeed &&
		h.networkNum != bxtypes.MainnetNum && h.networkNum != bxtypes.HoleskyNum {
		errMsg := fmt.Sprintf("%v Feed requires a websockets endpoint to be specifed via either --eth-ws-uri or --multi-node startup parameter", feed)
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, req.ID)
		return
	}

	var request *ClientReq

	request, err = h.createClientReq(req, feed, rpcParams)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	if request.MultiTxs && feed != types.NewTxsFeed && feed != types.PendingTxsFeed {
		log.Debugf("multi tx support only in new txs or pending txs, account id %v, remote addr %v", h.connectionAccount.AccountID, h.remoteAddress)
		sendErrorMsg(ctx, jsonrpc.InvalidParams, "multi tx support only in new txs or pending txs", conn, req.ID)
		return
	}

	ci, ro := h.createClientInfoAndRequestOpts(request)

	sub, errSubscribe := h.feedManager.Subscribe(feed, types.WebSocketFeed, conn, ci, ro, false)
	if errSubscribe != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errSubscribe.Error(), conn, req.ID)
		return
	}
	subscriptionID := sub.SubscriptionID

	defer func() {
		err = h.feedManager.Unsubscribe(subscriptionID, false, "")
		// unsubscription can be done by client if he sends unsubscribe request so no need to log if subscription not found
		if err != nil && !errors.Is(err, bxgateway.ErrSubscriptionNotFound) {
			h.log.Errorf("failed to unsubscribe from %v, subscriptionID %v: %v", feed, subscriptionID, err)
		}
	}()

	if err = conn.Reply(ctx, req.ID, subscriptionID); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}
	h.stats.LogSubscribeStats(subscriptionID,
		h.connectionAccount.AccountID,
		feed,
		h.connectionAccount.TierName,
		h.remoteAddress,
		h.networkNum,
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
	reqID jsonrpc2.ID, sub *feed.ClientSubscriptionHandlingInfo, subscriptionID string, feedName types.FeedType, request *ClientReq) {

	for {
		select {
		case <-conn.DisconnectNotify():
			return
		case errMsg := <-sub.ErrMsgChan:
			sendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, reqID)
			return
		case notification, ok := <-sub.FeedChan:
			if !ok {
				if h.feedManager.SubscriptionExists(subscriptionID) {
					sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, reqID)
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

				err := handler.HandleEthOnBlock(h.nodeWSManager, block, *request.calls, sendEthOnBlockWsNotification)
				if err != nil {
					sendErrorMsg(ctx, jsonrpc.InvalidRequest, err.Error(), conn, reqID)
					return
				}
			}
		}
	}
}

func (h *handlerObj) createClientInfoAndRequestOpts(request *ClientReq) (types.ClientInfo, types.ReqOptions) {
	ci := types.ClientInfo{
		RemoteAddress: h.remoteAddress,
		AccountID:     h.connectionAccount.AccountID,
		Tier:          string(h.connectionAccount.TierName),
		MetaInfo:      h.headers,
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
		if !errors.Is(err, jsonrpc2.ErrClosed) {
			h.log.Errorf("error notifying subscriptionID %v: %v", subscriptionID, err)
		}
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
			if !errors.Is(err, jsonrpc2.ErrClosed) {
				h.log.Errorf("error reply to subscriptionID %v: %v", subscriptionID, err.Error())
			}
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
				if h.feedManager.SubscriptionExists(subscriptionID) {
					sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
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
						if h.feedManager.SubscriptionExists(subscriptionID) {
							sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
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
					if !errors.Is(err, jsonrpc2.ErrClosed) {
						h.log.Errorf("error notifying subscriptionID %v: %v", subscriptionID, err)
					}
					return err
				}
			}
		}
	}
}

func filterAndIncludeTx(clientReq *ClientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID bxtypes.AccountID) *TxResult {
	if !shouldSendTx(clientReq, tx, remoteAddress, accountID) {
		return nil
	}

	return includeTx(clientReq, tx)
}

func shouldSendTx(clientReq *ClientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID bxtypes.AccountID) bool {
	if clientReq.Expr == nil {
		return true
	}

	filters := clientReq.Expr.Args()
	txFilters := tx.Filters(filters)

	// should be done after tx.Filters() to avoid nil pointer dereference
	txType := tx.BlockchainTransaction.(*types.EthTransaction).Type()

	if !filter.IsFiltersSupportedByTxType(txType, filters) {
		return false
	}

	// Evaluate if we should send the tx
	shouldSend, err := conditions.Evaluate(clientReq.Expr, txFilters)
	if err != nil {
		log.Errorf("error evaluate Filters. Feed: %v. filters: %s. remote address: %v. account id: %v error - %v tx: %v",
			clientReq.Feed, clientReq.Expr, remoteAddress, accountID, err.Error(), txFilters)
		return false
	}

	return shouldSend
}

func includeTx(clientReq *ClientReq, tx *types.NewTransactionNotification) *TxResult {
	hasTxContent := false
	var response TxResult
	for _, param := range clientReq.Includes {
		switch param {
		case "tx_hash":
			txHash := tx.GetHash()
			response.TxHash = &txHash
		case "time":
			timeNow := time.Now().Format(bxgateway.MicroSecTimeFormat)
			response.Time = &timeNow
		case "local_region":
			localRegion := tx.LocalRegion()
			response.LocalRegion = &localRegion
		case "raw_tx":
			rawTx := hexutil.Encode(tx.RawTx())
			response.RawTx = &rawTx
		default:
			if !hasTxContent && strings.HasPrefix(param, "tx_contents.") {
				hasTxContent = true
			}
		}
	}

	if hasTxContent {
		fields := tx.Fields(clientReq.Includes)
		if fields == nil {
			log.Errorf("Got nil from tx.Fields - need to be checked")
			return nil
		}
		response.TxContents = fields
	}

	return &response
}
