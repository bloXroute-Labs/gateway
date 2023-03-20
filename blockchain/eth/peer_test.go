package eth

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/stretchr/testify/assert"
)

func testPeer(writeChannelSize int, peerCount int) (*Peer, *test.MsgReadWriter, *utils.MockClock) {
	clock := &utils.MockClock{}
	rw := test.NewMsgReadWriter(100, writeChannelSize)
	peer := newPeer(context.Background(), p2p.NewPeerPipe(test.GenerateEnodeID(), fmt.Sprintf("test peer_%v", peerCount), []p2p.Cap{}, nil), rw, bxmessage.CurrentProtocol, clock, 1)
	return peer, rw, clock
}

func genForkID() [4]byte {
	forkID := make([]byte, 4)
	rand.Read(forkID)

	return *(*[4]byte)(forkID) // convert slice to array
}

func TestPeer_Handshake(t *testing.T) {
	networkNum := 1

	forkID1 := genForkID()
	forkID2 := genForkID()
	forkID3 := genForkID() // not in blockchain network

	executionLayerForks := []string{
		base64.StdEncoding.EncodeToString(forkID1[:]),
		base64.StdEncoding.EncodeToString(forkID2[:]),
	}

	peer, rw, _ := testPeer(-1, 1)

	peerStatus := eth.StatusPacket{
		ProtocolVersion: 1,
		NetworkID:       1,
		TD:              big.NewInt(10),
		Head:            common.Hash{1, 2, 3},
		Genesis:         common.Hash{2, 3, 4},
		ForkID:          forkid.ID{Hash: forkID1},
	}

	// matching parameters
	rw.QueueIncomingMessage(eth.StatusMsg, peerStatus)
	ps, err := peer.Handshake(1, uint64(networkNum), new(big.Int), common.Hash{1, 2, 3}, common.Hash{2, 3, 4}, executionLayerForks)
	assert.Nil(t, err)
	assert.Equal(t, peerStatus, *ps)

	// outgoing status message enqueued
	assert.Equal(t, 1, len(rw.WriteMessages))
	assert.Equal(t, uint64(eth.StatusMsg), rw.WriteMessages[0].Code)

	// version mismatch
	rw.QueueIncomingMessage(eth.StatusMsg, peerStatus)
	_, err = peer.Handshake(0, uint64(networkNum), new(big.Int), common.Hash{1, 2, 3}, common.Hash{2, 3, 4}, executionLayerForks)
	assert.NotNil(t, err)

	// network mismatch
	rw.QueueIncomingMessage(eth.StatusMsg, peerStatus)
	_, err = peer.Handshake(1, 0, new(big.Int), common.Hash{1, 2, 3}, common.Hash{2, 3, 4}, executionLayerForks)
	assert.NotNil(t, err)

	// head mismatch is ok
	rw.QueueIncomingMessage(eth.StatusMsg, peerStatus)
	_, err = peer.Handshake(1, uint64(networkNum), new(big.Int), common.Hash{3, 4, 5}, common.Hash{2, 3, 4}, executionLayerForks)
	assert.Nil(t, err)

	// genesis mismatch
	rw.QueueIncomingMessage(eth.StatusMsg, peerStatus)
	_, err = peer.Handshake(1, uint64(networkNum), new(big.Int), common.Hash{1, 2, 3}, common.Hash{3, 3, 4}, executionLayerForks)
	assert.NotNil(t, err)

	// forkID missmatch
	rw.QueueIncomingMessage(eth.StatusMsg, eth.StatusPacket{
		ProtocolVersion: 1,
		NetworkID:       1,
		TD:              big.NewInt(10),
		Head:            common.Hash{1, 2, 3},
		Genesis:         common.Hash{2, 3, 4},
		ForkID:          forkid.ID{Hash: forkID3},
	})
	_, err = peer.Handshake(1, uint64(networkNum), new(big.Int), common.Hash{1, 2, 3}, common.Hash{2, 3, 4}, executionLayerForks)
	assert.NotNil(t, err)
}

