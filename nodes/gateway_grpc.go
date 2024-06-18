package nodes

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	bxrpc "github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/zhouzhuojie/conditions"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	bdn                    = "BDN"
	maxTxsInSingleResponse = 50
)

var (
	txContentFields = []string{"tx_contents.nonce", "tx_contents.tx_hash",
		"tx_contents.gas_price", "tx_contents.gas", "tx_contents.to", "tx_contents.value", "tx_contents.input",
		"tx_contents.v", "tx_contents.r", "tx_contents.s", "tx_contents.from", "tx_contents.type", "tx_contents.access_list",
		"tx_contents.chain_id", "tx_contents.max_priority_fee_per_gas", "tx_contents.max_fee_per_gas", "tx_contents.max_fee_per_blob_gas", "tx_contents.blob_versioned_hashes", "tx_contents.y_parity"}
	validOnBlockParams   = []string{"name", "response", "block_height", "tag"}
	validTxReceiptParams = []string{"block_hash", "block_number", "contract_address",
		"cumulative_gas_used", "effective_gas_price", "from", "gas_used", "logs", "logs_bloom",
		"status", "to", "transaction_hash", "transaction_index", "type", "txs_count"}
	validBlockParams = append(txContentFields, "tx_contents.from", "hash", "header", "transactions", "uncles", "future_validator_info", "withdrawals")
)

// GatewayGrpc implementation of GatewayServer
type GatewayGrpc struct {
	pb.UnimplementedGatewayServer
	params GatewayGrpcParams
	log    *log.Entry
	*Bx
}

// GatewayGrpcParams GatewayGrpc params
type GatewayGrpcParams struct {
	sdn                         connections.SDNHTTP
	authorize                   func(accountID types.AccountID, secretHash string, allowAccessToInternalGateway, allowIntroductoryTierAccess bool, ip string) (sdnmessage.Account, error)
	bridge                      blockchain.Bridge
	blockchainPeers             []types.NodeEndpoint
	wsManager                   blockchain.WSManager
	bdnStats                    *bxmessage.BdnPerformanceStats
	timeStarted                 time.Time
	txsQueue                    services.MessageQueue
	txsOrderQueue               services.MessageQueue
	gatewayPublicKey            string
	feedManager                 *servers.FeedManager
	feedManagerChan             chan types.Notification
	txFromFieldIncludable       bool
	blockProposer               bsc.BlockProposer
	allowIntroductoryTierAccess bool
	intentsManager              services.IntentsManager
	grpcFeedManager             GRPCFeedManager
}

// NewGatewayGrpc return new GatewayGrpc object
func NewGatewayGrpc(bx *Bx, gatewayGrpcParams GatewayGrpcParams) *GatewayGrpc {
	return &GatewayGrpc{Bx: bx, params: gatewayGrpcParams, log: log.WithFields(log.Fields{"component": "gatewayGrpc"})}
}

// Peers return connected peers
func (g *GatewayGrpc) Peers(ctx context.Context, req *pb.PeersRequest) (*pb.PeersReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.Bx.Peers(ctx, req)
}

func (g *GatewayGrpc) validateAuthHeader(authHeader string, required, allowAccessToInternalGateway bool, ip string) (*sdnmessage.Account, error) {
	accountID, secretHash, err := g.accountIDAndHashFromAuthHeader(authHeader, required)
	if err != nil {
		return nil, err
	}

	accountModel, err := g.params.authorize(accountID, secretHash, allowAccessToInternalGateway, false, ip)
	if err != nil {
		return nil, err
	}

	return &accountModel, nil
}

func (g *GatewayGrpc) accountIDAndHashFromAuthHeader(authHeader string, required bool) (accountID types.AccountID, secretHash string, err error) {
	if authHeader == "" {
		if required {
			err = errors.New("auth header is missing")
			return
		}
		if g.params.sdn.AccountModel().AccountID == types.BloxrouteAccountID {
			err = errors.New("could not connect to internal gateway without auth header")
			return
		}
		authHeader = g.getHeaderFromGateway()
	}

	accountID, secretHash, err = utils.GetAccountIDSecretHashFromHeader(authHeader)

	return
}

func (g *GatewayGrpc) getHeaderFromGateway() string {
	accountID := g.params.sdn.AccountModel().AccountID
	secretHash := g.params.sdn.AccountModel().SecretHash
	accountIDAndHash := fmt.Sprintf("%s:%s", accountID, secretHash)
	return base64.StdEncoding.EncodeToString([]byte(accountIDAndHash))
}

