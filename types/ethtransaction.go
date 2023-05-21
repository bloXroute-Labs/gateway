package types

import (
	"bytes"
	"fmt"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"strconv"
	"strings"
	"sync"
)

// EthTransaction represents the JSON encoding of an Ethereum transaction
type EthTransaction struct {
	tx        *ethtypes.Transaction
	GasFeeCap *big.Int
	GasTipCap *big.Int
	ChainID   *big.Int
	From      *common.Address
	Nonce     uint64

	lock    *sync.Mutex
	filters map[string]interface{}
	fields  map[string]interface{}
}

var paramToName = map[string]string{
	"tx_hash":                  "hash",
	"access_list":              "accessList",
	"chain_id":                 "chainId",
	"gas_price":                "gasPrice",
	"max_fee_per_gas":          "maxFeePerGas",
	"max_priority_fee_per_gas": "maxPriorityFeePerGas",
}

// AllFields is used with blocks feeds
var AllFields []string

// EmptyFilteredTransactionMap is a map of key value used to check the filters provided by the websocket client
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
func NewEthTransaction(h SHA256Hash, rawEthTx *ethtypes.Transaction, sender Sender) (*EthTransaction, error) {
	var (
		ethSender common.Address
		err       error
	)
	if sender == EmptySender {
		ethSender, err = ethtypes.Sender(ethtypes.NewLondonSigner(rawEthTx.ChainId()), rawEthTx)
		if err != nil {
			return nil, fmt.Errorf("could not parse Ethereum transaction sender: %v", err)
		}
	} else {
		ethSender.SetBytes(sender[:])
	}
	ethTx := &EthTransaction{
		tx:      rawEthTx,
		From:    &ethSender,
		Nonce:   rawEthTx.Nonce(),
		ChainID: rawEthTx.ChainId(),
		lock:    &sync.Mutex{},
	}
	switch rawEthTx.Type() {
	case ethtypes.DynamicFeeTxType:
		ethTx.GasFeeCap = rawEthTx.GasFeeCap()
		ethTx.GasTipCap = rawEthTx.GasTipCap()
	default:
		ethTx.GasFeeCap = rawEthTx.GasPrice()
		ethTx.GasTipCap = rawEthTx.GasPrice()
	}
	return ethTx, nil
}

// Type provides the transaction type
func (et EthTransaction) Type() uint8 {
	return et.tx.Type()
}

