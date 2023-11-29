package eth

import (
	"context"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/logger"
	gateway_test "github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/forkid"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

const expectTimeout = time.Millisecond
const transactionBacklog = 500

func setup() (blockchain.Bridge, *Handler, []types.NodeEndpoint) {
	bridge := blockchain.NewBxBridge(Converter{}, false)
	config, _ := network.NewEthereumPreset("BSC-Mainnet")
	blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
	ctx := context.Background()
	handler := NewHandler(ctx, &config, NewChain(ctx, config.IgnoreBlockTimeout), bridge, NewEthWSManager(blockchainPeersInfo, NewMockWSProvider, bxgateway.WSProviderTimeout, false), make(map[string]struct{}))
	gateway_test.ConfigureLogger(logger.TraceLevel)
	return bridge, handler, blockchainPeers
}

func setupEthMainnet() (blockchain.Bridge, *Handler, []types.NodeEndpoint) {
	bridge := blockchain.NewBxBridge(Converter{}, false)
	config, _ := network.NewEthereumPreset("Mainnet")
	blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
	ctx := context.Background()
	handler := NewHandler(ctx, &config, NewChain(ctx, config.IgnoreBlockTimeout), bridge, NewEthWSManager(blockchainPeersInfo, NewMockWSProvider, bxgateway.WSProviderTimeout, false), make(map[string]struct{}))
	gateway_test.ConfigureLogger(logger.TraceLevel)
	return bridge, handler, blockchainPeers
}

func TestHandler_HandleStatus(t *testing.T) {
	_, handler, _ := setup()
	peer, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer)
	head := common.Hash{1, 2, 3}
	headDifficulty := big.NewInt(100)
	nextBlock := bxmock.NewEthBlock(2, head)

	err := handler.Handle(peer, &eth.StatusPacket{
		ProtocolVersion: eth.ETH66,
		NetworkID:       1,
		TD:              headDifficulty,
		Head:            head,
		Genesis:         common.Hash{2, 3, 4},
		ForkID:          forkid.ID{},
	})
	assert.Nil(t, err)

	// new difficulty is stored
	storedHeadDifficulty, ok := handler.chain.getBlockDifficulty(head)
	assert.True(t, ok)
	assert.Equal(t, headDifficulty, storedHeadDifficulty)

	// future blocks uses this difficulty
	nextBlockInfo := NewBlockInfo(nextBlock, nil)
	err = handler.chain.SetTotalDifficulty(nextBlockInfo)
	assert.Nil(t, err)
	assert.Equal(t, new(big.Int).Add(headDifficulty, nextBlock.Difficulty()), nextBlockInfo.TotalDifficulty())
}

func TestHandler_HandleTransactionsFromNode(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)

	txs := []*ethtypes.Transaction{
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, nil),
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, nil),
	}

	txsPacket := eth.TransactionsPacket(txs)

	err := handler.Handle(peer, &txsPacket)
	assert.Nil(t, err)

	bxTxs := <-bridge.ReceiveNodeTransactions()
	assert.Equal(t, len(txs), len(bxTxs.Transactions))

	for i, bxTx := range bxTxs.Transactions {
		tx := txs[i]
		assert.Equal(t, NewSHA256Hash(tx.Hash()), bxTx.Hash())

		encodedTx, _ := rlp.EncodeToBytes(tx)
		assert.Equal(t, encodedTx, []uint8(bxTx.Content()))
	}

	// pooled txs should have exact same behavior
	pooledTxsPacket := eth.PooledTransactionsPacket(txs)
	err = handler.Handle(peer, &pooledTxsPacket)
	assert.Nil(t, err)

	bxTxs2 := <-bridge.ReceiveNodeTransactions()
	assert.Equal(t, bxTxs, bxTxs2)
}

func TestHandler_HandleTransactionsFromBDN(t *testing.T) {
	var hash types.SHA256Hash
	hashRes, _ := hex.DecodeString("da605de1ee226fd20ba7e82745c742af5255284f8362d66fd8bcf89a318ac5f1")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8678201d785012a05f20082520894bbdef5f330f08afd93a7696f4ea79af4a41d0f8080808194a0d0f839e1efadc7f1f1cbba67a5dcee50e3c49d3b8b6bc5ebebcf4886d04260a7a07b4e4849bc016cbf17cd27e4fcbb301c5b25a14cc3c9d0b3c244567d7fbad6fc")

	var flags types.TxFlags
	flags |= types.TFPaidTx

	bxTx := types.NewRawBxTransaction(hash, content)

	bridge := blockchain.NewBxBridge(Converter{}, false)
	txs := blockchain.Transactions{
		Transactions: []*types.BxTransaction{bxTx},
	}

	err := bridge.SendTransactionsFromBDN(txs)
	assert.Nil(t, err)

	txHeard := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, txs.Transactions, txHeard.Transactions)
}

