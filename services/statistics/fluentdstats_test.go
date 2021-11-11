package statistics

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
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
	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0xffff), 1, 0, time.Now()),
		),
	)
	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x7d00), 1, 0, time.Now()),
		),
	)
	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x3e80), 1, 0, time.Now()),
		),
	)
	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x05dc), 1, 0, time.Now()),
		),
	)
	assert.True(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x0012), 1, 0, time.Now()),
		),
	)

	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x000a), 3, 0, time.Now()),
		),
	)
	assert.True(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x0000), 3, 0, time.Now()),
		),
	)

	assert.True(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x0000), 4, 0, time.Now()),
		),
	)

	assert.True(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x0010), 4, 0, time.Now()),
		),
	)

	assert.True(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x000f), 4, 0, time.Now()),
		),
	)

	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(generateRandTxHash(0x0011), 4, 0, time.Now()),
		),
	)

	hash, _ := types.NewSHA256HashFromString("fb0d50a5731201b9265c66444ce2d20973b4e16a540716d2d3be7f091d13b900")
	assert.False(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(hash, 4, 0, time.Now()),
		),
	)
	hash, _ = types.NewSHA256HashFromString("fb0d50a5731201b9265c66444ce2d20973b4e16a540716d2d3be7f091d130010")
	assert.True(
		t, ft.shouldLogEventForTx(
			types.NewBxTransaction(hash, 4, 0, time.Now()),
		),
	)
}

func generateRandTxHash(tail uint16) types.SHA256Hash {
	var hash types.SHA256Hash
	if _, err := rand.Read(hash[:]); err != nil {
		panic(err)
	}
	binary.BigEndian.PutUint16((hash)[types.SHA256HashLen-TailByteCount:], tail)

	return hash
}
