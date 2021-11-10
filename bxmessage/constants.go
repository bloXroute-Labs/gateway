package bxmessage

import (
	"bytes"
)

// StartingBytesLen is the byte length of the starting bytes of bloxroute messages
const StartingBytesLen = 4

// ControlByteLen is the byte length of the control byte
const ControlByteLen = 1

// ControlByte is the final byte of all bloxroute messages, indicating a fully packed message
const ControlByte = 0x01

// HeaderLen is the byte length of the common bloxroute message headers
const HeaderLen = 20

// PayloadSizeOffset is the byte offset of the packed message size
const PayloadSizeOffset = 16

// TypeOffset is the byte offset of the packed message type
const TypeOffset = 4

// TypeLength is the byte length of the packed message type
const TypeLength = 12

// EncryptedTypeLen is the byte length of the encrypted byte
const EncryptedTypeLen = 1

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
	BlockConfirmationTYpe        = "blkcnfrm"
	GetTransactionsType          = "gettxs"
	TransactionsType             = "txs"
	BDNPerformanceStatsType      = "bdnstats"
)

// TimestampLen is the byte length of timestamps
const TimestampLen = 8

// SourceIDLen is the byte length of message source IDs
const SourceIDLen = 16

// ProtocolLen is the byte length of the packed message protocol version
const ProtocolLen = 4

// EmptyProtocol can be used for connections that do not track versions
const EmptyProtocol = 0

// MinProtocol provides the minimal supported protocol version
const MinProtocol = 19

// CurrentProtocol tracks the most recent version of the bloxroute wire protocol
const CurrentProtocol = 23

// UnifiedRelayProtocol is the version of gateways that expects broadcast messages from relay proxy and discontinues split relays
const UnifiedRelayProtocol = 23

// AccountProtocol is the version that the account ID was introduced in tx messages
const AccountProtocol = 22

// NullByte is a character that is packed at the end of strings in buffers
const NullByte = "\x00"

// AccountIDLen is the byte length of AccountID
const AccountIDLen = 36

// NullByteAccountID is a null byte packed series, which represents an empty accountID
var NullByteAccountID = bytes.Repeat([]byte("\x00"), AccountIDLen)
