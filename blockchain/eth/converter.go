package eth

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	"github.com/bloXroute-Labs/gateway/v2/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v4/runtime/version"
)

type bxBlockRLP struct {
	Header  rlp.RawValue
	Txs     []rlp.RawValue
	Trailer rlp.RawValue
}

// Converter is an Ethereum-BDN converter struct
type Converter struct{}

// TransactionBDNToBlockchain convert a BDN transaction to an Ethereum one
func (c Converter) TransactionBDNToBlockchain(transaction *types.BxTransaction) (interface{}, error) {
	return TransactionBDNToBlockchain(transaction)
}

// TransactionBlockchainToBDN converts an Ethereum transaction to a BDN transaction
func (c Converter) TransactionBlockchainToBDN(i interface{}) (*types.BxTransaction, error) {
	transaction := i.(*ethtypes.Transaction)
	hash := NewSHA256Hash(transaction.Hash())

	content, err := rlp.EncodeToBytes(transaction)
	if err != nil {
		return nil, err
	}

	return types.NewRawBxTransaction(hash, content), nil
}

// BlockBlockchainToBDN converts an Ethereum block to a BDN block
func (c Converter) BlockBlockchainToBDN(i interface{}) (*types.BxBlock, error) {
	switch b := i.(type) {
	case *BlockInfo:
		return c.ethBlockBlockchainToBDN(b)
	case beacon.WrappedReadOnlySignedBeaconBlock:
		return c.beaconBlockBlockchainToBDN(b)
	case *ethtypes.Block:
		return c.ethBlockBlockchainToBDN(NewBlockInfo(b, b.Difficulty()))
	default:
		return nil, fmt.Errorf("could not convert blockchain block type %v", b)
	}
}

func (c Converter) ethBlockBlockchainToBDN(blockInfo *BlockInfo) (*types.BxBlock, error) {
	block := blockInfo.Block
	hash := NewSHA256Hash(block.Hash())

	encodedHeader, err := rlp.EncodeToBytes(block.Header())
	if err != nil {
		return nil, fmt.Errorf("could not encode block header: %v: %v", block.Header(), err)
	}

	// Skip to put txs
	encodedTrailer, err := rlp.EncodeToBytes(block.Uncles())
	if err != nil {
		return nil, fmt.Errorf("could not encode block trailer: %v: %v", block.Uncles(), err)
	}

	var txs []*types.BxBlockTransaction
	for _, tx := range block.Transactions() {
		txBytes, err := rlp.EncodeToBytes(tx)
		if err != nil {
			return nil, fmt.Errorf("could not encode transaction %v", tx)
		}

		txHash := NewSHA256Hash(tx.Hash())
		compressedTx := types.NewBxBlockTransaction(txHash, txBytes)
		txs = append(txs, compressedTx)
	}

	difficulty := blockInfo.TotalDifficulty()
	if difficulty == nil {
		difficulty = big.NewInt(0)
	}
	return types.NewBxBlock(hash, types.EmptyHash, types.BxBlockTypeEth, encodedHeader, txs, encodedTrailer, difficulty, block.Number(), int(block.Size()))
}

