package services

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const bytesPerBlob = 131072 + // Blob
	48 + // KgzCommitment
	48 // Proof

var (
	privateKey, _     = crypto.GenerateKey()
	blockchainNetwork = sdnmessage.BlockchainNetwork{
		AllowTimeReuseSenderNonce:           2,
		AllowGasPriceChangeReuseSenderNonce: 1.1,
		EnableCheckSenderNonce:              true,
		MaxTxAgeSeconds:                     30,
	}
)

func newTestBloomFilter(t *testing.T) BloomFilter {
	bf, err := NewBloomFilter(context.Background(), clock.RealClock{}, time.Hour, "", 1e6, 1000)
	require.NoError(t, err)
	return bf
}

func TestEthTxStore_InvalidChainID(t *testing.T) {
	store := NewEthTxStore(&clock.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage(), false)
	hash := types.SHA256Hash{1}
	tx := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil)
	content, _ := rlp.EncodeToBytes(&tx)

	// first time seeing valid tx
	result1 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.False(t, result1.FailedValidation)
	assert.False(t, result1.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 1, store.Count())
}

func makeSidecar(blobCount int) *ethtypes.BlobTxSidecar {
	blobs := make([]kzg4844.Blob, blobCount)
	var commitments []kzg4844.Commitment
	var proofs []kzg4844.Proof
	for i := 0; i < blobCount; i++ {
		commitments = append(commitments, kzg4844.Commitment{})

		proofs = append(proofs, kzg4844.Proof{})
	}

	return &ethtypes.BlobTxSidecar{
		Blobs:       blobs,
		Commitments: commitments,
		Proofs:      proofs,
	}
}

func addTx(t *testing.T, txs []*ethtypes.Transaction, blobCount int, store *EthTxStore, expTxIdxs []int, nonce uint64, txType uint8) []*ethtypes.Transaction {
	var tx *ethtypes.Transaction
	if txType == ethtypes.BlobTxType {
		tx = bxmock.NewSignedEthBlobTxWithSidecar(nonce, nil, nil, makeSidecar(blobCount))
	} else {
		tx = bxmock.NewSignedEthTx(txType, nonce, privateKey, nil)
	}

	content, _ := rlp.EncodeToBytes(&tx)

	result := store.Add(types.SHA256Hash(tx.Hash()), content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)
	assert.True(t, result.NewTx)
	assert.True(t, result.NewContent)
	assert.False(t, result.NewSID)
	assert.False(t, result.FailedValidation)
	assert.False(t, result.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, tx.Type() == ethtypes.BlobTxType, result.Transaction.Flags().IsWithSidecar())

	txs = append(txs, tx)

	expTxs := make([]*ethtypes.Transaction, 0)
	for _, idx := range expTxIdxs {
		expTxs = append(expTxs, txs[idx])
	}

	assertTxsInStore(t, expTxs, store)

	return txs
}

func assertTxsInStore(t *testing.T, txs []*ethtypes.Transaction, store *EthTxStore) {
	require.Equal(t, len(txs), store.Count())

	for _, tx := range txs {
		hash, err := types.NewSHA256Hash(tx.Hash().Bytes())
		assert.NoError(t, err)

		_, exists := store.Get(hash)
		require.True(t, exists)
	}
}

