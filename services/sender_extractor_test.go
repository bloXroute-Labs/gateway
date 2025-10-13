package services

import (
	"testing"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"math/big"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Test that SenderExtractor extracts and stores sender for a submitted eth transaction
func TestSenderExtractor_StoresSender(t *testing.T) {
	s := NewSenderExtractor()
	// run extractor loop
	go s.Run()

	// create signed transaction
	priv, _ := crypto.GenerateKey()
	tx := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, priv, nil)
	ethTx, err := types.NewEthTransaction(tx, types.EmptySender)
	require.NoError(t, err)

	// submit transaction for sender extraction
	s.submitEth(ethTx)

	// wait for extractor to pick it up (100ms timeout)
	found, ok := waitForSender(s, ethTx.Hash(), 100*time.Millisecond)
	require.True(t, ok, "expected sender to be extracted and stored")
	expected, err := ethTx.Sender()
	require.NoError(t, err)
	assert.Equal(t, expected, found)
}

// Test that GetSender returns false when hash not present
func TestSenderExtractor_GetSender_NotFound(t *testing.T) {
	s := NewSenderExtractor()
	hash := types.SHA256Hash{1, 2, 3}
	_, ok := s.GetSender(hash)
	assert.False(t, ok)
}

// Test that submitting nil transaction is ignored and does not panic or store anything
func TestSenderExtractor_SubmitNilIgnored(t *testing.T) {
	s := NewSenderExtractor()
	go s.Run()

	s.submitEth(nil)

	// small sleep to allow run loop to process (it should ignore nil)
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, 0, s.senders.Size())
}

// Test GetSendersFromBlockTxs returns senders for transactions present in the extractor
func TestSenderExtractor_GetSendersFromBlockTxs(t *testing.T) {
	s := NewSenderExtractor()

	// create two signed transactions
	priv, _ := crypto.GenerateKey()
	tx1 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 1, priv, nil)
	tx2 := bxmock.NewSignedEthTx(ethtypes.LegacyTxType, 2, priv, nil)

	// compute sender for tx1 and store it
	ethTx1, err := types.NewEthTransaction(tx1, types.EmptySender)
	require.NoError(t, err)
	sender, err := ethTx1.Sender()
	require.NoError(t, err)
	s.senders.Store(types.SHA256Hash(tx1.Hash()), senderEntry{sender: sender, timestamp: time.Now()})

	// build a block containing tx1 and tx2
	header := &ethtypes.Header{Number: big.NewInt(1)}
	body := ethtypes.Body{Transactions: []*ethtypes.Transaction{tx1, tx2}}
	blk := common.NewBlockWithHeader(header).WithBody(body)

	got := s.GetSendersFromBlockTxs(blk)
	assert.Len(t, got, 1)
	assert.Equal(t, sender, got[tx1.Hash().String()])
}

// waitForSender polls GetSender until timeout and returns the sender when found
func waitForSender(s *SenderExtractor, hash types.SHA256Hash, timeout time.Duration) (types.Sender, bool) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(timeout)
	for {
		select {
		case <-ticker.C:
			if sender, ok := s.GetSender(hash); ok {
				return sender, true
			}
		case <-deadline:
			return types.EmptySender, false
		}
	}
}
