package services

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// error constants for identifying special processing cases
var (
	ErrMissingShortIDs          = errors.New("missing short IDs")
	ErrUnknownBlockType         = errors.New("unknown block type")
	ErrNotCompatibleBeaconBlock = errors.New("not compatible beacon block")
)

func (e *ErrAlreadyProcessed) Error() string {
	return "already processed " + string(e.status)
}

// Status return ErrAlreadyProcessed status
func (e *ErrAlreadyProcessed) Status() SeenStatus {
	return e.status
}

func newErrAlreadyProcessed(status SeenStatus) error {
	return &ErrAlreadyProcessed{status}
}

// ErrAlreadyProcessed represent ErrAlreadyProcessed error with status
type ErrAlreadyProcessed struct {
	status SeenStatus
}

// BxBlockConverter is the service interface for converting broadcast messages to/from bx blocks
type BxBlockConverter interface {
	BxBlockToBroadcast(*types.BxBlock, bxtypes.NetworkNum, time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error)
	BxBlockFromBroadcast(*bxmessage.Broadcast) (*types.BxBlock, types.ShortIDList, error)
}

// BlockProcessor is the service interface for processing broadcast messages
type BlockProcessor interface {
	BxBlockConverter
}

// NewBlockProcessor returns a BlockProcessor for execution layer and consensus layer blocks encoded in broadcast messages
func NewBlockProcessor(txStore TxStore) BlockProcessor {
	bp := &blockProcessor{
		txStore:         txStore,
		processedBlocks: NewBlockHistory("processedBlocks", 30*time.Minute, clock.RealClock{}),
	}
	return bp
}

type blockProcessor struct {
	txStore         TxStore
	processedBlocks BlockHistory
}

type bxCompressedTransaction struct {
	IsFullTransaction bool
	Transaction       []byte `ssz-max:"1073741824"`
}

type bxBroadcastBSCBlobSidecar struct {
	Data rlp.RawValue
}

// BxBlockSSZ is a struct for SSZ encoding/decoding of a block
// To regenerate block_processor_encoding.go file:
// 1. clone github.com/prysmaticlabs/fastssz
// 2. $ go run sszgen/*.go --path {SUBSTITUTE_WITH_PATH_TO_REPO}/gateway/services --objs=BxBlockSSZ
type BxBlockSSZ struct {
	Block  []byte                     `ssz-max:"367832"`
	Txs    []*bxCompressedTransaction `ssz-max:"1048576,1073741825" ssz-size:"?,?"`
	Number uint64
}

type bxBlockRLP struct {
	Header          rlp.RawValue
	Txs             []bxCompressedTransaction
	Trailer         rlp.RawValue
	TotalDifficulty *big.Int
	Number          *big.Int
	Sidecars        []bxBroadcastBSCBlobSidecar `rlp:"optional"`
}

func (bp *blockProcessor) BxBlockToBroadcast(block *types.BxBlock, networkNum bxtypes.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
	blockHash := block.Hash().String()
	status := bp.processedBlocks.Status(blockHash)
	switch status {
	case SeenFromRelay:
		bp.processedBlocks.AddOrUpdate(blockHash, SeenFromNode)
		return nil, nil, newErrAlreadyProcessed(SeenFromRelay)
	case SeenFromNode:
		return nil, nil, newErrAlreadyProcessed(SeenFromNode)
	case SeenFromBoth:
		return nil, nil, newErrAlreadyProcessed(SeenFromBoth)
	}

	var usedShortIDs types.ShortIDList
	var broadcastMessage *bxmessage.Broadcast
	var err error
	switch block.Type {
	case types.BxBlockTypeEth:
		broadcastMessage, usedShortIDs, err = bp.newRLPBlockBroadcast(block, networkNum, minTxAge)
	case types.BxBlockTypeBeaconDeneb, types.BxBlockTypeBeaconElectra, types.BxBlockTypeBeaconFulu:
		broadcastMessage, usedShortIDs, err = bp.newSSZBlockBroadcast(block, networkNum, minTxAge)
	case types.BxBlockTypeUnknown:
		return nil, nil, ErrUnknownBlockType
	}

	if err != nil {
		return nil, nil, err
	}

	switch block.Type {
	case types.BxBlockTypeEth:
		bp.markProcessed(block.Hash(), SeenFromNode)
	case types.BxBlockTypeBeaconDeneb, types.BxBlockTypeBeaconElectra, types.BxBlockTypeBeaconFulu:
		bp.markProcessed(block.BeaconHash(), SeenFromNode)
	}

	return broadcastMessage, usedShortIDs, nil
}

