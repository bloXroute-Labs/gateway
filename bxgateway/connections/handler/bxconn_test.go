package handler

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/bxmessage"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/connections"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	"github.com/bloXroute-Labs/bxgateway-private-go/test/bxmock"
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"
	"time"
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

// semi integration test: in general, sleep should be avoided, but these closing tests cases are checking that we are closing goroutines correctly
func TestBxConn_ClosingFromHandler(t *testing.T) {
	startCount := runtime.NumGoroutine()

	th := testHandler{}
	tls, bx := bxConn(&th)
	th.setConn(bx)

	err := bx.Start()
	assert.Nil(t, err)

	// wait for hello message to be sent on connection so all goroutines are started
	_, err = tls.MockAdvanceSent()
	assert.Nil(t, err)

	// expect 2 additional goroutines: read loop, send loop
	assert.Equal(t, startCount+2, runtime.NumGoroutine())

	// queue message, which should trigger a close
	helloMessage := bxmessage.Hello{}
	b, err := helloMessage.Pack(bxmessage.CurrentProtocol)
	tls.MockQueue(b)

	// allow small delta for goroutines to finish
	time.Sleep(1 * time.Millisecond)

	endCount := runtime.NumGoroutine()
	assert.Equal(t, startCount, endCount)
}

func bxConn(handler connections.ConnHandler) (bxmock.MockTLS, *BxConn) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := bxmock.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := bxmock.TestCerts()
	b := NewBxConn(bxmock.MockBxListener{},
		func() (connections.Socket, error) {
			return tls, nil
		},
		handler, &certs, ip, port, "", utils.RelayTransaction, true, false, true, false, connections.LocalInitiatedPort, utils.RealClock{})
	return tls, b
}
