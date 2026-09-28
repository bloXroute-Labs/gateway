package types

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	log "github.com/bloXroute-Labs/bxcommon-go/v2/logger"
)

// ErrEmptyTransaction is returned when the tx is not set
var ErrEmptyTransaction = fmt.Errorf("empty transaction")

var paramToName = map[string]string{
	"tx_hash":                  "hash",
	"access_list":              "accessList",
	"chain_id":                 "chainId",
	"gas_price":                "gasPrice",
	"max_fee_per_gas":          "maxFeePerGas",
	"max_priority_fee_per_gas": "maxPriorityFeePerGas",
	"max_fee_per_blob_gas":     "maxFeePerBlobGas",
	"blob_versioned_hashes":    "blobVersionedHashes",

	"tx_contents.tx_hash":                  "hash",
	"tx_contents.nonce":                    "nonce",
	"tx_contents.input":                    "input",
	"tx_contents.v":                        "v",
	"tx_contents.r":                        "r",
	"tx_contents.s":                        "s",
	"tx_contents.access_list":              "accessList",
	"tx_contents.chain_id":                 "chainId",
	"tx_contents.max_fee_per_gas":          "maxFeePerGas",
	"tx_contents.max_priority_fee_per_gas": "maxPriorityFeePerGas",
	"tx_contents.gas_price":                "gasPrice",
	"tx_contents.type":                     "type",
	"tx_contents.value":                    "value",
	"tx_contents.gas":                      "gas",
	"tx_contents.to":                       "to",
	"tx_contents.from":                     "from",
	"tx_contents.max_fee_per_blob_gas":     "maxFeePerBlobGas",
	"tx_contents.blob_versioned_hashes":    "blobVersionedHashes",
	"tx_contents.y_parity":                 "yParity",
	"tx_contents.authorization_list":       "authorizationList",
}

// AllFields is used with blocks feeds
var AllFields = []string{
	"tx_contents.tx_hash", "tx_contents.nonce", "tx_contents.input", "tx_contents.v", "tx_contents.r",
	"tx_contents.s", "tx_contents.access_list", "tx_contents.chain_id", "tx_contents.max_fee_per_gas", "tx_contents.max_priority_fee_per_gas",
	"tx_contents.gas_price", "tx_contents.type", "tx_contents.value", "tx_contents.gas", "tx_contents.to", "tx_contents.max_fee_per_blob_gas",
	"tx_contents.blob_versioned_hashes", "tx_contents.y_parity", "tx_contents.authorization_list",
}

// AllFieldsWithFrom is used with the transactions feeds
var AllFieldsWithFrom = append(AllFields, "tx_contents.from")

// EthTransaction represents the JSON encoding of an Ethereum transaction.
// All fields are lazily loaded to minimize allocation cost.
// Thread-safe via atomic.Pointer — no mutex.
type EthTransaction struct {
	content TxContent
	tx      atomic.Pointer[ethtypes.Transaction]
	from    atomic.Pointer[common.Address]
	binary  atomic.Pointer[[]byte]
	hexTx   atomic.Pointer[string]
	filters atomic.Pointer[map[string]interface{}]
	fields  atomic.Pointer[map[string]interface{}]
}

// NewEthTransaction creates an EthTransaction from an already-decoded *ethtypes.Transaction.
// Returns no error; binary is computed lazily on first RawTx() call.
func NewEthTransaction(rawEthTx *ethtypes.Transaction, sender Sender) *EthTransaction {
	ethTx := &EthTransaction{}
	ethTx.tx.Store(rawEthTx)

	if sender != EmptySender {
		ethTx.from.Store((*common.Address)(sender[:]))
	}

	return ethTx
}

// NewEthTransactionFromBytes creates an EthTransaction that will lazily decode from TxContent.
func NewEthTransactionFromBytes(content TxContent, sender Sender) *EthTransaction {
	ethTx := &EthTransaction{
		content: content,
	}

	if sender != EmptySender {
		ethTx.from.Store((*common.Address)(sender[:]))
	}

	return ethTx
}

// loadOrDecodeTx returns the underlying Ethereum transaction, decoding lazily if needed.
// Uses CAS so only one goroutine decodes; losers read the winner's value.
func (et *EthTransaction) loadOrDecodeTx() (*ethtypes.Transaction, error) {
	if tx := et.tx.Load(); tx != nil {
		return tx, nil
	}

	if len(et.content) == 0 {
		return nil, ErrEmptyTransaction
	}

	var raw ethtypes.Transaction
	if err := rlp.DecodeBytes(et.content, &raw); err != nil {
		return nil, err
	}
	tx := &raw
	if !et.tx.CompareAndSwap(nil, tx) {
		return et.tx.Load(), nil
	}
	return tx, nil
}

