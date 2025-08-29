package types

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
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

// EthTransaction represents the JSON encoding of an Ethereum transaction
type EthTransaction struct {
	tx *ethtypes.Transaction

	// lazy loaded and excluded from 'all' fields
	from *common.Address

	// binary is the raw transaction bytes
	binary []byte

	lock    *sync.Mutex
	filters map[string]interface{}
	fields  map[string]interface{}
}

// NewEthTransaction converts a canonic Ethereum transaction to EthTransaction
func NewEthTransaction(rawEthTx *ethtypes.Transaction, sender Sender) (*EthTransaction, error) {
	ethTx := &EthTransaction{
		tx:      rawEthTx,
		lock:    &sync.Mutex{},
		filters: make(map[string]interface{}),
		fields:  make(map[string]interface{}),
	}

	if sender != EmptySender {
		ethTx.from = (*common.Address)(sender[:])
	}

	binary, err := rawEthTx.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx binary: %v", err)
	}
	ethTx.binary = binary

	return ethTx, nil
}

// Tx returns the underlying Ethereum transaction
func (et *EthTransaction) Tx() *ethtypes.Transaction {
	return et.tx
}

// From returns the sender of the transaction
func (et *EthTransaction) From() (*common.Address, error) {
	et.lock.Lock()
	defer et.lock.Unlock()

	return et.sender()
}