func TestHandler_BDNTransactionChannelTest(t *testing.T) {
	var hash types.SHA256Hash
	hashRes, _ := hex.DecodeString("da605de1ee226fd20ba7e82745c742af5255284f8362d66fd8bcf89a318ac5f1")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8678201d785012a05f20082520894bbdef5f330f08afd93a7696f4ea79af4a41d0f8080808194a0d0f839e1efadc7f1f1cbba67a5dcee50e3c49d3b8b6bc5ebebcf4886d04260a7a07b4e4849bc016cbf17cd27e4fcbb301c5b25a14cc3c9d0b3c244567d7fbad6fc")

	var flags types.TxFlags
	flags |= types.TFPaidTx

	bxTx := types.NewRawBxTransaction(hash, content)

	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)

	txs := blockchain.Transactions{
		Transactions: []*types.BxTransaction{bxTx},
	}

	// assume the transactionBacklog limit above normal capacity by certain amount, be able to handle this loading speed should indicate good rate of offloading channel.
	for i := 0; i < transactionBacklog; i++ {
		err := bridge.SendTransactionsFromBDN(txs)
		assert.Nil(t, err)
	}
}

func TestHandler_HandleTransactionHashes(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)

	txHashes := []types.SHA256Hash{
		types.GenerateSHA256Hash(),
		types.GenerateSHA256Hash(),
	}
	txHashesPacket := make(eth.NewPooledTransactionHashesPacket66, 0)
	for _, txHash := range txHashes {
		txHashesPacket = append(txHashesPacket, common.BytesToHash(txHash[:]))
	}

	err := handler.Handle(peer, &txHashesPacket)
	assert.Nil(t, err)

	txAnnouncements := <-bridge.ReceiveTransactionHashesAnnouncement()
	assert.Equal(t, peer.ID(), txAnnouncements.PeerID)

	for i, announcedHash := range txAnnouncements.Hashes {
		assert.Equal(t, txHashes[i], announcedHash)
	}
}

func TestHandler_HandleNewBlock_MultiNode_SlowNode(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)
	blockHeight := uint64(1)

	peer2, _, _ := testPeer(1, 2)
	_ = handler.peers.register(peer2)

	td := big.NewInt(10000)
	blockA := bxmock.NewEthBlock(blockHeight, common.Hash{})
	blockHeight++
	blockB := bxmock.NewEthBlock(blockHeight, blockA.Hash())
	blockHeight++
	blockC := bxmock.NewEthBlock(blockHeight, blockB.Hash())
	blockHeight++
	blockD := bxmock.NewEthBlock(blockHeight, blockC.Hash())
	blockHeight++
	blockE := bxmock.NewEthBlock(blockHeight, blockD.Hash())

	// both peers send blockA, second is not sent to BDN
	err := testHandleNewBlock(handler, peer, blockA, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockA.Hash())

	err = testHandleNewBlock(handler, peer2, blockA, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	// peer1 sends blocks B-E first, all sent to BDN
	err = testHandleNewBlock(handler, peer, blockB, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockB.Hash())

	err = testHandleNewBlock(handler, peer, blockC, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockC.Hash())

	err = testHandleNewBlock(handler, peer, blockD, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockD.Hash())

	err = testHandleNewBlock(handler, peer, blockE, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockE.Hash())

	// peer 2 sends same blocks B-E, none sent to BDN
	err = testHandleNewBlock(handler, peer2, blockB, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(handler, peer2, blockC, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(handler, peer2, blockD, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(handler, peer2, blockE, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlock_MultiNode_BroadcastAmongNodes(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)

	peer2, _, _ := testPeer(1, 2)
	_ = handler.peers.register(peer2)

	td := big.NewInt(10000)
	blockHeight := uint64(1)

	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	blockHeight++

	err := testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, block.Hash())
	assertQueuedBlockForBlockchain(t, peer2, block.Hash())
	assertNoBlockQueuedForBlockchain(t, peer)
}

func TestHandler_HandleNewBlock_MultiNode_Fork(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)
	blockHeight := uint64(1)

	peer2, _, _ := testPeer(1, 2)
	_ = handler.peers.register(peer2)

	td := big.NewInt(10000)
	blockA := bxmock.NewEthBlock(blockHeight, common.Hash{})
	blockHeight++
	blockB1 := bxmock.NewEthBlock(blockHeight, blockA.Hash())
	blockB2 := bxmock.NewEthBlock(blockHeight, blockA.Hash())
	blockHeight++
	blockC1 := bxmock.NewEthBlock(blockHeight, blockB1.Hash())
	blockC2 := bxmock.NewEthBlock(blockHeight, blockB2.Hash())
	blockHeight++
	blockD := bxmock.NewEthBlock(blockHeight, blockC1.Hash())

	err := testHandleNewBlock(handler, peer, blockA, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockA.Hash())

	// peer2 sends same block already sent by peer1, not sent to BDN
	err = testHandleNewBlock(handler, peer2, blockA, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(handler, peer, blockB1, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockB1.Hash())

	// peer2 sends block at same height, not sent to BDN
	err = testHandleNewBlock(handler, peer2, blockB2, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(handler, peer, blockC1, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockC1.Hash())

	// peer2 sends block at same height, not sent to BDN
	err = testHandleNewBlock(handler, peer2, blockC2, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(handler, peer, blockD, td)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, blockD.Hash())

	// peer2 sends same block already sent by peer1, not sent to BDN
	err = testHandleNewBlock(handler, peer2, blockD, td)
	assert.Nil(t, err)
	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlock(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)
	blockHeight := uint64(1)

	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	header := block.Header()
	td := big.NewInt(10000)

	err := testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)

	assertBlockSentToBDN(t, bridge, block.Hash())

	storedHeaderByHash, err := handler.GetHeaders(eth.HashOrNumber{
		Hash: block.Hash(),
	}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(storedHeaderByHash))
	assert.Equal(t, header, storedHeaderByHash[0])

	storedHeaderByHeight, err := handler.GetHeaders(eth.HashOrNumber{
		Number: blockHeight,
	}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(storedHeaderByHeight))
	assert.Equal(t, header, storedHeaderByHeight[0])
}

func TestHandler_HandleNewBlock_IgnoreAfterTheMerge(t *testing.T) {
	bridge, handler, _ := setupEthMainnet()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)
	blockHeight := uint64(1)

	header := bxmock.NewEthBlockHeader(blockHeight, common.Hash{})
	block := bxmock.NewEthBlockWithHeader(header)
	td := big.NewInt(10000)

	err := testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)

	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlock_TooOld(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)
	blockHeight := uint64(1)

	// oldest block that won't be sent to BDN
	header := bxmock.NewEthBlockHeader(blockHeight, common.Hash{})
	header.Time = uint64(time.Now().Add(-handler.config.IgnoreBlockTimeout).Add(-time.Second).Unix())
	block := bxmock.NewEthBlockWithHeader(header)
	td := big.NewInt(10000)

	err := testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)

	assertNoBlockSentToBDN(t, bridge)

	// oldest block that will be sent to BDN
	header = bxmock.NewEthBlockHeader(blockHeight, common.Hash{})
	header.Time = uint64(time.Now().Add(-handler.config.IgnoreBlockTimeout).Add(time.Second).Unix())
	block = bxmock.NewEthBlockWithHeader(header)
	td = big.NewInt(10000)

	err = testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)

	assertBlockSentToBDN(t, bridge, block.Hash())
}

