package eth

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"

	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
)

// ProtocolName is the official short name of the `eth` protocol used during
// devp2p capability negotiation.
const ProtocolName = "eth"

// ETH66, ETH67 are the protocols that dropped by the 'go-ethereum' which still should be supported
const (
	ETH66 = 66
	ETH67 = 67
)

const (
	// GetNodeDataMsg is the code of the GetNodeData message that was dropped by go-ethereum
	GetNodeDataMsg = 0x0d
	// NodeDataMsg is the code of the NodeData message that was dropped by go-ethereum
	NodeDataMsg = 0x0e
)

// Packet represents a p2p message in the `eth` protocol.
type Packet interface {
	Name() string // Name returns a string corresponding to the message type.
	Kind() byte   // Kind returns the message type.
}

// Custom protocol message structures that covers cases for ETH and BSC after EIP-4844

// NewBlockPacket is the network packet for the block propagation message.
type NewBlockPacket struct {
	Block    *ethtypes.Block
	TD       *big.Int
	Sidecars bxcommoneth.BlobSidecars `rlp:"optional"` // optional field for BSC
}

// Name implements the eth.Packet interface.
func (*NewBlockPacket) Name() string { return "NewBlock" }

// Kind implements the eth.Packet interface.
func (*NewBlockPacket) Kind() byte { return eth.NewBlockMsg }

// BlockBodiesResponse is the network packet for block content distribution.
type BlockBodiesResponse []*BlockBody

// BlockBodiesPacket is the network packet for block content distribution with
// request ID wrapping.
type BlockBodiesPacket struct {
	RequestID uint64
	BlockBodiesResponse
}

// BlockBody represents the data content of a single block.
type BlockBody struct {
	Transactions []*ethtypes.Transaction  // Transactions contained within a block
	Uncles       []*ethtypes.Header       // Uncles contained within a block
	Withdrawals  []*ethtypes.Withdrawal   `rlp:"optional"` // Withdrawals contained within a block
	Sidecars     bxcommoneth.BlobSidecars `rlp:"optional"` // Sidecars contained within a block
}

// Name implements the eth.Packet interface.
func (*BlockBodiesResponse) Name() string { return "BlockBodies" }

// Kind implements the eth.Packet interface.
func (*BlockBodiesResponse) Kind() byte { return eth.BlockBodiesMsg }

// Unpack retrieves the transactions and uncles from the range packet and returns
// them in a split flat format that's more consistent with the internal data structures.
func (p *BlockBodiesResponse) Unpack() ([][]*ethtypes.Transaction, [][]*ethtypes.Header, [][]*ethtypes.Withdrawal, []bxcommoneth.BlobSidecars) {
	// TODO(matt): add support for withdrawals to fetchers
	var (
		txset         = make([][]*ethtypes.Transaction, len(*p))
		uncleset      = make([][]*ethtypes.Header, len(*p))
		withdrawalset = make([][]*ethtypes.Withdrawal, len(*p))
		sidecarset    = make([]bxcommoneth.BlobSidecars, len(*p))
	)
	for i, body := range *p {
		txset[i], uncleset[i], withdrawalset[i], sidecarset[i] = body.Transactions, body.Uncles, body.Withdrawals, body.Sidecars
	}
	return txset, uncleset, withdrawalset, sidecarset
}

// end of custom protocol message structures

// NewPooledTransactionHashesPacket66 represents a transaction announcement packet on eth/66.
// Used for both eth/66 and eth/67.
type NewPooledTransactionHashesPacket66 []common.Hash

// Name implements the eth.Packet interface.
func (*NewPooledTransactionHashesPacket66) Name() string { return "NewPooledTransactionHashes" }

// Kind implements the eth.Packet interface.
func (*NewPooledTransactionHashesPacket66) Kind() byte { return eth.NewPooledTransactionHashesMsg }

// supportedProtocols is the map of networks to devp2p protocols supported by this client
var supportedProtocols = map[uint64][]uint{
	network.BSCMainnetChainID: {eth.ETH68},
	network.BSCTestnetChainID: {eth.ETH68},
	network.EthMainnetChainID: {ETH66, ETH67, eth.ETH68},
	network.HoleskyChainID:    {ETH67, eth.ETH68},
}

// protocolLengths is a mapping of each supported devp2p protocol to its message version length
var protocolLengths = map[uint]uint64{ETH66: 17, ETH67: 17, eth.ETH68: 17}

// Decoder represents any struct that can be decoded into an Ethereum message type
type Decoder interface {
	Decode(val interface{}) error
}
