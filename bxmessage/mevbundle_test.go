package bxmessage

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway/test/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
)

func TestMEVBundlePackSuccess(t *testing.T) {
	mevMinerNames := MEVMinerNames{"test miner1", "test miner2"}
	params := []byte("test params")

	mevMinerBundle, err := NewMEVBundle("eth_sendMegabundle", mevMinerNames, params)
	assert.NoError(t, err)
	assert.Equal(t, mevMinerNames, mevMinerBundle.Names())

	packPayload, _ := mevMinerBundle.Pack(0)

	err = mevMinerBundle.Unpack(packPayload, 0)
	assert.NoError(t, err)
	assert.Equal(t, string(params), string(mevMinerBundle.Params))
	assert.Equal(t, len(mevMinerNames), len(mevMinerBundle.Names()))
	assert.Equal(t, "eth_sendMegabundle", mevMinerBundle.Method)
	assert.Equal(t, 131, len(packPayload))
}

func TestMEVBundleNewFailedMinerNamesToLong(t *testing.T) {
	mevSearchersAuthorization := MEVMinerNames{}
	for i := 0; i < 258; i++ {
		mevSearchersAuthorization = append(mevSearchersAuthorization, strconv.Itoa(i))
	}
	params := []byte("content test")
	_, err := NewMEVBundle("eth_sendMegabundle", mevSearchersAuthorization, params)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("number of mev builders names %v exceeded the limit (%v)", len(mevSearchersAuthorization), mevBundleNameMaxSize), err.Error())
}

func TestMEVBundleUnpackSuccess(t *testing.T) {
	mevSearcher := MEVBundle{}
	buf, err := hex.DecodeString(fixtures.MEVBundlePayload)
	assert.NoError(t, err)

	err = mevSearcher.Unpack(buf, 0)
	require.NoError(t, err)
	assert.Equal(t, json.RawMessage(`{"test":"test"}`), mevSearcher.Params)
	assert.Equal(t, "eth_sendBundle", mevSearcher.Method)
	assert.Equal(t, MEVMinerNames{"test miner1", "test miner2"}, mevSearcher.Names())
}
