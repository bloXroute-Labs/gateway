package eth

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
	ssz "github.com/prysmaticlabs/fastssz"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bdn"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/types"
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
	case *core.BlockInfo:
		return c.ethBlockBlockchainToBDN(b)
	case beacon.WrappedReadOnlySignedBeaconBlock:
		return c.beaconBlockBlockchainToBDN(b)
	default:
		return nil, fmt.Errorf("could not convert blockchain block type %v", b)
	}
}

func (c Converter) bscSidecarsBlockchainToBDN(block *bxcommoneth.Block) []*types.BxBSCBlobSidecar {
	bdnSidecars := make([]*types.BxBSCBlobSidecar, len(block.Sidecars()))
	for i, sidecar := range block.Sidecars() {
		bdnSidecars[i] = types.NewBxBSCBlobSidecar(sidecar.TxIndex, sidecar.TxHash, false, sidecar.BlobTxSidecar)
	}
	return bdnSidecars
}

func (c Converter) ethBlockBlockchainToBDN(blockInfo *core.BlockInfo) (*types.BxBlock, error) {
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

	var bxSidecars []*types.BxBSCBlobSidecar
	if len(block.Sidecars()) > 0 {
		bxSidecars = c.bscSidecarsBlockchainToBDN(block)
	}

	difficulty := blockInfo.TotalDifficulty()
	if difficulty == nil {
		difficulty = big.NewInt(0)
	}
	return types.NewBxBlock(hash, types.EmptyHash, types.BxBlockTypeEth, encodedHeader, txs, encodedTrailer, difficulty, block.Number(), int(block.Size()), bxSidecars)
}

func (c Converter) beaconBlockBlockchainToBDN(wrappedBlock beacon.WrappedReadOnlySignedBeaconBlock) (*types.BxBlock, error) {
	// Safe modification
	block, err := wrappedBlock.Block.Copy()
	if err != nil {
		return nil, fmt.Errorf("could not copy block: %v", err)
	}

	if block.Version() != version.Deneb && block.Version() != version.Electra && block.Version() != version.Electra && block.Version() != version.Fulu {
		return nil, fmt.Errorf("block version %v is not supported", block.Version())
	}

	b, err := bdn.PbGenericBlock(block)
	if err != nil {
		return nil, fmt.Errorf("could not get generic block: %v", err)
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

	beaconHash := NewSHA256Hash(rawHash)

	var concreteBlock ssz.Marshaler
	var bxBlockType types.BxBlockType
	var resp *blockInfo

	switch block.Version() {
	case version.Deneb:
		blockDeneb := b.GetDeneb()
		if blockDeneb == nil {
			return nil, bdn.ErrNotDenebBlock
		}

		b := blockDeneb.GetBlock()

		resp, err = parseExecutionPayload(b.GetBlock().GetBody().GetExecutionPayload())
		if err != nil {
			return nil, err
		}

		b.Block.Body.ExecutionPayload.Transactions = nil
		concreteBlock = b
		bxBlockType = types.BxBlockTypeBeaconDeneb
	case version.Electra:
		blockElectra := b.GetElectra()
		if blockElectra == nil {
			return nil, bdn.ErrNotElectraBlock
		}

		b := blockElectra.GetBlock()

		resp, err = parseExecutionPayload(b.GetBlock().GetBody().GetExecutionPayload())
		if err != nil {
			return nil, err
		}

		b.Block.Body.ExecutionPayload.Transactions = nil
		concreteBlock = b
		bxBlockType = types.BxBlockTypeBeaconElectra
	case version.Fulu:
		blockFulu := b.GetFulu()
		if blockFulu == nil {
			return nil, bdn.ErrNotFuluBlock
		}

		b := blockFulu.GetBlock()

		resp, err = parseExecutionPayload(b.GetBlock().GetBody().GetExecutionPayload())
		if err != nil {
			return nil, err
		}

		b.Block.Body.ExecutionPayload.Transactions = nil
		concreteBlock = b
		bxBlockType = types.BxBlockTypeBeaconFulu
	default:
		return nil, fmt.Errorf("unrecognized beacon block %v version %v", beaconHash, block.Version())
	}

	encodedBlock, err := concreteBlock.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("could not encode block %v body: %v", beaconHash, err)
	}

	return types.NewBxBlock(resp.hash, beaconHash, bxBlockType, nil, resp.txs, encodedBlock, nil, new(big.Int).SetUint64(resp.number), blockSize, nil)
}

