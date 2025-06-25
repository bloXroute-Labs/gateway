package bsc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func testPeer(writeChannelSize int, peerCount int, readTimeout time.Duration) (*Peer, *test.MsgReadWriter) {
	rw := test.NewMsgReadWriter(100, writeChannelSize, readTimeout)
	peer := NewPeer(context.Background(), Bsc2, p2p.NewPeerPipe(test.GenerateEnodeID(), fmt.Sprintf("test peer_%v", peerCount), []p2p.Cap{}, nil), rw)
	return peer, rw
}

func TestPeer_Handshake(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		peer, rw := testPeer(1, 1, time.Second)
		rw.QueueIncomingMessage(BscCapMsg, CapPacket{ProtocolVersion: Bsc2, Extra: defaultExtra})

		err := peer.Handshake()
		require.NoError(t, err)
	})

	t.Run("handshake timeout", func(t *testing.T) {
		peer, _ := testPeer(1, 1, time.Second*10)
		err := peer.Handshake()
		require.Error(t, err)
		require.Equal(t, p2p.DiscReadTimeout, err)
	})

	t.Run("handshake with wrong version", func(t *testing.T) {
		peer, rw := testPeer(1, 1, time.Second)
		rw.QueueIncomingMessage(BscCapMsg, CapPacket{ProtocolVersion: 100, Extra: defaultExtra})

		err := peer.Handshake()
		require.Error(t, err)
		require.ErrorIs(t, err, errProtocolVersionMismatch)
	})
}

func TestPeer_RequestBlocksByRange(t *testing.T) {
	peer, rw := testPeer(1, 1, time.Second)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		resp, err := peer.RequestBlocksByRange(100, common.Hash(types.GenerateSHA256Hash().Bytes()), 1)
		require.NoError(t, err)
		require.Len(t, resp, 1)
	}()

	go func() {
		defer wg.Done()

		err := Handle(nil, peer)
		assert.ErrorIs(t, err, test.ErrReadTimeout, "this is expected since we don't send any messages")
	}()

	if !rw.ExpectWrite(time.Second * 2) {
		require.Fail(t, "timed out waiting for write")
	}

	require.EqualValues(t, GetBlocksByRangeMsg, rw.WriteMessages[0].Code, "dispatcher should send GetBlocksByRangeMsg")

	for rID := range peer.dispatcher.requests {
		rw.QueueIncomingMessage(BlocksByRangeMsg, BlocksByRangePacket{
			RequestId: rID,
			Blocks: []BlockData{
				{
					Header: &ethtypes.Header{},
				},
			},
		})

		break
	}

	wg.Wait()
}
