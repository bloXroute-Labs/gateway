package servers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
	"github.com/zhouzhuojie/conditions"
)

const localhost = "127.0.0.1"

// ErrWSConnDelay amount of time to sleep before closing a bad connection. This is configured by tests to a shorted value
var ErrWSConnDelay = 10 * time.Second

var upgrader = websocket.Upgrader{}

// ClientHandler is a struct for gateway client handler object
type ClientHandler struct {
	feedManager              *FeedManager
	websocketServer          *http.Server
	httpServer               *HTTPServer
	getQuotaUsage            func(accountID string) (*connections.QuotaResponseBody, error)
	enableBlockchainRPC      bool
	pendingTxsSourceFromNode *bool
	log                      *log.Entry
	authorize                func(accountID types.AccountID, secretHash string, allowAccessToInternalGateway bool) (sdnmessage.Account, error)
	txFromFieldIncludable    bool
}

// NewClientHandler is a constructor for ClientHandler
func NewClientHandler(feedManager *FeedManager, websocketServer *http.Server, httpServer *HTTPServer, enableBlockchainRPC bool, getQuotaUsage func(accountID string) (*connections.QuotaResponseBody, error), log *log.Entry, pendingTxsSourceFromNode *bool, authorize func(accountID types.AccountID, secretHash string, allowAccessToInternalGateway bool) (sdnmessage.Account, error), txFromFieldIncludable bool) *ClientHandler {
	return &ClientHandler{
		feedManager:              feedManager,
		websocketServer:          websocketServer,
		httpServer:               httpServer,
		getQuotaUsage:            getQuotaUsage,
		enableBlockchainRPC:      enableBlockchainRPC,
		pendingTxsSourceFromNode: pendingTxsSourceFromNode,
		authorize:                authorize,
		log:                      log,
		txFromFieldIncludable:    txFromFieldIncludable,
	}
}

// ManageWSServer manage the ws connection of the blockchain node
func (ch *ClientHandler) ManageWSServer(ctx context.Context, activeManagement bool) error {
	if !activeManagement {
		go ch.runWSServer()
	}

	for {
		select {
		case <-ctx.Done():
			ch.shutdownWSServer()
			return nil
		case syncStatus := <-ch.feedManager.nodeWSManager.ReceiveNodeSyncStatusUpdate():
			if !activeManagement {
				// consume update
				continue
			}

			switch syncStatus {
			case blockchain.Synced:
				go ch.runWSServer()
			case blockchain.Unsynced:
				ch.shutdownWSServer()
				ch.feedManager.subscriptionServices.SendSubscriptionResetNotification(make([]sdnmessage.SubscriptionModel, 0))
			}
		}
	}
}

// ManageHTTPServer runs http server for the gateway client handler
func (ch *ClientHandler) ManageHTTPServer() error {
	return ch.httpServer.Start()
}

// Stop stops the servers
func (ch *ClientHandler) Stop() error {
	ch.shutdownWSServer()
	return ch.httpServer.Stop()
}

func (ch *ClientHandler) runWSServer() error {
	ch.websocketServer = newWSServer(ch.feedManager, ch.getQuotaUsage, ch.enableBlockchainRPC, ch.pendingTxsSourceFromNode, ch.authorize, ch.txFromFieldIncludable)
	ch.log.Infof("starting websockets RPC server at: %v", ch.websocketServer.Addr)
	var err error
	if ch.feedManager.cfg.WebsocketTLSEnabled {
		ch.websocketServer.TLSConfig = &tls.Config{
			ClientAuth: tls.RequestClientCert,
		}
		err = ch.websocketServer.ListenAndServeTLS(ch.feedManager.certFile, ch.feedManager.keyFile)
	} else {
		err = ch.websocketServer.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("websockets RPC server failed to start: %v", err)
	}

	ch.log.Info("websockets RPC server has been closed")

	return nil
}

func (ch *ClientHandler) shutdownWSServer() {
	ch.log.Infof("shutting down websocket server")
	ch.feedManager.CloseAllClientConnections()
	err := ch.websocketServer.Shutdown(ch.feedManager.context)
	if err != nil {
		ch.log.Errorf("encountered error shutting down websocket server %v: %v", ch.feedManager.cfg.WebsocketPort, err)
	}
}

