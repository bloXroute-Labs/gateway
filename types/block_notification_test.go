package types

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEthBlock(t *testing.T) {
	block := EthBlockNotification{txsMu: &sync.RWMutex{}, rawTxsMu: &sync.RWMutex{}, Block: &bxethcommon.Block{}}
	_ = block.WithFields([]string{"hash", "header", "transactions", "uncles"})
	// TODO add test checking the header values
}

func TestCheckNonce(t *testing.T) {
	nonce := fmt.Sprintf("0x%016s", hexutil.EncodeUint64(57635743)[2:])
	assert.Equal(t, "0x00000000036f739f", nonce)

	nonce = fmt.Sprintf("0x%016s", hexutil.EncodeUint64(0)[2:])
	assert.Equal(t, "0x0000000000000000", nonce)
}

func TestParseTransactionsCachesPerSenderVariant(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	to := ethcommon.HexToAddress("0x3535353535353535353535353535353535353535")
	tx, err := ethtypes.SignNewTx(key, ethtypes.LatestSignerForChainID(big.NewInt(1)), &ethtypes.LegacyTx{
		Gas: 21_000, GasPrice: big.NewInt(1), To: &to, Value: big.NewInt(1),
	})
	require.NoError(t, err)

	header := &ethtypes.Header{Number: big.NewInt(1), Difficulty: big.NewInt(0), BaseFee: big.NewInt(1)}
	block := bxethcommon.NewBlockWithHeader(header).WithBody(ethtypes.Body{Transactions: []*ethtypes.Transaction{tx}})

	txs := func(n *EthBlockNotification, include string) []map[string]interface{} {
		return n.WithFields([]string{include}).(*EthBlockNotification).Transactions
	}

	for _, first := range []string{"transactions", "transactions_without_sender"} {
		t.Run(first+" first", func(t *testing.T) {
			n, err := NewEthBlockNotification("Mainnet", block.Hash(), block, nil)
			require.NoError(t, err)

			txs(n, first)

			withSender, withoutSender := txs(n, "transactions"), txs(n, "transactions_without_sender")
			require.Len(t, withSender, 1)
			require.Len(t, withoutSender, 1)
			assert.Contains(t, withSender[0], "from")
			assert.NotContains(t, withoutSender[0], "from")
		})
	}
}
