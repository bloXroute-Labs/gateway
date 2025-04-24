package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sourcegraph/jsonrpc2"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
)

var errSubmitBundleInvalidPayload = errors.New("params is missing in the request")

// Server handler http calls
type Server struct {
	server      *http.Server
	sdn         sdnsdk.SDNHTTP
	node        connections.BxListener
	feedManager *feed.Manager
	port        int
}

// NewServer creates and returns a new websocket server managed by feedManager
func NewServer(node connections.BxListener, feedManager *feed.Manager, port int, sdn sdnsdk.SDNHTTP) *Server {
	return &Server{
		port:        port,
		node:        node,
		feedManager: feedManager,
		sdn:         sdn,
	}
}

// Start setup handlers and start http server
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%v", s.port),
		ReadHeaderTimeout: time.Second * 5,
	}

	log.Infof("starting HTTP RPC server at: %v", s.server.Addr)
	s.server.Handler = s.setupHandlers()

	err := s.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start HTTP RPC server: %v", err)
	}

	return nil
}

// Shutdown stops the HTTP server
func (s *Server) Shutdown() {
	if s.server == nil {
		log.Warnf("stopping http server that was not initialized")
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(shutdownCtx)
	if err != nil {
		log.Errorf("failed to shutdown http server: %v", err)
	}
}

func (s *Server) setupHandlers() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.httpRPCHandler)

	return mux
}

func (s *Server) httpRPCHandler(w http.ResponseWriter, r *http.Request) {
	rpcRequest := jsonrpc2.Request{}
	err := json.NewDecoder(r.Body).Decode(&rpcRequest)
	if err != nil {
		writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, err)
		return
	}

	if rpcRequest.Params == nil {
		writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, errSubmitBundleInvalidPayload)
		return
	}

	switch jsonrpc.RPCRequestType(rpcRequest.Method) {
	case jsonrpc.RPCEthSendBundle:
		var bundlePayload []jsonrpc.RPCSendBundle
		if err = json.Unmarshal(*rpcRequest.Params, &bundlePayload); err != nil {
			writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, fmt.Errorf("failed to unmarshal mev bundle params: %v", err))
			return
		}

		if len(bundlePayload) != 1 {
			writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, fmt.Errorf("received invalid number of mev bundle payload: expected 1, got %d", len(bundlePayload)))
			return
		}

		payload := jsonrpc.RPCBundleSubmissionPayload{
			Transaction:             bundlePayload[0].Txs,
			BlockNumber:             bundlePayload[0].BlockNumber,
			MinTimestamp:            bundlePayload[0].MinTimestamp,
			MaxTimestamp:            bundlePayload[0].MaxTimestamp,
			RevertingHashes:         bundlePayload[0].RevertingTxHashes,
			UUID:                    bundlePayload[0].UUID,
			AvoidMixedBundles:       bundlePayload[0].AvoidMixedBundles,
			IncomingRefundRecipient: bundlePayload[0].RefundRecipient,
			BlocksCount:             bundlePayload[0].BlocksCount,
			DroppingHashes:          bundlePayload[0].DroppingTxHashes,
		}

		s.handleRPCBundleSubmission(w, r, rpcRequest, payload)
	case jsonrpc.RPCBundleSubmission:
		var params jsonrpc.RPCBundleSubmissionPayload
		if err = json.Unmarshal(*rpcRequest.Params, &params); err != nil {
			writeErrorJSON(w, rpcRequest.ID, http.StatusInternalServerError, fmt.Errorf("failed to unmarshal params for %v request: %v", jsonrpc.RPCBundleSubmission, err))
			return
		}

		s.handleRPCBundleSubmission(w, r, rpcRequest, params)
	default:
		err := fmt.Errorf("got unsupported method name: %v", rpcRequest.Method)
		writeErrorJSON(w, rpcRequest.ID, http.StatusNotFound, err)
	}
}

func (s *Server) handleRPCBundleSubmission(w http.ResponseWriter, r *http.Request, rpcRequest jsonrpc2.Request, payload jsonrpc.RPCBundleSubmissionPayload) {
	ws := connections.NewRPCConn(s.sdn.AccountModel().AccountID, r.RemoteAddr, s.sdn.NetworkNum(), bxtypes.Websocket)
	result, errCode, err := handler.HandleMEVBundle(s.node, ws, s.sdn.AccountModel(), &payload)
	if err != nil {
		if errors.Is(err, handler.ErrBundleAccountTierTooLow) {
			writeErrorJSON(w, rpcRequest.ID, http.StatusForbidden, err)
			return
		}
		switch errCode {
		case jsonrpc2.CodeInvalidRequest:
			writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, err)
			return
		default:
			writeErrorJSON(w, rpcRequest.ID, http.StatusInternalServerError, err)
			return
		}
	}

	writeJSON(w, rpcRequest.ID, http.StatusOK, result)
}

func writeErrorJSON(w http.ResponseWriter, id jsonrpc2.ID, statusCode int, err error) {
	jsonrpcErr := jsonrpc2.Error{}
	jsonrpcErr.SetError(err)

	resp := jsonrpc2.Response{
		ID:    id,
		Error: &jsonrpcErr,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("error: failed to marshal json to render an error, error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func writeJSON(w http.ResponseWriter, id jsonrpc2.ID, resultHTTPCode int, jsonAnswer interface{}) {
	resp := &jsonrpc2.Response{
		ID: id,
	}
	if err := resp.SetResult(jsonAnswer); err != nil {
		log.Errorf("error: failed to marshal json to render an error, error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resultHTTPCode)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("error: failed to marshal json to render an error, error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
