package eth

import (
	"context"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/forkid"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
	"time"
)

const expectTimeout = time.Millisecond

func setup(writeChannelSize int) (blockchain.Bridge, *Handler, *Peer) {
	bridge := blockchain.NewBxBridge(Converter{})
	config, _ := network.NewEthereumPreset("Mainnet")
	handler := NewHandler(context.Background(), bridge, &config, bxmock.NewMockWSProvider())
	peer, _ := testPeer(writeChannelSize)
	_ = handler.peers.register(peer)
	return bridge, handler, peer
}

func TestHandler_HandleStatus(t *testing.T) {
	_, handler, peer := setup(-1)
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

func TestHandler_HandleTransactions(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	bridge, handler, peer := setup(-1)

	txs := []*ethtypes.Transaction{
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey),
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey),
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

func TestHandler_HandleTransactionHashes(t *testing.T) {
	bridge, handler, peer := setup(-1)

	txHashes := []types.SHA256Hash{
		types.GenerateSHA256Hash(),
		types.GenerateSHA256Hash(),
	}
	txHashesPacket := make(eth.NewPooledTransactionHashesPacket, 0)
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

func TestHandler_HandleNewBlock(t *testing.T) {
	bridge, handler, peer := setup(-1)
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

func TestHandler_HandleNewBlock_TooOld(t *testing.T) {
	bridge, handler, peer := setup(-1)
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
	bridge, handler, peer := setup(-1)
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
	bridge, handler, peer := setup(2)
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
	bridge, handler, peer := setup(2)
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
	bridge, handler, peer := setup(2)
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
	bridge, handler, peer := setup(1)
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

func TestHandler_processBDNBlockResolveDifficulty(t *testing.T) {
	bridge, handler, peer := setup(1)
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
	bridge, handler, peer := setup(1)
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

func TestHandler_blockForks(t *testing.T) {
	var err error
	bridge, handler, peer := setup(1)
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

	// expectation: duplicate, nothing new
	handler.processBDNBlock(bxBlock1)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)

	// expectation: sent to blockchain node (next in confirmed chain)
	handler.processBDNBlock(bxBlock2a)
	assertNoBlockSentToBDN(t, bridge)
	assertBlockSentToBlockchain(t, peerRW, block2a.Hash())

	// expectation: sent to gateway as confirmation
	_ = testHandleNewBlock(handler, peer, newBlock2a.Block, newBlock2a.TD)
	assertBlockSentToBDN(t, bridge, block2a.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)

	// expectation: nothing sent anywhere (parked for blockchain, unconfirmed for BDN)
	handler.processBDNBlock(bxBlock2b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)

	// expectation: block sent to blockchain node
	handler.processBDNBlock(bxBlock3a)
	assertNoBlockSentToBDN(t, bridge)
	assertBlockSentToBlockchain(t, peerRW, block3a.Hash())

	// expectation: new block hashes confirms blocks, and sends to BDN
	_ = testHandleNewBlockHashes(handler, peer, block3a.Hash(), block3a.NumberU64())
	assertBlockSentToBDN(t, bridge, block3a.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)

	// expectation: nothing sent anywhere (parked + unconfirmed)
	handler.processBDNBlock(bxBlock3b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)

	// expectation: nothing sent anywhere (unconfirmed, blockchain node is on 3a/4a path)
	handler.processBDNBlock(bxBlock4b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, peerRW)

	// expectation: send 2b, 3b, 4b to BDN (confirmed now)
	err = handler.Handle(peer, &newBlock4b)
	_ = testHandleNewBlock(handler, peer, newBlock4b.Block, newBlock4b.TD)
	assertBlockSentToBDN(t, bridge, block2b.Hash())
	assertBlockSentToBDN(t, bridge, block3b.Hash())
	assertBlockSentToBDN(t, bridge, block4b.Hash())
	assertNoBlockSentToBlockchain(t, peerRW)
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
