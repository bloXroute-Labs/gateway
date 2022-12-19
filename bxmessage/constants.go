package bxmessage

import (
	"bytes"

	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// StartingBytesLen is the byte length of the starting bytes of bloxroute messages
const StartingBytesLen = 4

// ControlByteLen is the byte length of the control byte
const ControlByteLen = 1

// ValidControlByte is the final byte of all bloxroute messages, indicating a fully packed message
const ValidControlByte = 0x01

// HeaderLen is the byte length of the common bloxroute message headers
const HeaderLen = 20

// BroadcastHeaderLen is the byte length of the common bloxroute broadcast message headers
const BroadcastHeaderLen = 72

// PayloadSizeOffset is the byte offset of the packed message size
const PayloadSizeOffset = 16

// TypeOffset is the byte offset of the packed message type
const TypeOffset = 4

// TypeLength is the byte length of the packed message type
const TypeLength = 12

// EncryptedTypeLen is the byte length of the encrypted byte
const EncryptedTypeLen = 1

// IsBeaconLen is the byte of isBeacon byte
const IsBeaconLen = 1

// BroadcastTypeLen is the byte length of the broadcastType byte
const BroadcastTypeLen = 4

// Message type constants
const (
	HelloType                    = "hello"
	AckType                      = "ack"
	TxType                       = "tx"
	PingType                     = "ping"
	PongType                     = "pong"
	BroadcastType                = "broadcast"
	BlockTxsType                 = "blocktxs"
	TxCleanupType                = "txclnup"
	SyncTxsType                  = "txtxs"
	SyncReqType                  = "txstart"
	SyncDoneType                 = "txdone"
	DropRelayType                = "droprelay"
	RefreshBlockchainNetworkType = "blkntwrk"
	BlockConfirmationType        = "blkcnfrm"
	GetTransactionsType          = "gettxs"
	TransactionsType             = "txs"
	BDNPerformanceStatsType      = "bdnstats"
	ValidatorUpdatesType         = "validator"
	MEVBundleType                = "mevbundle"
	MEVSearcherType              = "mevsearcher"
	ErrorNotificationType        = "notify"
)

// SenderLen is the byte length of sender
const SenderLen = 20

// TimestampLen is the byte length of timestamps
const TimestampLen = 8

// ShortTimestampLen is the byte length of short timestamps
const ShortTimestampLen = 4

// SourceIDLen is the byte length of message source IDs
const SourceIDLen = 16

// ProtocolLen is the byte length of the packed message protocol version
const ProtocolLen = 4

// EmptyProtocol can be used for connections that do not track versions
const EmptyProtocol = 0

// MinProtocol provides the minimal supported protocol version
const MinProtocol = 19

// CurrentProtocol tracks the most recent version of the bloxroute wire protocol
const CurrentProtocol = NextValidatorMultipleProtocol

// NextValidatorMultipleProtocol is an enhancement to NextValidatorProtocol
const NextValidatorMultipleProtocol = 35

// BeaconBlockProtocol is the minimum protocol version that supports beacon blocks
const BeaconBlockProtocol = 34

// NextValidatorProtocol is the minimum protocol version that supports next validator
const NextValidatorProtocol = 33

// GatewayInboundConnections adds inbound connections data to BdnPerformanceStatsData.
const GatewayInboundConnections = 32

// IsConnectedToGateway adds flag to check if node is connected to the gateway in BdnPerformanceStatsData.
const IsConnectedToGateway = 31

// IsBeaconProtocol tracks if the node is beacon
const IsBeaconProtocol = 30

// MevMaxProfitBuilder send mev searcher request with request
const MevMaxProfitBuilder = 29

// ETHPosProtocol is the protocol version happened after the merge
const ETHPosProtocol = 28

// MevSearcherWithUUID send mev searcher request with request
const MevSearcherWithUUID = 27

// FlashbotsGatewayProtocol is the minimum protocol version that supports flashbots gateway without BDN
const FlashbotsGatewayProtocol = 26

// SenderProtocol is the minimum protocol version that supports sender in tx msg
const SenderProtocol = 25

// FullTxTimeStampProtocol is the minimum protocol version that supports full timestamp in TX message
// It includes BDN performance stat changes
const FullTxTimeStampProtocol = 25

// MinFastSyncProtocol is the minimum protocol version that supports fast sync
const MinFastSyncProtocol = 24

// MEVProtocol add to hello msg indication for the mev service
const MEVProtocol = 24

// UnifiedRelayProtocol is the version of gateways that expects broadcast messages from relay proxy and discontinues split relays
const UnifiedRelayProtocol = 23

// AccountProtocol is the version that the account ID was introduced in tx messages
const AccountProtocol = 22

// NullByte is a character that is packed at the end of strings in buffers
const NullByte = "\x00"

// AccountIDLen is the byte length of AccountID
const AccountIDLen = 36

// CapabilitiesLen is the byte length of the Capabilities
const CapabilitiesLen = 2

// ClientVersionLen is the byte length of the client version
const ClientVersionLen = 100

// NullByteAccountID is a null byte packed series, which represents an empty accountID
var NullByteAccountID = bytes.Repeat([]byte("\x00"), AccountIDLen)

var clock utils.Clock = utils.RealClock{}
