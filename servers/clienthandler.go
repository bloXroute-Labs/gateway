package servers

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/websocket"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
	"github.com/zhouzhuojie/conditions"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type clientHandler struct {
	feedManager *FeedManager
	wg          *sync.WaitGroup
}

// TxResponse - response of the jsonrpc params
type TxResponse struct {
	Subscription string   `json:"subscription"`
	Result       TxResult `json:"result"`
}

// TxResult - request of jsonrpc params
type TxResult struct {
	TxHash      *string                     `json:"txHash,omitempty"`
	TxContents  types.BlockchainTransaction `json:"txContents,omitempty"`
	LocalRegion *bool                       `json:"localRegion,omitempty"`
	Time        *string                     `json:"time,omitempty"`
	RawTx       *string                     `json:"rawTx,omitempty"`
}

// BlockResponse - response of the jsonrpc params
type BlockResponse struct {
	Subscription string             `json:"subscription"`
	Result       types.Notification `json:"result"`
}

type handlerObj struct {
	FeedManager       *FeedManager
	ClientReq         *clientReq
	remoteAddress     string
	connectionAccount sdnmessage.Account
}

type clientReq struct {
	includes []string
	feed     types.FeedType
	expr     conditions.Expr
}

type subscriptionRequest struct {
	feed    types.FeedType
	options subscriptionOptions
}

type subscriptionOptions struct {
	Include []string `json:"Include"`
	Filters string   `json:"Filters"`
}

type rpcPingResponse struct {
	Pong string `json:"pong"`
}

type rpcTxResponse struct {
	TxHash string `json:"txHash"`
}

var upgrader = websocket.Upgrader{}

var txContentFields = []string{"tx_contents.nonce", "tx_contents.tx_hash",
	"tx_contents.gas_price", "tx_contents.gas", "tx_contents.to", "tx_contents.value", "tx_contents.input",
	"tx_contents.v", "tx_contents.r", "tx_contents.s", "tx_contents.from", "tx_contents.type", "tx_contents.access_list",
	"tx_contents.chain_id", "tx_contents.max_priority_fee_per_gas", "tx_contents.max_fee_per_gas"}

var validTxParams = append(txContentFields, "tx_contents", "tx_hash", "local_region", "time", "raw_tx")

var validBlockParams = append(txContentFields, "hash", "header", "transactions", "uncles")

var validParams = map[types.FeedType][]string{
	types.NewTxsFeed:     validTxParams,
	types.BDNBlocksFeed:  validBlockParams,
	types.NewBlocksFeed:  validBlockParams,
	types.PendingTxsFeed: validTxParams,
}

var defaultParams = append(txContentFields, "tx_hash", "local_region", "time")

var availableFilters = []string{"gas", "gas_price", "value", "to", "from", "method_id", "type", "chain_id", "max_fee_per_gas", "max_priority_fee_per_gas"}

var availableFeeds = []types.FeedType{types.NewTxsFeed, types.NewBlocksFeed, types.BDNBlocksFeed, types.PendingTxsFeed}

// handleWsClientConnection - when new http connection is made we get here upgarde to ws, and start handling
func handleWSClientConnection(feedManager *FeedManager, w http.ResponseWriter, r *http.Request, accountModel sdnmessage.Account) {
	log.Debugf("New web-socket connection from %v", r.RemoteAddr)
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("error upgrading HTTP server connection to the WebSocket protocol - %v", err.Error())
		return
	}
	handler := jsonrpc2.AsyncHandler(&handlerObj{FeedManager: feedManager, remoteAddress: r.RemoteAddr, connectionAccount: accountModel})
	jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(connection), handler)
	<-jc.DisconnectNotify()
	err = connection.Close()
	if err != nil {
		log.Error(fmt.Errorf("unable to close connection with remote adress %v error %v", connection.RemoteAddr(), err))
	} else {
		log.Debugf("closed web-socket connection with %v", r.RemoteAddr)
	}
}

