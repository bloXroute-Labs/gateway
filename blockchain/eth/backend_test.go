package eth

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	eth2 "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	testUtils "github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	expectTimeout        = 3 * time.Millisecond
	transactionBacklog   = 500
	maxFutureBlockNumber = 100
)

func setupBSCMainnet() (blockchain.Bridge, *ethHandler) {
	return setupNet("BSC-Mainnet")
}

func setupEthMainnet() (blockchain.Bridge, *ethHandler) {
	return setupNet("Mainnet")
}

func setupNet(networkName string) (blockchain.Bridge, *ethHandler) {
	bridge := blockchain.NewBxBridge(Converter{}, false)
	config, _ := network.NewEthereumPreset(networkName)
	_, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
	ctx := context.Background()
	testHandler := newHandler(ctx, &config, core.NewChain(ctx, config.IgnoreBlockTimeout), bridge, NewEthWSManager(blockchainPeersInfo, NewMockWSProvider, bxgateway.WSProviderTimeout, false), make(map[string]struct{}))
	return bridge, (*ethHandler)(testHandler)
}

func testPeer(writeChannelSize int, peerCount int, version uint) (*eth2.Peer, *test.MsgReadWriter) {
	rw := test.NewMsgReadWriter(100, writeChannelSize, time.Second)
	peer := eth2.NewPeer(context.Background(), p2p.NewPeerPipe(test.GenerateEnodeID(), fmt.Sprintf("test peer_%v", peerCount), []p2p.Cap{}, nil), rw, version, 1)
	return peer, rw
}

func TestHandler_ReceiveRequestedTransactions(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()

	var hash types.SHA256Hash
	hashRes, _ := hex.DecodeString("da605de1ee226fd20ba7e82745c742af5255284f8362d66fd8bcf89a318ac5f1")
	copy(hash[:], hashRes)
	content, _ := hex.DecodeString("f8678201d785012a05f20082520894bbdef5f330f08afd93a7696f4ea79af4a41d0f8080808194a0d0f839e1efadc7f1f1cbba67a5dcee50e3c49d3b8b6bc5ebebcf4886d04260a7a07b4e4849bc016cbf17cd27e4fcbb301c5b25a14cc3c9d0b3c244567d7fbad6fc")

	bxTxs := []*types.BxTransaction{
		types.NewRawBxTransaction(hash, content),
	}

	go func() {
		// Simulating gateway layer
		req := <-bridge.ReceiveTransactionHashesRequestFromNode()

		err := bridge.SendRequestedTransactionsToNode(req.RequestID, bxTxs)
		require.NoError(t, err)
	}()

	rlpTxs, err := testHandler.RequestTransactions([]common.Hash{common.Hash(hash.Bytes())})

	contents := make([][]byte, len(rlpTxs))
	for i, rlpTx := range rlpTxs {
		contents[i] = rlpTx
	}

	require.NoError(t, err)
	require.Equal(t, [][]byte{content}, contents)
}

func TestHandler_TxChainID(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)

	txs := []*ethtypes.Transaction{
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, big.NewInt(network.BSCTestnetChainID)),
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, big.NewInt(network.EthMainnetChainID)),
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 3, privateKey, big.NewInt(network.BSCMainnetChainID)),
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 4, privateKey, big.NewInt(-1)),
	}
	assert.False(t, testHandler.isChainIDMatch(txs[0].ChainId().Uint64()))
	assert.False(t, testHandler.isChainIDMatch(txs[1].ChainId().Uint64()))
	assert.True(t, testHandler.isChainIDMatch(txs[2].ChainId().Uint64()))
	assert.True(t, testHandler.isChainIDMatch(txs[3].ChainId().Uint64()))
	tx3Hash := txs[2].Hash().String()

	txsPacket := eth.TransactionsPacket(txs)

	err := testHandler.Handle(peer, &txsPacket)
	assert.Nil(t, err)

	bxTxs := <-bridge.ReceiveNodeTransactions()
	assert.Equal(t, 2, len(bxTxs.Transactions), "two out of four txs should have different chainIDs")
	assert.Equal(t, "0x"+bxTxs.Transactions[0].Hash().String(), tx3Hash)
}

