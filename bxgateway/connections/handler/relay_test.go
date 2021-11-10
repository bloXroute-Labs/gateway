package handler

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/connections"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/sdnmessage"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	"github.com/bloXroute-Labs/bxgateway-private-go/test/bxmock"
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"
	"time"
)

// semi integration test: in general, sleep should be avoided, but these closing tests cases are checking that we are closing goroutines correctly
// relay tests differ from gateway tests in that relay connections are initiated by the node
func TestRelay_ClosingFromLocal(t *testing.T) {
	startCount := runtime.NumGoroutine()

	tls, r := relayConn()
	err := r.Start()
	assert.Nil(t, err)

	// wait for hello message to be sent on connection so all goroutines are started
	_, err = tls.MockAdvanceSent()
	assert.Nil(t, err)

	// expect 2 new  goroutines: read loop, send loop
	assert.Equal(t, startCount+2, runtime.NumGoroutine())

	err = r.Close("test close")
	assert.Nil(t, err)

	// allow small delta for goroutines to finish
	time.Sleep(1 * time.Millisecond)

	endCount := runtime.NumGoroutine()
	assert.Equal(t, startCount, endCount)
}

func TestRelay_ClosingFromRemote(t *testing.T) {
	startCount := runtime.NumGoroutine()

	tls, r := relayConn()
	err := r.Start()
	assert.Nil(t, err)

	// allow small wait for goroutines to start, returns when connection is ready and hello message sent out
	_, err = tls.MockAdvanceSent()
	assert.Nil(t, err)

	// expect 2 new goroutines: read loop, send loop
	startedCount := runtime.NumGoroutine()
	assert.Equal(t, startCount+2, startedCount)

	err = tls.Close("test close")
	assert.Nil(t, err)

	// allow small delta for goroutines to finish
	time.Sleep(1 * time.Millisecond)

	// only readloop go routines should be closed, since connection is expecting retry

	assert.Equal(t, startCount+1, runtime.NumGoroutine())
}

func relayConn() (bxmock.MockTLS, *Relay) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := bxmock.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := bxmock.TestCerts()
	r := NewRelay(bxmock.MockBxListener{},
		func() (connections.Socket, error) {
			return tls, nil
		},
		&certs, ip, port, "", utils.RelayTransaction, true, &sdnmessage.BlockchainNetworks{}, true, false, 0, utils.RealClock{})
	return tls, r
}
