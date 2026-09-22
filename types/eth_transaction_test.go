package types

import (
	"encoding/hex"
	"math/big"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
)

func ethTransaction(hashString string, txString string) (SHA256Hash, *EthTransaction, error) {
	hash, err := NewSHA256HashFromString(hashString)
	if err != nil {
		return SHA256Hash{}, nil, err
	}

	content, err := hex.DecodeString(txString)
	if err != nil {
		return hash, nil, err
	}

	tx := NewBxTransaction(hash, testNetworkNum, TFPaidTx, time.Now())
	tx.SetContent(content)

	ethTx := NewEthTransactionFromBytes(content, EmptySender)

	if _, err := ethTx.Tx(); err != nil {
		return hash, nil, err
	}

	return hash, ethTx, nil
}

func TestBigValueTransacrtion(t *testing.T) {
	hash, ethTx, err := ethTransaction(fixtures.BigValueTransactionHashBSC, fixtures.BigValueTransactionBSC)
	assert.NoError(t, err)
	_, _ = ethTx.Fields(AllFields)
	assert.Equal(t, "0x0", (*ethTx.fields.Load())["type"])
	assert.Equal(t, "0x"+hash.String(), (*ethTx.fields.Load())["hash"])
	jsonMap, err := ethTx.Fields([]string{
		"tx_contents.value",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.type",
	})
	assert.NoError(t, err)
	assert.Equal(t, "0x15fc2005198351000", jsonMap["value"])
}

func TestLegacyTransaction(t *testing.T) {
	expectedGasPrice := new(big.Int).SetInt64(fixtures.LegacyGasPrice)
	expectedFromAddress := test.NewEthAddress(fixtures.LegacyFromAddress)
	expectedChainID := new(big.Int).SetInt64(fixtures.LegacyChainID)

	hash, ethTx, err := ethTransaction(fixtures.LegacyTransactionHash, fixtures.LegacyTransaction)
	assert.NoError(t, err)

	// check decoding transaction structure
	_, _ = ethTx.Fields(AllFieldsWithFrom)
	assert.Equal(t, "0x0", (*ethTx.fields.Load())["type"])
	assert.Equal(t, "0x"+hash.String(), (*ethTx.fields.Load())["hash"])
	assert.Equal(t, BigIntAsString(expectedGasPrice), (*ethTx.fields.Load())["gasPrice"])
	assert.NotContains(t, (*ethTx.fields.Load()), "maxFeePerGas")
	assert.NotContains(t, (*ethTx.fields.Load()), "maxPriorityFeePerGas")
	chainID, err := ethTx.ChainID()
	assert.NoError(t, err)
	assert.Equal(t, expectedChainID, chainID)
	from, err := ethTx.From()
	assert.NoError(t, err)
	assert.Equal(t, expectedFromAddress, *from)

	// check WithFields
	jsonMap, err := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.type",
	})
	assert.NoError(t, err)
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
	filteredTx, err := ethTx.Filters()
	assert.NoError(t, err)
	assert.True(t, test.Contains(filteredTx, "type"))
	assert.Equal(t, fixtures.LegacyFromAddress, filteredTx["from"])
	assert.Equal(t, fixtures.LegacyGasPrice, filteredTx["gas_price"])
	// when chain ID is explicitly asked for it's included
	assert.Equal(t, 1, filteredTx["chain_id"])
}

