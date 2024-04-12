package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/sourcegraph/jsonrpc2"
)

// handleRPCMevSearcher handles RPC request for mev searcher
// Deprecated: use blxr_submit_bundle instead. Will be removed in the future.
func (h *handlerObj) handleRPCMevSearcher(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	// Handler is deprecated and will be removed in the future
	if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCMEVSearcher)
		h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		SendErrorMsg(ctx, jsonrpc.AccountIDError, errDifferentAccAuth, conn, req.ID)
		return
	}

	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCMEVSearcherPayload
	if err := json.Unmarshal(*req.Params, &params); err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCMEVSearcher, err), conn, req.ID)
		return
	}

	if len(params.Payload) != 1 {
		payloadErr := "received invalid number of mevSearcher payload, must be 1 element"
		h.log.Errorf(payloadErr)
		SendErrorMsg(ctx, jsonrpc.InvalidParams, payloadErr, conn, req.ID)
		return
	}

	mevBundleParams := &jsonrpc.RPCBundleSubmissionPayload{
		MEVBuilders:     params.MEVBuilders,
		Transaction:     params.Payload[0].Txs,
		BlockNumber:     params.Payload[0].BlockNumber,
		MinTimestamp:    params.Payload[0].MinTimestamp,
		MaxTimestamp:    params.Payload[0].MaxTimestamp,
		RevertingHashes: params.Payload[0].RevertingTxHashes,
		UUID:            params.Payload[0].UUID,
	}

	var ws connections.RPCConn
	if h.connectionAccount.AccountID == types.BloxrouteAccountID {
		// Bundle sent from cloud services, need to update account ID of the connection to be the origin sender
		ws = connections.NewRPCConn(types.AccountID(mevBundleParams.OriginalSenderAccountID), h.remoteAddress, h.FeedManager.networkNum, utils.CloudAPI)
	} else {
		ws = connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.FeedManager.networkNum, utils.Websocket)
	}

	result, errCode, err := HandleMEVBundle(h.FeedManager, ws, h.connectionAccount, mevBundleParams)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.RPCErrorCode(errCode), err.Error(), conn, req.ID)
		return
	}
	if err = conn.Reply(ctx, req.ID, result); err != nil {
		log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
	}
}
