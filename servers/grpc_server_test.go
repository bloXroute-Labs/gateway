package servers

import (
	"fmt"
	"testing"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/test"
)

func TestGRPCServer_RaceConditionOnShutdown(t *testing.T) {
	port := test.NextTestPort()
	s := NewGRPCServer("0.0.0.0", port, "", "", nil, "", pb.UnimplementedGatewayServer{})
	go s.Run()
	test.WaitServerStarted(t, fmt.Sprintf("%v:%v", localhost, port))

	s.Shutdown()
}
