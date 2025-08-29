package types

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bdn"
	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
)

// NewBeaconBlockNotification creates beacon block notification
func NewBeaconBlockNotification(block interfaces.ReadOnlySignedBeaconBlock) (BlockNotification, error) {
	genericBlock, err := bdn.PbGenericBlock(block)
	if err != nil {
		return nil, err
	}

	switch block.Version() {
	case version.Deneb:
		blk := genericBlock.GetDeneb().GetBlock()

		hash, err := blk.GetBlock().HashTreeRoot()
		if err != nil {
			return nil, err
		}

		return &DenebBlockNotification{
			Hash:                   hex.EncodeToString(hash[:]),
			SignedBeaconBlockDeneb: blk,
		}, nil
	case version.Electra:
		blk := genericBlock.GetElectra().GetBlock()

		hash, err := blk.GetBlock().HashTreeRoot()
		if err != nil {
			return nil, err
		}

		return &ElectraBlockNotification{
			Hash:                     hex.EncodeToString(hash[:]),
			SignedBeaconBlockElectra: blk,
		}, nil

	default:
		return nil, fmt.Errorf("not supported %s", version.String(block.Version()))
	}
}

// ElectraBlockNotification represents electra beacon block notification
type ElectraBlockNotification struct {
	*ethpb.SignedBeaconBlockElectra

	Hash string `json:"hash"`

	notificationType FeedType
	source           *NodeEndpoint
}

// WithFields returns notification with specified fields
func (e *ElectraBlockNotification) WithFields(fields []string) Notification {
	block := ElectraBlockNotification{}
	for _, param := range fields {
		switch param {
		case "hash":
			block.Hash = e.Hash
		case "header":
			if block.SignedBeaconBlockElectra == nil {
				block.SignedBeaconBlockElectra = &ethpb.SignedBeaconBlockElectra{}
			}

			if block.Block == nil {
				block.Block = &ethpb.BeaconBlockElectra{}
			}

			block.Block.Slot = e.GetBlock().GetSlot()
			block.Block.ProposerIndex = e.GetBlock().GetProposerIndex()
			block.Block.ParentRoot = e.GetBlock().GetParentRoot()
			block.Block.StateRoot = e.GetBlock().GetStateRoot()
		case "slot":
			if block.SignedBeaconBlockElectra == nil {
				block.SignedBeaconBlockElectra = &ethpb.SignedBeaconBlockElectra{}
			}

			if block.SignedBeaconBlockElectra.Block == nil {
				block.Block = &ethpb.BeaconBlockElectra{}
			}

			block.Block.Slot = e.GetBlock().GetSlot()
		case "body":
			if block.SignedBeaconBlockElectra == nil {
				block.SignedBeaconBlockElectra = &ethpb.SignedBeaconBlockElectra{}
			}

			if block.SignedBeaconBlockElectra.Block == nil {
				block.Block = &ethpb.BeaconBlockElectra{}
			}

			block.Block.Body = e.GetBlock().GetBody()
		}
	}

	return &block
}

// Filters -
func (e *ElectraBlockNotification) Filters() map[string]interface{} {
	return nil
}

// LocalRegion -
func (e *ElectraBlockNotification) LocalRegion() bool {
	return false
}

// GetHash returns block hash
func (e *ElectraBlockNotification) GetHash() string {
	return e.Hash
}

// NotificationType returns feed name
func (e *ElectraBlockNotification) NotificationType() FeedType {
	return e.notificationType
}

// SetNotificationType sets feed name
func (e *ElectraBlockNotification) SetNotificationType(feedType FeedType) {
	e.notificationType = feedType
}

// SetSource sets source endpoint
func (e *ElectraBlockNotification) SetSource(endpoint *NodeEndpoint) {
	e.source = endpoint
}

// IsNil returns true if nil
func (e *ElectraBlockNotification) IsNil() bool {
	return e == nil
}

// Clone clones notification
func (e *ElectraBlockNotification) Clone() BlockNotification {
	n := *e
	return &n
}

// DenebBlockNotification represents deneb beacon block notification
type DenebBlockNotification struct {
	*ethpb.SignedBeaconBlockDeneb

	Hash string `json:"hash"`

	notificationType FeedType
	source           *NodeEndpoint
}