func (ch *clientHandler) runWSServer() {
	defer ch.wg.Done()
	http.HandleFunc("/ws", func(responseWriter http.ResponseWriter, request *http.Request) {
		tokenString := request.Header.Get("Authorization")
		if tokenString == "" {
			log.Errorf("RemoteAddr: %v RequestURI: %v. missing authorization from method: %v.", request.RemoteAddr, request.RequestURI, request.Method)
			return
		}
		payload, err := base64.StdEncoding.DecodeString(tokenString)
		if err != nil {
			log.Errorf("RemoteAddr: %v RequestURI: %v. DecodeString error: %v. invalid tokenString: %v.", request.RemoteAddr, request.RequestURI, err.Error(), tokenString)
			return
		}
		accountIDAndHash := strings.SplitN(string(payload), ":", 2)
		if len(accountIDAndHash) == 1 {
			log.Errorf("RemoteAddr: %v RequestURI: %v. invalid tokenString- %v palyoad: %v.", request.RemoteAddr, request.RequestURI, tokenString, payload)
			return
		}
		connectionAccountID := types.AccountID(accountIDAndHash[0])
		connectionSecretHash := accountIDAndHash[1]
		customerAccountModel := sdnmessage.Account{}
		serverAccountID := ch.feedManager.accountModel.AccountID
		// if gateway received request from a customer with a different account id, it should verify it with the SDN.
		//if the gateway does not have permission to verify account id (which mostly happen with external gateways),
		//SDN will return StatusUnauthorized and fail this connection. if SDN return any other error -
		//assuming the issue is with the SDN and set default enterprise account for the customer. in order to send request to the gateway,
		//customer must be enterprise / elite account
		if connectionAccountID != serverAccountID {
			customerAccountModel, err = ch.feedManager.getCustomerAccountModel(connectionAccountID)
			if err != nil {
				if strings.Contains(strconv.FormatInt(http.StatusUnauthorized, 10), err.Error()) {
					log.Errorf("Account %v is not authorized to get other account %v information", serverAccountID, connectionAccountID)
					return
				}
				log.Errorf("Failed to get customer account model, account id: %v, error: %v", connectionAccountID, err)
				customerAccountModel = sdnmessage.DefaultEnterpriseAccount
				customerAccountModel.AccountID = connectionAccountID
				customerAccountModel.SecretHash = connectionSecretHash
			}
			if customerAccountModel.TierName != sdnmessage.ATierEnterprise && customerAccountModel.TierName != sdnmessage.ATierElite {
				log.Errorf("Customer account %v must be enterprise / enterprise elite but it is %v", connectionAccountID, customerAccountModel.TierName)
				return
			}
		} else {
			customerAccountModel = ch.feedManager.accountModel
		}
		if customerAccountModel.SecretHash != connectionSecretHash {
			log.Errorf("Account %v sent a different secret hash: %v then set in the account model: %v", connectionAccountID, connectionSecretHash, customerAccountModel.SecretHash)
			return
		}
		handleWSClientConnection(ch.feedManager, responseWriter, request, customerAccountModel)
	})
	panic(fmt.Errorf("could not listen on %v. error: %v", ch.feedManager.addr, http.ListenAndServe(ch.feedManager.addr, nil)))
}

// SendErrorMsg formats and sends an RPC error message back to the client
func SendErrorMsg(ctx context.Context, code RPCErrorCode, data string, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	rpcError := &jsonrpc2.Error{
		Code:    int64(code),
		Message: errorMsg[code],
	}
	rpcError.SetError(data)
	err := conn.ReplyWithError(ctx, req.ID, rpcError)
	if err != nil {
		log.Errorf("could not respond to client with error message: %v", err)
	}
}

func reply(ctx context.Context, conn *jsonrpc2.Conn, ID jsonrpc2.ID, result interface{}) error {
	if err := conn.Reply(ctx, ID, result); err != nil {
		return err
	}
	return nil
}

func (h *handlerObj) validateFeed(feedName types.FeedType, feedStreaming sdnmessage.BDNFeedService, includes []string, filters []string) error {
	expireDateTime, _ := time.Parse(bxgateway.TimeDateLayoutISO, feedStreaming.ExpireDate)
	if time.Now().UTC().After(expireDateTime) {
		return fmt.Errorf("%v is not allowed or date has been expired", feedName)
	}
	if feedStreaming.Feed.AllowFiltering && utils.Exists("all", feedStreaming.Feed.AvailableFields) {
		return nil
	}
	for _, include := range includes {
		if !utils.Exists(include, feedStreaming.Feed.AvailableFields) {
			return fmt.Errorf("including %v: %v is not allowed", feedName, include)
		}
	}
	if !feedStreaming.Feed.AllowFiltering && len(filters) > 0 {
		return fmt.Errorf("filtering in %v is not allowed", feedName)
	}
	return nil
}

