package services

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/bloXroute-Labs/bxcommon-go/v2/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/v2/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/v2/sdnsdk/message"
	"github.com/bloXroute-Labs/bxcommon-go/v2/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
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
func NewEthTxStore(clock clock.Clock, cleanupInterval time.Duration,
	noSIDAge time.Duration, assigner ShortIDAssigner, hashHistory HashHistory, cleanedShortIDsChannel chan types.ShortIDsByNetwork,
	networkConfig sdnmessage.BlockchainNetworks, bloom BloomFilter, blobCompressorStorage BlobCompressorStorage,
	blobsCleanerEnabled bool, senderExtractor *SenderExtractor,
) *EthTxStore {
	bxStore := newBxTxStore(clock, networkConfig, cleanupInterval, noSIDAge, assigner, hashHistory, cleanedShortIDsChannel, timeToAvoidReEntry, bloom, blobCompressorStorage, blobsCleanerEnabled, senderExtractor)
	return &EthTxStore{
		BxTxStore:    bxStore,
		nonceTracker: newNonceTracker(clock, networkConfig, cleanNonceInterval),
	}
}

// Add validates an Ethereum transaction and checks that its nonce has not been seen before
func (t *EthTxStore) Add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID,
	network bxtypes.NetworkNum, validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64,
	sender types.Sender,
) types.TransactionResult {
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
	network bxtypes.NetworkNum, validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64, sender types.Sender,
) types.TransactionResult {
	transaction := types.NewBxTransaction(hash, network, flags, timestamp)
	var err error

	needsValidation := validate && !t.HasContent(hash)

	// sender is only trusted when the tx does not require validation (e.g. comes from a relay);
	// otherwise it is extracted from the signature on demand
	ethTxSender := sender
	if needsValidation {
		ethTxSender = types.EmptySender
	}
	ethTx := types.NewEthTransactionFromBytes(content, ethTxSender)

	if needsValidation {
		transaction.SetContent(content)

		tx, decodeErr := ethTx.Tx()
		if decodeErr != nil {
			return types.TransactionResult{Transaction: transaction, EthTx: ethTx, FailedValidation: true, DebugData: decodeErr}
		}

		txChainID, err := ethTx.ChainID()
		if err != nil {
			return types.TransactionResult{Transaction: transaction, EthTx: ethTx, FailedValidation: true, DebugData: err}
		}
		if networkChainID != 0 && txChainID.Int64() != 0 && networkChainID != txChainID.Int64() {
			errChainIDMismatch := fmt.Errorf("chainID mismatch for hash %v - content chainID %v networkNum %v networkChainID %v", hash, txChainID, network, networkChainID)
			log.Error(errChainIDMismatch)
			return types.TransactionResult{Transaction: transaction, EthTx: ethTx, FailedValidation: true, DebugData: errChainIDMismatch}
		}

		sender = types.EmptySender

		if t.isReuseNonceActive(network) {
			sender, err = ethTx.Sender()
			if err != nil {
				errExtractionFailed := fmt.Errorf("failed to extract sender from transaction %v", hash)
				log.Error(errExtractionFailed)
				return types.TransactionResult{Transaction: transaction, EthTx: ethTx, FailedValidation: true, DebugData: errExtractionFailed}
			}
		}

		txType, err := ethTx.Type()
		if err != nil {
			return types.TransactionResult{Transaction: transaction, EthTx: ethTx, FailedValidation: true, DebugData: err}
		}
		if txType == ethtypes.BlobTxType {
			if tx.BlobTxSidecar() == nil {
				errEmptySidecar := fmt.Errorf("missing sidecar for hash %v", hash)
				log.Error(errEmptySidecar)
				return types.TransactionResult{Transaction: transaction, EthTx: ethTx, FailedValidation: true, DebugData: errEmptySidecar}
			}
			log.Tracef("adding flag TFWithSidecar for transaction %v", hash)

			transaction.AddFlags(types.TFWithSidecar)
		}
	}

	result := t.BxTxStore.Add(hash, content, shortID, network, false, transaction.Flags(), timestamp, networkChainID, sender)
	result.EthTx = ethTx

	if !result.NewContent {
		return result
	}
	if t.senderExtractor != nil {
		t.submitTxsToSenderExtractor(result)
	}

	if !result.Transaction.Flags().IsWithSidecar() && (!t.isReuseNonceActive(network) || sender == types.EmptySender) {
		return result
	}

	result.Nonce, err = ethTx.Nonce()
	if err != nil {
		log.Errorf("unable to get nonce for transaction %v", result.Transaction.Hash())
		result.FailedValidation = true
		return result
	}

	if result.Transaction.Flags().IsWithSidecar() {
		tx, err := ethTx.Tx()
		if err != nil {
			log.Errorf("unable to parse already validated transaction %v with content %v", result.Transaction.Hash(), result.Transaction.Content())
			result.FailedValidation = true
			return result
		}
		t.blobCompressorStorage.StoreKzgCommitmentToTxHashRecords(tx)
	}

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
		result.Transaction.AddFlags(types.TFReusedNonce)
		from, _ := ethTx.From()   //nolint:errcheck // from used for debug message only; error path is non-critical
		nonce, _ := ethTx.Nonce() //nolint:errcheck // nonce used for debug message only; error path is non-critical
		result.DebugData = fmt.Sprintf("reuse nonce detected. New transaction %v from %v with nonce %v is reusing nonce with existing tx %v on network %v", result.Transaction.Hash(), from, nonce, otherTx, networkChainID)
		return result
	}

	return result
}

