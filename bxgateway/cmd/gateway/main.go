package main

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/blockchain"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/blockchain/eth"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/blockchain/network"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/config"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/nodes"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/utils"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/version"
	"github.com/ethereum/go-ethereum/crypto"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	app := &cli.App{
		Name:  "gateway",
		Usage: "run a NG gateway",
		Flags: []cli.Flag{
			utils.ExternalIPFlag,
			utils.PortFlag,
			utils.SDNURLFlag,
			utils.CACertURLFlag,
			utils.RegistrationCertDirFlag,
			utils.WSFlag,
			utils.WSPortFlag,
			utils.EnvFlag,
			utils.LogLevelFlag,
			utils.LogFileLevelFlag,
			utils.LogMaxSizeFlag,
			utils.LogMaxAgeFlag,
			utils.LogMaxBackupsFlag,
			utils.TxTraceEnabledFlag,
			utils.TxTraceMaxFileSize,
			utils.TxTraceMaxFileCount,
			utils.AvoidPrioritySendingFlag,
			utils.RelayHostFlag,
			utils.DataDirFlag,
			utils.GRPCFlag,
			utils.GRPCHostFlag,
			utils.GRPCPortFlag,
			utils.GRPCUserFlag,
			utils.GRPCPasswordFlag,
			utils.BlockchainNetworkFlag,
			utils.EnodesFlag,
			utils.BlocksOnlyFlag,
			utils.AllTransactionsFlag,
			utils.PrivateKeyFlag,
			utils.EthWSUriFlag,
			utils.NodeTypeFlag,
			utils.DisableProfilingFlag,
			utils.FluentDFlag,
			utils.FluentdHostFlag,
			utils.LogNetworkContentFlag,
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

	ethConfig, err := network.NewPresetEthConfigFromCLI(c)
	if err != nil {
		return err
	}

	var blockchainPeers []types.NodeEndpoint
	for _, blockchainPeer := range ethConfig.StaticPeers {
		enodePublicKey := fmt.Sprintf("%x", crypto.FromECDSAPub(blockchainPeer.Pubkey())[1:])
		blockchainPeers = append(blockchainPeers, types.NodeEndpoint{IP: blockchainPeer.IP().String(), Port: blockchainPeer.TCP(), PublicKey: enodePublicKey})
	}
	startupBlockchainClient := len(blockchainPeers) > 0

	err = nodes.InitLogs(bxConfig.Log, version.BuildVersion)
	if err != nil {
		return err
	}

	var bridge blockchain.Bridge
	if startupBlockchainClient {
		bridge = blockchain.NewBxBridge(eth.Converter{})
	} else {
		bridge = blockchain.NewNoOpBridge(eth.Converter{})
	}

	gateway, err := nodes.NewGateway(ctx, bxConfig, bridge, blockchainPeers)
	if err != nil {
		return err
	}
	go func() {
		err = gateway.Run()
		if err != nil {
			panic(err)
		}
	}()

	var blockchainServer *eth.Server
	if startupBlockchainClient {
		log.Infof("starting blockchain client with config for network ID: %v", ethConfig.Network)

		blockchainServer, err = eth.NewServerWithEthLogger(ctx, ethConfig, bridge, c.String(utils.DataDirFlag.Name), c.String(utils.EthWSUriFlag.Name))
		if err != nil {
			return nil
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
