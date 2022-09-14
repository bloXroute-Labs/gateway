package services

import (
	"math/big"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestRLPBlockProcessor_BxBlockToBroadcast(t *testing.T) {
	store := newTestBxTxStore()
	bp := NewRLPBlockProcessor(&store)
	clock := utils.MockClock{}
	clock.SetTime(time.Now())

	blockHash := types.GenerateSHA256Hash()
	header, _ := rlp.EncodeToBytes(test.GenerateBytes(300))
	trailer, _ := rlp.EncodeToBytes(test.GenerateBytes(350))

	// note that txs[0] is a huge tx
	txs := []*types.BxBlockTransaction{
		types.NewBxBlockTransaction(types.GenerateSHA256Hash(), test.GenerateBytes(25000)),
		types.NewBxBlockTransaction(types.GenerateSHA256Hash(), test.GenerateBytes(250)),
		types.NewBxBlockTransaction(types.GenerateSHA256Hash(), test.GenerateBytes(250)),
		types.NewBxBlockTransaction(types.GenerateSHA256Hash(), test.GenerateBytes(250)),
		types.NewBxBlockTransaction(types.GenerateSHA256Hash(), test.GenerateBytes(250)),
	}
	blockSize := int(rlp.ListSize(300 + rlp.ListSize(35000+250+250+250+250) + 350))

	// create delay the txs[0], so it passes the age check
	store.Add(txs[0].Hash(), txs[0].Content(), 1, testNetworkNum, false, 0, clock.Now().Add(-2*time.Second), 0, types.EmptySender)

	// The txs[2] will not be included in shortID since it's too recent
	store.Add(txs[3].Hash(), txs[3].Content(), 2, testNetworkNum, false, 0, clock.Now(), 0, types.EmptySender)

	bxBlock, err := types.NewBxBlock(blockHash, types.BxBlockTypeEth, header, txs, trailer, big.NewInt(10000), big.NewInt(10), blockSize)
	assert.Nil(t, err)

	// assume the blockchain network MinTxAgeSecond is 2
	broadcastMessage, shortIDs, err := bp.BxBlockToBroadcast(bxBlock, testNetworkNum, time.Second*2)
	assert.Nil(t, err)

	// only the first shortID exists, the second Tx didn't get added into shortID
	assert.Equal(t, 1, len(shortIDs))
	assert.Contains(t, shortIDs, types.ShortID(1))
	assert.NotContains(t, shortIDs, types.ShortID(2))

	// check that block is definitely compressed (tx 0 is huge)
	assert.Less(t, len(broadcastMessage.Block()), 2000)

	// duplicate, skip this time
	// assume the blockchain network MinTxAgeSecond is 2
	_, _, err = bp.BxBlockToBroadcast(bxBlock, testNetworkNum, time.Second*2)
	assert.Equal(t, ErrAlreadyProcessed, err)

	// duplicate, skip from other direction too
	_, _, err = bp.BxBlockFromBroadcast(broadcastMessage)
	assert.Equal(t, ErrAlreadyProcessed, err)

	// decompress same block works after clearing processed list
	bp.(*rlpBlockProcessor).processedBlocks = NewHashHistory("processedBlocks", 30*time.Minute)
	decodedBxBlock, missingShortIDs, err := bp.BxBlockFromBroadcast(broadcastMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(missingShortIDs))

	assert.Equal(t, header, decodedBxBlock.Header)
	assert.Equal(t, trailer, decodedBxBlock.Trailer)

	for i, tx := range decodedBxBlock.Txs {
		assert.Equal(t, txs[i].Content(), tx.Content())
	}
}

func TestRLPBlockProcessor_BroadcastToBxBlockMissingShortIDs(t *testing.T) {
	broadcast := &bxmessage.Broadcast{}
	_ = broadcast.Unpack(common.Hex2Bytes(fixtures.BroadcastMessageWithShortIDs), 0)

	store := newTestBxTxStore()
	bp := NewRLPBlockProcessor(&store)

	bxBlock, missingShortIDs, err := bp.BxBlockFromBroadcast(broadcast)
	assert.NotNil(t, err)
	assert.Nil(t, bxBlock)
	assert.Equal(t, 2, len(missingShortIDs))

	// ok to reprocess, not successfully seen yet
	_, _, err = bp.BxBlockFromBroadcast(broadcast)
	assert.NotEqual(t, ErrAlreadyProcessed, err)
}

func TestRLPBlockProcessor_BroadcastToBxBlockShortIDs(t *testing.T) {
	broadcast := &bxmessage.Broadcast{}
	_ = broadcast.Unpack(common.Hex2Bytes(fixtures.BroadcastMessageWithShortIDs), 0)

	store := newTestBxTxStore()
	bp := NewRLPBlockProcessor(&store)

	txHash1, _ := types.NewSHA256HashFromString(fixtures.BroadcastTransactionHash1)
	txContent1 := common.Hex2Bytes(fixtures.BroadcastTransactionContent1)
	txHash2, _ := types.NewSHA256HashFromString(fixtures.BroadcastTransactionHash2)
	txContent2 := common.Hex2Bytes(fixtures.BroadcastTransactionContent2)

	txContents := [][]byte{txContent1, txContent2}

	store.Add(txHash1, txContent1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	store.Add(txHash2, txContent2, 2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)

	bxBlock, missingShortIDs, err := bp.BxBlockFromBroadcast(broadcast)
	assert.Nil(t, err)
	assert.NotNil(t, bxBlock)
	assert.Equal(t, 0, len(missingShortIDs))

	// note: this hash does not actually match the block contents (test data was generated as such)
	assert.Equal(t, broadcast.Hash(), bxBlock.Hash())

	// check transactions have been decompressed
	assert.Equal(t, 2, len(bxBlock.Txs))
	for i, blockTx := range bxBlock.Txs {
		assert.Equal(t, txContents[i], blockTx.Content())
	}

	// verify integrity of other fields
	var ethHeader ethtypes.Header
	if err := rlp.DecodeBytes(bxBlock.Header, &ethHeader); err != nil {
		t.Fatal(err)
	}

	var uncles []*ethtypes.Header
	if err := rlp.DecodeBytes(bxBlock.Trailer, &uncles); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(uncles))
	assert.Equal(t, common.Hex2Bytes(fixtures.BroadcastUncleParentHash), uncles[0].ParentHash.Bytes())

	assert.Equal(t, fixtures.BroadcastDifficulty, bxBlock.TotalDifficulty)
	assert.Equal(t, fixtures.BroadcastBlockNumber, bxBlock.Number)

	// duplicate, skip this time
	_, _, err = bp.BxBlockFromBroadcast(broadcast)
	assert.Equal(t, ErrAlreadyProcessed, err)
}

func TestRLPBlockProcessor_BroadcastToBxBlockFullTxs(t *testing.T) {
	broadcast := &bxmessage.Broadcast{}
	_ = broadcast.Unpack(common.Hex2Bytes(fixtures.BroadcastMessageFullTxs), 0)

	store := newTestBxTxStore()
	bp := NewRLPBlockProcessor(&store)

	txHash1, _ := types.NewSHA256HashFromString(fixtures.BroadcastTransactionHash1)
	txContent1 := common.Hex2Bytes(fixtures.BroadcastTransactionContent1)

	store.Add(txHash1, txContent1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)

	bxBlock, missingShortIDs, err := bp.BxBlockFromBroadcast(broadcast)
	assert.Nil(t, err)
	assert.NotNil(t, bxBlock)
	assert.Equal(t, 0, len(missingShortIDs))

	// note: this hash does not actually match the block contents (test data was generated as such)
	assert.Equal(t, broadcast.Hash(), bxBlock.Hash())

	assert.Equal(t, 1, len(bxBlock.Txs))
	assert.NotEqual(t, txContent1, bxBlock.Txs[0].Content())

	// verify integrity of other fields
	var ethHeader ethtypes.Header
	if err := rlp.DecodeBytes(bxBlock.Header, &ethHeader); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, common.Hex2Bytes(fixtures.BroadcastMessageFullTxsBlockHash), ethHeader.Hash().Bytes())

	var uncles []*ethtypes.Header
	if err := rlp.DecodeBytes(bxBlock.Trailer, &uncles); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(uncles))

	assert.Equal(t, fixtures.BroadcastDifficulty, bxBlock.TotalDifficulty)
	assert.Equal(t, fixtures.BroadcastBlockNumber, bxBlock.Number)
	assert.Equal(t, 2148, bxBlock.Size())
}