func TestHandler_HandleTransactionsFromNode(t *testing.T) {
	privateKey, _ := crypto.GenerateKey()
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)

	txs := []*ethtypes.Transaction{
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, privateKey, big.NewInt(network.BSCMainnetChainID)),
		bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, privateKey, big.NewInt(network.BSCMainnetChainID)),
	}

	txsPacket := eth.TransactionsPacket(txs)

	err := testHandler.Handle(peer, &txsPacket)
	assert.NoError(t, err)

	bxTxs := <-bridge.ReceiveNodeTransactions()
	assert.Equal(t, len(txs), len(bxTxs.Transactions))

	for i, bxTx := range bxTxs.Transactions {
		tx := txs[i]
		assert.Equal(t, NewSHA256Hash(tx.Hash()), bxTx.Hash())

		encodedTx, _ := rlp.EncodeToBytes(tx)
		assert.Equal(t, encodedTx, []uint8(bxTx.Content()))
	}

	// pooled txs should have exact same behavior
	pooledTxsPacket := eth.PooledTransactionsResponse(txs)
	err = testHandler.Handle(peer, &pooledTxsPacket)
	assert.NoError(t, err)

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
	assert.NoError(t, err)

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

	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)

	txs := blockchain.Transactions{
		Transactions: []*types.BxTransaction{bxTx},
	}

	// assume the transactionBacklog limit above normal capacity by certain amount,
	// be able to handle this loading speed should indicate a good rate of the offloading channel.
	for i := 0; i < transactionBacklog; i++ {
		err := bridge.SendTransactionsFromBDN(txs)
		assert.NoError(t, err)
	}
}

func TestHandler_HandleTransactionHashes66and67(t *testing.T) {
	testFunc := func(protocolVersion uint) {
		bridge, testHandler := setupBSCMainnet()
		peer, _ := testPeer(-1, 1, eth2.ETH66)
		_ = testHandler.peers.register(peer, nil)

		txHashes := types.SHA256HashList{
			types.GenerateSHA256Hash(),
			types.GenerateSHA256Hash(),
		}
		txHashesPacket := make(eth2.NewPooledTransactionHashesPacket66, 0)
		for _, txHash := range txHashes {
			txHashesPacket = append(txHashesPacket, common.BytesToHash(txHash[:]))
		}

		err := testHandler.Handle(peer, &txHashesPacket)
		require.NoError(t, err)

		txAnnouncements := <-bridge.ReceiveTransactionHashesAnnouncement()
		require.Equal(t, peer.ID(), txAnnouncements.PeerID)
		require.Equal(t, txHashes, txAnnouncements.Hashes)
	}

	testFunc(eth2.ETH66)
	testFunc(eth2.ETH67)
}

func TestHandler_HandleTransactionHashes68(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth.ETH68)
	_ = testHandler.peers.register(peer, nil)

	txHashesPacket := eth.NewPooledTransactionHashesPacket{
		Hashes: []common.Hash{
			common.Hash(types.GenerateSHA256Hash().Bytes()),
			common.Hash(types.GenerateSHA256Hash().Bytes()),
			common.Hash(types.GenerateSHA256Hash().Bytes()),
		},
		Types: []uint8{
			ethtypes.DynamicFeeTxType,
			ethtypes.BlobTxType,
			ethtypes.LegacyTxType,
		},
		Sizes: []uint32{
			100,
			200,
			300,
		},
	}

	err := testHandler.Handle(peer, &txHashesPacket)
	require.NoError(t, err)

	txAnnouncements := <-bridge.ReceiveTransactionHashesAnnouncement()
	require.Equal(t, peer.ID(), txAnnouncements.PeerID)
	require.Equal(t, txHashesPacket.Hashes, utils.ConvertSlice(
		txAnnouncements.Hashes,
		func(h types.SHA256Hash) common.Hash {
			return common.Hash(h)
		},
	))
}

