package connections_test

import (
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"
	"time"
)

func TestSSLConn_ClosingFromSend(t *testing.T) {
	startCount := runtime.NumGoroutine()

	_, s := sslConn(1)
	assert.Equal(t, startCount, runtime.NumGoroutine())

	// send loop started
	_ = s.Connect()
	assert.Equal(t, startCount+1, runtime.NumGoroutine())

	am := bxmessage.Ack{}
	_ = s.Send(&am)
	assert.True(t, s.IsOpen())

	_ = s.Send(&am)
	assert.False(t, s.IsOpen())

	time.Sleep(1 * time.Millisecond)
	assert.Equal(t, startCount, runtime.NumGoroutine())
}

func sslConn(backlog int) (bxmock.MockTLS, *connections.SSLConn) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := bxmock.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := bxmock.TestCerts()
	s := connections.NewSSLConnection(
		func() (connections.Socket, error) {
			return tls, nil
		},
		&certs, ip, port, bxmessage.CurrentProtocol, false, false, backlog, utils.RealClock{})
	return tls, s
}