// newWSServer creates and returns a new websocket server managed by FeedManager
func newWSServer(feedManager *FeedManager, getQuotaUsage func(accountID string) (*connections.QuotaResponseBody, error), enableBlockchainRPC bool, pendingTxsSourceFromNode *bool, authorize func(accountID types.AccountID, secretHash string, allowAccessToInternalGateway bool) (sdnmessage.Account, error), txFromFieldIncludable bool) *http.Server {
	handler := http.NewServeMux()
	wsHandler := func(responseWriter http.ResponseWriter, request *http.Request) {
		// if enable client handler - skip authorization
		serverAccountID := feedManager.accountModel.AccountID
		connectionAccountModel := sdnmessage.Account{}
		var err error
		var accountID types.AccountID
		var secretHash string
		if !enableBlockchainRPC {
			authHeader := request.Header.Get("Authorization")
			switch {
			case authHeader != "":
				accountID, secretHash, err = utils.GetAccountIDSecretHashFromHeader(authHeader)
				if err != nil {
					log.Errorf("remoteAddr: %v requestURI: %v - %v.", request.RemoteAddr, request.RequestURI, err.Error())
					errorWithDelay(responseWriter, request, "failed parsing the authorization header")
					return
				}
			case feedManager.cfg.WebsocketTLSEnabled:
				if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
					accountID, err = utils.GetAccountIDFromBxCertificate(request.TLS.PeerCertificates[0].Extensions)
					if err != nil {
						errorWithDelay(responseWriter, request, fmt.Errorf("failed to get account_id extension, %w", err).Error())
						return
					}
				}
			default:
				errorWithDelay(responseWriter, request, fmt.Errorf("missing authorization from method: %v", request.Method).Error())
				return
			}
			connectionAccountModel, err = authorize(accountID, secretHash, true)
			if err != nil {
				errorWithDelay(responseWriter, request, err.Error())
				return
			}
		} else {
			connectionAccountModel, err = feedManager.getCustomerAccountModel(serverAccountID)
			if err != nil {
				log.Errorf("failed to get customer account model, account id: %v, remote addr: %v, error: %v",
					serverAccountID, request.RemoteAddr, err)
			}
		}
		handleWSClientConnection(feedManager, responseWriter, request, connectionAccountModel, getQuotaUsage, enableBlockchainRPC, pendingTxsSourceFromNode, txFromFieldIncludable)
	}

	handler.HandleFunc("/ws", wsHandler)
	handler.HandleFunc("/", wsHandler)

	server := http.Server{
		Handler: handler,
	}
	if feedManager.cfg.WebsocketHost == localhost {
		server.Addr = fmt.Sprintf(":%v", feedManager.cfg.WebsocketPort)
	} else {
		server.Addr = fmt.Sprintf("%v:%v", feedManager.cfg.WebsocketHost, feedManager.cfg.WebsocketPort)
	}
	return &server
}

// handleWsClientConnection - when new http connection is made we get here upgrade to ws, and start handling
func handleWSClientConnection(feedManager *FeedManager, w http.ResponseWriter, r *http.Request, accountModel sdnmessage.Account, getQuotaUsage func(accountID string) (*connections.QuotaResponseBody, error), enableBlockchainRPC bool, pendingTxsSourceFromNode *bool, txFromFieldIncludable bool) {
	log.Debugf("new web-socket connection from %v", r.RemoteAddr)
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("error upgrading HTTP server connection to the WebSocket protocol - %v", err.Error())
		http.Error(w, "error upgrading HTTP server connection to the WebSocket protocol", http.StatusUpgradeRequired)
		time.Sleep(ErrWSConnDelay)
		return
	}

	logger := log.WithFields(log.Fields{
		"component":  "handlerObj",
		"remoteAddr": r.RemoteAddr,
	})

	handler := &handlerObj{
		FeedManager:              feedManager,
		remoteAddress:            r.RemoteAddr,
		connectionAccount:        accountModel,
		getQuotaUsage:            getQuotaUsage,
		enableBlockchainRPC:      enableBlockchainRPC,
		pendingTxsSourceFromNode: pendingTxsSourceFromNode,
		log:                      logger,
		ethSubscribeIDToChanMap:  make(map[string]chan bool),
		headers:                  types.SDKMetaFromHeaders(r.Header),
		stats:                    feedManager.stats,
		txFromFieldIncludable:    txFromFieldIncludable,
	}

	asyncHandler := jsonrpc2.AsyncHandler(handler)
	_ = jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(connection), asyncHandler)
}

