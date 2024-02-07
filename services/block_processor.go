package services

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/rlp"
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
	BxBlockToBroadcast(*types.BxBlock, types.NetworkNum, time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error)
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
		processedBlocks: NewBlockHistory("processedBlocks", 30*time.Minute, utils.RealClock{}),
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

type bxBlockSSZ struct {
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
}

func (bp *blockProcessor) BxBlockToBroadcast(block *types.BxBlock, networkNum types.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
	var blockHash string
	switch block.Type {
	case types.BxBlockTypeEth:
		blockHash = block.Hash().String()
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella:
		blockHash = block.BeaconHash().String()
	}
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
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella:
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
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella:
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
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella:
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

	var block *types.BxBlock
	switch broadcast.BlockType() {
	case types.BxBlockTypeEth:
		block, err = bp.newBxBlockFromRLPBroadcast(broadcast, bxTransactions)

		if err == nil {
			bp.markProcessed(broadcast.Hash(), SeenFromRelay)
		}
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella:
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
	blockSize := int(rlp.ListSize(uint64(len(rlpBlock.Header)) + rlp.ListSize(txsBytes) + uint64(len(rlpBlock.Trailer))))

	return types.NewRawBxBlock(broadcast.Hash(), types.EmptyHash, broadcast.BlockType(), rlpBlock.Header, txs, rlpBlock.Trailer, rlpBlock.TotalDifficulty, rlpBlock.Number, blockSize), nil
}

func (bp *blockProcessor) newBxBlockFromSSZBroadcast(broadcast *bxmessage.Broadcast, bxTransactions []*types.BxTransaction) (*types.BxBlock, error) {
	var sszBlock bxBlockSSZ
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

	return types.NewRawBxBlock(broadcast.Hash(), broadcast.BeaconHash(), broadcast.BlockType(), nil, txs, sszBlock.Block, nil, big.NewInt(int64(sszBlock.Number)), int(blockSize)), nil
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
			// Arbitery amount of bytes encoding length
			// Decoding BigEndian number from byte
			minus := int(new(big.Int).Sub(
				new(big.Int).SetBytes([]byte{rawTx[0]}),
				new(big.Int).SetBytes([]byte{0xb7}),
			).Uint64())
			txLen -= (minus + 1)
		}
	}

	return txLen
}

func (bp *blockProcessor) newRLPBlockBroadcast(block *types.BxBlock, networkNum types.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
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

	return bxmessage.NewBlockBroadcast(block.Hash(), types.EmptyHash, block.Type, encodedBlock, usedShortIDs, networkNum), usedShortIDs, nil
}

func (bp *blockProcessor) newSSZBlockBroadcast(block *types.BxBlock, networkNum types.NetworkNum, minTxAge time.Duration) (*bxmessage.Broadcast, types.ShortIDList, error) {
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

	sszBlock := bxBlockSSZ{
		Block:  block.Trailer,
		Txs:    txs,
		Number: block.Number.Uint64(),
	}

	encodedBlock, err := sszBlock.MarshalSSZ()
	if err != nil {
		return nil, usedShortIDs, err
	}

	return bxmessage.NewBlockBroadcast(block.Hash(), block.BeaconHash(), block.Type, encodedBlock, usedShortIDs, networkNum), usedShortIDs, nil
}

func (bp *blockProcessor) markProcessed(hash types.SHA256Hash, status SeenStatus) {
	bp.processedBlocks.AddOrUpdate(hash.String(), status)
}
