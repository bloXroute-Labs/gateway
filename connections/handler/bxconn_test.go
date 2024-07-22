package handler

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHandler struct {
	*BxConn
}

// testHandler immediately closes the connection when a message is received
func (th *testHandler) ProcessMessage(msg bxmessage.MessageBytes) {
	_ = th.BxConn.Close("message handler test")
}

func (th *testHandler) setConn(b *BxConn) {
	th.BxConn = b
}

func TestBxConn_ProtocolVersion(t *testing.T) {
	th := testHandler{}
	_, bx := bxConn(&th)

	// client sending hello with protocol version which older than the current protocol version, we should update the protocol
	{
		helloMessage := bxmessage.Hello{
			Protocol: bxmessage.CurrentProtocol - 2,
		}
		b, err := helloMessage.Pack(bxmessage.CurrentProtocol) // with wich protocol to pack here is not important
		require.NoError(t, err)
		msg := bxmessage.NewMessageBytes(b, time.Now())
		bx.ProcessMessage(msg)
		require.Equal(t, bxmessage.Protocol(bxmessage.CurrentProtocol-2), bx.Protocol())
	}

	// if protocol version updated and supported, we should update the protocol
	//
	// there was a bug where we were not updating the protocol version if previous protocol was older than current protocol
	// so using old protocol version before is required for this test
	{
		helloMessage := bxmessage.Hello{
			Protocol: bxmessage.CurrentProtocol - 1,
		}
		b, err := helloMessage.Pack(bxmessage.CurrentProtocol)
		require.NoError(t, err)
		msg := bxmessage.NewMessageBytes(b, time.Now())
		bx.ProcessMessage(msg)
		require.Equal(t, bxmessage.Protocol(bxmessage.CurrentProtocol-1), bx.Protocol())
	}

	// if protocol version is not supported, we should default to the latest supported - current protocol
	{
		helloMessage := bxmessage.Hello{
			Protocol: bxmessage.CurrentProtocol + 2,
		}
		b, err := helloMessage.Pack(bxmessage.CurrentProtocol)
		require.NoError(t, err)
		msg := bxmessage.NewMessageBytes(b, time.Now())
		bx.ProcessMessage(msg)
		require.Equal(t, bxmessage.Protocol(bxmessage.CurrentProtocol), bx.Protocol())
	}

	// protocol can be downgraded when changes are reverted
	{
		helloMessage := bxmessage.Hello{
			Protocol: bxmessage.CurrentProtocol - 1,
		}
		b, err := helloMessage.Pack(bxmessage.CurrentProtocol)
		require.NoError(t, err)
		msg := bxmessage.NewMessageBytes(b, time.Now())
		bx.ProcessMessage(msg)
		require.Equal(t, bxmessage.Protocol(bxmessage.CurrentProtocol-1), bx.Protocol())
	}
}

// semi integration test: in general, sleep should be avoided, but these closing tests cases are checking that we are closing goroutines correctly
func TestBxConn_ClosingFromHandler(t *testing.T) {
	startCount := runtime.NumGoroutine()

	th := testHandler{}
	tls, bx := bxConn(&th)
	th.setConn(bx)

	go bx.Start(context.Background())

	// wait for hello message to be sent on connection so all goroutines are started
	_, err := tls.MockAdvanceSent()
	assert.NoError(t, err)

	test.WaitUntilTrueOrFail(t, func() bool {
		return bx.Conn.IsOpen()
	})

	// expect 3 additional goroutines: read loop, send loop and read from receive channel
	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount+3
	})

	// queue message, which should trigger a close
	helloMessage := bxmessage.Hello{}
	b, err := helloMessage.Pack(bxmessage.CurrentProtocol)
	tls.MockQueue(b)

	test.WaitUntilTrueOrFail(t, func() bool {
		return tls.IsClosed() && !bx.Conn.IsOpen()
	})

	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount
	})
}

func bxConn(handler connections.ConnHandler) (*connections.MockTLS, *BxConn) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := connections.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := utils.TestCerts()
	b := NewBxConn(bxmock.MockBxListener{},
		func() (connections.Socket, error) {
			return tls, nil
		},
		handler, &certs, ip, port, "", utils.RelayTransaction, true, false, true, false, connections.LocalInitiatedPort, utils.RealClock{},
		false)
	return tls, b
}