// DisconnectInboundPeer disconnect inbound peer from gateway
func (g *GatewayGrpc) DisconnectInboundPeer(ctx context.Context, req *pb.DisconnectInboundPeerRequest) (*pb.DisconnectInboundPeerReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true, servers.GetPeerAddr(ctx))
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
func (g *GatewayGrpc) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true, servers.GetPeerAddr(ctx))
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
func (g *GatewayGrpc) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	var bdnConn = func() map[string]*pb.BDNConnStatus {
		var mp = make(map[string]*pb.BDNConnStatus)

		g.ConnectionsLock.RLock()
		for _, conn := range g.Connections {
			connectionType := conn.GetConnectionType()

			if connectionType&utils.Relay == 0 {
				continue
			}

			var connectionLatency *pb.ConnectionLatency
			if bxConn, ok := conn.(*handler.BxConn); ok {
				minMsFromPeer, minMsToPeer, slowTrafficCount, minMsRoundTrip := bxConn.GetMinLatencies()
				connectionLatency = &pb.ConnectionLatency{
					MinMsFromPeer:    minMsFromPeer,
					MinMsToPeer:      minMsToPeer,
					SlowTrafficCount: slowTrafficCount,
					MinMsRoundTrip:   minMsRoundTrip,
				}
			}

			peerIP := conn.GetPeerIP()

			if !conn.IsOpen() {
				mp[peerIP] = &pb.BDNConnStatus{
					Status: connectionStatusNotConnected,
				}

				continue
			}

			mp[peerIP] = &pb.BDNConnStatus{
				Status:      connectionStatusConnected,
				ConnectedAt: conn.GetConnectedAt().Format(time.RFC3339),
				Latency:     connectionLatency,
			}
		}
		g.ConnectionsLock.RUnlock()

		if len(mp) == 0 {
			// set "BDN: NOT_CONNECTED" in case of missing connections to any relay
			mp[bdn] = &pb.BDNConnStatus{
				Status: connectionStatusNotConnected,
			}
		}

		return mp
	}

	var nodeConn = func() map[string]*pb.NodeConnStatus {
		if len(g.params.blockchainPeers) == 0 {
			return nil // Gateway is not connected to nodes
		}

		err := g.params.bridge.SendBlockchainStatusRequest()
		if err != nil {
			g.log.Errorf("failed to send blockchain status request: %v", err)
			return nil
		}

		var wsProviders = g.params.wsManager.Providers()

		select {
		case status := <-g.params.bridge.ReceiveBlockchainStatusResponse():
			var mp = make(map[string]*pb.NodeConnStatus)
			var nodeStats = g.params.bdnStats.NodeStats()
			for _, peer := range status {
				connStatus := &pb.NodeConnStatus{
					Dynamic: peer.IsDynamic(),
					Version: int64(peer.Version),
					Name:    peer.Name,
				}

				nstat, ok := nodeStats[peer.IPPort()]
				if ok {
					connStatus.IsConnected = nstat.IsConnected
					connStatus.ConnectedAt = peer.ConnectedAt
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
				ipPort := strings.Replace(key, " ", ":", -1)
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
		case <-time.After(time.Second):
			g.log.Errorf("no blockchain status response from backend within 1sec timeout")
			return nil
		}
	}

	var (
		nodeModel    = g.params.sdn.NodeModel()
		accountModel = g.params.sdn.AccountModel()
	)

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
		},
		Nodes:  nodeConn(),
		Relays: bdnConn(),
		AccountInfo: &pb.AccountInfo{
			AccountId:  string(accountModel.AccountID),
			ExpireDate: accountModel.ExpireDate,
		},
		QueueStats: &pb.QueuesStats{
			TxsQueueCount:      g.params.txsQueue.TxsCount(),
			TxsOrderQueueCount: g.params.txsOrderQueue.TxsCount(),
		},
	}

	return rsp, nil
}

// Subscriptions return list of subscriptions
func (g *GatewayGrpc) Subscriptions(ctx context.Context, req *pb.SubscriptionsRequest) (*pb.SubscriptionsReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.params.feedManager.GetGrpcSubscriptionReply(), nil
}

