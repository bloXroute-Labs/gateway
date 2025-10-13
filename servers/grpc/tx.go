package grpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/filter"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler/validator"
	"github.com/bloXroute-Labs/gateway/v2/servers/ws"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const maxTxsInSingleResponse = 10

var (
	validTxReceiptParams = []string{"block_hash", "block_number", "contract_address",
		"cumulative_gas_used", "effective_gas_price", "from", "gas_used", "logs", "logs_bloom",
		"status", "to", "transaction_hash", "transaction_index", "type", "txs_count"}
)

// BlxrTx submit blxr tx
func (g *server) BlxrTx(ctx context.Context, req *pb.BlxrTxRequest) (*pb.BlxrTxReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	accountModel, err := g.validateAuthHeader(authHeader, true, false, getPeerAddr(ctx))
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	grpc := connections.NewRPCConn(*accountID, getPeerAddr(ctx), g.params.sdn.NetworkNum(), bxtypes.GRPC)
	txHash, ok, err := handler.HandleSingleTransaction(g.params.node, g.params.wsManager, req.Transaction, nil, grpc,
		req.NodeValidation, bxtypes.NetworkNumToChainID[g.params.sdn.NetworkNum()], true)
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
func (g *server) BlxrBatchTX(ctx context.Context, req *pb.BlxrBatchTXRequest) (*pb.BlxrBatchTXReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	accountModel, err := g.validateAuthHeader(authHeader, true, false, getPeerAddr(ctx))
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	var txErrors []*pb.ErrorIndex
	transactionsAndSenders := req.GetTransactionsAndSenders()
	txHashes := make([]*pb.TxIndex, 0, len(transactionsAndSenders))

	batchTxLimit := 10
	if len(transactionsAndSenders) > batchTxLimit {
		txError := fmt.Sprintf("blxr-batch-tx currently supports a maximum of %v transactions", batchTxLimit)
		txErrors = append(txErrors, &pb.ErrorIndex{Idx: 0, Error: txError})
		return &pb.BlxrBatchTXReply{TxErrors: txErrors}, nil
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	grpc := connections.NewRPCConn(*accountID, getPeerAddr(ctx), g.params.sdn.NetworkNum(), bxtypes.GRPC)

	for idx, transactionsAndSender := range transactionsAndSenders {
		tx := transactionsAndSender.GetTransaction()
		txHash, ok, err := handler.HandleSingleTransaction(g.params.node, g.params.wsManager, tx, transactionsAndSender.GetSender(), grpc, req.NodeValidation, g.params.chainID, true)
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
		"handleTime":     time.Since(startTime),
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
func (g *server) NewTxs(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.GetAuthHeader()) //nolint:staticcheck
	accountModel, err := g.validateAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.handleTransactions(req, stream, types.NewTxsFeed, *accountModel)
}

// PendingTxs subscribe to pending txs feed
func (g *server) PendingTxs(req *pb.TxsRequest, stream pb.Gateway_PendingTxsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.GetAuthHeader()) //nolint:staticcheck
	accountModel, err := g.validateAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.handleTransactions(req, stream, types.PendingTxsFeed, *accountModel)
}

// TxReceipts handler for stream of all transaction receipts in each newly mined block
func (g *server) TxReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.GetAuthHeader()) //nolint:staticcheck
	accountModel, err := g.validateAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.txReceipts(req, stream, *accountModel)
}

// TxsFromShortIDs txs from short ids
func (g *server) TxsFromShortIDs(ctx context.Context, req *pb.ShortIDListRequest) (*pb.TxListReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.GetAuthHeader()) //nolint:staticcheck
	_, err := g.validateAuthHeader(authHeader, false, false, getPeerAddr(ctx))
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
		txStoreTx, err := g.params.txStore.GetTxByShortID(types.ShortID(shortID), req.GetWithSidecars())
		if err != nil {
			return nil, errors.New("failed decompressing")
		}
		txList = append(txList, txStoreTx.Content())
	}

	return &pb.TxListReply{
		Txs: txList,
	}, nil
}

func (g *server) shortIDs(req *pb.ShortIDsRequest) (*pb.ShortIDsReply, error) {
	result := pb.ShortIDsReply{ShortIds: make([]uint32, len(req.GetTxHashes()))}

	for i, hash := range req.GetTxHashes() {
		txHash, err := types.NewSHA256Hash(hash)
		if err != nil {
			log.Errorf("failed to create SHA256 hash for input %v, position %v, err %v"+hex.EncodeToString(hash), strconv.FormatInt(int64(i), 10)+err.Error())
			continue
		}

		tx, exist := g.params.txStore.Get(txHash)
		if exist && len(tx.ShortIDs()) > 0 {
			result.ShortIds[i] = uint32(tx.ShortIDs()[0])
		}
	}
	return &result, nil
}

func (g *server) txReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(types.TxReceiptsFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to subscribe to gRPC txReceipts")
	}
	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %s feed: %v", types.TxReceiptsFeed, err)
		}
	}()

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

func (g *server) handleTransactions(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer, feedType types.FeedType, account sdnmessage.Account) error {
	var expr *filter.Expression
	if req.GetFilters() != "" {
		var err error
		expr, err = filter.NewDefaultExpression(req.GetFilters(), g.params.txFromFieldIncludable)
		if err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
	}

	includes, err := validator.ValidateIncludeParam(feedType, req.GetIncludes(), g.params.txFromFieldIncludable)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}

	ro := types.ReqOptions{
		Filters: req.GetFilters(),
	}

	sub, err := g.params.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, ro, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to subscribe to gRPC %v feed", feedType))
	}
	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %s feed: %v", feedType, err)
		}
	}()

	clReq := &ws.ClientReq{Includes: includes, Expr: expr, Feed: feedType}

	containsInclude := slices.Contains(includes, "tx_contents.from")

	var txsResponse []*pb.Tx
	for notification := range sub.FeedChan {
		processTx(clReq, notification, &txsResponse, ci.RemoteAddress, account.AccountID, feedType, containsInclude)

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
			Address:          interfaceToString(receiptLogMap["address"]),
			Topics:           interfaceToStringArray(receiptLogMap["topics"]),
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

func processTx(clientReq *ws.ClientReq, notification types.Notification, multiTxsResponse *[]*pb.Tx, remoteAddress string, accountID bxtypes.AccountID, feedType types.FeedType, txFromFieldIncludable bool) {
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

func shouldSendTx(clientReq *ws.ClientReq, tx *types.NewTransactionNotification, remoteAddress string, accountID bxtypes.AccountID) bool {
	if clientReq.Expr == nil {
		return true
	}

	txFilters := tx.Filters()

	// evaluate if we should send the tx
	shouldSend, err := clientReq.Expr.Evaluate(txFilters)
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
		// need to have entire transaction to get sender
		if err := transaction.MakeEthTransaction(); err != nil {
			log.Errorf("error making blockchain transaction: %v", err)
			return tx
		}

		sender, err := transaction.EthTransaction.Sender()
		if err != nil {
			log.Errorf("error getting sender from blockchain transaction: %v", err)
		} else {
			tx.From = sender.Bytes()
		}
	}

	return tx
}
