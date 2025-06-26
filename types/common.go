package types

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/syncmap"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
)

// UInt32Len is the byte length of unsigned 32bit integers
const UInt32Len = 4

// UInt64Len is the byte length of unsigned 64bit integers
const UInt64Len = 8

// UInt16Len is the byte length of unsigned 16bit integers
const UInt16Len = 2

// UInt8Len is the byte length of unsigned 8bit integers
const UInt8Len = 1

// TxFlagsLen represents the byte length of transaction flag
const TxFlagsLen = 2

// WalletIDLen represents the bytes length of the wallet id
const WalletIDLen = 42

// NodeEndpoint - represent the node endpoint struct sent in BxStatus
type NodeEndpoint struct {
	IP                string
	DNS               string
	Port              int
	PublicKey         string
	IsBeacon          bool
	BlockchainNetwork string
	Dynamic           bool
	ID                string
	Version           int
	Name              string
	ConnectedAt       string
	ConnectionType    string
}

// String returns string representation of NodeEndpoint
func (e NodeEndpoint) String() string {
	if e.DNS != "" {
		return fmt.Sprintf("%v %v %v", e.DNS, e.Port, e.PublicKey)
	}
	return fmt.Sprintf("%v %v %v", e.IP, e.Port, e.PublicKey)
}

// IPPort returns string of IP and Port
func (e NodeEndpoint) IPPort() string {
	return fmt.Sprintf("%v %v", e.IP, e.Port)
}

// IsDynamic return true if endpoint is inbound
func (e NodeEndpoint) IsDynamic() bool {
	return e.Dynamic
}

// ShortID represents the compressed transaction ID
type ShortID uint32

// ShortIDList represents short id list
type ShortIDList []ShortID

// ShortIDsByNetwork represents map of shortIDs by network
type ShortIDsByNetwork map[bxtypes.NetworkNum]ShortIDList

// Sender represents sender type
type Sender [20]byte

// EmptySender represents empty sender
var EmptySender = [20]byte{}

// String returns string of the Sender
func (s Sender) String() string {
	return hex.EncodeToString(s[:])
}

// Bytes gets the string representation of the underlying Sender.
func (s Sender) Bytes() []byte {
	return s[:]
}

// EmptyAccountID represent no Account ID set
const EmptyAccountID bxtypes.AccountID = ""

// NewAccountID constructs an accountID from bytes, stripping off null bytes.
func NewAccountID(b []byte) bxtypes.AccountID {
	trimmed := bytes.Trim(b, "\x00")
	return bxtypes.AccountID(trimmed)
}

// NodeIDLen is the number of characters in a NodeID
const NodeIDLen = 36

// ShortIDEmpty is the default value indicating no assigned short ID
const ShortIDEmpty = 0

// ShortIDLen is the byte length of packed short IDs
const ShortIDLen = UInt32Len

// BloxrouteGoGateway is initiated in gateway node model for the field: name
const BloxrouteGoGateway = "bloxroute go gateway"

// AllNetworkNum is the network number for relays that facilitate transactions from all networks
const AllNetworkNum bxtypes.NetworkNum = 0

// NetworkNumLen is the byte length of packed network numbers
const NetworkNumLen = UInt32Len

// ErrorNotificationCodeLen represents len of code
const ErrorNotificationCodeLen = 4

// UUIDv4Len is the byte length of UUID V4
const UUIDv4Len = 16

// RelayMonitorInterval is interval for relay monitor
const RelayMonitorInterval = time.Minute

// OFACMap represent ofac map
type OFACMap = syncmap.SyncMap[string, bool]