// BlxrSubmitBundle submit blxr bundle
func (g *GatewayGrpc) BlxrSubmitBundle(ctx context.Context, req *pb.BlxrSubmitBundleRequest) (*pb.BlxrSubmitBundleReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")

	accountModel, err := g.validateAuthHeader(authHeader, true, false, servers.GetPeerAddr(ctx))
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
		BundlePrice:             req.BundlePrice,
		EnforcePayout:           req.EnforcePayout,
		AvoidMixedBundles:       req.AvoidMixedBundles,
		PriorityFeeRefund:       req.PriorityFeeRefund,
		IncomingRefundRecipient: req.RefundRecipient,
	}

	grpc := connections.NewRPCConn(*accountID, servers.GetPeerAddr(ctx), g.params.sdn.NetworkNum(), utils.GRPC)
	bundleSubmitResult, _, err := servers.HandleMEVBundle(g.params.feedManager, grpc, *accountModel, mevBundleParams)
	if err != nil {
		// TODO need to refactor errors returned from HandleMEVBundle and then map them to jsonrpc and gRPC codes accordingly
		// TODO instead of returning protocol specific error codes
		return nil, err
	}

	return &pb.BlxrSubmitBundleReply{BundleHash: bundleSubmitResult.BundleHash}, nil
}

// BlxrTx submit blxr tx
func (g *GatewayGrpc) BlxrTx(ctx context.Context, req *pb.BlxrTxRequest) (*pb.BlxrTxReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, false, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	grpc := connections.NewRPCConn(*accountID, servers.GetPeerAddr(ctx), g.params.sdn.NetworkNum(), utils.GRPC)
	txHash, ok, err := servers.HandleSingleTransaction(g.params.feedManager, req.Transaction, nil, grpc,
		req.ValidatorsOnly, req.NextValidator, req.NodeValidation, req.FrontrunningProtection, uint16(req.Fallback),
		g.params.feedManager.GetNextValidatorMap(), g.params.feedManager.GetValidatorStatusMap())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if !ok {
		return nil, nil
	}

	log.Infof("grpc blxr_tx: Hash - 0x%v", txHash)
	return &pb.BlxrTxReply{TxHash: txHash}, nil
}

// BlxrBatchTX submit batch blxr txs
func (g *GatewayGrpc) BlxrBatchTX(ctx context.Context, req *pb.BlxrBatchTXRequest) (*pb.BlxrBatchTXReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, false, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	var txHashes []*pb.TxIndex
	var txErrors []*pb.ErrorIndex
	transactionsAndSenders := req.GetTransactionsAndSenders()

	batchTxLimit := 10
	if len(transactionsAndSenders) > batchTxLimit {
		txError := fmt.Sprintf("blxr-batch-tx currently supports a maximum of %v transactions", batchTxLimit)
		txErrors = append(txErrors, &pb.ErrorIndex{Idx: 0, Error: txError})
		return &pb.BlxrBatchTXReply{TxErrors: txErrors}, nil
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	grpc := connections.NewRPCConn(*accountID, servers.GetPeerAddr(ctx), g.params.sdn.NetworkNum(), utils.GRPC)

	for idx, transactionsAndSender := range transactionsAndSenders {
		tx := transactionsAndSender.GetTransaction()
		txHash, ok, err := servers.HandleSingleTransaction(g.params.feedManager, tx, transactionsAndSender.GetSender(), grpc,
			req.ValidatorsOnly, req.NextValidator, req.NodeValidation, req.FrontrunningProtection,
			uint16(req.Fallback), g.params.feedManager.GetNextValidatorMap(), g.params.feedManager.GetValidatorStatusMap())
		if err != nil {
			txErrors = append(txErrors, &pb.ErrorIndex{Idx: int32(idx), Error: err.Error()})
			continue
		}
		if !ok {
			continue
		}
		txHashes = append(txHashes, &pb.TxIndex{Idx: int32(idx), TxHash: txHash})
	}

	g.log.WithFields(log.Fields{
		"networkTime":    startTime.Sub(time.Unix(0, req.GetSendingTime())),
		"handleTime":     time.Now().Sub(startTime),
		"txsSuccess":     len(txHashes),
		"txsError":       len(txErrors),
		"validatorsOnly": req.ValidatorsOnly,
		"nextValidator":  req.NextValidator,
		"fallback":       req.Fallback,
		"nodeValidation": req.NodeValidation,
	}).Debug("blxr-batch-tx")

	return &pb.BlxrBatchTXReply{TxHashes: txHashes, TxErrors: txErrors}, nil
}

// NewTxs subscribe to new txs feed
func (g *GatewayGrpc) NewTxs(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.handleTransactions(req, stream, types.NewTxsFeed, *accountModel)
}

// PendingTxs subscribe to pending txs feed
func (g *GatewayGrpc) PendingTxs(req *pb.TxsRequest, stream pb.Gateway_PendingTxsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.handleTransactions(req, stream, types.PendingTxsFeed, *accountModel)
}

// NewBlocks subscribe to new blocks feed
func (g *GatewayGrpc) NewBlocks(req *pb.BlocksRequest, stream pb.Gateway_NewBlocksServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.handleBlocks(req, stream, types.NewBlocksFeed, *accountModel)
}

// BdnBlocks subscribe to bdn blocks feed
func (g *GatewayGrpc) BdnBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.handleBlocks(req, stream, types.BDNBlocksFeed, *accountModel)
}

