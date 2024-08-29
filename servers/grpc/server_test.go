package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func TestGRPCServer_RaceConditionOnShutdown(t *testing.T) {
	port := test.NextTestPort()
	s, _ := testGRPCServer(t, port, "", "")
	go s.Run()

	test.WaitServerStarted(t, fmt.Sprintf("0.0.0.0:%v", port))

	s.Shutdown()
}

func TestRetrieveOriginalSenderAccountID_NoMetadata(t *testing.T) {
	accountModel := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.BloxrouteAccountID,
		},
	}
	ctx := context.Background()

	_, err := retrieveOriginalSenderAccountID(ctx, accountModel)

	require.NotNil(t, err)
}

func TestRetrieveOriginalSenderAccountID_BloxrouteAccountID(t *testing.T) {
	accountModel := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID(types.BloxrouteAccountID),
		},
	}
	ctx := createMockContextWithMetadata(testGatewayAccountID)

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)

	require.Nil(t, err)

	expectedAccountID := types.AccountID(testGatewayAccountID)

	require.Equal(t, expectedAccountID, *accountID)
}

func TestRetrieveOriginalSenderAccountID_NonBloxrouteAccountID(t *testing.T) {
	accountModel := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID(testGatewayAccountID),
		},
	}
	ctx := createMockContextWithMetadata(testGatewayAccountID2)

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)

	require.Nil(t, err)

	expectedAccountID := types.AccountID(testGatewayAccountID)

	require.Equal(t, expectedAccountID, *accountID)
}

// Mocking the context metadata for testing.
func createMockContextWithMetadata(accountIDHeaderVal string) context.Context {
	md := metadata.Pairs(types.OriginalSenderAccountIDHeaderKey, accountIDHeaderVal)
	return metadata.NewIncomingContext(context.Background(), md)
}