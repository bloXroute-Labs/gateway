package bxmessage

import (
	"encoding/hex"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMEVSearcherPackSuccess(t *testing.T) {
	type fields struct {
		auth           MEVSearcherAuth
		fixture, uuid  string
		params         MEVBundleParams
		protocol       Protocol
		packPayloadLen int
	}

	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "without uuid",
			fields: fields{
				auth:           MEVSearcherAuth{"name test": "auth test"},
				fixture:        fixtures.MEVSearcherPayload,
				uuid:           "",
				params:         []byte(`{"test":"test"}`),
				protocol:       0,
				packPayloadLen: 131,
			},
		},
		{
			name: "with uuid",
			fields: fields{
				auth:           MEVSearcherAuth{"name test": "auth test"},
				fixture:        fixtures.MEVSearcherPayloadWithUUID,
				uuid:           "c40df8ec-844d-4887-8129-27bb80812680",
				params:         []byte(`{"test":"test"}`),
				protocol:       27,
				packPayloadLen: 167,
			},
		},
		{
			name: "with empty uuid",
			fields: fields{
				auth:           MEVSearcherAuth{"name test": "auth test"},
				fixture:        fixtures.MEVSearcherPayloadWithEmptyUUID,
				uuid:           "",
				params:         []byte(`{"test":"test"}`),
				protocol:       27,
				packPayloadLen: 167,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mevSearcher, err := NewMEVSearcher("eth_sendMegabundle", tt.fields.auth, tt.fields.uuid, tt.fields.params)
			require.NoError(t, err)
			assert.Equal(t, len(tt.fields.auth), len(mevSearcher.Auth()))

			packPayload, _ := mevSearcher.Pack(0)

			err = mevSearcher.Unpack(packPayload, 0)
			require.NoError(t, err)
			assert.Equal(t, string(tt.fields.params), string(mevSearcher.Params))
			assert.Equal(t, "eth_sendMegabundle", mevSearcher.Method)
			assert.Equal(t, len(tt.fields.auth), len(mevSearcher.Auth()))
		})
	}
}

func TestMEVSearcherPackFailedAuthToLong(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	for i := 0; i < 258; i++ {
		mevSearchersAuthorization[strconv.Itoa(i)] = strconv.Itoa(i)
	}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, "", params)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("number of mev builders names %v exceeded the limit (%v)", len(mevSearchersAuthorization), maxAuthNames), err.Error())
}

func TestMEVSearcherNewFailedInvalidUUID(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	for i := 0; i < 258; i++ {
		mevSearchersAuthorization[strconv.Itoa(i)] = strconv.Itoa(i)
	}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, "invalid", params)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("number of mev builders names %v exceeded the limit (%v)", len(mevSearchersAuthorization), maxAuthNames), err.Error())
}

func TestMEVSearcherPackFailedAuthLengthNotEnough(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, "", params)
	require.Error(t, err)
	assert.Equal(t, "at least 1 mev builder must be present", err.Error())
}

func TestMEVSearcherUnpackSuccess(t *testing.T) {
	type fields struct {
		auth           MEVSearcherAuth
		fixture, uuid  string
		params         MEVBundleParams
		protocol       Protocol
		packPayloadLen int
	}

	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "without uuid",
			fields: fields{
				auth:           MEVSearcherAuth{"name test": "auth test"},
				fixture:        fixtures.MEVSearcherPayload,
				uuid:           "",
				params:         []byte(`{"test":"test"}`),
				protocol:       0,
				packPayloadLen: 131,
			},
		},
		{
			name: "with uuid",
			fields: fields{
				auth:           MEVSearcherAuth{"name test": "auth test"},
				fixture:        fixtures.MEVSearcherPayloadWithUUID,
				uuid:           "c40df8ec-844d-4887-8129-27bb80812680",
				params:         []byte(`{"test":"test"}`),
				protocol:       27,
				packPayloadLen: 167,
			},
		},
		{
			name: "with empty uuid",
			fields: fields{
				auth:           MEVSearcherAuth{"name test": "auth test"},
				fixture:        fixtures.MEVSearcherPayloadWithEmptyUUID,
				uuid:           "",
				params:         []byte(`{"test":"test"}`),
				protocol:       27,
				packPayloadLen: 167,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mevSearcher := MEVSearcher{}

			buf, err := hex.DecodeString(tt.fields.fixture)
			require.NoError(t, err)

			err = mevSearcher.Unpack(buf, tt.fields.protocol)
			require.NoError(t, err)
			assert.Equal(t, string(tt.fields.params), string(mevSearcher.Params))
			assert.Equal(t, "eth_sendMegabundle", mevSearcher.Method)
			assert.Equal(t, tt.fields.auth, mevSearcher.Auth())
		})
	}
}
