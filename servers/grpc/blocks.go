package grpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	"github.com/golang/protobuf/ptypes/wrappers"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

var (
	txContentFields = []string{"tx_contents.nonce", "tx_contents.tx_hash",
		"tx_contents.gas_price", "tx_contents.gas", "tx_contents.to", "tx_contents.value", "tx_contents.input",
		"tx_contents.v", "tx_contents.r", "tx_contents.s", "tx_contents.from", "tx_contents.type", "tx_contents.access_list",
		"tx_contents.chain_id", "tx_contents.max_priority_fee_per_gas", "tx_contents.max_fee_per_gas", "tx_contents.max_fee_per_blob_gas",
		"tx_contents.blob_versioned_hashes", "tx_contents.y_parity", "tx_contents.authorization_list"}
	validOnBlockParams = []string{"name", "response", "block_height", "tag"}
	validBlockParams   = append(txContentFields, "tx_contents.from", "hash", "header", "transactions", "uncles", "future_validator_info", "withdrawals")
)

// NewBlocks subscribe to new blocks feed
func (g *server) NewBlocks(req *pb.BlocksRequest, stream pb.Gateway_NewBlocksServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.GetAuthHeader()) //nolint:staticcheck

	accountModel, err := g.validateAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	if req.GetParsedTxs() == nil {
		req.ParsedTxs = &wrappers.BoolValue{Value: true}
	}

	return g.handleBlocks(req, stream, types.NewBlocksFeed, *accountModel)
}

// BdnBlocks subscribe to bdn blocks feed
func (g *server) BdnBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.GetAuthHeader()) //nolint:staticcheck

	accountModel, err := g.validateAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	if req.GetParsedTxs() == nil {
		req.ParsedTxs = &wrappers.BoolValue{Value: true}
	}

	return g.handleBlocks(req, stream, types.BDNBlocksFeed, *accountModel)
}

// EthOnBlock handler for stream of changes in the EVM state when a new block is mined
func (g *server) EthOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.GetAuthHeader()) //nolint:staticcheck

	accountModel, err := g.validateAuthHeader(authHeader, true, true, getPeerAddr(stream.Context()))
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return g.ethOnBlock(req, stream, *accountModel)
}

func (g *server) handleBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer, feedType types.FeedType, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(feedType, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("failed to subscribe to gRPC %v Feed", feedType))
	}

	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %v feed: %v", feedType, err)
		}
	}()

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validBlockParams
	} else {
		includes = req.GetIncludes()
	}
	if !req.GetParsedTxs().GetValue() {
		if i := slices.Index(includes, "transactions"); i >= 0 {
			includes = slices.Replace(includes, i, i+1, "raw_transactions")
		}
	} else {
		if i := slices.Index(includes, "transactions"); i >= 0 {
			includes = slices.Replace(includes, i, i+1, "transactions_without_sender")
		}
	}

	for {
		select {
		case notification, ok := <-sub.FeedChan:
			if !ok {
				return status.Error(codes.Internal, "error when reading new notification for gRPC bdnBlocks")
			}

			blocks := notification.WithFields(includes).(*types.EthBlockNotification)
			blocksReply := g.generateBlockReply(blocks, req.GetParsedTxs().GetValue())

			err = stream.Send(blocksReply)
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
		case <-stream.Context().Done():
			log.Debugf("stream cancelled: remoteAddress: %s", ci.RemoteAddress)
			return nil
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

func (g *server) generateTxsWithSenders(n *types.EthBlockNotification) []map[string]interface{} {
	senders := g.params.senderExtractor.GetSendersFromBlockTxs(n.Block)
	return n.GetTxs(senders)
}

func (g *server) generateBlockReply(n *types.EthBlockNotification, parsedTxs bool) *pb.BlocksReply {
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

	if parsedTxs {
		blockReply.Transaction = g.generateBlockReplyWithParsedTxs(n)
	} else {
		blockReply.Transaction = g.generateBlockReplyWithRawTxs(n)
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

func (g *server) generateBlockReplyWithRawTxs(n *types.EthBlockNotification) []*pb.Tx {
	rawTxs := make([]*pb.Tx, 0)
	for _, tx := range n.GetRawTransactions() {
		rawTxs = append(rawTxs, &pb.Tx{RawTx: tx})
	}
	return rawTxs
}

func (g *server) generateBlockReplyWithParsedTxs(n *types.EthBlockNotification) []*pb.Tx {
	parsedTxs := make([]*pb.Tx, 0)
	for index, tx := range g.generateTxsWithSenders(n) {
		var from []byte
		if f, ok := tx["from"]; ok {
			from = g.decodeHex(f.(string))
		}

		blockTx := &pb.Tx{
			From:  from,
			RawTx: n.GetRawTxByIndex(index),
		}

		parsedTxs = append(parsedTxs, blockTx)
	}
	return parsedTxs
}

func (*server) decodeHex(data string) []byte {
	hexBytes, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if err != nil {
		log.Errorf("error decoding hexadecimal string: %v", err)
		hexBytes = nil
	}
	return hexBytes
}

func (*server) generateBlockReplyHeader(h *types.Header) *pb.BlockHeader {
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
	if h.RequestsHash != nil {
		blockReplyHeader.RequestsHash = h.RequestsHash.String()
	}
	if h.BlobGasUsed != "" {
		blockReplyHeader.BlobGasUsed = h.BlobGasUsed
	}
	if h.ExcessBlobGas != "" {
		blockReplyHeader.ExcessBlobGas = h.ExcessBlobGas
	}
	if h.ParentBeaconRoot != nil {
		blockReplyHeader.ParentBeaconRoot = h.ParentBeaconRoot.String()
	}
	return &blockReplyHeader
}

func (g *server) ethOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer, account sdnmessage.Account) error {
	ci := types.ClientInfo{
		AccountID:     account.AccountID,
		Tier:          string(account.TierName),
		MetaInfo:      types.SDKMetaFromContext(stream.Context()),
		RemoteAddress: getPeerAddr(stream.Context()),
	}

	sub, err := g.params.feedManager.Subscribe(types.OnBlockFeed, types.GRPCFeed, nil, ci, types.ReqOptions{}, false)
	if err != nil {
		return status.Error(codes.InvalidArgument, "failed to subscribe to gRPC ethOnBlock")
	}

	defer func() {
		err = g.params.feedManager.Unsubscribe(sub.SubscriptionID, false, "")
		if err != nil {
			log.Errorf("failed to unsubscribe from gRPC %s feed: %v", types.OnBlockFeed, err)
		}
	}()

	var includes []string
	if len(req.GetIncludes()) == 0 {
		includes = validOnBlockParams
	} else {
		includes = req.GetIncludes()
	}

	calls := make(map[string]*handler.RPCCall)
	for idx, callParams := range req.GetCallParams() {
		if callParams == nil {
			return status.Error(codes.InvalidArgument, "call-params cannot be nil")
		}
		err = handler.FillCalls(g.params.wsManager, calls, idx, callParams.Params)
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

		err = handler.HandleEthOnBlock(g.params.wsManager, block, calls, sendEthOnBlockGrpcNotification)
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}
}

func (g *server) validateAuthHeaderWithContext(ctx context.Context, authHeader string, required bool, allowAccessToInternalGateway bool) (*sdnmessage.Account, error) {
	accountModel, err := g.validateAuthHeader(authHeader, required, allowAccessToInternalGateway, getPeerAddr(ctx))
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
