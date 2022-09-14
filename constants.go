package bxgateway

import "time"

// MaxConnectionBacklog in addition to the socket write buffer and remote socket read buffer.
const MaxConnectionBacklog = 100000

// AllInterfaces binds a TCP server to accept on all interfaces.
// This is typically not recommended outside of development environments.
const AllInterfaces = "0.0.0.0"

// MicroSecTimeFormat - use for representing tx "time" in feed
const MicroSecTimeFormat = "2006-01-02 15:04:05.000000"

// SlowPingPong - ping/pong delay above it is considered a problem
const SlowPingPong = int64(100000) // 100 ms

// SyncChunkSize - maximum SYNC message size for gateway
const SyncChunkSize = 500 * 1024

// TxStoreMaxSize - If number of Txs in TxStore is above TxStoreMaxSize cleanup will bring it back to TxStoreMaxSize (per network)
const TxStoreMaxSize = 200000

// BlockRecoveryTimeout - max time to wait for block recovery before canceling block
const BlockRecoveryTimeout = 10 * time.Second

// ConnectionDisabledDuration - the duration for which an invalid connection is disabled before closing
const ConnectionDisabledDuration = 15 * time.Minute

// Ethereum - string representation for the Ethereum protocol
const Ethereum = "Ethereum"

// TimeDateLayoutISO - used to parse ISO time date format string
const TimeDateLayoutISO = "2006-01-02"

// TimeLayoutISO - used to parse ISO time format string
const TimeLayoutISO = "2006-01-02 15:04:05-0700"

// ExpiredDate - constant for an expired date
const ExpiredDate = "1970-01-01"

// AsyncMsgChannelSize - size of async message channel
const AsyncMsgChannelSize = 500

// BxNotificationChannelSize - is the size of feed channels
const BxNotificationChannelSize = 1000

// MaxEthOnBlockCallRetries - max number of retries for eth RPC calls executed for onBlock feed
const MaxEthOnBlockCallRetries = 2

// EthOnBlockCallRetrySleepInterval - duration of sleep between RPC call retry attempts
const EthOnBlockCallRetrySleepInterval = 10 * time.Millisecond

// MaxEthTxReceiptCallRetries - max number of retries for eth RPC calls executed for txReceipts feed
const MaxEthTxReceiptCallRetries = 5

// EthTxReceiptCallRetrySleepInterval - duration of sleep between RPC call retry attempts for txReceipts feed
const EthTxReceiptCallRetrySleepInterval = 2 * time.Millisecond

// TaskCompletedEvent - sent as notification on onBlock feed after all RPC calls are completed
const TaskCompletedEvent = "TaskCompletedEvent"

// TaskDisabledEvent - sent as notification on onBlock feed when a RPC call is disabled due to failure
const TaskDisabledEvent = "TaskDisabledEvent"

// BDNBlocksMaxBlocksAway - gateway should not publish blocks to BDNBlocks feed that are older than best height from node minus BDNBlocksMaxBlocksAway
const BDNBlocksMaxBlocksAway = 50

// MaxOldBDNBlocksToSkipPublish is the max number of blocks beyond BDNBlocksMaxBlocksAway to skip publishing to BDNBlocks feed
const MaxOldBDNBlocksToSkipPublish = 3

// CleanedShortIDsChannelSize is the size of cleaned short ids channel
const CleanedShortIDsChannelSize = 100

// WSConnectionID - special node ID to identify the websocket connection
const WSConnectionID = "WSConnectionID"

// DefaultRoutingConfigFileName - routingConfig cache file name
const DefaultRoutingConfigFileName = "defaultRoutingConfig.json"

// MaxAnnouncementFromNode restrict the size of the announcment message from the node
const MaxAnnouncementFromNode = 100

// GetMethod - get method for http
const GetMethod = "GET"

// PostMethod - post method for http
const PostMethod = "POST"

// TimeToWaitBeforeClosing - set time to wait before closing the connection
const TimeToWaitBeforeClosing = 500 * time.Millisecond

// WSProviderTimeout - sets timeout duration used by WSProvider
const WSProviderTimeout = 10 * time.Second

// SDNAccountRequestTimeout - duration after which SDN account requests are deleted if no response received
const SDNAccountRequestTimeout = time.Minute * 2

const (
	// InternalError - status Code for an unexpected condition was encountered
	InternalError = 500
	// NotImplemented - Status for the server does not recognize the request method
	NotImplemented = 501
	// BadGateway - status Code for a gateway which did not receive a timely response from the server
	BadGateway = 502
	// ServiceUnavailable - status Code for server unavailable on sdn
	ServiceUnavailable = 503
	// GatewayTimeout - status Code for invalid response from server
	GatewayTimeout = 504
)

// BloxrouteBuilderName - set bloxroute mev builder name
const BloxrouteBuilderName = "bloxroute"

// FlashbotsBuilderName - set flashbots mev builder name
const FlashbotsBuilderName = "flashbots"

// AllBuilderName - set all other external mev builders name
const AllBuilderName = "all"

// Mainnet - for Ethereum main net blockchain network name
const Mainnet = "Mainnet"

// BSCMainnet - for BSC main net blockchain network name
const BSCMainnet = "BSC-Mainnet"

// Ropsten - for Ropsten blockchain network name
const Ropsten = "Ropsten"

// PolygonMainnet - for Polygon main net blockchain network name
const PolygonMainnet = "Polygon-Mainnet"

//MainnetNum - for Ethereum main net blockchain network number
const MainnetNum = 5

//BSCMainnetNum - for BSC main net blockchain network number
const BSCMainnetNum = 10

// RopstenNum - for Ropsten blockchain network number
const RopstenNum = 32

//PolygonMainnetNum - for Polygon main net blockchain network number
const PolygonMainnetNum = 36
