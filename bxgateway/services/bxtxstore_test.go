package services

import (
	"github.com/bloXroute-Labs/gateway/bxgateway"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

const testNetworkNum types.NetworkNum = 5
const testChainID int64 = 1

func newTestBxTxStore() BxTxStore {
	return newBxTxStore(&bxmock.MockClock{}, 30*time.Second, 30*time.Second, 10*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil)
}

func TestBxTxStore_Add(t *testing.T) {
	store := newTestBxTxStore()

	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	hash2 := types.SHA256Hash{2}
	content2 := types.TxContent{2}
	hash3 := types.SHA256Hash{3}
	content3 := types.TxContent{3}

	// add content first, then short ID
	result11 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result11.NewTx)
	assert.True(t, result11.NewContent)
	assert.False(t, result11.NewSID)

	result12 := store.Add(hash1, content1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.False(t, result12.NewTx)
	assert.False(t, result12.NewContent)
	assert.True(t, result12.NewSID)

	// try adding some nonsense that should all be ignored
	result13 := store.Add(hash1, content2, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.False(t, result13.NewTx)
	assert.False(t, result13.NewContent)
	assert.False(t, result13.NewSID)

	for tx := range store.Iter() {
		assert.Equal(t, hash1, tx.Hash())
		assert.Equal(t, content1, tx.Content())
		assert.Equal(t, types.ShortID(1), tx.ShortIDs()[0])
	}

	// add short ID first, then content
	result21 := store.Add(hash2, types.TxContent{}, 2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result21.NewTx)
	assert.False(t, result21.NewContent)
	assert.True(t, result21.NewSID)

	result22 := store.Add(hash2, content2, 2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.False(t, result22.NewTx)
	assert.True(t, result22.NewContent)
	assert.False(t, result22.NewSID)

	// add content and short ID together
	result31 := store.Add(hash3, content3, 3, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result31.NewTx)
	assert.True(t, result31.NewContent)
	assert.True(t, result31.NewSID)

}

func TestBxTxStore_clean(t *testing.T) {
	clock := bxmock.MockClock{}

	cleanedShortIDsChan := make(chan types.ShortIDsByNetwork)
	store := newBxTxStore(&clock, 30*time.Second, 30*time.Second, 10*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), cleanedShortIDsChan)

	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	hash2 := types.SHA256Hash{2}
	content2 := types.TxContent{2}

	// add content first, no shortID
	result1 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.Equal(t, store.Count(), 1)
	result1.Transaction.SetAddTime(clock.Now())

	result2 := store.Add(hash2, content2, 2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewContent)
	assert.True(t, result2.NewSID)
	assert.Equal(t, store.Count(), 2)
	result2.Transaction.SetAddTime(clock.Now())

	clock.IncTime(20 * time.Second)
	cleaned, cleanedShortIDs := store.clean()
	assert.Equal(t, 0, len(cleanedShortIDs[testNetworkNum]))
	assert.Equal(t, store.Count(), 1)
	assert.Equal(t, cleaned, 1)
}

func TestBxTxStore_clean256K(t *testing.T) {
	otherNetworkTxs := 20
	extra := 10
	clock := bxmock.MockClock{}

	store := newBxTxStore(&clock, 30*time.Second, 300*time.Hour, 10*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil)
	// add some Tx from a different network to check that these will not be cleaned
	for i := 0; i < otherNetworkTxs; i++ {
		var h types.SHA256Hash
		var c types.TxContent
		copy(h[:], strconv.Itoa(i))
		result1 := store.Add(h, c, types.ShortID(i+1), testNetworkNum+1, false, types.TFPaidTx, time.Now(), testChainID)
		assert.True(t, result1.NewTx)
		assert.False(t, result1.NewContent)
		assert.True(t, result1.NewSID)
		result1.Transaction.SetAddTime(clock.Now())
		clock.IncTime(time.Second)
	}

	count := bxgateway.TxStoreMaxSize + extra
	for i := otherNetworkTxs; i < otherNetworkTxs+count; i++ {
		var h types.SHA256Hash
		var c types.TxContent
		copy(h[:], strconv.Itoa(i))
		result1 := store.Add(h, c, types.ShortID(i+1), testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
		assert.True(t, result1.NewTx)
		assert.False(t, result1.NewContent)
		assert.True(t, result1.NewSID)
		result1.Transaction.SetAddTime(clock.Now())
		clock.IncTime(time.Second)
	}
	assert.Equal(t, count+otherNetworkTxs, store.Count())
	assert.Equal(t, count+otherNetworkTxs, store.hashToContent.Count())
	assert.Equal(t, count+otherNetworkTxs, store.shortIDToHash.Count())

	cleaned, cleanedShortIDs := store.clean()
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.9+otherNetworkTxs, store.Count())
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.1+extra, len(cleanedShortIDs[testNetworkNum]))
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.9+otherNetworkTxs, store.hashToContent.Count())
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.9+otherNetworkTxs, store.shortIDToHash.Count())
	assert.Equal(t, 0, store.seenTxs.Count())

	assert.Equal(t, bxgateway.TxStoreMaxSize*0.1+extra, cleaned)
}

