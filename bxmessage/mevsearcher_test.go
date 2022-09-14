package bxmessage

import (
	"encoding/hex"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/stretchr/testify/require"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMEVSearcherPackSuccess(t *testing.T) {
	bigIntNumber := &big.Int{}
	bigIntNumber, _ = bigIntNumber.SetString("192503000000000000000", 10)

	type fields struct {
		auth              MEVSearcherAuth
		fixture, uuid     string
		frontrunning      bool
		effectiveGasPrice big.Int
		coinbaseProfit    big.Int
		params            MEVBundleParams
		protocol          Protocol
		packPayloadLen    int
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
		{
			name: "with mev max profit builder",
			fields: fields{
				auth:              MEVSearcherAuth{"name test": "auth test"},
				fixture:           fixtures.MEVSearcherPayloadWithMEVMaxProfitBuilder,
				effectiveGasPrice: *bigIntNumber,
				coinbaseProfit:    *bigIntNumber,
				frontrunning:      true,
				uuid:              "",
				params:            []byte(`{"test":"test"}`),
				protocol:          29,
				packPayloadLen:    167,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mevSearcher, err := NewMEVSearcher(
				"eth_sendMegabundle",
				tt.fields.auth,
				tt.fields.uuid,
				tt.fields.frontrunning,
				tt.fields.effectiveGasPrice,
				tt.fields.coinbaseProfit,
				tt.fields.params,
			)
			require.NoError(t, err)
			assert.Equal(t, len(tt.fields.auth), len(mevSearcher.Auth()))

			packPayload, err := mevSearcher.Pack(tt.fields.protocol)
			require.NoError(t, err)

			err = mevSearcher.Unpack(packPayload, tt.fields.protocol)
			require.NoError(t, err)
			assert.Equal(t, string(tt.fields.params), string(mevSearcher.Params))
			assert.Equal(t, "eth_sendMegabundle", mevSearcher.Method)
			assert.Equal(t, tt.fields.uuid, mevSearcher.UUID)
			assert.Equal(t, len(tt.fields.auth), len(mevSearcher.Auth()))
			assert.Equal(t, tt.fields.frontrunning, mevSearcher.Frontrunning)
			assert.Equal(t, tt.fields.coinbaseProfit, mevSearcher.CoinbaseProfit)
			assert.Equal(t, tt.fields.effectiveGasPrice, mevSearcher.EffectiveGasPrice)
		})
	}
}

func TestMEVSearcherPackFailedAuthToLong(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	for i := 0; i < 258; i++ {
		mevSearchersAuthorization[strconv.Itoa(i)] = strconv.Itoa(i)
	}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, "", false, big.Int{}, big.Int{}, params)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("number of mev builders names %v exceeded the limit (%v)", len(mevSearchersAuthorization), maxAuthNames), err.Error())
}

func TestMEVSearcherNewFailedInvalidUUID(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	for i := 0; i < 258; i++ {
		mevSearchersAuthorization[strconv.Itoa(i)] = strconv.Itoa(i)
	}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, "invalid", false, big.Int{}, big.Int{}, params)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("number of mev builders names %v exceeded the limit (%v)", len(mevSearchersAuthorization), maxAuthNames), err.Error())
}

func TestMEVSearcherPackFailedAuthLengthNotEnough(t *testing.T) {
	mevSearchersAuthorization := MEVSearcherAuth{}
	params := []byte("content test")
	_, err := NewMEVSearcher("eth_sendMegabundle", mevSearchersAuthorization, "", false, big.Int{}, big.Int{}, params)
	require.Error(t, err)
	assert.Equal(t, "at least 1 mev builder must be present", err.Error())
}

func TestMEVSearcherUnpackSuccess(t *testing.T) {
	bigIntNumber := &big.Int{}
	bigIntNumber, _ = bigIntNumber.SetString("192503000000000000000", 10)

	type fields struct {
		auth              MEVSearcherAuth
		fixture, uuid     string
		params            MEVBundleParams
		frontrunning      bool
		effectiveGasPrice big.Int
		coinbaseProfit    big.Int
		protocol          Protocol
		packPayloadLen    int
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
		{
			name: "with mev max profit builder",
			fields: fields{
				auth:              MEVSearcherAuth{"name test": "auth test"},
				fixture:           fixtures.MEVSearcherPayloadWithMEVMaxProfitBuilder,
				uuid:              "",
				effectiveGasPrice: *bigIntNumber,
				coinbaseProfit:    *bigIntNumber,
				frontrunning:      true,
				params:            []byte(`{"test":"test"}`),
				protocol:          29,
				packPayloadLen:    167,
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
			assert.Equal(t, tt.fields.frontrunning, mevSearcher.Frontrunning)
			assert.Equal(t, tt.fields.coinbaseProfit, mevSearcher.CoinbaseProfit)
			assert.Equal(t, tt.fields.effectiveGasPrice, mevSearcher.EffectiveGasPrice)
		})
	}
}
