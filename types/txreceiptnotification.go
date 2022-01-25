package types

import (
	"encoding/json"
)

const nullAddressStr = "0x"

// TxReceiptNotification - represents a transaction receipt feed entry
// to avoid deserializing/reserializing the message from Ethereum RPC, no conversion work is done
type TxReceiptNotification struct {
	receipt txReceipt
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
}

// NewTxReceiptNotification returns a new TxReceiptNotification
func NewTxReceiptNotification(txReceipt map[string]interface{}) *TxReceiptNotification {
	txReceiptNotification := TxReceiptNotification{}

	blockHash, ok := txReceipt["blockHash"]
	if ok {
		txReceiptNotification.receipt.BlockHash = blockHash.(string)
	}

	blockNumber, ok := txReceipt["blockNumber"]
	if ok {
		txReceiptNotification.receipt.BlockNumber = blockNumber.(string)
	}

	contractAddress, ok := txReceipt["contractAddress"]
	if ok {
		txReceiptNotification.receipt.ContractAddress = contractAddress
	}

	cumulativeGasUsed, ok := txReceipt["cumulativeGasUsed"]
	if ok {
		txReceiptNotification.receipt.CumulativeGasUsed = cumulativeGasUsed.(string)
	}

	effectiveGasPrice, ok := txReceipt["effectiveGasPrice"]
	if ok {
		txReceiptNotification.receipt.EffectiveGasPrice = effectiveGasPrice.(string)
	}

	from, ok := txReceipt["from"]
	if ok {
		txReceiptNotification.receipt.From = from
	}

	gasUsed, ok := txReceipt["gasUsed"]
	if ok {
		txReceiptNotification.receipt.GasUsed = gasUsed.(string)
	}

	logs, ok := txReceipt["logs"]
	if ok {
		txReceiptNotification.receipt.Logs = logs.([]interface{})
	}

	logsBloom, ok := txReceipt["logsBloom"]
	if ok {
		txReceiptNotification.receipt.LogsBloom = logsBloom.(string)
	}

	status, ok := txReceipt["status"]
	if ok {
		txReceiptNotification.receipt.Status = status.(string)
	}

	to, ok := txReceipt["to"]
	if ok {
		txReceiptNotification.receipt.To = to
	}

	transactionHash, ok := txReceipt["transactionHash"]
	if ok {
		txReceiptNotification.receipt.TransactionHash = transactionHash.(string)
	}

	transactionIndex, ok := txReceipt["transactionIndex"]
	if ok {
		txReceiptNotification.receipt.TransactionIndex = transactionIndex.(string)
	}

	txType, ok := txReceipt["type"]
	if ok {
		txReceiptNotification.receipt.TxType = txType.(string)
	}

	return &txReceiptNotification
}

// MarshalJSON formats txReceiptNotification, including nil "to" field if requested
func (r *TxReceiptNotification) MarshalJSON() ([]byte, error) {
	marshalled, err := json.Marshal(r.receipt)
	if r.receipt.To != nullAddressStr {
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
			txReceiptNotification.receipt.BlockHash = r.receipt.BlockHash
		case "block_number":
			txReceiptNotification.receipt.BlockNumber = r.receipt.BlockNumber
		case "contract_address":
			txReceiptNotification.receipt.ContractAddress = r.receipt.ContractAddress
		case "cumulative_gas_used":
			txReceiptNotification.receipt.CumulativeGasUsed = r.receipt.CumulativeGasUsed
		case "effective_gas_price":
			txReceiptNotification.receipt.EffectiveGasPrice = r.receipt.EffectiveGasPrice
		case "from":
			txReceiptNotification.receipt.From = r.receipt.From
		case "gas_used":
			txReceiptNotification.receipt.GasUsed = r.receipt.GasUsed
		case "logs":
			txReceiptNotification.receipt.Logs = r.receipt.Logs
		case "logs_bloom":
			txReceiptNotification.receipt.LogsBloom = r.receipt.LogsBloom
		case "status":
			txReceiptNotification.receipt.Status = r.receipt.Status
		case "to":
			txReceiptNotification.receipt.To = r.receipt.To
			if r.receipt.To == nil {
				txReceiptNotification.receipt.To = nullAddressStr
			}
		case "transaction_hash":
			txReceiptNotification.receipt.TransactionHash = r.receipt.TransactionHash
		case "transaction_index":
			txReceiptNotification.receipt.TransactionIndex = r.receipt.TransactionIndex
		case "type":
			txReceiptNotification.receipt.TxType = r.receipt.TxType
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
	return r.receipt.BlockHash
}

// NotificationType - feed name
func (r *TxReceiptNotification) NotificationType() FeedType {
	return TxReceiptsFeed
}
