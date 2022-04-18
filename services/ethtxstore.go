package services

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"math/big"
	"time"
)

// TODO : move ethtxstore and related tests outside of bxgateway package

const (
	cleanNonceInterval = 10 * time.Second
	timeToAvoidReEntry = 24 * time.Hour
)

// EthTxStore represents transaction storage and validation for Ethereum transactions
type EthTxStore struct {
	BxTxStore
	nonceTracker
}

// NewEthTxStore returns new manager for Ethereum transactions
func NewEthTxStore(clock utils.Clock, cleanupInterval time.Duration, maxTxAge time.Duration,
	noSIDAge time.Duration, assigner ShortIDAssigner, hashHistory HashHistory, cleanedShortIDsChannel chan types.ShortIDsByNetwork, networkConfig sdnmessage.BlockchainNetworks) *EthTxStore {
	return &EthTxStore{
		BxTxStore:    newBxTxStore(clock, cleanupInterval, maxTxAge, noSIDAge, assigner, hashHistory, cleanedShortIDsChannel, timeToAvoidReEntry),
		nonceTracker: newNonceTracker(clock, networkConfig, cleanNonceInterval),
	}
}

// Add validates an Ethereum transaction and checks that its nonce has not been seen before
func (t *EthTxStore) Add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID,
	network types.NetworkNum, validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64) TransactionResult {
	result := t.BxTxStore.Add(hash, content, shortID, network, false, flags, timestamp, networkChainID)

	// if no new content we can leave
	if !result.NewContent {
		return result
	}

	reuseNonceActive := t.isReuseNonceActive(network)
	// if validation is not needed and reuse nonce is not active we can leave to avoid parsing of the transaction
	if !validate && !reuseNonceActive {
		return result
	}

	// note: we need to continue and validate even if the content is coming from the BDN. This can still
	// be a reuseNonce case and we should start track the transaction for future reuse nonce with it.

	// upon first seeing tx, validate that it's an Ethereum transaction. Extract Sender only if reuse Nonce is Active
	blockchainTx, err := result.Transaction.BlockchainTransaction(reuseNonceActive)
	if err != nil {
		result.FailedValidation = true
		return result
	}

	ethTx := blockchainTx.(*types.EthTransaction)
	txChainID := ethTx.ChainID.Int64()
	if networkChainID != 0 && txChainID != 0 && networkChainID != txChainID {
		log.Errorf("chainID mismatch for hash %v - content chainID %v networkNum %v networkChainID %v", hash, txChainID, network, networkChainID)
		// remove the tx from the TxStore but allow it to get back in
		hashToRemove := make(types.SHA256HashList, 1)
		hashToRemove[0] = result.Transaction.Hash()
		t.BxTxStore.RemoveHashes(&hashToRemove, FullReEntryProtection, "chainID mismatch")
		result.FailedValidation = true
		return result

	}
	// validation done. If reuse nonce is not active we better return
	if !reuseNonceActive {
		return result
	}
	seenNonce, otherTx := t.track(ethTx, network)
	if !seenNonce {
		return result
	}

	// if we have shortID we should not block this transaction even in case of reuse nonce.
	// we called t.track() to keep track of this transaction so it may block other transactions.
	if shortID != types.ShortIDEmpty {
		log.Tracef("reuse nonce detected but ignored since having shortID (%v). "+
			"New transaction %v is reusing nonce with existing tx %v",
			shortID, result.Transaction.Hash(), otherTx)
		return result
	}

	result.ReuseSenderNonce = true
	result.DebugData = otherTx
	log.Tracef("reuse nonce detected. New transaction %v is reusing nonce with existing tx %v",
		result.Transaction.Hash(), otherTx)
	// remove the tx from the TxStore but allow it to get back in
	hashToRemove := make(types.SHA256HashList, 1)
	hashToRemove[0] = result.Transaction.Hash()
	t.BxTxStore.RemoveHashes(&hashToRemove, NoReEntryProtection, "reuse nonce detected")
	return result
}

// Stop halts the nonce tracker in addition to regular tx service cleanup
func (t *EthTxStore) Stop() {
	t.BxTxStore.Stop()
	t.nonceTracker.quit <- true
	<-t.nonceTracker.quit
}

type trackedTx struct {
	tx *types.EthTransaction

	// txs with a gas fees higher than both of this are not considered duplicates
	gasFeeCap types.EthBigInt
	gasTipCap types.EthBigInt

	expireTime time.Time // after this time, txs with same key are not considered duplicates
}

