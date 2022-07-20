package services

import (
	"crypto/sha256"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/struCoder/pidusage"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"
)

type TestManager struct {
	listTxHashes types.SHA256HashList
	quit1        chan bool
	quit2        chan bool
}

func generateSixHundredThousandTx(t *BxTxStore) {
	timeNow := time.Now()
	hashMap := make(map[types.SHA256Hash]bool)

	for i := 0; i < 600000; {
		hash := generateRandTxHash()
		if _, ok := hashMap[hash]; ok {
			continue
		}
		hashMap[hash] = true
		content := generateRandTxContent()
		result := t.Add(hash, content, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
		result.Transaction.SetAddTime(timeNow.Add(time.Duration(-33*i) * time.Millisecond))
		if result.NewTx {
			i++
		}
	}
}

func generateRandTxContent() []byte {
	num := 200 + rand.Intn(100)
	slice := make([]byte, num)
	if _, err := rand.Read(slice); err != nil {
		panic(err)
	}
	return slice
}

func generateRandTxHash() types.SHA256Hash {
	slice := make([]byte, 32)
	if _, err := rand.Read(slice[29:]); err != nil {
		panic(err)
	}
	var hash types.SHA256Hash
	copy(hash[:], slice)

	return hash
}

func BenchmarkTxService(b *testing.B) {
	txManager := NewBxTxStore(10*time.Second, 300*time.Minute, 10*time.Minute, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, 30*time.Minute)
	go func() {
		_ = txManager.Start()
	}()
	generateSixHundredThousandTx(&txManager)
	items := txManager.Count()
	fmt.Println("number of tx: ", items)

	var durationForAdd time.Duration
	durationForAdd = 0
	testManager := TestManager{
		quit1: make(chan bool),
		quit2: make(chan bool),
	}
	lock := sync.Mutex{}

	go func() {
		ticker := time.NewTicker(33 * time.Millisecond)
		tickerTImeForGet := time.NewTicker(10 * time.Second)
		tickerMapSize := time.NewTicker(30 * time.Second)
		for {
			select {
			case <-ticker.C:
				currTime := time.Now()
				result := txManager.Add(generateRandTxHash(), generateRandTxContent(), 1, testNetworkNum,
					false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
				after := time.Now().Sub(currTime)
				durationForAdd += after
				lock.Lock()
				testManager.listTxHashes = append(testManager.listTxHashes, result.Transaction.Hash())
				lock.Unlock()
			case <-tickerTImeForGet.C:
				fmt.Println("duration for add after 300 adds: ", durationForAdd)
				durationForAdd = 0
			case <-tickerMapSize.C:
				sysInfo, _ := pidusage.GetStat(os.Getpid())
				fmt.Println("map size after 30 sec: ", txManager.Count(), " rss: ", int64(sysInfo.Memory))
			case <-testManager.quit1:
				testManager.quit1 <- true
				ticker.Stop()
				tickerMapSize.Stop()
				tickerTImeForGet.Stop()
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		var newList types.SHA256HashList
		for {
			select {
			case <-ticker.C:
				currTime := time.Now()
				fmt.Println("len list hash before removing: ", len(testManager.listTxHashes))
				lock.Lock()
				testManager.listTxHashes = testManager.listTxHashes[len(testManager.listTxHashes)-200 : len(testManager.listTxHashes)]
				copy(newList, testManager.listTxHashes[len(testManager.listTxHashes)-200:len(testManager.listTxHashes)])
				testManager.listTxHashes = newList
				lock.Unlock()
				fmt.Println("len list hash after removing: ", len(testManager.listTxHashes))
				fmt.Println("len map before removing: ", txManager.Count())
				currTime = time.Now()
				txManager.RemoveHashes(&newList, FullReEntryProtection, "test")
				fmt.Println("len map after removing: ", txManager.Count())
				after := time.Now().Sub(currTime)
				fmt.Printf("time take to remove %v hash %v:\n ", len(newList), after)
			case <-testManager.quit2:
				testManager.quit2 <- true
				ticker.Stop()
				return
			}
		}
	}()
	time.Sleep(11 * time.Minute)
	//stop
	testManager.quit1 <- true
	<-testManager.quit1
	testManager.quit2 <- true
	<-testManager.quit2
	txManager.Stop()
}

func TestRemoveTxsByShortIDs(t *testing.T) {
	txService := NewBxTxStore(10*time.Second, 300*time.Second, 30*time.Second, NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, 30*time.Minute)

	content := generateRandTxContent()
	h := sha256.New()
	h.Write(content)
	hash := types.SHA256Hash{}
	copy(hash[:], h.Sum(nil))

	// add new transaction without short ID
	result := txService.Add(hash, content, types.ShortIDEmpty, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	if !result.NewTx || txService.Count() != 1 {
		t.Error("Failed to add transaction")
	}
	// remove some not existing transactions
	txService.RemoveShortIDs(&types.ShortIDList{1, 3, 5, 6}, FullReEntryProtection, "test")
	if txService.Count() != 1 {
		t.Error("Incorrect number of transactions in BxTxStore")
	}
	// assign 2 shortIDs to the existing transaction
	txService.Add(hash, content, types.ShortID(1001), testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	result = txService.Add(hash, content, types.ShortID(1002), testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)

	if result.NewTx || len(result.Transaction.ShortIDs()) != 2 || txService.Count() != 1 {
		t.Error("something went wrong")
	}

	txService.RemoveShortIDs(&types.ShortIDList{1002}, FullReEntryProtection, "test")
	if txService.Count() != 0 {
		t.Error("Failed to remove transaction by shortId")
	}

}
