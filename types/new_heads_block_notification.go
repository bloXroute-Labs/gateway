package types

import (
	ethcommon "github.com/ethereum/go-ethereum/common"
)

// NewHeadsBlock - represents newHeads feed notification for eth_subscribe newHeads feed
type NewHeadsBlock struct {
	BlockHash        *ethcommon.Hash    `json:"hash,omitempty"`
	Number           string             `json:"number,omitempty"`
	ParentHash       ethcommon.Hash     `json:"parentHash,omitempty"`
	Nonce            string             `json:"nonce,omitempty"`
	Sha3Uncles       ethcommon.Hash     `json:"sha3Uncles,omitempty"`
	LogsBloom        string             `json:"logsBloom,omitempty"`
	TransactionsRoot ethcommon.Hash     `json:"transactionsRoot,omitempty"`
	StateRoot        ethcommon.Hash     `json:"stateRoot,omitempty"`
	ReceiptsRoot     ethcommon.Hash     `json:"receiptsRoot,omitempty"`
	Miner            *ethcommon.Address `json:"miner,omitempty"`
	Difficulty       string             `json:"difficulty,omitempty"`
	ExtraData        string             `json:"extraData,omitempty"`
	GasLimit         string             `json:"gasLimit,omitempty"`
	GasUsed          string             `json:"gasUsed,omitempty"`
	Timestamp        string             `json:"timestamp,omitempty"`

	notificationType FeedType
	source           *NodeEndpoint
}

// NewHeadsBlockFromEthBlockNotification - convert a EthBlockNotification to NewHeadsBlock
func NewHeadsBlockFromEthBlockNotification(blockNotification *EthBlockNotification) *NewHeadsBlock {
	return &NewHeadsBlock{
		BlockHash:        blockNotification.BlockHash,
		Number:           blockNotification.Header.Number,
		ParentHash:       blockNotification.Header.ParentHash,
		Nonce:            blockNotification.Header.Nonce,
		Sha3Uncles:       blockNotification.Header.Sha3Uncles,
		LogsBloom:        blockNotification.Header.LogsBloom,
		TransactionsRoot: blockNotification.Header.TransactionsRoot,
		StateRoot:        blockNotification.Header.StateRoot,
		ReceiptsRoot:     blockNotification.Header.ReceiptsRoot,
		Miner:            blockNotification.Header.Miner,
		Difficulty:       blockNotification.Header.Difficulty,
		ExtraData:        blockNotification.Header.ExtraData,
		GasLimit:         blockNotification.Header.GasLimit,
		GasUsed:          blockNotification.Header.GasUsed,
		Timestamp:        blockNotification.Header.Timestamp,
		notificationType: blockNotification.notificationType,
		source:           blockNotification.source,
	}
}

// WithFields returns notification with specified fields
func (newHeadsBlock *NewHeadsBlock) WithFields(fields []string) Notification {
	return newHeadsBlock
}

// Filters converts filters as field value map
func (newHeadsBlock *NewHeadsBlock) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (newHeadsBlock *NewHeadsBlock) LocalRegion() bool {
	return false
}

// GetHash returns block hash
func (newHeadsBlock *NewHeadsBlock) GetHash() string {
	return newHeadsBlock.BlockHash.Hex()
}

// SetNotificationType - set feed name
func (newHeadsBlock *NewHeadsBlock) SetNotificationType(feedName FeedType) {
	newHeadsBlock.notificationType = feedName
}

// NotificationType - feed name
func (newHeadsBlock *NewHeadsBlock) NotificationType() FeedType {
	return newHeadsBlock.notificationType
}

// SetSource - source blockchain node endpoint
func (newHeadsBlock *NewHeadsBlock) SetSource(source *NodeEndpoint) {
	newHeadsBlock.source = source
}

// Source - source blockchain node endpoint
func (newHeadsBlock *NewHeadsBlock) Source() *NodeEndpoint {
	return newHeadsBlock.source
}

// IsNil return true if nil
func (newHeadsBlock *NewHeadsBlock) IsNil() bool {
	return newHeadsBlock == nil
}

// Clone clones notification
func (newHeadsBlock *NewHeadsBlock) Clone() BlockNotification {
	n := *newHeadsBlock
	return &n
}
