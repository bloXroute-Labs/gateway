package bxmessage

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway/test/fixtures"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMEVSearcherPackSuccess(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{"name test": "auth test"}
	params := []byte("content test")

	mevSearcher, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, params)
	assert.NoError(t, err)
	assert.Equal(t, len(mevSearchersAuthorization), len(mevSearcher.Auth()))

	packPayload, _ := mevSearcher.Pack(0)

	err = mevSearcher.Unpack(packPayload, 0)
	assert.NoError(t, err)
	assert.Equal(t, string(params), string(mevSearcher.Params))
	assert.Equal(t, "eth_sendMegabundle", mevSearcher.Method)
	assert.Equal(t, len(mevSearchersAuthorization), len(mevSearcher.Auth()))
}

func TestMEVSearcherPackFailedAuthToLong(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	for i := 0; i < 258; i++ {
		mevSearchersAuthorization[strconv.Itoa(i)] = strconv.Itoa(i)
	}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, params)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("number of mev builders names %v exceeded the limit (%v)", len(mevSearchersAuthorization), maxAuthNames), err.Error())
}

func TestMEVSearcherPackFailedAuthLengthNotEnough(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, params)
	require.Error(t, err)
	assert.Equal(t, "at least 1 mev builder must be present", err.Error())
}

func TestMEVSearcherUnpackSuccess(t *testing.T) {
	mevSearcher := MEVSearcher{}

	buf, err := hex.DecodeString(fixtures.MEVSearcherPayload)
	assert.NoError(t, err)

	assert.NoError(t, err)
	err = mevSearcher.Unpack(buf, 0)
	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(`{"test":"test"}`), mevSearcher.Params)
	assert.Equal(t, "eth_sendMegabundle", mevSearcher.Method)
	assert.Equal(t, MEVSearcherAuth{"name test": "auth test"}, mevSearcher.Auth())
}
