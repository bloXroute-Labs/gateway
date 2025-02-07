package services

import (
	"fmt"
	"runtime/debug"
	"sort"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/gateway/v2"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pbbase "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

const (
	defaultMaxTxAge = 24 * 3 * time.Hour

	defaultMaxTotalBlobTxSizeBytes = 500 * 1024 * 1024 // 500Mb
	blobTxCleanCoef                = 0.8               // clean to defaultMaxTotalBlobTxSize * blobTxCleanCoef
)

// BxTxStore represents the storage of transaction info for a given node
type BxTxStore struct {
	clock         utils.Clock
	networkConfig sdnmessage.BlockchainNetworks

	hashToContent              *syncmap.SyncMap[string, *types.BxTransaction]
	shortIDToHash              *syncmap.SyncMap[types.ShortID, types.SHA256Hash]
	blobTxSizeCleanerByNetwork *syncmap.SyncMap[uint32, *TxSizeCleaner]
	blobCompressorStorage      BlobCompressorStorage

	seenTxs            HashHistory
	timeToAvoidReEntry time.Duration

	cleanupFreq            time.Duration
	noSIDAge               time.Duration
	quit                   chan bool
	assigner               ShortIDAssigner
	cleanedShortIDsChannel chan types.ShortIDsByNetwork
	bloom                  BloomFilter
	blobsCleanerEnabled    bool
}

// NewBxTxStore creates a new BxTxStore to store and processes all relevant transactions
func NewBxTxStore(cleanupFreq time.Duration, networkConfig sdnmessage.BlockchainNetworks, noSIDAge time.Duration,
	assigner ShortIDAssigner, seenTxs HashHistory, cleanedShortIDsChannel chan types.ShortIDsByNetwork,
	timeToAvoidReEntry time.Duration, bloom BloomFilter, blobCompressorStorage BlobCompressorStorage,
	blobsCleanerEnabled bool,
) BxTxStore {
	return newBxTxStore(utils.RealClock{}, networkConfig, cleanupFreq, noSIDAge, assigner, seenTxs, cleanedShortIDsChannel, timeToAvoidReEntry, bloom, blobCompressorStorage, blobsCleanerEnabled)
}

func newBxTxStore(clock utils.Clock, networkConfig sdnmessage.BlockchainNetworks, cleanupFreq time.Duration,
	noSIDAge time.Duration, assigner ShortIDAssigner, seenTxs HashHistory, cleanedShortIDsChannel chan types.ShortIDsByNetwork,
	timeToAvoidReEntry time.Duration, bloom BloomFilter, blobCompressorStorage BlobCompressorStorage,
	blobsCleanerEnabled bool,
) BxTxStore {
	bxStore := BxTxStore{
		clock:                  clock,
		networkConfig:          networkConfig,
		hashToContent:          syncmap.NewStringMapOf[*types.BxTransaction](),
		shortIDToHash:          syncmap.NewIntegerMapOf[types.ShortID, types.SHA256Hash](),
		blobCompressorStorage:  blobCompressorStorage,
		seenTxs:                seenTxs,
		timeToAvoidReEntry:     timeToAvoidReEntry,
		cleanupFreq:            cleanupFreq,
		noSIDAge:               noSIDAge,
		quit:                   make(chan bool),
		assigner:               assigner,
		cleanedShortIDsChannel: cleanedShortIDsChannel,
		bloom:                  bloom,
		blobsCleanerEnabled:    blobsCleanerEnabled,
	}

	if blobsCleanerEnabled {
		bxStore.blobTxSizeCleanerByNetwork = syncmap.NewIntegerMapOf[uint32, *TxSizeCleaner]()
	}

	return bxStore
}

// Start initializes all relevant goroutines for the BxTxStore
func (t *BxTxStore) Start() error {
	t.cleanup()
	return nil
}

// Stop closes all running go routines for BxTxStore
func (t *BxTxStore) Stop() {
	t.quit <- true
	<-t.quit
}

// Clear removes all elements from txs and shortIDToHash
func (t *BxTxStore) Clear() {
	t.hashToContent.Clear()
	t.shortIDToHash.Clear()
	t.blobCompressorStorage.Clear()
	log.Debugf("Cleared tx service.")
}

// Count indicates the number of stored transaction in BxTxStore
func (t *BxTxStore) Count() int {
	return t.hashToContent.Size()
}

// remove deletes a single transaction, including its shortIDs
func (t *BxTxStore) remove(hash string, bxTransaction *types.BxTransaction, reEntryProtection ReEntryProtectionFlags, reason string) {
	t.hashToContent.Delete(hash)

	for _, shortID := range bxTransaction.ShortIDs() {
		t.shortIDToHash.Delete(shortID)
	}
	// if asked, add the hash to the history map so we remember this transaction for some time
	// and prevent if from being added back to the TxStore
	switch reEntryProtection {
	case NoReEntryProtection:
	case ShortReEntryProtection:
		t.seenTxs.Add(hash, ShortReEntryProtectionDuration)
		t.bloom.Add([]byte(hash))
	case FullReEntryProtection:
		t.seenTxs.Add(hash, t.timeToAvoidReEntry)
		t.bloom.Add([]byte(hash))
	default:
		log.Fatalf("unknown reEntryProtection value %v for hash %v", reEntryProtection, hash)
	}

	// remove the hash also from the blobCompressorStorage
	t.blobCompressorStorage.RemoveByTxHash(hash)

	// remove the hash from the blob cleaner if it is a blob transaction
	if t.blobsCleanerEnabled && bxTransaction.Flags().IsWithSidecar() {
		t.getBlobTxCleaner(bxTransaction.NetworkNum()).Remove(hash)
	}

	log.Tracef("TxStore: transaction %v, network %v, shortIDs %v removed (%v). reEntryProtection %v",
		bxTransaction.Hash(), bxTransaction.NetworkNum(), bxTransaction.ShortIDs(), reason, reEntryProtection)
}

// RemoveShortIDs deletes a series of transactions by their short IDs. RemoveShortIDs can take a potentially large short ID array, so it should be passed by reference.
func (t *BxTxStore) RemoveShortIDs(shortIDs *types.ShortIDList, reEntryProtection ReEntryProtectionFlags, reason string) {
	// note - it is OK for hashesToRemove to hold the same hash multiple times.
	hashesToRemove := make(types.SHA256HashList, 0)
	for _, shortID := range *shortIDs {
		if hash, ok := t.shortIDToHash.Load(shortID); ok {
			hashesToRemove = append(hashesToRemove, hash)
		}
	}
	t.RemoveHashes(&hashesToRemove, reEntryProtection, reason)
}

// GetTxByShortID lookup a transaction by its shortID. return error if not found
func (t *BxTxStore) GetTxByShortID(shortID types.ShortID, withSidecar bool) (*types.BxTransaction, error) {
	if hash, ok := t.shortIDToHash.Load(shortID); ok {
		if tx, exists := t.hashToContent.Load(string(hash[:])); exists {
			if !withSidecar && tx.Flags().IsWithSidecar() {
				var ethTx ethtypes.Transaction

				err := rlp.DecodeBytes(tx.Content(), &ethTx)
				if err != nil {
					log.Errorf("could not decode Ethereum transaction: %v", err)
					return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
				}

				log.Debugf("transaction %s has sidecar, removing it, short id: %v", ethTx.Hash().String(), shortID)
				// get transaction content without the blobs sidecars
				newTx := ethTx.WithoutBlobTxSidecar()

				newContent, err := rlp.EncodeToBytes(newTx)
				if err != nil {
					log.Errorf("could not encode Ethereum transaction: %v", err)
					return nil, fmt.Errorf("could not encode Ethereum transaction: %v", err)
				}

				txCopy := tx.CloneWithEmptyContent()
				txCopy.SetContent(newContent)
				return txCopy, nil
			}
			return tx, nil
		}
		return nil, fmt.Errorf("transaction content for shortID %v and hash %v does not exist", shortID, hash)
	}

	return nil, fmt.Errorf("transaction with shortID %v does not exist", shortID)
}

// GetTxByKzgCommitment lookup a transaction by its KzgCommitment. return error if not found
func (t *BxTxStore) GetTxByKzgCommitment(kzgCommitment string) (*types.BxTransaction, error) {
	if hash, ok := t.blobCompressorStorage.KzgCommitmentToTxHash(kzgCommitment); ok {
		if tx, exists := t.hashToContent.Load(hash); exists {
			return tx, nil
		}
		return nil, fmt.Errorf("transaction content for KzgCommitment %v and hash %v does not exist", kzgCommitment, hash)
	}

	return nil, fmt.Errorf("transaction with KzgCommitment %v does not exist", kzgCommitment)
}

// RemoveHashes deletes a series of transactions by their hash from BxTxStore. RemoveHashes can take a potentially large hash array, so it should be passed by reference.
func (t *BxTxStore) RemoveHashes(hashes *types.SHA256HashList, reEntryProtection ReEntryProtectionFlags, reason string) types.ShortIDList {
	shortIDList := make(types.ShortIDList, 0)
	for _, hash := range *hashes {
		hashStr := string(hash.Bytes())

		if bxTransaction, ok := t.hashToContent.Load(hashStr); ok {
			t.remove(hashStr, bxTransaction, reEntryProtection, reason)
			shortIDList = append(shortIDList, bxTransaction.ShortIDs()...)
		}
	}

	return shortIDList
}

// Iter returns a channel iterator for all transactions in BxTxStore
func (t *BxTxStore) Iter() (iter <-chan *types.BxTransaction) {
	newChan := make(chan *types.BxTransaction)
	go func() {
		t.hashToContent.Range(func(key string, bxTransaction *types.BxTransaction) bool {
			if t.clock.Now().Sub(bxTransaction.AddTime()) < t.maxTxAge(bxTransaction.NetworkNum()) {
				newChan <- bxTransaction
			}
			return true
		})

		close(newChan)
	}()
	return newChan
}

// Add adds a new transaction to BxTxStore
func (t *BxTxStore) Add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID, networkNum types.NetworkNum,
	_ bool, flags types.TxFlags, timestamp time.Time, _ int64, sender types.Sender,
) types.TransactionResult {
	if shortID == types.ShortIDEmpty && len(content) == 0 {
		debug.PrintStack()
		panic("Bad usage of Add function - content and shortID can't be both missing")
	}
	result := types.TransactionResult{}
	if t.clock.Now().Sub(timestamp) > t.maxTxAge(networkNum) {
		result.Transaction = types.NewBxTransaction(hash, networkNum, flags, timestamp)
		result.DebugData = fmt.Sprintf("Transaction is too old - %v", timestamp)
		return result
	}

	hashStr := string(hash[:])
	// if the hash is in history we treat it as IgnoreSeen
	if t.refreshSeenTx(hash) {
		// if the hash is in history, but we get a shortID for it, it means that the hash was not in the ATR history
		// and some GWs may get and use this shortID. In such a case we should remove the hash from history and allow
		// it to be added to the TxStore
		if shortID != types.ShortIDEmpty {
			t.seenTxs.Remove(hashStr)
		} else {
			result.Transaction = types.NewBxTransaction(hash, networkNum, flags, timestamp)
			result.DebugData = "Transaction already seen and deleted from store"
			result.AlreadySeen = true
			return result
		}
	}

	bxTransaction := types.NewBxTransaction(hash, networkNum, flags, timestamp)

	if tx, exists := t.hashToContent.LoadOrStore(hashStr, bxTransaction); exists {
		bxTransaction = tx
	} else {
		result.NewTx = true

		if bxTransaction.Flags().IsWithSidecar() && t.blobsCleanerEnabled {
			t.getBlobTxCleaner(networkNum).AddTx(hashStr, len(content), timestamp)
		}
	}

	// check hash in the bloom filter only after it is not seen in txStore
	if t.bloom != nil && shortID == types.ShortIDEmpty && !result.Reprocess && result.NewTx && t.bloom.Check(hash.Bytes()) {
		result.DebugData = "Transaction ignored due to already seen in bloom filter"
		result.AlreadySeen = true
	}

	shortID = bxTransaction.Update(&result, flags, shortID, content, sender, t.assigner.Next)

	if result.NewSID {
		t.shortIDToHash.Store(shortID, bxTransaction.Hash())
	}

	return result
}