func (c Converter) beaconBlockBlockchainToBDN(wrappedBlock beacon.WrappedReadOnlySignedBeaconBlock) (*types.BxBlock, error) {
	// Safe modification
	block, err := wrappedBlock.Block.Copy()
	if err != nil {
		return nil, fmt.Errorf("could not copy block: %v", err)
	}

	header, err := block.Header()
	if err != nil {
		return nil, fmt.Errorf("could not get header: %v", err)
	}

	rawHash, err := wrappedBlock.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get hash: %v: %v", header, err)
	}

	blockSize := block.SizeSSZ()

	// Bellatrix update sets hash to execution layer hash
	// Phase0 and Altair update passed by all networks so probably code should be removed
	var hash types.SHA256Hash
	beaconHash := NewSHA256Hash(rawHash)

	number := uint64(block.Block().Slot())

	var concreteBlock ssz.Marshaler
	var txs []*types.BxBlockTransaction
	var bxBlockType types.BxBlockType
	switch block.Version() {
	case version.Phase0:
		block, err := block.PbPhase0Block()
		if err != nil {
			return nil, fmt.Errorf("could not get phase 0 block %v: %v", beaconHash, err)
		}

		hash = beaconHash
		concreteBlock = block
		bxBlockType = types.BxBlockTypeBeaconPhase0
	case version.Altair:
		block, err := block.PbAltairBlock()
		if err != nil {
			return nil, fmt.Errorf("could not get altair block %v: %v", beaconHash, err)
		}

		hash = beaconHash
		concreteBlock = block
		bxBlockType = types.BxBlockTypeBeaconAltair
	case version.Bellatrix:
		block, err := block.PbBellatrixBlock()
		if err != nil {
			return nil, fmt.Errorf("could not get bellatrix block %v: %v", beaconHash, err)
		}

		copy(hash[:], block.GetBlock().GetBody().GetExecutionPayload().GetBlockHash())
		number = block.GetBlock().GetBody().GetExecutionPayload().GetBlockNumber()

		for i, tx := range block.GetBlock().GetBody().GetExecutionPayload().GetTransactions() {
			t := new(ethtypes.Transaction)
			if err := t.UnmarshalBinary(tx); err != nil {
				return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
			}

			// This is for back compatibility
			// Beacon block encodes transaction using MarshalBinary instead of rlp.EncodeBytes
			// For more info look at the comment of calcBeaconTransactionLength func
			txBytes, err := rlp.EncodeToBytes(t)
			if err != nil {
				return nil, fmt.Errorf("could not encode transaction %d: %v", i, err)
			}

			txHash := NewSHA256Hash(t.Hash())
			compressedTx := types.NewBxBlockTransaction(txHash, txBytes)
			txs = append(txs, compressedTx)
		}
		block.Block.Body.ExecutionPayload.Transactions = nil

		concreteBlock = block
		bxBlockType = types.BxBlockTypeBeaconBellatrix
	case version.Capella:
		b, err := block.PbCapellaBlock()
		if err != nil {
			return nil, fmt.Errorf("could not get capella block %v: %v", beaconHash, err)
		}

		copy(hash[:], b.GetBlock().GetBody().GetExecutionPayload().GetBlockHash())
		number = b.GetBlock().GetBody().GetExecutionPayload().GetBlockNumber()

		for i, tx := range b.GetBlock().GetBody().GetExecutionPayload().GetTransactions() {
			t := new(ethtypes.Transaction)
			if err := t.UnmarshalBinary(tx); err != nil {
				return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
			}

			// This is for back compatibility
			// Beacon block encodes transaction using MarshalBinary instead of rlp.EncodeBytes
			// For more info look at the comment of calcBeaconTransactionLength func
			txBytes, err := rlp.EncodeToBytes(t)
			if err != nil {
				return nil, fmt.Errorf("could not encode transaction %d: %v", i, err)
			}

			txHash := NewSHA256Hash(t.Hash())
			compressedTx := types.NewBxBlockTransaction(txHash, txBytes)
			txs = append(txs, compressedTx)
		}
		b.Block.Body.ExecutionPayload.Transactions = nil

		concreteBlock = b
		bxBlockType = types.BxBlockTypeBeaconCapella
	default:
		return nil, fmt.Errorf("unrecognized beacon block %v version %v", beaconHash, block.Version())
	}

	encodedBlock, err := concreteBlock.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("could not encode block %v body: %v", beaconHash, err)
	}

	return types.NewBxBlock(hash, beaconHash, bxBlockType, nil, txs, encodedBlock, nil, new(big.Int).SetUint64(number), blockSize)
}

// BlockBDNtoBlockchain converts a BDN block to an Ethereum block
func (c Converter) BlockBDNtoBlockchain(block *types.BxBlock) (interface{}, error) {
	switch block.Type {
	case types.BxBlockTypeEth:
		return c.ethBlockBDNtoBlockchain(block)
	case types.BxBlockTypeBeaconPhase0, types.BxBlockTypeBeaconAltair, types.BxBlockTypeBeaconBellatrix, types.BxBlockTypeBeaconCapella:
		return c.beaconBlockBDNtoBlockchain(block)
	default:
		return nil, fmt.Errorf("could not convert block %v block type %v", block.Hash(), block.Type)
	}
}

func (c Converter) ethBlockBDNtoBlockchain(block *types.BxBlock) (*BlockInfo, error) {
	txs := make([]rlp.RawValue, 0, len(block.Txs))
	for _, tx := range block.Txs {
		txs = append(txs, tx.Content())
	}

	b, err := rlp.EncodeToBytes(bxBlockRLP{
		Header:  block.Header,
		Txs:     txs,
		Trailer: block.Trailer,
	})
	if err != nil {
		return nil, fmt.Errorf("could not convert block %v to blockchain format: %v", block.Hash(), err)
	}

	var ethBlock ethtypes.Block
	if err = rlp.DecodeBytes(b, &ethBlock); err != nil {
		return nil, fmt.Errorf("could not convert block %v to blockchain format: %v", block.Hash(), err)
	}
	return NewBlockInfo(&ethBlock, block.TotalDifficulty), nil
}

