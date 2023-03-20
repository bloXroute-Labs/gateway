package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// BxBlockType is block type
type BxBlockType int

// block types of BxBlock
const (
	BxBlockTypeUnknown BxBlockType = iota
	BxBlockTypeEth
	BxBlockTypeBeaconPhase0
	BxBlockTypeBeaconAltair
	BxBlockTypeBeaconBellatrix
	BxBlockTypeBeaconCapella
)

// String implements Stringer interface
func (t BxBlockType) String() string {
	switch t {
	case BxBlockTypeEth:
		return "eth"
	case BxBlockTypeBeaconPhase0:
		return "phase0"
	case BxBlockTypeBeaconAltair:
		return "altair"
	case BxBlockTypeBeaconBellatrix:
		return "bellatrix"
	case BxBlockTypeBeaconCapella:
		return "capella"
	default:
		return ""
	}
}

// BxBlockTransaction represents a tx in the BxBlock.
type BxBlockTransaction struct {
	hash    SHA256Hash
	content []byte
}

// NewBxBlockTransaction creates a new tx in the BxBlock. This transaction is usable for compression.
func NewBxBlockTransaction(hash SHA256Hash, content []byte) *BxBlockTransaction {
	return &BxBlockTransaction{
		hash:    hash,
		content: content,
	}
}

// NewRawBxBlockTransaction creates a new transaction that's not ready for compression. This should only be used when parsing the result of an existing BxBlock.
func NewRawBxBlockTransaction(content []byte) *BxBlockTransaction {
	return &BxBlockTransaction{
		content: content,
	}
}

// Hash returns the transaction hash
func (b BxBlockTransaction) Hash() SHA256Hash {
	return b.hash
}

// Content returns the transaction bytes
func (b BxBlockTransaction) Content() []byte {
	return b.content
}

// BxBlock represents an encoded block ready for compression or decompression
type BxBlock struct {
	hash            SHA256Hash
	beaconHash      SHA256Hash
	Type            BxBlockType
	Header          []byte
	Txs             []*BxBlockTransaction
	Trailer         []byte
	TotalDifficulty *big.Int
	Number          *big.Int
	timestamp       time.Time
	size            int
}

// NewBxBlock creates a new BxBlock that's ready for compression. This means that all transaction hashes must be included.
func NewBxBlock(hash, beaconHash SHA256Hash, bType BxBlockType, header []byte, txs []*BxBlockTransaction, trailer []byte, totalDifficulty *big.Int, number *big.Int, size int) (*BxBlock, error) {
	for _, tx := range txs {
		if tx.Hash() == (SHA256Hash{}) {
			return nil, errors.New("all transactions must contain hashes")
		}
	}
	return NewRawBxBlock(hash, beaconHash, bType, header, txs, trailer, totalDifficulty, number, size), nil
}

// NewRawBxBlock create a new BxBlock without compression restrictions. This should only be used when parsing the result of an existing BxBlock.
func NewRawBxBlock(hash, beaconHash SHA256Hash, bType BxBlockType, header []byte, txs []*BxBlockTransaction, trailer []byte, totalDifficulty *big.Int, number *big.Int, size int) *BxBlock {
	bxBlock := &BxBlock{
		hash:            hash,
		beaconHash:      beaconHash,
		Type:            bType,
		Header:          header,
		Txs:             txs,
		Trailer:         trailer,
		TotalDifficulty: totalDifficulty,
		Number:          number,
		timestamp:       time.Now(),
		size:            size,
	}
	return bxBlock
}

// String implements Stringer interface
func (b BxBlock) String() string {
	if b.IsBeaconBlock() {
		return fmt.Sprintf("block beacon(hash: %s, beacon hash: %s, type: %s, number: %d, txs: %d)", b.hash, b.beaconHash, b.Type, b.Number.Uint64(), len(b.Txs))
	}

	return fmt.Sprintf("block(hash: %s, type: %s, number: %d, txs: %d)", b.hash, b.Type, b.Number.Uint64(), len(b.Txs))
}

// IsBeaconBlock returns true if block is beacon
func (b *BxBlock) IsBeaconBlock() bool {
	switch b.Type {
	case BxBlockTypeBeaconPhase0, BxBlockTypeBeaconAltair, BxBlockTypeBeaconBellatrix, BxBlockTypeBeaconCapella:
		return true
	default:
		return false
	}
}

// Serialize returns an expensive string representation of the BxBlock
func (b BxBlock) Serialize() string {
	m := make(map[string]interface{})
	m["header"] = hex.EncodeToString(b.Header)
	m["trailer"] = hex.EncodeToString(b.Trailer)
	m["totalDifficulty"] = b.TotalDifficulty.String()
	m["number"] = b.Number.String()

	txs := make([]string, 0, len(b.Txs))
	for _, tx := range b.Txs {
		txs = append(txs, hex.EncodeToString(tx.content))
	}
	m["txs"] = txs

	jsonBytes, _ := json.Marshal(m)
	return string(jsonBytes)
}

// Hash returns block hash
func (b BxBlock) Hash() SHA256Hash {
	return b.hash
}

// BeaconHash returns beacon hash
func (b BxBlock) BeaconHash() SHA256Hash {
	return b.beaconHash
}

// Timestamp returns block add time
func (b BxBlock) Timestamp() time.Time {
	return b.timestamp
}

// Size returns the original blockchain block
func (b BxBlock) Size() int {
	return b.size
}

// SetSize sets the original blockchain block
func (b *BxBlock) SetSize(size int) {
	b.size = size
}

// Equals checks the byte contents of each part of the provided BxBlock. Note that some fields are set throughout the object's lifecycle (bx block hash, transaction hash), so these fields are not checked for equality.
func (b *BxBlock) Equals(other *BxBlock) bool {
	if !bytes.Equal(b.Header, other.Header) || !bytes.Equal(b.Trailer, other.Trailer) {
		return false
	}

	for i, tx := range b.Txs {
		otherTx := other.Txs[i]
		if !bytes.Equal(tx.content, otherTx.content) {
			return false
		}
	}

	return b.TotalDifficulty.Cmp(other.TotalDifficulty) == 0 && b.Number.Cmp(other.Number) == 0
}