func (et *EthTransaction) sender() (*common.Address, error) {
	// cached
	if et.from != nil {
		return et.from, nil
	}

	from, err := ethtypes.Sender(LatestSignerForChainID(et.tx.ChainId()), et.tx)
	if err != nil {
		return nil, fmt.Errorf("could not parse Ethereum transaction from: %v", err)
	}

	// cache
	et.from = &from

	return &from, nil
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
func (et *EthTransaction) Type() uint8 {
	return et.tx.Type()
}

// Hash provides the transaction hash
func (et *EthTransaction) Hash() SHA256Hash {
	var hash SHA256Hash
	var err error
	if et.tx != nil {
		hash, err = NewSHA256Hash(et.tx.Hash().Bytes())
		if err != nil {
			log.Panic("failed to extract hash from a validated eth transaction")
		}
	}
	return hash
}

// AccessList returns access list
func (et *EthTransaction) AccessList() ethtypes.AccessList {
	return et.tx.AccessList()
}

func (et *EthTransaction) createFilters() {
	et.lock.Lock()
	defer et.lock.Unlock()

	if len(et.filters) > 0 {
		return
	}

	tx := et.tx
	et.filters["chain_id"] = int(tx.ChainId().Int64())

	switch tx.Type() {
	case ethtypes.BlobTxType: // 3
		et.filters["max_fee_per_gas"] = int(tx.GasFeeCap().Int64())
		et.filters["max_priority_fee_per_gas"] = int(tx.GasTipCap().Int64())
		et.filters["max_fee_per_blob_gas"] = int(tx.BlobGasFeeCap().Int64())
	case ethtypes.DynamicFeeTxType, ethtypes.SetCodeTxType: // 2, 4
		et.filters["max_fee_per_gas"] = int(tx.GasFeeCap().Int64())
		et.filters["max_priority_fee_per_gas"] = int(tx.GasTipCap().Int64())
	case ethtypes.AccessListTxType, ethtypes.LegacyTxType: // 1, 0
		et.filters["gas_price"] = BigIntAsFloat64(tx.GasPrice())
	}

	et.filters["type"] = strconv.Itoa(int(tx.Type()))
	et.filters["value"] = BigIntAsFloat64(tx.Value())
	et.filters["gas"] = float64(tx.Gas())

	if tx.To() != nil {
		et.filters["to"] = AddressAsString(tx.To())
	} else {
		et.filters["to"] = "0x0"
	}

	// note: from some reason method_id is only a filter field
	methodID := hexutil.Encode(tx.Data())
	if len(methodID) >= 10 {
		et.filters["method_id"] = "0x" + methodID[2:10]
	} else {
		et.filters["method_id"] = methodID
	}

	from, err := et.sender()
	if err == nil {
		et.filters["from"] = AddressAsString(from)
	}
}

func (et *EthTransaction) createFields() {
	et.lock.Lock()
	defer et.lock.Unlock()

	if len(et.fields) > 0 {
		return
	}

	tx := et.tx

	et.fields["hash"] = tx.Hash().String()
	et.fields["nonce"] = hexutil.EncodeUint64(tx.Nonce())
	et.fields["input"] = hexutil.Encode(tx.Data())
	v, r, s := tx.RawSignatureValues()
	et.fields["v"] = BigIntAsString(v)
	et.fields["r"] = BigIntAsString(r)
	et.fields["s"] = BigIntAsString(s)

	if tx.AccessList() != nil {
		et.fields["accessList"] = tx.AccessList()
	}

	if tx.Type() != ethtypes.LegacyTxType {
		et.fields["chainId"] = hexutil.EncodeUint64(tx.ChainId().Uint64())
		et.fields["yParity"] = hexutil.Uint64(v.Sign()).String()
	}

	et.fields["blobVersionedHashes"] = []string{}
	if tx.Type() == ethtypes.BlobTxType {
		et.fields["blobVersionedHashes"] = tx.BlobHashes()
		et.fields["maxFeePerGas"] = hexutil.EncodeBig(tx.GasFeeCap())
		et.fields["maxPriorityFeePerGas"] = hexutil.EncodeBig(tx.GasTipCap())
		et.fields["maxFeePerBlobGas"] = hexutil.EncodeBig(tx.BlobGasFeeCap())
		et.fields["gasPrice"] = nil
	} else if tx.Type() == ethtypes.DynamicFeeTxType || tx.Type() == ethtypes.SetCodeTxType {
		et.fields["maxFeePerGas"] = hexutil.EncodeBig(tx.GasFeeCap())
		et.fields["maxPriorityFeePerGas"] = hexutil.EncodeBig(tx.GasTipCap())
		et.fields["gasPrice"] = nil
	} else {
		et.fields["gasPrice"] = hexutil.EncodeBig(tx.GasPrice())
	}

	et.fields["type"] = hexutil.EncodeUint64(uint64(tx.Type()))

	et.fields["value"] = hexutil.EncodeBig(tx.Value())

	et.fields["gas"] = hexutil.EncodeUint64(tx.Gas())

	if tx.To() != nil {
		et.fields["to"] = AddressAsString(tx.To())
	}

	if len(tx.SetCodeAuthorizations()) != 0 {
		et.fields["authorizationList"] = tx.SetCodeAuthorizations()
	}
}

// EthTransactionFromBytes parses and constructs an Ethereum transaction from bytes
func ethTransactionFromBytes(tc TxContent, sender Sender) (*EthTransaction, error) {
	var rawEthTx ethtypes.Transaction

	err := rlp.DecodeBytes(tc, &rawEthTx)
	if err != nil {
		return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
	}

	return NewEthTransaction(&rawEthTx, sender)
}

// EffectiveGasFeeCap returns a common "gas fee cap" that can be used for all types of transactions
func (et *EthTransaction) EffectiveGasFeeCap() *big.Int {
	if et.Type() == ethtypes.DynamicFeeTxType || et.Type() == ethtypes.BlobTxType || et.Type() == ethtypes.SetCodeTxType {
		return et.tx.GasFeeCap()
	}

	return et.tx.GasPrice()
}

// EffectiveGasTipCap returns a common "gas tip cap" that can be used for all types of transactions
func (et *EthTransaction) EffectiveGasTipCap() *big.Int {
	if et.Type() == ethtypes.DynamicFeeTxType || et.Type() == ethtypes.BlobTxType || et.Type() == ethtypes.SetCodeTxType {
		return et.tx.GasTipCap()
	}

	return et.tx.GasPrice()
}

// EffectiveBlobGasFeeCap returns a common "gas fee per blob gas" that can be used for all types of transactions
func (et *EthTransaction) EffectiveBlobGasFeeCap() *big.Int {
	if et.Type() == ethtypes.BlobTxType {
		return et.tx.BlobGasFeeCap()
	}

	return big.NewInt(0)
}

// EffectiveBlobGasFeeCapIntCmp make a compare for "blob gas fee cap" that can be used for all types of transactions
func (et *EthTransaction) EffectiveBlobGasFeeCapIntCmp(other *big.Int) int {
	if et.Type() == ethtypes.BlobTxType {
		return et.tx.BlobGasFeeCap().Cmp(other)
	}

	// Legacy and dynamic fee transactions have no blob gas fee cap
	return 1
}

// ChainID returns the chain ID of the transaction
func (et *EthTransaction) ChainID() *big.Int {
	return et.tx.ChainId()
}

// Nonce returns the nonce of the transaction
func (et *EthTransaction) Nonce() uint64 {
	return et.tx.Nonce()
}

// Filters returns a map of key,value that can be used to filter transactions
func (et *EthTransaction) Filters() map[string]interface{} {
	et.createFilters()

	return et.filters
}

// Fields - creates a map with selected fields
func (et *EthTransaction) Fields(fields []string) map[string]interface{} {
	et.createFields()

	transactionContent := make(map[string]interface{})
	for _, param := range fields {
		if v, ok := paramToName[param]; ok {
			param = v
		}

		if v, ok := et.fields[param]; ok {
			transactionContent[param] = v
		} else if param == "from" {
			from, err := et.From()
			if err != nil {
				continue
			}
			transactionContent["from"] = AddressAsString(from)
		}
	}

	return transactionContent
}

// RawTx returns the raw transaction bytes
func (et *EthTransaction) RawTx() []byte {
	return et.binary
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
	var negative = ""

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