// EthOnBlock handler for stream of changes in the EVM state when a new block is mined
func (g *GatewayGrpc) EthOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return g.ethOnBlock(req, stream, *accountModel)
}

// ShortIDs returns short ids for the given tx hashes
func (g *GatewayGrpc) ShortIDs(ctx context.Context, req *pb.ShortIDsRequest) (*pb.ShortIDsReply, error) {
	authHeader, err := bxrpc.ReadAuthMetadata(ctx)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	if _, err = g.validateAuthHeader(authHeader, false, false, servers.GetPeerAddr(ctx)); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.params.blockProposer.ShortIDs(ctx, req)
}

func (g *GatewayGrpc) handleTransactions(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer, feedType types.FeedType, account sdnmessage.Account) error {
	var expr conditions.Expr
	if req.GetFilters() != "" {
		var err error
		expr, err = servers.ValidateFilters(req.GetFilters(), g.params.txFromFieldIncludable)
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	includes, err := servers.ValidateIncludeParam(feedType, req.GetIncludes(), g.params.txFromFieldIncludable)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: servers.GetPeerAddr(stream.Context()),
	}

	ro := types.ReqOptions{
		Filters: req.GetFilters(),
	}

	sub, err := g.params.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, ro, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to subscribe to gRPC %v feed", feedType))
	}
	defer g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	clReq := &servers.ClientReq{Includes: includes, Expr: expr, Feed: feedType}

	var txsResponse []*pb.Tx
	for notification := range sub.FeedChan {
		processTx(clReq, notification, &txsResponse, ci.RemoteAddress, account.AccountID, feedType, g.params.txFromFieldIncludable)

		if (len(sub.FeedChan) == 0 || len(txsResponse) == maxTxsInSingleResponse) && len(txsResponse) > 0 {
			err = stream.Send(&pb.TxsReply{Tx: txsResponse})
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}

			txsResponse = txsResponse[:0]
		}
	}
	return nil
}

func processTx(clientReq *servers.ClientReq, notification types.Notification, multiTxsResponse *[]*pb.Tx, remoteAddress string, accountID types.AccountID, feedType types.FeedType, txFromFieldIncludable bool) {
	var transaction *types.NewTransactionNotification
	switch feedType {
	case types.NewTxsFeed:
		transaction = (notification).(*types.NewTransactionNotification)
	case types.PendingTxsFeed:
		tx := (notification).(*types.PendingTransactionNotification)
		transaction = &tx.NewTransactionNotification
	}

	if shouldSendTx(clientReq, transaction, remoteAddress, accountID) {
		*multiTxsResponse = append(*multiTxsResponse, makeTransaction(transaction, txFromFieldIncludable))
	}
}

func shouldSendTx(clientReq *servers.ClientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID types.AccountID) bool {
	if clientReq.Expr == nil {
		return true
	}

	filters := clientReq.Expr.Args()
	txFilters := tx.Filters(filters)

	// should be done after tx.Filters() to avoid nil pointer dereference
	txType := tx.BlockchainTransaction.(*types.EthTransaction).Type()

	if !servers.IsFiltersSupportedByTxType(txType, filters) {
		return false
	}

	// Evaluate if we should send the tx
	shouldSend, err := conditions.Evaluate(clientReq.Expr, txFilters)
	if err != nil {
		log.Errorf("error evaluate Filters. Feed: %v. filters: %s. remote address: %v. account id: %v error - %v tx: %v",
			clientReq.Feed, clientReq.Expr, remoteAddress, accountID, err.Error(), txFilters)
		return false
	}

	return shouldSend
}

