package services

import (
	"strconv"
	"testing"
	"time"

	bxgateway "github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

const testNetworkNum types.NetworkNum = 5
const testChainID int64 = 1

func newTestBxTxStore() BxTxStore {
	return newBxTxStore(&utils.MockClock{}, 30*time.Second, 30*time.Second, 10*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, 30*time.Minute, NoOpBloomFilter{})
}

func TestBxTxStore_Add(t *testing.T) {
	store := newTestBxTxStore()

	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	hash2 := types.SHA256Hash{2}
	content2 := types.TxContent{2}
	hash3 := types.SHA256Hash{3}
	content3 := types.TxContent{3}
	hash4 := types.SHA256Hash{4}
	content4 := types.TxContent{4}

	// add content first
	result11 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, 0, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result11.NewTx)
	assert.True(t, result11.NewContent)
	assert.False(t, result11.NewSID)
	assert.False(t, result11.Reprocess)

	// reprocess paid tx
	result12 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result12.NewTx)
	assert.False(t, result12.NewContent)
	assert.False(t, result12.NewSID)
	assert.True(t, result12.Reprocess)

	// only reprocess paid tx once
	result13 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result13.NewTx)
	assert.False(t, result13.NewContent)
	assert.False(t, result13.NewSID)
	assert.False(t, result13.Reprocess)

	// then add short ID
	result14 := store.Add(hash1, content1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result14.NewTx)
	assert.False(t, result14.NewContent)
	assert.True(t, result14.NewSID)
	assert.False(t, result14.Reprocess)

	// try adding some nonsense that should all be ignored
	result15 := store.Add(hash1, content2, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result15.NewTx)
	assert.False(t, result15.NewContent)
	assert.False(t, result15.NewSID)
	assert.False(t, result15.Reprocess)

	for tx := range store.Iter() {
		assert.Equal(t, hash1, tx.Hash())
		assert.Equal(t, content1, tx.Content())
		assert.Equal(t, types.ShortID(1), tx.ShortIDs()[0])
	}

	// add short ID first
	result21 := store.Add(hash2, types.TxContent{}, 2, testNetworkNum, false, 0, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result21.NewTx)
	assert.False(t, result21.NewContent)
	assert.True(t, result21.NewSID)
	assert.False(t, result21.Reprocess)

	// reprocess deliverToNode tx
	result22 := store.Add(hash2, types.TxContent{}, 2, testNetworkNum, false, types.TFDeliverToNode, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result22.NewTx)
	assert.False(t, result22.NewContent)
	assert.False(t, result22.NewSID)
	assert.True(t, result22.Reprocess)

	// then add content
	result23 := store.Add(hash2, content2, 2, testNetworkNum, false, types.TFDeliverToNode, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result23.NewTx)
	assert.True(t, result23.NewContent)
	assert.False(t, result23.NewSID)
	assert.False(t, result23.Reprocess)

	// add content and short ID together
	result31 := store.Add(hash3, content3, 3, testNetworkNum, false, types.TFDeliverToNode, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result31.NewTx)
	assert.True(t, result31.NewContent)
	assert.True(t, result31.NewSID)
	assert.False(t, result31.Reprocess)

	// new validators_only transaction no shortID
	result32 := store.Add(hash4, content4, types.ShortIDEmpty, testNetworkNum, false, types.TFValidatorsOnly, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result32.NewTx)
	assert.True(t, result32.NewContent)
	assert.False(t, result32.NewSID)
	assert.False(t, result32.Reprocess)

	// ignore validators_only transaction without validators_only
	result33 := store.Add(hash4, content4, types.ShortIDEmpty, testNetworkNum, false, 0, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result33.NewTx)
	assert.False(t, result33.NewContent)
	assert.False(t, result33.NewSID)
	assert.False(t, result33.Reprocess)

	// ignore repeated validators_only transaction with validators_only
	result34 := store.Add(hash4, content4, types.ShortIDEmpty, testNetworkNum, false, types.TFValidatorsOnly, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result34.NewTx)
	assert.False(t, result34.NewContent)
	assert.False(t, result34.NewSID)
	assert.False(t, result34.Reprocess)
}

