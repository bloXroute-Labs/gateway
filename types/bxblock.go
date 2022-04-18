package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"time"
)

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
	Header          []byte
	Txs             []*BxBlockTransaction
	Trailer         []byte
	TotalDifficulty *big.Int
	Number          *big.Int
	timestamp       time.Time
	size            int
}

// NewBxBlock creates a new BxBlock that's ready for compression. This means that all transaction hashes must be included.
func NewBxBlock(hash SHA256Hash, header []byte, txs []*BxBlockTransaction, trailer []byte, totalDifficulty *big.Int, number *big.Int, size int) (*BxBlock, error) {
	for _, tx := range txs {
		if tx.Hash() == (SHA256Hash{}) {
			return nil, errors.New("all transactions must contain hashes")
		}
	}
	return NewRawBxBlock(hash, header, txs, trailer, totalDifficulty, number, size), nil
}

// NewRawBxBlock create a new BxBlock without compression restrictions. This should only be used when parsing the result of an existing BxBlock.
func NewRawBxBlock(hash SHA256Hash, header []byte, txs []*BxBlockTransaction, trailer []byte, totalDifficulty *big.Int, number *big.Int, size int) *BxBlock {
	bxBlock := &BxBlock{
		hash:            hash,
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
