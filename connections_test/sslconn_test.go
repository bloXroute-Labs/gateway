package connections_test

import (
	"runtime"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

func TestSSLConn_ClosingFromSend(t *testing.T) {
	startCount := runtime.NumGoroutine()

	_, s := sslConn(1)

	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount
	})

	// send loop started
	_ = s.Connect()

	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount+1
	})

	am := bxmessage.Ack{}
	_ = s.Send(&am)
	assert.True(t, s.IsOpen())

	_ = s.Send(&am)

	test.WaitUntilTrueOrFail(t, func() bool {
		return !s.IsOpen()
	})

	test.WaitUntilTrueOrFail(t, func() bool {
		return runtime.NumGoroutine() == startCount
	})
}

func sslConn(backlog int) (*bxmock.MockTLS, *connections.SSLConn) {
	ip := "127.0.0.1"
	port := int64(3000)

	tls := bxmock.NewMockTLS(ip, port, "", utils.ExternalGateway, "")
	certs := utils.TestCerts()
	s := connections.NewSSLConnection(
		func() (connections.Socket, error) {
			return tls, nil
		},
		&certs, ip, port, bxmessage.CurrentProtocol, false, false, backlog, utils.RealClock{})
	return tls, s
}
