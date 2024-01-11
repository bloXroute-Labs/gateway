package bxmessage

import (
	"encoding/hex"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/stretchr/testify/assert"
)

const (
	networkNum = types.NetworkNum(5)
)

func TestBroadcastPackUnpack(t *testing.T) {
	blockHash := types.GenerateSHA256Hash()
	beaconBlockHash := types.GenerateSHA256Hash()
	blockBody := test.GenerateBytes(500)
	broadcast := NewBlockBroadcast(blockHash, beaconBlockHash, types.BxBlockTypeBeaconBellatrix, blockBody, types.ShortIDList{}, networkNum)

	b, err := broadcast.Pack(BeaconBlockProtocol)
	assert.NoError(t, err)

	var decodedBroadcast Broadcast
	err = decodedBroadcast.Unpack(b, BeaconBlockProtocol)
	assert.NoError(t, err)

	assert.Equal(t, blockHash, decodedBroadcast.Hash())
	assert.Equal(t, beaconBlockHash, decodedBroadcast.BeaconHash())
	assert.Equal(t, blockBody, decodedBroadcast.Block())
	assert.Equal(t, networkNum, decodedBroadcast.GetNetworkNum())
}

func TestBroadcastUnpackFixtureWithShortIDs(t *testing.T) {
	b, _ := hex.DecodeString(fixtures.BroadcastMessageWithShortIDs)
	h, _ := types.NewSHA256HashFromString(fixtures.BroadcastShortIDsMessageHash)

	var broadcast Broadcast
	err := broadcast.Unpack(b, 0)
	assert.NoError(t, err)
	assert.Equal(t, networkNum, broadcast.networkNumber)
	assert.Equal(t, h, broadcast.Hash())
	assert.Equal(t, 2, len(broadcast.ShortIDs()))

	encodedBroadcast, err := broadcast.Pack(0)
	assert.NoError(t, err)
	assert.Equal(t, b, encodedBroadcast)
}

func TestBroadcastUnpackFixtureEmptyBlock(t *testing.T) {
	b, _ := hex.DecodeString(fixtures.BroadcastEmptyBlock)
	h, _ := types.NewSHA256HashFromString(fixtures.BroadcastEmptyBlockHash)

	var broadcast Broadcast
	err := broadcast.Unpack(b, 0)
	assert.NoError(t, err)
	assert.Equal(t, networkNum, broadcast.networkNumber)
	assert.Equal(t, h, broadcast.Hash())

	encodedBroadcast, err := broadcast.Pack(0)
	assert.NoError(t, err)
	assert.Equal(t, b, encodedBroadcast)
}