// Tx returns the underlying Ethereum transaction, decoding lazily if needed.
func (et *EthTransaction) Tx() (*ethtypes.Transaction, error) {
	return et.loadOrDecodeTx()
}

// loadOrComputeFrom computes the sender address if not already cached.
func (et *EthTransaction) loadOrComputeFrom() (*common.Address, error) {
	if from := et.from.Load(); from != nil {
		return from, nil
	}

	tx, err := et.loadOrDecodeTx()
	if err != nil {
		return nil, err
	}

	from, err := ethtypes.Sender(LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return nil, fmt.Errorf("could not parse Ethereum transaction from: %v", err)
	}
	p := &from
	if !et.from.CompareAndSwap(nil, p) {
		return et.from.Load(), nil
	}
	return p, nil
}

// From returns the sender of the transaction
func (et *EthTransaction) From() (*common.Address, error) {
	return et.loadOrComputeFrom()
}

// Sender returns the sender of the transaction
func (et *EthTransaction) Sender() (Sender, error) {
	from, err := et.From()
	if err != nil {
		return EmptySender, err
	}

	return Sender(*from), nil
}

// Type provides the transaction type
func (et *EthTransaction) Type() (uint8, error) {
	tx, err := et.Tx()
	if err != nil {
		return 0, err
	}
	return tx.Type(), nil
}

// Hash provides the transaction hash
func (et *EthTransaction) Hash() (SHA256Hash, error) {
	tx, err := et.Tx()
	if err != nil {
		return SHA256Hash{}, err
	}
	hash, err := NewSHA256Hash(tx.Hash().Bytes())
	if err != nil {
		log.Panic("failed to extract hash from a validated eth transaction")
	}
	return hash, nil
}

// AccessList returns access list
func (et *EthTransaction) AccessList() (ethtypes.AccessList, error) {
	tx, err := et.Tx()
	if err != nil {
		return nil, err
	}
	return tx.AccessList(), nil
}

// buildFilters constructs the full filters map from decoded tx data.
func (et *EthTransaction) buildFilters() (map[string]interface{}, error) {
	tx, err := et.loadOrDecodeTx()
	if err != nil {
		return nil, err
	}

	filters := make(map[string]interface{})
	filters["chain_id"] = int(tx.ChainId().Int64())

	switch tx.Type() {
	case ethtypes.BlobTxType: // 3
		filters["max_fee_per_gas"] = int(tx.GasFeeCap().Int64())
		filters["max_priority_fee_per_gas"] = int(tx.GasTipCap().Int64())
		filters["max_fee_per_blob_gas"] = int(tx.BlobGasFeeCap().Int64())
	case ethtypes.DynamicFeeTxType, ethtypes.SetCodeTxType: // 2, 4
		filters["max_fee_per_gas"] = int(tx.GasFeeCap().Int64())
		filters["max_priority_fee_per_gas"] = int(tx.GasTipCap().Int64())
	case ethtypes.AccessListTxType, ethtypes.LegacyTxType: // 1, 0
		filters["gas_price"] = BigIntAsFloat64(tx.GasPrice())
	}

	filters["type"] = strconv.Itoa(int(tx.Type()))
	filters["value"] = BigIntAsFloat64(tx.Value())
	filters["gas"] = float64(tx.Gas())

	if tx.To() != nil {
		filters["to"] = AddressAsString(tx.To())
	} else {
		filters["to"] = "0x0"
	}

	// note: from some reason method_id is only a filter field
	methodID := hexutil.Encode(tx.Data())
	if len(methodID) >= 10 {
		filters["method_id"] = "0x" + methodID[2:10]
	} else {
		filters["method_id"] = methodID
	}

	from, err := et.loadOrComputeFrom()
	if err == nil {
		filters["from"] = AddressAsString(from)
	}

	return filters, nil
}

// createFilters lazily builds and caches the filters map via CAS.
func (et *EthTransaction) createFilters() error {
	if et.filters.Load() != nil {
		return nil
	}

	filters, err := et.buildFilters()
	if err != nil {
		return err
	}

	if !et.filters.CompareAndSwap(nil, &filters) {
		return nil
	}
	return nil
}