func TestBxTxStore_clean(t *testing.T) {
	clock := utils.MockClock{}

	cleanedShortIDsChan := make(chan types.ShortIDsByNetwork)
	store := newBxTxStore(&clock, 30*time.Second, 30*time.Second, 10*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), cleanedShortIDsChan, 30*time.Minute, NoOpBloomFilter{})
	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	hash2 := types.SHA256Hash{2}
	content2 := types.TxContent{2}

	// add content first, no shortID
	result1 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.Equal(t, store.Count(), 1)
	result1.Transaction.SetAddTime(clock.Now())

	result2 := store.Add(hash2, content2, 2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
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
	clock := utils.MockClock{}

	store := newBxTxStore(&clock, 30*time.Second, 300*time.Hour, 10*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, 30*time.Minute, NoOpBloomFilter{})
	// add some Tx from a different network to check that these will not be cleaned
	for i := 0; i < otherNetworkTxs; i++ {
		var h types.SHA256Hash
		var c types.TxContent
		copy(h[:], strconv.Itoa(i))
		result1 := store.Add(h, c, types.ShortID(i+1), testNetworkNum+1, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
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
		result1 := store.Add(h, c, types.ShortID(i+1), testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
		assert.True(t, result1.NewTx)
		assert.False(t, result1.NewContent)
		assert.True(t, result1.NewSID)
		result1.Transaction.SetAddTime(clock.Now())
		clock.IncTime(time.Second)
	}
	assert.Equal(t, count+otherNetworkTxs, store.Count())
	assert.Equal(t, count+otherNetworkTxs, store.hashToContent.Size())
	assert.Equal(t, count+otherNetworkTxs, store.shortIDToHash.Size())

	cleaned, cleanedShortIDs := store.clean()
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.9+otherNetworkTxs, store.Count())
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.1+extra, len(cleanedShortIDs[testNetworkNum]))
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.9+otherNetworkTxs, store.hashToContent.Size())
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.9+otherNetworkTxs, store.shortIDToHash.Size())
	assert.Equal(t, bxgateway.TxStoreMaxSize*0.1+extra, store.seenTxs.Count())

	assert.Equal(t, bxgateway.TxStoreMaxSize*0.1+extra, cleaned)
}

