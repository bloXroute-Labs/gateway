package ws

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/services/account"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/services/validator"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const localhost = "127.0.0.1"

var upgrader = websocket.Upgrader{}

// Server is the websocket server
type Server struct {
	server *http.Server

	certFile              string
	keyFile               string
	cfg                   *config.Bx
	accService            account.Accounter
	sdn                   sdnsdk.SDNHTTP
	chainID               bxtypes.NetworkID
	node                  connections.BxListener
	feedManager           *feed.Manager
	nodeWSManager         blockchain.WSManager
	validatorsManager     *validator.Manager
	log                   *log.Entry
	networkNum            bxtypes.NetworkNum
	stats                 statistics.Stats
	wsConnDelayOnErr      time.Duration // wsConnDelayOnErr amount of time to sleep before closing a bad connection. This is configured by tests to a shorted value
	txFromFieldIncludable bool
}

// NewWSServer creates and returns a new websocket server
func NewWSServer(
	cfg *config.Bx,
	certFile,
	keyFile string,
	sdn sdnsdk.SDNHTTP,
	node connections.BxListener,
	accService account.Accounter,
	feedManager *feed.Manager,
	nodeWSManager blockchain.WSManager,
	validatorsManager *validator.Manager,
	stats statistics.Stats,
	txFromFieldIncludable bool) *Server {

	networkNum := sdn.NetworkNum()
	chainID := bxtypes.NetworkNumToChainID[networkNum]

	s := &Server{
		cfg:                   cfg,
		certFile:              certFile,
		keyFile:               keyFile,
		node:                  node,
		sdn:                   sdn,
		accService:            accService,
		chainID:               chainID,
		feedManager:           feedManager,
		nodeWSManager:         nodeWSManager,
		validatorsManager:     validatorsManager,
		log:                   log.WithField("component", "gatewayWs"),
		networkNum:            networkNum,
		stats:                 stats,
		wsConnDelayOnErr:      10 * time.Second,
		txFromFieldIncludable: txFromFieldIncludable,
	}

	return s
}

// Run starts the websocket server
func (s *Server) Run() error {
	handler := http.NewServeMux()
	handler.HandleFunc("/ws", s.wsHandler)
	handler.HandleFunc("/", s.wsHandler)

	s.server = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: time.Second * 5,
	}
	if s.cfg.WebsocketHost == localhost {
		s.server.Addr = fmt.Sprintf(":%v", s.cfg.WebsocketPort)
	} else {
		s.server.Addr = fmt.Sprintf("%v:%v", s.cfg.WebsocketHost, s.cfg.WebsocketPort)
	}

	if s.cfg.WebsocketTLSEnabled {
		s.server.TLSConfig = &tls.Config{
			ClientAuth: tls.RequestClientCert,
			MinVersion: tls.VersionTLS12,
		}
	}

	s.log.Infof("starting websockets RPC server at: %v", s.server.Addr)

	var err error
	if s.cfg.WebsocketTLSEnabled {
		err = s.server.ListenAndServeTLS(s.certFile, s.keyFile)
	} else {
		err = s.server.ListenAndServe()
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("websockets RPC server failed to start: %v", err)
	}

	s.log.Info("websockets RPC server has been closed")

	return nil
}

// Shutdown shuts down the websocket server
func (s *Server) Shutdown() {
	if s.server == nil {
		log.Warnf("stopping WS server that was not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	s.log.Infof("shutting down websocket server")
	err := s.server.Shutdown(ctx)
	if err != nil {
		s.log.Errorf("encountered error shutting down websocket server %v: %v", s.server.Addr, err)
	}
}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	// if enable client handler - skip authorization
	serverAccountID := s.sdn.AccountModel().AccountID

	var connectionAccountModel sdnmessage.Account
	var err error
	var accountID bxtypes.AccountID
	var secretHash string

	if !s.cfg.EnableBlockchainRPC {
		authHeader := r.Header.Get("Authorization")
		switch {
		case authHeader != "":
			accountID, secretHash, err = sdnsdk.GetAccountIDSecretHashFromHeader(authHeader)
			if err != nil {
				log.Errorf("remoteAddr: %v requestURI: %v - %v.", r.RemoteAddr, r.RequestURI, err.Error())
				s.errorWithDelay(w, r, "failed parsing the authorization header")
				return
			}
		case s.cfg.WebsocketTLSEnabled:
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				accountID, err = utils.GetAccountIDFromBxCertificate(r.TLS.PeerCertificates[0].Extensions)
				if err != nil {
					s.errorWithDelay(w, r, fmt.Errorf("failed to get account_id extension, %w", err).Error())
					return
				}
			}
		default:
			s.errorWithDelay(w, r, fmt.Errorf("missing authorization from method: %v", r.Method).Error())
			return
		}
		connectionAccountModel, err = s.accService.Authorize(accountID, secretHash, true, r.RemoteAddr)
		if err != nil {
			s.errorWithDelay(w, r, err.Error())
			return
		}
	} else {
		connectionAccountModel, err = s.sdn.FetchCustomerAccountModel(serverAccountID)
		if err != nil {
			log.Errorf("failed to get customer account model, account id: %v, remote addr: %v, error: %v",
				serverAccountID, r.RemoteAddr, err)
		}
	}

	log.Debugf("new web-socket connection from %v", r.RemoteAddr)
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("error upgrading HTTP server connection to the WebSocket protocol - %v", err.Error())
		http.Error(w, "error upgrading HTTP server connection to the WebSocket protocol", http.StatusUpgradeRequired)
		time.Sleep(s.wsConnDelayOnErr)
		return
	}

	logger := log.WithFields(log.Fields{
		"component":  "handlerObj",
		"remoteAddr": r.RemoteAddr,
	})

	handler := &handlerObj{
		chainID:                  s.chainID,
		sdn:                      s.sdn,
		node:                     s.node,
		feedManager:              s.feedManager,
		nodeWSManager:            s.nodeWSManager,
		validatorsManager:        s.validatorsManager,
		log:                      logger,
		networkNum:               s.networkNum,
		stats:                    s.stats,
		remoteAddress:            r.RemoteAddr,
		connectionAccount:        connectionAccountModel,
		serverAccountID:          serverAccountID,
		ethSubscribeIDToChanMap:  make(map[string]chan bool),
		headers:                  types.SDKMetaFromHeaders(r.Header),
		enableBlockchainRPC:      s.cfg.EnableBlockchainRPC,
		txFromFieldIncludable:    s.txFromFieldIncludable,
		pendingTxsSourceFromNode: s.cfg.PendingTxsSourceFromNode,
	}

	asyncHandler := jsonrpc2.AsyncHandler(handler)
	_ = jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(connection), asyncHandler)
}

func (s *Server) errorWithDelay(w http.ResponseWriter, r *http.Request, msg string) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("error replying with delay to request from RemoteAddr %v: %v", r.RemoteAddr, err.Error())
		time.Sleep(s.wsConnDelayOnErr)
		return
	}
	err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, msg))
	if err != nil {
		log.Errorf("error writing close message: %v", err)
	}

	// sleep for some time to prevent the client (bot) to reissue the same requests in a loop
	time.Sleep(s.wsConnDelayOnErr)

	err = c.Close()
	if err != nil {
		log.Errorf("error closing connection after delay: %v", err)
	}
}

// sendErrorMsg formats and sends an RPC error message back to the client
func sendErrorMsg(ctx context.Context, code jsonrpc.RPCErrorCode, data string, conn *jsonrpc2.Conn, reqID jsonrpc2.ID) {
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
