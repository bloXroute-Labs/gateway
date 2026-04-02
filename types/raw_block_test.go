package types

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"

	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

// NewBlockPacket is the network packet for the block propagation message.
type NewBlockPacket struct {
	Block    *gethtypes.Block
	TD       *big.Int
	Sidecars bxethcommon.BlobSidecars `rlp:"optional"` // optional field for BSC
}

// Name implements the eth.Packet interface.
func (*NewBlockPacket) Name() string { return "NewBlock" }

// Kind implements the eth.Packet interface.
func (*NewBlockPacket) Kind() byte { return eth.NewBlockMsg }

func TestRawBlock(t *testing.T) {
	header := &gethtypes.Header{
		ParentHash: common.HexToHash("0x01"),
		Number:     big.NewInt(1),
		Time:       1,
		GasLimit:   10_000_000,
		GasUsed:    0,
		Difficulty: big.NewInt(1),
	}

	tx := gethtypes.NewTransaction(
		0,
		common.HexToAddress("0x0000000000000000000000000000000000000001"),
		big.NewInt(0),
		21_000,
		big.NewInt(1),
		nil,
	)

	block := gethtypes.NewBlockWithHeader(header)
	block = block.WithBody(gethtypes.Body{
		Transactions: []*gethtypes.Transaction{tx},
	})

	newBlockPacket := NewBlockPacket{
		Block:    block,
		TD:       big.NewInt(1),
		Sidecars: nil,
	}

	newBlockPacketBytes, err := rlp.EncodeToBytes(&newBlockPacket)
	require.NoError(t, err)

	rawBlock, err := ParseNewBlockPacket(bytes.NewReader(newBlockPacketBytes))
	require.NoError(t, err)

	// Basic fields propagated correctly
	require.Equal(t, block.Hash(), rawBlock.Hash())
	require.Equal(t, block.ParentHash(), rawBlock.ParentHash())
	require.Equal(t, block.NumberU64(), rawBlock.Number())
	require.Equal(t, header.Time, rawBlock.Time())
	require.Equal(t, 0, rawBlock.TotalDifficulty().Cmp(big.NewInt(1)))

	// Decoded block matches original
	decoded := rawBlock.Block()
	require.NotNil(t, decoded)
	require.Equal(t, block.Hash(), decoded.Hash())
	require.Len(t, decoded.Transactions(), 1)
	require.Equal(t, tx.Hash(), decoded.Transactions()[0].Hash())
	require.Len(t, decoded.Uncles(), 0)
}
