package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"math"
	"math/big"
	"strconv"
	"strings"
)

var nullEthAddress = common.BigToAddress(big.NewInt(0))

// EthTransaction represents the JSON encoding of an Ethereum transaction
type EthTransaction struct {
	TxType     EthTransactionType
	From       EthAddress
	GasPrice   EthBigInt
	GasLimit   EthUInt64
	Hash       EthHash
	Input      EthBytes
	Nonce      EthUInt64
	Value      EthBigInt
	V          EthBigInt
	R          EthBigInt
	S          EthBigInt
	To         EthAddress
	ChainID    EthBigInt
	AccessList ethtypes.AccessList
	GasFeeCap  EthBigInt
	GasTipCap  EthBigInt
}

// ethTxJSON is the JSON representation of transactions, with pointer types for omission
type ethTxJSON struct {
	TxType *EthTransactionType `json:"type,omitempty"`
	Nonce  *EthUInt64          `json:"nonce,omitempty"`
	Gas    *EthUInt64          `json:"gas,omitempty"`
	Value  *EthBigInt          `json:"value,omitempty"`
	Input  *EthBytes           `json:"input,omitempty"`
	V      *EthBigInt          `json:"v,omitempty"`
	R      *EthBigInt          `json:"r,omitempty"`
	S      *EthBigInt          `json:"s,omitempty"`
	To     *EthAddress         `json:"to,omitempty"`
	From   *EthAddress         `json:"from,omitempty"`

	// Legacy and access list transactions only, never omitted
	GasPrice *EthBigInt `json:"gasPrice"`

	// JSON encoding only
	Hash *EthHash `json:"hash,omitempty"`

	// Access list and dynamic fee transactions only
	ChainID    *EthBigInt           `json:"chainId,omitempty"`
	AccessList *ethtypes.AccessList `json:"accessList,omitempty"`

	// Dynamic fee transaction only
	GasFeeCap *EthBigInt `json:"maxFeePerGas,omitempty"`
	GasTipCap *EthBigInt `json:"maxPriorityFeePerGas,omitempty"`
}

// EmptyFilteredTransactionMap - is an empty filtered transaction map use for evaluating filters
var EmptyFilteredTransactionMap = map[string]interface{}{
	"from":                     "0x0",
	"gas_price":                float64(0),
	"gas":                      float64(0),
	"tx_hash":                  "0x0",
	"input":                    "0x0",
	"method_id":                "0x0",
	"value":                    float64(0),
	"to":                       "0x0",
	"type":                     "0",
	"chain_id":                 float64(0),
	"max_fee_per_gas":          float64(0),
	"max_priority_fee_per_gas": float64(0),
}

// NewEthTransaction converts a canonic Ethereum transaction to EthTransaction
func NewEthTransaction(h SHA256Hash, rawEthTx *ethtypes.Transaction, extractSender bool) (*EthTransaction, error) {
	var (
		sender common.Address
		err    error
	)
	v, r, s := rawEthTx.RawSignatureValues()
	if extractSender {
		sender, err = ethtypes.Sender(ethtypes.NewLondonSigner(rawEthTx.ChainId()), rawEthTx)
		if err != nil {
			return nil, fmt.Errorf("could not parse Ethereum transaction sender: %v", err)
		}
	}
	ethTx := &EthTransaction{
		TxType:   EthTransactionType{rawEthTx.Type()},
		From:     EthAddress{Address: &sender},
		GasLimit: EthUInt64{UInt64: rawEthTx.Gas()},
		Hash:     EthHash{SHA256Hash: h},
		Input:    EthBytes{B: rawEthTx.Data()},
		Nonce:    EthUInt64{UInt64: rawEthTx.Nonce()},
		Value:    EthBigInt{Int: rawEthTx.Value()},
		V:        EthBigInt{Int: v},
		R:        EthBigInt{Int: r},
		S:        EthBigInt{Int: s},
		To:       EthAddress{Address: rawEthTx.To()},
	}
	ethTx.ChainID = EthBigInt{rawEthTx.ChainId()}
	ethTx.GasPrice = EthBigInt{Int: rawEthTx.GasPrice()}
	ethTx.AccessList = rawEthTx.AccessList()
	ethTx.GasFeeCap = EthBigInt{rawEthTx.GasFeeCap()}
	ethTx.GasTipCap = EthBigInt{rawEthTx.GasTipCap()}
	return ethTx, nil
}

// EthTransactionFromBytes parses and constructs an Ethereum transaction from bytes
func EthTransactionFromBytes(h SHA256Hash, tc TxContent, extractSender bool) (*EthTransaction, error) {
	var rawEthTx ethtypes.Transaction

	err := rlp.DecodeBytes(tc, &rawEthTx)
	if err != nil {
		return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
	}

	return NewEthTransaction(h, &rawEthTx, extractSender)
}

// EffectiveGasFeeCap returns a common "gas fee cap" that can be used for all types of transactions
func (et EthTransaction) EffectiveGasFeeCap() EthBigInt {
	switch et.TxType {
	case DynamicFeeTransactionType:
		return et.GasFeeCap
	}
	return et.GasPrice
}

