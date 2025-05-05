package statistics

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func TestShouldLog(t *testing.T) {
	networks := sdnmessage.BlockchainNetworks{
		1: bxmock.MockNetwork(1, "Ethereum", "1", 0.5),
		2: bxmock.MockNetwork(2, "Ethereum", "2", 50),
		3: bxmock.MockNetwork(3, "Ethereum", "3", 0.001),
		4: bxmock.MockNetwork(4, "Ethereum", "4", 0.025),
	}

	stats := newStats("localhost", "node", &networks, false)
	ft := stats.(FluentdStats)
	assert.False(t, ft.shouldLogEvent(1, generateRandHash(0xffff)))
	assert.False(t, ft.shouldLogEvent(1, generateRandHash(0x7d00)))
	assert.False(t, ft.shouldLogEvent(1, generateRandHash(0x3e80)))
	assert.False(t, ft.shouldLogEvent(1, generateRandHash(0x05dc)))
	assert.True(t, ft.shouldLogEvent(2, generateRandHash(0x05dc)))
	assert.True(t, ft.shouldLogEvent(1, generateRandHash(0x0012)))
	assert.False(t, ft.shouldLogEvent(3, generateRandHash(0x000a)))
	assert.True(t, ft.shouldLogEvent(3, generateRandHash(0x0000)))
	assert.True(t, ft.shouldLogEvent(4, generateRandHash(0x0000)))
	assert.True(t, ft.shouldLogEvent(4, generateRandHash(0x0010)))
	assert.True(t, ft.shouldLogEvent(4, generateRandHash(0x000f)))
	assert.False(t, ft.shouldLogEvent(4, generateRandHash(0x0011)))

	hash, err := types.NewSHA256HashFromString("fb0d50a5731201b9265c66444ce2d20973b4e16a540716d2d3be7f091d13b900")
	assert.Nil(t, err)
	assert.False(t, ft.shouldLogEvent(4, hash))
	hash, err = types.NewSHA256HashFromString("fb0d50a5731201b9265c66444ce2d20973b4e16a540716d2d3be7f091d130010")
	assert.Nil(t, err)
	assert.True(t, ft.shouldLogEvent(4, hash))
}

func generateRandHash(tail uint16) types.SHA256Hash {
	var hash types.SHA256Hash
	if _, err := rand.Read(hash[:]); err != nil {
		panic(err)
	}
	binary.BigEndian.PutUint16((hash)[types.SHA256HashLen-TailByteCount:], tail)

	return hash
}
