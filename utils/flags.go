package utils

import (
	"github.com/bloXroute-Labs/gateway"
	"github.com/urfave/cli/v2"
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
	LegacyTxPortFlag = &cli.IntFlag{
		Name:  "legacy-tx-port",
		Usage: "tx port for accepting 'RELAY_TRANSACTION' gateway connection (legacy flag)",
		Value: 1810,
	}
	RelayBlockPortFlag = &cli.Int64Flag{
		Name:    "relay-block-port",
		Usage:   "port of block relay",
		Aliases: []string{"rb"},
		Value:   0, // was 1809
	}
	RelayBlockHostFlag = &cli.StringFlag{
		Name:    "relay-block-host",
		Usage:   "host of block relay",
		Aliases: []string{"rbh"},
		Value:   "localhost",
	}
	RelayTxPortFlag = &cli.Int64Flag{
		Name:    "relay-tx-port",
		Usage:   "port of transaction relay",
		Aliases: []string{"rp"},
		Value:   0, // was 1811
	}
	RelayTxHostFlag = &cli.StringFlag{
		Name:    "relay-tx-host",
		Usage:   "host of transaction relay",
		Aliases: []string{"rth"},
		Value:   "localhost",
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
		Name:  "sdn-url",
		Usage: "SDN URL",
	}
	SDNSocketIPFlag = &cli.StringFlag{
		Name:  "sdn-socket-ip",
		Usage: "SDN socket broker IP address",
		Value: "127.0.0.1",
	}
	SDNSocketPortFlag = &cli.IntFlag{
		Name:  "sdn-socket-port",
		Usage: "SDN socket broker port",
		Value: 1800,
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
	HTTPPortFlag = &cli.IntFlag{
		Name:  "http-port",
		Usage: "port for HTTP server to run on",
		Value: 28335,
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
	}
	FluentDFlag = &cli.BoolFlag{
		Name:  "fluentd",
		Usage: "sends logs records to fluentD",
		Value: false,
	}
	LogNetworkContentFlag = &cli.BoolFlag{
		Name:   "log-network-content",
		Usage:  "sends blockchain content to fluentD",
		Value:  false,
		Hidden: true,
	}
	// TODO: this currently must be a file path in current code, not a URL
	RegistrationCertDirFlag = &cli.StringFlag{
		Name:  "registration-cert-dir",
		Usage: "base URL for retrieving SSL certificates",
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
	// TBD: remove priority queue and priority from code base. Left here for backward competability but hidden
	AvoidPrioritySendingFlag = &cli.BoolFlag{
		Name:   "avoid-priority-sending",
		Usage:  "avoid sending via priority queue",
		Value:  true,
		Hidden: true,
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
	RedisFlag = &cli.BoolFlag{
		Name:  "redis",
		Usage: "optionally enable Redis for extended caching support for tx trace",
		Value: false,
	}
	RedisHostFlag = &cli.StringFlag{
		Name:  "redis-host",
		Usage: "redis connection host address",
		Value: "127.0.0.1",
	}
	RedisPortFlag = &cli.IntFlag{
		Name:  "redis-port",
		Usage: "redis connection port",
		Value: 6379,
	}
	ContinentFlag = &cli.StringFlag{
		Name:     "continent",
		Usage:    "override value for continent current node is running in (otherwise autodetected from IP address)",
		Required: false,
	}
	CountryFlag = &cli.StringFlag{
		Name:     "country",
		Usage:    "override value for country current node is running in (otherwise autodetected from IP address)",
		Required: false,
	}
	RegionFlag = &cli.StringFlag{
		Name:     "region",
		Usage:    "override value for datacenter region current node is running in (otherwise autodetected from IP address)",
		Required: false,
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
		Name:  "grpc-auth",
		Usage: "raw authentication header for GRPC ",
	}
	PeerFileFlag = &cli.StringFlag{
		Name:  "peer-file",
		Usage: "peer file containing the ip:port list of potential peers for the node to connect to",
		Value: "proxypeers",
	}
	BlockchainNetworkFlag = &cli.StringFlag{
		Name:  "blockchain-network",
		Usage: "determine the blockchain network (Mainnet or BSC-Mainnet)",
		Value: "Mainnet",
	}
	SyncPeerIPFlag = &cli.StringFlag{
		Name:  "sync-peer-ip",
		Usage: "the ip address of the node that should sync this node. if not provided the ATR will be used",
	}
	PlatformProviderFlag = &cli.StringFlag{
		Name:     "platform-provider",
		Usage:    "override value for current node platform provider",
		Required: false,
	}
	DisableTxStoreCleanupFlag = &cli.BoolFlag{
		Name:  "disable-txstore-cleanup",
		Usage: "if true, relay would NOT be responsible for the txs cleanup",
		Value: false,
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
	GatewayModeFlag = &cli.StringFlag{
		Name:  "mode",
		Usage: "set gateway mode",
		Value: "bdn",
	}
	SSLFlag = &cli.BoolFlag{
		Name:  "ssl",
		Usage: "Opens a http/websocket server with TLS",
		Value: false,
	}
	ManageWSServer = &cli.BoolFlag{
		Name:  "manage-ws-server",
		Usage: "for gateways only, monitors blockchain node sync status and shuts down/restarts websocket server accordingly",
		Value: false,
	}
	MEVBuilderURIFlag = &cli.StringFlag{
		Name:  "mev-builder-uri",
		Usage: "set mev builder for gateway",
	}
	MEVMinerURIFlag = &cli.StringFlag{
		Name:  "mev-miner-uri",
		Usage: "set mev miner for gateway",
	}
	MEVBundleMethodNameFlag = &cli.StringFlag{
		Name:  "mev-bundle-method-name",
		Usage: "set custom method for mevBundle request",
		Value: "eth_sendBundle",
	}
	SendBlockConfirmation = &cli.BoolFlag{
		Name:  "send-block-confirmation",
		Usage: "sending block confirmation to relay",
		Value: false,
	}
	MegaBundleProcessing = &cli.BoolFlag{
		Name:  "mega-bundle-processing",
		Usage: "enabling mega-bundle processing",
		Value: false,
	}
)