func TestHistory(t *testing.T) {
	clock := bxmock.MockClock{}
	cleanedShortIDsChan := make(chan types.ShortIDsByNetwork)
	store := newBxTxStore(&clock, 30*time.Minute, 3*24*time.Hour, 10*time.Minute,
		NewEmptyShortIDAssigner(), newHashHistory("seenTxs", &clock, 30*time.Minute), cleanedShortIDsChan)
	shortIDsByNetwork := make(types.ShortIDsByNetwork)
	go func() {
		for {
			cleanedShortIDs := <-cleanedShortIDsChan
			shortIDsByNetwork[testNetworkNum] = append(shortIDsByNetwork[testNetworkNum], cleanedShortIDs[testNetworkNum]...)
		}
	}()

	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	hash2 := types.SHA256Hash{2}
	content2 := types.TxContent{2}
	shortID2 := types.ShortID(2)

	// add content first
	result := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result.NewTx)
	assert.True(t, result.NewContent)
	assert.False(t, result.NewSID)
	result1 := store.Add(hash2, content2, shortID2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.True(t, result1.NewSID)

	// remove it
	store.RemoveHashes(&types.SHA256HashList{hash1}, ReEntryProtection, "test")
	// make sure size is 1 (hash2)
	assert.Equal(t, 1, store.Count())
	assert.Equal(t, 1, store.seenTxs.Count())

	// add it again - should not get in due to history
	result = store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID)
	assert.False(t, result.NewTx)
	assert.False(t, result.NewContent)
	assert.False(t, result.NewSID)
	// move time behind history
	clock.IncTime(1*time.Minute + timeToAvoidReEntry)
	// force cleanup
	store.CleanNow()
	//assert.Equal(t, 1, len(cleanedShortIDsChan))
	// add it again - this time it should get in
	result = store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID)
	assert.True(t, result.NewTx)
	assert.True(t, result.NewContent)
	assert.False(t, result.NewSID)
	// make sure hash2 is already in store
	tx, err := store.GetTxByShortID(shortID2)
	assert.Nil(t, err)
	assert.Equal(t, content2, tx.Content())
	// make sure size is 2
	assert.Equal(t, 2, store.Count())
}

func TestGetTxByShortID(t *testing.T) {
	store := newTestBxTxStore()

	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	tx, err := store.GetTxByShortID(1)
	assert.Nil(t, tx)
	assert.NotNil(t, err)

	// add content first, then short ID
	result11 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.True(t, result11.NewTx)
	assert.True(t, result11.NewContent)
	assert.False(t, result11.NewSID)
	tx, err = store.GetTxByShortID(1)
	assert.Nil(t, tx)
	assert.NotNil(t, err)

	result12 := store.Add(hash1, content1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID)
	assert.False(t, result12.NewTx)
	assert.False(t, result12.NewContent)
	assert.True(t, result12.NewSID)
	tx, err = store.GetTxByShortID(1)
	assert.NotNil(t, tx)
	assert.Nil(t, err)
	assert.Equal(t, content1, tx.Content())

	store.remove(string(hash1[:]), ReEntryProtection, "TestGetTxByShortID")
	tx, err = store.GetTxByShortID(1)
	assert.Nil(t, tx)
	assert.NotNil(t, err)
}