// BxBlockFromBroadcast processes the encoded compressed block in a broadcast message, replacing all short IDs with their stored transaction contents
func (bp *blockProcessor) BxBlockFromBroadcast(broadcast *bxmessage.Broadcast) (*types.BxBlock, types.ShortIDList, error) {
	var blockHash string

	switch broadcast.BlockType() {
	case types.BxBlockTypeEth:
		blockHash = broadcast.Hash().String()
	case types.BxBlockTypeBeaconDeneb, types.BxBlockTypeBeaconElectra, types.BxBlockTypeBeaconFulu:
		if broadcast.BeaconHash().Empty() {
			return nil, nil, ErrNotCompatibleBeaconBlock
		}
		blockHash = broadcast.BeaconHash().String()
	case types.BxBlockTypeUnknown:
		return nil, nil, ErrUnknownBlockType
	}

	status := bp.processedBlocks.Status(blockHash)
	if status != FirstTimeSeen {
		bp.processedBlocks.AddOrUpdate(blockHash, SeenFromRelay)
		return nil, nil, newErrAlreadyProcessed(SeenStatus(status))
	}

	shortIDs := broadcast.ShortIDs()
	var err error

	bxTransactions, missingShortIDs := bp.fetchTxsWithRetry(shortIDs, 5*time.Millisecond, 200*time.Millisecond)
	if len(missingShortIDs) > 0 {
		return nil, missingShortIDs, ErrMissingShortIDs
	}

	var block *types.BxBlock
	switch broadcast.BlockType() {
	case types.BxBlockTypeEth:
		block, err = bp.newBxBlockFromRLPBroadcast(broadcast, bxTransactions)

		if err == nil {
			bp.markProcessed(broadcast.Hash(), SeenFromRelay)
		}
	case types.BxBlockTypeBeaconDeneb, types.BxBlockTypeBeaconElectra, types.BxBlockTypeBeaconFulu:
		block, err = bp.newBxBlockFromSSZBroadcast(broadcast, bxTransactions)

		if err == nil {
			bp.markProcessed(broadcast.Hash(), SeenFromRelay)
			bp.markProcessed(broadcast.BeaconHash(), SeenFromRelay)
		}
	case types.BxBlockTypeUnknown:
		return nil, nil, ErrUnknownBlockType
	}

	return block, missingShortIDs, err
}

// fetchTxsWithRetry tries to get transactions by short IDs from txStore, polling interval until timeout.
func (bp *blockProcessor) fetchTxsWithRetry(shortIDs []types.ShortID, interval time.Duration, timeout time.Duration) (txs []*types.BxTransaction, missing types.ShortIDList) {
	// split into helper calls for clarity
	txs, missingIndices := bp.initialFetch(shortIDs)
	if len(missingIndices) == 0 {
		return txs, nil
	}
	return bp.pollMissing(shortIDs, txs, missingIndices, interval, timeout)
}

// initialFetch tries to fetch all shortIDs once and returns the tx slice and indices that were missing
func (bp *blockProcessor) initialFetch(shortIDs []types.ShortID) ([]*types.BxTransaction, []int) {
	txs := make([]*types.BxTransaction, len(shortIDs))
	missingIndices := make([]int, 0)
	for i, sid := range shortIDs {
		bxTransaction, err := bp.txStore.GetTxByShortID(sid, false)
		if err == nil {
			txs[i] = bxTransaction
		} else {
			missingIndices = append(missingIndices, i)
		}
	}
	return txs, missingIndices
}