func errorWithDelay(w http.ResponseWriter, r *http.Request, msg string) {
	// sleep for 10 seconds to prevent the client (bot) to reissue the same requests in a loop
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("error replying with delay to request from RemoteAddr %v: %v", r.RemoteAddr, err.Error())
		time.Sleep(ErrWSConnDelay)
		return
	}
	_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, msg))
	time.Sleep(ErrWSConnDelay)
	c.Close()
}

// SendErrorMsg formats and sends an RPC error message back to the client
func SendErrorMsg(ctx context.Context, code jsonrpc.RPCErrorCode, data string, conn *jsonrpc2.Conn, reqID jsonrpc2.ID) {
	rpcError := &jsonrpc2.Error{
		Code:    int64(code),
		Message: jsonrpc.ErrorMsg[code],
	}
	rpcError.SetError(data)
	err := conn.ReplyWithError(ctx, reqID, rpcError)
	if err != nil {
		// TODO: move this to caller and add identifying information
		log.Errorf("could not respond to client with error message: %v", err)
	}
}

func filterAndInclude(clientReq *clientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID types.AccountID) *TxResult {
	if clientReq.expr != nil {
		filters := clientReq.expr.Args()
		txFilters := tx.Filters(filters)

		// should be doone after tx.Filters() to avoid nil pointer dereference
		txType := tx.BlockchainTransaction.(*types.EthTransaction).Type()

		if !isFiltersSupportedByTxType(txType, filters) {
			log.Tracef("skipping [%s] transaction evaluation for feed, configured unsupported filter %s for tx type: %d. feed: %v remote address: %v. account id: %v",
				tx.GetHash(), clientReq.expr, txType, clientReq.feed, remoteAddress, accountID)
			return nil
		}

		// Evaluate if we should send the tx
		shouldSend, err := conditions.Evaluate(clientReq.expr, txFilters)
		if err != nil {
			log.Errorf("error evaluate Filters. feed: %v. filters: %s. remote address: %v. account id: %v error - %v tx: %v",
				clientReq.feed, clientReq.expr, remoteAddress, accountID, err.Error(), txFilters)
			return nil
		}
		if !shouldSend {
			return nil
		}
	}

	hasTxContent := false
	var response TxResult
	for _, param := range clientReq.includes {
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
			if strings.HasPrefix(param, "tx_contents.") {
				hasTxContent = true
			}
		}
	}

	if hasTxContent {
		fields := tx.Fields(clientReq.includes)
		if fields == nil {
			log.Errorf("Got nil from tx.Fields - need to be checked")
			return nil
		}
		response.TxContents = fields
	}
	return &response
}