func (c Converter) beaconBlockBDNtoBlockchain(block *types.BxBlock) (interfaces.ReadOnlySignedBeaconBlock, error) {
	var blk interface{}
	switch block.Type {
	case types.BxBlockTypeBeaconPhase0:
		b := new(ethpb.SignedBeaconBlock)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format:%v", block.Hash(), err)
		}
		blk = b
	case types.BxBlockTypeBeaconAltair:
		b := new(ethpb.SignedBeaconBlockAltair)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format:%v", block.Hash(), err)
		}
		blk = b
	case types.BxBlockTypeBeaconBellatrix:
		b := new(ethpb.SignedBeaconBlockBellatrix)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format:%v", block.Hash(), err)
		}

		txs := make([][]byte, 0, len(block.Txs))
		for i, tx := range block.Txs {
			t := new(ethtypes.Transaction)
			if err := rlp.DecodeBytes(tx.Content(), t); err != nil {
				return nil, fmt.Errorf("could not decode transaction %d: %v", i, err)
			}

			// This is for back compatibility
			// Beacon block encodes transaction using MarshalBinary instead of rlp.EncodeBytes
			// For more info look at the comment of calcBeaconTransactionLength func
			txBytes, err := t.MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("invalid transaction %d: %v", i, err)

			}

			txs = append(txs, txBytes)
		}

		b.Block.Body.ExecutionPayload.Transactions = txs
		blk = b
	case types.BxBlockTypeBeaconCapella:
		b := new(ethpb.SignedBeaconBlockCapella)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format: %v", block.Hash(), err)
		}

		txs := make([][]byte, 0, len(block.Txs))
		for i, tx := range block.Txs {
			t := new(ethtypes.Transaction)
			if err := rlp.DecodeBytes(tx.Content(), t); err != nil {
				return nil, fmt.Errorf("could not decode transaction %d: %v", i, err)
			}

			// This is for back compatibility
			// Beacon block encodes transaction using MarshalBinary instead of rlp.EncodeBytes
			// For more info look at the comment of calcBeaconTransactionLength func
			txBytes, err := t.MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("invalid transaction %d: %v", i, err)

			}

			txs = append(txs, txBytes)
		}

		b.Block.Body.ExecutionPayload.Transactions = txs
		blk = b
	default:
		return nil, fmt.Errorf("could not convert block %v to beacon block %v", block.Hash(), block.Type)
	}

	return blocks.NewSignedBeaconBlock(blk)
}

// BeaconBlockToEthBlock converts beacon block to ETH block
func BeaconBlockToEthBlock(block interfaces.ReadOnlySignedBeaconBlock) (*ethtypes.Block, error) {
	execution, err := block.Block().Body().Execution()
	if err != nil {
		return nil, fmt.Errorf("could not get payload: %v", err)
	}

	if execution == nil {
		return nil, errors.New("payload is empty")
	}

	transactions, err := execution.Transactions()
	if err != nil {
		return nil, fmt.Errorf("could not fetch transactions: %v", err)
	}
	txs := make([]*ethtypes.Transaction, len(transactions))
	for i, tx := range transactions {
		t := new(ethtypes.Transaction)
		if err := t.UnmarshalBinary(tx); err != nil {
			return nil, fmt.Errorf("invalid transaction %d: %v", i, err)
		}

		txs[i] = t
	}

	var withdrawalsHash *ethcommon.Hash
	ethWithdrawals := ethtypes.Withdrawals{}

	if block.Version() >= version.Capella {
		withdrawals, err := execution.Withdrawals()
		if err != nil {
			return nil, fmt.Errorf("could not fetch withdrawals: %v", err)
		}

		for _, withdrawal := range withdrawals {
			ethWithdrawals = append(ethWithdrawals, &ethtypes.Withdrawal{
				Index:     withdrawal.Index,
				Validator: uint64(withdrawal.ValidatorIndex),
				Address:   ethcommon.BytesToAddress(withdrawal.Address),
				Amount:    withdrawal.Amount,
			})
		}

		wsHash := ethtypes.DeriveSha(ethWithdrawals, trie.NewStackTrie(nil))
		withdrawalsHash = &wsHash
	}

	header := &ethtypes.Header{
		ParentHash:      ethcommon.BytesToHash(execution.ParentHash()),
		UncleHash:       ethtypes.EmptyUncleHash,
		Coinbase:        ethcommon.BytesToAddress(execution.FeeRecipient()),
		Root:            ethcommon.BytesToHash(execution.StateRoot()),
		TxHash:          ethtypes.DeriveSha(ethtypes.Transactions(txs), trie.NewStackTrie(nil)),
		ReceiptHash:     ethcommon.BytesToHash(execution.ReceiptsRoot()),
		Bloom:           ethtypes.BytesToBloom(execution.LogsBloom()),
		Difficulty:      ethcommon.Big0,
		Number:          new(big.Int).SetUint64(execution.BlockNumber()),
		GasLimit:        execution.GasLimit(),
		GasUsed:         execution.GasUsed(),
		Time:            execution.Timestamp(),
		BaseFee:         new(big.Int).SetBytes(bytesutil.ReverseByteOrder(execution.BaseFeePerGas())),
		Extra:           execution.ExtraData(),
		MixDigest:       ethcommon.BytesToHash(execution.PrevRandao()),
		WithdrawalsHash: withdrawalsHash,
	}

	return ethtypes.NewBlockWithHeader(header).WithBody(txs, nil /* uncles */).WithWithdrawals(ethWithdrawals), nil
}