// WithFields returns notification with specified fields
func (beaconBlockNotification *DenebBlockNotification) WithFields(fields []string) Notification {
	block := DenebBlockNotification{}
	for _, param := range fields {
		switch param {
		case "hash":
			block.Hash = beaconBlockNotification.Hash
		case "header":
			if block.SignedBeaconBlockDeneb == nil {
				block.SignedBeaconBlockDeneb = &ethpb.SignedBeaconBlockDeneb{}
			}

			if block.Block == nil {
				block.Block = &ethpb.BeaconBlockDeneb{}
			}

			block.Block.Slot = beaconBlockNotification.GetBlock().GetSlot()
			block.Block.ProposerIndex = beaconBlockNotification.GetBlock().GetProposerIndex()
			block.Block.ParentRoot = beaconBlockNotification.GetBlock().GetParentRoot()
			block.Block.StateRoot = beaconBlockNotification.GetBlock().GetStateRoot()
		case "slot":
			if block.SignedBeaconBlockDeneb == nil {
				block.SignedBeaconBlockDeneb = &ethpb.SignedBeaconBlockDeneb{}
			}

			if block.SignedBeaconBlockDeneb.Block == nil {
				block.Block = &ethpb.BeaconBlockDeneb{}
			}

			block.Block.Slot = beaconBlockNotification.GetBlock().GetSlot()
		case "body":
			if block.SignedBeaconBlockDeneb == nil {
				block.SignedBeaconBlockDeneb = &ethpb.SignedBeaconBlockDeneb{}
			}

			if block.SignedBeaconBlockDeneb.Block == nil {
				block.Block = &ethpb.BeaconBlockDeneb{}
			}

			block.Block.Body = beaconBlockNotification.GetBlock().GetBody()
		}
	}

	return &block
}

// Filters converts filters as field value map
func (beaconBlockNotification *DenebBlockNotification) Filters() map[string]interface{} {
	return nil
}

// LocalRegion -
func (beaconBlockNotification *DenebBlockNotification) LocalRegion() bool {
	return false
}

// GetHash returns block hash
func (beaconBlockNotification *DenebBlockNotification) GetHash() string {
	return beaconBlockNotification.Hash
}

// SetNotificationType - set feed name
func (beaconBlockNotification *DenebBlockNotification) SetNotificationType(feedName FeedType) {
	beaconBlockNotification.notificationType = feedName
}

// NotificationType - feed name
func (beaconBlockNotification *DenebBlockNotification) NotificationType() FeedType {
	return beaconBlockNotification.notificationType
}

// SetSource - source blockchain node endpoint
func (beaconBlockNotification *DenebBlockNotification) SetSource(source *NodeEndpoint) {
	beaconBlockNotification.source = source
}

// Source - source blockchain node endpoint
func (beaconBlockNotification *DenebBlockNotification) Source() *NodeEndpoint {
	return beaconBlockNotification.source
}

// IsNil returns true if nil
func (beaconBlockNotification *DenebBlockNotification) IsNil() bool {
	return beaconBlockNotification == nil
}

// Clone clones notification
func (beaconBlockNotification *DenebBlockNotification) Clone() BlockNotification {
	n := *beaconBlockNotification
	return &n
}

// EthBlockNotification - represents a single block
type EthBlockNotification struct {
	BlockHash        *ethcommon.Hash `json:"hash,omitempty"`
	block            *bxethcommon.Block
	Header           *Header                  `json:"header,omitempty"`
	Transactions     []map[string]interface{} `json:"transactions,omitempty"`
	Uncles           []Header                 `json:"uncles,omitempty"`
	ValidatorInfo    []*FutureValidatorInfo   `json:"future_validator_info,omitempty"`
	Withdrawals      ethtypes.Withdrawals     `json:"withdrawals,omitempty"`
	RawTransactions  [][]byte                 `json:"raw_transactions,omitempty"`
	notificationType FeedType
	source           *NodeEndpoint
	rawTxsMu         *sync.RWMutex
	txsMu            *sync.RWMutex
}

