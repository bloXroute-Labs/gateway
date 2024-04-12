package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/sourcegraph/jsonrpc2"
)

// HTTPServer handler http calls
type HTTPServer struct {
	server      *http.Server
	feedManager *FeedManager
}

// NewHTTPServer creates and returns a new websocket server managed by FeedManager
func NewHTTPServer(feedManager *FeedManager, port int) *HTTPServer {
	return &HTTPServer{
		server: &http.Server{
			Addr: fmt.Sprintf(":%v", port),
		},
		feedManager: feedManager,
	}
}

// Start setup handlers and start http server
func (s *HTTPServer) Start() error {
	if s.server == nil {
		log.Fatalf("failed to start HTTP RPC server, server is not initialized")
	}
	log.Infof("starting HTTP RPC server at: %v", s.server.Addr)
	s.server.Handler = s.setupHandlers()

	err := s.server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to start HTTP RPC server: %v", err)
	}

	return nil
}

// Stop shutdown http server
func (s *HTTPServer) Stop() error {
	if s.server == nil {
		log.Warnf("stopping http server that was not initialized")
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

func (s *HTTPServer) setupHandlers() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.httpRPCHandler)

	return mux
}

func (s *HTTPServer) httpRPCHandler(w http.ResponseWriter, r *http.Request) {
	rpcRequest := jsonrpc2.Request{}
	err := json.NewDecoder(r.Body).Decode(&rpcRequest)
	if err != nil {
		writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, err)
		return
	}

	if rpcRequest.Params == nil {
		err := errors.New("failed to unmarshal request.Params for mevBundle from mev-builder, error: EOF")
		writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, err)
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
			Transaction:       bundlePayload[0].Txs,
			BlockNumber:       bundlePayload[0].BlockNumber,
			MinTimestamp:      bundlePayload[0].MinTimestamp,
			MaxTimestamp:      bundlePayload[0].MaxTimestamp,
			RevertingHashes:   bundlePayload[0].RevertingTxHashes,
			UUID:              bundlePayload[0].UUID,
			BundlePrice:       bundlePayload[0].BundlePrice,
			EnforcePayout:     bundlePayload[0].EnforcePayout,
			AvoidMixedBundles: bundlePayload[0].AvoidMixedBundles,
		}

		mevBundle, bundleHash, err := mevBundleFromRequest(&payload, s.feedManager.networkNum)
		var result interface{}
		if payload.UUID == "" {
			result = GatewayBundleResponse{BundleHash: bundleHash}
		}
		if err != nil {
			if errors.Is(err, errBlockedTxHashes) {
				writeJSON(w, rpcRequest.ID, http.StatusOK, result)
				return
			}

			writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, err)
			return
		}
		mevBundle.SetNetworkNum(s.feedManager.networkNum)

		if !s.feedManager.accountModel.TierName.IsElite() {
			log.Tracef("%s rejected for non EnterpriseElite account %v tier %v", mevBundle, s.feedManager.accountModel.AccountID, s.feedManager.accountModel.TierName)
			writeErrorJSON(w, rpcRequest.ID, http.StatusForbidden, errors.New("EnterpriseElite account is required in order to send bundle"))
			return
		}

		maxTxsLen := s.feedManager.accountModel.Bundles.Networks[bxgateway.Mainnet].TxsLenLimit
		if maxTxsLen > 0 && len(mevBundle.Transactions) > maxTxsLen {
			log.Tracef("%s rejected for exceeding txs limit %v", mevBundle, maxTxsLen)
			writeErrorJSON(w, rpcRequest.ID, http.StatusForbidden, fmt.Errorf("txs limit exceeded, max txs limit is %v", maxTxsLen))
			return
		}

		ws := connections.NewRPCConn(s.feedManager.accountModel.AccountID, r.RemoteAddr, s.feedManager.networkNum, utils.Websocket)

		if err = s.feedManager.node.HandleMsg(mevBundle, ws, connections.RunForeground); err != nil {
			// err here is not possible right now but anyway we don't want expose reason of internal error to the client
			log.Errorf("failed to process %s: %v", mevBundle, err)
			writeErrorJSON(w, rpcRequest.ID, http.StatusInternalServerError, nil)
			return
		}

		writeJSON(w, rpcRequest.ID, http.StatusOK, result)
	case jsonrpc.RPCBundleSubmission:
		var params jsonrpc.RPCBundleSubmissionPayload
		if err = json.Unmarshal(*rpcRequest.Params, &params); err != nil {
			writeErrorJSON(w, rpcRequest.ID, http.StatusInternalServerError, fmt.Errorf("failed to unmarshal params for %v request: %v", jsonrpc.RPCBundleSubmission, err))
			return
		}

		mevBundle, bundleHash, err := mevBundleFromRequest(&params, s.feedManager.networkNum)
		var result interface{}
		if params.UUID == "" {
			result = GatewayBundleResponse{BundleHash: bundleHash}
		}
		if err != nil {
			if errors.Is(err, errBlockedTxHashes) {
				writeJSON(w, rpcRequest.ID, http.StatusOK, result)
				return
			}

			convertParamsError := fmt.Errorf("failed to parse params for blxr_submit_bundle: %v", err)
			writeErrorJSON(w, rpcRequest.ID, http.StatusBadRequest, convertParamsError)
			return
		}
		mevBundle.SetNetworkNum(s.feedManager.networkNum)

		if !s.feedManager.accountModel.TierName.IsElite() {
			log.Tracef("%s rejected for non EnterpriseElite account %v tier %v", mevBundle, s.feedManager.accountModel.AccountID, s.feedManager.accountModel.TierName)
			writeErrorJSON(w, rpcRequest.ID, http.StatusForbidden, errors.New("EnterpriseElite account is required in order to send bundle"))
			return
		}

		ws := connections.NewRPCConn(s.feedManager.accountModel.AccountID, r.RemoteAddr, s.feedManager.networkNum, utils.Websocket)

		if err := s.feedManager.node.HandleMsg(mevBundle, ws, connections.RunForeground); err != nil {
			// err here is not possible right now but anyway we don't want expose reason of internal error to the client
			log.Errorf("failed to process %s: %v", mevBundle, err)
			writeErrorJSON(w, rpcRequest.ID, http.StatusInternalServerError, nil)
			return
		}

		writeJSON(w, rpcRequest.ID, http.StatusOK, result)
	default:
		err := fmt.Errorf("got unsupported method name: %v", rpcRequest.Method)
		writeErrorJSON(w, rpcRequest.ID, http.StatusNotFound, err)
	}
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
