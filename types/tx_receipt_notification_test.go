package types

import (
	"encoding/json"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/stretchr/testify/assert"
)

var txReceiptMap = map[string]interface{}{
	"to":                "0x18cf158e1766ca6bdbe2719dace440121b4603b3",
	"transactionHash":   "0x4df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b6",
	"blockHash":         "0x5df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b7",
	"blockNumber":       "0xd1d827",
	"contractAddress":   "0x28cf158e1766ca6bdbe2719dace440121b4603b2",
	"cumulativeGasUsed": "0xf9389e",
	"effectiveGasPrice": "0x1c298e1cb9",
	"from":              "0x13cf158e1766ca6bdbe2719dace440121b4603b1",
	"gasUsed":           "0x5208",
	"logs":              []interface{}{"0x7cf870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b5"},
	"logsBloom":         "0x3df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b4",
	"status":            "0x1",
	"transactionIndex":  "0x64",
	"type":              "0x2",
}

var validTxReceiptParams = []string{"block_hash", "block_number", "contract_address",
	"cumulative_gas_used", "effective_gas_price", "from", "gas_used", "logs", "logs_bloom",
	"status", "to", "transaction_hash", "transaction_index", "type", "txs_count"}

func TestTxReceiptNotification(t *testing.T) {
	txReceipt := NewTxReceipt(txReceiptMap, "0x0")

	txReceiptNotification := NewTxReceiptsNotification([]*TxReceipt{txReceipt})

	txReceiptWithFields := txReceiptNotification.WithFields(validTxReceiptParams)

	ethTxReceiptWithFields, ok := txReceiptWithFields.(*TxReceiptsNotification)
	assert.True(t, ok)

	receiptJSON, err := test.MarshallJSONToMap(ethTxReceiptWithFields.Receipts[0])
	assert.NoError(t, err)

	for _, param := range validTxReceiptParams {
		_, ok = receiptJSON[param]
		assert.Equal(t, true, ok)
	}
	for k, v := range txReceiptMap {
		assert.Equal(t, v, receiptJSON[test.ToSnakeCase(k)])
	}
}

func TestTxReceiptNotificationWithoutToField(t *testing.T) {
	txReceipt := NewTxReceipt(txReceiptMap, "0x0")

	txReceiptNotification := NewTxReceiptsNotification([]*TxReceipt{txReceipt})

	txReceiptWithFields := txReceiptNotification.WithFields([]string{"transaction_hash"})

	ethTxReceiptWithFields, ok := txReceiptWithFields.(*TxReceiptsNotification)
	assert.True(t, ok)

	receiptJSON, err := test.MarshallJSONToMap(ethTxReceiptWithFields.Receipts[0])
	assert.NoError(t, err)

	_, ok = receiptJSON["to"]
	assert.Equal(t, false, ok)
	assert.Equal(t, "0x4df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b6", receiptJSON["transaction_hash"])
}

func marshallJSONToMapArray(v interface{}) ([]map[string]interface{}, error) {
	var result []map[string]interface{}

	b, err := json.Marshal(v)
	if err != nil {
		return result, err
	}

	err = json.Unmarshal(b, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

func TestContractCreationTxReceipt(t *testing.T) {
	contractCreationReceiptMap := txReceiptMap
	for k, v := range txReceiptMap {
		contractCreationReceiptMap[k] = v
	}
	contractCreationReceiptMap["to"] = nil

	txReceipt := NewTxReceipt(contractCreationReceiptMap, "0x0")

	txReceiptNotification := NewTxReceiptsNotification([]*TxReceipt{txReceipt})

	txReceiptWithFields := txReceiptNotification.WithFields([]string{"to", "from"})

	ethTxReceiptWithFields, ok := txReceiptWithFields.(*TxReceiptsNotification)
	assert.True(t, ok)

	receiptJSON, err := marshallJSONToMapArray(ethTxReceiptWithFields)
	assert.NoError(t, err)

	to, ok := receiptJSON[0]["to"]
	assert.Equal(t, true, ok)
	assert.Equal(t, nil, to)
	assert.Equal(t, "0x13cf158e1766ca6bdbe2719dace440121b4603b1", receiptJSON[0]["from"])
}
