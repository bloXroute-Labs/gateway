package bxgateway

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// MaxConnectionBacklog in addition to the socket write buffer and remote socket read buffer.
const MaxConnectionBacklog = 100000

// AllInterfaces binds a TCP server to accept on all interfaces.
// This is typically not recommended outside of development environments.
const AllInterfaces = "0.0.0.0"

// MicroSecTimeFormat - use for representing tx "time" in feed
const MicroSecTimeFormat = "2006-01-02 15:04:05.000000"

// MillisecondsToNanosecondsMultiplier used to convert milliseconds to nanoseconds
const MillisecondsToNanosecondsMultiplier = 1000000

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

// EthFetchBlockCallRetrySleepInterval - duration of sleep between RPC call retry attempts
const EthFetchBlockCallRetrySleepInterval = 10 * time.Millisecond

// EthFetchBlockDeadlineInterval - maximum time to wait for fetch block RPC call return block
const EthFetchBlockDeadlineInterval = 2 * time.Second

// MaxEthTxReceiptCallRetries - max number of retries for eth RPC calls executed for txReceipts feed
const MaxEthTxReceiptCallRetries = 5

// EthTxReceiptCallRetrySleepInterval - duration of sleep between RPC call retry attempts for txReceipts feed
const EthTxReceiptCallRetrySleepInterval = 2 * time.Millisecond

// TaskCompletedEvent - sent as notification on onBlock feed after all RPC calls are completed
const TaskCompletedEvent = "TaskCompletedEvent"

// TaskDisabledEvent - sent as notification on onBlock feed when a RPC call is disabled due to failure
const TaskDisabledEvent = "TaskDisabledEvent"

// BDNBlocksMaxBlocksAway - gateway should not publish blocks to BDNBlocks feed that are older than the best height from node minus BDNBlocksMaxBlocksAway
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

const (
	// BloxrouteBuilderName - set bloxroute mev builder name
	BloxrouteBuilderName = "bloxroute"

	// FlashbotsBuilderName - set flashbots mev builder name
	FlashbotsBuilderName = "flashbots"

	// AllBuilderName - set all other external mev builders name
	AllBuilderName = "all"

	// External0x69BuilderName - set builder0x69 external mev builders name
	External0x69BuilderName = "builder0x69"

	// ExternalBeaverBuilderName - set beaverbuild external mev builders name
	ExternalBeaverBuilderName = "beaverbuild"

	// ExternalEdenBuilderName - set eden external mev builders name
	ExternalEdenBuilderName = "eden"

	// ExternalBlocknativeBuilderName - set blocknative external mev builders name
	ExternalBlocknativeBuilderName = "blocknative"
)

// Mainnet - for Ethereum main net blockchain network name
const Mainnet = "Mainnet"

// BSCMainnet - for BSC main net blockchain network name
const BSCMainnet = "BSC-Mainnet"

// BSCTestnet - for BSC testnet blockchain network name
const BSCTestnet = "BSC-Testnet"

// Ropsten - for Ropsten blockchain network name
const Ropsten = "Ropsten"

// Zhejiang - for Zhejiang blockchain network name
const Zhejiang = "Zhejiang"

// Goerli - for Goerli blockchain network name
const Goerli = "Goerli"

// PolygonMainnet - for Polygon main net blockchain network name
const PolygonMainnet = "Polygon-Mainnet"

// MainnetNum - for Ethereum main net blockchain network number
const MainnetNum types.NetworkNum = 5

// BSCMainnetNum - for BSC main net blockchain network number
const BSCMainnetNum types.NetworkNum = 10

// BSCChainID - BSC chain ID
const BSCChainID = 56

// EthChainID - eth chain ID
const EthChainID types.NetworkID = 1

// PolygonChainID - polygon chain ID
const PolygonChainID types.NetworkID = 137

// BSCTestnetChainID - BSC Testnet chain ID
const BSCTestnetChainID = 97

// PolygonMainnetNum - for Polygon main net blockchain network number
const PolygonMainnetNum types.NetworkNum = 36

// RopstenNum - for Ropsten blockchain network number
const RopstenNum types.NetworkNum = 32

// GoerliNum - for Goerli blockchain network number
const GoerliNum types.NetworkNum = 45

// BSCTestnetNum - for BSC-Testnet blockchain network number
const BSCTestnetNum types.NetworkNum = 42

// BlockchainNetworkToNetworkNum converts blockchain network to number
var BlockchainNetworkToNetworkNum = map[string]types.NetworkNum{
	Mainnet:        MainnetNum,
	BSCMainnet:     BSCMainnetNum,
	PolygonMainnet: PolygonMainnetNum,
	Ropsten:        RopstenNum,
	Goerli:         GoerliNum,
	BSCTestnet:     BSCTestnetNum,
}

// NetworkToBlockDuration defines block interval for each network
var NetworkToBlockDuration = map[string]time.Duration{
	Mainnet:        12 * time.Second,
	BSCMainnet:     3 * time.Second,
	BSCTestnet:     3 * time.Second,
	PolygonMainnet: 2 * time.Second,
}

// NetworkToChainID - Mapping from network to chainID
var NetworkToChainID = map[string]types.NetworkID{
	Mainnet:        EthChainID,
	BSCMainnet:     BSCChainID,
	PolygonMainnet: PolygonChainID,
}