func TestAccessListTransaction(t *testing.T) {
	expectedGasPrice := new(big.Int).SetInt64(fixtures.AccessListGasPrice)
	expectedFromAddress := test.NewEthAddress(fixtures.AccessListFromAddress)

	hash, ethTx, err := ethTransaction(fixtures.AccessListTransactionHash, fixtures.AccessListTransaction)
	assert.NoError(t, err)

	// check decoding transaction structure
	_, _ = ethTx.Fields(AllFieldsWithFrom)
	assert.Equal(t, "0x1", (*ethTx.fields.Load())["type"])
	assert.Equal(t, "0x"+hash.String(), (*ethTx.fields.Load())["hash"])
	assert.Equal(t, BigIntAsString(expectedGasPrice), (*ethTx.fields.Load())["gasPrice"])
	assert.NotContains(t, (*ethTx.fields.Load()), "maxFeePerGas")
	assert.NotContains(t, (*ethTx.fields.Load()), "maxPriorityFeePerGas")
	from, err := ethTx.From()
	assert.NoError(t, err)
	assert.Equal(t, expectedFromAddress, *from)

	// check WithFields
	jsonMap, err := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.access_list",
	})
	assert.NoError(t, err)

	assert.Equal(t, fixtures.AccessListFromAddress, jsonMap["from"])
	assert.Equal(t, fixtures.AccessListTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeBig(expectedGasPrice), jsonMap["gasPrice"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.AccessListChainID), jsonMap["chainId"])
	assert.Equal(t, fixtures.AccessListLength, len(jsonMap["accessList"].(ethtypes.AccessList)))

	assert.False(t, test.Contains(jsonMap, "maxFeePerGas"))
	assert.False(t, test.Contains(jsonMap, "maxPriorityFeePerGas"))

	// check Filters
	filteredTx, err := ethTx.Filters()
	assert.NoError(t, err)
	assert.Equal(t, "1", filteredTx["type"])
	assert.Equal(t, fixtures.AccessListFromAddress, filteredTx["from"])
	assert.Equal(t, fixtures.AccessListGasPrice, filteredTx["gas_price"])
	assert.Equal(t, fixtures.AccessListChainID, filteredTx["chain_id"])
}

func TestDynamicFeeTransaction(t *testing.T) {
	expectedMaxFeePerGas := new(big.Int).SetInt64(fixtures.DynamicFeeFeePerGas)
	expectedPriorityFeePerGas := new(big.Int).SetInt64(fixtures.DynamicFeeTipPerGas)
	expectedFromAddress := test.NewEthAddress(fixtures.DynamicFeeFromAddress)

	hash, ethTx, err := ethTransaction(fixtures.DynamicFeeTransactionHash, fixtures.DynamicFeeTransaction)
	assert.NoError(t, err)

	// check decoding transaction structure
	_, _ = ethTx.Fields(AllFieldsWithFrom)
	assert.Equal(t, "0x2", (*ethTx.fields.Load())["type"])
	assert.Equal(t, "0x"+hash.String(), (*ethTx.fields.Load())["hash"])
	assert.Equal(t, BigIntAsString(expectedMaxFeePerGas), (*ethTx.fields.Load())["maxFeePerGas"])
	assert.Equal(t, BigIntAsString(expectedPriorityFeePerGas), (*ethTx.fields.Load())["maxPriorityFeePerGas"])
	assert.Nil(t, (*ethTx.fields.Load())["gasPrice"])
	from, err := ethTx.From()
	assert.NoError(t, err)
	assert.Equal(t, expectedFromAddress, *from)

	// check WithFields
	jsonMap, err := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.access_list",
		"tx_contents.type",
	})
	assert.NoError(t, err)

	assert.Equal(t, fixtures.DynamicFeeFromAddress, jsonMap["from"])
	assert.Equal(t, fixtures.DynamicFeeTransactionHash, jsonMap["hash"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeChainID), jsonMap["chainId"])
	assert.Equal(t, fixtures.DynamicFeeAccessListLength, len(jsonMap["accessList"].(ethtypes.AccessList)))
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeFeePerGas), jsonMap["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeTipPerGas), jsonMap["maxPriorityFeePerGas"])
	assert.Equal(t, "0x2", jsonMap["type"])
	assert.Equal(t, nil, jsonMap["gasPrice"])

	// check WithFields without type
	jsonMapWithoutType, err := ethTx.Fields([]string{
		"tx_contents.gas_price",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
	})
	assert.NoError(t, err)

	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeFeePerGas), jsonMapWithoutType["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(fixtures.DynamicFeeTipPerGas), jsonMapWithoutType["maxPriorityFeePerGas"])
	assert.Equal(t, nil, jsonMapWithoutType["gasPrice"])

	// check Filters
	filteredTx, err := ethTx.Filters()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.DynamicFeeFromAddress, filteredTx["from"])
	assert.Equal(t, fixtures.DynamicFeeChainID, filteredTx["chain_id"])
	assert.Equal(t, fixtures.DynamicFeeFeePerGas, filteredTx["max_fee_per_gas"])
	assert.Equal(t, fixtures.DynamicFeeTipPerGas, filteredTx["max_priority_fee_per_gas"])
}