// Handle - handling client request
func (h *handlerObj) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	start := time.Now()
	defer func() {
		log.Debugf("websocket handling for %v ended. Duration %v", RPCRequestType(req.Method), time.Now().Sub(start))
	}()
	switch RPCRequestType(req.Method) {
	case RPCSubscribe:
		request, err := h.createClientReq(req)
		if err != nil {
			SendErrorMsg(ctx, InvalidParams, err.Error(), conn, req)
			return
		}
		feedName := request.feed
		subscriptionID, feedChan, errSubscribe := h.FeedManager.Subscribe(request.feed)
		if errSubscribe != nil {
			SendErrorMsg(ctx, InvalidParams, errSubscribe.Error(), conn, req)
			return
		}

		defer h.FeedManager.Unsubscribe(*subscriptionID)
		if err = reply(ctx, conn, req.ID, subscriptionID); err != nil {
			log.Errorf("error reply to subscriptionID: %v : %v ", subscriptionID, err)
			SendErrorMsg(ctx, InternalError, string(rune(websocket.CloseMessage)), conn, req)
			return
		}

		for {
			select {
			case notification, ok := <-(*feedChan):
				if !ok {
					if h.FeedManager.subscriptionExists(*subscriptionID) {
						SendErrorMsg(ctx, InternalError, string(rune(websocket.CloseMessage)), conn, req)
					}
					return
				}
				switch feedName {
				case types.NewTxsFeed:
					tx := (*notification).(*types.NewTransactionNotification)
					if h.sendTx(ctx, subscriptionID, request, conn, tx) != nil {
						return
					}
				case types.PendingTxsFeed:
					tx := (*notification).(*types.PendingTransactionNotification)
					if h.sendTx(ctx, subscriptionID, request, conn, &tx.NewTransactionNotification) != nil {
						return
					}
				case types.BDNBlocksFeed, types.NewBlocksFeed:
					block := (*notification).(*types.BlockNotification)
					if h.sendBlock(ctx, subscriptionID, request, conn, block) != nil {
						return
					}
				}
			}
		}
	case RPCUnsubscribe:
		var params []string
		_ = json.Unmarshal(*req.Params, &params)
		uid, _ := uuid.FromString(params[0])
		if err := h.FeedManager.Unsubscribe(uid); err != nil {
			log.Infof("subscription id %v was not found", uid)
		}
	case RPCTx:
		if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
			err := fmt.Errorf("blxr_tx is not allowed when account authentication is different from the node account")
			log.Errorf("%v. account auth: %v, node account: %v ", err, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
			SendErrorMsg(ctx, InvalidRequest, err.Error(), conn, req)
			return
		}
		var params struct {
			Transaction string `json:"transaction"`
		}
		err := json.Unmarshal(*req.Params, &params)
		if err != nil {
			log.Errorf("unmarshal req.Params error - %v", err.Error())
			SendErrorMsg(ctx, InvalidParams, err.Error(), conn, req)
			return
		}

		txBytes, err := hex.DecodeString(params.Transaction)
		if err != nil {
			log.Errorf("invalid hex string: %v", err)
			SendErrorMsg(ctx, InvalidParams, err.Error(), conn, req)
			return
		}

		// Ethereum transactions encoded for RPC interfaces is slightly different from the RLP encoded format, so much decoded + reencode the transaction for consistency.
		// Specifically, note `UnmarshalBinary` should be used for RPC interfaces, and rlp.DecodeBytes should be used for the wire protocol.
		var ethTx ethtypes.Transaction
		err = ethTx.UnmarshalBinary(txBytes)
		if err != nil {
			log.Errorf("could not decode Ethereum transaction: %v", err)
			SendErrorMsg(ctx, InvalidParams, err.Error(), conn, req)
			return
		}
		txContent, _ := rlp.EncodeToBytes(&ethTx)

		var hash types.SHA256Hash
		copy(hash[:], ethTx.Hash().Bytes())

		tx := bxmessage.NewTx(hash, txContent, h.FeedManager.networkNum, types.TFPaidTx|types.TFLocalRegion|types.TFDeliverToNode, h.connectionAccount.AccountID)
		ws := connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.FeedManager.networkNum)
		// call the Handler. Don't invoke in a go routine
		_ = h.FeedManager.node.HandleMsg(tx, ws, connections.RunForeground)

		response := rpcTxResponse{
			TxHash: tx.Hash().String(),
		}
		if err = reply(ctx, conn, req.ID, response); err != nil {
			log.Errorf("%v reply error - %v", RPCTx, err)
			return
		}
		log.Infof("blxr_tx: Hash - 0x%v", response.TxHash)
	case RPCPing:
		response := rpcPingResponse{
			Pong: time.Now().UTC().Format(bxgateway.MicroSecTimeFormat),
		}
		if err := reply(ctx, conn, req.ID, response); err != nil {
			log.Errorf("%v reply error - %v", RPCPing, err)
		}
	default:
		err := fmt.Errorf("got unsupported method name: %v", req.Method)
		SendErrorMsg(ctx, MethodNotFound, err.Error(), conn, req)
		return
	}
}

