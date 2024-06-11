package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/sourcegraph/jsonrpc2"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
)

var (
	// ErrInvalidSenderAddress is returned when the sender address is invalid
	ErrInvalidSenderAddress = errors.New("sender_address is invalid")
	// ErrInvalidIntent is returned when the intent is invalid
	ErrInvalidIntent = errors.New("intent is required")
)

type rpcIntentResponse struct {
	IntentID  string    `json:"intent_id"`
	FirstSeen time.Time `json:"first_seen,omitempty"`
}

type rpcIntentSolutionResponse struct {
	SolutionID string    `json:"solution_id"`
	FirstSeen  time.Time `json:"first_seen,omitempty"`
}

func (h *handlerObj) handleSubmitIntent(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCSubmitIntentPayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	if err = validateSubmitIntentPayload(&params); err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	id := utils.GenerateUUID()
	t := time.Now()
	intentMsg := bxmessage.NewIntent(id, params.DappAddress, params.SenderAddress, params.Hash, params.Signature, t, params.Intent)

	err = h.FeedManager.node.HandleMsg(intentMsg, nil, connections.RunBackground)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}

	response := rpcIntentResponse{IntentID: id}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}
}

func (h *handlerObj) handleSubmitIntentSolution(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCSubmitIntentPayloadSolution
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	if err = validateSubmitIntentSolutionPayload(&params); err != nil {
		SendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	id := utils.GenerateUUID()
	firstSeen := time.Now()
	intentMsg := bxmessage.NewIntentSolution(id, params.SolverAddress, params.IntentID, params.Hash, params.Signature, firstSeen, params.IntentSolution)

	err = h.FeedManager.node.HandleMsg(intentMsg, nil, connections.RunBackground)
	if err != nil {
		SendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}

	response := rpcIntentSolutionResponse{SolutionID: id, FirstSeen: firstSeen}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}
}

func validateSubmitIntentPayload(payload *jsonrpc.RPCSubmitIntentPayload) error {
	if (payload.DappAddress != payload.SenderAddress) && !common.IsHexAddress(payload.SenderAddress) {
		return ErrInvalidSenderAddress
	}

	if len(payload.Intent) == 0 {
		return ErrInvalidIntent
	}

	return intent.ValidateSignature(payload.SenderAddress, payload.Hash, payload.Signature)
}

func validateSubmitIntentSolutionPayload(payload *jsonrpc.RPCSubmitIntentPayloadSolution) error {
	if len(payload.IntentSolution) == 0 {
		return ErrInvalidIntent
	}

	return intent.ValidateSignature(payload.SolverAddress, payload.Hash, payload.Signature)
}