func TestHistory(t *testing.T) {
	clock := utils.MockClock{}
	// have to use date between 1678 and 2262 for UnixNano to work
	clock.SetTime(time.Date(2000, 01, 01, 00, 00, 00, 00, time.UTC))

	cleanedShortIDsChan := make(chan types.ShortIDsByNetwork)
	store := newBxTxStore(&clock, 30*time.Minute, 3*24*time.Hour, 10*time.Minute,
		NewEmptyShortIDAssigner(), newHashHistory("seenTxs", &clock, 30*time.Minute), cleanedShortIDsChan, 30*time.Minute, NoOpBloomFilter{})
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
	result := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result.NewTx)
	assert.True(t, result.NewContent)
	assert.False(t, result.NewSID)
	result1 := store.Add(hash2, content2, shortID2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.True(t, result1.NewSID)

	// remove it
	store.RemoveHashes(&types.SHA256HashList{hash1}, ShortReEntryProtection, "test")
	// make sure size is 1 (hash2)
	assert.Equal(t, 1, store.Count())
	assert.Equal(t, 1, store.seenTxs.Count())

	// add it again - should not get in due to history
	result = store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.False(t, result.NewTx)
	assert.False(t, result.NewContent)
	assert.False(t, result.NewSID)
	// move time behind history
	clock.IncTime(1*time.Minute + 24*time.Hour)
	// force cleanup
	store.CleanNow()
	//assert.Equal(t, 1, len(cleanedShortIDsChan))
	// add it again - this time it should get in
	result = store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
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
	result11 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.True(t, result11.NewTx)
	assert.True(t, result11.NewContent)
	assert.False(t, result11.NewSID)
	tx, err = store.GetTxByShortID(1)
	assert.Nil(t, tx)
	assert.NotNil(t, err)

	result12 := store.Add(hash1, content1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	assert.False(t, result12.NewTx)
	assert.False(t, result12.NewContent)
	assert.True(t, result12.NewSID)
	tx, err = store.GetTxByShortID(1)
	assert.NotNil(t, tx)
	assert.Nil(t, err)
	assert.Equal(t, content1, tx.Content())

	store.remove(string(hash1[:]), FullReEntryProtection, "TestGetTxByShortID")
	tx, err = store.GetTxByShortID(1)
	assert.Nil(t, tx)
	assert.NotNil(t, err)
}

func TestHistoryTxWithShortID(t *testing.T) {
	clock := utils.MockClock{}

	cleanedShortIDsChan := make(chan types.ShortIDsByNetwork)
	store := newBxTxStore(&clock, 30*time.Second, 30*time.Second, 10*time.Second, NewEmptyShortIDAssigner(), newHashHistory("seenTxs", &clock, 60*time.Minute), cleanedShortIDsChan, 30*time.Minute, NoOpBloomFilter{})
	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}

	// add content first, no shortID
	result1 := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.False(t, result1.NewSID)
	assert.Equal(t, store.Count(), 1)

	clock.IncTime(40 * time.Second)
	go store.CleanNow()
	<-cleanedShortIDsChan
	// tx should not be added since it is history
	result := store.Add(hash1, content1, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.False(t, result.NewTx)
	assert.False(t, result.NewContent)
	assert.True(t, result.AlreadySeen)

	// tx should be added because it has short id
	result2 := store.Add(hash1, content1, 1, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewSID)
}

func TestBxTxStore_ResetSeenTxTime(t *testing.T) {
	clock := utils.MockClock{}
	cleanedShortIDsChan := make(chan types.ShortIDsByNetwork)
	store := newBxTxStore(&clock, 30*time.Second, 30*time.Second, 10*time.Second, NewEmptyShortIDAssigner(), newHashHistory("seenTxs", &clock, 60*time.Minute), cleanedShortIDsChan, 30*time.Second, NoOpBloomFilter{})

	// Case 1:
	// ConnDetails case, add hash1 to TxStore and wait for TxStore to clean up, so hash1 is stored in SeenTx
	// without calling Add() or Get() to reset the hash1 in seenTx, hash1 will expire after cleanupFreq which is 30 seconds

	hash1 := types.SHA256Hash{1}
	content1 := types.TxContent{1}
	result1 := store.Add(hash1, content1, types.ShortID(1), testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.True(t, result1.NewTx)
	assert.True(t, result1.NewContent)
	assert.Equal(t, store.Count(), 1)

	str := string(hash1[:])
	// hash1 is still in TxStore, not in seenTx yet
	assert.False(t, store.seenTxs.Exists(str))
	clock.IncTime(31 * time.Second)
	go store.CleanNow()
	<-cleanedShortIDsChan
	// clean up done, the hash1 should have been removed from TxStore, and added into seenTx
	assert.True(t, store.seenTxs.Exists(str))

	clock.IncTime(29 * time.Second)
	// hash1 in seenTx will expire in next second
	assert.True(t, store.seenTxs.Exists(str))

	clock.IncTime(time.Second)
	// hash1 expires now in seenTx
	assert.False(t, store.seenTxs.Exists(str))

	// Case 2
	// Testing if Get() reset the seenTx timestamp of hash 2
	// add hash2 to TxStore and wait for TxStore to clean up, call Get() right before it expire and check if the expiration time renewed
	hash2 := types.SHA256Hash{1}
	content2 := types.TxContent{1}
	result2 := store.Add(hash2, content2, types.ShortID(2), testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.True(t, result2.NewTx)
	assert.True(t, result2.NewContent)
	assert.Equal(t, store.Count(), 1)

	// clean up hash2 to add it into seenTx
	clock.IncTime(31 * time.Second)
	go store.CleanNow()
	<-cleanedShortIDsChan
	str = string(hash2[:])
	assert.True(t, store.seenTxs.Exists(str))

	// right before it expire
	clock.IncTime(29 * time.Second)
	// Get() should renew the hash2 in seenTx, plus the hash2 in seenTx, Get() should return (nil, false)
	_, ok := store.Get(hash2)
	assert.False(t, ok)

	// Since Get() is called, hash2 still in seenTx
	clock.IncTime(29 * time.Second)
	assert.True(t, store.seenTxs.Exists(str))

	// now hash2 expires, reset in Get() works as expected
	clock.IncTime(time.Second)
	assert.False(t, store.seenTxs.Exists(str))

	// case 3
	// Testing if Add() reset the seenTx timestamp of hash 3
	// add hash3 to TxStore and wait for TxStore to clean up, call Add() right before it expire and check if the expiration time renewed
	hash3 := types.SHA256Hash{1}
	content3 := types.TxContent{1}
	result3 := store.Add(hash3, content3, types.ShortID(3), testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.True(t, result3.NewTx)
	assert.True(t, result3.NewContent)
	assert.Equal(t, store.Count(), 1)

	// clean up hash3 to add it into seenTx
	clock.IncTime(31 * time.Second)
	go store.CleanNow()
	<-cleanedShortIDsChan

	clock.IncTime(29 * time.Second)
	// hash3 will expire in next second in seenTx, now call
	result3 = store.Add(hash3, content3, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, clock.Now(), testChainID, types.EmptySender)
	assert.False(t, result3.NewTx)
	assert.False(t, result3.NewContent)
	assert.Equal(t, 0, store.Count())

	clock.IncTime(29 * time.Second)
	// because Add() renew the hash3 in seenTx, it expires in seenTx the next second, still exists in seenTx for the moment
	assert.True(t, store.seenTxs.Exists(str))

	// hash3 expires now
	clock.IncTime(time.Second)
	assert.False(t, store.seenTxs.Exists(str))
}