type networkData struct {
	maxAge     time.Duration
	ages       []int
	cleanAge   int
	cleanNoSID int
}

func (t *BxTxStore) clean() (cleaned int, cleanedShortIDs types.ShortIDsByNetwork) {
	currTime := t.clock.Now()

	networks := make(map[types.NetworkNum]*networkData)
	cleanedShortIDs = make(types.ShortIDsByNetwork)

	t.hashToContent.Range(func(key string, bxTransaction *types.BxTransaction) bool {
		netData, netDataExists := networks[bxTransaction.NetworkNum()]
		if !netDataExists {
			netData = &networkData{}
			networks[bxTransaction.NetworkNum()] = netData
		}
		txAge := int(currTime.Sub(bxTransaction.AddTime()) / time.Second)
		networks[bxTransaction.NetworkNum()].ages = append(networks[bxTransaction.NetworkNum()].ages, txAge)

		return true
	})

	for net, netData := range networks {
		// if we are below the number of allowed Txs, no need to do anything
		if len(netData.ages) <= bxgateway.TxStoreMaxSize {
			networks[net].maxAge = t.maxTxAge(net)
			continue
		}
		// per network, sort ages in ascending order
		sort.Ints(netData.ages)
		// in order to avoid many cleanup msgs, cleanup only 90% of the TxStoreMaxSize
		networks[net].maxAge = time.Duration(netData.ages[int(bxgateway.TxStoreMaxSize*0.9)-1]) * time.Second
		if networks[net].maxAge > t.maxTxAge(net) {
			networks[net].maxAge = t.maxTxAge(net)
		}
		log.Debugf("TxStore size for network %v is %v. Cleaning %v transactions older than %v",
			net, len(netData.ages), len(netData.ages)-bxgateway.TxStoreMaxSize, networks[net].maxAge)
	}

	t.hashToContent.Range(func(key string, bxTransaction *types.BxTransaction) bool {
		networkNum := bxTransaction.NetworkNum()
		netData, netDataExists := networks[networkNum]
		removeReason := ""
		txAge := currTime.Sub(bxTransaction.AddTime())

		if netDataExists && txAge > netData.maxAge {
			removeReason = fmt.Sprintf("transation age %v is greater than  %v", txAge, netData.maxAge)
			netData.cleanAge++
		} else {
			if txAge > t.noSIDAge && len(bxTransaction.ShortIDs()) == 0 {
				removeReason = fmt.Sprintf("transation age %v but no short ID", txAge)
				netData.cleanNoSID++
			}
		}

		if removeReason != "" {
			// remove the transaction by hash from both maps
			// no need to add the hash to the history as it is deleted after long time
			// dec-5-2021: add to hash history to prevent a lot of reentry (BSC)
			t.remove(key, bxTransaction, FullReEntryProtection, removeReason)

			cleanedShortIDs[networkNum] = append(cleanedShortIDs[networkNum], bxTransaction.ShortIDs()...)
		}

		return true
	})

	for net, netData := range networks {
		log.Debugf("TxStore network %v #txs before cleanup %v cleaned %v missing SID entries and %v aged entries",
			net, len(netData.ages), netData.cleanNoSID, netData.cleanAge)
		cleaned += netData.cleanNoSID + netData.cleanAge
	}

	return cleaned, cleanedShortIDs
}

