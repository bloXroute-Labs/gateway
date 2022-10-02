package types

import (
	"encoding/hex"
	"fmt"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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
	LogsBloom        string             `json:"logsBloom"`
	Difficulty       string             `json:"difficulty"`
	Number           string             `json:"number"`
	GasLimit         string             `json:"gasLimit"`
	GasUsed          string             `json:"gasUsed"`
	Timestamp        string             `json:"timestamp"`
	ExtraData        string             `json:"extraData"`
	MixHash          ethcommon.Hash     `json:"mixHash"`
	Nonce            string             `json:"nonce"`
	BaseFee          *int               `json:"baseFeePerGas,omitempty"`
	hexNumber        uint64
}

// GetNumber returns the block number from the header in uint64
func (h Header) GetNumber() uint64 {
	return h.hexNumber
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
		LogsBloom:        fmt.Sprintf("0x%x", hex.EncodeToString(ethHeader.Bloom.Bytes())),
		Difficulty:       hexutil.EncodeBig(ethHeader.Difficulty),
		hexNumber:        ethHeader.Number.Uint64(),
		Number:           hexutil.EncodeBig(ethHeader.Number),
		GasLimit:         hexutil.EncodeUint64(ethHeader.GasLimit),
		GasUsed:          hexutil.EncodeUint64(ethHeader.GasUsed),
		Timestamp:        hexutil.EncodeUint64(ethHeader.Time),
		ExtraData:        hexutil.Encode(ethHeader.Extra),
		MixHash:          ethHeader.MixDigest,
		Nonce:            hexutil.EncodeUint64(ethHeader.Nonce.Uint64()),
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
