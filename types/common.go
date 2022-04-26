package types

import (
	"bytes"
	"encoding/hex"
	"fmt"
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

// NodeEndpoint - represent the node endpoint struct sent in BdnPerformanceStats
type NodeEndpoint struct {
	IP        string
	Port      int
	PublicKey string
}

// String returns string representation of NodeEndpoint
func (e NodeEndpoint) String() string {
	return fmt.Sprintf("%v %v %v", e.IP, e.Port, e.PublicKey)
}

// IPPort returns string of IP and Port
func (e NodeEndpoint) IPPort() string {
	return fmt.Sprintf("%v %v", e.IP, e.Port)
}

// ShortID represents the compressed transaction ID
type ShortID uint32

// ShortIDList represents short id list
type ShortIDList []ShortID

// ShortIDsByNetwork represents map of shortIDs by network
type ShortIDsByNetwork map[NetworkNum]ShortIDList

// NodeID represents a node's assigned ID. This field is a UUID.
type NodeID string

// AccountID represents a user's BDN account. This field is a UUID.
type AccountID string

// Sender represents sender type
type Sender [20]byte

// String returns string of the Sender
func (s Sender) String() string {
	return hex.EncodeToString(s[:])
}

// EmptyAccountID represent no Account ID set
const EmptyAccountID AccountID = ""

// NewAccountID constructs an accountID from bytes, stripping off null bytes.
func NewAccountID(b []byte) AccountID {
	trimmed := bytes.Trim(b, "\x00")
	return AccountID(trimmed)
}

// NodeIDLen is the number of characters in a NodeID
const NodeIDLen = 36

// ShortIDEmpty is the default value indicating no assigned short ID
const ShortIDEmpty = 0

// ShortIDLen is the byte length of packed short IDs
const ShortIDLen = UInt32Len

// NetworkNum represents the network that a message is being routed in (Ethereum Mainnet, Ethereum Ropsten, etc.)
type NetworkNum uint32

// NetworkNumLen is the byte length of packed network numbers
const NetworkNumLen = UInt32Len

// BloxrouteAccountID marks an internally generated certificate (e.g. for relays / internal gateways)
const BloxrouteAccountID = "bloXroute LABS"

// BloxrouteGoGateway is initiated in gateway node model for the field: name
const BloxrouteGoGateway = "bloxroute go gateway"

// GoGatewayVersion is version of gateway
const GoGatewayVersion = "2.0.1"

// AllNetworkNum is the network number for relays that facilitate transactions from all networks
const AllNetworkNum NetworkNum = 0

// ErrorNotificationCodeLen represents len of code
const ErrorNotificationCodeLen = 4