type blockInfo struct {
	hash   types.SHA256Hash
	number uint64
	txs    []*types.BxBlockTransaction
}

func parseExecutionPayload(ep *v1.ExecutionPayloadDeneb) (*blockInfo, error) {
	resp := &blockInfo{}

	copy(resp.hash[:], ep.GetBlockHash())
	resp.number = ep.GetBlockNumber()

	for i, tx := range ep.GetTransactions() {
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
		resp.txs = append(resp.txs, compressedTx)
	}

	return resp, nil
}

// BlockBDNtoBlockchain converts a BDN block to an Ethereum block
func (c Converter) BlockBDNtoBlockchain(block *types.BxBlock) (interface{}, error) {
	switch block.Type {
	case types.BxBlockTypeEth:
		return c.ethBlockBDNtoBlockchain(block)
	case types.BxBlockTypeBeaconDeneb, types.BxBlockTypeBeaconElectra, types.BxBlockTypeBeaconFulu:
		return c.beaconBlockBDNtoBlockchain(block)
	default:
		return nil, fmt.Errorf("could not convert block %v block type %v", block.Hash(), block.Type)
	}
}

func (c Converter) bscSidecarsBDNtoBlockchain(block *types.BxBlock) ([]*bxcommoneth.BlobSidecar, error) {
	sidecars := make([]*bxcommoneth.BlobSidecar, len(block.BlobSidecars))
	for i, sidecar := range block.BlobSidecars {
		blockchainSidecar := &bxcommoneth.BlobSidecar{
			BlobTxSidecar: sidecar.TxSidecar,
			BlockNumber:   block.Number,
			BlockHash:     ethcommon.Hash(block.Hash()),
			TxIndex:       sidecar.TxIndex,
			TxHash:        sidecar.TxHash,
		}
		sidecars[i] = blockchainSidecar
	}

	return sidecars, nil
}

func (c Converter) ethBlockBDNtoBlockchain(block *types.BxBlock) (*core.BlockInfo, error) {
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
		return nil, fmt.Errorf("could not encode block %v data bxBlockRLP format: %v", block.Hash(), err)
	}

	var commonBlock bxcommoneth.Block
	if err = rlp.DecodeBytes(b, &commonBlock); err != nil {
		return nil, fmt.Errorf("could not convert block %v to blockchain format: %v", block.Hash(), err)
	}

	if len(block.BlobSidecars) > 0 {
		sidecars, err := c.bscSidecarsBDNtoBlockchain(block)
		if err != nil {
			return nil, err
		}
		commonBlock.SetBlobSidecars(sidecars)
	}

	return core.NewBlockInfo(&commonBlock, block.TotalDifficulty), nil
}

