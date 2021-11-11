package types

import (
	"encoding/hex"
	"github.com/bloXroute-Labs/gateway/test"
	"github.com/bloXroute-Labs/gateway/test/fixtues"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
	"time"
)

func ethTransaction(hashString string, txString string) (SHA256Hash, EthTransaction, *BxTransaction, error) {
	hash, err := NewSHA256HashFromString(hashString)
	if err != nil {
		return SHA256Hash{}, EthTransaction{}, nil, err
	}

	content, err := hex.DecodeString(txString)
	if err != nil {
		return hash, EthTransaction{}, nil, err
	}

	tx := NewBxTransaction(hash, testNetworkNum, TFPaidTx, time.Now())
	tx.SetContent(content)

	blockchainTx, err := tx.BlockchainTransaction()
	if err != nil {
		return hash, EthTransaction{}, tx, err
	}

	return hash, *blockchainTx.(*EthTransaction), tx, nil
}

func TestLegacyTransaction(t *testing.T) {
	expectedGasPrice := new(big.Int).SetInt64(fixtues.LegacyGasPrice)
	expectedFromAddress := test.NewEthAddress(fixtues.LegacyFromAddress)

	hash, ethTx, _, err := ethTransaction(fixtues.LegacyTransactionHash, fixtues.LegacyTransaction)
	assert.Nil(t, err)

	// check decoding transaction structure
	assert.Equal(t, LegacyTransactionType, ethTx.TxType)
	assert.Equal(t, hash, ethTx.Hash.SHA256Hash)
	assert.Equal(t, EthBigInt{Int: expectedGasPrice}, ethTx.GasPrice)
	assert.Equal(t, EthBigInt{Int: expectedGasPrice}, ethTx.GasFeeCap)
	assert.Equal(t, EthBigInt{Int: expectedGasPrice}, ethTx.GasTipCap)
	assert.Equal(t, expectedFromAddress.Bytes(), ethTx.From.Bytes())

	// check JSON serialization
	jsonMap, err := test.MarshallJSONToMap(ethTx)
	assert.Nil(t, err)
	assert.Equal(t, fixtues.LegacyFromAddress, jsonMap["from"])
	assert.Equal(t, fixtues.LegacyTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeBig(expectedGasPrice), jsonMap["gasPrice"])

	assert.False(t, test.Contains(jsonMap, "maxFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "maxPriorityFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "chainId"))
	assert.False(t, test.Contains(jsonMap, "accessList"))

	// check Filters
	filteredTx := ethTx.Filters([]string{
		"from",
		"chain_id",
		"gas_price",
		"max_fee_per_gas",
		"max_priority_fee_per_gas",
	})
	assert.Nil(t, err)
	assert.Equal(t, "0x0", jsonMap["type"])
	assert.Equal(t, fixtues.LegacyFromAddress, filteredTx["from"])
	assert.Equal(t, fixtues.LegacyGasPrice, filteredTx["gas_price"])
	// when chain ID is explicitly asked for it's included
	assert.Equal(t, 1, filteredTx["chain_id"])
	assert.Equal(t, -1, filteredTx["max_fee_per_gas"])
	assert.Equal(t, -1, filteredTx["max_priority_fee_per_gas"])

	// check WithFields
	fieldsTx := ethTx.WithFields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
	})
	assert.Nil(t, err)

	ethFieldsTx := fieldsTx.(EthTransaction)

	assert.Equal(t, &expectedFromAddress, ethFieldsTx.From.Address)
	assert.Equal(t, hash, ethFieldsTx.Hash.SHA256Hash)
	assert.Equal(t, expectedGasPrice, ethFieldsTx.GasPrice.Int)
	assert.Equal(t, EthBigInt{big.NewInt(1)}, ethFieldsTx.ChainID)
	assert.Equal(t, EthBigInt{}, ethFieldsTx.GasFeeCap)
	assert.Equal(t, EthBigInt{}, ethFieldsTx.GasTipCap)
}

func TestAccessListTransaction(t *testing.T) {
	expectedGasPrice := new(big.Int).SetInt64(fixtues.AccessListGasPrice)
	expectedFromAddress := test.NewEthAddress(fixtues.AccessListFromAddress)

	hash, ethTx, _, err := ethTransaction(fixtues.AccessListTransactionHash, fixtues.AccessListTransaction)
	assert.Nil(t, err)

	// check decoding transaction structure
	assert.Equal(t, AccessListTransactionType, ethTx.TxType)
	assert.Equal(t, hash, ethTx.Hash.SHA256Hash)
	assert.Equal(t, EthBigInt{Int: expectedGasPrice}, ethTx.GasPrice)
	assert.Equal(t, EthBigInt{Int: expectedGasPrice}, ethTx.GasFeeCap)
	assert.Equal(t, EthBigInt{Int: expectedGasPrice}, ethTx.GasTipCap)
	assert.Equal(t, expectedFromAddress.Bytes(), ethTx.From.Bytes())

	// check JSON serialization
	jsonMap, err := test.MarshallJSONToMap(ethTx)
	assert.Nil(t, err)
	assert.Equal(t, fixtues.AccessListFromAddress, jsonMap["from"])
	assert.Equal(t, fixtues.AccessListTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeBig(expectedGasPrice), jsonMap["gasPrice"])
	assert.Equal(t, hexutil.EncodeUint64(fixtues.AccessListChainID), jsonMap["chainId"])
	assert.Equal(t, fixtues.AccessListLength, len(jsonMap["accessList"].([]interface{})))

	assert.False(t, test.Contains(jsonMap, "maxFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "maxPriorityFeePerGas"))

	// check Filters
	filteredTx := ethTx.Filters([]string{
		"from",
		"chain_id",
		"gas_price",
		"max_fee_per_gas",
		"max_priority_fee_per_gas",
	})
	assert.Nil(t, err)
	assert.Equal(t, "0x1", jsonMap["type"])
	assert.Equal(t, fixtues.AccessListFromAddress, filteredTx["from"])
	assert.Equal(t, fixtues.AccessListGasPrice, filteredTx["gas_price"])
	assert.Equal(t, fixtues.AccessListChainID, filteredTx["chain_id"])
	assert.Equal(t, -1, filteredTx["max_fee_per_gas"])
	assert.Equal(t, -1, filteredTx["max_priority_fee_per_gas"])

	// check WithFields
	fieldsTx := ethTx.WithFields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
	})
	assert.Nil(t, err)

	ethFieldsTx := fieldsTx.(EthTransaction)

	assert.Equal(t, &expectedFromAddress, ethFieldsTx.From.Address)
	assert.Equal(t, hash, ethFieldsTx.Hash.SHA256Hash)
	assert.Equal(t, expectedGasPrice, ethFieldsTx.GasPrice.Int)
	assert.Equal(t, int64(fixtues.AccessListChainID), ethFieldsTx.ChainID.Int64())
	assert.Equal(t, EthBigInt{}, ethFieldsTx.GasFeeCap)
	assert.Equal(t, EthBigInt{}, ethFieldsTx.GasTipCap)
}

