package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
)

// Peers return connected peers
func (g *server) Peers(ctx context.Context, req *pb.PeersRequest) (*pb.PeersReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck

	_, err := g.validateAuthHeader(authHeader, false, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	peers := g.params.connector.Peers(req.Type)
	resp := &pb.PeersReply{}
	for i := range peers {
		resp.Peers = append(resp.Peers, &pb.Peer{
			Ip:               peers[i].IP,
			NodeId:           peers[i].NodeID,
			Protocol:         peers[i].Protocol,
			Type:             peers[i].Type,
			State:            peers[i].State,
			Network:          peers[i].Network,
			Initiator:        &wrapperspb.BoolValue{Value: peers[i].Initiator},
			AccountId:        peers[i].AccountID,
			Port:             peers[i].Port,
			Disabled:         &wrapperspb.BoolValue{Value: peers[i].Disabled},
			Capability:       peers[i].Capability,
			Trusted:          peers[i].Trusted,
			MinUsFromPeer:    peers[i].MinUsFromPeer,
			MinUsToPeer:      peers[i].MinUsToPeer,
			SlowTrafficCount: peers[i].SlowTrafficCount,
			MinUsRoundTrip:   peers[i].MinUsRoundTrip,
		})
	}

	return resp, nil
}
