package eth

import (
	"context"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/eth/test"
	test2 "github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/forkid"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
	"time"
)

func setup(writeChannelSize int) (blockchain.Bridge, *Handler, *Peer) {
	bridge := blockchain.NewBxBridge(Converter{})
	handler := NewHandler(context.Background(), bridge, nil, "")
	peer, _ := testPeer(writeChannelSize)
	_ = handler.peers.register(peer)
	return bridge, handler, peer
}

func TestHandler_HandleStatus(t *testing.T) {
	_, handler, peer := setup(-1)
	head := common.Hash{1, 2, 3}
	headDifficulty := big.NewInt(100)
	currentBlock := test2.NewEthBlock(2, head)

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
	storedHeadDifficulty, ok := handler.getBlockDifficulty(head)
	assert.True(t, ok)
	assert.Equal(t, headDifficulty, storedHeadDifficulty)

	// future blocks uses this difficulty
	nextBlockInfo := NewBlockInfo(currentBlock, nil)
	err = handler.setTotalDifficulty(nextBlockInfo)
	assert.Nil(t, err)
	assert.Equal(t, new(big.Int).Add(headDifficulty, currentBlock.Difficulty()), nextBlockInfo.TotalDifficulty())
}

func TestHandler_HandleTransactions(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	bridge, handler, peer := setup(-1)

	txs := []*ethtypes.Transaction{
		test2.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey),
		test2.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey),
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

	block := test2.NewEthBlock(blockHeight, common.Hash{})
	header := block.Header()
	td := big.NewInt(10000)

	newBlockPacket := eth.NewBlockPacket{
		Block: block,
		TD:    td,
	}
	err := handler.Handle(peer, &newBlockPacket)
	assert.Nil(t, err)

	bxBlock := <-bridge.ReceiveBlockFromNode()
	assert.Equal(t, block.Hash().Bytes(), bxBlock.Block.Hash().Bytes())
	assert.Equal(t, td, bxBlock.Block.TotalDifficulty)

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

func TestHandler_HandleNewBlockHashes(t *testing.T) {
	bridge, handler, peer := setup(-1)

	blockHeight := uint64(1)
	block := test2.NewEthBlock(blockHeight, common.Hash{})

	newBlockHashesPacket := eth.NewBlockHashesPacket{
		{
			Hash:   block.Hash(),
			Number: blockHeight,
		},
	}
	err := handler.Handle(peer, &newBlockHashesPacket)
	assert.Nil(t, err)

	announcement := <-bridge.ReceiveBlockAnnouncement()
	assert.Equal(t, block.Hash().Bytes(), announcement.Hash.Bytes())
	assert.Equal(t, peer.ID(), announcement.PeerID)
}

func TestHandler_GetHeadersByNumber(t *testing.T) {
	_, handler, _ := setup(-1)

	header1 := test2.NewEthBlockHeader(1, common.Hash{})
	header2 := test2.NewEthBlockHeader(2, header1.Hash())
	header3a := test2.NewEthBlockHeader(3, header2.Hash())
	header3b := test2.NewEthBlockHeader(3, header2.Hash())
	header4 := test2.NewEthBlockHeader(4, header3b.Hash())

	handler.storeBlockHeader(header1)
	handler.storeBlockHeader(header2)
	handler.storeBlockHeader(header3a)
	handler.storeBlockHeader(header3b)
	handler.storeBlockHeader(header4)

	var (
		headers []*ethtypes.Header
		err     error
	)

	// expected: err
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 10}, 1, 0, false)
	assert.NotNil(t, err)

	// expected: 1
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 1}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, header1, headers[0])

	// fork point, expected: 3b
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 3}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, header3b, headers[0])

	// expected: 1, 2, 3b, 4
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 1}, 4, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, header1, headers[0])
	assert.Equal(t, header2, headers[1])
	assert.Equal(t, header3b, headers[2])
	assert.Equal(t, header4, headers[3])

	// expected: 1, 3b
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 1}, 2, 1, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, header1, headers[0])
	assert.Equal(t, header3b, headers[1])

	// expected: 4, 2
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 4}, 2, 1, true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, header4, headers[0])
	assert.Equal(t, header2, headers[1])

	// expected: 1, 2, 3b, 4 (found all that was possible)
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 1}, 100, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, header1, headers[0])
	assert.Equal(t, header2, headers[1])
	assert.Equal(t, header3b, headers[2])
	assert.Equal(t, header4, headers[3])

	// expected: err (header couldn't be located at the requested height in the past, so most create error)
	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: 1}, 100, 0, true)
	assert.NotNil(t, err)
}