func TestBlobTransaction(t *testing.T) {
	chainID := big.NewInt(10)
	key, err := crypto.HexToECDSA("dae2cb3b03f8a1bbaedae4d43e159360c8d07ffab119d5d7311a81a9d4f53bd1")
	assert.NoError(t, err)
	address := crypto.PubkeyToAddress(key.PublicKey)

	unsignedTx := ethtypes.NewTx(&ethtypes.BlobTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      1,
		GasTipCap:  uint256.NewInt(100),
		GasFeeCap:  uint256.NewInt(100),
		Gas:        0,
		To:         address,
		Value:      uint256.NewInt(1),
		Data:       []byte{},
		BlobFeeCap: uint256.NewInt(100),
		BlobHashes: []common.Hash{},
		Sidecar: &ethtypes.BlobTxSidecar{
			Blobs:       []kzg4844.Blob{},
			Commitments: []kzg4844.Commitment{},
			Proofs:      []kzg4844.Proof{},
		},
		AccessList: ethtypes.AccessList{},
		V:          &uint256.Int{},
		R:          &uint256.Int{},
		S:          &uint256.Int{},
	})

	signer := LatestSignerForChainID(chainID)
	hash := signer.Hash(unsignedTx)
	signature, signErr := crypto.Sign(hash.Bytes(), key)
	assert.NoError(t, signErr)
	signedTx, signTxErr := unsignedTx.WithSignature(signer, signature)
	assert.NoError(t, signTxErr)

	txBytes, encErr := rlp.EncodeToBytes(signedTx)
	assert.NoError(t, encErr)

	ethTx := NewEthTransactionFromBytes(txBytes, EmptySender)

	// check decoding transaction structure
	_, _ = ethTx.Fields(AllFieldsWithFrom)
	assert.Equal(t, "0x3", (*ethTx.fields.Load())["type"])
	assert.Equal(t, signedTx.Hash().Hex(), (*ethTx.fields.Load())["hash"])
	assert.Equal(t, hexutil.EncodeUint64(100), (*ethTx.fields.Load())["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(100), (*ethTx.fields.Load())["maxPriorityFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(100), (*ethTx.fields.Load())["maxFeePerBlobGas"])
	assert.Nil(t, (*ethTx.fields.Load())["gasPrice"])
	txChainID, err := ethTx.ChainID()
	assert.NoError(t, err)
	assert.Equal(t, chainID, txChainID)
	from, ferr := ethTx.From()
	assert.NoError(t, ferr)
	assert.NotNil(t, from)

	// check WithFields
	jsonMap, err := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.max_fee_per_blob_gas",
		"tx_contents.blob_versioned_hashes",
		"tx_contents.type",
	})
	assert.NoError(t, err)
	assert.Equal(t, "0x3", jsonMap["type"])
	assert.Equal(t, hexutil.EncodeUint64(100), jsonMap["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(100), jsonMap["maxPriorityFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(100), jsonMap["maxFeePerBlobGas"])
	assert.Equal(t, nil, jsonMap["gasPrice"])
	assert.Equal(t, hexutil.EncodeUint64(chainID.Uint64()), jsonMap["chainId"])
	assert.Empty(t, jsonMap["blobVersionedHashes"])

	// check Filters
	filteredTx, err := ethTx.Filters()
	assert.NoError(t, err)
	assert.Equal(t, "3", filteredTx["type"])
	assert.Equal(t, int(chainID.Int64()), filteredTx["chain_id"])
	assert.Equal(t, int(100), filteredTx["max_fee_per_gas"])
	assert.Equal(t, int(100), filteredTx["max_priority_fee_per_gas"])
	assert.Equal(t, int(100), filteredTx["max_fee_per_blob_gas"])

	// check EffectiveGasFeeCap and EffectiveGasTipCap
	effectiveGasFeeCap, err := ethTx.EffectiveGasFeeCap()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), effectiveGasFeeCap)
	effectiveGasTipCap, err := ethTx.EffectiveGasTipCap()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), effectiveGasTipCap)
	effectiveBlobGasFeeCap, err := ethTx.EffectiveBlobGasFeeCap()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), effectiveBlobGasFeeCap)

	// check RawTx
	rawTx, err := ethTx.RawTx()
	assert.NoError(t, err)
	assert.NotNil(t, rawTx)
	rawTxHex, err := ethTx.RawTxHex()
	assert.NoError(t, err)
	assert.Equal(t, hexutil.Encode(rawTx), rawTxHex)
}

