package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	bxrpc "github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/account"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/version"
)

const bdn = "BDN"

var (
	errMissingAuthHeader        = errors.New("auth header is missing")
	errInternalGwRequiredHeader = errors.New("could not connect to internal gateway without auth header")
)

// server implementation of GatewayServer
type server struct {
	pb.UnimplementedGatewayServer
	params grpcParams
	log    *log.Entry
}

// grpcParams server params
type grpcParams struct {
	node                           connections.BxListener
	sdn                            sdnsdk.SDNHTTP
	accService                     account.Accounter
	bridge                         blockchain.Bridge
	blockchainPeers                []types.NodeEndpoint
	wsManager                      blockchain.WSManager
	bdnStats                       *bxmessage.BdnPerformanceStats
	timeStarted                    time.Time
	gatewayPublicKey               string
	connector                      Connector
	txFromFieldIncludable          bool
	feedManager                    feedManager
	txStore                        services.TxStore
	chainID                        bxtypes.NetworkID
	oFACList                       *types.OFACMap
}

// newServer return new server object
func newServer(gatewayGrpcParams grpcParams) *server {
	return &server{params: gatewayGrpcParams, log: log.WithFields(log.Fields{"component": "gatewayGrpc"})}
}

// DisconnectInboundPeer disconnect inbound peer from gateway
func (g *server) DisconnectInboundPeer(ctx context.Context, req *pb.DisconnectInboundPeerRequest) (*pb.DisconnectInboundPeerReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	_, err := g.validateAuthHeader(authHeader, false, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	err = g.params.bridge.SendDisconnectEvent(types.NodeEndpoint{IP: req.PeerIp, Port: int(req.PeerPort), PublicKey: req.PublicKey})
	if err != nil {
		return &pb.DisconnectInboundPeerReply{Status: err.Error()}, status.Error(codes.Internal, err.Error())
	}
	return &pb.DisconnectInboundPeerReply{Status: fmt.Sprintf("Sent request to disconnect peer %v %v %v", req.PublicKey, req.PeerIp, req.PeerPort)}, nil
}

// Version return current gw version
func (g *server) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	_, err := g.validateAuthHeader(authHeader, false, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	resp := &pb.VersionReply{
		Version:   version.BuildVersion,
		BuildDate: version.BuildDate,
	}
	return resp, nil
}

// Status return gw status
func (g *server) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	_, err := g.validateAuthHeader(authHeader, false, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	if err = g.params.bridge.SendTrustedPeerRequest(); err != nil {
		g.log.Errorf("failed to send trusted peer request: %v", err)
		return nil, err
	}

	trustedPeers, err := g.params.bridge.ReceiveTrustedPeerResponse()
	if err != nil {
		g.log.Errorf("failed to receive trusted peer response: %v", err)
		return nil, err
	}

	if err = g.params.bridge.SendBlockchainStatusRequest(); err != nil {
		g.log.Errorf("failed to send blockchain status request: %v", err)
		return nil, err
	}

	bchStatus, err := g.params.bridge.ReceiveBlockchainStatusResponse()
	if err != nil {
		g.log.Errorf("failed to receive blockchain status response: %v", err)
		return nil, err
	}

	nodeModel := g.params.sdn.NodeModel()
	accountModel := g.params.sdn.AccountModel()

	trustedPeersStr := make([]string, 0, len(trustedPeers))
	for _, peer := range trustedPeers {
		trustedPeersStr = append(trustedPeersStr, peer.String())
	}

	rsp := &pb.StatusResponse{
		GatewayInfo: &pb.GatewayInfo{
			Version:          version.BuildVersion,
			NodeId:           string(nodeModel.NodeID),
			IpAddress:        nodeModel.ExternalIP,
			TimeStarted:      g.params.timeStarted.Format(time.RFC3339),
			Continent:        nodeModel.Continent,
			Country:          nodeModel.Country,
			Network:          nodeModel.Network,
			StartupParams:    strings.Join(os.Args[1:], " "),
			GatewayPublicKey: g.params.gatewayPublicKey,
			TrustedPeers:     trustedPeersStr,
			P2PServers:       bchStatus.ServerAddresses,
		},
		Nodes:  g.nodes(bchStatus.Endpoints),
		Relays: g.relays(),
		AccountInfo: &pb.AccountInfo{
			AccountId:  string(accountModel.AccountID),
			ExpireDate: accountModel.ExpireDate,
		},
	}

	return rsp, nil
}

// Subscriptions return list of subscriptions
func (g *server) Subscriptions(ctx context.Context, req *pb.SubscriptionsRequest) (*pb.SubscriptionsReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	_, err := g.validateAuthHeader(authHeader, false, true, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	subs := g.params.feedManager.GetGrpcSubscriptionReply()
	resp := &pb.SubscriptionsReply{
		Subscriptions: make([]*pb.Subscription, len(subs)),
	}

	for i, sub := range subs {
		resp.Subscriptions[i] = &pb.Subscription{
			AccountId:    string(sub.AccountID),
			Tier:         sub.Tier,
			FeedName:     string(sub.FeedName),
			Network:      uint32(sub.Network),
			RemoteAddr:   sub.RemoteAddr,
			Include:      sub.Include,
			Filter:       sub.Filter,
			Age:          sub.Age,
			MessagesSent: sub.MessagesSent,
			ConnType:     string(sub.ConnType),
		}
	}

	return resp, nil
}

// ShortIDs returns short ids for the given tx hashes
func (g *server) ShortIDs(ctx context.Context, req *pb.ShortIDsRequest) (*pb.ShortIDsReply, error) {
	authHeader, err := bxrpc.ReadAuthMetadata(ctx)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	if _, err = g.validateAuthHeader(authHeader, false, false, getPeerAddr(ctx)); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.shortIDs(req)
}

func (g *server) relays() map[string]*pb.BDNConnStatus {
	mp := g.params.connector.Relays()

	if len(mp) == 0 {
		// set "BDN: NOT_CONNECTED" in case of missing connections to any relay
		return map[string]*pb.BDNConnStatus{bdn: {Status: connectionStatusNotConnected}}
	}

	relays := make(map[string]*pb.BDNConnStatus)
	for ip, relayStatus := range mp {
		connStatus := &pb.BDNConnStatus{
			Status:      relayStatus.Status,
			ConnectedAt: relayStatus.ConnectedAt,
		}
		if relayStatus.Latency != nil {
			connStatus.Latency = &pb.ConnectionLatency{
				MinMsFromPeer:    relayStatus.Latency.MinMsFromPeer,
				MinMsToPeer:      relayStatus.Latency.MinMsToPeer,
				SlowTrafficCount: relayStatus.Latency.SlowTrafficCount,
				MinMsRoundTrip:   relayStatus.Latency.MinMsRoundTrip,
			}
		}
		relays[ip] = connStatus
	}

	return relays
}

func (g *server) nodes(endpoints []*types.NodeEndpoint) map[string]*pb.NodeConnStatus {
	wsProviders := g.params.wsManager.Providers()

	mp := make(map[string]*pb.NodeConnStatus)
	nodeStats := g.params.bdnStats.NodeStats()

	for _, peer := range endpoints {
		connStatus := &pb.NodeConnStatus{
			Dynamic: peer.IsDynamic(),
			Version: int64(peer.Version),
			Name:    peer.Name,
		}

		nstat, ok := nodeStats[peer.IPPort()]
		if ok {
			connStatus.IsConnected = nstat.IsConnected
			connStatus.ConnectedAt = peer.ConnectedAt
			connStatus.ConnectionType = peer.ConnectionType
			if peer.IsBeacon {
				connStatus.PeerId = peer.ID
			}
			connStatus.NodePerformance = &pb.NodePerformance{
				Since:                                   g.params.bdnStats.StartTime().Format(time.RFC3339),
				NewBlocksReceivedFromBlockchainNode:     uint32(nstat.NewBlocksReceivedFromBlockchainNode),
				NewBlocksReceivedFromBdn:                uint32(nstat.NewBlocksReceivedFromBdn),
				NewBlocksSeen:                           nstat.NewBlocksSeen,
				NewBlockMessagesFromBlockchainNode:      nstat.NewBlockMessagesFromBlockchainNode,
				NewBlockAnnouncementsFromBlockchainNode: nstat.NewBlockAnnouncementsFromBlockchainNode,
				NewTxReceivedFromBlockchainNode:         nstat.NewTxReceivedFromBlockchainNode,
				NewTxReceivedFromBdn:                    nstat.NewTxReceivedFromBdn,
				TxSentToNode:                            nstat.TxSentToNode,
				DuplicateTxFromNode:                     nstat.DuplicateTxFromNode,
			}
		}

		mp[ipport(peer.IP, peer.Port)] = connStatus

		wsPeer, ok := wsProviders[peer.IPPort()]
		if !ok {
			continue
		}

		connStatus.WsConnection = &pb.WsConnStatus{
			Addr: wsPeer.Addr(),
			ConnStatus: func() string {
				if wsPeer.IsOpen() {
					return connectionStatusConnected
				}
				return connectionStatusNotConnected
			}(),
			SyncStatus: strings.ToLower(string(wsPeer.SyncStatus())),
		}
	}

	// If a node was disconnected through the interval then they are not connected.
	// Let state this explicitly.
	for key, peer := range nodeStats {
		ipPort := strings.ReplaceAll(key, " ", ":")
		if _, ok := mp[ipPort]; !ok {
			mp[ipPort] = &pb.NodeConnStatus{
				IsConnected: peer.IsConnected,
				Dynamic:     peer.Dynamic,
				NodePerformance: &pb.NodePerformance{
					Since:                                   g.params.bdnStats.StartTime().Format(time.RFC3339),
					NewBlocksReceivedFromBlockchainNode:     uint32(peer.NewBlocksReceivedFromBlockchainNode),
					NewBlocksReceivedFromBdn:                uint32(peer.NewBlocksReceivedFromBdn),
					NewBlocksSeen:                           peer.NewBlocksSeen,
					NewBlockMessagesFromBlockchainNode:      peer.NewBlockMessagesFromBlockchainNode,
					NewBlockAnnouncementsFromBlockchainNode: peer.NewBlockAnnouncementsFromBlockchainNode,
					NewTxReceivedFromBlockchainNode:         peer.NewTxReceivedFromBlockchainNode,
					NewTxReceivedFromBdn:                    peer.NewTxReceivedFromBdn,
					TxSentToNode:                            peer.TxSentToNode,
					DuplicateTxFromNode:                     peer.DuplicateTxFromNode,
				},
			}
		}
	}

	return mp
}

func (g *server) validateAuthHeader(authHeader string, isRequiredForExternalGateway, allowAccessByOtherAccounts bool, ip string) (*sdnmessage.Account, error) {
	accountID, secretHash, err := g.accountIDAndHashFromAuthHeader(authHeader, isRequiredForExternalGateway)
	if err != nil {
		return nil, err
	}

	accountModel, err := g.params.accService.Authorize(accountID, secretHash, allowAccessByOtherAccounts, ip)
	if err != nil {
		return nil, err
	}

	return &accountModel, nil
}

func (g *server) accountIDAndHashFromAuthHeader(authHeader string, isRequiredForExternalGateway bool) (accountID bxtypes.AccountID, secretHash string, err error) {
	if authHeader == "" {
		if isRequiredForExternalGateway {
			err = errMissingAuthHeader
			return
		}
		if g.params.sdn.AccountModel().AccountID == bxtypes.BloxrouteAccountID {
			err = errInternalGwRequiredHeader
			return
		}
		authHeader = g.getHeaderFromGateway()
	}

	accountID, secretHash, err = sdnsdk.GetAccountIDSecretHashFromHeader(authHeader)

	return
}

func (g *server) getHeaderFromGateway() string {
	accountID := g.params.sdn.AccountModel().AccountID
	secretHash := g.params.sdn.AccountModel().SecretHash
	accountIDAndHash := fmt.Sprintf("%s:%s", accountID, secretHash)
	return base64.StdEncoding.EncodeToString([]byte(accountIDAndHash))
}

func (g *server) notify(notification types.Notification) {
	g.params.feedManager.Notify(notification)
}

func interfaceToString(value interface{}) string {
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return ""
}

func interfaceToStringArray(value interface{}) []string {
	if stringArray, ok := value.([]string); ok {
		return stringArray
	}
	return []string{}
}

func interfaceToBool(value interface{}) bool {
	if boolValue, ok := value.(bool); ok {
		return boolValue
	}
	return false
}
