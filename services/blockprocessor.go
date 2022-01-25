package services

import (
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/rlp"
	log "github.com/sirupsen/logrus"
	"math/big"
	"sync"
	"time"
)

// error constants for identifying special processing casess
var (
	ErrAlreadyProcessed = errors.New("already processed")
	ErrMissingShortIDs  = errors.New("missing short IDs")
)

// BxBlockConverter is the service interface for converting broadcast messages to/from bx blocks
type BxBlockConverter interface {
	BxBlockToBroadcast(*types.BxBlock, types.NetworkNum, time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error)
	BxBlockFromBroadcast(*bxmessage.Broadcast) (*types.BxBlock, types.ShortIDList, error)
}

// BlockProcessor is the service interface for processing broadcast messages
type BlockProcessor interface {
	BxBlockConverter

	ShouldProcess(hash types.SHA256Hash) bool
	ProcessBroadcast(*bxmessage.Broadcast) (block *types.BxBlock, missingShortIDsCount int, err error)
}

// NewRLPBlockProcessor returns a BlockProcessor for Ethereum blocks encoded in broadcast messages
func NewRLPBlockProcessor(txStore TxStore) BlockProcessor {
	bp := &rlpBlockProcessor{
		txStore:         txStore,
		processedBlocks: NewHashHistory("processedBlocks", 30*time.Minute),
		lock:            &sync.Mutex{},
	}
	return bp
}

type rlpBlockProcessor struct {
	txStore         TxStore
	processedBlocks HashHistory
	lock            *sync.Mutex
}

type bxCompressedTransaction struct {
	IsFullTransaction bool
	Transaction       []byte
}

type bxBlockRLP struct {
	Header          rlp.RawValue
	Txs             []bxCompressedTransaction
	Trailer         rlp.RawValue
	TotalDifficulty *big.Int
	Number          *big.Int
}

func (bp *rlpBlockProcessor) ProcessBroadcast(broadcast *bxmessage.Broadcast) (*types.BxBlock, int, error) {
	bxBlock, missingShortIDs, err := bp.BxBlockFromBroadcast(broadcast)
	if err == ErrMissingShortIDs {
		log.Debugf("block %v from BDN is missing %v short IDs", broadcast.Hash(), len(missingShortIDs))
		return nil, len(missingShortIDs), ErrMissingShortIDs
	} else if err != nil {
		return nil, len(missingShortIDs), err
	}

	return bxBlock, len(missingShortIDs), nil
}

func (bp *rlpBlockProcessor) BxBlockToBroadcast(block *types.BxBlock, networkNum types.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
	bp.lock.Lock()
	defer bp.lock.Unlock()

	blockHash := block.Hash()
	if !bp.ShouldProcess(blockHash) {
		return nil, nil, ErrAlreadyProcessed
	}

	usedShortIDs := make(types.ShortIDList, 0)
	txs := make([]bxCompressedTransaction, 0, len(block.Txs))
	maxTimestampForCompression := time.Now().Add(-minTxAge)
	// compress transactions in block if short ID is known
	for _, tx := range block.Txs {
		txHash := tx.Hash()

		bxTransaction, ok := bp.txStore.Get(txHash)
		if ok && bxTransaction.AddTime().Before(maxTimestampForCompression) {
			shortIDs := bxTransaction.ShortIDs()
			if len(shortIDs) > 0 {
				shortID := shortIDs[0]
				usedShortIDs = append(usedShortIDs, shortID)
				txs = append(txs, bxCompressedTransaction{
					IsFullTransaction: false,
					Transaction:       []byte{},
				})
				continue
			}
		}
		txs = append(txs, bxCompressedTransaction{
			IsFullTransaction: true,
			Transaction:       tx.Content(),
		})
	}

	rlpBlock := bxBlockRLP{
		Header:          block.Header,
		Txs:             txs,
		Trailer:         block.Trailer,
		TotalDifficulty: block.TotalDifficulty,
		Number:          block.Number,
	}
	encodedBlock, err := rlp.EncodeToBytes(rlpBlock)
	if err != nil {
		return nil, usedShortIDs, err
	}

	bp.markProcessed(blockHash)
	broadcastMessage := bxmessage.NewBlockBroadcast(block.Hash(), encodedBlock, usedShortIDs, networkNum)
	return broadcastMessage, usedShortIDs, nil
}

// BxBlockFromBroadcast processes the encoded compressed block in a broadcast message, replacing all short IDs with their stored transaction contents
func (bp *rlpBlockProcessor) BxBlockFromBroadcast(broadcast *bxmessage.Broadcast) (*types.BxBlock, types.ShortIDList, error) {
	bp.lock.Lock()
	defer bp.lock.Unlock()

	blockHash := broadcast.Hash()
	if !bp.ShouldProcess(blockHash) {
		return nil, nil, ErrAlreadyProcessed
	}

	shortIDs := broadcast.ShortIDs()
	var bxTransactions []*types.BxTransaction
	var missingShortIDs types.ShortIDList
	var err error

	// looking for missing sids
	for _, sid := range shortIDs {
		bxTransaction, err := bp.txStore.GetTxByShortID(sid)
		if err == nil { // sid exists in TxStore
			bxTransactions = append(bxTransactions, bxTransaction)
		} else {
			missingShortIDs = append(missingShortIDs, sid)
		}
	}

	if len(missingShortIDs) > 0 {
		return nil, missingShortIDs, ErrMissingShortIDs
	}

	var rlpBlock bxBlockRLP
	if err = rlp.DecodeBytes(broadcast.Block(), &rlpBlock); err != nil {
		return nil, missingShortIDs, err
	}

	compressedTransactionCount := 0
	txs := make([]*types.BxBlockTransaction, 0, len(rlpBlock.Txs))

	for _, tx := range rlpBlock.Txs {
		if !tx.IsFullTransaction {
			if compressedTransactionCount >= len(bxTransactions) {
				return nil, missingShortIDs, fmt.Errorf("could not decompress bad block: more empty transactions than short IDs provided")
			}
			txs = append(txs, types.NewRawBxBlockTransaction(bxTransactions[compressedTransactionCount].Content()))
			compressedTransactionCount++
		} else {
			txs = append(txs, types.NewRawBxBlockTransaction(tx.Transaction))
		}
	}

	bp.markProcessed(blockHash)
	block := types.NewRawBxBlock(broadcast.Hash(), rlpBlock.Header, txs, rlpBlock.Trailer, rlpBlock.TotalDifficulty, rlpBlock.Number)
	return block, missingShortIDs, err
}

func (bp *rlpBlockProcessor) ShouldProcess(hash types.SHA256Hash) bool {
	return !bp.processedBlocks.Exists(hash.String())
}

func (bp *rlpBlockProcessor) markProcessed(hash types.SHA256Hash) {
	bp.processedBlocks.Add(hash.String(), 10*time.Minute)
}
