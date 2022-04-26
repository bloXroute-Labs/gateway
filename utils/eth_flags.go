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
		Usage: "comma separated list of enode and Ethereum websockets endpoint pairs. " +
			"Each pair is divided by a plus sign, and it is permissible to omit a websockets endpoint from any pair. " +
			"Example: enode1+eth-ws-uri-1,enode2+,enode3+eth-ws-uri-3... " +
			"(This parameter need only be used if multiple p2p or websockets connections are desired.)",
	}
)