func TestSetCodeTransaction(t *testing.T) {
	chainID := big.NewInt(10)
	key, err := crypto.HexToECDSA("dae2cb3b03f8a1bbaedae4d43e159360c8d07ffab119d5d7311a81a9d4f53bd1")
	assert.NoError(t, err)

	keyA, gerr := crypto.GenerateKey()
	assert.NoError(t, gerr)

	auth, authErr := ethtypes.SignSetCode(keyA, ethtypes.SetCodeAuthorization{
		ChainID: *uint256.MustFromBig(params.TestChainConfig.ChainID),
		Address: common.Address{0x42},
		Nonce:   0,
	})
	assert.NoError(t, authErr)

	address := crypto.PubkeyToAddress(key.PublicKey)
	unsignedTx := ethtypes.NewTx(&ethtypes.SetCodeTx{
		ChainID:    uint256.MustFromBig(chainID),
		Nonce:      1,
		GasTipCap:  uint256.NewInt(100),
		GasFeeCap:  uint256.NewInt(100),
		Gas:        0,
		To:         address,
		Value:      uint256.NewInt(1),
		Data:       []byte{},
		AccessList: ethtypes.AccessList{},
		AuthList:   []ethtypes.SetCodeAuthorization{auth},
		V:          &uint256.Int{},
		R:          &uint256.Int{},
		S:          &uint256.Int{},
	})

	signer := LatestSignerForChainID(chainID)
	hash := signer.Hash(unsignedTx)
	signature, signErr := crypto.Sign(hash.Bytes(), key)
	assert.NoError(t, signErr)
	signedTx, signTxErr := unsignedTx.WithSignature(signer, signature)
	assert.NoError(t, signTxErr)

	txBytes, encErr := rlp.EncodeToBytes(signedTx)
	assert.NoError(t, encErr)

	ethTx := NewEthTransactionFromBytes(txBytes, EmptySender)

	// verify transaction decodes
	decodedTx, dErr := ethTx.Tx()
	assert.NoError(t, dErr)
	assert.NotNil(t, decodedTx)

	// check decoding transaction structure
	_, _ = ethTx.Fields(AllFieldsWithFrom)
	assert.Equal(t, "0x4", (*ethTx.fields.Load())["type"])
	assert.Equal(t, signedTx.Hash().Hex(), (*ethTx.fields.Load())["hash"])
	assert.Equal(t, hexutil.EncodeUint64(100), (*ethTx.fields.Load())["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(100), (*ethTx.fields.Load())["maxPriorityFeePerGas"])
	assert.Nil(t, (*ethTx.fields.Load())["gasPrice"])
	txChainID, err := ethTx.ChainID()
	assert.NoError(t, err)
	assert.Equal(t, chainID, txChainID)
	from, ferr := ethTx.From()
	assert.NoError(t, ferr)
	assert.NotNil(t, from)

	// check authorization list is present
	authList := (*ethTx.fields.Load())["authorizationList"]
	assert.NotNil(t, authList)
	assert.IsType(t, []ethtypes.SetCodeAuthorization{}, authList)
	assert.Equal(t, 1, len(authList.([]ethtypes.SetCodeAuthorization)))

	// check WithFields
	jsonMap, err := ethTx.Fields([]string{
		"tx_contents.from",
		"tx_contents.tx_hash",
		"tx_contents.gas_price",
		"tx_contents.chain_id",
		"tx_contents.max_fee_per_gas",
		"tx_contents.max_priority_fee_per_gas",
		"tx_contents.type",
		"tx_contents.authorization_list",
	})
	assert.NoError(t, err)
	assert.Equal(t, "0x4", jsonMap["type"])
	assert.Equal(t, hexutil.EncodeUint64(100), jsonMap["maxFeePerGas"])
	assert.Equal(t, hexutil.EncodeUint64(100), jsonMap["maxPriorityFeePerGas"])
	assert.Equal(t, nil, jsonMap["gasPrice"])
	assert.Equal(t, hexutil.EncodeUint64(chainID.Uint64()), jsonMap["chainId"])

	// check Filters
	filteredTx, err := ethTx.Filters()
	assert.NoError(t, err)
	assert.Equal(t, "4", filteredTx["type"])
	assert.Equal(t, int(chainID.Int64()), filteredTx["chain_id"])
	assert.Equal(t, int(100), filteredTx["max_fee_per_gas"])
	assert.Equal(t, int(100), filteredTx["max_priority_fee_per_gas"])

	// check EffectiveGasFeeCap and EffectiveGasTipCap
	effectiveGasFeeCap, err := ethTx.EffectiveGasFeeCap()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), effectiveGasFeeCap)
	effectiveGasTipCap, err := ethTx.EffectiveGasTipCap()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), effectiveGasTipCap)

	// check RawTx
	rawTx, err := ethTx.RawTx()
	assert.NoError(t, err)
	assert.NotNil(t, rawTx)
	rawTxHex, err := ethTx.RawTxHex()
	assert.NoError(t, err)
	assert.Equal(t, hexutil.Encode(rawTx), rawTxHex)
}