// pollMissing polls only the missing indices until timeout and fills txs in-place when found
func (bp *blockProcessor) pollMissing(shortIDs []types.ShortID, txs []*types.BxTransaction, missingIndices []int, interval, timeout time.Duration) ([]*types.BxTransaction, types.ShortIDList) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timeoutCh := time.After(timeout)
	intervalsMade := 1

	for {
		select {
		case <-ticker.C:
			intervalsMade++
			var stillMissing []int
			for _, idx := range missingIndices {
				sid := shortIDs[idx]
				bxTransaction, err := bp.txStore.GetTxByShortID(sid, false)
				if err == nil {
					txs[idx] = bxTransaction
				} else {
					stillMissing = append(stillMissing, idx)
				}
			}

			if len(stillMissing) == 0 {
				if intervalsMade > 1 {
					log.Debugf("successfully fetched %d transactions with %d intervals", len(txs), intervalsMade)
				}
				return txs, nil
			}
			missingIndices = stillMissing
		case <-timeoutCh:
			missingShortIDs := make(types.ShortIDList, 0, len(missingIndices))
			for _, idx := range missingIndices {
				missingShortIDs = append(missingShortIDs, shortIDs[idx])
			}
			return nil, missingShortIDs
		}
	}
}

func (bp *blockProcessor) processSidecarsFromRLPBroadcast(rlpSidecars []bxBroadcastBSCBlobSidecar) ([]*types.BxBSCBlobSidecar, uint64, error) {
	blobSidecars := make([]*types.BxBSCBlobSidecar, len(rlpSidecars))
	sidecarsSizeBytes := uint64(0)

	for i, rlpSidecar := range rlpSidecars {
		sidecarsSizeBytes += uint64(len(rlpSidecar.Data))

		blobSidecar := new(types.BxBSCBlobSidecar)
		if err := rlp.DecodeBytes(rlpSidecar.Data, blobSidecar); err != nil {
			return nil, 0, fmt.Errorf("failed to decode sidecar: %v", err)
		}

		if blobSidecar.IsCompressed {
			hash, err := types.NewSHA256Hash(blobSidecar.TxHash[:])
			if err != nil {
				return nil, 0, fmt.Errorf("failed to create SHA256 hash from tx hash: %v", err)
			}

			tx, ok := bp.txStore.Get(hash)
			if !ok {
				return nil, 0, fmt.Errorf("failed to get blob sidecar by tx hash: %v", hash)
			}

			var ethTx ethtypes.Transaction
			err = rlp.DecodeBytes(tx.Content(), &ethTx)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to decode Ethereum transaction: %v", err)
			}

			if ethTx.BlobTxSidecar() == nil {
				return nil, 0, fmt.Errorf("failed to get blob sidecar from Ethereum transaction")
			}

			log.Tracef("successfully decompressed eth block blob sidecar, index: %d, tx hash: %s", blobSidecar.TxIndex, blobSidecar.TxHash.String())

			blobSidecar.TxSidecar = &common.BlobTxSidecar{
				Blobs:       ethTx.BlobTxSidecar().Blobs,
				Commitments: ethTx.BlobTxSidecar().Commitments,
				Proofs:      ethTx.BlobTxSidecar().Proofs,
			}
			blobSidecar.IsCompressed = false
		} else {
			log.Tracef("eth block blob sidecar is not compressed, tx hash: %s", blobSidecar.TxHash.String())
		}
		blobSidecars[i] = blobSidecar
	}

	return blobSidecars, sidecarsSizeBytes, nil
}

