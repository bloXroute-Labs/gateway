package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"runtime/debug"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/nodes"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	httputils "github.com/bloXroute-Labs/gateway/v2/utils/http"
)

func main() {
	app := &cli.App{
		Name:  "gateway",
		Usage: "run a GO gateway",
		Flags: []cli.Flag{
			utils.ExternalIPFlag,
			utils.PortFlag,
			utils.SDNURLFlag,
			utils.CACertURLFlag,
			utils.RegistrationCertDirFlag,
			utils.WSFlag,
			utils.WSPortFlag,
			utils.WSHostFlag,
			utils.HTTPPortFlag,
			utils.EnvFlag,
			utils.LogLevelFlag,
			utils.LogFileLevelFlag,
			utils.LogMaxSizeFlag,
			utils.LogMaxAgeFlag,
			utils.LogMaxBackupsFlag,
			utils.TxTraceEnabledFlag,
			utils.TxTraceMaxFileSizeFlag,
			utils.TxTraceMaxBackupFilesFlag,
			utils.AvoidPrioritySendingFlag,
			utils.RelayHostsFlag,
			utils.DataDirFlag,
			utils.GRPCFlag,
			utils.GRPCHostFlag,
			utils.GRPCPortFlag,
			utils.GRPCUserFlag,
			utils.GRPCPasswordFlag,
			utils.BlockchainNetworkFlag,
			utils.EnodesFlag,
			utils.EthWSUriFlag,
			utils.MultiNode,
			utils.BeaconENRFlag,
			utils.BeaconMultiaddrFlag,
			utils.PrysmGRPCFlag,
			utils.BeaconAPIUriFlag,
			utils.BlocksOnlyFlag,
			utils.GensisFilePath,
			utils.AllTransactionsFlag,
			utils.PrivateKeyFlag,
			utils.NodeTypeFlag,
			utils.DisableProfilingFlag,
			utils.FluentDFlag,
			utils.FluentdHostFlag,
			utils.ManageWSServer,
			utils.LogNetworkContentFlag,
			utils.WSTLSFlag,
			utils.MEVBuildersFilePathFlag,
			utils.MEVBundleMethodNameFlag,
			utils.SendBlockConfirmation,
			utils.TerminalTotalDifficulty,
			utils.EnableDynamicPeers,
			utils.ForwardTransactionEndpoint,
			utils.ForwardTransactionMethod,
			utils.TransactionHoldDuration,
			utils.TransactionPassedDueDuration,
			utils.EnableBlockchainRPCMethodSupport,
			utils.DialRatio,
			utils.NumRecommendedPeers,
			utils.NoTxsToBlockchain,
			utils.NoBlocks,
			utils.NoStats,
			utils.EnableBloomFilter,
			utils.TxIncludeSenderInFeed,
			utils.BeaconTrustedPeersFileFlag,
			utils.BeaconPort,
		},
		Action:         runGateway,
		Before:         config.BeforeFunc,
		After:          config.AfterFunc,
		ExitErrHandler: config.ExitErrHandler,
	}

	_ = app.Run(os.Args) //nolint:errcheck
}