type nonceTracker struct {
	clock            utils.Clock
	addressNonceToTx cmap.ConcurrentMap
	cleanInterval    time.Duration
	networkConfig    sdnmessage.BlockchainNetworks
	quit             chan bool
}

func fromNonceKey(from types.EthAddress, nonce types.EthUInt64) string {
	return fmt.Sprintf("%v:%v", from, nonce)
}

func newNonceTracker(clock utils.Clock, networkConfig sdnmessage.BlockchainNetworks, cleanInterval time.Duration) nonceTracker {
	nt := nonceTracker{
		clock:            clock,
		networkConfig:    networkConfig,
		addressNonceToTx: cmap.New(),
		cleanInterval:    cleanInterval,
		quit:             make(chan bool),
	}
	go nt.cleanLoop()
	return nt
}

func (nt *nonceTracker) getTransaction(from types.EthAddress, nonce types.EthUInt64) (*trackedTx, bool) {
	k := fromNonceKey(from, nonce)
	utx, ok := nt.addressNonceToTx.Get(k)
	if !ok {
		return nil, ok
	}
	tx := utx.(trackedTx)
	return &tx, ok
}

func (nt *nonceTracker) setTransaction(tx *types.EthTransaction, network types.NetworkNum) {
	reuseNonceGasChange := new(big.Float).SetFloat64(nt.networkConfig[network].AllowGasPriceChangeReuseSenderNonce)
	reuseNonceDelay := time.Duration(nt.networkConfig[network].AllowTimeReuseSenderNonce) * time.Second

	intGasFeeCap := new(big.Int)
	gasFeeCap := new(big.Float).SetInt(tx.EffectiveGasFeeCap().Int)
	gasFeeCap.Mul(gasFeeCap, reuseNonceGasChange).Int(intGasFeeCap)

	intGasTipCap := new(big.Int)
	gasTipCap := new(big.Float).SetInt(tx.EffectiveGasTipCap().Int)
	gasTipCap.Mul(gasTipCap, reuseNonceGasChange).Int(intGasTipCap)

	tracked := trackedTx{
		tx:         tx,
		expireTime: nt.clock.Now().Add(reuseNonceDelay),
		gasFeeCap:  types.EthBigInt{Int: intGasFeeCap},
		gasTipCap:  types.EthBigInt{Int: intGasTipCap},
	}
	nt.addressNonceToTx.Set(fromNonceKey(tx.From, tx.Nonce), tracked)
}

// isReuseNonceActive returns whether reuse nonce tracking is active
func (nt nonceTracker) isReuseNonceActive(networkNum types.NetworkNum) bool {
	config := nt.networkConfig[networkNum]
	return config != nil && config.EnableCheckSenderNonce
}

// track returns whether the tx is the newest from its address, and if it should be considered a duplicate
func (nt *nonceTracker) track(tx *types.EthTransaction, network types.NetworkNum) (bool, *types.SHA256Hash) {
	oldTx, ok := nt.getTransaction(tx.From, tx.Nonce)
	if !ok {
		nt.setTransaction(tx, network)
		return false, nil
	}

	if (tx.EffectiveGasFeeCap().GreaterThan(oldTx.gasFeeCap) && tx.EffectiveGasTipCap().GreaterThan(oldTx.gasTipCap)) || nt.clock.Now().After(oldTx.expireTime) {
		nt.setTransaction(tx, network)
		return false, nil
	}
	return true, &oldTx.tx.Hash.SHA256Hash
}

func (nt *nonceTracker) cleanLoop() {
	ticker := nt.clock.Ticker(nt.cleanInterval)
	for {
		select {
		case <-ticker.Alert():
			nt.clean()
		case <-nt.quit:
			ticker.Stop()
			return
		}
	}
}

func (nt *nonceTracker) clean() {
	currentTime := nt.clock.Now()
	sizeBefore := nt.addressNonceToTx.Count()
	removed := 0
	for item := range nt.addressNonceToTx.IterBuffered() {
		tracked := item.Val.(trackedTx)
		if currentTime.After(tracked.expireTime) {
			nt.addressNonceToTx.Remove(item.Key)
			removed++
		}
	}
	log.Tracef("nonceTracker Cleanup done. Size at start %v, cleaned %v", sizeBefore, removed)
}