func TestHandler_GetHeadersByHash(t *testing.T) {
	_, handler, _ := setup(-1)

	header1 := test2.NewEthBlockHeader(1, common.Hash{})
	header2 := test2.NewEthBlockHeader(2, header1.Hash())
	header3a := test2.NewEthBlockHeader(3, header2.Hash())
	header3b := test2.NewEthBlockHeader(3, header2.Hash())
	header4 := test2.NewEthBlockHeader(4, header3b.Hash())

	handler.storeBlockHeader(header1)
	handler.storeBlockHeader(header2)
	handler.storeBlockHeader(header3a)
	handler.storeBlockHeader(header3b)
	handler.storeBlockHeader(header4)

	var (
		headers []*ethtypes.Header
		err     error
	)

	// expected: 1
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header1.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, header1, headers[0])

	// fork point, expected: 3a
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header3a.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, header3a, headers[0])

	// fork point, expected: 3b
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header3b.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, header3b, headers[0])

	// expected: 1, 2, 3b, 4
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header1.Hash()}, 4, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, header1, headers[0])
	assert.Equal(t, header2, headers[1])
	assert.Equal(t, header3b, headers[2])
	assert.Equal(t, header4, headers[3])

	// expected: 1, 3b
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header1.Hash()}, 2, 1, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, header1, headers[0])
	assert.Equal(t, header3b, headers[1])

	// expected: 4, 2
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header4.Hash()}, 2, 1, true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, header4, headers[0])
	assert.Equal(t, header2, headers[1])

	// expected: 1, 2, 3b, 4 (found all that was possible)
	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: header1.Hash()}, 100, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, header1, headers[0])
	assert.Equal(t, header2, headers[1])
	assert.Equal(t, header3b, headers[2])
	assert.Equal(t, header4, headers[3])
}

func TestHandler_processBDNBlock(t *testing.T) {
	bridge, handler, peer := setup(1)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// generate bx block for processing
	ethBlock := test2.NewEthBlock(10, common.Hash{})
	blockHash := ethBlock.Hash()
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, td))

	handler.processBDNBlock(bxBlock)

	// expect message to be sent to a peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 1, len(peerRW.WriteMessages))
	msg := peerRW.WriteMessages[0]
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)

	var blockPacket eth.NewBlockPacket
	err := msg.Decode(&blockPacket)
	assert.Nil(t, err)
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	// contents stored in cache
	assert.True(t, handler.hasBlock(blockHash))

	storedHeader, ok := handler.getBlockHeader(10, blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Header(), storedHeader)

	storedBody, ok := handler.getBlockBody(blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Body().Uncles, storedBody.Uncles)
	for i, tx := range ethBlock.Body().Transactions {
		assert.Equal(t, tx.Hash(), storedBody.Transactions[i].Hash())
	}
}

func TestHandler_processBDNBlockResolveDifficulty(t *testing.T) {
	bridge, handler, peer := setup(1)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// preprocess a parent for calculating difficulty
	parentBlock := test2.NewEthBlock(9, common.Hash{})
	parentHash := parentBlock.Hash()
	parentTD := big.NewInt(1000)
	err := handler.processBlock(peer, NewBlockInfo(parentBlock, parentTD))
	assert.Nil(t, err)

	// generate bx block for processing
	ethBlock := test2.NewEthBlock(10, parentHash)
	blockHash := ethBlock.Hash()
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, nil))

	handler.processBDNBlock(bxBlock)

	// expect message to be sent to a peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 1, len(peerRW.WriteMessages))
	msg := peerRW.WriteMessages[0]
	assert.Equal(t, uint64(eth.NewBlockMsg), msg.Code)

	var blockPacket eth.NewBlockPacket
	err = msg.Decode(&blockPacket)
	assert.Nil(t, err)
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
	ethBlock := test2.NewEthBlock(height, common.Hash{})
	blockHash := ethBlock.Hash()
	bxBlock, _ := bridge.BlockBlockchainToBDN(NewBlockInfo(ethBlock, nil))

	handler.processBDNBlock(bxBlock)

	// expect message to be sent to a peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 1, len(peerRW.WriteMessages))
	msg := peerRW.WriteMessages[0]
	assert.Equal(t, uint64(eth.NewBlockHashesMsg), msg.Code)

	var blockPacket eth.NewBlockHashesPacket
	err := msg.Decode(&blockPacket)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(blockPacket))
	assert.Equal(t, height, blockPacket[0].Number)
	assert.Equal(t, blockHash, blockPacket[0].Hash)
}

