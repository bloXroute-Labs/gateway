package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

type connMock struct {
	id        connections.Socket
	nodeID    string
	accountID string
	connType  bxtypes.NodeType
}

func (c connMock) ID() connections.Socket                                   { return c.id }
func (c connMock) IsOpen() bool                                             { return true }
func (c connMock) IsDisabled() bool                                         { panic("implement me") }
func (c connMock) Protocol() bxmessage.Protocol                             { return 0 }
func (c connMock) Log() *log.Entry                                          { return log.Discard() }
func (c connMock) SetProtocol(_ bxmessage.Protocol)                         { panic("implement me") }
func (c connMock) Connect() error                                           { panic("implement me") }
func (c connMock) Send(_ bxmessage.Message) error                           { panic("implement me") }
func (c connMock) SendWithDelay(_ bxmessage.Message, _ time.Duration) error { panic("implement me") }
func (c connMock) Disable(_ string)                                         { panic("implement me") }
func (c connMock) Close(_ string) error                                     { panic("implement me") }
func (c connMock) ReadMessages(_ func(bxmessage.MessageBytes), _ time.Duration, _ int, _ func([]byte) int) (int, error) {
	panic("implement me")
}

func (c connMock) GetNodeID() bxtypes.NodeID              { return bxtypes.NodeID(c.nodeID) }
func (c connMock) GetPeerIP() string                      { return "" }
func (c connMock) GetVersion() string                     { panic("implement me") }
func (c connMock) GetPeerPort() int64                     { return 0 }
func (c connMock) GetPeerEnode() string                   { panic("implement me") }
func (c connMock) GetLocalPort() int64                    { return 0 }
func (c connMock) GetAccountID() bxtypes.AccountID        { return bxtypes.AccountID(c.accountID) }
func (c connMock) GetNetworkNum() bxtypes.NetworkNum      { return 0 }
func (c connMock) GetConnectedAt() time.Time              { panic("implement me") }
func (c connMock) GetCapabilities() types.CapabilityFlags { return 0 }
func (c connMock) GetConnectionType() bxtypes.NodeType    { return c.connType }
func (c connMock) GetConnectionState() string             { panic("implement me") }
func (c connMock) IsLocalGEO() bool                       { panic("implement me") }
func (c connMock) IsInitiator() bool                      { panic("implement me") }
func (c connMock) IsSameRegion() bool                     { panic("implement me") }
func (c connMock) IsPrivateNetwork() bool                 { panic("implement me") }

func TestMessageQueueStop(t *testing.T) {
	messagesCount := atomic.Int64{}
	conn := connMock{}
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
	conn := connMock{}
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
	conn := connMock{}

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
