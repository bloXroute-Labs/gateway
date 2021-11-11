package services

import (
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

var privateKey, _ = crypto.GenerateKey()
var blockchainNetwork = sdnmessage.BlockchainNetwork{
	AllowTimeReuseSenderNonce:           2,
	AllowGasPriceChangeReuseSenderNonce: 1.1,
	EnableCheckSenderNonce:              true,
}

func TestEthTxStore_InvalidChainID(t *testing.T) {
	store := NewEthTxStore(&bxmock.MockClock{}, 30*time.Second, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork})
	hash := types.SHA256Hash{1}
	tx := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey)
	content, _ := rlp.EncodeToBytes(&tx)

	// first time seeing valid tx
	result1 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64())
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.False(t, result1.FailedValidation)
	assert.False(t, result1.ReuseSenderNonce)
	assert.Equal(t, 1, store.Count())

}

func TestEthTxStore_Add(t *testing.T) {
	store := NewEthTxStore(&bxmock.MockClock{}, 30*time.Second, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork})
	hash := types.SHA256Hash{1}
	tx := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey)
	content, _ := rlp.EncodeToBytes(&tx)

	// first time seeing valid tx
	result1 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result1.FailedValidation)
	assert.Equal(t, 0, store.Count())

	// ignore, transaction is a duplicate
	result2 := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64())
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewContent)
	assert.False(t, result2.NewSID)
	assert.False(t, result2.FailedValidation)
	assert.False(t, result2.ReuseSenderNonce)
	assert.Equal(t, 1, store.Count())

	// add short ID for already seen tx
	result3 := store.Add(hash, content, 1, testNetworkNum, true, types.TFPaidTx, time.Now(), tx.ChainId().Int64())
	assert.False(t, result3.NewTx)
	assert.False(t, result3.NewContent)
	assert.True(t, result3.NewSID)
	assert.False(t, result3.FailedValidation)
	assert.False(t, result3.ReuseSenderNonce)
	assert.Equal(t, 1, store.Count())

}

func TestEthTxStore_AddReuseSenderNonce(t *testing.T) {
	mc := bxmock.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 10
	store := NewEthTxStore(&mc, 30*time.Second, 30*time.Second, 20*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &nc})

	// original transaction
	hash1 := types.SHA256Hash{1}
	tx1 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey)
	content1, _ := rlp.EncodeToBytes(&tx1)

	// transaction that reuses nonce
	hash2 := types.SHA256Hash{2}
	tx2 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey)
	content2, _ := rlp.EncodeToBytes(&tx2)

	// incremented nonce
	hash3 := types.SHA256Hash{3}
	tx3 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey)
	content3, _ := rlp.EncodeToBytes(&tx3)

	// different sender
	privateKey2, _ := crypto.GenerateKey()
	hash4 := types.SHA256Hash{4}
	tx4 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey2)
	content4, _ := rlp.EncodeToBytes(&tx4)

	// add original transaction
	result0 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx1.ChainId().Int64())
	assert.Equal(t, 1, store.Count())
	result0.Transaction.SetAddTime(mc.Now())
	//result0.Transaction.MarkProcessed()

	// add transaction with same nonce/sender, should be notified and not added to tx store
	result1 := store.Add(hash2, content2, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx2.ChainId().Int64())
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.False(t, result1.FailedValidation)
	assert.True(t, result1.ReuseSenderNonce)
	assert.Equal(t, 1, store.Count())
	result1.Transaction.SetAddTime(mc.Now())

	// add transaction with incremented nonce
	result2 := store.Add(hash3, content3, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx3.ChainId().Int64())
	assert.Equal(t, 2, store.Count())
	result2.Transaction.SetAddTime(mc.Now())

	// add transaction with different sender
	result3 := store.Add(hash4, content4, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx4.ChainId().Int64())
	assert.Equal(t, 3, store.Count())
	result3.Transaction.SetAddTime(mc.Now())

	// time has elapsed, ok now
	mc.IncTime(11 * time.Second)
	result2 = store.Add(hash2, content2, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx2.ChainId().Int64())
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewContent)
	assert.False(t, result2.NewSID)
	assert.False(t, result2.FailedValidation)
	assert.False(t, result2.ReuseSenderNonce)
	assert.Equal(t, 4, store.Count())

	// clean tx without shortID - we clean all tx excluding hash2 that was added 11  seconds ago
	mc.IncTime(11 * time.Second)
	cleaned, cleanedShortIDs := store.BxTxStore.clean()
	assert.Equal(t, 0, len(cleanedShortIDs[testNetworkNum]))
	assert.Equal(t, 3, cleaned)
	// clean tx without shortID - now we should clean hash2
	mc.IncTime(11 * time.Second)
	cleaned, cleanedShortIDs = store.BxTxStore.clean()
	assert.Equal(t, 0, len(cleanedShortIDs[testNetworkNum]))
	assert.Equal(t, 1, cleaned)

	// now, should be able to add it back
	mc.IncTime(11 * time.Second)
	result2 = store.Add(hash2, content2, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, mc.Now(), tx2.ChainId().Int64())
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewContent)
	assert.False(t, result2.NewSID)
	assert.False(t, result2.FailedValidation)
	assert.False(t, result2.ReuseSenderNonce)
	assert.Equal(t, 1, store.Count())

}