func TestHandler_HandleNewBlock_MultiNode_SlowNode(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	blockHeight := uint64(1)

	peer2, _ := testPeer(1, 2, eth2.ETH66)
	_ = testHandler.peers.register(peer2, nil)

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
	err := testHandleNewBlock(testHandler, peer, blockA, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockA.Hash())

	err = testHandleNewBlock(testHandler, peer2, blockA, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	// peer1 sends blocks B-E first, all sent to BDN
	err = testHandleNewBlock(testHandler, peer, blockB, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockB.Hash())

	err = testHandleNewBlock(testHandler, peer, blockC, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockC.Hash())

	err = testHandleNewBlock(testHandler, peer, blockD, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockD.Hash())

	err = testHandleNewBlock(testHandler, peer, blockE, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockE.Hash())

	// peer 2 sends same blocks B-E, none sent to BDN
	err = testHandleNewBlock(testHandler, peer2, blockB, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(testHandler, peer2, blockC, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(testHandler, peer2, blockD, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(testHandler, peer2, blockE, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlock_MultiNode_BroadcastAmongNodes(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)

	peer2, _ := testPeer(1, 2, eth2.ETH66)
	_ = testHandler.peers.register(peer2, nil)

	td := big.NewInt(10000)
	blockHeight := uint64(1)

	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	blockHeight++

	err := testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, block.Hash())
	assertQueuedBlockForBlockchain(t, peer2, block.Hash())
	assertNoBlockQueuedForBlockchain(t, peer)
}

func TestHandler_HandleNewBlock_MultiNode_Fork(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	blockHeight := uint64(1)

	peer2, _ := testPeer(1, 2, eth2.ETH66)
	_ = testHandler.peers.register(peer2, nil)

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

	err := testHandleNewBlock(testHandler, peer, blockA, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockA.Hash())

	// peer2 sends the same block already sent by peer1, not sent to BDN
	err = testHandleNewBlock(testHandler, peer2, blockA, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(testHandler, peer, blockB1, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockB1.Hash())

	// peer2 sends block at the same height, not sent to BDN
	err = testHandleNewBlock(testHandler, peer2, blockB2, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(testHandler, peer, blockC1, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockC1.Hash())

	// peer2 sends block at the same height, not sent to BDN
	err = testHandleNewBlock(testHandler, peer2, blockC2, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)

	err = testHandleNewBlock(testHandler, peer, blockD, td)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, blockD.Hash())

	// peer2 sends the same block already sent by peer1, not sent to BDN
	err = testHandleNewBlock(testHandler, peer2, blockD, td)
	assert.NoError(t, err)
	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlock(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	blockHeight := uint64(1)

	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	header := block.Header()
	td := big.NewInt(10000)

	err := testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)

	assertBlockSentToBDN(t, bridge, block.Hash())

	storedHeaderByHash, err := testHandler.chain.GetHeaders(eth.HashOrNumber{
		Hash: block.Hash(),
	}, 1, 0, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(storedHeaderByHash))
	assert.Equal(t, header, storedHeaderByHash[0])

	storedHeaderByHeight, err := testHandler.chain.GetHeaders(eth.HashOrNumber{
		Number: blockHeight,
	}, 1, 0, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(storedHeaderByHeight))
	assert.Equal(t, header, storedHeaderByHeight[0])
}

func TestHandler_HandleNewBlock_IgnoreAfterTheMerge(t *testing.T) {
	bridge, testHandler := setupEthMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	blockHeight := uint64(1)

	header := bxmock.NewEthBlockHeader(blockHeight, common.Hash{})
	block := bxmock.NewEthBlockWithHeader(header)
	td := big.NewInt(10000)

	err := testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)

	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleNewBlock_TooOld(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	blockHeight := uint64(1)

	// the oldest block that won't be sent to BDN
	header := bxmock.NewEthBlockHeader(blockHeight, common.Hash{})
	header.Time = uint64(time.Now().Add(-testHandler.config.IgnoreBlockTimeout).Add(-time.Second).Unix())
	block := bxmock.NewEthBlockWithHeader(header)
	td := big.NewInt(10000)

	err := testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)

	assertNoBlockSentToBDN(t, bridge)

	// the oldest block that will be sent to BDN
	header = bxmock.NewEthBlockHeader(blockHeight, common.Hash{})
	header.Time = uint64(time.Now().Add(-testHandler.config.IgnoreBlockTimeout).Add(time.Second).Unix())
	block = bxmock.NewEthBlockWithHeader(header)
	td = big.NewInt(10000)

	err = testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)

	assertBlockSentToBDN(t, bridge, block.Hash())
}

func TestHandler_HandleNewBlock_TooFarInFuture(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, _ := testPeer(-1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	blockHeight := uint64(maxFutureBlockNumber + 100)

	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	td := big.NewInt(10000)

	err := testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)

	// ok, always send initial block
	assertBlockSentToBDN(t, bridge, block.Hash())

	block = bxmock.NewEthBlock(blockHeight*2, block.Hash())
	err = testHandleNewBlock(testHandler, peer, block, td)
	assert.NoError(t, err)

	assertNoBlockSentToBDN(t, bridge)
}

func TestHandler_HandleGetBlockHeaders(t *testing.T) {
	_, testHandler := setupBSCMainnet()
	peer, rw := testPeer(1, 1, eth2.ETH66)

	peer.PassCheckpoint()

	_ = testHandler.peers.register(peer, nil)
	peer.Start()
	time.Sleep(15 * time.Millisecond)

	block1 := bxmock.NewEthBlock(1, common.Hash{})

	block2 := bxmock.NewEthBlock(2, block1.Hash())
	bi := core.NewBlockInfo(block2, big.NewInt(1))
	_ = testHandler.chain.SetTotalDifficulty(bi)
	testHandler.chain.AddBlock(bi, core.BSBlockchain)

	block3 := bxmock.NewEthBlock(3, block2.Hash())
	bi = core.NewBlockInfo(block3, big.NewInt(1))
	_ = testHandler.chain.SetTotalDifficulty(bi)
	testHandler.chain.AddBlock(bi, core.BSBlockchain)

	rw.QueueIncomingMessage(uint64(eth.GetBlockHeadersMsg), eth.GetBlockHeadersPacket{
		RequestId: 1,
		GetBlockHeadersRequest: &eth.GetBlockHeadersRequest{
			Origin: eth.HashOrNumber{
				// Requesting number 1 to receive ErrAncientHeaders (we have tail 2 and head 3)
				Number: 1,
			},
			Amount:  1,
			Skip:    0,
			Reverse: false,
		},
	})

	err := eth2.ReadAndHandle(testHandler, peer)
	require.NoError(t, err)

	// need to switch the context and background goroutine to handle the response
	time.Sleep(1 * time.Millisecond)

	// need to take the requestId of the initial request - it was randomly generated in the process
	// of handling GetBlockHeadersMsg message.
	require.True(t, len(peer.ResponseQueue.Keys()) == 1)
	requestID := peer.ResponseQueue.Keys()[0]

	// creating answer for the request
	blockHeaders := eth.BlockHeadersPacket{
		RequestId: requestID,
		BlockHeadersRequest: eth.BlockHeadersRequest{
			block1.Header(),
		},
	}

	eth2.UpdatePeerHeadFromHeaders(blockHeaders, peer)
	handled, err := peer.NotifyResponse(blockHeaders.RequestId, &blockHeaders.BlockHeadersRequest)
	require.NoError(t, err)
	require.True(t, handled)
}

func TestHandler_HandleNewBlockHashes66(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, rw := testPeer(2, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	peer.Start()

	blockHeight := uint64(1)
	block := bxmock.NewEthBlock(blockHeight, common.Hash{})
	err := testHandleNewBlockHashes(testHandler, peer, block.Hash(), blockHeight)

	require.NoError(t, err)

	// expect get headers + get bodies request to peer
	assert.True(t, rw.ExpectWrite(time.Millisecond))
	assert.True(t, rw.ExpectWrite(time.Millisecond))
	assert.Equal(t, 2, len(rw.WriteMessages))

	getHeadersMsg := rw.WriteMessages[0]
	getBodiesMsg := rw.WriteMessages[1]

	assert.Equal(t, uint64(eth.GetBlockHeadersMsg), getHeadersMsg.Code)
	assert.Equal(t, uint64(eth.GetBlockBodiesMsg), getBodiesMsg.Code)

	var getHeaders eth.GetBlockHeadersPacket
	err = getHeadersMsg.Decode(&getHeaders)
	assert.NoError(t, err)
	assert.Equal(t, block.Hash(), getHeaders.Origin.Hash)
	assert.Equal(t, uint64(1), getHeaders.Amount)
	assert.Equal(t, uint64(0), getHeaders.Skip)
	assert.Equal(t, false, getHeaders.Reverse)

	var getBodies eth.GetBlockBodiesPacket
	err = getBodiesMsg.Decode(&getBodies)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(getBodies.GetBlockBodiesRequest))
	assert.Equal(t, block.Hash(), getBodies.GetBlockBodiesRequest[0])

	headersID := getHeaders.RequestId
	bodiesID := getBodies.RequestId

	// send header/body from peer (out of order with request IDs)
	rw.QueueIncomingMessage(uint64(eth.BlockBodiesMsg), eth2.BlockBodiesPacket{
		RequestID: bodiesID,
		BlockBodiesResponse: eth2.BlockBodiesResponse{&eth2.BlockBody{
			Transactions: block.Transactions(),
			Uncles:       block.Uncles(),
		}},
	})
	rw.QueueIncomingMessage(uint64(eth.BlockHeadersMsg), eth.BlockHeadersPacket{
		RequestId:           headersID,
		BlockHeadersRequest: eth.BlockHeadersRequest{block.Header()},
	})

	// expect bodies message, then peer message
	err = eth2.ReadAndHandle(testHandler, peer)
	assert.NoError(t, err)

	err = eth2.ReadAndHandle(testHandler, peer)
	assert.NoError(t, err)

	expectedBlock, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block, nil))
	newBlock := assertBlockSentToBDN(t, bridge, block.Hash())
	assert.True(t, expectedBlock.Equals(newBlock.Block))

	// difficulty is unknown from header/body handling
	assert.Equal(t, big.NewInt(0), newBlock.Block.TotalDifficulty)

	// check peer state is cleaned up
	assert.Equal(t, 0, len(peer.ResponseQueue.Keys()))
}

func TestHandler_processBDNBlock(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, rw := testPeer(2, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	peer.Start()

	// generate bx block for processing
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	blockHash := ethBlock.Hash()
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlock, td))

	// indicate previous head from the status message
	peer.ConfirmedHead.Store(core.BlockRef{Hash: ethBlock.ParentHash()})

	(*handler)(testHandler).processBDNBlock(bxBlock)

	// expect the message to be sent to a peer
	blockPacket := assertBlockSentToBlockchain(t, rw, ethBlock.Hash())
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	// contents stored in cache
	assert.True(t, testHandler.chain.HasBlock(blockHash))

	storedHeader, ok := testHandler.chain.GetBlockHeader(10, blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Header(), storedHeader)

	storedBody, ok := testHandler.chain.GetBlockBody(blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Body().Uncles, storedBody.Uncles)
	for i, tx := range ethBlock.Body().Transactions {
		assert.Equal(t, tx.Hash(), storedBody.Transactions[i].Hash())
	}

	// confirm block, should send back to BDN and update head
	err := testHandler.Handle(peer, &eth.BlockHeadersRequest{ethBlock.Header()})
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, ethBlock.Hash())
}