func TestHandler_processBDNBlockRequest(t *testing.T) {
	bridge, handler, peer := setup(2)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	// process parent block for calculating difficulty
	parentBlock := test2.NewEthBlock(9, common.Hash{})
	parentHash := parentBlock.Hash()
	parentTD := big.NewInt(1000)
	err := handler.processBlock(peer, NewBlockInfo(parentBlock, parentTD))
	assert.Nil(t, err)

	// pop parent off bridge
	_ = <-bridge.ReceiveBlockFromNode()

	blockHeight := uint64(2)
	block := test2.NewEthBlock(blockHeight, parentHash)

	handler.processBDNBlockRequest(blockchain.BlockAnnouncement{
		Hash:   NewSHA256Hash(block.Hash()),
		PeerID: peer.ID(),
	})

	// start message handling goroutine
	go func() {
		for {
			if err := handleMessage(handler, peer); err != nil {
				assert.Fail(t, "unexpected message handling failure")
			}
		}
	}()

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
	newBlock := <-bridge.ReceiveBlockFromNode()
	assert.True(t, expectedBlock.Equals(newBlock.Block))

	// difficulty is unknown from header/body handling
	assert.Equal(t, expectedDifficulty, newBlock.Block.TotalDifficulty)

	// check peer internal state is cleaned up
	assert.Equal(t, 0, len(peer.responseQueue))
}

func TestHandler_processBDNBlockRequestHandlingError(t *testing.T) {
	bridge, handler, peer := setup(2)
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	blockHeight := uint64(1)
	block := test2.NewEthBlock(blockHeight, common.Hash{})

	handler.processBDNBlockRequest(blockchain.BlockAnnouncement{
		Hash:   NewSHA256Hash(block.Hash()),
		PeerID: peer.ID(),
	})

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
	err := handleMessage(handler, peer)
	assert.Nil(t, err)

	select {
	case _ = <-bridge.ReceiveBlockFromNode():
		assert.Fail(t, "unexpected block received on bridge")
	default:
	}

	time.Sleep(1 * time.Millisecond)
	assert.True(t, peer.disconnected)
}

func TestHandler_processBDNBlockRequest66(t *testing.T) {
	bridge, handler, peer := setup(2)
	peer.version = eth.ETH66
	peer.Start()
	peerRW := peer.rw.(*test.MsgReadWriter)

	blockHeight := uint64(1)
	block := test2.NewEthBlock(blockHeight, common.Hash{})

	go handler.processBDNBlockRequest(blockchain.BlockAnnouncement{
		Hash:   NewSHA256Hash(block.Hash()),
		PeerID: peer.ID(),
	})

	// expect get headers + get bodies request to peer
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.True(t, peerRW.ExpectWrite(time.Millisecond))
	assert.Equal(t, 2, len(peerRW.WriteMessages))

	getHeadersMsg := peerRW.WriteMessages[0]
	getBodiesMsg := peerRW.WriteMessages[1]

	assert.Equal(t, uint64(eth.GetBlockHeadersMsg), getHeadersMsg.Code)
	assert.Equal(t, uint64(eth.GetBlockBodiesMsg), getBodiesMsg.Code)

	var getHeaders eth.GetBlockHeadersPacket66
	err := getHeadersMsg.Decode(&getHeaders)
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
	newBlock := <-bridge.ReceiveBlockFromNode()
	assert.True(t, expectedBlock.Equals(newBlock.Block))

	// difficulty is unknown from header/body handling
	assert.Equal(t, big.NewInt(0), newBlock.Block.TotalDifficulty)

	// check peer state is cleaned up
	assert.Equal(t, 0, len(peer.responseQueue66.Keys()))
}

