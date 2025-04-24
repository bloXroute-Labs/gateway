package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
)

type MockConnection struct {
	handler.Relay
}

func (m MockConnection) GetNodeID() bxtypes.NodeID {
	return "test"
}

func TestMessageQueueStop(t *testing.T) {
	messagesCount := atomic.Int64{}
	conn := MockConnection{}
	wg := sync.WaitGroup{}

	testAdapter := func(msg bxmessage.Message, source connections.Conn, waitingDuration time.Duration, workerChannelPosition int) {
		messagesCount.Add(1)
		wg.Done()
	}

	messageQueue := NewMsgQueue(3, 100, testAdapter)
	wg.Add(3)
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	wg.Wait()
	require.Equal(t, uint64(3), messageQueue.TxsCount())
	require.Equal(t, int64(3), messagesCount.Load())

	messageQueue.Stop()
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	require.Equal(t, uint64(3), messageQueue.TxsCount())
	require.Equal(t, 0, messageQueue.Len())
	require.Equal(t, int64(3), messagesCount.Load())
}

func TestMessageQueueInsert(t *testing.T) {
	conn := MockConnection{}
	wg := sync.WaitGroup{}

	testAdapter := func(msg bxmessage.Message, source connections.Conn, waitingDuration time.Duration, workerChannelPosition int) {
		wg.Done()
	}

	messageQueue := NewMsgQueue(3, 100, testAdapter)
	wg.Add(3)
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	wg.Wait()
	require.Equal(t, uint64(3), messageQueue.TxsCount())
}

func TestMessageQueueChannelIsFull(t *testing.T) {
	conn := MockConnection{}

	syncChan := make(chan struct{})

	testAdapter := func(msg bxmessage.Message, source connections.Conn, waitingDuration time.Duration, workerChannelPosition int) {
		syncChan <- struct{}{}
		<-syncChan
	}

	messageQueue := NewMsgQueue(1, 1, testAdapter)
	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	<-syncChan // wait until worker read the channel and blocks

	require.NoError(t, messageQueue.Insert(&bxmessage.Tx{}, conn))
	require.Error(t, messageQueue.Insert(&bxmessage.Tx{}, conn))

	syncChan <- struct{}{} // release worker
}