//sendBlock - build a response according to client request and notify client
func (h *handlerObj) sendBlock(ctx context.Context, subscriptionID *uuid.UUID, clientReq *clientReq, conn *jsonrpc2.Conn, block *types.BlockNotification) error {
	response := BlockResponse{
		Subscription: subscriptionID.String(),
	}
	blockContent := block.WithFields(clientReq.includes)
	response.Result = blockContent
	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		log.Errorf("error reply to subscriptionID: %v : %v ", subscriptionID, err.Error())
		return err
	}
	return nil
}

//sendTx - build a response according to client request and notify client
func (h *handlerObj) sendTx(ctx context.Context, subscriptionID *uuid.UUID, clientReq *clientReq, conn *jsonrpc2.Conn, tx *types.NewTransactionNotification) error {
	hasTxContent := false

	response := TxResponse{
		Subscription: subscriptionID.String(),
		Result:       TxResult{},
	}

	if clientReq.expr != nil {
		txFilters := tx.Filters(clientReq.expr.Args())
		if txFilters == nil {
			return nil
		}
		//Evaluate if we should send the tx
		shouldSend, err := conditions.Evaluate(clientReq.expr, txFilters)
		if err != nil {
			log.Errorf("error evaluate Filters. feed: %v. method: %v. Filters: %v. remote adress: %v. account id: %v error - %v tx: %v.",
				clientReq.feed, clientReq.includes[0], clientReq.expr.String(), h.remoteAddress, h.connectionAccount.AccountID, err.Error(), txFilters)
			return nil
		}
		if !shouldSend {
			return nil
		}
	}

	for _, param := range clientReq.includes {
		if strings.Contains(param, "tx_contents") {
			hasTxContent = true
		}
		switch param {
		case "tx_hash":
			txHash := tx.GetHash()
			response.Result.TxHash = &txHash
		case "time":
			timeNow := time.Now().Format(bxgateway.MicroSecTimeFormat)
			response.Result.Time = &timeNow
		case "local_region":
			localRegion := tx.LocalRegion()
			response.Result.LocalRegion = &localRegion
		case "raw_tx":
			rawTx := hexutil.Encode(tx.RawTx())
			response.Result.RawTx = &rawTx
		}
	}
	if hasTxContent {
		txContent := tx.WithFields(clientReq.includes)
		if txContent.(*types.NewTransactionNotification).BlockchainTransaction == nil {
			return nil
		}
		response.Result.TxContents = txContent.(*types.NewTransactionNotification).BlockchainTransaction
	}

	err := conn.Notify(ctx, "subscribe", response)
	if err != nil {
		log.Errorf("error reply to subscriptionID: %v : %v ", subscriptionID, err.Error())
		return err
	}

	return nil
}

