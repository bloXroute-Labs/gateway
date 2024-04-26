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
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/blockproposer"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/nodes"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/httpclient"
	"github.com/bloXroute-Labs/gateway/v2/version"
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
			utils.GatewayModeFlag,
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
			utils.PolygonMainnetHeimdallEndpoints,
			utils.TransactionHoldDuration,
			utils.TransactionPassedDueDuration,
			utils.EnableBlockchainRPCMethodSupport,
			utils.DialRatio,
			utils.NumRecommendedPeers,
			utils.NoTxsToBlockchain,
			utils.NoBlocks,
			utils.NoStats,
			utils.EnableBloomFilter,
			utils.BlocksToCacheWhileProposing,
			utils.BSCProposeBlocks,
			utils.BSCRegularBlockSendDelayInitialMSFlag,
			utils.BSCRegularBlockSendDelaySecondMSFlag,
			utils.BSCRegularBlockSendDelayIntervalMSFlag,
			utils.BSCHighLoadBlockSendDelayInitialMSFlag,
			utils.BSCHighLoadBlockSendDelaySecondMSFlag,
			utils.BSCHighLoadBlockSendDelayIntervalMSFlag,
			utils.BSCHighLoadTxNumThresholdFlag,
			utils.TxIncludeSenderInFeed,
			utils.EnableIntroductoryIntentsAccess,
			utils.BeaconTrustedPeersFileFlag,
			utils.BeaconPort,
		},
		Action: runGateway,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func runGateway(c *cli.Context) error {
	ctx := utils.ContextWithSignal(c.Context)

	group, ctx := errgroup.WithContext(ctx)

	var pprofServer *http.Server
	if !c.Bool(utils.DisableProfilingFlag.Name) {
		pprofServer = &http.Server{Addr: "0.0.0.0:6060"}
		group.Go(func() error {
			log.Infof("pprof http server is running on 0.0.0.0:6060 - %v", "http://localhost:6060/debug/pprof")

			if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("failed to start pprof http server: %v", err)
			}

			return nil
		})
	}

	bxConfig, err := config.NewBxFromCLI(c)
	if err != nil {
		return err
	}

	err = log.Init(bxConfig.Config, version.BuildVersion)
	if err != nil {
		return err
	}

	dataDir := c.String(utils.DataDirFlag.Name)
	ethConfig, gatewayPublicKey, err := network.NewPresetEthConfigFromCLI(c, dataDir)
	if err != nil {
		return err
	}

	var blockchainPeers []types.NodeEndpoint
	var prysmEndpoint types.NodeEndpoint
	var prysmAddr string
	blockchainNetwork := c.String(utils.BlockchainNetworkFlag.Name)

	for _, blockchainPeerInfo := range ethConfig.StaticPeers {
		var endpoint types.NodeEndpoint
		if blockchainPeerInfo.Enode != nil {
			endpoint = utils.EnodeToNodeEndpoint(blockchainPeerInfo.Enode, blockchainNetwork)
		} else if blockchainPeerInfo.Multiaddr != nil {
			endpoint = utils.MultiaddrToNodeEndoint(*blockchainPeerInfo.Multiaddr, blockchainNetwork)
			prysmEndpoint = endpoint
		} else if blockchainPeerInfo.BeaconAPIURI != "" {
			endpointPtr, err := beacon.CreateAPIEndpoint(blockchainPeerInfo.BeaconAPIURI, blockchainNetwork)
			if err != nil {
				return err
			}
			endpoint = *endpointPtr
		} else {
			continue
		}

		blockchainPeers = append(blockchainPeers, endpoint)

		if blockchainPeerInfo.PrysmAddr != "" {
			prysmAddr = blockchainPeerInfo.PrysmAddr
		}
	}

	sslCerts, sdn, err := nodes.InitSDN(bxConfig, blockchainPeers, nodes.GeneratePeers(ethConfig.StaticPeers), len(ethConfig.StaticEnodes()))
	if err != nil {
		return err
	}

	// create a set of recommended peers
	recommendedPeers := make(map[string]struct{})

	startupBeaconNode := bxConfig.GatewayMode.IsBDN() && len(ethConfig.BeaconNodes()) > 0 || c.Int(utils.BeaconPort.Name) != 0
	startupBeaconAPIClients := bxConfig.GatewayMode.IsBDN() && len(ethConfig.BeaconAPIEndpoints()) > 0
	startupBlockchainClient := startupBeaconAPIClients || startupBeaconNode || len(ethConfig.StaticEnodes()) > 0 || bxConfig.EnableDynamicPeers // if beacon node running we need to receive txs also
	startupPrysmClient := bxConfig.GatewayMode.IsBDN() && prysmAddr != ""

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
	if (bxConfig.WebsocketEnabled || bxConfig.WebsocketTLSEnabled) && !ethConfig.ValidWSAddr() {
		log.Warn("websocket server enabled but no valid websockets endpoint specified via --eth-ws-uri nor --multi-node: only newTxs and bdnBlocks feeds are available")
	}
	if bxConfig.ManageWSServer && !ethConfig.ValidWSAddr() {
		return fmt.Errorf("if websocket server management is enabled, a valid websocket address must be provided")
	}
	if bxConfig.EnableBlockchainRPC && !ethConfig.ValidWSAddr() {
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
			genesisPath, err = downloadGenesisFile(c.String(utils.BlockchainNetworkFlag.Name), localGenesisFile)
			if err != nil {
				return err
			}
		}
		log.Info("connecting to beacon node using ", genesisPath)

		beaconNode, err = beacon.NewNode(beacon.NodeParams{
			ParentContext:        ctx,
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

		if err = beaconNode.Start(); err != nil {
			return err
		}
	}

	beaconAPIClients := make([]*beacon.APIClient, 0)
	if startupBeaconAPIClients {
		for _, endpoint := range ethConfig.BeaconAPIEndpoints() {
			client, err := beacon.NewAPIClient(ctx, httpclient.Client(nil), ethConfig, bridge, endpoint, blockchainNetwork)
			if err != nil {
				return fmt.Errorf("error creating new beacon api client: %v", err)
			}
			client.Start()
			beaconAPIClients = append(beaconAPIClients, client)
		}
	}

	var blobsManager *beacon.BlobSidecarCacheManager
	if startupBeaconNode || startupBeaconAPIClients {
		blobsManager = beacon.NewBlobSidecarCacheManager(ethConfig.GenesisTime)
		go beacon.HandleBDNBeaconMessages(ctx, bridge, beaconNode, blobsManager)
		go beacon.HandleBDNBlocks(ctx, bridge, beaconNode, beaconAPIClients, blobsManager)
	}

	gateway, err := nodes.NewGateway(
		ctx,
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
		len(ethConfig.StaticEnodes()),
		c.String(utils.PolygonMainnetHeimdallEndpoints.Name),
		c.Int(utils.TransactionHoldDuration.Name),
		c.Int(utils.TransactionPassedDueDuration.Name),
		c.Bool(utils.EnableBloomFilter.Name),
		c.Int64(utils.BlocksToCacheWhileProposing.Name),
		c.Bool(utils.TxIncludeSenderInFeed.Name),
		&blockproposer.SendingConfig{
			Enabled:                        c.Bool(utils.BSCProposeBlocks.Name),
			RegularBlockSendDelayInitial:   time.Duration(c.Int(utils.BSCRegularBlockSendDelayInitialMSFlag.Name)) * time.Millisecond,
			RegularBlockSendDelaySecond:    time.Duration(c.Int(utils.BSCRegularBlockSendDelaySecondMSFlag.Name)) * time.Millisecond,
			RegularBlockSendDelayInterval:  time.Duration(c.Int(utils.BSCRegularBlockSendDelayIntervalMSFlag.Name)) * time.Millisecond,
			HighLoadBlockSendDelayInitial:  time.Duration(c.Int(utils.BSCHighLoadBlockSendDelayInitialMSFlag.Name)) * time.Millisecond,
			HighLoadBlockSendDelaySecond:   time.Duration(c.Int(utils.BSCHighLoadBlockSendDelaySecondMSFlag.Name)) * time.Millisecond,
			HighLoadBlockSendDelayInterval: time.Duration(c.Int(utils.BSCHighLoadBlockSendDelayIntervalMSFlag.Name)) * time.Millisecond,
			HighLoadTxNumThreshold:         c.Int(utils.BSCHighLoadTxNumThresholdFlag.Name),
		},
	)
	if err != nil {
		return err
	}

	group.Go(func() error {
		return gateway.Run()
	})

	// Required for beacon node and prysm to sync
	ethChain := eth.NewChain(ctx, ethConfig.IgnoreBlockTimeout)

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

		blockchainServer, err = eth.NewServerWithEthLogger(ctx, port, externalIP, ethConfig, ethChain, bridge, dataDir, wsManager, dynamicPeers, dialRatio, recommendedPeers)
		if err != nil {
			return err
		}

		if err = blockchainServer.AddEthLoggerFileHandler(bxConfig.Config.FileName); err != nil {
			log.Warnf("skipping reconfiguration of eth p2p server logger due to error: %v", err)
		}

		if err = blockchainServer.Start(); err != nil {
			return err
		}
	} else {
		log.Infof("skipping starting blockchain client as no enodes have been provided")
	}

	var prysmClient *beacon.PrysmClient
	if startupPrysmClient {
		prysmClient = beacon.NewPrysmClient(ctx, ethConfig, prysmAddr, bridge, prysmEndpoint)
		prysmClient.Start()
	}

	<-ctx.Done()

	log.Infof("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = gateway.Close()
	if err != nil {
		log.Errorf("error shutting down gateway: %v", err)
	}

	if pprofServer != nil {
		if err = pprofServer.Shutdown(shutdownCtx); err != nil {
			log.Errorf("error shutting down pprof server: %v", err)
		}
	}

	if blockchainServer != nil {
		log.Infof("stopping blockchain client...")
		blockchainServer.Stop()
	}

	if beaconNode != nil {
		log.Infof("stopping beacon node...")
		beaconNode.Stop()
	}

	return group.Wait()
}

func downloadGenesisFile(network, genesisFilePath string) (string, error) {
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

	resp, err := http.Get(genesisFileURL)
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