func (bp *blockProcessor) newBxBlockFromRLPBroadcast(broadcast *bxmessage.Broadcast, bxTransactions []*types.BxTransaction) (*types.BxBlock, error) {
	var rlpBlock bxBlockRLP
	if err := rlp.DecodeBytes(broadcast.Block(), &rlpBlock); err != nil {
		return nil, err
	}

	compressedTransactionCount := 0
	txs := make([]*types.BxBlockTransaction, 0, len(rlpBlock.Txs))

	var txsBytes uint64
	for _, tx := range rlpBlock.Txs {
		if !tx.IsFullTransaction {
			if compressedTransactionCount >= len(bxTransactions) {
				return nil, fmt.Errorf("could not decompress bad block: more empty transactions than short IDs provided")
			}
			txs = append(txs, types.NewBxBlockTransaction(bxTransactions[compressedTransactionCount].Hash(), bxTransactions[compressedTransactionCount].Content()))
			txsBytes += uint64(len(bxTransactions[compressedTransactionCount].Content()))
			compressedTransactionCount++
		} else {
			txs = append(txs, types.NewRawBxBlockTransaction(tx.Transaction))
			txsBytes += uint64(len(tx.Transaction))
		}
	}

	var blobSidecars []*types.BxBSCBlobSidecar
	var err error
	var blobSidecarsSize uint64
	if len(rlpBlock.Sidecars) > 0 {
		blobSidecars, blobSidecarsSize, err = bp.processSidecarsFromRLPBroadcast(rlpBlock.Sidecars)
		if err != nil {
			return nil, fmt.Errorf("failed to process sidecars: %v", err)
		}
	}
	blockSize := int(rlp.ListSize(uint64(len(rlpBlock.Header))+rlp.ListSize(txsBytes)+uint64(len(rlpBlock.Trailer))+blobSidecarsSize)) + 1 // empty withdrawals list

	return types.NewRawBxBlock(broadcast.Hash(), types.EmptyHash, broadcast.BlockType(), rlpBlock.Header, txs, rlpBlock.Trailer, rlpBlock.TotalDifficulty, rlpBlock.Number, blockSize, blobSidecars), nil
}

func (bp *blockProcessor) newBxBlockFromSSZBroadcast(broadcast *bxmessage.Broadcast, bxTransactions []*types.BxTransaction) (*types.BxBlock, error) {
	var sszBlock BxBlockSSZ
	if err := sszBlock.UnmarshalSSZ(broadcast.Block()); err != nil {
		return nil, err
	}

	compressedTransactionCount := 0
	txs := make([]*types.BxBlockTransaction, 0, len(sszBlock.Txs))

	var txsBytes int
	for _, tx := range sszBlock.Txs {
		if !tx.IsFullTransaction {
			if compressedTransactionCount >= len(bxTransactions) {
				return nil, fmt.Errorf("could not decompress bad block: more empty transactions than short IDs provided")
			}
			txs = append(txs, types.NewRawBxBlockTransaction(bxTransactions[compressedTransactionCount].Content()))
			txsBytes += calcBeaconTransactionLength(bxTransactions[compressedTransactionCount].Content())
			compressedTransactionCount++
		} else {
			txs = append(txs, types.NewRawBxBlockTransaction(tx.Transaction))
			txsBytes += calcBeaconTransactionLength(tx.Transaction)
		}
	}

	blockSize := len(sszBlock.Block) + txsBytes

	return types.NewRawBxBlock(broadcast.Hash(), broadcast.BeaconHash(), broadcast.BlockType(), nil, txs, sszBlock.Block, nil, big.NewInt(0).SetUint64(sszBlock.Number), blockSize, nil), nil
}

func calcBeaconTransactionLength(rawTx []byte) int {
	// tx.MarshalBinary which used in beacon blocks encodes non Legacy transactions differently
	// It puts first byte with type and then encodes everything else in RLP
	// On other side our gateway using tx.EncodeRLP which instead puts everything including type in RLP
	// Which means that it would have 1-3 bytes overhead
	// More info could be found in source of mentioned methods and in RLP docs:
	// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/#definition

	if len(rawTx) == 0 {
		return 0
	}

	// Anyway beside said above SSZ encodes 4 bytes for length of transaction
	txLen := len(rawTx) + 4

	// Checking transaction is non Legacy
	// Also first bytes saying in what ranges is transaction length
	if rawTx[0] < 0xC0 {
		// Only one byte for encoding transaction legth
		if rawTx[0] == 0x80 {
			txLen -= 2
		} else if rawTx[0] > 0x80 {
			// Arbitrary amount of bytes encoding length
			// Decoding BigEndian number from byte
			minus := int(new(big.Int).Sub(
				new(big.Int).SetBytes([]byte{rawTx[0]}),
				new(big.Int).SetBytes([]byte{0xb7}),
			).Uint64())
			txLen -= minus + 1
		}
	}

	return txLen
}

