package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	bxgateway "github.com/bloXroute-Labs/gateway/v2"
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
func (s *HTTPServer) Start() {
	if s.server == nil {
		log.Fatalf("failed to start HTTP RPC server, server is not initialized")
	}
	log.Infof("starting HTTP RPC server at: %v", s.server.Addr)
	s.server.Handler = s.setupHandlers()
	err := s.server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to start HTTP RPC server: %v", err)
	}
}

// Stop shutdown http server
func (s HTTPServer) Stop() {
	if s.server == nil {
		log.Warnf("stopping http server that was not initialized")
		return
	}

	err := s.server.Shutdown(context.Background())
	if err != nil {
		log.Fatalf("failed to stop http server: %v", err)
	}

}

func (s *HTTPServer) setupHandlers() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.httpRPCHandler)

	return mux
}

func (s HTTPServer) httpRPCHandler(w http.ResponseWriter, r *http.Request) {
	rpcRequest := jsonrpc2.Request{}
	err := json.NewDecoder(r.Body).Decode(&rpcRequest)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, err)
		return
	}

	if rpcRequest.Params == nil {
		err := errors.New("failed to unmarshal request.Params for mevBundle from mev-builder, error: EOF")
		writeErrorJSON(w, http.StatusBadRequest, err)
		return
	}

	switch jsonrpc.RPCRequestType(rpcRequest.Method) {
	case jsonrpc.RPCEthSendBundle, jsonrpc.RPCEthSendMegaBundle:
		bundlePayload := []jsonrpc.RPCSendBundle{}
		if err := json.Unmarshal(*rpcRequest.Params, &bundlePayload); err != nil {
			log.Errorf("failed to unmarshal mevBundle params: %v", err)
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}

		if len(bundlePayload) != 1 {
			err := errors.New("received invalid number of mevBundle payload, must be 1 element")
			log.Error(err)
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}

		// Flashbots and bloXroute used by default. See below
		if !s.feedManager.accountModel.TierName.IsElite() {
			accError := fmt.Errorf("EnterpriseElite account is required in order to send %s to %s", rpcRequest.Method, bxgateway.BloxrouteBuilderName)
			writeErrorJSON(w, http.StatusInternalServerError, accError)
			log.Error(accError)
			return
		}

		payload := jsonrpc.RPCBundleSubmissionPayload{
			BlockchainNetwork: bxgateway.Mainnet,
			MEVBuilders: map[string]string{
				bxgateway.BloxrouteBuilderName: "",
				bxgateway.FlashbotsBuilderName: "",
			},
			Frontrunning:    bundlePayload[0].Frontrunning,
			Transaction:     bundlePayload[0].Txs,
			BlockNumber:     bundlePayload[0].BlockNumber,
			MinTimestamp:    bundlePayload[0].MinTimestamp,
			MaxTimestamp:    bundlePayload[0].MaxTimestamp,
			RevertingHashes: bundlePayload[0].RevertingTxHashes,
			UUID:            bundlePayload[0].UUID,
		}

		mevBundle, bundleHash, err := mevBundleFromRequest(&payload)
		if err != nil {
			if errors.Is(err, errBlockedTxHashes) {
				var result interface{}
				if payload.UUID != "" {
					result = GatewayBundleResponse{BundleHash: bundleHash}
				}

				writeJSON(w, http.StatusOK, map[string]interface{}{"Result": result})
				return
			}

			convertParamsError := errors.New("failed to parse params for blxr_submit_bundle")
			writeErrorJSON(w, http.StatusBadRequest, convertParamsError)
			return
		}
		mevBundle.SetNetworkNum(s.feedManager.networkNum)

		ws := connections.NewRPCConn(s.feedManager.accountModel.AccountID, r.RemoteAddr, s.feedManager.networkNum, utils.Websocket)

		err = s.feedManager.node.HandleMsg(mevBundle, ws, connections.RunForeground)
		if err != nil {
			err := fmt.Errorf("failed to process mevBundle message: %v", err)
			log.Error(err)
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case jsonrpc.RPCBundleSubmission:
		params := jsonrpc.RPCBundleSubmissionPayload{
			Frontrunning: true,
		}
		if err := json.Unmarshal(*rpcRequest.Params, &params); err != nil {
			log.Errorf("failed to unmarshal mevBundle params: %v", err)
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}

		// If MEVBuilders request parameter is empty, only send to default builders.
		if len(params.MEVBuilders) == 0 {
			params.MEVBuilders = map[string]string{
				bxgateway.BloxrouteBuilderName: "",
				bxgateway.FlashbotsBuilderName: "",
			}
		}

		for mevBuilder := range params.MEVBuilders {
			if strings.ToLower(mevBuilder) == bxgateway.BloxrouteBuilderName && !s.feedManager.accountModel.TierName.IsElite() {
				accError := fmt.Errorf("EnterpriseElite account is required in order to send %s to %s", jsonrpc.RPCBundleSubmission, bxgateway.BloxrouteBuilderName)
				writeErrorJSON(w, http.StatusInternalServerError, accError)
				log.Error(accError)
				return
			}
		}

		mevBundle, bundleHash, err := mevBundleFromRequest(&params)
		if err != nil {
			if errors.Is(err, errBlockedTxHashes) {
				var result interface{}
				if params.UUID != "" {
					result = GatewayBundleResponse{BundleHash: bundleHash}
				}

				writeJSON(w, http.StatusOK, map[string]interface{}{"Result": result})
				return
			}

			convertParamsError := errors.New("failed to parse params for blxr_submit_bundle")
			writeErrorJSON(w, http.StatusBadRequest, convertParamsError)
			return
		}

		mevBundle.SetNetworkNum(s.feedManager.networkNum)

		ws := connections.NewRPCConn(s.feedManager.accountModel.AccountID, r.RemoteAddr, s.feedManager.networkNum, utils.Websocket)

		if err := s.feedManager.node.HandleMsg(mevBundle, ws, connections.RunForeground); err != nil {
			err := fmt.Errorf("failed to process mevBundle message: %v", err)
			log.Error(err)
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		err := fmt.Errorf("got unsupported method name: %v", rpcRequest.Method)
		writeErrorJSON(w, http.StatusNotFound, err)
	}
}

func writeErrorJSON(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, resultHTTPCode int, jsonAnswer interface{}) {
	if jsonAnswer == nil {
		w.WriteHeader(resultHTTPCode)
		return
	}

	payload, err := json.Marshal(jsonAnswer)
	if err != nil {
		log.Errorf("error: failed to marshal json to render an error, error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resultHTTPCode)
	_, _ = w.Write(payload)
}