func TestHandler_processBDNBlock_MultiNode(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, peerRW := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)

	// add extra peers
	peer2, peerRW2 := testPeer(1, 2, eth2.ETH66)
	_ = testHandler.peers.register(peer2, nil)
	peer3, peerRW3 := testPeer(1, 3, eth2.ETH66)
	_ = testHandler.peers.register(peer3, nil)

	peer.Start()
	peer2.Start()
	peer3.Start()

	// generate bx block for processing
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	blockHash := ethBlock.Hash()
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlock, td))

	// indicate previous head from the status message
	peer.ConfirmedHead.Store(core.BlockRef{Hash: ethBlock.ParentHash()})
	peer2.ConfirmedHead.Store(core.BlockRef{Hash: ethBlock.ParentHash()})

	(*handler)(testHandler).processBDNBlock(bxBlock)

	// expect the message to be sent to peer 1
	blockPacket := assertBlockSentToBlockchain(t, peerRW, ethBlock.Hash())
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	// expect the message to be sent to peer 2
	blockPacket2 := assertBlockSentToBlockchain(t, peerRW2, ethBlock.Hash())
	assert.Equal(t, blockHash, blockPacket2.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket2.Block.Transactions()))
	assert.Equal(t, td, blockPacket2.TD)

	// expect no block sent to peer3 because confirmedHead is not a parent
	assertNoBlockSentToBlockchain(t, peerRW3)

	// contents stored in cache
	assert.True(t, testHandler.chain.HasBlock(blockHash))

	storedHeader, ok := testHandler.chain.GetBlockHeader(10, blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Header(), storedHeader)

	storedBody, ok := testHandler.chain.GetBlockBody(blockHash)
	assert.True(t, ok)
	assert.Equal(t, ethBlock.Body().Uncles, storedBody.Uncles)
	for i, tx := range ethBlock.Body().Transactions {
		assert.Equal(t, tx.Hash(), storedBody.Transactions[i].Hash())
	}

	// confirm block, should send back to BDN and update head
	err := testHandler.Handle(peer, &eth.BlockHeadersRequest{ethBlock.Header()})
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, ethBlock.Hash())
}

