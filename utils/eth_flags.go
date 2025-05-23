package utils

import (
	"github.com/urfave/cli/v2"
)

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
	BeaconMultiaddrFlag = &cli.StringFlag{
		Name:  "multiaddr",
		Usage: "specify the beacon multiaddr",
	}
	BeaconTrustedPeersFileFlag = &cli.StringFlag{
		Name:  "beacon-trusted-peers-file",
		Usage: "specify the file containing the list of trusted peers for the beacon chain",
	}
	BeaconPort = &cli.IntFlag{
		Name:  "beacon-port",
		Usage: "specify the port for the beacon chain",
	}
	EnableQuicFlag = &cli.BoolFlag{
		Name:  "enable-quic",
		Usage: "enable QUIC protocol for the beacon chain",
		Value: false,
	}
	PrivateKeyFlag = &cli.StringFlag{
		Name:     "private-key",
		Usage:    "private key for encrypted communication with Ethereum node",
		Required: false,
	}
	EthWSUriFlag = &cli.StringFlag{
		Name:        "eth-ws-uri",
		Usage:       "Ethereum websockets endpoint",
		DefaultText: "",
	}
	BSCWSUriFlag = &cli.StringFlag{
		Name:        "bsc-ws-uri",
		Usage:       "Ethereum websockets endpoint",
		DefaultText: "",
	}
	BeaconAPIUriFlag = &cli.StringFlag{
		Name:     "beacon-api-uri",
		Usage:    "Beacon API endpoints. Expected format: IP:PORT",
		Required: false,
	}
	PrysmGRPCFlag = &cli.StringFlag{
		Name:  "prysm-grpc-uri",
		Usage: "Prysm gRPC endpoint. Expected format: IP:PORT",
	}
	MultiNode = &cli.StringFlag{
		Name: "multi-node",
		Usage: `comma separated list of nodes.
	Each connection URI is divided by a plus sign, and it is permissible to omit websockets and beacon endpoint from any node.
	Syntax: [enode[+eth-ws-uri],enr[+prysm://prysm-host:prysm-port],multiaddr[+prysm://prysm-host:prysm-port],beacon-api://ip:port]
	Example: enode://aaa...bbb@1.1.1.1:30303+ws://1.1.1.1:5456,enr://....+prysm://1.1.1.1:4000,multiaddr:/ip4/2.2.2.2/tcp/13000/p2p/...+prysm://2.2.2.2:4000,beacon-api://2.1.1.7:3500`,
	}
	GensisFilePath = &cli.StringFlag{
		Name:   "genesis-path",
		Usage:  "overrides the genesis block from the internet",
		Hidden: true,
	}
	EthPropagationBlockDelay = &cli.DurationFlag{
		Name:   "eth-propagation-delay",
		Value:  0,
		Usage:  "ethereum execution layer block propagation delay for gateways",
		Hidden: true,
	}
)
