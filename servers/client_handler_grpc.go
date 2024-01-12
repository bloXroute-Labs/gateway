package servers

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/zhouzhuojie/conditions"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

//go:generate mockgen -destination ../../bxgateway/test/mock/mock_grpc_feed_manager.go -package mock . GRPCFeedManager

// GRPCFeedManager declares the interface of the feed manager for grpc handler
type GRPCFeedManager interface {
	Subscribe(feedName types.FeedType, feedConnectionType types.FeedConnectionType, conn *jsonrpc2.Conn, ci types.ClientInfo, ro types.ReqOptions, ethSubscribe bool) (*ClientSubscriptionHandlingInfo, error)
	Unsubscribe(subscriptionID string, closeClientConnection bool, errMsg string) error
	GetSyncedWSProvider(preferredProviderEndpoint *types.NodeEndpoint) (blockchain.WSProvider, bool)
}

const maxTxsInSingleResponse = 50

// GrpcHandler is an instance to handle gateway GRPC requests(part of requests)
type GrpcHandler struct {
	feedManager           GRPCFeedManager
	nodeWSManager         blockchain.WSManager
	txFromFieldIncludable bool
	IntentsStore          *services.UserIntentStore
}

// NewGrpcHandler create new instance of GrpcHandler
func NewGrpcHandler(feedManager GRPCFeedManager, nodeWSManager blockchain.WSManager, txFromFieldIncludable bool) *GrpcHandler {
	return &GrpcHandler{
		feedManager:           feedManager,
		nodeWSManager:         nodeWSManager,
		txFromFieldIncludable: txFromFieldIncludable,
		IntentsStore:          services.NewUserIntentsStore(),
	}
}

func (*GrpcHandler) decodeHex(data string) []byte {
	hexBytes, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if err != nil {
		log.Errorf("Error decoding hexadecimal string: %v", err)
		hexBytes = nil
	}
	return hexBytes
}

func (*GrpcHandler) generateBlockReplyHeader(h *types.Header) *pb.BlockHeader {
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

func (g *GrpcHandler) generateBlockReply(n *types.EthBlockNotification) *pb.BlocksReply {
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

func generateEthOnBlockReply(n *types.OnBlockNotification) *pb.EthOnBlockReply {
	return &pb.EthOnBlockReply{
		Name:        n.Name,
		Response:    n.Response,
		BlockHeight: n.BlockHeight,
		Tag:         n.Tag,
	}
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

// NewTxs handler for stream of new transactions
func (g *GrpcHandler) NewTxs(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer, account sdnmessage.Account) error {
	return g.handleTransactions(req, stream, types.NewTxsFeed, account)
}

// PendingTxs handler for stream of pending transactions
func (g *GrpcHandler) PendingTxs(req *pb.TxsRequest, stream pb.Gateway_PendingTxsServer, account sdnmessage.Account) error {
	return g.handleTransactions(req, stream, types.PendingTxsFeed, account)
}

func processTx(clientReq *clientReq, notification types.Notification, multiTxsResponse *[]*pb.Tx, remoteAddress string, accountID types.AccountID, feedType types.FeedType, txFromFieldIncludable bool) {
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

func (g *GrpcHandler) handleTransactions(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer, feedType types.FeedType, account sdnmessage.Account) error {
	var expr conditions.Expr
	if req.GetFilters() != "" {
		var err error
		expr, err = validateFilters(req.GetFilters(), g.txFromFieldIncludable)
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	includes, err := validateIncludeParam(feedType, req.GetIncludes(), g.txFromFieldIncludable)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: GetPeerAddr(stream.Context()),
	}

	ro := types.ReqOptions{
		Filters: req.GetFilters(),
	}

	sub, err := g.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, ro, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to subscribe to gRPC %v feed", feedType))
	}
	defer g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	clReq := &clientReq{includes: includes, expr: expr, feed: feedType}

	var txsResponse []*pb.Tx
	for notification := range sub.FeedChan {
		processTx(clReq, notification, &txsResponse, ci.RemoteAddress, account.AccountID, feedType, g.txFromFieldIncludable)

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

// NewBlocks handler for stream of new blocks
func (g *GrpcHandler) NewBlocks(req *pb.BlocksRequest, stream pb.Gateway_NewBlocksServer, account sdnmessage.Account) error {
	return g.handleBlocks(req, stream, types.NewBlocksFeed, account)
}

// BdnBlocks handler for stream of BDN blocks
func (g *GrpcHandler) BdnBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer, account sdnmessage.Account) error {
	return g.handleBlocks(req, stream, types.BDNBlocksFeed, account)
}

// EthOnBlock handler for stream of changes in the EVM state when a new block is mined
func (g *GrpcHandler) EthOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: GetPeerAddr(stream.Context()),
	}

	sub, err := g.feedManager.Subscribe(types.OnBlockFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to subscribe to gRPC ethOnBlock")
	}

	defer g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validOnBlockParams
	} else {
		includes = req.GetIncludes()
	}

	calls := make(map[string]*RPCCall)
	for idx, callParams := range req.GetCallParams() {
		if callParams == nil {
			return status.Error(codes.InvalidArgument, "call-params cannot be nil")
		}
		err = fillCalls(g.nodeWSManager, calls, idx, callParams.Params)
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

		err = handleEthOnBlock(g.nodeWSManager, g.feedManager, block, calls, sendEthOnBlockGrpcNotification)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
}

// TxReceipts handler for stream of all transaction receipts in each newly mined block
func (g *GrpcHandler) TxReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: GetPeerAddr(stream.Context()),
	}

	sub, err := g.feedManager.Subscribe(types.TxReceiptsFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to subscribe to gRPC txReceipts")
	}
	defer g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validTxReceiptParams
	} else {
		includes = req.GetIncludes()
	}

	for notification := range sub.FeedChan {
		txReceiptsNotificationReply := notification.WithFields(includes).(*types.TxReceiptsNotification)
		for _, receipt := range txReceiptsNotificationReply.Receipts {
			grpcTxReceiptsNotificationReply := generateTxReceiptReply(receipt)
			if err := stream.Send(grpcTxReceiptsNotificationReply); err != nil {
				return status.Error(codes.Internal, err.Error())
			}
		}
	}

	return status.Error(codes.Internal, "error when reading new block from gRPC txReceipts")
}

func (g *GrpcHandler) handleBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: GetPeerAddr(stream.Context()),
	}

	sub, err := g.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to subscribe to gRPC %v feed", feedType))
	}
	defer g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

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

// GetPeerAddr returns the address of the gRPC connected client given its context
func GetPeerAddr(ctx context.Context) string {
	var peerAddress string
	if p, ok := peer.FromContext(ctx); ok {
		peerAddress = p.Addr.String()
	}
	return peerAddress
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
