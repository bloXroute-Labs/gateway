package services

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	bf, err := NewBloomFilter(context.Background(), utils.RealClock{}, time.Hour, "", 1e6, 1000)
	require.NoError(t, err)
	return bf
}

func TestEthTxStore_InvalidChainID(t *testing.T) {
	store := NewEthTxStore(&utils.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage())
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

func TestEthTxStore_AddBlobTx(t *testing.T) {
	store := NewEthTxStore(&utils.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage())
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
	store := NewEthTxStore(&utils.MockClock{}, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage())
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
	mc := utils.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 10
	store := NewEthTxStore(&mc, 30*time.Second, 20*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &nc}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage())

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

	//// Hash2 is already in txstore
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
	//// clean tx without shortID - now we should clean hash2
	//mc.IncTime(11 * time.Second)
	//cleaned, cleanedShortIDs = store.BxTxStore.clean()
	//assert.Equal(t, 0, len(cleanedShortIDs[testNetworkNum]))
	//assert.Equal(t, 1, cleaned)

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
	store := NewEthTxStore(&utils.MockClock{}, 30*time.Second, 10*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{blockchainNetwork.NetworkNum: &blockchainNetwork}, newTestBloomFilter(t), NewNoOpBlockCompressorStorage())
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
	signedTx, err := ethtypes.SignTx(rawTx, ethtypes.NewCancunSigner(rawTx.ChainId()), privateKey)
	if err != nil {
		return nil, err
	}

	return types.NewEthTransaction(types.SHA256Hash(signedTx.Hash()), signedTx, types.EmptySender)
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
	signedTx, err := ethtypes.SignTx(rawTx, ethtypes.NewCancunSigner(rawTx.ChainId()), privateKey)
	if err != nil {
		return nil, err
	}

	return types.NewEthTransaction(types.SHA256Hash(signedTx.Hash()), signedTx, types.EmptySender)
}

func TestNonceTracker_track(t *testing.T) {
	c := utils.MockClock{}
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
	c := utils.MockClock{}

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

func TestNonceTracker_clean(t *testing.T) {
	c := utils.MockClock{}
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
