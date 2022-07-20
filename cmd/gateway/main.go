package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/nodes"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/urfave/cli/v2"
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
			utils.PrysmHostFlag,
			utils.PrysmPortFlag,
			utils.BlockchainNetworkFlag,
			utils.EnodesFlag,
			utils.EthWSUriFlag,
			utils.MultiNode,
			utils.BlocksOnlyFlag,
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
			utils.MEVBuilderURIFlag,
			utils.MEVMinerURIFlag,
			utils.MEVBundleMethodNameFlag,
			utils.SendBlockConfirmation,
			utils.MegaBundleProcessing,
			utils.TerminalTotalDifficulty,
		},
		Action: runGateway,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func runGateway(c *cli.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !c.Bool(utils.DisableProfilingFlag.Name) {
		go func() {
			log.Infof("pprof http server is running on 0.0.0.0:6060 - %v", "http://localhost:6060/debug/pprof")
			log.Error(http.ListenAndServe("0.0.0.0:6060", nil))
		}()
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

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
	for _, blockchainPeerInfo := range ethConfig.StaticPeers {
		blockchainPeer := blockchainPeerInfo.Enode
		enodePublicKey := fmt.Sprintf("%x", crypto.FromECDSAPub(blockchainPeer.Pubkey())[1:])
		blockchainPeers = append(blockchainPeers, types.NodeEndpoint{IP: blockchainPeer.IP().String(), Port: blockchainPeer.TCP(), PublicKey: enodePublicKey})
	}
	startupBlockchainClient := bxConfig.GatewayMode.IsBDN() && len(blockchainPeers) > 0

	var bridge blockchain.Bridge
	if startupBlockchainClient {
		bridge = blockchain.NewBxBridge(eth.Converter{})
	} else {
		bridge = blockchain.NewNoOpBridge(eth.Converter{})
	}

	if bxConfig.ManageWSServer && !bxConfig.WebsocketEnabled && !bxConfig.WebsocketTLSEnabled {
		return fmt.Errorf("websocket server must be enabled using --ws or --ws-tls if --manage-ws-server is enabled")
	}
	wsManager := eth.NewEthWSManager(ethConfig.StaticPeers, eth.NewWSProvider, bxgateway.WSProviderTimeout)
	if (bxConfig.WebsocketEnabled || bxConfig.WebsocketTLSEnabled) && !ethConfig.ValidWSAddr() {
		log.Warn("websocket server enabled but no valid websockets endpoint specified via --eth-ws-uri nor --multi-node: only newTxs and bdnBlocks feeds are available")
	}
	if bxConfig.ManageWSServer && !ethConfig.ValidWSAddr() {
		return fmt.Errorf("if websocket server management is enabled, a valid websocket address must be provided")
	}

	gateway, err := nodes.NewGateway(ctx, bxConfig, bridge, wsManager, blockchainPeers, ethConfig.StaticPeers, gatewayPublicKey)
	if err != nil {
		return err
	}

	if err = gateway.Run(); err != nil {
		// TODO close the gateway while notify all other go routine (bridge, ws server, ...)
		log.Errorf("closing gateway with err %v", err)
		log.Exit(0)
	}

	var blockchainServer *eth.Server
	if startupBlockchainClient {
		log.Infof("starting blockchain client with config for network ID: %v", ethConfig.Network)

		blockchainServer, err = eth.NewServerWithEthLogger(ctx, ethConfig, bridge, dataDir, wsManager)
		if err != nil {
			return nil
		}

		if err = blockchainServer.AddEthLoggerFileHandler(bxConfig.Config.FileName); err != nil {
			log.Warnf("skipping reconfiguration of eth p2p server logger due to error: %v", err)
		}

		if err = blockchainServer.Start(); err != nil {
			return nil
		}
	} else {
		log.Infof("skipping starting blockchain client as no enodes have been provided")
	}

	<-sigc

	if blockchainServer != nil {
		blockchainServer.Stop()
	}
	return nil
}