func TestEthTxStore_AddInvalidTx(t *testing.T) {
	store := NewEthTxStore(&bxmock.MockClock{}, 30*time.Second, 30*time.Second, 10*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{blockchainNetwork.NetworkNum: &blockchainNetwork})
	hash := types.SHA256Hash{1}
	content := types.TxContent{1, 2, 3}

	// invalid tx
	result := store.Add(hash, content, types.ShortIDEmpty, testNetworkNum, true, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result.NewTx)
	assert.True(t, result.NewContent)
	assert.False(t, result.NewSID)
	assert.True(t, result.FailedValidation)
	assert.False(t, result.ReuseSenderNonce)
	assert.Equal(t, 1, store.Count())
}

func TestNonceTracker_track(t *testing.T) {
	c := bxmock.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 1
	nc.NetworkNum = testNetworkNum
	n := newNonceTracker(&c, sdnmessage.BlockchainNetworks{nc.NetworkNum: &nc}, 10)
	var fromBytes common.Address
	rand.Read(fromBytes[:])
	address := types.EthAddress{Address: &fromBytes}

	tx := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(100)},
	}
	txSame := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(100)},
	}
	txLowerGas := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(5)},
	}
	txSlightlyHigherGas := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(101)},
	}
	txHigherGas := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(111)},
	}

	duplicate, _ := n.track(&tx, testNetworkNum)
	assert.False(t, duplicate)

	duplicate, _ = n.track(&txSame, testNetworkNum)
	assert.True(t, duplicate)

	duplicate, _ = n.track(&txLowerGas, testNetworkNum)
	assert.True(t, duplicate)

	duplicate, _ = n.track(&txSlightlyHigherGas, testNetworkNum)
	assert.True(t, duplicate)

	duplicate, _ = n.track(&txHigherGas, testNetworkNum)
	assert.False(t, duplicate)

	c.IncTime(5 * time.Second)
	duplicate, _ = n.track(&txLowerGas, testNetworkNum)
	assert.False(t, duplicate)
}

func TestNonceTracker_clean(t *testing.T) {
	c := bxmock.MockClock{}
	nc := blockchainNetwork
	nc.AllowTimeReuseSenderNonce = 1
	nc.NetworkNum = testNetworkNum
	n := newNonceTracker(&c, sdnmessage.BlockchainNetworks{nc.NetworkNum: &nc}, 10)
	var fromBytes common.Address
	rand.Read(fromBytes[:])
	address := types.EthAddress{Address: &fromBytes}

	tx := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(100)},
		Nonce:    types.EthUInt64{UInt64: 1},
	}
	tx2 := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(100)},
		Nonce:    types.EthUInt64{UInt64: 2},
	}
	tx3 := types.EthTransaction{
		From:     address,
		GasPrice: types.EthBigInt{Int: big.NewInt(100)},
		Nonce:    types.EthUInt64{UInt64: 3},
	}

	n.track(&tx, testNetworkNum)
	c.IncTime(500 * time.Millisecond)

	n.track(&tx2, testNetworkNum)
	c.IncTime(500 * time.Millisecond)

	n.track(&tx3, testNetworkNum)
	c.IncTime(100 * time.Millisecond)

	n.clean()

	rtx, ok := n.getTransaction(address, types.EthUInt64{UInt64: 1})
	assert.Nil(t, rtx)
	assert.False(t, ok)

	rtx, ok = n.getTransaction(address, types.EthUInt64{UInt64: 2})
	assert.Equal(t, &tx2, rtx.tx)
	assert.True(t, ok)

	rtx, ok = n.getTransaction(address, types.EthUInt64{UInt64: 3})
	assert.Equal(t, &tx3, rtx.tx)
	assert.True(t, ok)
}
