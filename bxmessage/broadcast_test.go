package bxmessage

import (
	"encoding/hex"
	"github.com/bloXroute-Labs/gateway/test"
	"github.com/bloXroute-Labs/gateway/test/fixtures"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	networkNum = types.NetworkNum(5)
)

func TestBroadcastPackUnpack(t *testing.T) {
	blockHash := types.GenerateSHA256Hash()
	blockBody := test.GenerateBytes(500)
	broadcast := NewBlockBroadcast(blockHash, blockBody, types.ShortIDList{}, networkNum)

	b, err := broadcast.Pack(0)
	assert.Nil(t, err)

	var decodedBroadcast Broadcast
	err = decodedBroadcast.Unpack(b, 0)
	assert.Nil(t, err)

	assert.Equal(t, blockHash, decodedBroadcast.BlockHash())
	assert.Equal(t, blockBody, decodedBroadcast.Block())
	assert.Equal(t, networkNum, decodedBroadcast.GetNetworkNum())
}

func TestBroadcastUnpackFixtureWithShortIDs(t *testing.T) {
	b, _ := hex.DecodeString(fixtures.BroadcastMessageWithShortIDs)
	h, _ := types.NewSHA256HashFromString(fixtures.BroadcastShortIDsMessageHash)

	var broadcast Broadcast
	err := broadcast.Unpack(b, 0)
	assert.Nil(t, err)
	assert.Equal(t, networkNum, broadcast.networkNumber)
	assert.Equal(t, h, broadcast.BlockHash())
	assert.Equal(t, 2, len(broadcast.ShortIDs()))

	encodedBroadcast, err := broadcast.Pack(0)
	assert.Nil(t, err)
	assert.Equal(t, b, encodedBroadcast)
}

func TestBroadcastUnpackFixtureEmptyBlock(t *testing.T) {
	b, _ := hex.DecodeString(fixtures.BroadcastEmptyBlock)
	h, _ := types.NewSHA256HashFromString(fixtures.BroadcastEmptyBlockHash)

	var broadcast Broadcast
	err := broadcast.Unpack(b, 0)
	assert.Nil(t, err)
	assert.Equal(t, networkNum, broadcast.networkNumber)
	assert.Equal(t, h, broadcast.BlockHash())

	encodedBroadcast, err := broadcast.Pack(0)
	assert.Nil(t, err)
	assert.Equal(t, b, encodedBroadcast)
}