// EffectiveGasTipCap returns a common "gas tip cap" that can be used for all types of transactions
func (et EthTransaction) EffectiveGasTipCap() EthBigInt {
	switch et.TxType {
	case DynamicFeeTransactionType:
		return et.GasTipCap
	}
	return et.GasPrice
}

// Filters returns the same transaction, with only the specified fields included in a map data format for running through a conditional filter. -1 values are provided for fields like gas_price, max_fee_per_gas, etc. since these are typically expected to be unsigned integers.
func (et EthTransaction) Filters(filters []string) map[string]interface{} {
	var transactionFilters = make(map[string]interface{})
	for _, param := range filters {
		switch param {
		case "value":
			floatValue, _ := new(big.Float).SetInt(et.Value.Int).Float64()
			transactionFilters[param] = floatValue
		case "gas":
			transactionFilters[param] = float64(et.GasLimit.UInt64)
		case "gas_price":
			if et.GasPrice.Int != nil && et.TxType != DynamicFeeTransactionType {
				transactionFilters[param], _ = new(big.Float).SetInt(et.GasPrice.Int).Float64()
			} else {
				transactionFilters[param] = -1
			}
		case "to":
			if et.To.Address != nil {
				transactionFilters[param] = strings.ToLower(et.To.Address.String())
			} else {
				transactionFilters[param] = ""
			}
		case "from":
			if et.From.Address != nil {
				transactionFilters[param] = strings.ToLower(et.From.Address.String())
			} else {
				transactionFilters[param] = ""
			}
		case "method_id":
			methodID := strings.ToLower(hexutil.Encode(et.Input.B))
			if len(methodID) >= 10 {
				transactionFilters[param] = "0x" + methodID[2:10]
			} else {
				transactionFilters[param] = methodID
			}
		case "type":
			transactionFilters[param] = strconv.Itoa(int(et.TxType.UInt8))
		case "chain_id":
			if et.ChainID.Int != nil {
				transactionFilters[param] = int(et.ChainID.Int64())
			}
		case "max_fee_per_gas":
			if et.GasFeeCap.Int != nil && et.TxType == DynamicFeeTransactionType {
				transactionFilters[param] = int(et.GasFeeCap.Int64())
			} else {
				transactionFilters[param] = -1
			}
		case "max_priority_fee_per_gas":
			if et.GasTipCap.Int != nil && et.TxType == DynamicFeeTransactionType {
				transactionFilters[param] = int(et.GasTipCap.Int64())
			} else {
				transactionFilters[param] = -1
			}
		}
	}
	return transactionFilters
}

// WithFields returns the same transaction, with only the specified fields included for serialization purposes
func (et EthTransaction) WithFields(fields []string) BlockchainTransaction {
	transactionContent := EthTransaction{}
	transactionContent.TxType = NilTransactionType
	transactionContent.Nonce = nilNonceValue

	for _, param := range fields {
		switch param {
		case "tx_contents.tx_hash":
			transactionContent.Hash = et.Hash
		case "tx_contents.nonce":
			transactionContent.Nonce = et.Nonce
		case "tx_contents.gas_price":
			if et.TxType != DynamicFeeTransactionType {
				transactionContent.GasPrice = et.GasPrice
			}
		case "tx_contents.gas":
			transactionContent.GasLimit = et.GasLimit
		case "tx_contents.to":
			transactionContent.To = et.To
			if et.To.Address == nil {
				transactionContent.To.Address = &nullEthAddress
			}
		case "tx_contents.value":
			transactionContent.Value = et.Value
		case "tx_contents.input":
			transactionContent.Input = et.Input
		case "tx_contents.v":
			transactionContent.V = et.V
		case "tx_contents.r":
			transactionContent.R = et.R
		case "tx_contents.s":
			transactionContent.S = et.S
		case "tx_contents.from":
			transactionContent.From = et.From
		case "tx_contents.type":
			transactionContent.TxType = et.TxType
		case "tx_contents.access_list":
			transactionContent.AccessList = et.AccessList
		case "tx_contents.chain_id":
			transactionContent.ChainID = et.ChainID
		case "tx_contents.max_fee_per_gas":
			if et.TxType == DynamicFeeTransactionType {
				transactionContent.GasFeeCap = et.GasFeeCap
			}
		case "tx_contents.max_priority_fee_per_gas":
			if et.TxType == DynamicFeeTransactionType {
				transactionContent.GasTipCap = et.GasTipCap
			}
		}
	}
	return transactionContent
}

// EthTransactionType represents different types of encoded Ethereum transactions
type EthTransactionType EthUInt8

// Constants for identifying each Ethereum transaction types
var (
	NilTransactionType        = EthTransactionType{math.MaxUint8}
	LegacyTransactionType     = EthTransactionType{0}
	AccessListTransactionType = EthTransactionType{1}
	DynamicFeeTransactionType = EthTransactionType{2}
)

var nilNonceValue = EthUInt64{math.MaxUint64}

// EthAddress wraps an Ethereum address
type EthAddress struct {
	*common.Address
}