func (c Converter) beaconBlockBDNtoBlockchain(block *types.BxBlock) (interfaces.ReadOnlySignedBeaconBlock, error) {
	var blk interface{}
	switch block.Type {
	case types.BxBlockTypeBeaconDeneb:
		b := new(ethpb.SignedBeaconBlockDeneb)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format: %v", block.Hash(), err)
		}

		txs, err := c.extractTransactionsFromBlock(block)
		if err != nil {
			return nil, err
		}

		b.Block.Body.ExecutionPayload.Transactions = txs
		blk = b
	case types.BxBlockTypeBeaconElectra:
		b := new(ethpb.SignedBeaconBlockElectra)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format: %v", block.Hash(), err)
		}

		txs, err := c.extractTransactionsFromBlock(block)
		if err != nil {
			return nil, err
		}

		b.Block.Body.ExecutionPayload.Transactions = txs
		blk = b
	case types.BxBlockTypeBeaconFulu:
		b := new(ethpb.SignedBeaconBlockFulu)
		if err := b.UnmarshalSSZ(block.Trailer); err != nil {
			return nil, fmt.Errorf("could not convert block %v body to blockchain format: %v", block.Hash(), err)
		}

		txs, err := c.extractTransactionsFromBlock(block)
		if err != nil {
			return nil, err
		}

		b.Block.Body.ExecutionPayload.Transactions = txs
		blk = b
	default:
		return nil, fmt.Errorf("could not convert block %v to beacon block %v", block.Hash(), block.Type)
	}

	return blocks.NewSignedBeaconBlock(blk)
}

func (c Converter) extractTransactionsFromBlock(block *types.BxBlock) ([][]byte, error) {
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

	return txs, nil
}

// BeaconBlockToEthBlock converts beacon block to ETH block
func BeaconBlockToEthBlock(block interfaces.ReadOnlySignedBeaconBlock) (*bxcommoneth.Block, error) {
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

	withdrawals, err := execution.Withdrawals()
	if err != nil {
		return nil, fmt.Errorf("could not fetch withdrawals: %v", err)
	}

	ethWithdrawals := ethtypes.Withdrawals{}

	for _, withdrawal := range withdrawals {
		ethWithdrawals = append(ethWithdrawals, &ethtypes.Withdrawal{
			Index:     withdrawal.Index,
			Validator: uint64(withdrawal.ValidatorIndex),
			Address:   ethcommon.BytesToAddress(withdrawal.Address),
			Amount:    withdrawal.Amount,
		})
	}

	wsHash := ethtypes.DeriveSha(ethWithdrawals, trie.NewStackTrie(nil))
	withdrawalsHash := &wsHash

	executionBlobGasUsed, err := execution.BlobGasUsed()
	if err != nil {
		return nil, fmt.Errorf("could not fetch blob gas used: %v", err)
	}

	executionExcessBlobGas, err := execution.ExcessBlobGas()
	if err != nil {
		return nil, fmt.Errorf("could not fetch excess blob gas: %v", err)
	}

	blobGasUsed := &executionBlobGasUsed
	excessBlobGas := &executionExcessBlobGas

	var requestsHash *ethcommon.Hash
	if block.Version() >= version.Electra {
		requests, err := block.Block().Body().ExecutionRequests()
		if err != nil {
			return nil, fmt.Errorf("could not get requests: %v", err)
		}

		hash, err := requests.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("could not get requests hash: %v", err)
		}

		hashPtr := ethcommon.Hash(hash)
		requestsHash = &hashPtr
	}

	parentBeaconRootBytes := block.Block().ParentRoot()
	parentBeaconRoot := ethcommon.BytesToHash(parentBeaconRootBytes[:])

	header := &ethtypes.Header{
		ParentHash:       ethcommon.BytesToHash(execution.ParentHash()),
		UncleHash:        ethtypes.EmptyUncleHash,
		Coinbase:         ethcommon.BytesToAddress(execution.FeeRecipient()),
		Root:             ethcommon.BytesToHash(execution.StateRoot()),
		TxHash:           ethtypes.DeriveSha(ethtypes.Transactions(txs), trie.NewStackTrie(nil)),
		ReceiptHash:      ethcommon.BytesToHash(execution.ReceiptsRoot()),
		Bloom:            ethtypes.BytesToBloom(execution.LogsBloom()),
		Difficulty:       ethcommon.Big0,
		Number:           new(big.Int).SetUint64(execution.BlockNumber()),
		GasLimit:         execution.GasLimit(),
		GasUsed:          execution.GasUsed(),
		Time:             execution.Timestamp(),
		Extra:            execution.ExtraData(),
		MixDigest:        ethcommon.BytesToHash(execution.PrevRandao()),
		Nonce:            ethtypes.BlockNonce{},
		BaseFee:          new(big.Int).SetBytes(bytesutil.ReverseByteOrder(execution.BaseFeePerGas())),
		WithdrawalsHash:  withdrawalsHash,
		BlobGasUsed:      blobGasUsed,
		ExcessBlobGas:    excessBlobGas,
		ParentBeaconRoot: &parentBeaconRoot,
		RequestsHash:     requestsHash,
	}

	return bxcommoneth.NewBlockWithHeader(header).WithBody(ethtypes.Body{Transactions: txs, Uncles: nil, Withdrawals: ethWithdrawals}), nil
}

