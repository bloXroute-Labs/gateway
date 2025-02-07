package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// BlxrSubmitBundle submit blxr bundle
func (g *server) BlxrSubmitBundle(ctx context.Context, req *pb.BlxrSubmitBundleRequest) (*pb.BlxrSubmitBundleReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")

	accountModel, err := g.validateAuthHeader(authHeader, true, false, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	mevBundleParams := &jsonrpc.RPCBundleSubmissionPayload{
		MEVBuilders:             req.MevBuilders,
		Transaction:             req.Transactions,
		BlockNumber:             req.BlockNumber,
		MinTimestamp:            int(req.MinTimestamp),
		MaxTimestamp:            int(req.MaxTimestamp),
		RevertingHashes:         req.RevertingHashes,
		UUID:                    req.Uuid,
		AvoidMixedBundles:       req.AvoidMixedBundles,
		PriorityFeeRefund:       req.PriorityFeeRefund,
		IncomingRefundRecipient: req.RefundRecipient,
		BlocksCount:             int(req.BlocksCount),
		DroppingHashes:          req.DroppingHashes,
		EndOfBlock:              req.EndOfBlock,
	}

	grpc := connections.NewRPCConn(*accountID, getPeerAddr(ctx), g.params.sdn.NetworkNum(), utils.GRPC)
	bundleSubmitResult, _, err := handler.HandleMEVBundle(g.params.node, grpc, *accountModel, mevBundleParams)
	if err != nil {
		// TODO need to refactor errors returned from HandleMEVBundle and then map them to jsonrpc and gRPC codes accordingly
		// TODO instead of returning protocol specific error codes
		return nil, err
	}

	return &pb.BlxrSubmitBundleReply{BundleHash: bundleSubmitResult.BundleHash}, nil
}