// buildFields constructs the full fields map from decoded tx data.
func (et *EthTransaction) buildFields() (map[string]interface{}, error) {
	tx, err := et.loadOrDecodeTx()
	if err != nil {
		return nil, err
	}

	fields := make(map[string]interface{})

	fields["hash"] = tx.Hash().String()
	fields["nonce"] = hexutil.EncodeUint64(tx.Nonce())
	fields["input"] = hexutil.Encode(tx.Data())
	v, r, s := tx.RawSignatureValues()
	fields["v"] = BigIntAsString(v)
	fields["r"] = BigIntAsString(r)
	fields["s"] = BigIntAsString(s)

	if tx.AccessList() != nil {
		fields["accessList"] = tx.AccessList()
	}

	if tx.Type() != ethtypes.LegacyTxType {
		fields["chainId"] = hexutil.EncodeUint64(tx.ChainId().Uint64())
		fields["yParity"] = hexutil.Uint64(uint64(v.Sign())).String() //nolint:gosec // v.Sign() returns -1, 0, or 1; always non-negative for valid signatures
	}

	fields["blobVersionedHashes"] = []string{}
	if tx.Type() == ethtypes.BlobTxType {
		fields["blobVersionedHashes"] = tx.BlobHashes()
		fields["maxFeePerGas"] = hexutil.EncodeBig(tx.GasFeeCap())
		fields["maxPriorityFeePerGas"] = hexutil.EncodeBig(tx.GasTipCap())
		fields["maxFeePerBlobGas"] = hexutil.EncodeBig(tx.BlobGasFeeCap())
		fields["gasPrice"] = nil
	} else if tx.Type() == ethtypes.DynamicFeeTxType || tx.Type() == ethtypes.SetCodeTxType {
		fields["maxFeePerGas"] = hexutil.EncodeBig(tx.GasFeeCap())
		fields["maxPriorityFeePerGas"] = hexutil.EncodeBig(tx.GasTipCap())
		fields["gasPrice"] = nil
	} else {
		fields["gasPrice"] = hexutil.EncodeBig(tx.GasPrice())
	}

	fields["type"] = hexutil.EncodeUint64(uint64(tx.Type()))

	fields["value"] = hexutil.EncodeBig(tx.Value())

	fields["gas"] = hexutil.EncodeUint64(tx.Gas())

	if tx.To() != nil {
		fields["to"] = AddressAsString(tx.To())
	}

	if len(tx.SetCodeAuthorizations()) != 0 {
		fields["authorizationList"] = tx.SetCodeAuthorizations()
	}

	return fields, nil
}

// createFields lazily builds and caches the fields map via CAS.
func (et *EthTransaction) createFields() error {
	if et.fields.Load() != nil {
		return nil
	}

	fields, err := et.buildFields()
	if err != nil {
		return err
	}

	if !et.fields.CompareAndSwap(nil, &fields) {
		return nil
	}
	return nil
}

// EffectiveGasFeeCap returns a common "gas fee cap" that can be used for all types of transactions
func (et *EthTransaction) EffectiveGasFeeCap() (*big.Int, error) {
	tx, err := et.Tx()
	if err != nil {
		return nil, err
	}
	txType := tx.Type()
	if txType == ethtypes.DynamicFeeTxType || txType == ethtypes.BlobTxType || txType == ethtypes.SetCodeTxType {
		return tx.GasFeeCap(), nil
	}
	return tx.GasPrice(), nil
}

// EffectiveGasTipCap returns a common "gas tip cap" that can be used for all types of transactions
func (et *EthTransaction) EffectiveGasTipCap() (*big.Int, error) {
	tx, err := et.Tx()
	if err != nil {
		return nil, err
	}
	txType := tx.Type()
	if txType == ethtypes.DynamicFeeTxType || txType == ethtypes.BlobTxType || txType == ethtypes.SetCodeTxType {
		return tx.GasTipCap(), nil
	}
	return tx.GasPrice(), nil
}

// EffectiveBlobGasFeeCap returns a common "gas fee per blob gas" that can be used for all types of transactions
func (et *EthTransaction) EffectiveBlobGasFeeCap() (*big.Int, error) {
	txType, err := et.Type()
	if err != nil {
		return big.NewInt(0), err
	}
	if txType == ethtypes.BlobTxType {
		tx, err := et.Tx()
		if err != nil {
			return big.NewInt(0), err
		}
		return tx.BlobGasFeeCap(), nil
	}
	return big.NewInt(0), nil
}

