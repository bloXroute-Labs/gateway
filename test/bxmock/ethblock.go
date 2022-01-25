package bxmock

import (
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"time"
)

// NewEthBlockHeader generates a test header. Note that tx hash, uncle hash, receipt hash, and bloom will be overridden by when actually constructing a blocks
func NewEthBlockHeader(height uint64, parentHash common.Hash) *ethtypes.Header {
	if parentHash == (common.Hash{}) {
		parentHash = common.BytesToHash(types.GenerateSHA256Hash().Bytes())
	}
	header := ethtypes.Header{
		ParentHash:  parentHash,
		UncleHash:   common.BytesToHash(types.GenerateSHA256Hash().Bytes()),
		Coinbase:    GenerateAddress(),
		Root:        common.BytesToHash(types.GenerateSHA256Hash().Bytes()),
		TxHash:      common.BytesToHash(types.GenerateSHA256Hash().Bytes()),
		ReceiptHash: common.BytesToHash(types.GenerateSHA256Hash().Bytes()),
		Bloom:       GenerateBloom(),
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(int64(height)),
		GasLimit:    uint64(1),
		GasUsed:     uint64(1),
		Time:        uint64(time.Now().Unix()),
		Extra:       []byte{},
		MixDigest:   common.BytesToHash(types.GenerateSHA256Hash().Bytes()),
		Nonce:       GenerateBlockNonce(),
		BaseFee:     big.NewInt(1),
	}
	return &header
}

// NewEthBlockWithHeader generates an Ethereum block testing purposes from a header
func NewEthBlockWithHeader(header *ethtypes.Header) *ethtypes.Block {
	txs := []*ethtypes.Transaction{
		NewSignedEthTx(ethtypes.LegacyTxType, 1, nil),
		NewSignedEthTx(ethtypes.AccessListTxType, 2, nil),
		NewSignedEthTx(ethtypes.DynamicFeeTxType, 3, nil),
	}
	uncles := []*ethtypes.Header{
		NewEthBlockHeader(header.Number.Uint64(), common.Hash{}),
		NewEthBlockHeader(header.Number.Uint64(), common.Hash{}),
	}

	block := ethtypes.NewBlock(header, txs, uncles, nil, NewTestHasher())
	return block
}

// NewEthBlock generates an Ethereum block for testing purposes
func NewEthBlock(height uint64, parentHash common.Hash) *ethtypes.Block {
	if parentHash == (common.Hash{}) {
		parentHash = common.BytesToHash(types.GenerateSHA256Hash().Bytes())
	}

	initialHeader := NewEthBlockHeader(height, parentHash)
	return NewEthBlockWithHeader(initialHeader)
}