func TestEthTxStore_CleanBlobsTxs(t *testing.T) {
	maxBlobTxs := 5
	blobsPerTx := 2
	maxBlobsCount := blobsPerTx * maxBlobTxs
	txSizeWithoutSidecar := 196

	blockchainNetwork := sdnmessage.BlockchainNetwork{
		AllowTimeReuseSenderNonce:           2,
		AllowGasPriceChangeReuseSenderNonce: 1.1,
		EnableCheckSenderNonce:              true,
		MaxTxAgeSeconds:                     30,
		MaxTotalBlobTxSizeBytes:             uint64(maxBlobsCount)*uint64(bytesPerBlob) + uint64(maxBlobTxs*txSizeWithoutSidecar),
	}

	store := NewEthTxStore(&clock.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage(), true)

	txs := make([]*ethtypes.Transaction, 0)
	expTxs := make([]int, 0)

	addExpTxs := func(expTx int) []int {
		expTxs = append(expTxs, expTx)

		return expTxs
	}

	removeExpTxs := func(expTx ...int) []int {
		for i := len(expTx) - 1; i >= 0; i-- {
			idx := expTx[i]
			expTxs = append(expTxs[:idx], expTxs[idx+1:]...)
		}

		return expTxs
	}

	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(0), 0, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(1), 1, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(2), 2, ethtypes.BlobTxType) // will be removed
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(3), 3, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(4), 4, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(5), 5, ethtypes.BlobTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(6), 6, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(7), 7, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(8), 8, ethtypes.BlobTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(9), 9, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(10), 10, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(11), 11, ethtypes.BlobTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(12), 12, ethtypes.AccessListTxType)
	txs = addTx(t, txs, blobsPerTx, store, addExpTxs(13), 13, ethtypes.AccessListTxType)

	removeExpTxs(2)

	_ = addTx(t, txs, blobsPerTx, store, addExpTxs(14), 14, ethtypes.BlobTxType)
}

func TestEthTxStore_AddBlobTx(t *testing.T) {
	store := NewEthTxStore(&clock.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage(), false)
	hash := types.SHA256Hash{1}

	// add valid blob type transaction
	tx := bxmock.NewSignedEthTx(ethtypes.BlobTxType, 1, privateKey, nil)
	content, _ := rlp.EncodeToBytes(&tx)
	result1 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)

	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.False(t, result1.FailedValidation)
	assert.False(t, result1.Transaction.Flags().IsReuseSenderNonce())
	assert.True(t, result1.Transaction.Flags().IsWithSidecar())
	assert.Equal(t, 1, store.Count())
}

func TestEthTxStore_Add(t *testing.T) {
	store := NewEthTxStore(&clock.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage(), false)
	hash := types.SHA256Hash{1}
	tx := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil)
	content, _ := rlp.EncodeToBytes(&tx)

	// ignore, transaction is a duplicate
	result2 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewContent)
	assert.False(t, result2.NewSID)
	assert.False(t, result2.FailedValidation)
	assert.False(t, result2.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 1, store.Count())

	// add short ID for already seen tx
	result3 := store.Add(hash, content, 1, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)
	assert.False(t, result3.NewTx)
	assert.False(t, result3.NewContent)
	assert.True(t, result3.NewSID)
	assert.False(t, result3.FailedValidation)
	assert.False(t, result3.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 1, store.Count())

	// add invalid transaction - should not be added but should get to History
	tx = bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, nil)
	content, _ = rlp.EncodeToBytes(&tx)
	hash = types.SHA256Hash{2}

	result1 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result1.FailedValidation)
	assert.Equal(t, 1, store.Count())

	// add it again. should fail validation again
	result3 = store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result3.NewTx)
	assert.False(t, result3.NewContent)
	assert.False(t, result3.NewSID)
	assert.True(t, result3.FailedValidation)
	assert.False(t, result3.Transaction.Flags().IsReuseSenderNonce())

	assert.Equal(t, 1, store.Count())

	// add valid blob type transaction
	tx = bxmock.NewSignedEthTx(ethtypes.BlobTxType, 3, privateKey, nil)
	content, _ = rlp.EncodeToBytes(&tx)
	hash = types.SHA256Hash{3}

	result4 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)

	assert.True(t, result4.NewTx)
	assert.True(t, result4.NewContent)
	assert.False(t, result4.NewSID)
	assert.False(t, result4.FailedValidation)
	assert.False(t, result4.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 2, store.Count())

	// add valid blob type transaction with same nonce
	tx = bxmock.NewSignedEthTx(ethtypes.BlobTxType, 1, privateKey, nil)
	content, _ = rlp.EncodeToBytes(&tx)
	hash = types.SHA256Hash{4}

	result5 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64(), types.EmptySender)

	assert.True(t, result5.NewTx)
	assert.True(t, result5.NewContent)
	assert.False(t, result5.NewSID)
	assert.False(t, result5.FailedValidation)
	assert.True(t, result5.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 3, store.Count())
}

