package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
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

	params := jsonrpc.RPCMEVSearcherPayload{
		BlockchainNetwork: bxgateway.Mainnet,
		Frontrunning:      true,
	}
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
		BlockchainNetwork: params.BlockchainNetwork,
		MEVBuilders:       params.MEVBuilders,
		Frontrunning:      params.Frontrunning,
		Transaction:       params.Payload[0].Txs,
		BlockNumber:       params.Payload[0].BlockNumber,
		MinTimestamp:      params.Payload[0].MinTimestamp,
		MaxTimestamp:      params.Payload[0].MaxTimestamp,
		RevertingHashes:   params.Payload[0].RevertingTxHashes,
		UUID:              params.Payload[0].UUID,
	}

	h.handleMEVBundle(ctx, conn, req, mevBundleParams)
}
