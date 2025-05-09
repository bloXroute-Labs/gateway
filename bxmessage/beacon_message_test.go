package bxmessage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func TestBeaconMessagePackUnpack(t *testing.T) {
	hash := types.GenerateSHA256Hash()
	blockHash := types.GenerateSHA256Hash()
	blobBody := test.GenerateBytes(500)

	beaconMessage := NewBeaconMessage(hash, blockHash, types.BxBeaconMessageTypeEthBlob, blobBody, 3, 2, 1)
	b, err := beaconMessage.Pack(MinProtocol)
	assert.NoError(t, err)

	var decodedBeaconMessage BeaconMessage
	err = decodedBeaconMessage.Unpack(b, MinProtocol)
	assert.NoError(t, err)

	beaconMessage.msgType = BeaconMessageType // TODO: fix ?
	assert.Equal(t, beaconMessage, &decodedBeaconMessage)
}
