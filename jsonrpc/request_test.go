package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRPCTxPayload_UnmarshalJSON(t *testing.T) {
	original := RPCTxPayload{
		Transaction:    "12312312312abacasdf",
		ValidatorsOnly: false,
	}

	singleSerialized, err := json.Marshal(original)
	assert.Nil(t, err)

	var singleResult RPCTxPayload
	err = json.Unmarshal(singleSerialized, &singleResult)
	assert.Nil(t, err)
	assert.Equal(t, original, singleResult)

	gethSerialized, err := json.Marshal([]RPCTxPayload{original})
	assert.Nil(t, err)

	var gethResult RPCTxPayload
	err = json.Unmarshal(gethSerialized, &gethResult)
	assert.Nil(t, err)
	assert.Equal(t, original, gethResult)
}

func TestRPCBatchTxPayload_UnmarshalJSON(t *testing.T) {
	original := RPCBatchTxPayload{
		Transactions:   []string{"12312312312abacasdf", "12312312312abacasdd"},
		ValidatorsOnly: false,
	}

	batchSerialized, err := json.Marshal(original)
	assert.Nil(t, err)

	var singleResult RPCBatchTxPayload
	err = json.Unmarshal(batchSerialized, &singleResult)
	assert.Nil(t, err)
	assert.Equal(t, original, singleResult)

	_, err = json.Marshal([]RPCBatchTxPayload{original})
	assert.Nil(t, err)

}