// CleanNow performs an immediate cleanup of the TxStore
func (t *BxTxStore) CleanNow() {
	mapSizeBeforeClean := t.Count()
	timeStart := t.clock.Now()
	cleaned, cleanedShortIDs := t.clean()
	log.Debugf("TxStore cleaned %v entries in %v. size before clean: %v size after clean: %v",
		cleaned, t.clock.Now().Sub(timeStart), mapSizeBeforeClean, t.Count())
	if t.cleanedShortIDsChannel != nil && len(cleanedShortIDs) > 0 {
		t.cleanedShortIDsChannel <- cleanedShortIDs
	}
}

func (t *BxTxStore) cleanup() {
	ticker := t.clock.Ticker(t.cleanupFreq)
	for {
		select {
		case <-ticker.Alert():
			t.CleanNow()
		case <-t.quit:
			t.quit <- true
			ticker.Stop()
			return
		}
	}
}

// Get returns a single transaction from the transaction service
func (t *BxTxStore) Get(hash types.SHA256Hash) (*types.BxTransaction, bool) {
	// reset the timestamp of this hash in the seenTx hashHistory, if it exists in the hashHistory
	if t.refreshSeenTx(hash) {
		return nil, false
	}
	tx, ok := t.hashToContent.Load(string(hash[:]))
	if !ok {
		return nil, ok
	}
	return tx, ok
}

