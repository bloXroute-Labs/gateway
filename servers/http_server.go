package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/utils"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/jsonrpc2"
	"net/http"
)

// HTTPServer handler http calls
type HTTPServer struct {
	server *http.Server

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
		log.Fatalf("failed to start http server, server is not initialized")
	}
	log.Infof("starting http server on addr %v", s.server.Addr)
	s.server.Handler = s.setupHandlers()
	err := s.server.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to start http server: %v", err)
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

	switch RPCRequestType(rpcRequest.Method) {
	case RPCEthSendBundle:
		params, _ := rpcRequest.Params.MarshalJSON()

		sendBundleArgs := []sendBundleArgs{}
		err = json.Unmarshal(params, &sendBundleArgs)
		if err != nil {
			log.Errorf("failed to unmarshal mevBundle params: %v", err)
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}

		if len(sendBundleArgs) != 1 {
			err := errors.New("received invalid number of mevBundle payload, must be 1 element")
			log.Error(err)
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}

		if err := sendBundleArgs[0].validate(); err != nil {
			log.Errorf("mevBundle payload validation failed: %v", err)
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}

		// bxmessage.MEVMinerNames empty because that is the relay responsibility to add the correct MEVBuilderNames
		mevBundle, err := bxmessage.NewMEVBundle(string(RPCEthSendBundle), bxmessage.MEVMinerNames{}, params)
		if err != nil {
			err := fmt.Errorf("failed to create new mevBundle: %v", err)
			log.Error(err)
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}

		ws := connections.NewRPCConn(s.feedManager.accountModel.AccountID, r.RemoteAddr, s.feedManager.networkNum, utils.Websocket)

		mevBundle.SetHash()
		mevBundle.SetNetworkNum(s.feedManager.networkNum)
		err = s.feedManager.node.HandleMsg(&mevBundle, ws, connections.RunForeground)
		if err != nil {
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
