package types

import (
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// BlockNotification - represents a single block
type BlockNotification struct {
	BlockHash        ethcommon.Hash   `json:"hash,omitempty"`
	Header           *Header          `json:"header,omitempty"`
	Transactions     []EthTransaction `json:"transactions,omitempty"`
	Uncles           []Header         `json:"uncles,omitempty"`
	notificationType FeedType
	source           *NodeEndpoint
}

// Header - represents Ethereum block header
type Header struct {
	ParentHash       ethcommon.Hash `json:"parentHash"`
	Sha3Uncles       ethcommon.Hash `json:"sha3Uncles"`
	Miner            EthAddress     `json:"miner"`
	StateRoot        ethcommon.Hash `json:"stateRoot"`
	TransactionsRoot ethcommon.Hash `json:"transactionsRoot"`
	ReceiptsRoot     ethcommon.Hash `json:"receiptsRoot"`
	LogsBloom        EthBigInt      `json:"logsBloom"`
	Difficulty       EthBigInt      `json:"difficulty"`
	Number           EthBigInt      `json:"number"`
	GasLimit         EthUInt64      `json:"gasLimit"`
	GasUsed          EthUInt64      `json:"gasUsed"`
	Timestamp        EthUInt64      `json:"timestamp"`
	ExtraData        EthBytes       `json:"extraData"`
	MixHash          ethcommon.Hash `json:"mixHash"`
	Nonce            EthUInt64      `json:"nonce"`
	BaseFee          *int           `json:"baseFeePerGas,omitempty"`
}

// ConvertEthHeaderToBlockNotificationHeader converts Ethereum header to bloxroute Ethereum Header
func ConvertEthHeaderToBlockNotificationHeader(ethHeader *ethtypes.Header) *Header {
	newHeader := Header{
		ParentHash:       ethHeader.ParentHash,
		Sha3Uncles:       ethHeader.UncleHash,
		Miner:            EthAddress{Address: &ethHeader.Coinbase},
		StateRoot:        ethHeader.Root,
		TransactionsRoot: ethHeader.TxHash,
		ReceiptsRoot:     ethHeader.ReceiptHash,
		LogsBloom:        EthBigInt{Int: ethHeader.Bloom.Big()},
		Difficulty:       EthBigInt{Int: ethHeader.Difficulty},
		Number:           EthBigInt{Int: ethHeader.Number},
		GasLimit:         EthUInt64{UInt64: ethHeader.GasLimit},
		GasUsed:          EthUInt64{UInt64: ethHeader.GasUsed},
		Timestamp:        EthUInt64{UInt64: ethHeader.Time},
		ExtraData:        EthBytes{B: ethHeader.Extra},
		MixHash:          ethHeader.MixDigest,
		Nonce:            EthUInt64{UInt64: ethHeader.Nonce.Uint64()},
	}
	if ethHeader.BaseFee != nil {
		baseFee := int(ethHeader.BaseFee.Int64())
		newHeader.BaseFee = &baseFee
	}
	return &newHeader
}

// WithFields -
func (ethBlockNotification *BlockNotification) WithFields(fields []string) Notification {
	block := BlockNotification{}
	for _, param := range fields {
		switch param {
		case "hash":
			block.BlockHash = ethBlockNotification.BlockHash
		case "header":
			block.Header = ethBlockNotification.Header
		case "transactions":
			block.Transactions = ethBlockNotification.Transactions
		case "uncles":
			block.Uncles = ethBlockNotification.Uncles
		}
	}
	return &block
}

// Filters -
func (ethBlockNotification *BlockNotification) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (ethBlockNotification *BlockNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (ethBlockNotification *BlockNotification) GetHash() string {
	return ethBlockNotification.BlockHash.Hex()
}

// SetNotificationType - set feed name
func (ethBlockNotification *BlockNotification) SetNotificationType(feedName FeedType) {
	ethBlockNotification.notificationType = feedName
}

// NotificationType - feed name
func (ethBlockNotification *BlockNotification) NotificationType() FeedType {
	return ethBlockNotification.notificationType
}

// SetSource - source blockchain node endpoint
func (ethBlockNotification *BlockNotification) SetSource(source *NodeEndpoint) {
	ethBlockNotification.source = source
}

// Source - source blockchain node endpoint
func (ethBlockNotification *BlockNotification) Source() *NodeEndpoint {
	return ethBlockNotification.source
}
