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
	// TODO: this currently must be a file path in current code, not a URL
	RegistrationCertDirFlag = &cli.StringFlag{
		Name:   "registration-cert-dir",
		Usage:  "base URL for retrieving SSL certificates",
		Hidden: true,
	}
	DisableProfilingFlag = &cli.BoolFlag{
		Name:  "disable-profiling",
		Usage: "true to disable the pprof http server (for relays, where profiling is enabled by default)",
		Value: false,
	}
	EnableUnpaidTxsRateLimit = &cli.BoolFlag{
		Name:  "enable-unpaid-txs-rate-limit",
		Usage: "true to enable the rate limit for unpaid transactions",
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
		Name:  "auth-header",
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
	TxCheckerPoolCapacity = &cli.IntFlag{
		Name:  "tx-checker-pool-capacity",
		Usage: "for relays only, sets max number of workers that check transaction if it is valid (0 disable checks)",
		Value: 2,
	}
	NodeTypeFlag = &cli.StringFlag{
		Name:  "node-type",
		Usage: "set node type",
		Value: "external_gateway",
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
	MEVBuildersFilePathFlag = &cli.StringFlag{
		Name:   "mev-builders-file-path",
		Usage:  "set mev builders file path for gateway",
		Hidden: true,
	}
	MEVBundleMethodNameFlag = &cli.StringFlag{
		Name:  "mev-bundle-method-name",
		Usage: "set custom method for mevBundle request",
		Value: "eth_sendBundle",
	}
	SendBlockConfirmation = &cli.BoolFlag{
		Name:   "send-block-confirmation",
		Usage:  "sending block confirmation to relay",
		Value:  false,
		Hidden: true,
	}
	AuthHeaderFlag = &cli.StringFlag{
		Name:  "auth-header",
		Usage: "Authentication header for cloud services",
	}
	RPCSSLBaseURL = &cli.StringFlag{
		Name:  "rpc-ssl-base-url",
		Usage: "certs for https connection",
	}
	ETHGatewayEndpoint = &cli.StringFlag{
		Name:  "eth-source-endpoint",
		Usage: "websocket endpoint for Ethereum mainnet go-gateway",
	}
	BSCMainnetGatewayEndpoint = &cli.StringFlag{
		Name:  "bsc-source-endpoint",
		Usage: "websocket endpoint for BSC mainnet go-gateway",
	}
	PolygonMainnetGatewayEndpoint = &cli.StringFlag{
		Name:  "polygon-source-endpoint",
		Usage: "websocket endpoint for Polygon mainnet go-gateway",
	}
	PolygonMainnetHeimdallEndpoints = &cli.StringFlag{
		Name:    "polygon-heimdall-endpoints",
		Aliases: []string{"polygon-heimdall-endpoint"},
		Usage:   "tcp endpoints for Polygon mainnet heimdall server",
	}
	CheckMevCredit = &cli.BoolFlag{
		Name:  "check-mev-credit",
		Usage: "enable this flag will forward the mev rpc request to cloud api",
		Value: false,
	}
	TerminalTotalDifficulty = &cli.StringFlag{
		Name:   "terminal-total-difficulty",
		Usage:  "Overrides the terminal total difficulty settings of the blockchain network",
		Hidden: true,
	}
	EnableDynamicPeers = &cli.BoolFlag{
		Name:  "enable-dynamic-peers",
		Usage: "enable dynamic peers for gw",
		Value: false,
	}
	EnableBloomFilter = &cli.BoolFlag{
		Name:   "enable-bloom-filter",
		Usage:  "enables bloom filter for relayproxy to ignore already seen transactions",
		Value:  false,
		Hidden: true,
	}
	ForwardTransactionEndpoint = &cli.StringFlag{
		Name:  "forward-transaction-endpoint",
		Usage: "forward transaction to rpc endpoint",
		Value: "",
	}
	ForwardTransactionMethod = &cli.StringFlag{
		Name:  "forward-transaction-method",
		Usage: "calling method of the forwarding transaction to rpc endpoint",
		Value: "",
	}
	TransactionHoldDuration = &cli.IntFlag{
		Name: "transaction-hold-duration",
		Usage: "number of millisecond before next block, marking the starting window time for processing front-running protection transaction on " +
			"validator gateway",
		Value: 500,
	}
	TransactionPassedDueDuration = &cli.IntFlag{
		Name: "transaction-slot-end-duration",
		Usage: "number of millisecond before next block, marking the ending window time for processing front-running protection transaction on " +
			"validator gateway",
		Value: 200,
	}
	EnableBlockchainRPCMethodSupport = &cli.BoolFlag{
		Name:  "enable-blockchain-rpc",
		Usage: "forwards blockchain RPC methods to the node and returns node response",
		Value: false,
	}
	DialRatio = &cli.IntFlag{
		Name:   "dial-ratio",
		Usage:  "fraction of total peers that are outbound (i.e. 3 will mean 1/3 of total peers should be outbound)",
		Value:  2,
		Hidden: true,
	}
	CloudAPIAddress = &cli.StringFlag{
		Name:  "cloud-api-url",
		Usage: "setting cloudAPI url",
		Value: "https://mev.api.blxrbdn.com/",
	}
	NumRecommendedPeers = &cli.IntFlag{
		Name:   "num-recommended-peers",
		Usage:  "number of recommended peers to connect to",
		Value:  0,
		Hidden: true,
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
	BundleSimulationAuthHeader = &cli.StringFlag{
		Name:     "bundle-simulation-auth-header",
		Usage:    "provide authorisation header for cloudAPI bundle simulation",
		Required: true,
	}
	NoStats = &cli.BoolFlag{
		Name:   "no-stats",
		Usage:  "enable no-stats for not processing stats",
		Value:  false,
		Hidden: true,
	}
	BSCBundleMinAvgGasFee = &cli.IntFlag{
		Name:  "bsc-bundle-min-avg-gas-fee",
		Usage: "provide the minimum gwei gas fee needed for a BSC bundle",
		Value: 3,
	}

	BSCProposeBlocks = &cli.BoolFlag{
		Name:  "bsc-propose-blocks",
		Usage: "enable block proposing for BSC",
		Value: false,
	}
	BSCRegularBlockSendDelayInitialMSFlag = &cli.IntFlag{
		Name: "bsc-regular-block-send-delay-initial-ms",
		Usage: "How long the builder waits each block before sending initial ProposedBlocks to validators " +
			"(in milliseconds) under regular load",
		Value: DefaultRegularBlockSendDelayInitialMS,
	}
	BSCRegularBlockSendDelaySecondMSFlag = &cli.IntFlag{
		Name: "bsc-regular-block-send-delay-second-ms",
		Usage: "How long the builder waits each block before sending second block to validators " +
			"(in milliseconds) under regular load",
		Value: DefaultRegularBlockSendDelaySecondMS,
	}
	BSCRegularBlockSendDelayIntervalMSFlag = &cli.IntFlag{
		Name: "bsc-regular-block-send-delay-interval-ms",
		Usage: "How long the builder waits each block before sending block after second one periodically to validators " +
			"(in milliseconds) under regular load",
		Value: DefaultRegularBlockSendDelayIntervalMS,
	}
	BSCHighLoadBlockSendDelayInitialMSFlag = &cli.IntFlag{
		Name: "bsc-highload-block-send-delay-initial-ms",
		Usage: "How long the builder waits each block before sending initial ProposedBlocks to validators " +
			"(in milliseconds) under high load",
		Value: DefaultHighLoadBlockSendDelayInitialMS,
	}
	BSCHighLoadBlockSendDelaySecondMSFlag = &cli.IntFlag{
		Name: "bsc-highload-block-send-delay-second-ms",
		Usage: "How long the builder waits each block before sending second block to validators " +
			"(in milliseconds) under high load",
		Value: DefaultHighLoadBlockSendDelaySecondMS,
	}
	BSCHighLoadBlockSendDelayIntervalMSFlag = &cli.IntFlag{
		Name: "bsc-highload-block-send-delay-interval-ms",
		Usage: "How long the builder waits each block before sending block after second one periodically to validators " +
			"(in milliseconds) under high load",
		Value: DefaultHighLoadBlockSendDelayIntervalMS,
	}
	BSCHighLoadTxNumThresholdFlag = &cli.IntFlag{
		Name:  "bsc-highload-txnum-threshold",
		Usage: "Number of txs included in the block that indicates there is a network high load",
		Value: DefaultHighLoadTxNumThreshold,
	}

	DatabaseFlag = &cli.StringFlag{
		Name:     "dbdsn",
		Usage:    "Database DSN string <username:password@tcp(dns:port)/schema>",
		Required: true,
	}
	BloxrouteAccountsFlag = &cli.StringFlag{
		Name:  "bloxroute-accounts",
		Usage: "enable detailed bundle trace response for these accounts",
	}
	BlocksToCacheWhileProposing = &cli.Int64Flag{
		Name:   "blocks-to-cache-while-proposing",
		Usage:  "number of blocks to cache while proposing for statistics",
		Hidden: true,
		Value:  3,
	}
	TxIncludeSenderInFeed = &cli.BoolFlag{
		Name:   "tx-include-sender-in-feed",
		Usage:  "(for gateways only) include sender address in transaction feed",
		Hidden: true,
		Value:  false,
	}
	EnableIntroductoryIntentsAccess = &cli.BoolFlag{
		Name:   "enable-introductory-intents-access",
		Usage:  "enable intents access for Introductory tiers",
		Hidden: true,
		Value:  false,
	}
)

