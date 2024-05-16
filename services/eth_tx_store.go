package services

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
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
	noSIDAge time.Duration, assigner ShortIDAssigner, hashHistory HashHistory, cleanedShortIDsChannel chan types.ShortIDsByNetwork,
	networkConfig sdnmessage.BlockchainNetworks, bloom BloomFilter, blobCompressorStorage BlobCompressorStorage) *EthTxStore {
	return &EthTxStore{
		BxTxStore:    newBxTxStore(clock, cleanupInterval, maxTxAge, noSIDAge, assigner, hashHistory, cleanedShortIDsChannel, timeToAvoidReEntry, bloom, blobCompressorStorage),
		nonceTracker: newNonceTracker(clock, networkConfig, cleanNonceInterval),
	}
}

// Add validates an Ethereum transaction and checks that its nonce has not been seen before
func (t *EthTxStore) Add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID,
	network types.NetworkNum, validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64,
	sender types.Sender) TransactionResult {
	result := t.add(hash, content, shortID, network, validate, flags, timestamp, networkChainID, sender)

	if result.Transaction.Flags().IsReuseSenderNonce() {
		// make sure reuse nonce will not be delivered to the node
		result.Transaction.RemoveFlags(types.TFDeliverToNode)

		// no reprocess in case of reuse nonce
		result.Reprocess = false
	}

	return result
}

// Add validates an Ethereum transaction and checks that its nonce has not been seen before
func (t *EthTxStore) add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID,
	network types.NetworkNum, validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64, sender types.Sender) TransactionResult {

	transaction := types.NewBxTransaction(hash, network, flags, timestamp)
	var blockchainTx types.BlockchainTransaction
	var err error

	if validate && !t.HasContent(hash) {
		// If validate is true we got the tx from gw or cloud-api (with content).
		// If we don't know this hash, or we don't have its content we should validate
		// it and extract the sender (so we pass EmptySender)
		transaction.SetContent(content)
		blockchainTx, err = transaction.BlockchainTransaction(types.EmptySender)
		if err != nil {
			return TransactionResult{Transaction: transaction, FailedValidation: true, DebugData: err}
		}

		ethTx := blockchainTx.(*types.EthTransaction)

		txChainID := ethTx.ChainID().Int64()
		if networkChainID != 0 && txChainID != 0 && networkChainID != txChainID {
			errChainIDMismatch := fmt.Errorf("chainID mismatch for hash %v - content chainID %v networkNum %v networkChainID %v", hash, txChainID, network, networkChainID)
			log.Error(errChainIDMismatch)
			return TransactionResult{Transaction: transaction, FailedValidation: true, DebugData: errChainIDMismatch}
		}

		sender = types.EmptySender

		// If this is not trusted source(external gateway) and network requires, then we extract sender
		if t.isReuseNonceActive(network) {
			sender, err = ethTx.Sender()
			if err != nil {
				errExtractionFailed := fmt.Errorf("failed to extract sender from transaction %v", hash)
				log.Error(errExtractionFailed)
				return TransactionResult{Transaction: transaction, FailedValidation: true, DebugData: errExtractionFailed}
			}
		}

		if ethTx.Type() == ethtypes.BlobTxType {
			if ethTx.Tx().BlobTxSidecar() == nil {
				errEmptySidecar := fmt.Errorf("missing sidecar for hash %v", hash)
				log.Error(errEmptySidecar)
				return TransactionResult{Transaction: transaction, FailedValidation: true, DebugData: errEmptySidecar}
			}
			log.Tracef("adding flag TFWithSidecar for transaction %v", hash)

			transaction.AddFlags(types.TFWithSidecar)
		}

	}

	result := t.BxTxStore.Add(hash, content, shortID, network, false, transaction.Flags(), timestamp, networkChainID, sender)

	// if no new content we can leave
	if !result.NewContent || result.FailedValidation {
		return result
	}

	// sender should already be populated here (not EMPTY) so we will not extract it
	if blockchainTx == nil {
		blockchainTx, err = result.Transaction.BlockchainTransaction(sender)
		if err != nil {
			log.Errorf("unable to parse already validated transaction %v with content %v", result.Transaction.Hash(), result.Transaction.Content())
			result.FailedValidation = true
			return result
		}
	}

	ethTx := blockchainTx.(*types.EthTransaction)
	result.Nonce = ethTx.Nonce()

	if result.Transaction.Flags().IsWithSidecar() {
		// this function is NoOp on Relays
		t.blobCompressorStorage.StoreKzgCommitmentToTxHashRecords(ethTx.Tx())
	}

	// if reuseNonce is disabled or network disables sender extraction, we can leave
	if !t.isReuseNonceActive(network) || sender == types.EmptySender {
		return result
	}

	seenNonce, otherTx, err := t.track(ethTx, network)
	if err != nil {
		log.Errorf("unable to track transaction %v with content %v", result.Transaction.Hash(), result.Transaction.Content())
		result.FailedValidation = true
		return result
	}

	if seenNonce {
		// mark tx as reuse nonce
		result.Transaction.AddFlags(types.TFReusedNonce)
		from, _ := ethTx.From() // we already validated this transaction so we can ignore the error
		result.DebugData = fmt.Sprintf("reuse nonce detected. New transaction %v from %v with nonce %v is reusing nonce with existing tx %v on network %v", result.Transaction.Hash(), from, ethTx.Nonce(), otherTx, networkChainID)
		return result
	}

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

	// all gas prices should be increased to not consider the same transaction as duplicate
	gasFeeCap  *big.Int
	gasTipCap  *big.Int
	blobGasCap *big.Int

	expireTime time.Time // after this time, txs with same key are not considered duplicates
}