func (t *EthTxStore) submitTxsToSenderExtractor(result types.TransactionResult) {
	t.senderExtractor.submitEth(result.EthTx)
}

// Stop halts the nonce tracker in addition to regular tx service cleanup
func (t *EthTxStore) Stop() {
	t.BxTxStore.Stop()
	t.nonceTracker.quit <- true
	<-t.nonceTracker.quit
}

type trackedTx struct {
	hash types.SHA256Hash

	// all gas prices should be increased to not consider the same transaction as duplicate
	gasFeeCap  *big.Int
	gasTipCap  *big.Int
	blobGasCap *big.Int

	expireTime time.Time // after this time, txs with same key are not considered duplicates
}

type nonceTracker struct {
	clock            clock.Clock
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

func newNonceTracker(clock clock.Clock, networkConfig sdnmessage.BlockchainNetworks, cleanInterval time.Duration) nonceTracker {
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

func (nt *nonceTracker) setTransaction(tx *types.EthTransaction, from *common.Address, network bxtypes.NetworkNum) {
	reuseNonceGasChange := new(big.Float).SetFloat64(nt.networkConfig[network].AllowGasPriceChangeReuseSenderNonce)
	reuseNonceDelay := time.Duration(nt.networkConfig[network].AllowTimeReuseSenderNonce) * time.Second

	gasFeeCap, err := tx.EffectiveGasFeeCap()
	if err != nil {
		log.Errorf("unable to get gas fee cap for transaction: %v", err)
		return
	}
	intGasFeeCap := new(big.Int)
	gasFeeCapF := new(big.Float).SetInt(gasFeeCap)
	gasFeeCapF.Mul(gasFeeCapF, reuseNonceGasChange).Int(intGasFeeCap)

	gasTipCap, err := tx.EffectiveGasTipCap()
	if err != nil {
		log.Errorf("unable to get gas tip cap for transaction: %v", err)
		return
	}
	intGasTipCap := new(big.Int)
	gasTipCapF := new(big.Float).SetInt(gasTipCap)
	gasTipCapF.Mul(gasTipCapF, reuseNonceGasChange).Int(intGasTipCap)

	blobGasCap, err := tx.EffectiveBlobGasFeeCap()
	if err != nil {
		log.Errorf("unable to get blob gas fee cap for transaction: %v", err)
		return
	}
	intBlobGasCap := new(big.Int)
	blobGasCapF := new(big.Float).SetInt(blobGasCap)
	blobGasCapF.Mul(blobGasCapF, reuseNonceGasChange).Int(intBlobGasCap)

	txHash, err := tx.Hash()
	if err != nil {
		log.Errorf("unable to get hash for transaction: %v", err)
		return
	}

	nonce, err := tx.Nonce()
	if err != nil {
		log.Errorf("unable to get nonce for transaction: %v", err)
		return
	}

	tracked := trackedTx{
		hash:       txHash,
		expireTime: nt.clock.Now().Add(reuseNonceDelay),
		gasFeeCap:  intGasFeeCap,
		gasTipCap:  intGasTipCap,
		blobGasCap: intBlobGasCap,
	}
	nt.addressNonceToTx.Store(fromNonceKey(from, nonce), tracked)
}

// isReuseNonceActive returns whether reuse nonce tracking is active
func (nt nonceTracker) isReuseNonceActive(networkNum bxtypes.NetworkNum) bool {
	config := nt.networkConfig[networkNum]
	return config != nil && config.EnableCheckSenderNonce
}

// track returns whether the tx is the newest from its address, and if it should be considered a duplicate
func (nt *nonceTracker) track(tx *types.EthTransaction, network bxtypes.NetworkNum) (bool, *types.SHA256Hash, error) {
	from, err := tx.From()
	if err != nil {
		return false, nil, err
	}

	nonce, err := tx.Nonce()
	if err != nil {
		return false, nil, err
	}

	oldTx, ok := nt.getTransaction(from, nonce)
	if !ok {
		nt.setTransaction(tx, from, network)
		return false, nil, nil
	}

	gasFeeCap, err := tx.EffectiveGasFeeCap()
	if err != nil {
		return false, nil, err
	}
	gasTipCap, err := tx.EffectiveGasTipCap()
	if err != nil {
		return false, nil, err
	}
	blobGasCmp, err := tx.EffectiveBlobGasFeeCapIntCmp(oldTx.blobGasCap)
	if err != nil {
		return false, nil, err
	}

	if (gasFeeCap.Cmp(oldTx.gasFeeCap) >= 0 && gasTipCap.Cmp(oldTx.gasTipCap) >= 0 && blobGasCmp >= 0) || nt.clock.Now().After(oldTx.expireTime) {
		nt.setTransaction(tx, from, network)
		return false, nil, nil
	}
	return true, &oldTx.hash, nil
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