func TestEthTxStore_AddReuseSenderNonce(t *testing.T) {
	mc := clock.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 10
	store := NewEthTxStore(&mc, 30*time.Second, 20*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &nc}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage(), false)

	// original transaction
	hash1 := types.SHA256Hash{1}
	tx1 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil)
	content1, _ := rlp.EncodeToBytes(&tx1)

	// transaction that reuses nonce
	hash2 := types.SHA256Hash{2}
	tx2 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil)
	content2, _ := rlp.EncodeToBytes(&tx2)

	// incremented nonce
	hash3 := types.SHA256Hash{3}
	tx3 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, nil)
	content3, _ := rlp.EncodeToBytes(&tx3)

	// different sender
	privateKey2, _ := crypto.GenerateKey()
	hash4 := types.SHA256Hash{4}
	tx4 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey2, nil)
	content4, _ := rlp.EncodeToBytes(&tx4)

	// add original transaction
	result0 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx1.ChainId().Int64(), types.EmptySender)
	assert.Equal(t, 1, store.Count())
	result0.Transaction.SetAddTime(mc.Now())
	assert.False(t, result0.Transaction.Flags().IsReuseSenderNonce())
	// result0.Transaction.MarkProcessed()

	// add transaction with same nonce/sender, should be notified and not added to tx store
	result1 := store.Add(hash2, content2, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx2.ChainId().Int64(), types.EmptySender)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.False(t, result1.FailedValidation)
	assert.True(t, result1.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 2, store.Count())
	result1.Transaction.SetAddTime(mc.Now())

	// add transaction with incremented nonce
	result2 := store.Add(hash3, content3, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx3.ChainId().Int64(), types.EmptySender)
	assert.Equal(t, 3, store.Count())
	assert.False(t, result2.Transaction.Flags().IsReuseSenderNonce())
	result2.Transaction.SetAddTime(mc.Now())

	// add transaction with different sender
	result3 := store.Add(hash4, content4, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx4.ChainId().Int64(), types.EmptySender)
	assert.Equal(t, 4, store.Count())
	assert.False(t, result3.Transaction.Flags().IsReuseSenderNonce())
	result3.Transaction.SetAddTime(mc.Now())

	// // Hash2 is already in txstore
	mc.IncTime(time.Duration(1+nc.AllowTimeReuseSenderNonce) * time.Second)
	result2 = store.Add(hash2, content2, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx2.ChainId().Int64(), types.EmptySender)
	assert.False(t, result2.NewTx)
	assert.False(t, result2.NewContent)
	assert.False(t, result2.NewSID)
	assert.False(t, result2.FailedValidation)
	assert.True(t, result2.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 4, store.Count())

	// clean tx without shortID - we clean all tx excluding hash2 that was added 11  seconds ago
	mc.IncTime(11 * time.Second)
	cleaned, cleanedShortIDs := store.BxTxStore.clean()
	assert.Equal(t, 0, len(cleanedShortIDs[testNetworkNum]))
	assert.Equal(t, 4, cleaned)
	// // clean tx without shortID - now we should clean hash2
	// mc.IncTime(11 * time.Second)
	// cleaned, cleanedShortIDs = store.BxTxStore.clean()
	// assert.Equal(t, 0, len(cleanedShortIDs[testNetworkNum]))
	// assert.Equal(t, 1, cleaned)

	// still, can't add it back due to history
	mc.IncTime(11 * time.Second)
	result2 = store.Add(hash2, content2, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx2.ChainId().Int64(), types.EmptySender)
	assert.False(t, result2.NewTx)
	assert.False(t, result2.NewContent)
	assert.False(t, result2.NewSID)
	assert.False(t, result2.FailedValidation)
	assert.False(t, result2.Transaction.Flags().IsReuseSenderNonce())
	assert.True(t, result2.AlreadySeen)
	assert.Equal(t, 0, store.Count())
}