func TestPeer_SendNewBlock(t *testing.T) {
	var (
		msg p2p.Msg
		err error
	)

	peer, rw, _ := testPeer(1, 1)
	maxWriteTimeout := time.Millisecond // to allow for blockLoop goroutine to write to buffer
	clock := peer.clock.(*utils.MockClock)
	go peer.Start()

	block1a := bxmock.NewEthBlock(1, common.Hash{})
	block1b := bxmock.NewEthBlock(1, common.Hash{})
	block2a := bxmock.NewEthBlock(2, block1a.Hash())
	block2b := bxmock.NewEthBlock(2, block1b.Hash())
	block3 := bxmock.NewEthBlock(3, block2b.Hash())
	block4 := bxmock.NewEthBlock(4, block3.Hash())
	block5a := bxmock.NewEthBlock(5, common.Hash{})
	block5b := bxmock.NewEthBlock(5, block4.Hash())

	peer.confirmedHead = blockRef{0, block1a.ParentHash()}
	peer.QueueNewBlock(block1a, big.NewInt(10))

	// block 1a instantly sent (first block)
	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(rw.WriteMessages))

	msg = rw.WriteMessages[0]
	var blockPacket1 eth.NewBlockPacket
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)
	err = msg.Decode(&blockPacket1)
	assert.Nil(t, err)
	assert.Equal(t, block1a.Hash(), blockPacket1.Block.Hash())

	// confirm block 1b
	peer.UpdateHead(1, block1b.Hash())

	// block 1b ignored since stale
	peer.QueueNewBlock(block1b, big.NewInt(10))
	assert.False(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(rw.WriteMessages))

	// block 3 queued for a while
	peer.QueueNewBlock(block3, big.NewInt(10))
	assert.False(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(rw.WriteMessages))

	// block 2b will be instantly sent, 2a will never be sent (1b was the confirmation)
	peer.QueueNewBlock(block2a, big.NewInt(10))
	peer.QueueNewBlock(block2b, big.NewInt(10))

	// block 2b instantly sent
	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 2, len(rw.WriteMessages))

	var blockPacket2 eth.NewBlockPacket
	msg = rw.WriteMessages[1]
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)
	err = msg.Decode(&blockPacket2)
	assert.Nil(t, err)
	assert.Equal(t, block2b.Hash(), blockPacket2.Block.Hash())

	peer.QueueNewBlock(block4, big.NewInt(10))
	peer.QueueNewBlock(block5a, big.NewInt(10))
	peer.QueueNewBlock(block5b, big.NewInt(10))

	// confirm block 3, so skip block 3 and go directly to 4
	peer.UpdateHead(3, block3.Hash())
	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 3, len(rw.WriteMessages))

	var blockPacket4 eth.NewBlockPacket
	msg = rw.WriteMessages[2]
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)
	err = msg.Decode(&blockPacket4)
	assert.Nil(t, err)
	assert.Equal(t, block4.Hash(), blockPacket4.Block.Hash())

	// next block will never be released without confirmation
	clock.IncTime(100 * time.Second)
	assert.False(t, rw.ExpectWrite(maxWriteTimeout))

	// confirm block 4, 5b should be released (even though it's queued second at height 5)
	peer.UpdateHead(4, block4.Hash())

	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 4, len(rw.WriteMessages))

	var blockPacket5 eth.NewBlockPacket
	msg = rw.WriteMessages[3]
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)
	err = msg.Decode(&blockPacket5)
	assert.Nil(t, err)
	assert.Equal(t, block5b.Hash(), blockPacket5.Block.Hash())
}