func TestHandler_processBDNBlockResolveDifficulty(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, rw := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	peer.Start()

	// preprocess a parent for calculating difficulty
	parentBlock := bxmock.NewEthBlock(9, common.Hash{})
	parentHash := parentBlock.Hash()
	parentTD := big.NewInt(1000)

	err := testHandleNewBlock(testHandler, peer, parentBlock, parentTD)
	assert.NoError(t, err)

	// generate bx block for processing
	ethBlock := bxmock.NewEthBlock(10, parentHash)
	blockHash := ethBlock.Hash()
	bxBlock, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlock, nil))

	(*handler)(testHandler).processBDNBlock(bxBlock)

	blockPacket := assertBlockSentToBlockchain(t, rw, blockHash)
	assert.Equal(t, blockHash, blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlock.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, new(big.Int).Add(parentTD, ethBlock.Difficulty()), blockPacket.TD)
}

func TestHandler_processBDNBlockUnresolvableDifficulty(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()
	peer, rw := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	peer.Start()

	// generate bx block for processing
	height := uint64(10)
	ethBlock := bxmock.NewEthBlock(height, common.Hash{})
	blockHash := ethBlock.Hash()
	bxBlock, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlock, nil))

	(*handler)(testHandler).processBDNBlock(bxBlock)

	// expect a message to be sent to a peer
	assert.True(t, rw.ExpectWrite(time.Millisecond))
	assert.Equal(t, 1, len(rw.WriteMessages))
	msg := rw.WriteMessages[0]
	assert.Equal(t, uint64(eth.NewBlockHashesMsg), msg.Code)

	var blockHashesPacket eth.NewBlockHashesPacket
	err := msg.Decode(&blockHashesPacket)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(blockHashesPacket))
	assert.Equal(t, height, blockHashesPacket[0].Number)
	assert.Equal(t, blockHash, blockHashesPacket[0].Hash)
}

func createBxBlockFromEthHeader(header *ethtypes.Header, chain *core.Chain, bridge blockchain.Bridge) (*types.BxBlock, error) {
	body, ok := chain.GetBlockBody(header.Hash())
	if !ok {
		return nil, core.ErrBodyNotFound
	}
	ethBlock := bxcommoneth.NewBlockWithHeader(header).WithBody(ethtypes.Body{Transactions: body.Transactions, Uncles: body.Uncles})

	sidecars, ok := chain.GetBlockBlobSidecars(header.Hash())
	if ok {
		ethBlock = ethBlock.WithSidecars(sidecars)
	}

	bi := core.BlockInfo{Block: ethBlock, TD: header.Difficulty}
	bxBlock, err := bridge.BlockBlockchainToBDN(&bi)
	if err != nil {
		return nil, fmt.Errorf("failed to convert block %v to BDN format: %v", header.Hash().String(), err)
	}

	return bxBlock, nil
}