func runGateway(c *cli.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic: %v", r)
			log.Errorf(string(debug.Stack()))
		}
	}()
	ctx := utils.ContextWithSignal(c.Context)
	group, gCtx := errgroup.WithContext(ctx)

	var pprofServer *http.Server
	if !c.Bool(utils.DisableProfilingFlag.Name) {
		pprofServer = &http.Server{Addr: "0.0.0.0:6060"}
		group.Go(func() error {
			log.Infof("pprof http server is running on 0.0.0.0:6060 - %v", "http://localhost:6060/debug/pprof")

			err := pprofServer.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("failed to start pprof http server: %v", err)
			}

			return nil
		})
	}

	dataDir := c.String(utils.DataDirFlag.Name)
	ethConfig, gatewayPublicKey, err := network.NewPresetEthConfigFromCLI(c, dataDir)
	if err != nil {
		return err
	}

	blockchainNetwork := c.String(utils.BlockchainNetworkFlag.Name)
	blockchainPeers := ethConfig.StaticPeers.Endpoints()

	bxConfig := c.Context.Value(bxgateway.BxConfigKey).(*config.Bx)
	if bxConfig == nil {
		return fmt.Errorf("bxConfig is not available in context")
	}
	sslCerts, sdn, err := nodes.InitSDN(bxConfig, blockchainPeers, nodes.GeneratePeers(ethConfig.StaticPeers), len(ethConfig.StaticPeers.Enodes()))
	if err != nil {
		return err
	}

	// set node ID for fluentd logging
	log.SetNodeID(string(sdn.NodeID()))

	// create a set of recommended peers
	recommendedPeers := make(map[string]struct{})

	startupBeaconNode := len(ethConfig.StaticPeers.BeaconNodes()) > 0 || c.Int(utils.BeaconPort.Name) != 0
	startupBeaconAPIClients := len(ethConfig.StaticPeers.BeaconAPIEndpoints()) > 0
	startupBlockchainClient := startupBeaconAPIClients || startupBeaconNode || len(ethConfig.StaticPeers.Enodes()) > 0 || bxConfig.EnableDynamicPeers // if beacon node running we need to receive txs also
	startupPrysmClient := len(ethConfig.StaticPeers.PrysmAddrs()) > 0

	var bridge blockchain.Bridge
	if blockchainNetwork == bxgateway.Mainnet || startupBlockchainClient || startupPrysmClient {
		// for ethereum initialize bridge even if startupPrysmClient and startupBlockchainClient are false
		bridge = blockchain.NewBxBridge(eth.Converter{}, startupBeaconNode || startupBeaconAPIClients)
	} else {
		bridge = blockchain.NewNoOpBridge(eth.Converter{})
	}

	if bxConfig.ManageWSServer && !bxConfig.WebsocketEnabled && !bxConfig.WebsocketTLSEnabled {
		return fmt.Errorf("websocket server must be enabled using --ws or --ws-tls if --manage-ws-server is enabled")
	}
	if bxConfig.EnableBlockchainRPC && !bxConfig.WebsocketEnabled && !bxConfig.WebsocketTLSEnabled {
		return fmt.Errorf("websocket server must be enabled using --ws or --ws-tls if --enable-blockchain-rpc is used")
	}
	wsManager := eth.NewEthWSManager(ethConfig.StaticPeers, eth.NewWSProvider, bxgateway.WSProviderTimeout, bxConfig.EnableBlockchainRPC)
	if (bxConfig.WebsocketEnabled || bxConfig.WebsocketTLSEnabled) && !ethConfig.StaticPeers.ValidWSAddr() {
		log.Warn("websocket server enabled but no valid websockets endpoint specified via --eth-ws-uri nor --multi-node: only newTxs and bdnBlocks feeds are available")
	}
	if bxConfig.ManageWSServer && !ethConfig.StaticPeers.ValidWSAddr() {
		return fmt.Errorf("if websocket server management is enabled, a valid websocket address must be provided")
	}
	if bxConfig.EnableBlockchainRPC && !ethConfig.StaticPeers.ValidWSAddr() {
		return fmt.Errorf("if blockchan rpc is enabled, a valid websocket address must be provided")
	}

	var beaconNode *beacon.Node
	if startupBeaconNode {
		var genesisPath string
		localGenesisFile := path.Join(dataDir, "genesis.ssz")
		if c.IsSet(utils.GensisFilePath.Name) {
			localGenesisFile = c.String(utils.GensisFilePath.Name)
			genesisPath = localGenesisFile
		} else {
			genesisPath, err = downloadGenesisFile(gCtx, c.String(utils.BlockchainNetworkFlag.Name), localGenesisFile)
			if err != nil {
				return err
			}
		}
		log.Info("connecting to beacon node using ", genesisPath)

		beaconNode, err = beacon.NewNode(beacon.NodeParams{
			ParentContext:        gCtx,
			NetworkName:          c.String(utils.BlockchainNetworkFlag.Name),
			EthConfig:            ethConfig,
			TrustedPeersFilePath: c.String(utils.BeaconTrustedPeersFileFlag.Name),
			GenesisFilePath:      localGenesisFile,
			Bridge:               bridge,
			Port:                 c.Int(utils.BeaconPort.Name),
			InboundLimit:         int(sdn.AccountModel().InboundNodeConnections.MsgQuota.Limit),
		})
		if err != nil {
			return err
		}

		group.Go(beaconNode.Start)
	}

	beaconAPIClients := make([]*beacon.APIClient, 0)
	if startupBeaconAPIClients {
		apiShredSync := beacon.NewAPISharedSync()
		for _, beaconAPI := range ethConfig.StaticPeers.BeaconAPIEndpoints() {
			client := beacon.NewAPIClient(gCtx, httputils.Client(nil), ethConfig, bridge, beaconAPI.BeaconAPIURI, beaconAPI.Endpoint, apiShredSync)

			group.Go(client.Start)

			beaconAPIClients = append(beaconAPIClients, client)
		}
	}

	var blobsManager *beacon.BlobSidecarCacheManager
	if startupBeaconNode || startupBeaconAPIClients {
		blobsManager = beacon.NewBlobSidecarCacheManager(ethConfig.GenesisTime)
		group.Go(func() error {
			beacon.HandleBDNBeaconMessages(gCtx, bridge, beaconNode, blobsManager)
			return nil
		})
		group.Go(func() error {
			beacon.HandleBDNBlocks(gCtx, bridge, beaconNode, beaconAPIClients, blobsManager)
			return nil
		})
	}

	gateway, err := nodes.NewGateway(
		gCtx,
		bxConfig,
		bridge,
		wsManager,
		blobsManager,
		blockchainPeers,
		ethConfig.StaticPeers,
		recommendedPeers,
		gatewayPublicKey,
		sdn,
		sslCerts,
		len(ethConfig.StaticPeers.Enodes()),
		c.Int(utils.TransactionHoldDuration.Name),
		c.Int(utils.TransactionPassedDueDuration.Name),
		c.Bool(utils.EnableBloomFilter.Name),
		c.Bool(utils.TxIncludeSenderInFeed.Name),
	)
	if err != nil {
		return err
	}

	group.Go(gateway.Run)

	// Required for beacon node and prysm to sync
	ethChain := eth.NewChain(gCtx, ethConfig.IgnoreBlockTimeout)

	var blockchainServer *eth.Server
	if startupBlockchainClient {
		log.Infof("starting blockchain client with config for network ID: %v", ethConfig.Network)

		// TODO: use resolver to get public IP if externalIP flag is omitted
		port, externalIP := c.Int(utils.PortFlag.Name), net.ParseIP(c.String(utils.ExternalIPFlag.Name))

		dynamicPeers := int(sdn.AccountModel().InboundNodeConnections.MsgQuota.Limit)
		if !bxConfig.EnableDynamicPeers {
			dynamicPeers = 0
		}

		dialRatio := c.Int(utils.DialRatio.Name)

		blockchainServer, err = eth.NewServerWithEthLogger(gCtx, port, externalIP, ethConfig, ethChain, bridge, dataDir, wsManager, dynamicPeers, dialRatio, recommendedPeers)
		if err != nil {
			return err
		}

		if err = blockchainServer.AddEthLoggerFileHandler(bxConfig.Config.FileName); err != nil {
			log.Warnf("skipping reconfiguration of eth p2p server logger due to error: %v", err)
		}

		group.Go(blockchainServer.Start)
	} else {
		log.Infof("skipping starting blockchain client as no enodes have been provided")
	}

	if startupPrysmClient {
		for _, prysmAddr := range ethConfig.StaticPeers.PrysmAddrs() {
			prysmClient := beacon.NewPrysmClient(gCtx, ethConfig, prysmAddr.PrysmAddr, bridge, prysmAddr.Endpoint)
			group.Go(func() error {
				prysmClient.Start()
				return nil
			})
		}
	}

	<-gCtx.Done()
	if pprofServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err = pprofServer.Shutdown(shutdownCtx); err != nil {
			log.Errorf("error shutting down pprof server: %v", err)
		}
	}

	log.Infof("shutting down...")

	gErr := group.Wait()
	if gErr != nil {
		log.Errorf("one or more components failed to start: %v", gErr)
	}

	err = gateway.Close()
	if err != nil {
		log.Errorf("error shutting down gateway: %v", err)
	}

	if blockchainServer != nil {
		log.Infof("stopping blockchain client...")
		blockchainServer.Stop()
	}

	if beaconNode != nil {
		log.Infof("stopping beacon node...")
		beaconNode.Stop()
	}

	return gErr
}

func downloadGenesisFile(ctx context.Context, network, genesisFilePath string) (string, error) {
	var genesisFileURL string
	switch network {
	case bxgateway.Mainnet:
		genesisFileURL = "https://github.com/eth-clients/eth2-mainnet/raw/master/genesis.ssz"
	case bxgateway.Holesky:
		genesisFileURL = "https://github.com/eth-clients/holesky/raw/main/custom_config_data/genesis.ssz"
	default:
		return "", fmt.Errorf("beacon node is only supported on Ethereum")
	}

	out, err := os.Create(genesisFilePath)
	if err != nil {
		return "", fmt.Errorf("failed creating %v file %v", genesisFilePath, err)
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, genesisFileURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed creating request for genesis.ssz from %v %v", genesisFileURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed calling server for genesis.ssz from %v %v", genesisFileURL, err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed downloading genesis.ssz from %v %v", genesisFileURL, err)
	}

	return genesisFileURL, nil
}
