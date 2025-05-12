package servers

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/servers/grpc"
	http2 "github.com/bloXroute-Labs/gateway/v2/servers/http"
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
	httpServer      *http2.Server
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
) *ClientHandler {

	var websocketServer *ws.Server
	var gRPCServer *grpc.Server

	if config.WebsocketEnabled || config.WebsocketTLSEnabled {
		websocketServer = ws.NewWSServer(config, certFile, keyFile,
			sdn, node, accService, feedManager, nodeWSManager, stats, txFromFieldIncludable)
	}

	if config.GRPC.Enabled {
		gRPCServer = grpc.NewGRPCServer(config, stats, node, sdn, accService,
			bridge, blockchainPeers, nodeWSManager, bdnStats, timeStarted,
			gatewayPublicKey, bx, feedManager, txStore,
		)
	}

	httpServer := http2.NewServer(node, feedManager, config.HTTPPort, sdn)

	return &ClientHandler{
		subscriptionServices: subscriptionServices,
		nodeWSManager:        nodeWSManager,
		feedManager:          feedManager,
		websocketServer:      websocketServer,
		httpServer:           httpServer,
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
				wait = ch.runServers()
			case blockchain.Unsynced:
				ch.shutdownServers()
				// in case the 'unsynced' status comes first, check if the servers were even started
				if wait != nil {
					// wait for the servers to stop
					err := wait()
					if err != nil {
						ch.log.Errorf("error running servers: %v", err)
					}
				}
				ch.subscriptionServices.SendSubscriptionResetNotification(make([]types.SubscriptionModel, 0))
			}
		}
	}
}

func (ch *ClientHandler) runServers() (wait func() error) {
	eg := &errgroup.Group{}

	ch.log.Info("starting servers")

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

	if ch.gRPCServer != nil {
		eg.Go(func() error {
			err := ch.gRPCServer.Run()
			if err != nil {
				log.Errorf("error running grpc server, err: %v", err)
				return err
			}
			return nil
		})
	}
	if ch.httpServer != nil {
		eg.Go(func() error {
			err := ch.httpServer.Start()
			if err != nil {
				log.Errorf("error running http server, err: %v", err)
				return err
			}
			return nil
		})
	}

	return eg.Wait
}

func (ch *ClientHandler) shutdownServers() {
	log.Info("shutting down servers")

	if ch.websocketServer != nil {
		ch.websocketServer.Shutdown()
	}

	if ch.gRPCServer != nil {
		ch.gRPCServer.Shutdown()
	}

	if ch.httpServer != nil {
		ch.httpServer.Shutdown()
	}

	ch.feedManager.CloseAllClientConnections()
}

// Stop stops the servers
func (ch *ClientHandler) Stop() error {
	ch.shutdownServers()
	return nil
}