func TestHandler_BlockAtDepth(t *testing.T) {
	c := core.NewChain(context.Background(), 10 /*5, 5, time.Hour, 1000*/)
	blockConfirmationCounts := 4
	_, testHandler := setupBSCMainnet()
	peer, _ := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
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

	testHandler.chain = c
	bxb1, err := createBxBlockFromEthHeader(block1.Header(), c, testHandler.bridge)
	assert.NoError(t, err)

	// head of the chain state is block 5, for depth 4, the block should be 1
	b1, err := testHandler.blockAtDepth(blockConfirmationCounts)
	assert.NoError(t, err)
	assert.Equal(t, b1.Hash(), bxb1.Hash())

	// for depth 5, the chain state is not deep enough
	blockConfirmationCounts = 5
	_, err = testHandler.blockAtDepth(blockConfirmationCounts)
	assert.NotNil(t, err)
}

func TestHandler_blockForks(t *testing.T) {
	var err error
	bridge, testHandler := setupBSCMainnet()
	peer, rw := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer, nil)
	testHandler.config.SendBlockConfirmation = true
	testHandler.config.BlockConfirmationsCount = 3
	peer.Start()

	block1 := bxmock.NewEthBlock(uint64(1), common.Hash{})
	block2a := bxmock.NewEthBlock(uint64(2), block1.Hash())
	block2b := bxmock.NewEthBlock(uint64(2), block1.Hash())
	block3a := bxmock.NewEthBlock(uint64(3), block2a.Hash())
	block3b := bxmock.NewEthBlock(uint64(3), block2b.Hash())
	block4b := bxmock.NewEthBlock(uint64(4), block3b.Hash())

	// sequence of blocks received from blockchain
	newBlock1 := eth2.NewBlockPacket{Block: &block1.Block, TD: big.NewInt(100)}
	newBlock2a := eth2.NewBlockPacket{Block: &block2a.Block, TD: big.NewInt(201)}
	// newBlock3a sent as NewBlockHashes instead of block packet
	newBlock4b := eth2.NewBlockPacket{Block: &block4b.Block, TD: big.NewInt(402)}

	// sequence of blocks received from BDN
	bxBlock1, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block1, big.NewInt(100)))
	bxBlock2a, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block2a, big.NewInt(201)))
	bxBlock2b, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block2b, big.NewInt(202)))
	bxBlock3a, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block3a, big.NewInt(301)))
	bxBlock3b, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block3b, big.NewInt(302)))
	bxBlock4b, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(block4b, big.NewInt(402)))

	/*
		sending sequences:
			1 blockchain
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
	err = testHandleNewBlock(testHandler, peer, block1, newBlock1.TD)
	assert.NoError(t, err)
	assertBlockSentToBDN(t, bridge, block1.Hash())
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: duplicate, nothing new
	(*handler)(testHandler).processBDNBlock(bxBlock1)

	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: sent to blockchain node (next in confirmed chain)
	(*handler)(testHandler).processBDNBlock(bxBlock2a)

	assertNoBlockSentToBDN(t, bridge)
	assertBlockSentToBlockchain(t, rw, block2a.Hash())
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: sent to gateway as confirmation
	_ = testHandleNewBlock(testHandler, peer, block2a, newBlock2a.TD)
	assertBlockSentToBDN(t, bridge, block2a.Hash())
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: nothing sent anywhere (parked for blockchain, unconfirmed for BDN)
	(*handler)(testHandler).processBDNBlock(bxBlock2b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: block sent to blockchain node
	(*handler)(testHandler).processBDNBlock(bxBlock3a)
	assertNoBlockSentToBDN(t, bridge)
	assertBlockSentToBlockchain(t, rw, block3a.Hash())
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: new block hashes confirms blocks, and sends to BDN
	_ = testHandleNewBlockHashes(testHandler, peer, block3a.Hash(), block3a.NumberU64())
	assertBlockSentToBDN(t, bridge, block3a.Hash())
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: nothing sent anywhere (parked + unconfirmed)
	(*handler)(testHandler).processBDNBlock(bxBlock3b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: nothing sent anywhere (unconfirmed, blockchain node is on 3a/4a path)
	(*handler)(testHandler).processBDNBlock(bxBlock4b)
	assertNoBlockSentToBDN(t, bridge)
	assertNoBlockSentToBlockchain(t, rw)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	// expectation: send 2b, 3b, 4b to BDN (confirmed now)
	err = testHandler.Handle(peer, &newBlock4b)
	assert.NoError(t, err)
	_ = testHandleNewBlock(testHandler, peer, block4b, newBlock4b.TD)
	assertBlockSentToBDN(t, bridge, block2b.Hash())
	assertBlockSentToBDN(t, bridge, block3b.Hash())
	assertBlockSentToBDN(t, bridge, block4b.Hash())
	assertNoBlockSentToBlockchain(t, rw)
	assertConfirmationBlockSentToGateway(t, bridge, bxBlock1)
}

func TestHandler_ConfirmBlockFromWS(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()

	peer1, rw1 := testPeer(1, 1, eth2.ETH66)
	err := testHandler.peers.register(peer1, nil)
	require.NoError(t, err)
	peer2, rw2 := testPeer(1, 2, eth2.ETH66)
	err = testHandler.peers.register(peer2, nil)
	require.NoError(t, err)
	peer3, rw3 := testPeer(1, 3, eth2.ETH66)
	err = testHandler.peers.register(peer3, nil)
	require.NoError(t, err)

	peer1.Start()
	peer2.Start()
	peer3.Start()

	height := big.NewInt(1)
	ethBlockA := bxmock.NewEthBlock(height.Uint64(), common.Hash{})

	peer1.ConfirmedHead.Store(core.BlockRef{Hash: ethBlockA.ParentHash()})
	peer2.ConfirmedHead.Store(core.BlockRef{Hash: ethBlockA.ParentHash()})
	peer3.ConfirmedHead.Store(core.BlockRef{Hash: ethBlockA.ParentHash()})

	td := big.NewInt(10000)
	blockA, err := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlockA, td))
	require.NoError(t, err)
	(*handler)(testHandler).processBDNBlock(blockA)

	// process all queue entries first
	time.Sleep(100 * time.Millisecond)

	blockPacket := assertBlockSentToBlockchain(t, rw1, ethBlockA.Hash())
	assert.Equal(t, ethBlockA.Hash(), blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlockA.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	blockPacket = assertBlockSentToBlockchain(t, rw2, ethBlockA.Hash())
	assert.Equal(t, ethBlockA.Hash(), blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlockA.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	blockPacket = assertBlockSentToBlockchain(t, rw3, ethBlockA.Hash())
	assert.Equal(t, ethBlockA.Hash(), blockPacket.Block.Hash())
	assert.Equal(t, len(ethBlockA.Transactions()), len(blockPacket.Block.Transactions()))
	assert.Equal(t, td, blockPacket.TD)

	(*handler)(testHandler).confirmBlockFromWS(ethBlockA.Hash(), height, peer1)
	time.Sleep(time.Millisecond)
	assert.Equal(t, core.BlockRef{Height: 1, Hash: ethBlockA.Hash()}, peer1.GetConfirmedHead())
	assert.Equal(t, core.BlockRef{Height: 0, Hash: ethBlockA.ParentHash()}, peer2.GetConfirmedHead())
	assert.Equal(t, core.BlockRef{Height: 0, Hash: ethBlockA.ParentHash()}, peer3.GetConfirmedHead())
	assertBlockSentToBDN(t, bridge, ethBlockA.Hash())
	assertConfirmationBlockSentToGateway(t, bridge, blockA)

	ethBlockB := bxmock.NewEthBlock(height.Uint64(), common.Hash{})
	blockB, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlockB, td))
	(*handler)(testHandler).processBDNBlock(blockB)
	(*handler)(testHandler).confirmBlockFromWS(ethBlockB.Hash(), height, peer2)
	time.Sleep(time.Millisecond)
	assert.Equal(t, core.BlockRef{Height: 1, Hash: ethBlockA.Hash()}, peer1.GetConfirmedHead())
	assert.Equal(t, core.BlockRef{Height: 1, Hash: ethBlockB.Hash()}, peer2.GetConfirmedHead())
	assert.Equal(t, core.BlockRef{Height: 0, Hash: ethBlockA.ParentHash()}, peer3.GetConfirmedHead())
	assertNoBlockSentToBDN(t, bridge)
	assertNoConfirmationBlockSentToBDN(t, bridge)

	height = big.NewInt(2)
	ethBlockB2 := bxmock.NewEthBlock(height.Uint64(), ethBlockB.Hash())
	blockB2, _ := bridge.BlockBlockchainToBDN(core.NewBlockInfo(ethBlockB2, td))
	(*handler)(testHandler).processBDNBlock(blockB2)
	(*handler)(testHandler).confirmBlockFromWS(ethBlockB2.Hash(), height, peer2)
	time.Sleep(time.Millisecond)
	assert.Equal(t, core.BlockRef{Height: 1, Hash: ethBlockA.Hash()}, peer1.GetConfirmedHead())
	assert.Equal(t, core.BlockRef{Height: 2, Hash: ethBlockB2.Hash()}, peer2.GetConfirmedHead())
	assert.Equal(t, core.BlockRef{Height: 0, Hash: ethBlockA.ParentHash()}, peer3.GetConfirmedHead())
	assertBlockSentToBDN(t, bridge, ethBlockB.Hash())
	assertBlockSentToBDN(t, bridge, ethBlockB2.Hash())
	assertConfirmationBlockSentToGateway(t, bridge, blockB2)
}

func TestHandler_DisconnectInboundPeer(t *testing.T) {
	bridge, testHandler := setupBSCMainnet()

	peer1, _ := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer1, nil)

	peer1.Start()
	go func() {
		(*handler)(testHandler).handleBDNBridge(context.Background())
	}()

	err := bridge.SendDisconnectEvent(types.NodeEndpoint{PublicKey: peer1.IPEndpoint().PublicKey})
	assert.NoError(t, err)
}

func TestHandler_ConnectionCloseOnContextClosure(t *testing.T) {
	bridge := blockchain.NewBxBridge(Converter{}, false)
	config, _ := network.NewEthereumPreset("BSC-Mainnet")
	_, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(1)
	ctx := context.Background()
	testHandler := newHandler(ctx, &config, core.NewChain(ctx, config.IgnoreBlockTimeout), bridge, NewEthWSManager(blockchainPeersInfo, NewMockWSProvider, bxgateway.WSProviderTimeout, false), make(map[string]struct{}))

	peer1, _ := testPeer(1, 1, eth2.ETH66)
	_ = testHandler.peers.register(peer1, nil)

	peer1.Start()
	ctx, cancel := context.WithCancel(ctx)

	// Create a done channel to signal completion
	for _, provider := range testHandler.wsManager.Providers() {
		// Run the runEthSub method in a goroutine
		provider.UpdateSyncStatus(blockchain.Synced)
	}
	provider, ok := testHandler.wsManager.SyncedProvider()
	require.True(t, ok)
	// Run the runEthSub method in a goroutine
	wg := new(sync.WaitGroup)
	wg.Add(1) // calling before the goroutine starts to avoid the race
	go func() {
		testHandler.runEthSub(ctx, provider, wg)
	}()

	testUtils.WaitUntilTrueOrFail(t, func() bool {
		return provider.IsOpen()
	})

	cancel()

	// Wait for the goroutine to complete
	wg.Wait()
	require.False(t, provider.IsOpen())
}

// variety of handling functions here to trigger handlers in handlers.go instead of directly invoking the handler (useful for setting state on Peer during handling)
func testHandleNewBlock(handler *ethHandler, peer *eth2.Peer, block *bxcommoneth.Block, td *big.Int) error {
	newBlockPacket := &eth2.NewBlockPacket{
		Block:    &block.Block,
		TD:       td,
		Sidecars: block.Sidecars(),
	}

	return eth2.HandleMassage(handler, encodeRLP(eth.NewBlockMsg, newBlockPacket), peer)
}

func testHandleNewBlockHashes(handler *ethHandler, peer *eth2.Peer, hash common.Hash, height uint64) error {
	newBlockHashesPacket := &eth.NewBlockHashesPacket{
		{
			Hash:   hash,
			Number: height,
		},
	}

	return eth2.HandleMassage(handler, encodeRLP(eth.NewBlockHashesMsg, newBlockHashesPacket), peer)
}

func encodeRLP(code uint64, data interface{}) p2p.Msg {
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
	assert.NoError(t, err)

	assert.Equal(t, hash, newBlocks.Block.Hash())
	return newBlocks
}

func assertNoBlockSentToBlockchain(t *testing.T, rw *test.MsgReadWriter) {
	assert.False(t, rw.ExpectWrite(expectTimeout))
}

func assertQueuedBlockForBlockchain(t *testing.T, peer *eth2.Peer, hash common.Hash) {
	select {
	case sentBlock := <-peer.ReceiveNewBlock():
		assert.Equal(t, hash.Bytes(), sentBlock.Block.Hash().Bytes())
	case <-time.After(expectTimeout):
		assert.FailNow(t, "Peer did not receive block", "hash=%v", hash)
	}
}

func assertNoBlockQueuedForBlockchain(t *testing.T, peer *eth2.Peer) {
	select {
	case sentBlock := <-peer.ReceiveNewBlock():
		assert.FailNow(t, "Peer received unexpected block", "hash=%v", sentBlock.Block.Hash())
	case <-time.After(expectTimeout):
	}
}

func addBlock(c *core.Chain, block *bxcommoneth.Block) int {
	return addBlockWithTD(c, block, nil)
}

func addBlockWithTD(c *core.Chain, block *bxcommoneth.Block, td *big.Int) int {
	bi := core.NewBlockInfo(block, td)
	_ = c.SetTotalDifficulty(bi)
	return c.AddBlock(bi, core.BSBlockchain)
}