func TestHandler_HandleNewBlock_TooFarInFuture(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(-1, 1)
	_ = handler.peers.register(peer)
	blockHeight := uint64(maxFutureBlockNumber + 100)

	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	td := big.NewInt(10000)

	err := testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)

	// ok, always send initial block
	assertBlockSentToBDN(t, bridge, block.Hash())

	block = bxmock.NewEthBlock(blockHeight*2, block.Hash())
	err = testHandleNewBlock(handler, peer, block, td)
	assert.Nil(t, err)

	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlockHashes(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(2, 1)
	_ = handler.peers.register(peer)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// process parent block for calculating difficulty
	parentBlock := bxmock.NewEthBlock(9, common.Hash{})
	parentHash := parentBlock.Hash()
	parentTD := big.NewInt(1000)

	err := handler.processBlock(peer, NewBlockInfo(parentBlock, parentTD))
	assert.Nil(t, err)

	assertBlockSentToBDN(t, bridge, parentHash)

	// start message handling goroutines
	go func() {
		for {
			if err := handleMessage(handler, peer); err != nil {
				assert.Fail(t, "unexpected message handling failure")
			}
		}
	}()

	// process new block request
	blockHeight := uint64(10)
	block := bxmock.NewEthBlock(blockHeight, parentHash)

	err = testHandleNewBlockHashes(handler, peer, block.Hash(), blockHeight)
	assert.Nil(t, err)

	// expect get headers + get bodies request to peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 2, len(peerRW.WriteMessages))

	getHeadersMsg := peerRW.WriteMessages[0]
	getBodiesMsg := peerRW.WriteMessages[1]

	assert.Equal(t, uint64(eth.GetBlockHeadersMsg), getHeadersMsg.Code)
	assert.Equal(t, uint64(eth.GetBlockBodiesMsg), getBodiesMsg.Code)

	var getHeaders eth.GetBlockHeadersPacket
	err = getHeadersMsg.Decode(&getHeaders)
	assert.Nil(t, err)
	assert.Equal(t, block.Hash(), getHeaders.Origin.Hash)
	assert.Equal(t, uint64(1), getHeaders.Amount)
	assert.Equal(t, uint64(0), getHeaders.Skip)
	assert.Equal(t, false, getHeaders.Reverse)

	assert.Nil(t, err)

	var getBodies eth.GetBlockBodiesPacket
	err = getBodiesMsg.Decode(&getBodies)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(getBodies))
	assert.Equal(t, block.Hash(), getBodies[0])

	// send header/body from peer
	peerRW.QueueIncomingMessage(uint64(eth.BlockHeadersMsg), eth.BlockHeadersPacket{block.Header()})
	peerRW.QueueIncomingMessage(uint64(eth.BlockBodiesMsg), eth.BlockBodiesPacket{&eth.BlockBody{
		Transactions: block.Transactions(),
		Uncles:       block.Uncles(),
	}})

	expectedDifficulty := new(big.Int).Add(parentTD, block.Difficulty())
	expectedBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block, expectedDifficulty))
	newBlock := assertBlockSentToBDN(t, bridge, block.Hash())
	assert.True(t, expectedBlock.Equals(newBlock.Block))

	// difficulty is unknown from header/body handling, but still set by stored difficulties
	assert.Equal(t, expectedDifficulty, newBlock.Block.TotalDifficulty)

	// check peer internal state is cleaned up
	assert.Equal(t, 0, len(peer.responseQueue))
}