// BeaconMessageToBDN converts a beacon message to a BDN beacon message
func (c Converter) BeaconMessageToBDN(msg interface{}) (*types.BxBeaconMessage, error) {
	switch m := msg.(type) {
	case *ethpb.BlobSidecar:
		data, err := m.MarshalSSZ()
		if err != nil {
			return nil, fmt.Errorf("could not marshal blob sidecar: %v", err)
		}

		hash, err := m.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("could not get hash: %v", err)
		}

		blockHash, err := m.SignedBlockHeader.Header.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("could not get block hash: %v", err)
		}

		//nolint:gosec
		// G115: blob index and slot values will never exceed uint32 in practice
		return types.NewBxBeaconMessage(
			hash,
			NewSHA256Hash(blockHash),
			types.BxBeaconMessageTypeEthBlob,
			data,
			uint32(m.GetIndex()),
			uint32(m.GetSignedBlockHeader().GetHeader().GetSlot()),
		), nil
	case *ethpb.DataColumnSidecar:
		data, err := m.MarshalSSZ()
		if err != nil {
			return nil, fmt.Errorf("could not marshal blob sidecar: %v", err)
		}

		hash, err := m.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("could not get hash: %v", err)
		}

		blockHash, err := m.SignedBlockHeader.Header.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("could not get block hash: %v", err)
		}

		//nolint:gosec
		// G115: blob index and slot values will never exceed uint32 in practice
		return types.NewBxBeaconMessage(
			hash,
			NewSHA256Hash(blockHash),
			types.BxBeaconMessageTypeEthDataColumn,
			data,
			uint32(m.GetIndex()),
			uint32(m.GetSignedBlockHeader().GetHeader().GetSlot()),
		), nil
	default:
		return nil, fmt.Errorf("could not convert beacon message %v", m)
	}
}

// BeaconMessageBDNToBlockchain converts BDN beacon message to blockchain beacon message
func (c Converter) BeaconMessageBDNToBlockchain(msg *types.BxBeaconMessage) (interface{}, error) {
	switch msg.Type {
	case types.BxBeaconMessageTypeEthBlob:
		blob := new(ethpb.BlobSidecar)
		if err := blob.UnmarshalSSZ(msg.Data); err != nil {
			return nil, fmt.Errorf("could not unmarshal blob sidecar: %v", err)
		}

		return blob, nil
	case types.BxBeaconMessageTypeEthDataColumn:
		dataColumnSidecar := new(ethpb.DataColumnSidecar)
		if err := dataColumnSidecar.UnmarshalSSZ(msg.Data); err != nil {
			return nil, fmt.Errorf("could not unmarshal blob sidecar: %v", err)
		}

		return dataColumnSidecar, nil
	default:
		return nil, fmt.Errorf("could not convert beacon message %v", msg)
	}
}

// NewSHA256Hash is a utility function for converting between Ethereum common hashes and bloxroute hashes
func NewSHA256Hash(hash ethcommon.Hash) types.SHA256Hash {
	var sha256Hash types.SHA256Hash
	copy(sha256Hash[:], hash.Bytes())
	return sha256Hash
}
