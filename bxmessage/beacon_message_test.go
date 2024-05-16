package bxmessage

import (
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestBeaconMessagePackUnpack(t *testing.T) {
	hash := types.GenerateSHA256Hash()
	blockHash := types.GenerateSHA256Hash()
	blobBody := test.GenerateBytes(500)

	beaconMessage := NewBeaconMessage(hash, blockHash, types.BxBeaconMessageTypeEthBlob, blobBody, 3, 2, 1)
	b, err := beaconMessage.Pack(BeaconBlockProtocol)
	assert.NoError(t, err)

	var decodedBeaconMessage BeaconMessage
	err = decodedBeaconMessage.Unpack(b, BeaconBlockProtocol)
	assert.NoError(t, err)

	beaconMessage.msgType = BeaconMessageType // TODO: fix ?
	assert.Equal(t, beaconMessage, &decodedBeaconMessage)
}