func (h *handlerObj) createClientReq(req *jsonrpc2.Request) (*clientReq, error) {
	request := subscriptionRequest{}
	var rpcParams []json.RawMessage
	err := json.Unmarshal(*req.Params, &rpcParams)
	if err != nil {
		return nil, err
	}
	if len(rpcParams) < 2 {
		log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote adress: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("number of params must be at least length 2. requested params: %s", *req.Params)
	}
	err = json.Unmarshal(rpcParams[0], &request.feed)
	if err != nil {
		return nil, err
	}
	if !types.Exists(request.feed, availableFeeds) {
		log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote adress: %v account id: %v",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("got unsupported feed name %v. possible feeds are %v", request.feed, availableFeeds)
	}
	err = json.Unmarshal(rpcParams[1], &request.options)
	if err != nil {
		return nil, err
	}
	if request.options.Include == nil {
		log.Debugf("invalid param from request id: %v. method: %v. params: %s. remote adress: %v account id: %v.",
			req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID)
		return nil, fmt.Errorf("got unsupported params %v", string(rpcParams[1]))
	}
	var requestedFields []string
	if len(request.options.Include) == 0 {
		switch request.feed {
		case types.BDNBlocksFeed, types.NewBlocksFeed:
			requestedFields = validBlockParams
		case types.NewTxsFeed:
			requestedFields = defaultParams
		case types.PendingTxsFeed:
			requestedFields = defaultParams
		}
	}
	for _, param := range request.options.Include {
		switch request.feed {
		case types.BDNBlocksFeed:
			if !utils.Exists(param, validParams[types.BDNBlocksFeed]) {
				return nil, fmt.Errorf("got unsupported params %v", param)
			}
		case types.NewTxsFeed:
			if !utils.Exists(param, validTxParams) {
				return nil, fmt.Errorf("got unsupported params %v", param)
			}
		case types.PendingTxsFeed:
			if !utils.Exists(param, validTxParams) {
				return nil, fmt.Errorf("got unsupported params %v", param)
			}
		}
		if param == "tx_contents" {
			requestedFields = append(requestedFields, txContentFields...)
		}
		requestedFields = append(requestedFields, param)
	}
	request.options.Include = requestedFields

	var expr conditions.Expr
	if request.options.Filters != "" {
		// Parse the condition language and get expression
		p := conditions.NewParser(strings.NewReader(strings.ToLower(strings.Replace(request.options.Filters, "'", "\"", -1))))
		expr, err = p.Parse()
		if err != nil {
			log.Debugf("error parsing Filters from request id: %v. method: %v. params: %s. remote adress: %v account id: %v error - %v",
				req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err.Error())
			return nil, fmt.Errorf("error parsing Filters %v", err.Error())

		}
		err = h.evaluateFilters(expr)
		if err != nil {
			log.Debugf("error evalued Filters from request id: %v. method: %v. params: %s. remote adress: %v account id: %v error - %v",
				req.ID, req.Method, *req.Params, h.remoteAddress, h.connectionAccount.AccountID, err.Error())
			return nil, fmt.Errorf("error evaluat Filters- %v", err.Error())
		}
		if expr != nil {
			log.Infof("GetTxContentAndFilters string - %s, GetTxContentAndFilters args - %s", expr, expr.Args())
		}
	}

	// check if valid feed
	var filters []string
	if expr != nil {
		filters = expr.Args()
	}

	feedStreaming := sdnmessage.BDNFeedService{}
	switch request.feed {
	case types.NewTxsFeed:
		feedStreaming = h.connectionAccount.NewTransactionStreaming
	case types.PendingTxsFeed:
		feedStreaming = h.connectionAccount.PendingTransactionStreaming
	case types.BDNBlocksFeed, types.NewBlocksFeed:
		feedStreaming = h.connectionAccount.NewBlockStreaming
	}
	err = h.validateFeed(request.feed, feedStreaming, request.options.Include, filters)
	if err != nil {
		return nil, err
	}

	clientRequest := &clientReq{}
	clientRequest.includes = request.options.Include
	clientRequest.feed = request.feed
	clientRequest.expr = expr
	return clientRequest, nil
}

// filtersHasEmptyValue - checks if some filter has empty value like "Filters":"({to})"
func (h *handlerObj) filtersHasEmptyValue(rawFilters string) error {
	rex := regexp.MustCompile(`\(([^)]+)\)`)
	out := rex.FindAllStringSubmatch(rawFilters, -1)
	for _, i := range out {
		for _, filter := range availableFilters {
			if i[1] == filter || filter == rawFilters {
				return fmt.Errorf("%v", i[1])
			}
		}
	}
	return nil
}

// evaluateFilters - evaluating if the Filters provided by the user are ok
func (h *handlerObj) evaluateFilters(expr conditions.Expr) error {
	isEmptyValue := h.filtersHasEmptyValue(expr.String())
	if isEmptyValue != nil {
		return isEmptyValue
	}
	//Evaluate if we should send the tx
	_, err := conditions.Evaluate(expr, types.EmptyFilteredTransactionMap)
	return err
}
