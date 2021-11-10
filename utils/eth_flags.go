package utils

import "github.com/urfave/cli/v2"

// Ethereum specific flags
var (
	EnodesFlag = &cli.StringFlag{
		Name:  "enodes",
		Usage: "comma separated list of enode peers to connect to (multiple peers is currently not yet supported, so please only specify one)",
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
)
