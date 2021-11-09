package bxmessage

import (
	"encoding/hex"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	"github.com/bloXroute-Labs/bxgateway-private-go/test"
	"github.com/bloXroute-Labs/bxgateway-private-go/test/fixtures"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTxs_PackUnpack(t *testing.T) {
	txs := Txs{
		items: []TxsItem{
			{
				Hash:    types.GenerateSHA256Hash(),
				Content: test.GenerateBytes(100),
				ShortID: 1,
			},
			{
				Hash:    types.GenerateSHA256Hash(),
				Content: test.GenerateBytes(150),
				ShortID: 2,
			},
		},
	}

	b, err := txs.Pack(0)
	assert.Nil(t, err)

	var unpackedTxs Txs
	err = unpackedTxs.Unpack(b, 0)
	assert.Nil(t, err)

	assert.Equal(t, txs, unpackedTxs)
}

func TestTxs_Fixture(t *testing.T) {
	expectedHash1, _ := types.NewSHA256HashFromString(fixtures.TxsHash1)
	expectedContent1, _ := hex.DecodeString(fixtures.TxsContent1)
	expectedHash2, _ := types.NewSHA256HashFromString(fixtures.TxsHash2)
	expectedContent2, _ := hex.DecodeString(fixtures.TxsContent2)

	b, _ := hex.DecodeString(fixtures.TxsMessage)

	var txs Txs
	err := txs.Unpack(b, 0)
	assert.Nil(t, err)

	items := txs.Items()
	assert.Equal(t, 2, len(items))

	assert.Equal(t, expectedHash1, items[0].Hash)
	assert.Equal(t, types.TxContent(expectedContent1), items[0].Content)
	assert.Equal(t, types.ShortID(fixtures.TxsShortID1), items[0].ShortID)
	assert.Equal(t, expectedHash2, items[1].Hash)
	assert.Equal(t, types.TxContent(expectedContent2), items[1].Content)
	assert.Equal(t, types.ShortID(fixtures.TxsShortID2), items[1].ShortID)

	output, err := txs.Pack(0)
	assert.Nil(t, err)
	assert.Equal(t, b, output)
}