func TestRLPBlockProcessor_ProcessBroadcast(t *testing.T) {
	broadcast := &bxmessage.Broadcast{}
	_ = broadcast.Unpack(common.Hex2Bytes(fixtures.BroadcastMessageWithShortIDs), 0)

	txHash1, _ := types.NewSHA256HashFromString(fixtures.BroadcastTransactionHash1)
	txContent1 := common.Hex2Bytes(fixtures.BroadcastTransactionContent1)
	txHash2, _ := types.NewSHA256HashFromString(fixtures.BroadcastTransactionHash2)
	txContent2 := common.Hex2Bytes(fixtures.BroadcastTransactionContent2)

	store := newTestBxTxStore()
	store.Add(txHash1, txContent1, 1, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)
	store.Add(txHash2, txContent2, 2, testNetworkNum, false, types.TFPaidTx, time.Now(), testChainID, types.EmptySender)

	bp := NewRLPBlockProcessor(&store)

	bxBlock, _, err := bp.ProcessBroadcast(broadcast)
	assert.Nil(t, err)

	assert.NotNil(t, bxBlock)
	assert.Equal(t, broadcast.Hash(), bxBlock.Hash())

	_, _, err = bp.ProcessBroadcast(broadcast)
	assert.Equal(t, ErrAlreadyProcessed, err)
}