func TestHandler_HandleNewBlockHashes_HandlingError(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(2, 1)
	_ = handler.peers.register(peer)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	blockHeight := uint64(1)
	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	err := testHandleNewBlockHashes(handler, peer, block.Hash(), blockHeight)

	// expect get headers + get bodies request to peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 2, len(peerRW.WriteMessages))

	getHeadersMsg := peerRW.WriteMessages[0]
	getBodiesMsg := peerRW.WriteMessages[1]

	assert.Equal(t, uint64(eth.GetBlockHeadersMsg), getHeadersMsg.Code)
	assert.Equal(t, uint64(eth.GetBlockBodiesMsg), getBodiesMsg.Code)

	peerRW.QueueIncomingMessage(uint64(eth.BlockBodiesMsg), eth.BlockBodiesPacket{&eth.BlockBody{
		Transactions: block.Transactions(),
		Uncles:       block.Uncles(),
	}})

	// handle bad block body
	err = handleMessage(handler, peer)
	assert.Nil(t, err)

	assertNoBlockSentToBDN(t, bridge)

	time.Sleep(1 * time.Millisecond)
	assert.True(t, peer.disconnected)
}

func TestHandler_HandleNewBlockHashes66(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(2, 1)
	_ = handler.peers.register(peer)
	peer.version = eth.ETH66
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	blockHeight := uint64(1)
	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	err := testHandleNewBlockHashes(handler, peer, block.Hash(), blockHeight)

	// expect get headers + get bodies request to peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 2, len(peerRW.WriteMessages))

	getHeadersMsg := peerRW.WriteMessages[0]
	getBodiesMsg := peerRW.WriteMessages[1]

	assert.Equal(t, uint64(eth.GetBlockHeadersMsg), getHeadersMsg.Code)
	assert.Equal(t, uint64(eth.GetBlockBodiesMsg), getBodiesMsg.Code)

	var getHeaders eth.GetBlockHeadersPacket66
	err = getHeadersMsg.Decode(&getHeaders)
	assert.Nil(t, err)
	assert.Equal(t, block.Hash(), getHeaders.Origin.Hash)
	assert.Equal(t, uint64(1), getHeaders.Amount)
	assert.Equal(t, uint64(0), getHeaders.Skip)
	assert.Equal(t, false, getHeaders.Reverse)

	var getBodies eth.GetBlockBodiesPacket66
	err = getBodiesMsg.Decode(&getBodies)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(getBodies.GetBlockBodiesPacket))
	assert.Equal(t, block.Hash(), getBodies.GetBlockBodiesPacket[0])

	headersID := getHeaders.RequestId
	bodiesID := getBodies.RequestId

	// send header/body from peer (out of order with request IDs)
	peerRW.QueueIncomingMessage(uint64(eth.BlockBodiesMsg), eth.BlockBodiesPacket66{
		RequestId: bodiesID,
		BlockBodiesPacket: eth.BlockBodiesPacket{&eth.BlockBody{
			Transactions: block.Transactions(),
			Uncles:       block.Uncles(),
		}},
	})
	peerRW.QueueIncomingMessage(uint64(eth.BlockHeadersMsg), eth.BlockHeadersPacket66{
		RequestId:          headersID,
		BlockHeadersPacket: eth.BlockHeadersPacket{block.Header()},
	})

	// expect bodies message, then peer message
	err = handleMessage(handler, peer)
	assert.Nil(t, err)

	err = handleMessage(handler, peer)
	assert.Nil(t, err)

	expectedBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block, nil))
	newBlock := assertBlockSentToBDN(t, bridge, block.Hash())
	assert.True(t, expectedBlock.Equals(newBlock.Block))

	// difficulty is unknown from header/body handling
	assert.Equal(t, big.NewInt(0), newBlock.Block.TotalDifficulty)

	// check peer state is cleaned up
	assert.Equal(t, 0, len(peer.responseQueue66.Keys()))
}

