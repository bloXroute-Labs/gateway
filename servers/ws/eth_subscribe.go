package ws

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	feedTypeNewPendingTransactions = "newPendingTransactions"
	feedTypeNewHeads               = "newHeads"
)

// EthSubscribeTxResponse - response of the jsonrpc params
type EthSubscribeTxResponse struct {
	Subscription string `json:"subscription"`
	Result       string `json:"result"`
}

// EthSubscribeFeedResponse - response of the jsonrpc params
type EthSubscribeFeedResponse struct {
	Subscription string      `json:"subscription"`
	Result       interface{} `json:"result"`
}

func (h *handlerObj) handleRPCEthSubscribe(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, ws blockchain.WSProvider, rpcParams []interface{}) {
	if len(rpcParams) < 1 || len(rpcParams) > 2 {
		err := fmt.Sprintf("unable to process %v RPC request: expected at least 1 param but no more than 2, got %d", jsonrpc.RPCEthSubscribe, len(rpcParams))
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err, conn, req.ID)
		return
	}

	ci := types.ClientInfo{
		RemoteAddress: h.remoteAddress,
		AccountID:     h.connectionAccount.AccountID,
		Tier:          string(h.connectionAccount.TierName),
		MetaInfo:      h.headers,
	}

	feedType := rpcParams[0].(string)
	switch feedType {
	case feedTypeNewPendingTransactions:
		h.handleEthSubscribeNewPendingTxs(ctx, conn, req, ci)
	case feedTypeNewHeads:
		h.handleEthSubscribeNewHeads(ctx, conn, req, ci)
	default:
		h.handleEthSubscribeFeed(ctx, feedType, conn, req, ws, rpcParams)
	}
}

func (h *handlerObj) handleEthSubscribeNewPendingTxs(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, ci types.ClientInfo) {
	var feed types.FeedType
	if h.pendingTxsSourceFromNode {
		feed = types.PendingTxsFeed
	} else {
		feed = types.NewTxsFeed
	}

	request := &ClientReq{
		Feed:     feed,
		Includes: []string{"tx_hash"},
	}

	ro := types.ReqOptions{
		Includes: strings.Join(request.Includes, ","),
	}
	// since we are replacing newPendingTransactions with newTxs/pendingTx, any existing newTxs/pendingTxs suppose to make newPendingTransactions a duplicate subscription.
	// But this is used only in external gateway where gateway account id is the same with request account id, so this is avoided
	sub, errSubscribe := h.feedManager.Subscribe(request.Feed, types.WebSocketFeed, conn, ci, ro, true)
	if errSubscribe != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errSubscribe.Error(), conn, req.ID)
		return
	}

	subscriptionID := sub.SubscriptionID
	defer func() {
		err := h.feedManager.Unsubscribe(subscriptionID, false, "")
		if err != nil {
			h.log.Errorf("failed to unsubscribe from %v, method %v: %v", h.remoteAddress, req.Method, err)
		}
	}()

	if err := conn.Reply(ctx, req.ID, subscriptionID); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}

	for {
		select {
		case <-conn.DisconnectNotify():
			return
		case errMsg := <-sub.ErrMsgChan:
			sendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, req.ID)
			return
		case notification, ok := <-sub.FeedChan:
			if !ok {
				if h.feedManager.SubscriptionExists(subscriptionID) {
					sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
				}
				return
			}

			switch request.Feed {
			case types.NewTxsFeed:
				tx := (notification).(*types.NewTransactionNotification)
				if h.sendTxNotificationEthFormat(ctx, subscriptionID, request, conn, tx) != nil {
					return
				}
			case types.PendingTxsFeed:
				tx := (notification).(*types.PendingTransactionNotification)
				if h.sendTxNotificationEthFormat(ctx, subscriptionID, request, conn, &tx.NewTransactionNotification) != nil {
					return
				}
			}
		}
	}
}