type nonceTracker struct {
	clock            utils.Clock
	addressNonceToTx *syncmap.SyncMap[string, trackedTx]
	cleanInterval    time.Duration
	networkConfig    sdnmessage.BlockchainNetworks
	quit             chan bool
}

func fromNonceKey(from *common.Address, nonce uint64) string {
	b := strings.Builder{}
	b.WriteString(string(from.Bytes()))
	b.WriteString(":")
	b.WriteString(strconv.FormatUint(nonce, 10))
	return b.String()
}

func newNonceTracker(clock utils.Clock, networkConfig sdnmessage.BlockchainNetworks, cleanInterval time.Duration) nonceTracker {
	nt := nonceTracker{
		clock:            clock,
		networkConfig:    networkConfig,
		addressNonceToTx: syncmap.NewStringMapOf[trackedTx](),
		cleanInterval:    cleanInterval,
		quit:             make(chan bool),
	}
	go nt.cleanLoop()
	return nt
}

func (nt *nonceTracker) getTransaction(from *common.Address, nonce uint64) (*trackedTx, bool) {
	k := fromNonceKey(from, nonce)
	utx, ok := nt.addressNonceToTx.Load(k)
	if !ok {
		return nil, ok
	}
	tx := utx
	return &tx, ok
}

func (nt *nonceTracker) setTransaction(tx *types.EthTransaction, from *common.Address, network types.NetworkNum) {
	reuseNonceGasChange := new(big.Float).SetFloat64(nt.networkConfig[network].AllowGasPriceChangeReuseSenderNonce)
	reuseNonceDelay := time.Duration(nt.networkConfig[network].AllowTimeReuseSenderNonce) * time.Second

	intGasFeeCap := new(big.Int)
	gasFeeCap := new(big.Float).SetInt(tx.EffectiveGasFeeCap())
	gasFeeCap.Mul(gasFeeCap, reuseNonceGasChange).Int(intGasFeeCap)

	intGasTipCap := new(big.Int)
	gasTipCap := new(big.Float).SetInt(tx.EffectiveGasTipCap())
	gasTipCap.Mul(gasTipCap, reuseNonceGasChange).Int(intGasTipCap)

	intBlobGasCap := new(big.Int)
	blobGasCap := new(big.Float).SetInt(tx.EffectiveBlobGasFeeCap())
	blobGasCap.Mul(blobGasCap, reuseNonceGasChange).Int(intBlobGasCap)

	tracked := trackedTx{
		tx:         tx,
		expireTime: nt.clock.Now().Add(reuseNonceDelay),
		gasFeeCap:  intGasFeeCap,
		gasTipCap:  intGasTipCap,
		blobGasCap: intBlobGasCap,
	}
	nt.addressNonceToTx.Store(fromNonceKey(from, tx.Nonce()), tracked)
}

// isReuseNonceActive returns whether reuse nonce tracking is active
func (nt nonceTracker) isReuseNonceActive(networkNum types.NetworkNum) bool {
	config := nt.networkConfig[networkNum]
	return config != nil && config.EnableCheckSenderNonce
}

// track returns whether the tx is the newest from its address, and if it should be considered a duplicate
func (nt *nonceTracker) track(tx *types.EthTransaction, network types.NetworkNum) (bool, *types.SHA256Hash, error) {
	from, err := tx.From()
	if err != nil {
		return false, nil, err
	}

	oldTx, ok := nt.getTransaction(from, tx.Nonce())
	if !ok {
		nt.setTransaction(tx, from, network)
		return false, nil, nil
	}

	if (tx.EffectiveGasFeeCap().Cmp(oldTx.gasFeeCap) >= 0 && tx.EffectiveGasTipCap().Cmp(oldTx.gasTipCap) >= 0 && tx.EffectiveBlobGasFeeCapIntCmp(oldTx.blobGasCap) >= 0) || nt.clock.Now().After(oldTx.expireTime) {
		nt.setTransaction(tx, from, network)
		return false, nil, nil
	}
	hash := oldTx.tx.Hash()
	return true, &hash, nil
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
	sizeBefore := nt.addressNonceToTx.Size()
	removed := 0

	nt.addressNonceToTx.Range(func(key string, tracked trackedTx) bool {
		if currentTime.After(tracked.expireTime) {
			nt.addressNonceToTx.Delete(key)
			removed++
		}
		return true
	})

	log.Tracef("nonceTracker Cleanup done. Size at start %v, cleaned %v", sizeBefore, removed)
}