func TestHandler_processBDNBlock(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(2, 1)
	_ = handler.peers.register(peer)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// generate bx block for processing
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	blockHash := ethBlock.Hash()
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, td))

	// indicate previous head from status message
	peer.confirmedHead = blockRef{hash: ethBlock.ParentHash()}

	handler.processBDNBlock(bxBlock)

	// expect message to be sent to a peer
	blockPacket := assertBlockSentToBlockchain(t, peerRW, ethBlock.Hash())
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	// contents stored in cache
	assert.True(t, handler.chain.HasBlock(blockHash))

	storedHeader, ok := handler.chain.getBlockHeader(10, blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Header(), storedHeader)

	storedBody, ok := handler.chain.getBlockBody(blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Body().Uncles, storedBody.Uncles)
	for i, tx := range ethBlock.Body().Transactions {
		assert.Equal(t, tx.Hash(), storedBody.Transactions[i].Hash())
	}

	// confirm block, should send back to BDN and update head
	err := handler.Handle(peer, &eth.BlockHeadersPacket{ethBlock.Header()})
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, ethBlock.Hash())
}

func TestHandler_processBDNBlock_MultiNode(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer)

	// add extra peers
	peer2, _, _ := testPeer(1, 2)
	_ = handler.peers.register(peer2)
	peer3, _, _ := testPeer(1, 3)
	_ = handler.peers.register(peer3)

	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)
	peer2.Start()
	peerRW2 := peer2.rw.(*test.MsgReadWriter)
	peer3.Start()
	peerRW3 := peer3.rw.(*test.MsgReadWriter)

	// generate bx block for processing
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	blockHash := ethBlock.Hash()
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, td))

	// indicate previous head from status message
	peer.confirmedHead = blockRef{hash: ethBlock.ParentHash()}
	peer2.confirmedHead = blockRef{hash: ethBlock.ParentHash()}

	handler.processBDNBlock(bxBlock)

	// expect message to be sent to peer 1
	blockPacket := assertBlockSentToBlockchain(t, peerRW, ethBlock.Hash())
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	// expect message to be sent to peer 2
	blockPacket2 := assertBlockSentToBlockchain(t, peerRW2, ethBlock.Hash())
	assert.Equal(t, blockHash, blockPacket2.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket2.Block.Transactions()))
	assert.Equal(t, td, blockPacket2.TD)

	// expect no block sent to peer3 because confirmedHead is not parent
	assertNoBlockSentToBlockchain(t, peerRW3)

	// contents stored in cache
	assert.True(t, handler.chain.HasBlock(blockHash))

	storedHeader, ok := handler.chain.getBlockHeader(10, blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Header(), storedHeader)

	storedBody, ok := handler.chain.getBlockBody(blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Body().Uncles, storedBody.Uncles)
	for i, tx := range ethBlock.Body().Transactions {
		assert.Equal(t, tx.Hash(), storedBody.Transactions[i].Hash())
	}

	// confirm block, should send back to BDN and update head
	err := handler.Handle(peer, &eth.BlockHeadersPacket{ethBlock.Header()})
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, ethBlock.Hash())
}

func TestHandler_processBDNBlockResolveDifficulty(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// preprocess a parent for calculating difficulty
	parentBlock := bxmock.NewEthBlock(9, common.Hash{})
	parentHash := parentBlock.Hash()
	parentTD := big.NewInt(1000)

	err := testHandleNewBlock(handler, peer, parentBlock, parentTD)
	assert.Nil(t, err)

	// generate bx block for processing
	ethBlock := bxmock.NewEthBlock(10, parentHash)
	blockHash := ethBlock.Hash()
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, nil))

	handler.processBDNBlock(bxBlock)

	blockPacket := assertBlockSentToBlockchain(t, peerRW, blockHash)
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, new(big.Int).Add(parentTD, ethBlock.Difficulty()), blockPacket.TD)
}

func TestHandler_processBDNBlockUnresolvableDifficulty(t *testing.T) {
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// generate bx block for processing
	height := uint64(10)
	ethBlock := bxmock.NewEthBlock(height, common.Hash{})
	blockHash := ethBlock.Hash()
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, nil))

	handler.processBDNBlock(bxBlock)

	// expect message to be sent to a peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 1, len(peerRW.WriteMessages))
	msg := peerRW.WriteMessages[0]
	assert.Equal(t, uint64(eth.NewBlockHashesMsg), msg.Code)

	var blockHashesPacket eth.NewBlockHashesPacket
	err := msg.Decode(&blockHashesPacket)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(blockHashesPacket))
	assert.Equal(t, height, blockHashesPacket[0].Number)
	assert.Equal(t, blockHash, blockHashesPacket[0].Hash)
}