func TestContractCreationTx(t *testing.T) {
	hash, ethTx, err := ethTransaction(fixtures.ContractCreationTxHash, fixtures.ContractCreationTx)
	assert.NoError(t, err)
	_, _ = ethTx.Fields([]string{})
	filters, err := ethTx.Filters()
	assert.NoError(t, err)
	assert.Equal(t, "0x0", filters["to"])

	assert.Equal(t, "0x"+hash.String(), (*ethTx.fields.Load())["hash"])

	ethJSON, err := ethTx.Fields([]string{"tx_contents.to", "tx_contents.from"})
	assert.NoError(t, err)

	to, ok := ethJSON["to"]
	assert.Equal(t, false, ok)
	assert.Equal(t, nil, to)
	assert.Equal(t, "0x09e9ff67d9d5a25fa465db6f0bede5560581f8cb", ethJSON["from"])
}

func TestEthTransactionRawTxLazy(t *testing.T) {
	_, ethTx, err := ethTransaction(fixtures.LegacyTransactionHash, fixtures.LegacyTransaction)
	assert.NoError(t, err)

	tx, err := ethTx.Tx()
	assert.NoError(t, err)

	expectedBinary, err := tx.MarshalBinary()
	assert.NoError(t, err)

	rawTx, err := ethTx.RawTx()
	assert.NoError(t, err)
	assert.Equal(t, expectedBinary, rawTx)
	rawTxHex, err := ethTx.RawTxHex()
	assert.NoError(t, err)
	assert.Equal(t, hexutil.Encode(expectedBinary), rawTxHex)
}

func TestEthTransactionSetRawTx(t *testing.T) {
	content, err := hex.DecodeString(fixtures.LegacyTransaction)
	assert.NoError(t, err)

	ethTx := NewEthTransactionFromBytes(content, EmptySender)

	// simulates cloud-api type 3 txs: raw tx from the feed (including blob sidecar)
	// must be returned verbatim instead of being re-marshaled from sidecar-less content
	seededRawTx := "0x03deadbeef"
	ethTx.SetRawTx(seededRawTx)

	rawTxHex, err := ethTx.RawTxHex()
	assert.NoError(t, err)
	assert.Equal(t, seededRawTx, rawTxHex)
	rawTx, err := ethTx.RawTx()
	assert.NoError(t, err)
	assert.Equal(t, hexutil.MustDecode(seededRawTx), rawTx)
}

// TestEthTransactionRawTxHexConcurrent is a regression test: RawTxHex must never
// return "" when many goroutines race on a fresh transaction (previously a loser
// of the binary CAS could observe binary set but hexTx still unset).
func TestEthTransactionRawTxHexConcurrent(t *testing.T) {
	content, err := hex.DecodeString(fixtures.LegacyTransaction)
	require.NoError(t, err)

	expected, err := NewEthTransactionFromBytes(content, EmptySender).RawTxHex()
	require.NoError(t, err)
	require.NotEmpty(t, expected)

	goroutines := 2 * runtime.GOMAXPROCS(0)
	for i := 0; i < 5000; i++ {
		ethTx := NewEthTransactionFromBytes(content, EmptySender)

		var (
			start   sync.WaitGroup
			wg      sync.WaitGroup
			results = make([]string, goroutines)
		)
		start.Add(1)
		wg.Add(goroutines)
		for g := 0; g < goroutines; g++ {
			go func(idx int) {
				defer wg.Done()
				start.Wait() // maximize simultaneous entry
				if idx%2 == 0 {
					_, _ = ethTx.RawTx()
				}
				results[idx], _ = ethTx.RawTxHex()
			}(g)
		}
		start.Done()
		wg.Wait()

		for idx, r := range results {
			require.Equalf(t, expected, r, "iteration %d goroutine %d", i, idx)
		}
	}
}
