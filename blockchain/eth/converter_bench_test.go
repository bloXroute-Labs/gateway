package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func benchBxBlock(tb testing.TB, txCount int) *types.BxBlock {
	txs := make([]*ethtypes.Transaction, 0, txCount)
	for i := 0; i < txCount; i++ {
		txs = append(txs, bxmock.NewSignedEthTx(uint8(i%3), uint64(i), nil, nil))
	}

	header := bxmock.NewEthBlockHeader(1000, common.Hash{})
	block := bxethcommon.NewBlock(header, &ethtypes.Body{Transactions: txs, Withdrawals: []*ethtypes.Withdrawal{}}, nil, bxmock.NewTestHasher())

	bxBlock, err := Converter{}.BlockBlockchainToBDN(bxethcommon.NewBlockInfo(block, big.NewInt(100)))
	require.NoError(tb, err)

	return bxBlock
}

func BenchmarkEthBlockBDNtoBlockchain(b *testing.B) {
	c := Converter{}
	bxBlock := benchBxBlock(b, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.ethBlockBDNtoBlockchain(bxBlock)
		require.NoError(b, err)
	}
}

func BenchmarkEthBlockBDNtoBlockchainEncode(b *testing.B) {
	bxBlock := benchBxBlock(b, 100)
	txs := make([]rlp.RawValue, 0, len(bxBlock.Txs))
	for _, tx := range bxBlock.Txs {
		txs = append(txs, tx.Content())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rlp.EncodeToBytes(bxBlockRLP{Header: bxBlock.Header, Txs: txs, Trailer: bxBlock.Trailer})
		require.NoError(b, err)
	}
}

func BenchmarkEthBlockBDNtoBlockchainDecode(b *testing.B) {
	bxBlock := benchBxBlock(b, 100)
	txs := make([]rlp.RawValue, 0, len(bxBlock.Txs))
	for _, tx := range bxBlock.Txs {
		txs = append(txs, tx.Content())
	}
	encoded, err := rlp.EncodeToBytes(bxBlockRLP{Header: bxBlock.Header, Txs: txs, Trailer: bxBlock.Trailer})
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var commonBlock bxethcommon.Block
		require.NoError(b, rlp.DecodeBytes(encoded, &commonBlock))
	}
}

// BenchmarkEthBlockBDNtoBlockchainDecodeNoTxs measures the same decode without the block
// transactions, i.e. the floor the conversion could reach if transactions were decoded lazily.
func BenchmarkEthBlockBDNtoBlockchainDecodeNoTxs(b *testing.B) {
	bxBlock := benchBxBlock(b, 100)
	encoded, err := rlp.EncodeToBytes(bxBlockRLP{Header: bxBlock.Header, Trailer: bxBlock.Trailer})
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var commonBlock bxethcommon.Block
		require.NoError(b, rlp.DecodeBytes(encoded, &commonBlock))
	}
}

func BenchmarkEthBlockRawTransactions(b *testing.B) {
	bxBlock := benchBxBlock(b, 100)
	blockInfo, err := Converter{}.ethBlockBDNtoBlockchain(bxBlock)
	require.NoError(b, err)

	b.Run("marshal_binary", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			raw := make([][]byte, 0, len(blockInfo.Block.Transactions()))
			for _, tx := range blockInfo.Block.Transactions() {
				encoded, err := tx.MarshalBinary()
				require.NoError(b, err)
				raw = append(raw, encoded)
			}
			require.Len(b, raw, 100)
		}
	})

	b.Run("from_bxblock_content", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			raw, err := RawTransactionsFromBxBlock(bxBlock)
			require.NoError(b, err)
			require.Len(b, raw, 100)
		}
	})
}
