package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/sourcegraph/jsonrpc2"
)

func (h *handlerObj) handleRPCBundleSubmission(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if h.FeedManager.accountModel.AccountID != h.connectionAccount.AccountID {
		errDifferentAccAuth := fmt.Sprintf(errFDifferentAccAuth, jsonrpc.RPCBundleSubmission)
		h.log.Errorf("%v. account auth: %v, node account: %v", errDifferentAccAuth, h.connectionAccount.AccountID, h.FeedManager.accountModel.AccountID)
		SendErrorMsg(ctx, jsonrpc.AccountIDError, errDifferentAccAuth, conn, req.ID)
		return
	}

	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	// Defaults
	params := jsonrpc.RPCBundleSubmissionPayload{
		BlockchainNetwork: bxgateway.Mainnet,
		Frontrunning:      true,
	}

	if err := json.Unmarshal(*req.Params, &params); err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCBundleSubmission, err), conn, req.ID)
		return
	}

	h.handleMEVBundle(ctx, conn, req, &params)
}

func (h *handlerObj) handleMEVBundle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, params *jsonrpc.RPCBundleSubmissionPayload) {
	// If MEVBuilders request parameter is empty, only send to default builders.
	if len(params.MEVBuilders) == 0 {
		params.MEVBuilders = map[string]string{
			bxgateway.BloxrouteBuilderName: "",
			bxgateway.FlashbotsBuilderName: "",
		}
	}

	mevBundle, bundleHash, err := mevBundleFromRequest(params)
	var result interface{}
	if params.UUID == "" {
		result = GatewayBundleResponse{BundleHash: bundleHash}
	}
	if err != nil {
		if errors.Is(err, errBlockedTxHashes) {
			if err = conn.Reply(ctx, req.ID, result); err != nil {
				h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
			}
			return
		}

		SendErrorMsg(ctx, jsonrpc2.CodeInvalidParams, err.Error(), conn, req.ID)
		return
	}
	mevBundle.SetNetworkNum(h.FeedManager.networkNum)

	if !h.connectionAccount.TierName.IsEnterprise() {
		h.log.Tracef("%s rejected for non Enterprise account %v tier %v", mevBundle, h.connectionAccount.AccountID, h.connectionAccount.TierName)
		SendErrorMsg(ctx, jsonrpc2.CodeInvalidRequest, "Enterprise account is required in order to send bundle", conn, req.ID)
		return
	}

	maxTxsLen := h.connectionAccount.Bundles.Networks[params.BlockchainNetwork].TxsLenLimit
	if maxTxsLen > 0 && len(mevBundle.Transactions) > maxTxsLen {
		h.log.Tracef("%s rejected for exceeding txs limit %v", mevBundle, maxTxsLen)
		SendErrorMsg(ctx, jsonrpc2.CodeInvalidRequest, fmt.Sprintf("txs limit exceeded, max txs allowed: %v", maxTxsLen), conn, req.ID)
		return
	}

	var ws connections.RPCConn
	if h.connectionAccount.AccountID == types.BloxrouteAccountID {
		// Bundle sent from cloud services, need to update account ID of the connection to be the origin sender
		ws = connections.NewRPCConn(types.AccountID(params.OriginalSenderAccountID), h.remoteAddress, h.FeedManager.networkNum, utils.CloudAPI)
	} else {
		ws = connections.NewRPCConn(h.connectionAccount.AccountID, h.remoteAddress, h.FeedManager.networkNum, utils.Websocket)
	}

	if err = h.FeedManager.node.HandleMsg(mevBundle, ws, connections.RunForeground); err != nil {
		// err here is not possible right now but anyway we don't want expose reason of internal error to the client
		h.log.Errorf("failed to process %s: %v", mevBundle, err)
		SendErrorMsg(ctx, jsonrpc2.CodeInternalError, "", conn, req.ID)
		return
	}

	if err = conn.Reply(ctx, req.ID, result); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
	}
}