func makeTransaction(transaction *types.NewTransactionNotification, txFromFieldIncludable bool) *pb.Tx {
	tx := &pb.Tx{
		LocalRegion: transaction.LocalRegion(),
		Time:        time.Now().UnixNano(),
		RawTx:       transaction.RawTx(),
	}

	if txFromFieldIncludable {
		// Need to have entire transaction to get sender
		if err := transaction.MakeBlockchainTransaction(); err != nil {
			log.Errorf("error making blockchain transaction: %v", err)
			return tx
		}

		sender, err := transaction.BlockchainTransaction.Sender()
		if err != nil {
			log.Errorf("error getting sender from blockchain transaction: %v", err)
		} else {
			tx.From = sender.Bytes()
		}
	}

	return tx
}

func (g *GatewayGrpc) ethOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: servers.GetPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(types.OnBlockFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to subscribe to gRPC ethOnBlock")
	}

	defer g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validOnBlockParams
	} else {
		includes = req.GetIncludes()
	}

	calls := make(map[string]*servers.RPCCall)
	for idx, callParams := range req.GetCallParams() {
		if callParams == nil {
			return status.Error(codes.InvalidArgument, "call-params cannot be nil")
		}
		err = servers.FillCalls(g.params.wsManager, calls, idx, callParams.Params)
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	for {
		notification, ok := <-sub.FeedChan
		if !ok {
			return status.Error(codes.Internal, "error when reading new block from gRPC ethOnBlock")
		}

		block := notification.(*types.EthBlockNotification)
		sendEthOnBlockGrpcNotification := func(notification *types.OnBlockNotification) error {
			ethOnBlockNotificationReply := notification.WithFields(includes).(*types.OnBlockNotification)
			grpcEthOnBlockNotificationReply := generateEthOnBlockReply(ethOnBlockNotificationReply)
			return stream.Send(grpcEthOnBlockNotificationReply)
		}

		err = servers.HandleEthOnBlock(g.params.wsManager, g.params.feedManager, block, calls, sendEthOnBlockGrpcNotification)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
}

func generateEthOnBlockReply(n *types.OnBlockNotification) *pb.EthOnBlockReply {
	return &pb.EthOnBlockReply{
		Name:        n.Name,
		Response:    n.Response,
		BlockHeight: n.BlockHeight,
		Tag:         n.Tag,
	}
}

func (g *GatewayGrpc) handleBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: servers.GetPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to subscribe to gRPC %v Feed", feedType))
	}
	defer g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validBlockParams
	} else {
		includes = req.GetIncludes()
	}

	for {
		select {
		case notification, ok := <-sub.FeedChan:
			if !ok {
				return status.Error(codes.Internal, "error when reading new notification for gRPC bdnBlocks")
			}

			blocks := notification.WithFields(includes).(*types.EthBlockNotification)
			blocksReply := g.generateBlockReply(blocks)
			blocksReply.SubscriptionID = sub.SubscriptionID

			err = stream.Send(blocksReply)
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
		}
	}
}

func (g *GatewayGrpc) generateBlockReply(n *types.EthBlockNotification) *pb.BlocksReply {
	blockReply := &pb.BlocksReply{}
	if n.BlockHash != nil {
		blockReply.Hash = n.BlockHash.String()
	}
	if n.Header != nil {
		blockReply.Header = g.generateBlockReplyHeader(n.Header)
	}
	for _, vi := range n.ValidatorInfo {
		blockReply.FutureValidatorInfo = append(blockReply.FutureValidatorInfo, &pb.FutureValidatorInfo{
			BlockHeight: strconv.FormatUint(vi.BlockHeight, 10),
			WalletId:    vi.WalletID,
			Accessible:  strconv.FormatBool(vi.Accessible),
		})
	}

	for index, tx := range n.Transactions {
		var from []byte
		if f, ok := tx["from"]; ok {
			from = g.decodeHex(f.(string))
		}

		blockTx := &pb.Tx{
			From:  from,
			RawTx: n.GetRawTxByIndex(index),
		}

		blockReply.Transaction = append(blockReply.Transaction, blockTx)
	}
	for _, withdrawal := range n.Withdrawals {
		blockReply.Withdrawals = append(blockReply.Withdrawals, &pb.Withdrawal{
			Address:        withdrawal.Address.Hex(),
			Amount:         hexutil.Uint64(withdrawal.Amount).String(),
			Index:          hexutil.Uint64(withdrawal.Index).String(),
			ValidatorIndex: hexutil.Uint64(withdrawal.Validator).String(),
		})
	}
	return blockReply
}

