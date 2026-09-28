package servers

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	log "github.com/bloXroute-Labs/bxcommon-go/v2/logger"
	"github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/servers/grpc"
	"github.com/bloXroute-Labs/gateway/v2/servers/ws"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/account"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// ClientHandler is a struct for gateway client handler object
type ClientHandler struct {
	subscriptionServices services.SubscriptionServices
	feedManager          *feed.Manager
	nodeWSManager        blockchain.WSManager

	log *log.Entry

	// servers
	websocketServer *ws.Server
	gRPCServer      *grpc.Server
}

// NewClientHandler is a constructor for ClientHandler
func NewClientHandler(
	bx grpc.Connector,
	config *config.Bx,
	node connections.BxListener,
	sdn sdnsdk.SDNHTTP,
	accService account.Accounter,
	bridge blockchain.Bridge,
	blockchainPeers []types.NodeEndpoint,
	subscriptionServices services.SubscriptionServices,
	nodeWSManager blockchain.WSManager,
	bdnStats *bxmessage.BdnPerformanceStats,
	timeStarted time.Time,
	gatewayPublicKey string,
	feedManager *feed.Manager,
	stats statistics.Stats,
	txStore services.TxStore,
	txFromFieldIncludable bool,
	certFile,
	keyFile string,
	senderExtractor *services.SenderExtractor,
) *ClientHandler {
	var websocketServer *ws.Server
	var gRPCServer *grpc.Server

	if config.WebsocketEnabled || config.WebsocketTLSEnabled {
		websocketServer = ws.NewWSServer(config, certFile, keyFile,
			sdn, node, accService, feedManager, nodeWSManager, stats, txFromFieldIncludable, senderExtractor)
	}

	if config.GRPC.Enabled {
		gRPCServer = grpc.NewGRPCServer(config, stats, node, sdn, accService, bridge, blockchainPeers,
			nodeWSManager, bdnStats, timeStarted, gatewayPublicKey, bx, feedManager, txStore, txFromFieldIncludable, senderExtractor,
		)
	}

	return &ClientHandler{
		subscriptionServices: subscriptionServices,
		nodeWSManager:        nodeWSManager,
		feedManager:          feedManager,
		websocketServer:      websocketServer,
		gRPCServer:           gRPCServer,
		log:                  log.WithFields(log.Fields{"component": "gatewayClientHandler"}),
	}
}

// ManageServers manage the ws and grpc connection of the blockchain node
func (ch *ClientHandler) ManageServers(ctx context.Context, activeManagement bool) error {
	if !activeManagement {
		go func() {
			wait := ch.runServers()
			err := wait()
			if err != nil {
				ch.log.Errorf("error running servers: %v", err)
			}
		}()
	} else {
		ch.log.Info("active management of servers started")
		ch.runGRPCServer()
	}

	var wait func() error

	for {
		select {
		case <-ctx.Done():
			return nil
		case syncStatus := <-ch.nodeWSManager.ReceiveNodeSyncStatusUpdate():
			if !activeManagement {
				// consume update
				continue
			}

			switch syncStatus {
			case blockchain.Synced:
				wait = ch.runWSServer()
			case blockchain.Unsynced:
				// the WS server's graceful Shutdown intentionally uses its own timeout context
				ch.shutdownWSServer() //nolint:contextcheck
				if wait != nil {
					err := wait()
					if err != nil {
						ch.log.Errorf("error running ws server: %v", err)
					}
				}
				ch.subscriptionServices.SendSubscriptionResetNotification(make([]types.SubscriptionModel, 0))
			}
		}
	}
}

// runServers starts both the gRPC and websocket servers, returning a function that blocks
// until the websocket server stops. Used when the servers are not actively managed by the
// node sync status; the gRPC server runs detached for the lifetime of the gateway.
func (ch *ClientHandler) runServers() (wait func() error) {
	ch.runGRPCServer()
	return ch.runWSServer()
}

func (ch *ClientHandler) runWSServer() (wait func() error) {
	eg := &errgroup.Group{}

	ch.log.Info("starting ws server")

	if ch.websocketServer != nil {
		eg.Go(func() error {
			err := ch.websocketServer.Run()
			if err != nil {
				log.Errorf("error running ws server, err: %v", err)
				return err
			}
			return nil
		})
	}

	return eg.Wait
}

func (ch *ClientHandler) runGRPCServer() {
	if ch.gRPCServer == nil {
		return
	}

	go func() {
		ch.log.Info("starting grpc server")
		if err := ch.gRPCServer.Run(); err != nil {
			log.Errorf("error running grpc server, err: %v", err)
		}
	}()
}

func (ch *ClientHandler) shutdownServers() {
	log.Info("shutting down servers")

	if ch.websocketServer != nil {
		ch.websocketServer.Shutdown()
	}

	if ch.gRPCServer != nil {
		ch.gRPCServer.Shutdown()
	}

	ch.feedManager.CloseAllClientConnections()
}

func (ch *ClientHandler) shutdownWSServer() {
	ch.log.Info("shutting down ws server")

	if ch.websocketServer != nil {
		ch.websocketServer.Shutdown()
	}

	ch.feedManager.CloseAllClientConnections()
}

// Stop stops the servers
func (ch *ClientHandler) Stop() error {
	ch.shutdownServers()

	return ch.feedManager.Close()
}