func TestEthTxStore_AddInvalidTx(t *testing.T) {
	store := NewEthTxStore(&clock.MockClock{}, 30*time.Second, 10*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{blockchainNetwork.NetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage(), false)
	hash := types.SHA256Hash{1}
	content := types.TxContent{1, 2, 3}

	// invalid tx
	result := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result.NewTx)
	assert.False(t, result.NewContent)
	assert.False(t, result.NewSID)
	assert.True(t, result.FailedValidation)
	assert.False(t, result.Transaction.Flags().IsReuseSenderNonce())
	assert.Equal(t, 0, store.Count())
}

func newEthTransaction(nonce uint64, gasFee, gasTip int64) (*types.EthTransaction, error) {
	to := common.HexToAddress("0x12345")

	rawTx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:    big.NewInt(testChainID),
		Nonce:      nonce,
		GasFeeCap:  big.NewInt(gasFee),
		GasTipCap:  big.NewInt(gasTip),
		Gas:        1000,
		To:         &to,
		Value:      big.NewInt(100),
		Data:       []byte{},
		AccessList: nil,
	})

	// Sign the transaction with same private key
	signedTx, err := ethtypes.SignTx(rawTx, types.LatestSignerForChainID(rawTx.ChainId()), privateKey)
	if err != nil {
		return nil, err
	}

	return types.NewEthTransaction(signedTx, types.EmptySender)
}

func newBlobTypeTransaction(nonce uint64, gasFee, gasTip, blobFeeCap uint64) (*types.EthTransaction, error) {
	to := common.HexToAddress("0x12345")

	rawTx := ethtypes.NewTx(&ethtypes.BlobTx{
		ChainID:    uint256.NewInt(uint64(testChainID)),
		Nonce:      nonce,
		GasFeeCap:  uint256.NewInt(gasFee),
		GasTipCap:  uint256.NewInt(gasTip),
		BlobFeeCap: uint256.NewInt(blobFeeCap),
		Gas:        1000,
		To:         to,
		Value:      uint256.NewInt(100),
		Data:       []byte{},
		AccessList: nil,
		BlobHashes: []common.Hash{
			common.HexToHash("0x12345"),
		},
	})

	// Sign the transaction with same private key
	signedTx, err := ethtypes.SignTx(rawTx, types.LatestSignerForChainID(rawTx.ChainId()), privateKey)
	if err != nil {
		return nil, err
	}

	return types.NewEthTransaction(signedTx, types.EmptySender)
}

func newSetCodeTransaction(nonce uint64, gasFee, gasTip uint64) (*types.EthTransaction, error) {
	keyA, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	to := common.HexToAddress("0x12345")

	auth, err := ethtypes.SignSetCode(keyA, ethtypes.SetCodeAuthorization{
		ChainID: *uint256.MustFromBig(params.TestChainConfig.ChainID),
		Address: common.Address{0x42},
		Nonce:   0,
	})
	if err != nil {
		return nil, err
	}

	rawTx := ethtypes.NewTx(&ethtypes.SetCodeTx{
		ChainID:   uint256.NewInt(uint64(testChainID)),
		Nonce:     nonce,
		GasFeeCap: uint256.NewInt(gasFee),
		GasTipCap: uint256.NewInt(gasTip),
		Gas:       1000,
		To:        to,
		Value:     uint256.NewInt(100),
		Data:      []byte{},
		AuthList:  []ethtypes.SetCodeAuthorization{auth},
	})

	// Sign the transaction with same private key
	signedTx, err := ethtypes.SignTx(rawTx, types.LatestSignerForChainID(rawTx.ChainId()), privateKey)
	if err != nil {
		return nil, err
	}

	return types.NewEthTransaction(signedTx, types.EmptySender)
}

