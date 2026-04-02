package types

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// RawBlock represents a raw block from the blockchain.
type RawBlock struct {
	hash            common.Hash
	parentHash      common.Hash
	number          uint64
	txs             []*BxBlockTransaction
	time            uint64
	header          []byte
	trailer         []byte
	sidecars        []*BxBSCBlobSidecar
	totalDifficulty *big.Int
}

// Hash returns the hash of the block.
func (b *RawBlock) Hash() common.Hash {
	return b.hash
}

// ParentHash returns the hash of the parent block.
func (b *RawBlock) ParentHash() common.Hash {
	return b.parentHash
}

// Number returns the number of the block.
func (b *RawBlock) Number() uint64 {
	return b.number
}

// Transactions returns the transactions of the block.
func (b *RawBlock) Transactions() []*BxBlockTransaction {
	return b.txs
}

// Time returns the timestamp of the block.
func (b *RawBlock) Time() uint64 {
	return b.time
}

// TotalDifficulty returns the total difficulty of the block.
func (b *RawBlock) TotalDifficulty() *big.Int {
	return b.totalDifficulty
}

// Block returns the block.
func (b *RawBlock) Block() *bxethcommon.Block {
	if b == nil {
		return nil
	}

	var header types.Header
	if err := rlp.DecodeBytes(b.header, &header); err != nil {
		return nil
	}

	block := bxethcommon.NewBlockWithHeader(&header)

	transactions := make([]*types.Transaction, 0, len(b.txs))
	for _, tx := range b.txs {
		if tx == nil {
			continue
		}

		var decoded types.Transaction
		if err := rlp.DecodeBytes(tx.Content(), &decoded); err != nil {
			log.Errorf("failed to decode transaction %v: %v", tx.Hash(), err)
			continue
		}
		transactions = append(transactions, &decoded)
	}

	var uncles []*types.Header
	if len(b.trailer) == 0 || bytes.Equal(b.trailer, rlp.EmptyList) {
		uncles = []*types.Header{}
	} else {
		var decodedUncles []*types.Header
		if err := rlp.DecodeBytes(b.trailer, &decodedUncles); err == nil {
			uncles = decodedUncles
		} else {
			uncles = []*types.Header{}
		}
	}

	body := types.Body{
		Transactions: transactions,
		Uncles:       uncles,
	}
	block = block.WithBody(body)

	var sidecars bxethcommon.BlobSidecars
	for _, sidecar := range b.sidecars {
		if sidecar == nil {
			continue
		}

		// capture TxSidecar in a local variable to avoid race condition with
		// processBlobSidecarToRLPBroadcast which may set TxSidecar to nil concurrently
		txSidecar := sidecar.TxSidecar
		if txSidecar == nil {
			continue
		}

		sidecars = append(sidecars, &bxethcommon.BlobSidecar{
			BlobTxSidecar: &bxethcommon.BlobTxSidecar{
				Blobs:       txSidecar.Blobs,
				Commitments: txSidecar.Commitments,
				Proofs:      txSidecar.Proofs,
			},
			BlockNumber: new(big.Int).SetUint64(b.number),
			BlockHash:   b.hash,
			TxIndex:     sidecar.TxIndex,
			TxHash:      sidecar.TxHash,
		})
	}
	if len(sidecars) > 0 {
		block = block.WithSidecars(sidecars)
	}

	return block
}

// BxBlock returns the BxBlock.
func (b *RawBlock) BxBlock() (*BxBlock, error) {
	hash, err := NewSHA256Hash(b.hash.Bytes())
	if err != nil {
		return nil, err
	}

	totalDifficulty := b.totalDifficulty
	if totalDifficulty == nil {
		totalDifficulty = big.NewInt(0)
	}

	trailer := b.trailer
	if trailer == nil {
		trailer = rlp.EmptyList
	}

	return NewRawBxBlock(
		hash,
		EmptyHash,
		BxBlockTypeEth,
		b.header,
		b.txs,
		trailer,
		totalDifficulty,
		new(big.Int).SetUint64(b.number),
		0,
		b.sidecars,
	), nil
}