// NewEthBlockNotification creates ETH block notification
func NewEthBlockNotification(hash ethcommon.Hash, block *bxethcommon.Block, info []*FutureValidatorInfo) (*EthBlockNotification, error) {
	if hash == (ethcommon.Hash{}) {
		return nil, errors.New("empty block hash")
	}
	ethUncles := make([]Header, 0, len(block.Uncles()))
	for _, uncle := range block.Uncles() {
		ethUncle := ConvertEthHeaderToBlockNotificationHeader(uncle)
		ethUncles = append(ethUncles, *ethUncle)
	}
	return &EthBlockNotification{
		BlockHash:     &hash,
		block:         block,
		Header:        ConvertEthHeaderToBlockNotificationHeader(block.Header()),
		Uncles:        ethUncles,
		ValidatorInfo: info,
		Withdrawals:   block.Withdrawals(),
		// to parse raw transactions and transactions separately, we need to lock the mutexes
		rawTxsMu: &sync.RWMutex{},
		txsMu:    &sync.RWMutex{},
	}, nil
}

// GetTransactions returns a shallow copy of the transactions slice
func (ethBlockNotification *EthBlockNotification) GetTransactions() []map[string]interface{} {
	return ethBlockNotification.parseTransactions()
}

func (ethBlockNotification *EthBlockNotification) parseRawTransactions() [][]byte {
	ethBlockNotification.rawTxsMu.Lock()
	defer ethBlockNotification.rawTxsMu.Unlock()
	if ethBlockNotification.RawTransactions != nil {
		return ethBlockNotification.RawTransactions
	}

	rawTransactions := make([][]byte, 0)
	if ethBlockNotification.block != nil {
		for _, tx := range ethBlockNotification.block.Transactions() {
			rawTx, err := tx.MarshalBinary()
			if err != nil {
				log.Errorf("failed to marshal transaction: %v", err)
				return nil
			}
			rawTransactions = append(rawTransactions, rawTx)
		}

	} else {
		log.Errorf("block is nil, cannot parse raw transactions for block hash: %s", ethBlockNotification.GetHash())
	}

	ethBlockNotification.RawTransactions = rawTransactions
	return rawTransactions
}

// GetRawTransactions returns raw transactions
func (ethBlockNotification *EthBlockNotification) GetRawTransactions() [][]byte {
	ethBlockNotification.rawTxsMu.RLock()
	defer ethBlockNotification.rawTxsMu.RUnlock()
	return ethBlockNotification.RawTransactions
}

// GetParsedTransactions returns parsed transactions
func (ethBlockNotification *EthBlockNotification) GetParsedTransactions() []map[string]interface{} {
	ethBlockNotification.txsMu.RLock()
	defer ethBlockNotification.txsMu.RUnlock()
	return ethBlockNotification.Transactions
}

func (ethBlockNotification *EthBlockNotification) parseTransactions() []map[string]interface{} {
	ethBlockNotification.txsMu.Lock()
	defer ethBlockNotification.txsMu.Unlock()
	if ethBlockNotification.Transactions != nil {
		return ethBlockNotification.Transactions
	}

	if ethBlockNotification.block != nil {
		ethTxs := make([]map[string]interface{}, 0)

		for _, tx := range ethBlockNotification.block.Transactions() {
			txFields, err := parseFieldsFromTx(tx)
			if err != nil {
				log.Errorf("failed to parse fields from txs: %v", err)
				return nil
			}
			ethTxs = append(ethTxs, txFields)
		}

		ethBlockNotification.Transactions = ethTxs
		return ethTxs
	}

	ethTxsFromRaw, err := ethBlockNotification.parseTransactionsFromRaw()
	if err != nil {
		log.Errorf("failed to parse transactions from raw: %v", err)
		return nil
	}

	ethBlockNotification.Transactions = ethTxsFromRaw
	return ethTxsFromRaw
}

func (ethBlockNotification *EthBlockNotification) parseTransactionsFromRaw() ([]map[string]any, error) {
	rawTxs := ethBlockNotification.GetRawTransactions()
	ethTxs := make([]map[string]any, 0)

	for _, rawTx := range rawTxs {
		var tx ethtypes.Transaction
		err := tx.UnmarshalBinary(rawTx)
		if err != nil {
			return nil, err
		}

		ethTxFields, err := parseFieldsFromTx(&tx)
		if err != nil {
			return nil, err
		}
		ethTxs = append(ethTxs, ethTxFields)
	}

	return ethTxs, nil
}