func TestNonceTracker_track(t *testing.T) {
	c := clock.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 1
	nc.NetworkNum = testNetworkNum
	n := newNonceTracker(&c, sdnmessage.BlockchainNetworks{nc.NetworkNum: &nc}, 10)
	nonce := uint64(1)

	tx, err := newEthTransaction(nonce, 100, 100)
	require.NoError(t, err)

	txSame, err := newEthTransaction(nonce, 100, 100)
	require.NoError(t, err)

	txLowerGas, err := newEthTransaction(nonce, 5, 5)
	require.NoError(t, err)

	txSlightlyHigherGas, err := newEthTransaction(nonce, 101, 101)
	require.NoError(t, err)

	txHigherGas, err := newEthTransaction(nonce, 111, 1111)
	require.NoError(t, err)

	duplicate, _, err := n.track(tx, testNetworkNum)
	require.NoError(t, err)
	assert.False(t, duplicate)

	duplicate, _, err = n.track(txSame, testNetworkNum)
	assert.NoError(t, err)
	assert.True(t, duplicate)

	duplicate, _, err = n.track(txLowerGas, testNetworkNum)
	assert.NoError(t, err)
	assert.True(t, duplicate)

	duplicate, _, err = n.track(txSlightlyHigherGas, testNetworkNum)
	assert.NoError(t, err)
	assert.True(t, duplicate)

	duplicate, _, err = n.track(txHigherGas, testNetworkNum)
	assert.NoError(t, err)
	assert.False(t, duplicate)

	c.IncTime(5 * time.Second)
	duplicate, _, err = n.track(txLowerGas, testNetworkNum)
	assert.NoError(t, err)
	assert.False(t, duplicate)
}

func TestNonceTrackerBlobTx_track(t *testing.T) {
	c := clock.MockClock{}

	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 1
	nc.NetworkNum = testNetworkNum
	n := newNonceTracker(&c, sdnmessage.BlockchainNetworks{nc.NetworkNum: &nc}, 10)
	nonce := uint64(1)

	tx, err := newBlobTypeTransaction(nonce, 100, 100, 100)
	require.NoError(t, err)

	txSame, err := newBlobTypeTransaction(nonce, 100, 100, 100)
	require.NoError(t, err)

	txLowerGas, err := newBlobTypeTransaction(nonce, 5, 5, 5)
	require.NoError(t, err)

	txSlightlyHigherGas, err := newBlobTypeTransaction(nonce, 101, 101, 101)
	require.NoError(t, err)

	txHigherGas, err := newBlobTypeTransaction(nonce, 111, 1111, 1111)
	require.NoError(t, err)

	duplicate, _, err := n.track(tx, testNetworkNum)
	require.NoError(t, err)

	require.False(t, duplicate)

	duplicate, _, err = n.track(txSame, testNetworkNum)
	require.NoError(t, err)
	require.True(t, duplicate)

	duplicate, _, err = n.track(txLowerGas, testNetworkNum)
	require.NoError(t, err)
	require.True(t, duplicate)

	duplicate, _, err = n.track(txSlightlyHigherGas, testNetworkNum)
	require.NoError(t, err)

	require.True(t, duplicate)

	duplicate, _, err = n.track(txHigherGas, testNetworkNum)
	require.NoError(t, err)
	require.False(t, duplicate)

	c.IncTime(5 * time.Second)
	duplicate, _, err = n.track(txLowerGas, testNetworkNum)
	require.NoError(t, err)
	require.False(t, duplicate)
}

