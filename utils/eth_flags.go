package utils

import "github.com/urfave/cli/v2"

// Ethereum specific flags
var (
	EnodesFlag = &cli.StringFlag{
		Name:  "enodes",
		Usage: "specify a single enode peer to connect to (if you would like to specify multiple peers, use multi-enode)",
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
	MultiNode = &cli.StringFlag{
		Name: "multi-node",
		Usage: "comma separated list of nodes." +
			"Each connection URI is divided by a plus sign, and it is permissible to omit websockets and beacon endpoint from any node. " +
			"Example: enode[+eth-ws-uri][+beacon:prysm-host:prysm-port] " +
			"(This parameter need only be used if multiple p2p, websockets or prysm connections are desired.)",
	}
)
