package types

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
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

	blockchainTx, err := tx.BlockchainTransaction(EmptySender)
	if err != nil {
		return hash, EthTransaction{}, tx, err
	}

	return hash, *blockchainTx.(*EthTransaction), tx, nil
}
func TestBigValueTransacrtion(t *testing.T) {
	hash, ethTx, _, err := ethTransaction(fixtures.BigValueTransactionHashBSC, fixtures.BigValueTransactionBSC)
	assert.Nil(t, err)
	ethTx.Filters([]string{})
	assert.Equal(t, "0x0", ethTx.fields["type"])
	assert.Equal(t, "0x"+hash.String(), ethTx.fields["hash"])
	jsonMap := ethTx.Fields([]string{
		"tx_contents.value",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.type",
	})
	assert.Equal(t, "0x15fc2005198351000", jsonMap["value"])

}

func TestLegacyTransaction(t *testing.T) {
	expectedGasPrice := new(big.Int).SetInt64(fixtures.LegacyGasPrice)
	expectedFromAddress := test.NewEthAddress(fixtures.LegacyFromAddress)
	expectedChainID := new(big.Int).SetInt64(fixtures.LegacyChainID)

	hash, ethTx, _, err := ethTransaction(fixtures.LegacyTransactionHash, fixtures.LegacyTransaction)
	assert.Nil(t, err)

	// check decoding transaction structure
	ethTx.Filters([]string{})
	assert.Equal(t, "0x0", ethTx.fields["type"])
	assert.Equal(t, "0x"+hash.String(), ethTx.fields["hash"])
	assert.Equal(t, BigIntAsString(expectedGasPrice), ethTx.fields["gasPrice"])
	assert.Equal(t, expectedGasPrice, ethTx.GasFeeCap)
	assert.Equal(t, expectedGasPrice, ethTx.GasTipCap)
	assert.Equal(t, expectedChainID, ethTx.ChainID)
	assert.Equal(t, strings.ToLower(expectedFromAddress.String()), ethTx.fields["from"])

	// check WithFields
	jsonMap := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.type",
	})
	assert.Equal(t, "0x0", jsonMap["type"])
	assert.Equal(t, fixtures.LegacyFromAddress, jsonMap["from"])
	assert.Equal(t, fixtures.LegacyTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeBig(expectedGasPrice), jsonMap["gasPrice"])

	assert.False(t, test.Contains(jsonMap, "maxFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "maxPriorityFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "accessList"))
	// chainID not included during serialization of LegacyTransaction
	assert.False(t, test.Contains(jsonMap, "chainID"))

	// check Filters
	filteredTx := ethTx.Filters([]string{
		"from",
		"chain_id",
		"gas_price",
		"max_fee_per_gas",
		"max_priority_fee_per_gas",
	})
	assert.Nil(t, err)
	assert.True(t, test.Contains(filteredTx, "type"))
	assert.Equal(t, fixtures.LegacyFromAddress, filteredTx["from"])
	assert.Equal(t, fixtures.LegacyGasPrice, filteredTx["gas_price"])
	// when chain ID is explicitly asked for it's included
	assert.Equal(t, 1, filteredTx["chain_id"])
}