func (h *handlerObj) handleEthSubscribeNewHeads(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, ci types.ClientInfo) {
	request := &ClientReq{
		Feed:     types.NewBlocksFeed,
		Includes: []string{"header", "hash", "tx_contents.nonce"},
	}

	ro := types.ReqOptions{
		Includes: strings.Join(request.Includes, ","),
	}
	// since we are replacing newPendingTransactions with newTxs/pendingTx, any existing newTxs/pendingTxs suppose to make newPendingTransactions a duplicate subscription.
	// But this is used only in external gateway where gateway account id is the same with request account id, so this is avoided
	sub, errSubscribe := h.feedManager.Subscribe(request.Feed, types.WebSocketFeed, conn, ci, ro, true)
	if errSubscribe != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errSubscribe.Error(), conn, req.ID)
		return
	}

	subscriptionID := sub.SubscriptionID
	defer func() {
		err := h.feedManager.Unsubscribe(subscriptionID, false, "")
		if err != nil {
			h.log.Errorf("failed to unsubscribe from %v, method %v: %v", h.remoteAddress, req.Method, err)
		}
	}()

	if err := conn.Reply(ctx, req.ID, subscriptionID); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}

	for {
		select {
		case <-conn.DisconnectNotify():
			return
		case errMsg := <-sub.ErrMsgChan:
			sendErrorMsg(ctx, jsonrpc.InvalidParams, errMsg, conn, req.ID)
			return
		case notification, ok := <-sub.FeedChan:
			if !ok {
				if h.feedManager.SubscriptionExists(subscriptionID) {
					sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
				}
				return
			}
			bxBlock := (notification).(*types.EthBlockNotification)
			NewHeadsBlock := types.NewHeadsBlockFromEthBlockNotification(bxBlock)
			if h.sendNotification(ctx, subscriptionID, request, conn, NewHeadsBlock) != nil {
				return
			}
		}
	}
}

func (h *handlerObj) handleEthSubscribeFeed(ctx context.Context, feedType string, conn *jsonrpc2.Conn, req *jsonrpc2.Request, ws blockchain.WSProvider, rpcParams []interface{}) {
	var err error
	var sub *blockchain.Subscription
	subscribeChan := make(chan interface{})

	if len(rpcParams) > 1 {
		sub, err = ws.Subscribe(subscribeChan, feedType, rpcParams[1])
	} else {
		sub, err = ws.Subscribe(subscribeChan, feedType)
	}

	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	subscription := sub.Sub.(*rpc.ClientSubscription)
	defer subscription.Unsubscribe()

	subscriptionID, err := utils.GenerateU128()
	if err != nil {
		h.log.Errorf("can't generate u128 subscription ID: %v", err.Error())
		sendErrorMsg(ctx, jsonrpc.InternalError, fmt.Sprintf("can't assign subscriptionID for %v request", feedType), conn, req.ID)
		return
	}

	cancelChan := make(chan bool, 1)
	h.ethSubscribeIDToChanMap[subscriptionID] = cancelChan
	defer delete(h.ethSubscribeIDToChanMap, subscriptionID)

	if err = conn.Reply(ctx, req.ID, subscriptionID); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		sendErrorMsg(ctx, jsonrpc.InternalError, string(rune(websocket.CloseMessage)), conn, req.ID)
		return
	}

	for {
		select {
		case <-cancelChan:
			return
		case <-conn.DisconnectNotify():
			return
		case err = <-subscription.Err():
			h.log.Errorf("failed to subscribe to eth_subscribe Feed %v: %v", feedType, err.Error())
			sendErrorMsg(ctx, jsonrpc.InternalError, fmt.Sprintf("failed to subscribe to eth_subscribe Feed %v: %v", feedType, err.Error()), conn, req.ID)
			return
		case response := <-subscribeChan:
			err = h.sendEthSubscribeNotification(ctx, subscriptionID, conn, response)
			if err != nil {
				h.log.Errorf("%v reply error - %v", req.Method, err)
				return
			}
		}
	}
}

// sendTxNotificationEthFormat builds a response according to client request and notifies the client
func (h *handlerObj) sendTxNotificationEthFormat(ctx context.Context, subscriptionID string, clientReq *ClientReq, conn *jsonrpc2.Conn, tx *types.NewTransactionNotification) error {
	result := filterAndIncludeTx(clientReq, tx, h.remoteAddress, h.connectionAccount.AccountID)
	if result == nil {
		return nil
	}
	response := EthSubscribeTxResponse{
		Subscription: subscriptionID,
		Result:       tx.GetHash(),
	}

	err := conn.Notify(ctx, string(jsonrpc.RPCEthSubscribe), response)
	if err != nil {
		h.log.Errorf("error notify to subscriptionID %v: %v", subscriptionID, err.Error())
		return err
	}

	return nil
}

// sendEthSubscribeNotification builds a response according to client request and notifies the client
func (h *handlerObj) sendEthSubscribeNotification(ctx context.Context, subscriptionID string, conn *jsonrpc2.Conn, payload interface{}) error {
	response := EthSubscribeFeedResponse{
		Subscription: subscriptionID,
		Result:       payload,
	}

	err := conn.Notify(ctx, string(jsonrpc.RPCEthSubscribe), response)
	if err != nil {
		h.log.Errorf("error notify to subscriptionID: %v, %v ", subscriptionID, err.Error())
		return err
	}

	return nil
}
