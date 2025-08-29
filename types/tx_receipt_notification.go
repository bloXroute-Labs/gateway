package types

import (
	"encoding/json"
	"fmt"
)

const nullAddressStr = "0x"

// TxReceiptsNotification - represents a transaction receipt feed entry
// to avoid deserializing/reserializing the message from Ethereum RPC, no conversion work is done
type TxReceiptsNotification struct {
	Receipts []*TxReceipt
}

// NewTxReceiptsNotification returns a new tx receipts notification object
func NewTxReceiptsNotification(receipts []*TxReceipt) *TxReceiptsNotification {
	return &TxReceiptsNotification{Receipts: receipts}
}

// TxReceipt - represents a transaction receipt
type TxReceipt struct {
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
	BlobGasUsed       string        `json:"blobGasUsed,omitempty"`
	BlobGasPrice      string        `json:"blobGasPrice,omitempty"`
}

// NewTxReceipt returns a new tx receipt object created from a map
func NewTxReceipt(receiptMap map[string]interface{}, txsCount string) *TxReceipt {
	txReceipt := TxReceipt{}

	blockHash, ok := receiptMap["blockHash"]
	if ok {
		txReceipt.BlockHash = blockHash.(string)
	}

	blockNumber, ok := receiptMap["blockNumber"]
	if ok {
		txReceipt.BlockNumber = blockNumber.(string)
	}

	contractAddress, ok := receiptMap["contractAddress"]
	if ok {
		txReceipt.ContractAddress = contractAddress
	}

	cumulativeGasUsed, ok := receiptMap["cumulativeGasUsed"]
	if ok {
		txReceipt.CumulativeGasUsed = cumulativeGasUsed.(string)
	}

	effectiveGasPrice, ok := receiptMap["effectiveGasPrice"]
	if ok {
		txReceipt.EffectiveGasPrice = effectiveGasPrice.(string)
	}

	from, ok := receiptMap["from"]
	if ok {
		txReceipt.From = from
	}

	gasUsed, ok := receiptMap["gasUsed"]
	if ok {
		txReceipt.GasUsed = gasUsed.(string)
	}

	logs, ok := receiptMap["logs"]
	if ok {
		txReceipt.Logs = logs.([]interface{})
	}

	logsBloom, ok := receiptMap["logsBloom"]
	if ok {
		txReceipt.LogsBloom = logsBloom.(string)
	}

	status, ok := receiptMap["status"]
	if ok {
		txReceipt.Status = status.(string)
	}

	to, ok := receiptMap["to"]
	if ok {
		txReceipt.To = to
	}

	transactionHash, ok := receiptMap["transactionHash"]
	if ok {
		txReceipt.TransactionHash = transactionHash.(string)
	}

	transactionIndex, ok := receiptMap["transactionIndex"]
	if ok {
		txReceipt.TransactionIndex = transactionIndex.(string)
	}

	txType, ok := receiptMap["type"]
	if ok {
		txReceipt.TxType = txType.(string)
	}

	txReceipt.TxsCount = txsCount

	blobGasUsed, ok := receiptMap["blobGasUsed"]
	if ok {
		txReceipt.BlobGasUsed = blobGasUsed.(string)
	}

	blobGasPrice, ok := receiptMap["blobGasPrice"]
	if ok {
		txReceipt.BlobGasPrice = blobGasPrice.(string)
	}

	return &txReceipt
}

// MarshalJSON formats txReceiptNotification, including nil "to" field if requested
func (r *TxReceipt) marshalJSON() ([]byte, error) {
	marshalled, err := json.Marshal(r)
	if r.To != nullAddressStr {
		return marshalled, err
	}
	var mapWithNilToField map[string]interface{}
	json.Unmarshal(marshalled, &mapWithNilToField)
	mapWithNilToField["to"] = nil
	return json.Marshal(mapWithNilToField)
}

// MarshalJSON formats txReceiptsNotification, including nil "to" field if requested
func (r *TxReceiptsNotification) MarshalJSON() ([]byte, error) {
	if r.Receipts == nil {
		return nil, fmt.Errorf("TxReceiptsNotification: Receipt is nil")
	}

	// Create a temporary slice to hold marshaled JSON data for each receipt
	marshalledReceipts := make([]json.RawMessage, len(r.Receipts))

	for i, receipt := range r.Receipts {
		marshalled, err := receipt.marshalJSON()
		if err != nil {
			return nil, err
		}

		marshalledReceipts[i] = marshalled
	}

	return json.Marshal(marshalledReceipts)
}

// WithFields -
func (r *TxReceiptsNotification) WithFields(fields []string) Notification {
	txReceiptsNotification := TxReceiptsNotification{Receipts: []*TxReceipt{}}

	for _, receipt := range r.Receipts {
		newReceipt := &TxReceipt{}

		for _, param := range fields {
			switch param {
			case "block_hash":
				newReceipt.BlockHash = receipt.BlockHash
			case "block_number":
				newReceipt.BlockNumber = receipt.BlockNumber
			case "contract_address":
				newReceipt.ContractAddress = receipt.ContractAddress
			case "cumulative_gas_used":
				newReceipt.CumulativeGasUsed = receipt.CumulativeGasUsed
			case "effective_gas_price":
				newReceipt.EffectiveGasPrice = receipt.EffectiveGasPrice
			case "from":
				newReceipt.From = receipt.From
			case "gas_used":
				newReceipt.GasUsed = receipt.GasUsed
			case "logs":
				newReceipt.Logs = receipt.Logs
			case "logs_bloom":
				newReceipt.LogsBloom = receipt.LogsBloom
			case "status":
				newReceipt.Status = receipt.Status
			case "to":
				newReceipt.To = receipt.To
				if receipt.To == nil {
					newReceipt.To = nullAddressStr
				}
			case "transaction_hash":
				newReceipt.TransactionHash = receipt.TransactionHash
			case "transaction_index":
				newReceipt.TransactionIndex = receipt.TransactionIndex
			case "type":
				newReceipt.TxType = receipt.TxType
			case "txs_count":
				newReceipt.TxsCount = receipt.TxsCount
			case "blob_gas_used":
				newReceipt.BlobGasUsed = receipt.BlobGasUsed
			case "blob_gas_price":
				newReceipt.BlobGasPrice = receipt.BlobGasPrice
			}
		}

		txReceiptsNotification.Receipts = append(txReceiptsNotification.Receipts, newReceipt)
	}

	return &txReceiptsNotification
}

// Filters -
func (r *TxReceiptsNotification) Filters() map[string]interface{} {
	return nil
}

// LocalRegion -
func (r *TxReceiptsNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (r *TxReceiptsNotification) GetHash() string {
	if len(r.Receipts) == 0 {
		return ""
	}
	return r.Receipts[0].BlockHash
}

// NotificationType - feed name
func (r *TxReceiptsNotification) NotificationType() FeedType {
	return TxReceiptsFeed
}