func TestPeer_SendNewBlock_HeadUpdates(t *testing.T) {
	var (
		msg p2p.Msg
		err error
	)

	peer, rw, _ := testPeer(1, 1)
	maxWriteTimeout := time.Millisecond // to allow for blockLoop goroutine to write to buffer
	go peer.Start()

	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3 := bxmock.NewEthBlock(3, block2.Hash())
	block4a := bxmock.NewEthBlock(4, block3.Hash())
	block4b := bxmock.NewEthBlock(4, block3.Hash())
	block5b := bxmock.NewEthBlock(5, block4b.Hash())

	peer.confirmedHead = blockRef{1, block1.Hash()}

	peer.QueueNewBlock(block3, big.NewInt(30))
	peer.QueueNewBlock(block4a, big.NewInt(41))
	peer.QueueNewBlock(block4b, big.NewInt(42))
	peer.QueueNewBlock(block5b, big.NewInt(52))

	// process all queue entries first
	time.Sleep(time.Millisecond)

	// queue before 5b should be cleared out
	peer.UpdateHead(4, block4a.Hash())
	assert.False(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(peer.queuedBlocks))

	// 5b queued since wrong parent, but once 4b confirmed should be released
	peer.UpdateHead(4, block4b.Hash())

	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(rw.WriteMessages))

	var blockPacket eth.NewBlockPacket
	msg = rw.WriteMessages[0]
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)
	err = msg.Decode(&blockPacket)
	assert.Nil(t, err)
	assert.Equal(t, block5b.Hash(), blockPacket.Block.Hash())
}

func TestPeer_RequestBlockHeaderNonBlocking(t *testing.T) {
	var (
		msg p2p.Msg
		err error
	)

	peer, rw, _ := testPeer(-1, 1)
	peer.version = eth.ETH66

	err = peer.RequestBlockHeader(common.Hash{})
	assert.Nil(t, err)

	assert.Equal(t, 1, len(rw.WriteMessages))

	var getHeaders eth.GetBlockHeadersPacket66
	msg = rw.WriteMessages[0]
	err = msg.Decode(&getHeaders)
	assert.Nil(t, err)

	requestID := getHeaders.RequestId
	rw.QueueIncomingMessage(eth.BlockHeadersMsg, eth.BlockHeadersPacket66{
		RequestId:          requestID,
		BlockHeadersPacket: nil,
	})

	// should not block, since no response needed
	handled, err := peer.NotifyResponse66(requestID, nil)
	assert.False(t, handled)
	assert.Nil(t, err)
}

func TestPeer_BSC_SendFutureBlock_Pass(t *testing.T) {
	peer, rw, _ := testPeer(1, 1)
	peer.chainID = bxgateway.BSCChainID
	maxWriteTimeout := time.Millisecond // to allow for blockLoop goroutine to write to buffer
	mockClock := peer.clock.(*utils.MockClock)
	go peer.Start()

	block1a := bxmock.NewEthBlock(1, common.Hash{})
	peer.confirmedHead = blockRef{0, block1a.ParentHash()}
	// block 1a will be sent, within the range of limit
	mockClock.SetTime(time.Unix(int64(block1a.Time()), 0))
	peer.QueueNewBlock(block1a, big.NewInt(10))

	// block 1a should be sent immediately
	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(rw.WriteMessages))
}

func TestPeer_BSC_SendFutureBlock_Delay(t *testing.T) {
	peer, rw, _ := testPeer(1, 1)
	peer.chainID = bxgateway.BSCChainID
	maxWriteTimeout := time.Millisecond // to allow for blockLoop goroutine to write to buffer
	mockClock := peer.clock.(*utils.MockClock)
	go peer.Start()

	block1a := bxmock.NewEthBlock(1, common.Hash{})
	peer.confirmedHead = blockRef{0, block1a.ParentHash()}
	// block is in the future, send after delay
	mockClock.SetTime(time.Unix(int64(block1a.Time())-int64(1), 0))
	peer.QueueNewBlock(block1a, big.NewInt(10))

	// block should not be sent yet
	assert.False(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 0, len(rw.WriteMessages))

	mockClock.IncTime(time.Second)

	// block 1a will be sent with delay
	assert.True(t, rw.ExpectWrite(maxWriteTimeout))
	assert.Equal(t, 1, len(rw.WriteMessages))
}