// Hash provides the transaction hash
func (et EthTransaction) Hash() SHA256Hash {
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

// CreateFieldsGRPC creates fields for GRPC
func (et *EthTransaction) CreateFieldsGRPC() *pb.Tx {
	et.lock.Lock()
	defer et.lock.Unlock()
	tx := et.tx

	var gasPrice string
	if tx.Type() == ethtypes.DynamicFeeTxType {
		gasPrice = ""
	} else {
		gasPrice = hexutil.EncodeBig(tx.GasPrice())
	}

	var to string
	if tx.To() != nil {
		to = AddressAsString(tx.To())
	} else {
		to = "0x0"
	}

	v, r, s := tx.RawSignatureValues()

	grpcTx := &pb.Tx{
		Hash:     strings.ToLower(tx.Hash().String()),
		Nonce:    strings.ToLower(hexutil.EncodeUint64(tx.Nonce())),
		GasPrice: gasPrice,
		Gas:      hexutil.EncodeUint64(tx.Gas()),
		To:       to,
		Value:    hexutil.EncodeBig(tx.Value()),
		Input:    strings.ToLower(hexutil.Encode(tx.Data())),
		V:        BigIntAsString(v),
		R:        BigIntAsString(r),
		S:        BigIntAsString(s),
		From:     AddressAsString(et.From),
		Type:     hexutil.EncodeUint64(uint64(tx.Type())),
	}
	if tx.Type() != ethtypes.LegacyTxType {
		grpcTx.ChainID = strings.ToLower(hexutil.EncodeUint64(tx.ChainId().Uint64()))
	}
	if tx.Type() == ethtypes.DynamicFeeTxType {
		grpcTx.MaxFeePerGas = strings.ToLower(hexutil.EncodeUint64(tx.GasFeeCap().Uint64()))
		grpcTx.MaxPriorityFeePerGas = strings.ToLower(hexutil.EncodeUint64(tx.GasTipCap().Uint64()))
	}
	return grpcTx
}

// AccessList returns access list
func (et *EthTransaction) AccessList() ethtypes.AccessList {
	return et.tx.AccessList()
}

func (et *EthTransaction) createFields() {
	et.lock.Lock()
	defer et.lock.Unlock()
	if et.filters != nil {
		return
	}

	var transactionFilters = make(map[string]interface{})
	tx := et.tx

	fields := make(map[string]interface{})
	fields["hash"] = strings.ToLower(tx.Hash().String())
	fields["nonce"] = strings.ToLower(hexutil.EncodeUint64(tx.Nonce()))
	fields["input"] = strings.ToLower(hexutil.Encode(tx.Data()))
	v, r, s := tx.RawSignatureValues()
	fields["v"] = BigIntAsString(v)
	fields["r"] = BigIntAsString(r)
	fields["s"] = BigIntAsString(s)

	if tx.AccessList() != nil {
		fields["accessList"] = tx.AccessList()
	}

	transactionFilters["chain_id"] = int(tx.ChainId().Int64())
	if tx.Type() != ethtypes.LegacyTxType {
		fields["chainId"] = strings.ToLower(hexutil.EncodeUint64(tx.ChainId().Uint64()))
	}

	if tx.Type() == ethtypes.DynamicFeeTxType {
		transactionFilters["max_fee_per_gas"] = int(tx.GasFeeCap().Int64())
		fields["maxFeePerGas"] = hexutil.EncodeBig(tx.GasFeeCap())
		transactionFilters["max_priority_fee_per_gas"] = int(tx.GasTipCap().Int64())
		fields["maxPriorityFeePerGas"] = hexutil.EncodeBig(tx.GasTipCap())
		transactionFilters["gas_price"] = nil
		fields["gasPrice"] = nil
	} else {
		transactionFilters["gas_price"] = BigIntAsFloat64(tx.GasPrice())
		fields["gasPrice"] = hexutil.EncodeBig(tx.GasPrice())
	}

	transactionFilters["type"] = strconv.Itoa(int(tx.Type()))
	fields["type"] = hexutil.EncodeUint64(uint64(tx.Type()))

	transactionFilters["value"] = BigIntAsFloat64(tx.Value())
	fields["value"] = hexutil.EncodeBig(tx.Value())

	transactionFilters["gas"] = float64(tx.Gas())
	fields["gas"] = hexutil.EncodeUint64(tx.Gas())

	if tx.To() != nil {
		transactionFilters["to"] = AddressAsString(tx.To())
		fields["to"] = AddressAsString(tx.To())
	} else {
		transactionFilters["to"] = "0x0"
	}

	//if sender != nil {
	transactionFilters["from"] = AddressAsString(et.From)
	fields["from"] = transactionFilters["from"]
	//}

	// note: from some reason method_id is only a filter field
	methodID := strings.ToLower(hexutil.Encode(tx.Data()))
	if len(methodID) >= 10 {
		transactionFilters["method_id"] = "0x" + methodID[2:10]
	} else {
		transactionFilters["method_id"] = methodID
	}

	et.fields = fields
	et.filters = transactionFilters
}

// EthTransactionFromBytes parses and constructs an Ethereum transaction from bytes
func ethTransactionFromBytes(h SHA256Hash, tc TxContent, sender Sender) (*EthTransaction, error) {
	var rawEthTx ethtypes.Transaction

	err := rlp.DecodeBytes(tc, &rawEthTx)
	if err != nil {
		return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
	}

	return NewEthTransaction(h, &rawEthTx, sender)
}

// EffectiveGasFeeCap returns a common "gas fee cap" that can be used for all types of transactions
func (et EthTransaction) EffectiveGasFeeCap() *big.Int {
	return et.GasFeeCap
}

// EffectiveGasTipCap returns a common "gas tip cap" that can be used for all types of transactions
func (et EthTransaction) EffectiveGasTipCap() *big.Int {
	return et.GasTipCap
}

// Filters returns a map of key,value that can be used to filter transactions
func (et *EthTransaction) Filters(filters []string) map[string]interface{} {
	et.createFields()
	return et.filters
}

// Fields - creates a map with selected fields
func (et *EthTransaction) Fields(fields []string) map[string]interface{} {
	et.createFields()

	if len(fields) == 0 {
		return et.fields
	}

	transactionContent := make(map[string]interface{})
	for _, param := range fields {
		parts := strings.Split(param, ".")
		if len(parts) != 2 {
			continue
		}
		name := parts[1]
		if v, ok := paramToName[name]; ok {
			name = v
		}

		if v, ok := et.fields[name]; ok {
			transactionContent[name] = v
		}
	}
	return transactionContent
}

// AddressAsString converts address to string
func AddressAsString(addr *common.Address) string {
	if addr == nil {
		return string([]byte("0x"))
	}
	return strings.ToLower(addr.Hex())
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
	return string(b.String())
}