func TestNonceTrackerSetCodeTx_track(t *testing.T) {
	c := clock.MockClock{}

	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 1
	nc.NetworkNum = testNetworkNum
	n := newNonceTracker(&c, sdnmessage.BlockchainNetworks{nc.NetworkNum: &nc}, 10)
	nonce := uint64(1)

	tx, err := newSetCodeTransaction(nonce, 100, 100)
	require.NoError(t, err)

	txSame, err := newSetCodeTransaction(nonce, 100, 100)
	require.NoError(t, err)

	txLowerGas, err := newSetCodeTransaction(nonce, 5, 5)
	require.NoError(t, err)

	txSlightlyHigherGas, err := newSetCodeTransaction(nonce, 101, 101)
	require.NoError(t, err)

	txHigherGas, err := newSetCodeTransaction(nonce, 111, 1111)
	require.NoError(t, err)

	duplicate, _, err := n.track(tx, testNetworkNum)
	require.NoError(t, err)

	require.False(t, duplicate)

	duplicate, _, err = n.track(txSame, testNetworkNum)
	require.NoError(t, err)
	require.True(t, duplicate)

	duplicate, _, err = n.track(txLowerGas, testNetworkNum)
	require.NoError(t, err)
	require.True(t, duplicate)

	duplicate, _, err = n.track(txSlightlyHigherGas, testNetworkNum)
	require.NoError(t, err)

	require.True(t, duplicate)

	duplicate, _, err = n.track(txHigherGas, testNetworkNum)
	require.NoError(t, err)
	require.False(t, duplicate)

	c.IncTime(5 * time.Second)
	duplicate, _, err = n.track(txLowerGas, testNetworkNum)
	require.NoError(t, err)
	require.False(t, duplicate)
}

func TestNonceTracker_clean(t *testing.T) {
	c := clock.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 1
	nc.NetworkNum = testNetworkNum
	n := newNonceTracker(&c, sdnmessage.BlockchainNetworks{nc.NetworkNum: &nc}, 10)

	tx, err := newEthTransaction(1, 100, 100)
	require.NoError(t, err)

	tx2, err := newEthTransaction(2, 100, 100)
	require.NoError(t, err)

	tx3, err := newEthTransaction(3, 100, 100)
	require.NoError(t, err)

	tx4, err := newBlobTypeTransaction(4, 100, 100, 100)
	require.NoError(t, err)

	n.track(tx, testNetworkNum)
	c.IncTime(500 * time.Millisecond)

	n.track(tx2, testNetworkNum)
	c.IncTime(500 * time.Millisecond)

	n.track(tx3, testNetworkNum)
	c.IncTime(100 * time.Millisecond)

	n.track(tx4, testNetworkNum)
	c.IncTime(100 * time.Millisecond)

	n.clean()

	fromTx1, err := tx.From()
	require.NoError(t, err)

	rtx, ok := n.getTransaction(fromTx1, 1)
	assert.Nil(t, rtx)
	assert.False(t, ok)

	fromTx2, err := tx2.From()
	require.NoError(t, err)

	rtx, ok = n.getTransaction(fromTx2, 2)
	assert.Equal(t, tx2.Hash(), rtx.hash)
	assert.True(t, ok)

	fromTx3, err := tx3.From()
	require.NoError(t, err)

	rtx, ok = n.getTransaction(fromTx3, 3)
	assert.Equal(t, tx3.Hash(), rtx.hash)
	assert.True(t, ok)

	fromTx4, err := tx4.From()
	require.NoError(t, err)

	rtx, ok = n.getTransaction(fromTx4, 4)
	assert.Equal(t, tx4.Hash(), rtx.hash)
	assert.True(t, ok)

	c.IncTime(500 * time.Millisecond)
	n.clean()

	rtx, ok = n.getTransaction(fromTx2, 2)
	assert.Nil(t, rtx)
	assert.False(t, ok)

	rtx, ok = n.getTransaction(fromTx3, 3)
	assert.Equal(t, tx3.Hash(), rtx.hash)
	assert.True(t, ok)

	rtx, ok = n.getTransaction(fromTx4, 4)
	assert.Equal(t, tx4.Hash(), rtx.hash)
	assert.True(t, ok)

	c.IncTime(500 * time.Millisecond)
	n.clean()

	rtx, ok = n.getTransaction(fromTx3, 3)
	assert.Nil(t, rtx)
	assert.False(t, ok)

	rtx, ok = n.getTransaction(fromTx4, 4)
	assert.Nil(t, rtx)
	assert.False(t, ok)
}
