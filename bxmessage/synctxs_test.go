package bxmessage

import (
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestPackUnpackTimeStamp(t *testing.T) {
	tx1 := &types.BxTransaction{}
	tx1.AddShortID(1)
	tx1.SetAddTime(time.Now())

	tx2 := &types.BxTransaction{}
	tx2.AddShortID(2)
	tx2.SetAddTime(time.Unix(0, 0))

	syncTxs1 := SyncTxsMessage{}
	syncTxs1.Add(tx1)
	syncTxs1.Add(tx2)
	buf, err := syncTxs1.Pack(CurrentProtocol)
	assert.Nil(t, err)

	syncTxs2 := SyncTxsMessage{}
	err = syncTxs2.Unpack(buf, CurrentProtocol)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(syncTxs2.ContentShortIds))
	//log.Info(syncTxs2.ContentShortIds[0].Timestamp().Second())
	assert.Equal(t, tx1.AddTime().Second(), syncTxs2.ContentShortIds[0].Timestamp().Second())
	assert.Equal(t, time.Unix(0, 0).Second(), syncTxs2.ContentShortIds[1].Timestamp().Second())

}

func TestPackUnpackBadBuffer(t *testing.T) {
	tx1 := &types.BxTransaction{}
	tx1.AddShortID(1)
	tx1.SetContent([]byte{1})
	tx1.SetAddTime(time.Now())

	tx2 := &types.BxTransaction{}
	tx2.AddShortID(2)
	tx2.SetContent([]byte{1, 2, 3, 4, 5})
	tx2.SetAddTime(time.Unix(0, 0))

	syncTxs1 := SyncTxsMessage{}
	syncTxs1.Add(tx1)
	syncTxs1.Add(tx2)
	buf, err := syncTxs1.Pack(CurrentProtocol)
	assert.Nil(t, err)

	syncTxs2 := SyncTxsMessage{}
	err = syncTxs2.Unpack(buf[0:0], CurrentProtocol)
	assert.NotNil(t, err)
	err = syncTxs2.Unpack(buf[0:1], CurrentProtocol)
	assert.NotNil(t, err)
	err = syncTxs2.Unpack(buf[0:2], CurrentProtocol)
	assert.NotNil(t, err)
	err = syncTxs2.Unpack(buf[0:20], CurrentProtocol)
	assert.NotNil(t, err)
	err = syncTxs2.Unpack(buf[0:30], CurrentProtocol)
	assert.NotNil(t, err)

	err = syncTxs2.Unpack(buf[0:65+5], CurrentProtocol)
	assert.NotNil(t, err)

	err = syncTxs2.Unpack(buf[0:77+35], CurrentProtocol)
	assert.NotNil(t, err)

	err = syncTxs2.Unpack(buf[0:124+3], CurrentProtocol)
	assert.NotNil(t, err)

	err = syncTxs2.Unpack(buf[0:len(buf)-2], CurrentProtocol)
	assert.NotNil(t, err)

	err = syncTxs2.Unpack(buf[0:], CurrentProtocol)
	assert.Nil(t, err)

}