func (*GatewayGrpc) decodeHex(data string) []byte {
	hexBytes, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if err != nil {
		log.Errorf("Error decoding hexadecimal string: %v", err)
		hexBytes = nil
	}
	return hexBytes
}

func (*GatewayGrpc) generateBlockReplyHeader(h *types.Header) *pb.BlockHeader {
	blockReplyHeader := pb.BlockHeader{}
	blockReplyHeader.ParentHash = h.ParentHash.String()
	blockReplyHeader.Sha3Uncles = h.Sha3Uncles.String()
	blockReplyHeader.Miner = strings.ToLower(h.Miner.String())
	blockReplyHeader.StateRoot = h.StateRoot.String()
	blockReplyHeader.TransactionsRoot = h.TransactionsRoot.String()
	blockReplyHeader.ReceiptsRoot = h.ReceiptsRoot.String()
	blockReplyHeader.LogsBloom = h.LogsBloom
	blockReplyHeader.Difficulty = h.Difficulty
	blockReplyHeader.Number = h.Number
	blockReplyHeader.GasLimit = h.GasLimit
	blockReplyHeader.GasUsed = h.GasUsed
	blockReplyHeader.Timestamp = h.Timestamp
	blockReplyHeader.ExtraData = h.ExtraData
	blockReplyHeader.MixHash = h.MixHash.String()
	blockReplyHeader.Nonce = h.Nonce
	if h.WithdrawalsHash != nil {
		blockReplyHeader.WithdrawalsRoot = h.WithdrawalsHash.String()
	}
	if h.BaseFee != nil {
		blockReplyHeader.BaseFeePerGas = strconv.FormatInt(int64(*h.BaseFee), 10)
	}
	return &blockReplyHeader
}

// ProposedBlock proposes a block to the validator
func (g *GatewayGrpc) ProposedBlock(ctx context.Context, req *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error) {
	authHeader, err := bxrpc.ReadAuthMetadata(ctx)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	if _, err = g.validateAuthHeader(authHeader, false, false, servers.GetPeerAddr(ctx)); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.params.blockProposer.ProposedBlock(ctx, req)
}

// ProposedBlockStats method for getting block stats
func (g *GatewayGrpc) ProposedBlockStats(ctx context.Context, req *pb.ProposedBlockStatsRequest) (*pb.ProposedBlockStatsReply, error) {
	authHeader, err := bxrpc.ReadAuthMetadata(ctx)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	if _, err = g.validateAuthHeaderWithContext(ctx, authHeader, false, false); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.params.blockProposer.ProposedBlockStats(ctx, req)
}

// TxReceipts handler for stream of all transaction receipts in each newly mined block
func (g *GatewayGrpc) TxReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true, servers.GetPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return g.txReceipts(req, stream, *accountModel)
}

func (g *GatewayGrpc) txReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: servers.GetPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(types.TxReceiptsFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to subscribe to gRPC txReceipts")
	}
	defer g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validTxReceiptParams
	} else {
		includes = req.GetIncludes()
	}

	for {
		select {
		case errMsg := <-sub.ErrMsgChan:
			return status.Error(codes.Internal, errMsg)
		case notification := <-sub.FeedChan:
			txReceiptsNotificationReply := notification.WithFields(includes).(*types.TxReceiptsNotification)
			for _, receipt := range txReceiptsNotificationReply.Receipts {
				grpcTxReceiptsNotificationReply := generateTxReceiptReply(receipt)
				if err := stream.Send(grpcTxReceiptsNotificationReply); err != nil {
					return status.Error(codes.Internal, err.Error())
				}
			}
		}
	}
}

