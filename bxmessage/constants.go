package bxmessage

import (
	"bytes"

	"github.com/bloXroute-Labs/gateway/v2/types"
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

// BroadcastHeaderOffset is the byte offset of the common bloxroute broadcast message headers
const BroadcastHeaderOffset = HeaderLen + types.SHA256HashLen + types.NetworkNumLen + SourceIDLen

// BroadcastHeaderLen is the byte len of the common bloxroute broadcast message headers
const BroadcastHeaderLen = BroadcastHeaderOffset + ControlByteLen

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

// BeaconMessageTypeLen is the byte length of the beacon message type
const BeaconMessageTypeLen = 4

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
	ErrorNotificationType        = "notify"
	IntentType                   = "intent"
	IntentSolutionType           = "intentsol"
	IntentsSubscriptionType      = "intentssub"
	IntentsUnsubscriptionType    = "intentsunsub"
	SolutionsSubscriptionType    = "solssub"
	SolutionsUnsubscriptionType  = "solsunsub"
	BeaconMessageType            = "beaconmsg"
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
const MinProtocol = IsBeaconProtocol

// CurrentProtocol tracks the most recent version of the bloxroute wire protocol
const CurrentProtocol = BundleRefundProtocol

// BundleRefundProtocol is the minimum protocol version that supports bundle refund recipient address
const BundleRefundProtocol = 48

// IntentSolutionProtocol is the minimum protocol version that supports dApp address in intent solutions
const IntentSolutionProtocol = 47

// BlobCompressionProtocol is the minimum protocol version that supports blob compression
const BlobCompressionProtocol = 46

// BundlePriorityFeeRefundProtocol is the minimum protocol version that supports bundle priority fee refund
const BundlePriorityFeeRefundProtocol = 45

// BeaconMessagesProtocol is the minimum protocol version that supports beacon messages
const BeaconMessagesProtocol = 44

// AvoidMixedBundleProtocol is the minimum protocol version that supports avoiding mixed bundles
const AvoidMixedBundleProtocol = 43

// IntentsWithAnySenderProtocol is the minimum protocol version that supports Intents with any intent sender
const IntentsWithAnySenderProtocol = 42

// BundlesOverBDNOriginalSenderTierProtocol is the minimum protocol version that supports bundles over BDN with original tier
const BundlesOverBDNOriginalSenderTierProtocol = 41

// BundlesOverBDNOriginalSenderAccountProtocol is the minimum protocol version that supports bundles over BDN with original sender account
const BundlesOverBDNOriginalSenderAccountProtocol = 40

// IntentsProtocol is the minimum protocol version that supports Intents
const IntentsProtocol = 39

// BundlesOverBDNPayoutProtocol is the minimum protocol version that supports bundles over BDN with payout
const BundlesOverBDNPayoutProtocol = 38

// BundlesOverBDNProtocol is the minimum protocol version that supports bundles over BDN
const BundlesOverBDNProtocol = 37

// ShanghaiProtocol is the minimum protocol version that supports Capella blocks
const ShanghaiProtocol = 36

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

// UUIDv4Len is the byte length of UUID V4
const UUIDv4Len = 16

// ETHAddressLen is the byte length of ETH Address
const ETHAddressLen = 20

// Keccak256HashLen is the byte length of Keccak256Hash
const Keccak256HashLen = 32

// ECDSASignatureLen is the byte length of ECDSASignature in Ethereum ecosystem
const ECDSASignatureLen = 65