// Known returns whether if a tx hash is in seenTx
func (t *BxTxStore) Known(hash types.SHA256Hash) bool {
	return t.refreshSeenTx(hash)
}

// HasContent returns if a given transaction is in the transaction service
func (t *BxTxStore) HasContent(hash types.SHA256Hash) bool {
	tx, ok := t.Get(hash)
	if !ok {
		return false
	}
	return tx.Content() != nil
}

// Summarize returns some info about the tx service
func (t *BxTxStore) Summarize() *pbbase.TxStoreReply {
	networks := make(map[types.NetworkNum]*pbbase.TxStoreNetworkData)
	res := pbbase.TxStoreReply{
		TxCount:      uint64(t.hashToContent.Size()),
		ShortIdCount: uint64(t.shortIDToHash.Size()),
	}

	t.hashToContent.Range(func(key string, bxTransaction *types.BxTransaction) bool {
		networkData, exists := networks[bxTransaction.NetworkNum()]
		if !exists {
			networkData = &pbbase.TxStoreNetworkData{}
			networkData.OldestTx = bxTransaction.Protobuf()
			networkData.TxCount++
			networkData.SizeBytes += uint64(len(bxTransaction.Content()))
			networkData.Network = uint64(bxTransaction.NetworkNum())
			networkData.ShortIdCount += uint64(len(bxTransaction.ShortIDs()))
			networks[bxTransaction.NetworkNum()] = networkData

			// continue iteration
			return true
		}
		oldestTx := networkData.OldestTx
		oldestTxTS := oldestTx.AddTime
		if bxTransaction.AddTime().Before(oldestTxTS.AsTime()) {
			networkData.OldestTx = bxTransaction.Protobuf()
		}
		networkData.TxCount++
		networkData.SizeBytes += uint64(len(bxTransaction.Content()))
		networkData.ShortIdCount += uint64(len(bxTransaction.ShortIDs()))

		return true
	})

	for _, netData := range networks {
		res.NetworkData = append(res.NetworkData, netData)
	}

	return &res
}