// EffectiveBlobGasFeeCapIntCmp make a compare for "blob gas fee cap" that can be used for all types of transactions
func (et *EthTransaction) EffectiveBlobGasFeeCapIntCmp(other *big.Int) (int, error) {
	txType, err := et.Type()
	if err != nil {
		return 1, err
	}
	if txType == ethtypes.BlobTxType {
		tx, err := et.Tx()
		if err != nil {
			return 1, err
		}
		return tx.BlobGasFeeCap().Cmp(other), nil
	}
	return 1, nil
}

// ChainID returns the chain ID of the transaction
func (et *EthTransaction) ChainID() (*big.Int, error) {
	tx, err := et.Tx()
	if err != nil {
		return nil, err
	}
	return tx.ChainId(), nil
}

// Nonce returns the nonce of the transaction
func (et *EthTransaction) Nonce() (uint64, error) {
	tx, err := et.Tx()
	if err != nil {
		return 0, err
	}
	return tx.Nonce(), nil
}

// Filters returns a map of key,value that can be used to filter transactions
func (et *EthTransaction) Filters() (map[string]interface{}, error) {
	if err := et.createFilters(); err != nil {
		return nil, err
	}
	p := et.filters.Load()
	if p == nil {
		return nil, ErrEmptyTransaction
	}
	return *p, nil
}

// Fields - creates a map with selected fields
func (et *EthTransaction) Fields(fields []string) (map[string]interface{}, error) {
	if err := et.createFields(); err != nil {
		return nil, err
	}

	p := et.fields.Load()
	if p == nil {
		return nil, ErrEmptyTransaction
	}
	cached := *p
	transactionContent := make(map[string]interface{})
	for _, param := range fields {
		if v, ok := paramToName[param]; ok {
			param = v
		}

		if v, ok := cached[param]; ok {
			transactionContent[param] = v
		} else if param == "from" {
			from, err := et.loadOrComputeFrom()
			if err != nil {
				continue
			}
			transactionContent["from"] = AddressAsString(from)
		}
	}

	return transactionContent, nil
}

// SetRawTx seeds the raw transaction bytes from a pre-encoded hex string.
// This is used by cloud-api with type 3 txs: the feed txContents is missing the blob
// sidecar, so the raw tx provided by the feed must be used verbatim instead of
// being re-marshaled from the (sidecar-less) decoded content.
func (et *EthTransaction) SetRawTx(rawTx string) {
	et.hexTx.Store(&rawTx)
	if binary, err := hexutil.Decode(rawTx); err == nil {
		et.binary.Store(&binary)
	}
}

// RawTx returns the raw transaction bytes, computed lazily.
func (et *EthTransaction) RawTx() ([]byte, error) {
	if b := et.binary.Load(); b != nil {
		return *b, nil
	}

	tx, err := et.loadOrDecodeTx()
	if err != nil {
		return nil, err
	}

	binary, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if !et.binary.CompareAndSwap(nil, &binary) {
		// another goroutine won (or SetRawTx seeded it); use its value
		return *et.binary.Load(), nil
	}
	return binary, nil
}

// RawTxHex returns the hex-encoded raw transaction, computed lazily.
// hexTx is derived from RawTx here (not in RawTx) so that a concurrent
// caller can never observe binary set but hexTx still unset.
func (et *EthTransaction) RawTxHex() (string, error) {
	if h := et.hexTx.Load(); h != nil {
		return *h, nil
	}

	b, err := et.RawTx()
	if err != nil {
		return "", err
	}
	if b == nil {
		return "", ErrEmptyTransaction
	}

	h := hexutil.Encode(b)
	et.hexTx.CompareAndSwap(nil, &h)
	// reload to honor a SetRawTx seed or concurrent winner
	return *et.hexTx.Load(), nil
}

// AddressAsString converts address to string
func AddressAsString(addr *common.Address) string {
	if addr == nil {
		return "0x"
	}
	return fmt.Sprintf("0x%s", hex.EncodeToString(addr.Bytes()))
}

// BigIntAsFloat64 converts BigInt to float64
func BigIntAsFloat64(bigint *big.Int) float64 {
	floatValue, _ := new(big.Float).SetInt(bigint).Float64()
	return floatValue
}

// BigIntAsString converts BigInt to string
func BigIntAsString(bi *big.Int) string {
	var b bytes.Buffer
	negative := ""

	if bi == nil {
		b.WriteString("\"\"")
		return b.String()
	}

	// if negative remember and take absolute value
	if bi.Sign() == -1 {
		negative = "-"
		bi = big.NewInt(0).Abs(bi)
	}
	t := bi.Text(16)

	b.WriteString(negative + "0x" + t)
	return b.String()
}