func TestAccessListTransaction(t *testing.T) {
	expectedGasPrice := new(big.Int).SetInt64(fixtures.AccessListGasPrice)
	expectedFromAddress := test.NewEthAddress(fixtures.AccessListFromAddress)

	hash, ethTx, _, err := ethTransaction(fixtures.AccessListTransactionHash, fixtures.AccessListTransaction)
	assert.Nil(t, err)

	// check decoding transaction structure
	ethTx.Filters([]string{})
	assert.Equal(t, "0x1", ethTx.fields["type"])
	assert.Equal(t, "0x"+hash.String(), ethTx.fields["hash"])
	assert.Equal(t, BigIntAsString(expectedGasPrice), ethTx.fields["gasPrice"])
	assert.Equal(t, expectedGasPrice, ethTx.GasFeeCap)
	assert.Equal(t, expectedGasPrice, ethTx.GasTipCap)
	assert.Equal(t, strings.ToLower(expectedFromAddress.String()), ethTx.fields["from"])

	// check WithFields
	jsonMap := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.access_list",
	})
	assert.Nil(t, err)

	assert.Equal(t, fixtures.AccessListFromAddress, jsonMap["from"])
	assert.Equal(t, fixtures.AccessListTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeBig(expectedGasPrice), jsonMap["gasPrice"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.AccessListChainID), jsonMap["chainId"])
	assert.Equal(t, fixtures.AccessListLength, len(jsonMap["accessList"].(types.AccessList)))

	assert.False(t, test.Contains(jsonMap, "maxFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "maxPriorityFeePerGas"))

	// check Filters
	filteredTx := ethTx.Filters([]string{
		"from",
		"chain_id",
		"gas_price",
		"max_fee_per_gas",
		"max_priority_fee_per_gas",
		"type",
	})
	assert.Nil(t, err)
	assert.Equal(t, "1", filteredTx["type"])
	assert.Equal(t, fixtures.AccessListFromAddress, filteredTx["from"])
	assert.Equal(t, fixtures.AccessListGasPrice, filteredTx["gas_price"])
	assert.Equal(t, fixtures.AccessListChainID, filteredTx["chain_id"])
}

func TestDynamicFeeTransaction(t *testing.T) {
	expectedFromAddress := test.NewEthAddress(fixtures.DynamicFeeFromAddress)

	hash, ethTx, _, err := ethTransaction(fixtures.DynamicFeeTransactionHash, fixtures.DynamicFeeTransaction)
	assert.Nil(t, err)

	// check decoding transaction structure
	ethTx.Filters([]string{})
	assert.Equal(t, "0x2", ethTx.fields["type"])
	assert.Equal(t, "0x"+hash.String(), ethTx.fields["hash"])
	assert.Equal(t, int64(fixtures.DynamicFeeFeePerGas), ethTx.GasFeeCap.Int64())
	assert.Equal(t, int64(fixtures.DynamicFeeTipPerGas), ethTx.GasTipCap.Int64())
	assert.Equal(t, strings.ToLower(expectedFromAddress.String()), ethTx.fields["from"])

	// check WithFields
	jsonMap := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.access_list",
		"tx_contents.type",
	})
	assert.Nil(t, err)

	assert.Nil(t, err)
	assert.Equal(t, fixtures.DynamicFeeFromAddress, jsonMap["from"])
	assert.Equal(t, fixtures.DynamicFeeTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeChainID), jsonMap["chainId"])
	assert.Equal(t, fixtures.DynamicFeeAccessListLength, len(jsonMap["accessList"].(types.AccessList)))
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeFeePerGas), jsonMap["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeTipPerGas), jsonMap["maxPriorityFeePerGas"])
	assert.Equal(t, "0x2", jsonMap["type"])
	assert.Equal(t, nil, jsonMap["gasPrice"])

	// check WithFields without type
	jsonMapWithoutType := ethTx.Fields([]string{
		"tx_contents.gas_price",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
	})
	assert.Nil(t, err)

	assert.Nil(t, err)
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeFeePerGas), jsonMapWithoutType["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeTipPerGas), jsonMapWithoutType["maxPriorityFeePerGas"])
	assert.Equal(t, nil, jsonMapWithoutType["gasPrice"])

	// check Filters
	filteredTx := ethTx.Filters([]string{
		"from",
		"chain_id",
		"gas_price",
		"max_fee_per_gas",
		"max_priority_fee_per_gas",
	})
	assert.Nil(t, err)
	assert.Equal(t, fixtures.DynamicFeeFromAddress, filteredTx["from"])
	assert.Equal(t, fixtures.DynamicFeeChainID, filteredTx["chain_id"])
	assert.Equal(t, fixtures.DynamicFeeFeePerGas, filteredTx["max_fee_per_gas"])
	assert.Equal(t, fixtures.DynamicFeeTipPerGas, filteredTx["max_priority_fee_per_gas"])
}

func TestContractCreationTx(t *testing.T) {
	hash, ethTx, _, err := ethTransaction(fixtures.ContractCreationTxHash, fixtures.ContractCreationTx)
	assert.Nil(t, err)
	filters := ethTx.Filters([]string{})
	assert.Equal(t, "0x0", filters["to"])

	assert.Equal(t, "0x"+hash.String(), ethTx.fields["hash"])

	ethJSON := ethTx.Fields([]string{"tx_contents.to", "tx_contents.from"})

	to, ok := ethJSON["to"]
	assert.Equal(t, false, ok)
	assert.Equal(t, nil, to)
	assert.Equal(t, "0x09e9ff67d9d5a25fa465db6f0bede5560581f8cb", ethJSON["from"])
}