// validateTxFromExternalSource validate transaction from external source (ws / grpc), return bool indicates if tx is pending reevaluation
func validateTxFromExternalSource(transaction string, txBytes []byte, validatorsOnly bool, gatewayChainID types.NetworkID, nextValidator bool, fallback uint16, nextValidatorMap *orderedmap.OrderedMap, validatorStatusMap *syncmap.SyncMap[string, bool], networkNum types.NetworkNum, accountID types.AccountID, nodeValidationRequested bool, wsManager blockchain.WSManager, source connections.Conn, pendingBSCNextValidatorTxHashToInfo map[string]PendingNextValidatorTxInfo, frontRunningProtection bool) (*bxmessage.Tx, bool, error) {
	// Ethereum's transactions encoding for RPC interfaces is slightly different from the RLP encoded format, so decode + re-encode the transaction for consistency.
	// Specifically, note `UnmarshalBinary` should be used for RPC interfaces, and rlp.DecodeBytes should be used for the wire protocol.
	var ethTx ethtypes.Transaction
	err := ethTx.UnmarshalBinary(txBytes)
	if err != nil {
		// If UnmarshalBinary failed, we will try RLP in case user made mistake
		e := rlp.DecodeBytes(txBytes, &ethTx)
		if e != nil {
			return nil, false, fmt.Errorf("failed to unmarshal tx: %w", err)
		}
		log.Warnf("Ethereum transaction was in RLP format instead of binary," +
			" transaction has been processed anyway, but it'd be best to use the Ethereum binary standard encoding")
	}

	if ethTx.ChainId().Int64() != 0 && gatewayChainID != 0 && types.NetworkID(ethTx.ChainId().Int64()) != gatewayChainID {
		log.Debugf("chainID mismatch for hash %v - tx chainID %v , gateway networkNum %v networkChainID %v", ethTx.Hash().String(), ethTx.ChainId().Int64(), networkNum, gatewayChainID)
		return nil, false, fmt.Errorf("chainID mismatch for hash %v, expect %v got %v, make sure the tx is sent with the right blockchain network", ethTx.Hash().String(), gatewayChainID, ethTx.ChainId().Int64())
	}

	txContent, err := rlp.EncodeToBytes(&ethTx)

	if err != nil {
		return nil, false, err
	}

	var txFlags = types.TFPaidTx | types.TFLocalRegion
	if validatorsOnly {
		txFlags |= types.TFValidatorsOnly
	} else if nextValidator {
		txFlags |= types.TFNextValidator
	} else {
		txFlags |= types.TFDeliverToNode
	}

	if frontRunningProtection {
		txFlags |= types.TFFrontRunningProtection
	}

	var hash types.SHA256Hash
	copy(hash[:], ethTx.Hash().Bytes())

	// should set the account of the sender, not the account of the gateway itself
	tx := bxmessage.NewTx(hash, txContent, networkNum, txFlags, accountID)
	if nextValidator {
		txPendingReevaluation, err := ProcessNextValidatorTx(tx, fallback, nextValidatorMap, validatorStatusMap, networkNum, source, pendingBSCNextValidatorTxHashToInfo)
		if err != nil {
			return nil, false, err
		}
		if txPendingReevaluation {
			return tx, true, nil
		}
	}

	if nodeValidationRequested && !tx.Flags().IsNextValidator() && !tx.Flags().IsValidatorsOnly() {
		syncedWS, ok := wsManager.SyncedProvider()
		if ok {
			_, err := syncedWS.SendTransaction(
				fmt.Sprintf("%v%v", "0x", transaction),
				blockchain.RPCOptions{
					RetryAttempts: 1,
					RetryInterval: 10 * time.Millisecond,
				},
			)
			if err != nil {
				if !strings.Contains(err.Error(), "already known") { // gateway propagates tx to node before doing this check
					errMsg := fmt.Sprintf("tx (%v) failed node validation with error: %v", tx.Hash(), err.Error())
					return nil, false, errors.New(errMsg)
				}
			}
		} else {
			return nil, false, fmt.Errorf("failed to validate tx (%v) via node: no synced WS provider available", tx.Hash())
		}
	}
	return tx, false, nil
}

