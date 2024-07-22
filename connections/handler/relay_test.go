package handler

import (
	"context"
	"runtime"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

// semi integration test: in general, sleep should be avoided, but these closing tests cases are checking that we are closing goroutines correctly
// relay tests differ from gateway tests in that relay connections are initiated by the node
func TestRelay_ClosingFromLocal(t *testing.T) {
	startCount := runtime.NumGoroutine()

	tls, r := relayConn()
	go r.Start(context.Background())

	// wait for hello message to be sent on connection so all goroutines are started
	_, err := tls.MockAdvanceSent()
	assert.NoError(t, err)

	test.WaitUntilTrueOrFail(t, func() bool {
		return r.Conn.IsOpen()
	})

	// expect 3 new  goroutines: read loop, send loop and read from receive channel
	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount+3
	})

	err = r.Close("test close")
	assert.NoError(t, err)

	test.WaitUntilTrueOrFail(t, func() bool {
		return tls.IsClosed() && !r.Conn.IsOpen()
	})

	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount
	})
}

func TestRelay_ClosingFromRemote(t *testing.T) {
	startCount := runtime.NumGoroutine()

	tls, r := relayConn()
	go r.Start(context.Background())

	// allow small wait for goroutines to start, returns when connection is ready and hello message sent out
	_, err := tls.MockAdvanceSent()
	assert.NoError(t, err)

	test.WaitUntilTrueOrFail(t, func() bool {
		return r.Conn.IsOpen()
	})

	// expect 2 new goroutines: read loop, send loop and read from receive channel
	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount+3
	})

	err = tls.Close("test close")
	assert.NoError(t, err)

	// only readloop go routines should be closed, since connection is expecting retry
	test.WaitUntilTrueOrFail(t, func() bool {
		return tls.IsClosed() && r.Conn.IsOpen()
	})

	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount+2
	})
}

func relayConn() (*connections.MockTLS, *Relay) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := connections.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := utils.TestCerts()
	r := NewRelay(bxmock.MockBxListener{},
		func() (connections.Socket, error) {
			return tls, nil
		},
		&certs, ip, port, "", utils.RelayTransaction, true, &sdnmessage.BlockchainNetworks{}, true, false, 0, utils.RealClock{},
		false)
	return tls, r
}
