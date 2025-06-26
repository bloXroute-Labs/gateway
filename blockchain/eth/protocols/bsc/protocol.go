package bsc

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
)

const (
	// ProtocolName is the official short name of the `bsc` protocol used during
	// devp2p capability negotiation.
	ProtocolName = "bsc"

	// Bsc1 represents the basic version of the `bsc` protocol.
	Bsc1 = 1
	// Bsc2 represents the second version of the `bsc` protocol with additional supported messages.
	Bsc2 = 2

	// BscCapMsg is the message code for the bsc capability message used upon handshake.
	BscCapMsg = 0x00
	// VotesMsg is the message code for votes, which is not relevant for gateway.
	VotesMsg = 0x01
	// GetBlocksByRangeMsg is the message code for requesting a range of blocks from a remote peer.
	// It can request (StartBlockHeight-Count, StartBlockHeight] range blocks from remote peer
	GetBlocksByRangeMsg = 0x02 //
	// BlocksByRangeMsg is the message code for the replied blocks from a remote peer.
	BlocksByRangeMsg = 0x03

	// maxMessageSize is the maximum cap on the size of a protocol message.
	maxMessageSize = 10 * 1024 * 1024
)

// ProtocolVersions are the supported versions of the `bsc` protocol (first
// is primary).
var ProtocolVersions = []uint{Bsc1, Bsc2}

// protocolLengths are the number of implemented messages corresponding to
// different protocol versions.
var protocolLengths = map[uint]uint64{Bsc1: 2, Bsc2: 4}

var defaultExtra = []byte{0x00}

var (
	errNoBscCapMsg             = errors.New("no bsc capability message")
	errMsgTooLarge             = errors.New("message too long")
	errDecode                  = errors.New("invalid message")
	errInvalidMsgCode          = errors.New("invalid message code")
	errProtocolVersionMismatch = errors.New("protocol version mismatch")
)

// Packet represents a p2p message in the `bsc` protocol.
type Packet interface {
	Name() string // Name returns a string corresponding to the message type.
	Kind() byte   // Kind returns the message type.
}

// CapPacket is the network packet for bsc capability message.
type CapPacket struct {
	ProtocolVersion uint
	Extra           rlp.RawValue // for extension
}

// Name implements the Packet interface for CapPacket.
func (*CapPacket) Name() string { return "BscCap" }

// Kind implements the Packet interface for CapPacket.
func (*CapPacket) Kind() byte { return BscCapMsg }

// GetBlocksByRangePacket is the network packet for requesting a range of blocks from a remote peer.
type GetBlocksByRangePacket struct {
	RequestId        uint64      //nolint
	StartBlockHeight uint64      // The start block height expected to be obtained from
	StartBlockHash   common.Hash // The start block hash expected to be obtained from
	Count            uint64      // Get the number of blocks from the start
}

// Name implements the Packet interface for GetBlocksByRangePacket.
func (*GetBlocksByRangePacket) Name() string { return "GetBlocksByRange" }

// Kind implements the Packet interface for GetBlocksByRangePacket.
func (*GetBlocksByRangePacket) Kind() byte { return GetBlocksByRangeMsg }

// BlockData contains types.extblock + sidecars
type BlockData struct {
	Header      *types.Header
	Txs         []*types.Transaction     // Transactions contained within a block
	Uncles      []*types.Header          // Uncles contained within a block
	Withdrawals []*types.Withdrawal      `rlp:"optional"` // Withdrawals contained within a block
	Sidecars    bxcommoneth.BlobSidecars `rlp:"optional"` // Sidecars contained within a block
}

// BlocksByRangePacket is the network packet for the replied blocks from a remote peer.
type BlocksByRangePacket struct {
	RequestId uint64 //nolint
	Blocks    []BlockData
}

// Name implements the Packet interface for BlocksByRangePacket.
func (*BlocksByRangePacket) Name() string { return "BlocksByRange" }

// Kind implements the Packet interface for BlocksByRangePacket.
func (*BlocksByRangePacket) Kind() byte { return BlocksByRangeMsg }