func TestHandler_resolvedForkedHeaders(t *testing.T) {
	_, handler, _ := setup(-1)

	header1 := test2.NewEthBlockHeader(1, common.Hash{})
	header2 := test2.NewEthBlockHeader(2, header1.Hash())
	header2b := test2.NewEthBlockHeader(2, header1.Hash())
	header3 := test2.NewEthBlockHeader(3, header2.Hash())

	handler.storeBlockHeader(header1)
	handler.storeBlockHeader(header2)
	handler.storeBlockHeader(header2b)
	handler.storeBlockHeader(header3)

	// header 2 is part of chain, so select it
	candidateHeaders := []*ethtypes.Header{header2, header2b}
	canonicalHeader, err := handler.resolveForkedHeaders(candidateHeaders, 2)
	assert.Nil(t, err)
	assert.Equal(t, header2, canonicalHeader)

	// none of headers found because wrong height requested
	canonicalHeader, err = handler.resolveForkedHeaders(candidateHeaders, 3)
	assert.NotNil(t, err)

	header4 := test2.NewEthBlockHeader(4, common.Hash{})
	header4b := test2.NewEthBlockHeader(4, common.Hash{})
	header5 := test2.NewEthBlockHeader(5, header4.Hash())

	handler.storeBlockHeader(header4)
	handler.storeBlockHeader(header4b)
	handler.storeBlockHeader(header5)

	// header 4 is part of chain, so select ii
	candidateHeaders = []*ethtypes.Header{header4, header4b}
	canonicalHeader, err = handler.resolveForkedHeaders(candidateHeaders, 4)
	assert.Nil(t, err)
	assert.Equal(t, header4, canonicalHeader)

	// partial chainstate has been pruned, since link from 4 => 3 has been broken
	assert.Equal(t, 2, len(handler.partialChainstate))

	// cannot retrieve previous valid header 2, since 4 => 3 has been broken
	candidateHeaders = []*ethtypes.Header{header2, header2b}
	canonicalHeader, err = handler.resolveForkedHeaders(candidateHeaders, 2)
	assert.NotNil(t, err)
}

func TestHandler_clean(t *testing.T) {
	_, handler, _ := setup(-1)

	for i := 0; i < 100; i++ {
		handler.storeBlock(test2.NewEthBlock(uint64(i), common.Hash{}), big.NewInt(int64(i)))
	}

	block101 := test2.NewEthBlock(101, common.Hash{})
	header101 := block101.Header()
	handler.storeBlock(block101, big.NewInt(110))

	block102 := test2.NewEthBlock(102, header101.Hash())
	header102 := block102.Header()
	handler.storeBlock(block102, big.NewInt(120))

	block103a := test2.NewEthBlock(103, header102.Hash())
	handler.storeBlock(block103a, big.NewInt(131))

	block103b := test2.NewEthBlock(103, header102.Hash())
	header103b := block103b.Header()
	handler.storeBlock(block103b, big.NewInt(132))

	block104 := test2.NewEthBlock(104, header103b.Hash())
	handler.storeBlock(block104, big.NewInt(140))

	var (
		headers []*ethtypes.Header
		bodies  []*ethtypes.Body
		err     error
	)

	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: block101.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, header101, headers[0])

	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: block101.NumberU64()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, header101, headers[0])

	bodies, err = handler.GetBodies([]common.Hash{block101.Hash()})
	assert.Nil(t, err)
	assert.Equal(t, block101.Body(), bodies[0])

	// clean once with value close to actual block store size
	lowest, highest, num := handler.clean(100)
	assert.Equal(t, 5, num)
	assert.Equal(t, 0, lowest)
	assert.Equal(t, 4, highest)

	// clean many entries
	lowest, highest, num = handler.clean(2)
	assert.Equal(t, 97, num)
	assert.Equal(t, 5, lowest)
	assert.Equal(t, 102, highest)

	headers, err = handler.GetHeaders(eth.HashOrNumber{Hash: block101.Hash()}, 1, 0, false)
	assert.NotNil(t, err)

	headers, err = handler.GetHeaders(eth.HashOrNumber{Number: block101.NumberU64()}, 1, 0, false)
	assert.NotNil(t, err)

	bodies, err = handler.GetBodies([]common.Hash{block101.Hash()})
	assert.NotNil(t, err)
}