// ProcessNextValidatorTx - sets next validator wallets if accessible and returns bool indicating if tx is pending reevaluation due to inaccessible first validator for BSC
func ProcessNextValidatorTx(tx *bxmessage.Tx, fallback uint16, nextValidatorMap *orderedmap.OrderedMap, validatorStatusMap *syncmap.SyncMap[string, bool], networkNum types.NetworkNum, source connections.Conn, pendingBSCNextValidatorTxHashToInfo map[string]PendingNextValidatorTxInfo) (bool, error) {
	if networkNum != bxgateway.BSCMainnetNum && networkNum != bxgateway.PolygonMainnetNum {
		return false, errors.New("currently next_validator is only supported on BSC and Polygon networks, please contact bloXroute support")
	}

	if nextValidatorMap == nil {
		log.Errorf("failed to process next validator tx, because next validator map is nil, tx %v", tx.Hash().String())
		return false, errors.New("failed to send next validator tx, please contact bloXroute support")
	}

	tx.SetFallback(fallback)

	// take the latest two blocks from the ordered map for updating txMsg walletID
	n2Validator := nextValidatorMap.Newest()
	if n2Validator == nil {
		return false, errors.New("can't send tx with next_validator because the gateway encountered an issue fetching the epoch block, please try again later or contact bloXroute support")
	}

	if networkNum == bxgateway.BSCMainnetNum {
		n1Validator := n2Validator.Prev()
		n1ValidatorAccessible := false
		n1Wallet := ""
		if n1Validator != nil {
			n1Wallet = n1Validator.Value.(string)
			accessible, exist := validatorStatusMap.Load(n1Wallet)
			if exist {
				n1ValidatorAccessible = accessible
			}
		}

		if n1ValidatorAccessible {
			tx.SetWalletID(0, n1Wallet)
		} else {
			blockIntervalBSC := bxgateway.NetworkToBlockDuration[bxgateway.BSCMainnet]
			if fallback != 0 && fallback < uint16(blockIntervalBSC.Milliseconds()) {
				return false, nil
			}
			pendingBSCNextValidatorTxHashToInfo[tx.Hash().String()] = PendingNextValidatorTxInfo{
				Tx:            tx,
				Fallback:      fallback,
				TimeOfRequest: time.Now(),
				Source:        source,
			}
			return true, nil
		}
	}

	if networkNum == bxgateway.PolygonMainnetNum || networkNum == bxgateway.PolygonMumbaiNum {
		n1Validator := n2Validator.Prev()
		if n1Validator != nil {
			tx.SetWalletID(0, n1Validator.Value.(string))
			tx.SetWalletID(1, n2Validator.Value.(string))
		} else {
			tx.SetWalletID(0, n2Validator.Value.(string))
		}
	}

	return false, nil
}

// HandleMEVBundle handles the submission of a bundle and returns its hash, an error and the equivalent error code that we need to send in the response
func HandleMEVBundle(feedManager *FeedManager, conn connections.Conn, connectionAccount sdnmessage.Account, params *jsonrpc.RPCBundleSubmissionPayload) (*GatewayBundleResponse, int, error) {
	// If MEVBuilders request parameter is empty, only send to default builders.
	if len(params.MEVBuilders) == 0 {
		params.MEVBuilders = map[string]string{
			bxgateway.BloxrouteBuilderName: "",
			bxgateway.FlashbotsBuilderName: "",
		}
	}

	mevBundle, bundleHash, err := mevBundleFromRequest(params, feedManager.networkNum)
	var result *GatewayBundleResponse
	if params.UUID == "" {
		result = &GatewayBundleResponse{BundleHash: bundleHash}
	}
	if err != nil {
		if errors.Is(err, errBlockedTxHashes) {
			return result, 0, nil
		}
		return nil, jsonrpc2.CodeInvalidParams, err
	}
	mevBundle.SetNetworkNum(feedManager.networkNum)

	if !connectionAccount.TierName.IsElite() {
		log.Tracef("%s rejected for non EnterpriseElite account %v tier %v", mevBundle, connectionAccount.AccountID, connectionAccount.TierName)
		return nil, jsonrpc2.CodeInvalidRequest, errors.New("enterprise account is required in order to send bundle")
	}

	maxTxsLen := connectionAccount.Bundles.Networks[bxgateway.NetworkNumToBlockchainNetwork[feedManager.networkNum]].TxsLenLimit
	if maxTxsLen > 0 && len(mevBundle.Transactions) > maxTxsLen {
		log.Tracef("%s rejected for exceeding txs limit %v", mevBundle, maxTxsLen)
		return nil, jsonrpc2.CodeInvalidRequest, fmt.Errorf("txs limit exceeded, max txs allowed: %v", maxTxsLen)
	}

	if err := feedManager.node.HandleMsg(mevBundle, conn, connections.RunForeground); err != nil {
		// err here is not possible right now, but anyway we don't want expose reason of internal error to the client
		log.Errorf("failed to process %s: %v", mevBundle, err)
		return nil, jsonrpc2.CodeInternalError, err
	}

	return result, 0, nil
}
