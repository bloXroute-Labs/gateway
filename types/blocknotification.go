package types

import (
	"encoding/hex"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethpb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
)

// BellatrixBlockNotification represents bellatrix beacon block
type BellatrixBlockNotification struct {
	*ethpb.SignedBeaconBlockBellatrix

	Hash string `json:"hash"`

	notificationType FeedType      `json:"-"`
	source           *NodeEndpoint `json:"-"`
}

// WithFields -
func (beaconBlockNotification *BellatrixBlockNotification) WithFields(fields []string) Notification {
	block := BellatrixBlockNotification{}
	for _, param := range fields {
		switch param {
		case "hash":
			block.Hash = beaconBlockNotification.Hash
		case "header":
			if block.SignedBeaconBlockBellatrix == nil {
				block.SignedBeaconBlockBellatrix = &ethpb.SignedBeaconBlockBellatrix{}
			}

			if block.Block == nil {
				block.Block = &ethpb.BeaconBlockBellatrix{}
			}

			block.Block.Slot = beaconBlockNotification.GetBlock().GetSlot()
			block.Block.ProposerIndex = beaconBlockNotification.GetBlock().GetProposerIndex()
			block.Block.ParentRoot = beaconBlockNotification.GetBlock().GetParentRoot()
			block.Block.StateRoot = beaconBlockNotification.GetBlock().GetStateRoot()
		case "slot":
			if block.SignedBeaconBlockBellatrix == nil {
				block.SignedBeaconBlockBellatrix = &ethpb.SignedBeaconBlockBellatrix{}
			}

			if block.SignedBeaconBlockBellatrix.Block == nil {
				block.Block = &ethpb.BeaconBlockBellatrix{}
			}

			block.Block.Slot = beaconBlockNotification.GetBlock().GetSlot()
		case "body":
			if block.SignedBeaconBlockBellatrix == nil {
				block.SignedBeaconBlockBellatrix = &ethpb.SignedBeaconBlockBellatrix{}
			}

			if block.SignedBeaconBlockBellatrix.Block == nil {
				block.Block = &ethpb.BeaconBlockBellatrix{}
			}

			block.Block.Body = beaconBlockNotification.GetBlock().GetBody()
		}
	}

	return &block
}

// Filters -
func (beaconBlockNotification *BellatrixBlockNotification) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (beaconBlockNotification *BellatrixBlockNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (beaconBlockNotification *BellatrixBlockNotification) GetHash() string {
	return beaconBlockNotification.Hash
}

// SetNotificationType - set feed name
func (beaconBlockNotification *BellatrixBlockNotification) SetNotificationType(feedName FeedType) {
	beaconBlockNotification.notificationType = feedName
}

// NotificationType - feed name
func (beaconBlockNotification *BellatrixBlockNotification) NotificationType() FeedType {
	return beaconBlockNotification.notificationType
}

// SetSource - source blockchain node endpoint
func (beaconBlockNotification *BellatrixBlockNotification) SetSource(source *NodeEndpoint) {
	beaconBlockNotification.source = source
}

// Source - source blockchain node endpoint
func (beaconBlockNotification *BellatrixBlockNotification) Source() *NodeEndpoint {
	return beaconBlockNotification.source
}

// IsNil returns true if nil
func (beaconBlockNotification *BellatrixBlockNotification) IsNil() bool {
	return beaconBlockNotification == nil
}

// Clone clones notification
func (beaconBlockNotification *BellatrixBlockNotification) Clone() BlockNotification {
	n := *beaconBlockNotification
	return &n
}

// EthBlockNotification - represents a single block
type EthBlockNotification struct {
	BlockHash        *ethcommon.Hash          `json:"hash,omitempty"`
	Header           *Header                  `json:"header,omitempty"`
	Transactions     []map[string]interface{} `json:"transactions,omitempty"`
	Uncles           []Header                 `json:"uncles,omitempty"`
	ValidatorInfo    []*FutureValidatorInfo   `json:"future_validator_info,omitempty"`
	notificationType FeedType
	source           *NodeEndpoint
}

// FutureValidatorInfo - represents information about the validator information of the second block after the current block
type FutureValidatorInfo struct {
	BlockHeight uint64 `json:"block_height"`
	WalletID    string `json:"wallet_id"`
	Accessible  bool   `json:"accessible"`
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
func (ethBlockNotification *EthBlockNotification) WithFields(fields []string) Notification {
	block := EthBlockNotification{}
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
		case "future_validator_info":
			block.ValidatorInfo = ethBlockNotification.ValidatorInfo
		}
	}
	return &block
}

// Filters -
func (ethBlockNotification *EthBlockNotification) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (ethBlockNotification *EthBlockNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (ethBlockNotification *EthBlockNotification) GetHash() string {
	return ethBlockNotification.BlockHash.Hex()
}

// SetNotificationType - set feed name
func (ethBlockNotification *EthBlockNotification) SetNotificationType(feedName FeedType) {
	ethBlockNotification.notificationType = feedName
}

// NotificationType - feed name
func (ethBlockNotification *EthBlockNotification) NotificationType() FeedType {
	return ethBlockNotification.notificationType
}

// SetSource - source blockchain node endpoint
func (ethBlockNotification *EthBlockNotification) SetSource(source *NodeEndpoint) {
	ethBlockNotification.source = source
}

// Source - source blockchain node endpoint
func (ethBlockNotification *EthBlockNotification) Source() *NodeEndpoint {
	return ethBlockNotification.source
}

// IsNil return true if nil
func (ethBlockNotification *EthBlockNotification) IsNil() bool {
	return ethBlockNotification == nil
}

// Clone clones notification
func (ethBlockNotification *EthBlockNotification) Clone() BlockNotification {
	n := *ethBlockNotification
	return &n
}
