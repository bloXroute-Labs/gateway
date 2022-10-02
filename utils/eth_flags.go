package utils

import "github.com/urfave/cli/v2"

// Ethereum specific flags
var (
	EnodesFlag = &cli.StringFlag{
		Name:  "enodes",
		Usage: "specify a single enode peer to connect to (if you would like to specify multiple peers, use multi-enode)",
	}
	BeaconENRFlag = &cli.StringFlag{
		Name:  "enr",
		Usage: "specify the beacon enr, you can extract it from your beacon node using 'http://<beacon-ip>:<beacon-http-port>/eth/v1/node/identity'",
	}
	PrivateKeyFlag = &cli.StringFlag{
		Name:     "private-key",
		Usage:    "private key for encrypted communication with Ethereum node",
		Required: false,
	}
	EthWSUriFlag = &cli.StringFlag{
		Name:  "eth-ws-uri",
		Usage: "Ethereum websockets endpoint",
	}
	PrysmGRPCFlag = &cli.StringFlag{
		Name:  "prysm-grpc-uri",
		Usage: "Prysm gRPC endpoint. Expected format: IP:PORT",
	}
	MultiNode = &cli.StringFlag{
		Name: "multi-node",
		Usage: "comma separated list of nodes." +
			"Each connection URI is divided by a plus sign, and it is permissible to omit websockets and beacon endpoint from any node. \n" +
			"Syntax: [enode[+eth-ws-uri],enr[+prysm://prysm-host:prysm-port]]+ \n" +
			"Example: enode://aaa...bbb@1.1.1.1:30303+ws://1.1.1.1:5456,enr://....+prysm://1.1.1.1:4000",
	}
	GensisFilePath = &cli.StringFlag{
		Name:   "genesis-path",
		Usage:  "overrides the genesis block from the internet",
		Hidden: true,
	}
	BeaconBlock = &cli.BoolFlag{
		Name:   "beacon-block",
		Usage:  "if true will propagate beacon block",
		Hidden: true,
	}
)
