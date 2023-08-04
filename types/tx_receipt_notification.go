package types

import (
	"encoding/json"
)

const nullAddressStr = "0x"

// TxReceiptNotification - represents a transaction receipt feed entry
// to avoid deserializing/reserializing the message from Ethereum RPC, no conversion work is done
type TxReceiptNotification struct {
	Receipt txReceipt
}

type txReceipt struct {
	BlockHash         string        `json:"block_hash,omitempty"`
	BlockNumber       string        `json:"block_number,omitempty"`
	ContractAddress   interface{}   `json:"contract_address,omitempty"`
	CumulativeGasUsed string        `json:"cumulative_gas_used,omitempty"`
	EffectiveGasPrice string        `json:"effective_gas_price,omitempty"`
	From              interface{}   `json:"from,omitempty"`
	GasUsed           string        `json:"gas_used,omitempty"`
	Logs              []interface{} `json:"logs,omitempty"`
	LogsBloom         string        `json:"logs_bloom,omitempty"`
	Status            string        `json:"status,omitempty"`
	To                interface{}   `json:"to,omitempty"`
	TransactionHash   string        `json:"transaction_hash,omitempty"`
	TransactionIndex  string        `json:"transaction_index,omitempty"`
	TxType            string        `json:"type,omitempty"`
	TxsCount          string        `json:"txs_count,omitempty"`
}

// NewTxReceiptNotification returns a new TxReceiptNotification
func NewTxReceiptNotification(txReceipt map[string]interface{}, txsCount string) *TxReceiptNotification {
	txReceiptNotification := TxReceiptNotification{}

	blockHash, ok := txReceipt["blockHash"]
	if ok {
		txReceiptNotification.Receipt.BlockHash = blockHash.(string)
	}

	blockNumber, ok := txReceipt["blockNumber"]
	if ok {
		txReceiptNotification.Receipt.BlockNumber = blockNumber.(string)
	}

	contractAddress, ok := txReceipt["contractAddress"]
	if ok {
		txReceiptNotification.Receipt.ContractAddress = contractAddress
	}

	cumulativeGasUsed, ok := txReceipt["cumulativeGasUsed"]
	if ok {
		txReceiptNotification.Receipt.CumulativeGasUsed = cumulativeGasUsed.(string)
	}

	effectiveGasPrice, ok := txReceipt["effectiveGasPrice"]
	if ok {
		txReceiptNotification.Receipt.EffectiveGasPrice = effectiveGasPrice.(string)
	}

	from, ok := txReceipt["from"]
	if ok {
		txReceiptNotification.Receipt.From = from
	}

	gasUsed, ok := txReceipt["gasUsed"]
	if ok {
		txReceiptNotification.Receipt.GasUsed = gasUsed.(string)
	}

	logs, ok := txReceipt["logs"]
	if ok {
		txReceiptNotification.Receipt.Logs = logs.([]interface{})
	}

	logsBloom, ok := txReceipt["logsBloom"]
	if ok {
		txReceiptNotification.Receipt.LogsBloom = logsBloom.(string)
	}

	status, ok := txReceipt["status"]
	if ok {
		txReceiptNotification.Receipt.Status = status.(string)
	}

	to, ok := txReceipt["to"]
	if ok {
		txReceiptNotification.Receipt.To = to
	}

	transactionHash, ok := txReceipt["transactionHash"]
	if ok {
		txReceiptNotification.Receipt.TransactionHash = transactionHash.(string)
	}

	transactionIndex, ok := txReceipt["transactionIndex"]
	if ok {
		txReceiptNotification.Receipt.TransactionIndex = transactionIndex.(string)
	}

	txType, ok := txReceipt["type"]
	if ok {
		txReceiptNotification.Receipt.TxType = txType.(string)
	}

	txReceiptNotification.Receipt.TxsCount = txsCount

	return &txReceiptNotification
}

// MarshalJSON formats txReceiptNotification, including nil "to" field if requested
func (r *TxReceiptNotification) MarshalJSON() ([]byte, error) {
	marshalled, err := json.Marshal(r.Receipt)
	if r.Receipt.To != nullAddressStr {
		return marshalled, err
	}
	var mapWithNilToField map[string]interface{}
	json.Unmarshal(marshalled, &mapWithNilToField)
	mapWithNilToField["to"] = nil
	return json.Marshal(mapWithNilToField)
}

// WithFields -
func (r *TxReceiptNotification) WithFields(fields []string) Notification {
	txReceiptNotification := TxReceiptNotification{}
	for _, param := range fields {
		switch param {
		case "block_hash":
			txReceiptNotification.Receipt.BlockHash = r.Receipt.BlockHash
		case "block_number":
			txReceiptNotification.Receipt.BlockNumber = r.Receipt.BlockNumber
		case "contract_address":
			txReceiptNotification.Receipt.ContractAddress = r.Receipt.ContractAddress
		case "cumulative_gas_used":
			txReceiptNotification.Receipt.CumulativeGasUsed = r.Receipt.CumulativeGasUsed
		case "effective_gas_price":
			txReceiptNotification.Receipt.EffectiveGasPrice = r.Receipt.EffectiveGasPrice
		case "from":
			txReceiptNotification.Receipt.From = r.Receipt.From
		case "gas_used":
			txReceiptNotification.Receipt.GasUsed = r.Receipt.GasUsed
		case "logs":
			txReceiptNotification.Receipt.Logs = r.Receipt.Logs
		case "logs_bloom":
			txReceiptNotification.Receipt.LogsBloom = r.Receipt.LogsBloom
		case "status":
			txReceiptNotification.Receipt.Status = r.Receipt.Status
		case "to":
			txReceiptNotification.Receipt.To = r.Receipt.To
			if r.Receipt.To == nil {
				txReceiptNotification.Receipt.To = nullAddressStr
			}
		case "transaction_hash":
			txReceiptNotification.Receipt.TransactionHash = r.Receipt.TransactionHash
		case "transaction_index":
			txReceiptNotification.Receipt.TransactionIndex = r.Receipt.TransactionIndex
		case "type":
			txReceiptNotification.Receipt.TxType = r.Receipt.TxType
		case "txs_count":
			txReceiptNotification.Receipt.TxsCount = r.Receipt.TxsCount
		}
	}
	return &txReceiptNotification
}

// Filters -
func (r *TxReceiptNotification) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (r *TxReceiptNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (r *TxReceiptNotification) GetHash() string {
	return r.Receipt.BlockHash
}

// NotificationType - feed name
func (r *TxReceiptNotification) NotificationType() FeedType {
	return TxReceiptsFeed
}
