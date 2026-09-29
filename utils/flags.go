package utils

import (
	"github.com/urfave/cli/v2"

	"github.com/bloXroute-Labs/gateway/v2"
)

// CLI flag variable definitions
var (
	HostFlag = &cli.StringFlag{
		Name:  "host",
		Usage: "listening interface to bind server on (can be omitted to default to 0.0.0.0)",
		Value: bxgateway.AllInterfaces,
	}
	ExternalIPFlag = &cli.StringFlag{
		Name:    "external-ip",
		Usage:   "public IP address to send to bxapi (can be omitted to be derived on startup)",
		Aliases: []string{"ip"},
	}
	PortFlag = &cli.IntFlag{
		Name:    "port",
		Usage:   "port for accepting gateway connection",
		Aliases: []string{"p"},
		Value:   1809,
	}
	RelayHostsFlag = &cli.StringFlag{
		Name:    "relays",
		Usage:   "host of relay",
		Aliases: []string{"relay-ip"},
		Value:   "auto",
	}
	EnvFlag = &cli.StringFlag{
		Name:  "env",
		Usage: "development environment (local, localproxy, testnet, mainnet)",
		Value: "mainnet",
	}
	SDNURLFlag = &cli.StringFlag{
		Name:   "sdn-url",
		Usage:  "SDN URL",
		Hidden: true,
	}
	WSFlag = &cli.BoolFlag{
		Name:  "ws",
		Usage: "starts a websocket RPC server",
		Value: false,
	}
	WSTLSFlag = &cli.BoolFlag{
		Name:  "ws-tls",
		Usage: "starts the websocket server using TLS",
		Value: false,
	}
	WSHostFlag = &cli.StringFlag{
		Name:  "ws-host",
		Usage: "host address for RPC server to run on",
		Value: "127.0.0.1",
	}
	WSPortFlag = &cli.IntFlag{
		Name:    "ws-port",
		Usage:   "port for RPC server to run on",
		Aliases: []string{"wsp", "rpc-port"},
		Value:   28333,
	}
	CACertURLFlag = &cli.StringFlag{
		Name:  "ca-cert-url",
		Usage: "URL for retrieving CA certificates",
	}
	FluentdHostFlag = &cli.StringFlag{
		Name:    "fluentd-host",
		Usage:   "fluentd host",
		Aliases: []string{"fh"},
		Value:   "localhost",
		Hidden:  true,
	}
	FluentDFlag = &cli.BoolFlag{
		Name:   "fluentd",
		Usage:  "sends logs records to fluentD",
		Value:  false,
		Hidden: true,
	}
	LogNetworkContentFlag = &cli.BoolFlag{
		Name:   "log-network-content",
		Usage:  "sends blockchain content to fluentD",
		Value:  false,
		Hidden: true,
	}
	RegistrationCertDirFlag = &cli.StringFlag{
		Name:   "registration-cert-dir",
		Usage:  "base dir for retrieving SSL certificates",
		Hidden: true,
	}
	DisableProfilingFlag = &cli.BoolFlag{
		Name:  "disable-profiling",
		Usage: "true to disable the pprof http server (for relays, where profiling is enabled by default)",
		Value: false,
	}
	DataDirFlag = &cli.StringFlag{
		Name:  "data-dir",
		Usage: "directory for storing various persistent files (e.g. private SSL certs)",
		Value: "datadir",
	}
	LogLevelFlag = &cli.StringFlag{
		Name:    "log-level",
		Usage:   "log level for stdout",
		Aliases: []string{"l"},
		Value:   "info",
	}
	LogFileLevelFlag = &cli.StringFlag{
		Name:  "log-file-level",
		Usage: "log level for the log file",
		Value: "info",
	}
	LogMaxSizeFlag = &cli.IntFlag{
		Name:  "log-max-size",
		Usage: "maximum size in megabytes of the log file before it gets rotated",
		Value: 100,
	}
	LogMaxAgeFlag = &cli.IntFlag{
		Name:  "log-max-age",
		Usage: "maximum number of days to retain old log files based on the timestamp encoded in their filename",
		Value: 10,
	}
	LogMaxBackupsFlag = &cli.IntFlag{
		Name:  "log-max-backups",
		Usage: "maximum number of old log files to retain",
		Value: 10,
	}
	GRPCFlag = &cli.BoolFlag{
		Name:  "grpc",
		Usage: "starts the GRPC server",
		Value: false,
	}
	GRPCHostFlag = &cli.StringFlag{
		Name:  "grpc-host",
		Usage: "host address for GRPC server to run on",
		Value: "127.0.0.1",
	}
	GRPCPortFlag = &cli.IntFlag{
		Name:  "grpc-port",
		Usage: "port for GRPC server to run on",
		Value: 5001,
	}
	GRPCUserFlag = &cli.StringFlag{
		Name:  "grpc-user",
		Usage: "user for GRPC authentication",
		Value: "",
	}
	GRPCPasswordFlag = &cli.StringFlag{
		Name:  "grpc-password",
		Usage: "password for GRPC authentication",
		Value: "",
	}
	GRPCAuthFlag = &cli.StringFlag{
		Name:  "auth-header",
		Usage: "raw authentication header for GRPC ",
	}
	BlockchainNetworkFlag = &cli.StringFlag{
		Name:  "blockchain-network",
		Usage: "determine the blockchain network (Mainnet or BSC-Mainnet)",
		Value: "Mainnet",
	}
	BlocksOnlyFlag = &cli.BoolFlag{
		Name:    "blocks-only",
		Usage:   "set this flag to only propagate blocks from the BDN to the connected node",
		Aliases: []string{"miner"},
		Value:   false,
	}
	AllTransactionsFlag = &cli.BoolFlag{
		Name:  "all-txs",
		Usage: "set this flag to propagate all transactions from the BDN to the connected node (warning: may result in worse performance and propagation times)",
		Value: false,
	}
	TxTraceEnabledFlag = &cli.BoolFlag{
		Name:  "txtrace",
		Usage: "for gateways only, enables transaction trace logging",
		Value: false,
	}
	TxTraceMaxFileSizeFlag = &cli.IntFlag{
		Name:  "txtrace-max-file-size",
		Usage: "for gateways only, sets max size of individual tx trace log file (megabytes)",
		Value: 100,
	}
	TxTraceMaxBackupFilesFlag = &cli.IntFlag{
		Name:  "txtrace-max-backup-files",
		Usage: "for gateways only, sets max number of backup tx trace log files retained (0 enables unlimited backups)",
		Value: 3,
	}
	NodeTypeFlag = &cli.StringFlag{
		Name:  "node-type",
		Usage: "set node type",
		Value: "external_gateway",
	}
	ManageWSServer = &cli.BoolFlag{
		Name:  "manage-ws-server",
		Usage: "for gateways only, monitors blockchain node sync status and shuts down/restarts websocket server accordingly",
		Value: false,
	}
	SendBlockConfirmation = &cli.BoolFlag{
		Name:   "send-block-confirmation",
		Usage:  "sending block confirmation to relay",
		Value:  false,
		Hidden: true,
	}
	TerminalTotalDifficulty = &cli.StringFlag{
		Name:   "terminal-total-difficulty",
		Usage:  "Overrides the terminal total difficulty settings of the blockchain network",
		Hidden: true,
	}
	EnableBloomFilter = &cli.BoolFlag{
		Name:   "enable-bloom-filter",
		Usage:  "enables bloom filter for relayproxy to ignore already seen transactions",
		Value:  false,
		Hidden: true,
	}
	EnableBlockchainRPCMethodSupport = &cli.BoolFlag{
		Name:  "enable-blockchain-rpc",
		Usage: "forwards blockchain RPC methods to the node and returns node response",
		Value: false,
	}
	PendingTxsSourceFromNode = cli.BoolFlag{
		Name:  "new-pending-txs-source-from-node",
		Usage: "enable this flag will make the source of newPendingTransactions feed to be node pendingTxs",
		Value: false,
	}
	NoTxsToBlockchain = &cli.BoolFlag{
		Name:   "no-txs",
		Usage:  "enable NoTxsToBlockchain for not sending txs to blockchain nodes",
		Value:  false,
		Hidden: true,
	}
	NoBlocks = &cli.BoolFlag{
		Name:   "no-blocks",
		Usage:  "enable no-blocks for not processing blocks",
		Value:  false,
		Hidden: true,
	}
	NoStats = &cli.BoolFlag{
		Name:   "no-stats",
		Usage:  "enable no-stats for not processing stats",
		Value:  false,
		Hidden: true,
	}
	TxIncludeSenderInFeed = &cli.BoolFlag{
		Name:   "tx-include-sender-in-feed",
		Usage:  "(for gateways only) include sender address in transaction feed",
		Hidden: true,
		Value:  false,
	}
	SubmitBeaconBlockToAPI = &cli.BoolFlag{
		Name:   "submit-beacon-block-to-api",
		Usage:  "submit beacon block to api",
		Hidden: true,
		Value:  true,
	}
)

