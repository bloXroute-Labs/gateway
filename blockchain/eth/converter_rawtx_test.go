package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// the mock block carries one transaction of every supported type, including blob and set code
func TestRawTransactionsFromBxBlock(t *testing.T) {
	block := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := Converter{}.BlockBlockchainToBDN(bxethcommon.NewBlockInfo(block, big.NewInt(100)))
	require.NoError(t, err)

	raw, err := RawTransactionsFromBxBlock(bxBlock)
	require.NoError(t, err)
	require.Len(t, raw, len(block.Transactions()))

	for i, tx := range block.Transactions() {
		expected, err := tx.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, expected, raw[i], "transaction %v of type %v", i, tx.Type())
	}
}

// transactions already in canonical form are not in-block RLP and must be rejected instead of
// being sliced into something else
func TestRawTransactionsFromBxBlockRejectsCanonicalTxs(t *testing.T) {
	block := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := Converter{}.BlockBlockchainToBDN(bxethcommon.NewBlockInfo(block, big.NewInt(100)))
	require.NoError(t, err)

	for i, tx := range block.Transactions() {
		if tx.Type() == ethtypes.LegacyTxType {
			continue
		}

		canonical, err := tx.MarshalBinary()
		require.NoError(t, err)

		bxBlock.Txs[i] = types.NewRawBxBlockTransaction(canonical)
		_, err = RawTransactionsFromBxBlock(bxBlock)
		require.Error(t, err)

		return
	}
}

func TestSeededRawTransactionsMatchEncoded(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := c.BlockBlockchainToBDN(bxethcommon.NewBlockInfo(block, big.NewInt(100)))
	require.NoError(t, err)

	blockInfo, err := c.ethBlockBDNtoBlockchain(bxBlock)
	require.NoError(t, err)

	newNotification := func() *types.EthBlockNotification {
		n, nErr := types.NewEthBlockNotification("BSC-Mainnet", common.Hash(bxBlock.ExecutionHash()), blockInfo.Block, nil)
		require.NoError(t, nErr)
		return n
	}

	raw, err := RawTransactionsFromBxBlock(bxBlock)
	require.NoError(t, err)

	seeded := newNotification()
	seeded.SeedRawTransactions(raw)

	encoded := newNotification().WithFields([]string{"raw_transactions"}).(*types.EthBlockNotification)
	fromSeed := seeded.WithFields([]string{"raw_transactions"}).(*types.EthBlockNotification)
	require.Equal(t, encoded.RawTransactions, fromSeed.RawTransactions)

	// a copy that did not ask for raw transactions must not carry them
	headerOnly := seeded.WithFields([]string{"hash", "header"}).(*types.EthBlockNotification)
	require.Nil(t, headerOnly.RawTransactions)

	// GetRawTxByIndex serves the same bytes from the shared cache
	for i := range raw {
		require.Equal(t, encoded.RawTransactions[i], headerOnly.GetRawTxByIndex(i))
	}
}