func generateTxReceiptReply(n *types.TxReceipt) *pb.TxReceiptsReply {
	txReceiptsReply := &pb.TxReceiptsReply{
		BlocKHash:         n.BlockHash,
		BlockNumber:       n.BlockNumber,
		ContractAddress:   interfaceToString(n.ContractAddress),
		CumulativeGasUsed: n.CumulativeGasUsed,
		EffectiveGasUsed:  n.EffectiveGasPrice,
		From:              interfaceToString(n.From),
		GasUsed:           n.GasUsed,
		LogsBloom:         n.LogsBloom,
		Status:            n.Status,
		To:                interfaceToString(n.To),
		TransactionHash:   n.TransactionHash,
		TransactionIndex:  n.TransactionIndex,
		Type:              n.TxType,
		TxsCount:          n.TxsCount,
	}

	for _, receiptLog := range n.Logs {
		receiptLogMap, ok := receiptLog.(map[string]interface{})
		if !ok {
			continue
		}

		txReceiptsReply.Logs = append(txReceiptsReply.Logs, &pb.TxLogs{
			Address: interfaceToString(receiptLogMap["address"]),
			Topics: func(topics []string) []string {
				var stringTopics []string
				for _, topic := range topics {
					stringTopics = append(stringTopics, topic)
				}
				return stringTopics
			}(interfaceToStringArray(receiptLogMap["topics"])),
			Data:             interfaceToString(receiptLogMap["data"]),
			BlockNumber:      interfaceToString(receiptLogMap["blockNumber"]),
			TransactionHash:  interfaceToString(receiptLogMap["transactionHash"]),
			TransactionIndex: interfaceToString(receiptLogMap["transactionIndex"]),
			BlockHash:        interfaceToString(receiptLogMap["blockHash"]),
			LogIndex:         interfaceToString(receiptLogMap["logIndex"]),
			Removed:          interfaceToBool(receiptLogMap["removed"]),
		})
	}

	return txReceiptsReply
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

func (g *GatewayGrpc) validateAuthHeaderWithContext(ctx context.Context, authHeader string, required bool, allowAccessToInternalGateway bool) (*sdnmessage.Account, error) {
	accountModel, err := g.validateAuthHeader(authHeader, required, allowAccessToInternalGateway, servers.GetPeerAddr(ctx))
	if err != nil {
		return accountModel, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return accountModel, err
	}
}

// TxsFromShortIDs txs from short ids
func (g *GatewayGrpc) TxsFromShortIDs(ctx context.Context, req *pb.ShortIDListRequest) (*pb.TxListReply, error) {
	_, err := g.validateAuthHeader(retrieveAuthHeader(ctx, req.AuthHeader), false, false, servers.GetPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	shortIDs := req.GetShortIDs()
	if len(shortIDs) == 0 {
		return nil, errors.New("missing shortIDs")
	}

	txList := make([][]byte, 0, len(shortIDs))

	for _, shortID := range shortIDs {
		if shortID == 0 {
			txList = append(txList, []byte{})
			continue
		}
		txStoreTx, err := g.TxStore.GetTxByShortID(types.ShortID(shortID))
		if err != nil {
			return nil, errors.New("failed decompressing")
		}
		txList = append(txList, txStoreTx.Content())
	}

	return &pb.TxListReply{
		Txs: txList,
	}, nil
}

func (g *GatewayGrpc) broadcast(msg bxmessage.Message, source connections.Conn, to utils.NodeType) types.BroadcastResults {
	results := types.BroadcastResults{}

	g.ConnectionsLock.RLock()
	for _, conn := range g.Connections {
		connectionType := conn.GetConnectionType()

		// if a connection type is not in target - skip
		if connectionType&to == 0 {
			continue
		}

		results.RelevantPeers++
		if !conn.IsOpen() || source != nil && conn.ID() == source.ID() {
			results.NotOpenPeers++
			continue
		}

		err := conn.Send(msg)
		if err != nil {
			conn.Log().Errorf("error writing to connection, closing")
			results.ErrorPeers++
			continue
		}

		if connections.IsGateway(connectionType) {
			results.SentGatewayPeers++
		}

		results.SentPeers++
	}
	g.ConnectionsLock.RUnlock()

	return results
}

func (g *GatewayGrpc) notify(notification types.Notification) {
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled {
		select {
		case g.params.feedManagerChan <- notification:
		default:
			g.log.Warnf("gateway feed channel is full. Can't add %v without blocking. Ignoring hash %v", reflect.TypeOf(notification), notification.GetHash())
		}
	}
}
