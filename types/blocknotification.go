package types

import (
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"math/big"
)

// BlockNotification - represents a single block
type BlockNotification struct {
	BlockHash        ethcommon.Hash           `json:"hash,omitempty"`
	Header           *Header                  `json:"header,omitempty"`
	Transactions     []map[string]interface{} `json:"transactions,omitempty"`
	Uncles           []Header                 `json:"uncles,omitempty"`
	notificationType FeedType
	source           *NodeEndpoint
}

// Header - represents Ethereum block header
type Header struct {
	ParentHash       ethcommon.Hash     `json:"parentHash"`
	Sha3Uncles       ethcommon.Hash     `json:"sha3Uncles"`
	Miner            *ethcommon.Address `json:"miner"`
	StateRoot        ethcommon.Hash     `json:"stateRoot"`
	TransactionsRoot ethcommon.Hash     `json:"transactionsRoot"`
	ReceiptsRoot     ethcommon.Hash     `json:"receiptsRoot"`
	LogsBloom        *big.Int           `json:"logsBloom"`
	Difficulty       *big.Int           `json:"difficulty"`
	Number           *big.Int           `json:"number"`
	GasLimit         uint64             `json:"gasLimit"`
	GasUsed          uint64             `json:"gasUsed"`
	Timestamp        uint64             `json:"timestamp"`
	ExtraData        []byte             `json:"extraData"`
	MixHash          ethcommon.Hash     `json:"mixHash"`
	Nonce            uint64             `json:"nonce"`
	BaseFee          *int               `json:"baseFeePerGas,omitempty"`
}

// ConvertEthHeaderToBlockNotificationHeader converts Ethereum header to bloxroute Ethereum Header
func ConvertEthHeaderToBlockNotificationHeader(ethHeader *ethtypes.Header) *Header {
	newHeader := Header{
		ParentHash:       ethHeader.ParentHash,
		Sha3Uncles:       ethHeader.UncleHash,
		Miner:            &ethHeader.Coinbase,
		StateRoot:        ethHeader.Root,
		TransactionsRoot: ethHeader.TxHash,
		ReceiptsRoot:     ethHeader.ReceiptHash,
		LogsBloom:        ethHeader.Bloom.Big(),
		Difficulty:       ethHeader.Difficulty,
		Number:           ethHeader.Number,
		GasLimit:         ethHeader.GasLimit,
		GasUsed:          ethHeader.GasUsed,
		Timestamp:        ethHeader.Time,
		ExtraData:        ethHeader.Extra,
		MixHash:          ethHeader.MixDigest,
		Nonce:            ethHeader.Nonce.Uint64(),
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