func TestDynamicFeeTransaction(t *testing.T) {
	expectedFromAddress := test.NewEthAddress(fixtues.DynamicFeeFromAddress)

	hash, ethTx, _, err := ethTransaction(fixtues.DynamicFeeTransactionHash, fixtues.DynamicFeeTransaction)
	assert.Nil(t, err)

	// check decoding transaction structure
	assert.Equal(t, DynamicFeeTransactionType, ethTx.TxType)
	assert.Equal(t, hash, ethTx.Hash.SHA256Hash)
	assert.Equal(t, int64(fixtues.DynamicFeeFeePerGas), ethTx.GasPrice.Int64())
	assert.Equal(t, int64(fixtues.DynamicFeeFeePerGas), ethTx.GasFeeCap.Int64())
	assert.Equal(t, int64(fixtues.DynamicFeeTipPerGas), ethTx.GasTipCap.Int64())
	assert.Equal(t, expectedFromAddress.Bytes(), ethTx.From.Bytes())

	// check JSON serialization
	jsonMap, err := test.MarshallJSONToMap(ethTx)
	assert.Nil(t, err)
	assert.Equal(t, "0x2", jsonMap["type"])
	assert.Equal(t, fixtues.DynamicFeeFromAddress, jsonMap["from"])
	assert.Equal(t, fixtues.DynamicFeeTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeUint64(fixtues.DynamicFeeChainID), jsonMap["chainId"])
	assert.Equal(t, fixtues.DynamicFeeAccessListLength, len(jsonMap["accessList"].([]interface{})))
	assert.Equal(t, hexutil.EncodeUint64(fixtues.DynamicFeeFeePerGas), jsonMap["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(fixtues.DynamicFeeTipPerGas), jsonMap["maxPriorityFeePerGas"])
	assert.Equal(t, nil, jsonMap["gasPrice"])

	// check Filters
	filteredTx := ethTx.Filters([]string{
		"from",
		"chain_id",
		"gas_price",
		"max_fee_per_gas",
		"max_priority_fee_per_gas",
	})
	assert.Nil(t, err)
	assert.Equal(t, fixtues.DynamicFeeFromAddress, filteredTx["from"])
	assert.Equal(t, fixtues.DynamicFeeChainID, filteredTx["chain_id"])
	assert.Equal(t, -1, filteredTx["gas_price"])
	assert.Equal(t, fixtues.DynamicFeeFeePerGas, filteredTx["max_fee_per_gas"])
	assert.Equal(t, fixtues.DynamicFeeTipPerGas, filteredTx["max_priority_fee_per_gas"])

	// check WithFields
	fieldsTx := ethTx.WithFields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
	})
	assert.Nil(t, err)

	ethFieldsTx := fieldsTx.(EthTransaction)

	assert.Equal(t, &expectedFromAddress, ethFieldsTx.From.Address)
	assert.Equal(t, hash, ethFieldsTx.Hash.SHA256Hash)
	assert.Equal(t, int64(fixtues.DynamicFeeChainID), ethFieldsTx.ChainID.Int64())
	assert.Equal(t, EthBigInt{}, ethFieldsTx.GasPrice)
	assert.Equal(t, int64(fixtues.DynamicFeeFeePerGas), ethFieldsTx.GasFeeCap.Int64())
	assert.Equal(t, int64(fixtues.DynamicFeeTipPerGas), ethFieldsTx.GasTipCap.Int64())
}

func TestContractCreationTx(t *testing.T) {
	hash, ethTx, _, err := ethTransaction(fixtues.ContractCreationTxHash, fixtues.ContractCreationTx)
	assert.Nil(t, err)
	assert.Equal(t, hash, ethTx.Hash.SHA256Hash)

	txWithFields := ethTx.WithFields([]string{"tx_contents.to", "tx_contents.from"})
	assert.Nil(t, err)

	ethTxWithFields, ok := txWithFields.(EthTransaction)
	assert.True(t, ok)

	ethJSON, err := test.MarshallJSONToMap(ethTxWithFields)
	assert.Nil(t, err)

	to, ok := ethJSON["to"]
	assert.Equal(t, nil, to)
	assert.Equal(t, "0x09e9ff67d9d5a25fa465db6f0bede5560581f8cb", ethJSON["from"])
}