func parseFieldsFromTx(tx *ethtypes.Transaction) (map[string]any, error) {
	ethTx, err := NewEthTransaction(tx, EmptySender)
	if err != nil {
		return nil, err
	}
	fields := ethTx.Fields(AllFieldsWithFrom)
	if ethTx.Type() >= ethtypes.DynamicFeeTxType {
		fields["gasPrice"] = fields["maxFeePerGas"]
	}

	return fields, nil
}

// FutureValidatorInfo - represents information about the validator information of the second block after the current block
type FutureValidatorInfo struct {
	BlockHeight uint64 `json:"block_height"`
	WalletID    string `json:"wallet_id"`
	Accessible  bool   `json:"accessible"`
}

// Header - represents Ethereum block header
type Header struct {
	ParentHash       ethcommon.Hash     `json:"parentHash"`
	Sha3Uncles       ethcommon.Hash     `json:"sha3Uncles"`
	Miner            *ethcommon.Address `json:"miner"`
	StateRoot        ethcommon.Hash     `json:"stateRoot"`
	TransactionsRoot ethcommon.Hash     `json:"transactionsRoot"`
	ReceiptsRoot     ethcommon.Hash     `json:"receiptsRoot"`
	LogsBloom        string             `json:"logsBloom"`
	Difficulty       string             `json:"difficulty"`
	Number           string             `json:"number"`
	GasLimit         string             `json:"gasLimit"`
	GasUsed          string             `json:"gasUsed"`
	Timestamp        string             `json:"timestamp"`
	ExtraData        string             `json:"extraData"`
	MixHash          ethcommon.Hash     `json:"mixHash"`
	Nonce            string             `json:"nonce"`
	BaseFee          *int               `json:"baseFeePerGas,omitempty"`
	WithdrawalsHash  *ethcommon.Hash    `json:"withdrawalsRoot,omitempty"`
	BlobGasUsed      string             `json:"blobGasUsed,omitempty"`
	ExcessBlobGas    string             `json:"excessBlobGas,omitempty"`
	ParentBeaconRoot *ethcommon.Hash    `json:"parentBeaconBlockRoot,omitempty"`
	RequestsHash     *ethcommon.Hash    `json:"requestsHash,omitempty"`
	hexNumber        uint64
}

// GetNumber returns the block number from the header in uint64
func (h *Header) GetNumber() uint64 {
	return h.hexNumber
}

// UpdateNumber updates the block number from the header in uint64
func (h *Header) UpdateNumber(number uint64) {
	h.hexNumber = number
}

// ConvertEthHeaderToBlockNotificationHeader converts Ethereum header to bloxroute Ethereum Header
func ConvertEthHeaderToBlockNotificationHeader(ethHeader *ethtypes.Header) *Header {

	var blobGasUsed, excessBlobGas string
	var parentBeaconRoot *ethcommon.Hash
	if ethHeader.BlobGasUsed != nil {
		blobGasUsed = hexutil.EncodeUint64(*ethHeader.BlobGasUsed)
	}
	if ethHeader.ExcessBlobGas != nil {
		excessBlobGas = hexutil.EncodeUint64(*ethHeader.ExcessBlobGas)
	}
	if ethHeader.ParentBeaconRoot != nil {
		parentBeaconRoot = ethHeader.ParentBeaconRoot
	}

	newHeader := Header{
		ParentHash:       ethHeader.ParentHash,
		Sha3Uncles:       ethHeader.UncleHash,
		Miner:            &ethHeader.Coinbase,
		StateRoot:        ethHeader.Root,
		TransactionsRoot: ethHeader.TxHash,
		ReceiptsRoot:     ethHeader.ReceiptHash,
		LogsBloom:        fmt.Sprintf("0x%v", hex.EncodeToString(ethHeader.Bloom.Bytes())),
		Difficulty:       hexutil.EncodeBig(ethHeader.Difficulty),
		hexNumber:        ethHeader.Number.Uint64(),
		Number:           hexutil.EncodeBig(ethHeader.Number),
		GasLimit:         hexutil.EncodeUint64(ethHeader.GasLimit),
		GasUsed:          hexutil.EncodeUint64(ethHeader.GasUsed),
		Timestamp:        hexutil.EncodeUint64(ethHeader.Time),
		ExtraData:        hexutil.Encode(ethHeader.Extra),
		MixHash:          ethHeader.MixDigest,
		Nonce:            fmt.Sprintf("0x%016s", hexutil.EncodeUint64(ethHeader.Nonce.Uint64())[2:]),
		WithdrawalsHash:  ethHeader.WithdrawalsHash,
		BlobGasUsed:      blobGasUsed,
		ExcessBlobGas:    excessBlobGas,
		ParentBeaconRoot: parentBeaconRoot,
		RequestsHash:     ethHeader.RequestsHash,
	}
	if ethHeader.BaseFee != nil {
		baseFee := int(ethHeader.BaseFee.Int64())
		newHeader.BaseFee = &baseFee
	}
	return &newHeader
}