// ParseNewBlockPacket parses a new block packet.
func ParseNewBlockPacket(r io.Reader) (*RawBlock, error) {
	stream := rlp.NewStream(r, 0)

	if _, err := stream.List(); err != nil { // packet
		return nil, err
	}

	if _, err := stream.List(); err != nil { // block
		return nil, err
	}

	headerRaw, err := stream.Raw()
	if err != nil {
		return nil, err
	}

	var blockHash common.Hash
	sha := crypto.NewKeccakState()
	sha.Reset()
	sha.Write(headerRaw)
	_, err = sha.Read(blockHash[:])
	if err != nil {
		return nil, err
	}

	headerStream := rlp.NewStream(bytes.NewBuffer(headerRaw), uint64(len(headerRaw)))
	if _, err := headerStream.List(); err != nil {
		return nil, err
	}

	parentHash, err := headerStream.Bytes()
	if err != nil {
		return nil, err
	}

	if _, err := headerStream.Raw(); err != nil { // uncle hash
		return nil, err
	}
	if _, err := headerStream.Raw(); err != nil { // coinbase
		return nil, err
	}
	if _, err := headerStream.Raw(); err != nil { // state root
		return nil, err
	}
	if _, err := headerStream.Raw(); err != nil { // tx hash
		return nil, err
	}
	if _, err := headerStream.Raw(); err != nil { // receipt hash
		return nil, err
	}
	if _, err := headerStream.Raw(); err != nil { // bloom
		return nil, err
	}
	if _, err := headerStream.Raw(); err != nil { // difficulty
		return nil, err
	}

	number, err := headerStream.Uint64()
	if err != nil {
		return nil, err
	}

	if _, err := headerStream.Uint64(); err != nil { // gas limit
		return nil, err
	}
	if _, err := headerStream.Uint64(); err != nil { // gas used
		return nil, err
	}

	timestamp, err := headerStream.Uint64()
	if err != nil {
		return nil, err
	}

	for headerStream.MoreDataInList() {
		if _, err := headerStream.Raw(); err != nil {
			return nil, err
		}
	}

	if err := headerStream.ListEnd(); err != nil {
		return nil, fmt.Errorf("failed to parse block header list: %w", err)
	}

	if _, err := stream.List(); err != nil { // tx list
		return nil, err
	}

	var txs []*BxBlockTransaction
	for stream.MoreDataInList() {
		var txHash common.Hash

		kind, _, err := stream.Kind()
		if err != nil {
			return nil, err
		}

		var txRawEncoded []byte
		var txRawForHash []byte
		switch kind {
		case rlp.String:
			txRawEncoded, err = stream.Raw()
			if err != nil {
				return nil, err
			}
			txRawForHash, _, err = rlp.SplitString(txRawEncoded)
			if err != nil {
				return nil, err
			}
		case rlp.List:
			txRawEncoded, err = stream.Raw()
			if err != nil {
				return nil, err
			}
			txRawForHash = txRawEncoded
		default:
			return nil, rlp.ErrExpectedList
		}

		sha := crypto.NewKeccakState()
		sha.Reset()
		sha.Write(txRawForHash)
		_, err = sha.Read(txHash[:])
		if err != nil {
			return nil, err
		}

		tHash, err := NewSHA256Hash(txHash[:])
		if err != nil {
			return nil, err
		}
		txs = append(txs, NewBxBlockTransaction(tHash, txRawEncoded))
	}
	if err := stream.ListEnd(); err != nil { // end tx list
		return nil, fmt.Errorf("failed to parse transactions list: %w", err)
	}

	if _, err := stream.Raw(); err != nil { // uncles list
		return nil, err
	}

	if stream.MoreDataInList() { // withdrawals list (optional)
		if _, err := stream.Raw(); err != nil {
			return nil, err
		}
	}

	if err := stream.ListEnd(); err != nil { // end block list
		return nil, fmt.Errorf("failed to parse block list: %w", err)
	}

	tdRaw, err := stream.Raw() // TD
	if err != nil {
		return nil, err
	}

	// TODO: consider raw decode
	var totalDifficulty big.Int
	if err := rlp.DecodeBytes(tdRaw, &totalDifficulty); err != nil {
		return nil, err
	}

	var sidecars []*BxBSCBlobSidecar
	if stream.MoreDataInList() {
		rawSidecars, err := stream.Raw()
		if err != nil {
			return nil, err
		}

		var blobSidecars bxethcommon.BlobSidecars
		if err := rlp.DecodeBytes(rawSidecars, &blobSidecars); err != nil {
			return nil, err
		}

		sidecars = make([]*BxBSCBlobSidecar, 0, len(blobSidecars))
		for _, sidecar := range blobSidecars {
			if sidecar == nil {
				continue
			}
			sidecars = append(sidecars, NewBxBSCBlobSidecar(sidecar.TxIndex, sidecar.TxHash, false, sidecar.BlobTxSidecar))
		}
	}

	if err := stream.ListEnd(); err != nil { // end packet list
		return nil, fmt.Errorf("failed to parse packet list: %w", err)
	}
	return &RawBlock{
		hash:            blockHash,
		parentHash:      common.Hash(parentHash),
		number:          number,
		txs:             txs,
		time:            timestamp,
		header:          headerRaw,
		trailer:         rlp.EmptyList,
		sidecars:        sidecars,
		totalDifficulty: &totalDifficulty,
	}, nil
}