func TestHandler_BlockAtDepth(t *testing.T) {
	c := newChain(context.Background(), 10, 5, 5, time.Hour, 1000)
	blockConfirmationCounts := 4
	_, handler, _ := setup()
	peer, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer)
	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3 := bxmock.NewEthBlock(3, block2.Hash())
	block4 := bxmock.NewEthBlock(4, block3.Hash())
	block5 := bxmock.NewEthBlock(5, block4.Hash())
	addBlock(c, block1)
	addBlock(c, block2)
	addBlock(c, block3)
	addBlock(c, block4)
	addBlock(c, block5)

	handler.chain = c
	bxb1, err := handler.createBxBlockFromEthHeader(block1.Header())

	// head of the chain state is block 5, for depth 4, the block should be 1
	b1, err := handler.blockAtDepth(blockConfirmationCounts)
	assert.Nil(t, err)
	assert.Equal(t, b1.Hash(), bxb1.Hash())

	// for depth 5, the chain state is not deep enough
	blockConfirmationCounts = 5
	_, err = handler.blockAtDepth(blockConfirmationCounts)
	assert.NotNil(t, err)
}

func TestHandler_blockForks(t *testing.T) {
	var err error
	bridge, handler, _ := setup()
	peer, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer)
	handler.config.SendBlockConfirmation = true
	handler.config.BlockConfirmationsCount = 3
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	block1 := bxmock.NewEthBlock(uint64(1), common.Hash{})
	block2a := bxmock.NewEthBlock(uint64(2), block1.Hash())
	block2b := bxmock.NewEthBlock(uint64(2), block1.Hash())
	block3a := bxmock.NewEthBlock(uint64(3), block2a.Hash())
	block3b := bxmock.NewEthBlock(uint64(3), block2b.Hash())
	block4b := bxmock.NewEthBlock(uint64(4), block3b.Hash())

	// sequence of blocks received from blockchain
	newBlock1 := eth.NewBlockPacket{Block: block1, TD: big.NewInt(100)}
	newBlock2a := eth.NewBlockPacket{Block: block2a, TD: big.NewInt(201)}
	// newBlock3a sent as NewBlockHashes instead of block packet
	newBlock4b := eth.NewBlockPacket{Block: block4b, TD: big.NewInt(402)}

	// sequence of blocks received from BDN
	bxBlock1, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block1, big.NewInt(100)))
	bxBlock2a, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block2a, big.NewInt(201)))
	bxBlock2b, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block2b, big.NewInt(202)))
	bxBlock3a, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block3a, big.NewInt(301)))
	bxBlock3b, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block3b, big.NewInt(302)))
	bxBlock4b, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(block4b, big.NewInt(402)))

	/*
		sending sequence:
			1  blockchain
			2a BDN
			2a blockchain
			2b BDN
			3a BDN
			3a blockchain
			3b BDN
			4b BDN
			4b blockchain (maybe this should be a confirmation instead of a full block?)
	*/

	// expectation: block is sent to BDN
	err = testHandleNewBlock(handler, peer, newBlock1.Block, newBlock1.TD)
	assert.Nil(t, err)
	assertBlockSentToBDN(t, bridge, block1.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: duplicate, nothing new
	handler.processBDNBlock(bxBlock1)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: sent to blockchain node (next in confirmed chain)
	handler.processBDNBlock(bxBlock2a)
	assertNoBlockSentToBDN(t, bridge)
	assertBlockSentToBlockchain(t, peerRW, block2a.Hash())
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: sent to gateway as confirmation
	_ = testHandleNewBlock(handler, peer, newBlock2a.Block, newBlock2a.TD)
	assertBlockSentToBDN(t, bridge, block2a.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: nothing sent anywhere (parked for blockchain, unconfirmed for BDN)
	handler.processBDNBlock(bxBlock2b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: block sent to blockchain node
	handler.processBDNBlock(bxBlock3a)
	assertNoBlockSentToBDN(t, bridge)
	assertBlockSentToBlockchain(t, peerRW, block3a.Hash())
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: new block hashes confirms blocks, and sends to BDN
	_ = testHandleNewBlockHashes(handler, peer, block3a.Hash(), block3a.NumberU64())
	assertBlockSentToBDN(t, bridge, block3a.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: nothing sent anywhere (parked + unconfirmed)
	handler.processBDNBlock(bxBlock3b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: nothing sent anywhere (unconfirmed, blockchain node is on 3a/4a path)
	handler.processBDNBlock(bxBlock4b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: send 2b, 3b, 4b to BDN (confirmed now)
	err = handler.Handle(peer, &newBlock4b)
	_ = testHandleNewBlock(handler, peer, newBlock4b.Block, newBlock4b.TD)
	assertBlockSentToBDN(t, bridge, block2b.Hash())
	assertBlockSentToBDN(t, bridge, block3b.Hash())
	assertBlockSentToBDN(t, bridge, block4b.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)
	assertConfirmationBlockSentToGateway(t, bridge, bxBlock1)
}

func TestHandler_ConfirmBlockFromWS(t *testing.T) {
	bridge, handler, _ := setup()

	peer1, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer1)
	peer2, _, _ := testPeer(1, 2)
	_ = handler.peers.register(peer2)
	peer3, _, _ := testPeer(1, 3)
	_ = handler.peers.register(peer3)

	peer1.Start()
	peerRW1 := peer1.rw.(*test.MsgReadWriter)
	peer2.Start()
	peerRW2 := peer2.rw.(*test.MsgReadWriter)
	peer3.Start()
	peerRW3 := peer3.rw.(*test.MsgReadWriter)

	height := big.NewInt(1)
	ethBlockA := bxmock.NewEthBlock(height.Uint64(), common.Hash{})

	peer1.confirmedHead = blockRef{hash: ethBlockA.ParentHash()}
	peer2.confirmedHead = blockRef{hash: ethBlockA.ParentHash()}
	peer3.confirmedHead = blockRef{hash: ethBlockA.ParentHash()}

	td := big.NewInt(10000)
	blockA, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlockA, td))
	handler.processBDNBlock(blockA)

	blockPacket := assertBlockSentToBlockchain(t, peerRW1, ethBlockA.Hash())
	assert.Equal(t, ethBlockA.Hash(), blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlockA.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	blockPacket = assertBlockSentToBlockchain(t, peerRW2, ethBlockA.Hash())
	assert.Equal(t, ethBlockA.Hash(), blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlockA.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	blockPacket = assertBlockSentToBlockchain(t, peerRW3, ethBlockA.Hash())
	assert.Equal(t, ethBlockA.Hash(), blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlockA.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	handler.confirmBlockFromWS(ethBlockA.Hash(), height, peer1)
	time.Sleep(time.Millisecond)
	assert.Equal(t, blockRef{height: 1, hash: ethBlockA.Hash()}, peer1.confirmedHead)
	assert.Equal(t, blockRef{height: 0, hash: ethBlockA.ParentHash()}, peer2.confirmedHead)
	assert.Equal(t, blockRef{height: 0, hash: ethBlockA.ParentHash()}, peer3.confirmedHead)
	assertBlockSentToBDN(t, bridge, ethBlockA.Hash())
	assertConfirmationBlockSentToGateway(t, bridge, blockA)

	ethBlockB := bxmock.NewEthBlock(height.Uint64(), common.Hash{})
	blockB, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlockB, td))
	handler.processBDNBlock(blockB)
	handler.confirmBlockFromWS(ethBlockB.Hash(), height, peer2)
	time.Sleep(time.Millisecond)
	assert.Equal(t, blockRef{height: 1, hash: ethBlockA.Hash()}, peer1.confirmedHead)
	assert.Equal(t, blockRef{height: 1, hash: ethBlockB.Hash()}, peer2.confirmedHead)
	assert.Equal(t, blockRef{height: 0, hash: ethBlockA.ParentHash()}, peer3.confirmedHead)
	assertNoBlockSentToBDN(t, bridge)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	height = big.NewInt(2)
	ethBlockB2 := bxmock.NewEthBlock(height.Uint64(), ethBlockB.Hash())
	blockB2, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlockB2, td))
	handler.processBDNBlock(blockB2)
	handler.confirmBlockFromWS(ethBlockB2.Hash(), height, peer2)
	time.Sleep(time.Millisecond)
	assert.Equal(t, blockRef{height: 1, hash: ethBlockA.Hash()}, peer1.confirmedHead)
	assert.Equal(t, blockRef{height: 2, hash: ethBlockB2.Hash()}, peer2.confirmedHead)
	assert.Equal(t, blockRef{height: 0, hash: ethBlockA.ParentHash()}, peer3.confirmedHead)
	assertBlockSentToBDN(t, bridge, ethBlockB.Hash())
	assertBlockSentToBDN(t, bridge, ethBlockB2.Hash())
	assertConfirmationBlockSentToGateway(t, bridge, blockB2)
}

func TestHandler_DisconnectInboundPeer(t *testing.T) {
	bridge, handler, _ := setup()

	peer1, _, _ := testPeer(1, 1)
	_ = handler.peers.register(peer1)

	peer1.Start()
	go func() {
		handler.handleBDNBridge(context.Background())
	}()

	err := bridge.SendDisconnectEvent(types.NodeEndpoint{PublicKey: peer1.IPEndpoint().PublicKey})
	assert.Nil(t, err)
}

// variety of handling functions here to trigger handlers in handlers.go instead of directly invoking the handler (useful for setting state on Peer during handling)
func testHandleNewBlock(handler *Handler, peer *Peer, block *ethtypes.Block, td *big.Int) error {
	newBlockPacket := eth.NewBlockPacket{
		Block: block,
		TD:    td,
	}
	return handleNewBlockMsg(handler, encodeRLP(eth.NewBlockMsg, newBlockPacket), peer)
}

func testHandleNewBlockHashes(handler *Handler, peer *Peer, hash common.Hash, height uint64) error {
	newBlockHashesPacket := eth.NewBlockHashesPacket{
		{
			Hash:   hash,
			Number: height,
		},
	}
	return handleNewBlockHashes(handler, encodeRLP(eth.NewBlockHashesMsg, newBlockHashesPacket), peer)
}

func encodeRLP(code uint64, data interface{}) Decoder {
	size, r, err := rlp.EncodeToReader(data)
	if err != nil {
		panic(err)
	}
	return p2p.Msg{
		Code:    code,
		Size:    uint32(size),
		Payload: r,
	}
}

func assertNewHeadSentToPeer(t *testing.T, peer *Peer, hash common.Hash, height uint64) {
	select {
	case sentHead := <-peer.newHeadCh:
		assert.Equal(t, hash, sentHead.hash)
		assert.Equal(t, height, sentHead.height)
	case <-time.After(expectTimeout):
		assert.FailNow(t, "peer did not receive newHead", "hash=%v", hash)
	}
}

func assertNoNewHeadSentToPeer(t *testing.T, peer *Peer) {
	select {
	case sentHead := <-peer.newHeadCh:
		assert.FailNow(t, "peer received unexpected newHead", "hash=%v", sentHead.hash)
	case <-time.After(expectTimeout):
	}
}

func assertConfirmationBlockSentToGateway(t *testing.T, bridge blockchain.Bridge, block *types.BxBlock) {
	select {
	case blockcnf := <-bridge.ReceiveConfirmedBlockFromNode():
		assert.Equal(t, blockcnf.Block.Hash(), block.Hash())
	case <-time.After(expectTimeout):
		assert.FailNow(t, "BDN did not receive confirmed block", "hash=%v", block.Hash())
	}
}

func assertNoConfirmationBlockSentToBDN(t *testing.T, bridge blockchain.Bridge) {
	select {
	case sentBlock := <-bridge.ReceiveConfirmedBlockFromNode():
		assert.FailNow(t, "BDN received unexpected block confirmation", "hash=%v", sentBlock.Block.Hash())
	case <-time.After(expectTimeout):
	}
}

func assertBlockSentToBDN(t *testing.T, bridge blockchain.Bridge, hash common.Hash) blockchain.BlockFromNode {
	select {
	case sentBlock := <-bridge.ReceiveBlockFromNode():
		assert.Equal(t, hash.Bytes(), sentBlock.Block.Hash().Bytes())
		return sentBlock
	case <-time.After(expectTimeout):
		assert.FailNow(t, "BDN did not receive block", "hash=%v", hash)
	}
	return blockchain.BlockFromNode{}
}

func assertNoBlockSentToBDN(t *testing.T, bridge blockchain.Bridge) {
	select {
	case sentBlock := <-bridge.ReceiveBlockFromNode():
		assert.FailNow(t, "BDN received unexpected block", "hash=%v", sentBlock.Block.Hash())
	case <-time.After(expectTimeout):
	}
}

func assertBlockSentToBlockchain(t *testing.T, rw *test.MsgReadWriter, hash common.Hash) eth.NewBlockPacket {
	assert.True(t, rw.ExpectWrite(time.Millisecond))
	assert.Equal(t, 1, len(rw.WriteMessages))
	msg := rw.PopWrittenMessage()
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)

	var newBlocks eth.NewBlockPacket
	err := msg.Decode(&newBlocks)
	assert.Nil(t, err)

	assert.Equal(t, hash, newBlocks.Block.Hash())
	return newBlocks
}

func assertNoBlockSentToBlockchain(t *testing.T, rw *test.MsgReadWriter) {
	assert.False(t, rw.ExpectWrite(expectTimeout))
}

func assertQueuedBlockForBlockchain(t *testing.T, peer *Peer, hash common.Hash) {
	select {
	case sentBlock := <-peer.newBlockCh:
		assert.Equal(t, hash.Bytes(), sentBlock.Block.Hash().Bytes())
	case <-time.After(expectTimeout):
		assert.FailNow(t, "Peer did not receive block", "hash=%v", hash)
	}
}

func assertNoBlockQueuedForBlockchain(t *testing.T, peer *Peer) {
	select {
	case sentBlock := <-peer.newBlockCh:
		assert.FailNow(t, "Peer received unexpected block", "hash=%v", sentBlock.Block.Hash())
	case <-time.After(expectTimeout):
	}
}
