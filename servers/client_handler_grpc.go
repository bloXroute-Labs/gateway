package servers

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/zhouzhuojie/conditions"
	"google.golang.org/grpc/peer"
)

// GrpcHandler is an instance to handle gateway GRPC requests(part of requests)
type GrpcHandler struct {
	feedManager *FeedManager
}

// NewGrpcHandler create new instance of GrpcHandler
func NewGrpcHandler(feedManager *FeedManager) *GrpcHandler {
	return &GrpcHandler{
		feedManager: feedManager,
	}
}

func (GrpcHandler) decodeHex(data string) []byte {
	hexBytes, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if err != nil {
		log.Errorf("Error decoding hexadecimal string: %v", err)
		hexBytes = nil
	}
	return hexBytes
}

func (GrpcHandler) generateBlockReplyHeader(h *types.Header) *pb.BlockHeader {
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
	if h.WithdrawalsHash != nil {
		blockReplyHeader.WithdrawalsRoot = h.WithdrawalsHash.String()
	}
	if h.BaseFee != nil {
		blockReplyHeader.BaseFeePerGas = strconv.FormatInt(int64(*h.BaseFee), 10)
	}
	return &blockReplyHeader
}

func (g GrpcHandler) generateBlockReply(n *types.EthBlockNotification) *pb.BlocksReply {
	blockReply := &pb.BlocksReply{}
	blockReply.Hash = n.BlockHash.String()
	blockReply.Header = g.generateBlockReplyHeader(n.Header)
	for _, vi := range n.ValidatorInfo {
		blockReply.FutureValidatorInfo = append(blockReply.FutureValidatorInfo, &pb.FutureValidatorInfo{
			BlockHeight: strconv.FormatUint(vi.BlockHeight, 10),
			WalletId:    vi.WalletID,
			Accessible:  strconv.FormatBool(vi.Accessible),
		})
	}

	for index, tx := range n.Transactions {
		blockTx := &pb.Tx{
			From:  g.decodeHex(tx["from"].(string)),
			RawTx: n.GetRawTxByIndex(index),
		}

		blockReply.Transaction = append(blockReply.Transaction, blockTx)
	}
	return blockReply
}

func makeTransaction(transaction types.NewTransactionNotification) *pb.Tx {
	tx := &pb.Tx{
		From:        transaction.Sender().Bytes(),
		LocalRegion: transaction.LocalRegion(),
		Time:        time.Now().UnixNano(),
		RawTx:       transaction.RawTx(),
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

func (g *GrpcHandler) handleTransactions(req *pb.TxsRequest, stream pb.Gateway_PendingTxsServer, feedType types.FeedType, account sdnmessage.Account) error {
	var expr conditions.Expr
	if req.GetFilters() != "" {
		var err error
		expr, err = createFiltersExpression(req.GetFilters())
		if err != nil {
			return err
		}
	}

	sub, err := g.feedManager.Subscribe(feedType, types.GRPCFeed, nil, account.TierName, account.AccountID, "", req.GetFilters(), "", "", false)
	if err != nil {
		return errors.New("failed to subscribe to gRPC pendingTxs")
	}
	defer g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	clientReq := &clientReq{includes: req.GetIncludes(), expr: expr, feed: feedType}

	remoteAddress := "grpc"
	streamPeer, ok := peer.FromContext(stream.Context())
	if ok {
		remoteAddress = streamPeer.Addr.String()
	}

	for {
		select {
		case notification, ok := <-sub.FeedChan:
			if !ok {
				return fmt.Errorf("error when reading new notification for gRPC %v", feedType)
			}

			var transaction *types.NewTransactionNotification
			switch feedType {
			case types.NewTxsFeed:
				transaction = (notification).(*types.NewTransactionNotification)
			case types.PendingTxsFeed:
				tx := (notification).(*types.PendingTransactionNotification)
				transaction = &tx.NewTransactionNotification
			}

			txResult := filterAndInclude(clientReq, transaction, remoteAddress, account.AccountID)
			if txResult != nil {
				txsReply := &pb.TxsReply{Tx: []*pb.Tx{makeTransaction(*transaction)}}
				err = stream.Send(txsReply)
				if err != nil {
					return err
				}
			}

		}
	}
}

// NewBlocks handler for stream of new blocks
func (g *GrpcHandler) NewBlocks(req *pb.BlocksRequest, stream pb.Gateway_NewBlocksServer, account sdnmessage.Account) error {
	return g.handleBlocks(req, stream, types.NewBlocksFeed, account)
}

// BdnBlocks handler for stream of BDN blocks
func (g *GrpcHandler) BdnBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer, account sdnmessage.Account) error {
	return g.handleBlocks(req, stream, types.BDNBlocksFeed, account)
}

func (g *GrpcHandler) handleBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer, feedType types.FeedType, account sdnmessage.Account) error {
	sub, err := g.feedManager.Subscribe(feedType, types.GRPCFeed, nil, account.TierName, account.AccountID, "", "", "", "", false)
	if err != nil {
		return errors.New("failed to subscribe to gRPC bdnBlocks")
	}
	defer g.feedManager.Unsubscribe(sub.SubscriptionID, false, "")

	includes := []string{}
	if len(req.GetIncludes()) == 0 {
		includes = validBlockParams
	} else {
		includes = req.GetIncludes()
	}

	for {
		select {
		case notification, ok := <-sub.FeedChan:
			if !ok {
				return errors.New("error when reading new notification for gRPC bdnBlocks")
			}

			blocks := notification.WithFields(includes).(*types.EthBlockNotification)
			blocksReply := g.generateBlockReply(blocks)
			blocksReply.SubscriptionID = sub.SubscriptionID

			err = stream.Send(blocksReply)
			if err != nil {
				return err
			}
		}
	}
}