func (bp *blockProcessor) processBlobSidecarToRLPBroadcast(bxBlobSidecars []*types.BxBSCBlobSidecar, maxTimestampForCompression time.Time) ([]bxBroadcastBSCBlobSidecar, error) {
	rlpBlobSidecars := make([]bxBroadcastBSCBlobSidecar, len(bxBlobSidecars))
	for i, sidecar := range bxBlobSidecars {
		hash, err := types.NewSHA256Hash(sidecar.TxHash[:])
		if err != nil {
			return nil, fmt.Errorf("failed to create SHA256 hash from tx hash: %v", err)
		}

		bxTransaction, ok := bp.txStore.Get(hash)
		if ok && bxTransaction.AddTime().Before(maxTimestampForCompression) {
			log.Tracef("Successfully compressed eth block blob sidecar, index: %d, tx hash: %s", sidecar.TxIndex, sidecar.TxHash.String())
			sidecar.IsCompressed = true
			sidecar.TxSidecar = nil
		} else {
			log.Tracef("Unable to compress eth block blob sidecar, sending as is: %s", sidecar.TxHash.String())
			sidecar.IsCompressed = false
		}

		encodedSidecar, err := rlp.EncodeToBytes(sidecar)
		if err != nil {
			return nil, fmt.Errorf("failed to encode sidecar: %v", err)
		}
		rlpBlobSidecars[i] = bxBroadcastBSCBlobSidecar{Data: encodedSidecar}
	}
	return rlpBlobSidecars, nil
}

func (bp *blockProcessor) newRLPBlockBroadcast(block *types.BxBlock, networkNum bxtypes.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
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

	var blobSidecars []bxBroadcastBSCBlobSidecar
	var err error
	if len(block.BlobSidecars) > 0 {
		blobSidecars, err = bp.processBlobSidecarToRLPBroadcast(block.BlobSidecars, maxTimestampForCompression)
		if err != nil {
			return nil, usedShortIDs, err
		}
		log.Tracef("Successfully processed %d blob sidecars in block %s", len(blobSidecars), block.Hash().String())
	} else {
		log.Tracef("No blob sidecars found in block %s", block.Hash().String())
	}

	rlpBlock := bxBlockRLP{
		Header:          block.Header,
		Txs:             txs,
		Trailer:         block.Trailer,
		TotalDifficulty: block.TotalDifficulty,
		Number:          block.Number,
		Sidecars:        blobSidecars,
	}

	encodedBlock, err := rlp.EncodeToBytes(rlpBlock)
	if err != nil {
		return nil, usedShortIDs, err
	}

	return bxmessage.NewBlockBroadcast(block.ExecutionHash(), types.EmptyHash, block.Type, encodedBlock, usedShortIDs, networkNum), usedShortIDs, nil
}

func (bp *blockProcessor) newSSZBlockBroadcast(block *types.BxBlock, networkNum bxtypes.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
	usedShortIDs := make(types.ShortIDList, 0)
	txs := make([]*bxCompressedTransaction, 0, len(block.Txs))
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
				txs = append(txs, &bxCompressedTransaction{
					IsFullTransaction: false,
					Transaction:       []byte{},
				})
				continue
			}
		}
		txs = append(txs, &bxCompressedTransaction{
			IsFullTransaction: true,
			Transaction:       tx.Content(),
		})
	}

	sszBlock := BxBlockSSZ{
		Block:  block.Trailer,
		Txs:    txs,
		Number: block.Number.Uint64(),
	}

	encodedBlock, err := sszBlock.MarshalSSZ()
	if err != nil {
		return nil, usedShortIDs, err
	}

	return bxmessage.NewBlockBroadcast(block.ExecutionHash(), block.BeaconHash(), block.Type, encodedBlock, usedShortIDs, networkNum), usedShortIDs, nil
}

func (bp *blockProcessor) markProcessed(hash types.SHA256Hash, status SeenStatus) {
	bp.processedBlocks.AddOrUpdate(hash.String(), status)
}