// WithFields returns notification with specified fields
func (ethBlockNotification *EthBlockNotification) WithFields(fields []string) Notification {
	block := EthBlockNotification{txsMu: ethBlockNotification.txsMu, rawTxsMu: ethBlockNotification.rawTxsMu, block: ethBlockNotification.block}

	for _, param := range fields {
		switch param {
		case "hash":
			block.BlockHash = ethBlockNotification.BlockHash
		case "header":
			block.Header = ethBlockNotification.Header
		case "transactions":
			block.Transactions = ethBlockNotification.parseTransactions()
		case "raw_transactions":
			block.RawTransactions = ethBlockNotification.parseRawTransactions()
		case "uncles":
			block.Uncles = ethBlockNotification.Uncles
		case "future_validator_info":
			block.ValidatorInfo = ethBlockNotification.ValidatorInfo
		case "withdrawals":
			block.Withdrawals = ethBlockNotification.Withdrawals
		}
	}
	return &block
}

// Filters converts filters as field value map
func (ethBlockNotification *EthBlockNotification) Filters() map[string]interface{} {
	return nil
}

// LocalRegion -
func (ethBlockNotification *EthBlockNotification) LocalRegion() bool {
	return false
}

// GetHash returns block hash
func (ethBlockNotification *EthBlockNotification) GetHash() string {
	return ethBlockNotification.BlockHash.Hex()
}

// SetNotificationType - set feed name
func (ethBlockNotification *EthBlockNotification) SetNotificationType(feedName FeedType) {
	ethBlockNotification.notificationType = feedName
}

// NotificationType - feed name
func (ethBlockNotification *EthBlockNotification) NotificationType() FeedType {
	return ethBlockNotification.notificationType
}

// SetSource - source blockchain node endpoint
func (ethBlockNotification *EthBlockNotification) SetSource(source *NodeEndpoint) {
	ethBlockNotification.source = source
}

// Source - source blockchain node endpoint
func (ethBlockNotification *EthBlockNotification) Source() *NodeEndpoint {
	return ethBlockNotification.source
}

// IsNil return true if nil
func (ethBlockNotification *EthBlockNotification) IsNil() bool {
	return ethBlockNotification == nil
}

// Clone clones notification
func (ethBlockNotification *EthBlockNotification) Clone() BlockNotification {
	n := *ethBlockNotification
	return &n
}

// GetRawTxByIndex return rawTransaction data by given index
func (ethBlockNotification *EthBlockNotification) GetRawTxByIndex(index int) []byte {
	raws := ethBlockNotification.parseRawTransactions()
	if index < 0 || index >= len(raws) {
		log.Errorf("failed to find raw transaction by index: %d, block hash %s", index, ethBlockNotification.GetHash())
		return []byte{}
	}
	return raws[index]
}

// SetLocks sets mutex locks for transactions and raw transactions
func (ethBlockNotification *EthBlockNotification) SetLocks() {
	ethBlockNotification.rawTxsMu = &sync.RWMutex{}
	ethBlockNotification.txsMu = &sync.RWMutex{}
}