// MarshalJSON formats an EthAddress according to the bloxroute spec
func (a EthAddress) MarshalJSON() ([]byte, error) {
	if a.Address == nil {
		return json.Marshal("0x")
	}
	return json.Marshal(strings.ToLower(a.Address.Hex()))
}

// EthBigInt wraps an big.Int value
type EthBigInt struct {
	*big.Int
}

// LessThan compares the wrapped big.Int with another
func (bi EthBigInt) LessThan(other EthBigInt) bool {
	return bi.Int.Cmp(other.Int) == -1
}

// GreaterThan compares the wrapped big.Int with another
func (bi EthBigInt) GreaterThan(other EthBigInt) bool {
	return bi.Int.Cmp(other.Int) == 1
}

// GreaterThanOrEqualTo compares the wrapped big.Int with another
func (bi EthBigInt) GreaterThanOrEqualTo(other EthBigInt) bool {
	return bi.Int.Cmp(other.Int) != -1
}

// MarshalJSON formats an big.Int according to the bloxroute spec
func (bi EthBigInt) MarshalJSON() ([]byte, error) {
	if bi.Int != nil {
		return json.Marshal(strings.ToLower(hexutil.EncodeBig(bi.Int)))
	}
	return json.Marshal("")
}

// EthUInt64 wraps a uint64
type EthUInt64 struct {
	UInt64 uint64
}

// MarshalJSON formats an uint64 according to the bloxroute spec
func (ui EthUInt64) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(hexutil.EncodeUint64(ui.UInt64)))
}

// LessThan compares the wrapped uint64 with another
func (ui EthUInt64) LessThan(other EthUInt64) bool {
	return ui.UInt64 < other.UInt64
}

// GreaterThan compares the wrapped uint64 with another
func (ui EthUInt64) GreaterThan(other EthUInt64) bool {
	return ui.UInt64 > other.UInt64
}

// EthUInt8 wraps an uint8
type EthUInt8 struct {
	UInt8 uint8
}

// MarshalJSON formats an uint8 according to the bloxroute spec
func (ui EthUInt8) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(hexutil.EncodeUint64(uint64(ui.UInt8))))
}

// LessThan compares the wrapped uint8 with another
func (ui EthUInt8) LessThan(other EthUInt8) bool {
	return ui.UInt8 < other.UInt8
}

// GreaterThan compares the wrapped uint8 with another
func (ui EthUInt8) GreaterThan(other EthUInt8) bool {
	return ui.UInt8 > other.UInt8
}

// MarshalJSON formats an EthTransactionType according to the bloxroute spec
func (ui EthTransactionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(hexutil.EncodeUint64(uint64(ui.UInt8))))
}

// EthBytes wraps an Ethereum bytearray
type EthBytes struct {
	B []byte
}

// MarshalJSON formats an wrapped []byte according to the bloxroute spec
func (eb EthBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(hexutil.Encode(eb.B)))
}

// EthHash wraps an SHA256Hash
type EthHash struct {
	SHA256Hash
}

// MarshalJSON formats an wrapped SHA256Hash according to the bloxroute spec
func (eh EthHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.ToLower(eh.SHA256Hash.Format(true)))
}

// MarshalJSON formats an ethTxJSON according to the bloxroute spec via EthTransaction
func (et EthTransaction) MarshalJSON() ([]byte, error) {
	var enc ethTxJSON

	if et.TxType != NilTransactionType {
		enc.TxType = &et.TxType
	}
	if et.AccessList != nil {
		enc.AccessList = &et.AccessList
	}
	emptyByteVar := make([]byte, 32)
	if !bytes.Equal(et.Hash.SHA256Hash[:], emptyByteVar) {
		enc.Hash = &et.Hash
	}
	if et.Nonce != nilNonceValue {
		enc.Nonce = &et.Nonce
	}
	if et.GasLimit.UInt64 != 0 {
		enc.Gas = &et.GasLimit
	}
	if et.Input.B != nil {
		enc.Input = &et.Input
	}
	if et.GasPrice.Int != nil {
		enc.GasPrice = &et.GasPrice
	}
	if et.Value.Int != nil {
		enc.Value = &et.Value
	}
	if et.V.Int != nil {
		enc.V = &et.V
	}
	if et.R.Int != nil {
		enc.R = &et.R
	}
	if et.S.Int != nil {
		enc.S = &et.S
	}
	if et.To.Address != nil {
		enc.To = &et.To
	}
	if et.From.Address != nil {
		enc.From = &et.From
	}
	if et.TxType != LegacyTransactionType {
		enc.ChainID = &et.ChainID
	}
	if et.GasFeeCap.Int != nil {
		enc.GasFeeCap = &et.GasFeeCap
	}
	if et.GasTipCap.Int != nil {
		enc.GasTipCap = &et.GasTipCap
	}

	// include nil "to" field if requested
	marshalled, err := json.Marshal(&enc)
	if enc.To != nil && enc.To.Address != &nullEthAddress {
		return marshalled, err
	}
	var mapWithNilToField map[string]interface{}
	json.Unmarshal(marshalled, &mapWithNilToField)
	mapWithNilToField["to"] = nil
	return json.Marshal(mapWithNilToField)
}