func (t *BxTxStore) getBlobTxCleaner(networkNum types.NetworkNum) *TxSizeCleaner {
	cleaner, _ := t.blobTxSizeCleanerByNetwork.LoadOrStore(uint32(networkNum), NewTxSizeCleaner(fmt.Sprintf("blobs cleaner %d network", networkNum), t.totalMaxBlobTxSize(networkNum), blobTxCleanCoef, func(hashes *types.SHA256HashList) {
		shortIDs := t.RemoveHashes(hashes, FullReEntryProtection, "blob storage out of space")

		if t.cleanedShortIDsChannel != nil && len(shortIDs) > 0 {
			t.cleanedShortIDsChannel <- types.ShortIDsByNetwork{networkNum: shortIDs}
		}
	}))

	return cleaner
}

func (t *BxTxStore) totalMaxBlobTxSize(networkNum types.NetworkNum) uint64 {
	network, ok := t.networkConfig[networkNum]
	if !ok {
		return defaultMaxTotalBlobTxSizeBytes
	}

	if network.MaxTotalBlobTxSizeBytes == 0 {
		return defaultMaxTotalBlobTxSizeBytes
	}

	return network.MaxTotalBlobTxSizeBytes
}

func (t *BxTxStore) refreshSeenTx(hash types.SHA256Hash) bool {
	if t.seenTxs.Exists(string(hash[:])) {
		t.seenTxs.Add(string(hash[:]), t.timeToAvoidReEntry)
		return true
	}
	return false
}

func (t *BxTxStore) maxTxAge(networkNum types.NetworkNum) time.Duration {
	config := t.networkConfig[networkNum]
	if config == nil || config.MaxTxAgeSeconds == 0 {
		return defaultMaxTxAge
	}

	return time.Duration(config.MaxTxAgeSeconds) * time.Second
}
