package bxmessage

import (
	"bytes"
	"testing"
	"time"

	bxclock "github.com/bloXroute-Labs/bxcommon-go/clock"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/assert"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

var nullByteAccountID = bytes.Repeat([]byte("\x00"), 36)

func TestTx_AccountIDEmpty(t *testing.T) {
	tx := Tx{}
	assert.Equal(t, bxtypes.AccountID(""), tx.AccountID())
}

func TestTx_AccountIDNullBytes(t *testing.T) {
	accountID := [AccountIDLen]byte{}
	copy(accountID[:], nullByteAccountID)

	tx := Tx{
		accountID: accountID,
	}
	assert.Equal(t, bxtypes.AccountID(""), tx.AccountID())

	b, _ := tx.Pack(MinProtocol)
	tx2 := Tx{}
	_ = tx2.Unpack(b, MinProtocol)
	assert.Equal(t, bxtypes.AccountID(""), tx2.AccountID())
}

func TestTx_SourceIDValid(t *testing.T) {
	sourceID := "4c5df5f8-2fd9-4739-a319-8beeba554a88"
	tx := Tx{}
	err := tx.SetSourceID(bxtypes.NodeID(sourceID))
	assert.NoError(t, err)
	assert.Equal(t, bxtypes.NodeID(sourceID), tx.SourceID())

	newSourceID := "9ee4ec57-d189-428e-92e6-d496670b5022"
	err = tx.SetSourceID(bxtypes.NodeID(newSourceID))
	assert.NoError(t, err)
	assert.Equal(t, bxtypes.NodeID(newSourceID), tx.SourceID())

	newInvalidSourceID := "invalid-source-id"
	err = tx.SetSourceID(bxtypes.NodeID(newInvalidSourceID))
	assert.NotNil(t, err)
}

func TestTx_TimeStamp(t *testing.T) {
	mockClock := bxclock.MockClock{}
	clock = &mockClock

	// case 1, normal situation
	// test Tx that got packed at certain time, and unpacked after one second
	// 2022-03-22 11:35:33.797682 -0500 CDT m=+298.247378704
	// 1011011011110110000+01001001111101100111011101101000+0010110000
	mockClock.SetTime(time.Unix(0, 1647966890567246000)) // 2022-03-22 11:35:33.79
	packTime := mockClock.Now()
	tx := Tx{
		timestamp: packTime,
	}
	b, _ := tx.Pack(MinProtocol)
	tx1 := Tx{}
	mockClock.IncTime(time.Second * 1)
	_ = tx1.Unpack(b, MinProtocol)
	// microsecond precision
	assert.Equal(t, packTime.UnixNano()>>10, tx1.Timestamp().UnixNano()>>10)

	// case 2, overflow
	// test Tx packed at 2022-03-18 10:42:54.22 , with has binary format of 0b1011011011101100000+11111111111111111111111111111111+1111111111
	// after one second 5 second: 2022-03-18 10:42:59.22 , the time format will be 1011011011101100001+00000000010010101000000101111100+0111111111, the merged timestamp will be
	// 1011011011101100001+11111111111111111111111111111111+0000000000 which is 2022-03-18 11:56:12.26884608 -0500, and this is far in the future because
	// the binary overflow, this case should be handled by subtract the TxTime by 1 hour and 13 mins
	mockClock.SetTime(time.Unix(0, 0x16dd83ffffffffff)) // 2022-03-18 10:42:54.22
	packTime = mockClock.Now()                          //
	tx = Tx{
		timestamp: packTime,
	}
	b, _ = tx.Pack(MinProtocol)
	tx2 := Tx{}
	// advance time by 1 nanosecond to create overflow
	mockClock.SetTime(time.Unix(0, 0x16dd840000000000)) // 2022-03-18 10:42:59.22
	_ = tx2.Unpack(b, MinProtocol)
	assert.Equal(t, packTime.UnixNano()>>10, tx2.Timestamp().UnixNano()>>10)

	// case 3, underflow
	// test Tx packed at 0x16dd840000000000, with binary format of 1011011011101100001+00000000010010101000000101111100+0111111111
	// and when the receiver unpack it, its time is 5 second earlier with local machine's timestamp being 2022-03-18 10:42:54.22 ,
	// with has binary format of 0b1011011011101100000+11111111111111111111111111111111+1111111111, after the merge the Tx time is
	// 0b1011011011101100000+00000000010010101000000101111100+0000000000, which is 2022-03-18 09:29:41.175824384 -0500 CDT, and it's more than
	// an hour before the current time
	packTime = time.Unix(0, 0x16dd840000000000) // 2022-03-18 10:42:59.22
	tx = Tx{
		timestamp: packTime,
	}
	b, _ = tx.Pack(MinProtocol)
	tx3 := Tx{}
	// unpack time is five seconds before pack time
	mockClock.SetTime(time.Unix(0, 0x16dd83ffffffffff)) // 2022-03-18 10:42:54.22
	_ = tx3.Unpack(b, MinProtocol)
	assert.Equal(t, packTime.UnixNano()>>10, tx3.Timestamp().UnixNano()>>10)
}

func TestTx_NextValidator(t *testing.T) {
	var flags types.TxFlags
	flags |= types.TFNextValidator

	tx1 := Tx{
		walletIDs: []string{"0x8b6c8fd93d6f4cea42bbb345dbc6f0dfdb5bec73", "0xe9ae3261a475a27bb1028f140bc2a7c843318afd"},
		fallback:  10,
		flags:     flags,
		sender:    types.Sender{1},
		content:   []byte{123},
	}

	b, _ := tx1.Pack(NextValidatorMultipleProtocol)
	txMsg3 := Tx{}
	_ = txMsg3.Unpack(b, NextValidatorMultipleProtocol)
	assert.Equal(t, 10, int(txMsg3.fallback))
	assert.Equal(t, "0x8b6c8fd93d6f4cea42bbb345dbc6f0dfdb5bec73", txMsg3.walletIDs[0])
	assert.Equal(t, "0xe9ae3261a475a27bb1028f140bc2a7c843318afd", txMsg3.walletIDs[1])
	assert.Equal(t, types.Sender{1}, txMsg3.sender)

	flags = 0
	tx2 := Tx{
		walletIDs: []string{"0x8b6c8fd93d6f4cea42bbb345dbc6f0dfdb5bec73"},
		fallback:  10,
		flags:     flags,
	}
	b, _ = tx2.Pack(MinProtocol)
	txMsg2 := Tx{}
	_ = txMsg2.Unpack(b, MinProtocol)
	assert.Equal(t, 0, int(txMsg2.fallback))
	assert.Nil(t, txMsg2.walletIDs)
}
