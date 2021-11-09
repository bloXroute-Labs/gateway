package servers

import (
	"fmt"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/sourcegraph/jsonrpc2"
	websocketjsonrpc2 "github.com/sourcegraph/jsonrpc2/websocket"
	"net/http"
)

// WebsocketRPCServer represents a simple RPC server
type WebsocketRPCServer struct {
	host    string
	port    int
	handler jsonrpc2.Handler
}

// NewWebsocketRPCServer initializes an RPC server with any RPC handler
func NewWebsocketRPCServer(host string, port int, handler jsonrpc2.Handler) WebsocketRPCServer {
	return WebsocketRPCServer{
		host:    host,
		port:    port,
		handler: handler,
	}
}

// Run starts the RPC Server
func (ws *WebsocketRPCServer) Run() error {
	listenAddr := fmt.Sprintf("%v:%v", ws.host, ws.port)

	handler := http.NewServeMux()
	handler.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Info("got a connection, upgrading to websockets")
		upgrader := websocket.Upgrader{}
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Errorf("error upgrading HTTP server connection to websocket protocol: %v", err)
			return
		}
		handler := jsonrpc2.AsyncHandler(ws.handler)
		jc := jsonrpc2.NewConn(r.Context(), websocketjsonrpc2.NewObjectStream(connection), handler)
		<-jc.DisconnectNotify()

		err = connection.Close()
		if err != nil {
			log.Errorf("error closing connection: %v", err)
		}
	})
	log.Infof("starting rpc server on %v", listenAddr)

	err := http.ListenAndServe(listenAddr, handler)
	if err != nil {
		panic(fmt.Errorf("could not listen on %v. error: %v", listenAddr, err))
	}
	return nil
}
