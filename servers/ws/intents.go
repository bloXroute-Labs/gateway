package ws

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
	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
)

const (
	solutionsWaitTime     = time.Millisecond * 50
	solutionsPollInterval = time.Millisecond * 5
)

var (
	// ErrInvalidSenderAddress is returned when the sender address is invalid
	ErrInvalidSenderAddress = errors.New("sender_address is invalid")
	// ErrInvalidIntent is returned when the intent is invalid
	ErrInvalidIntent = errors.New("intent is required")
	// ErrRequiredIntentID is returned when the intent ID is invalid
	ErrRequiredIntentID = errors.New("intent_id is required")
)

type rpcIntentResponse struct {
	IntentID  string    `json:"intent_id"`
	FirstSeen time.Time `json:"first_seen,omitempty"`
}

type rpcIntentSolutionResponse struct {
	SolutionID string    `json:"solution_id"`
	FirstSeen  time.Time `json:"first_seen,omitempty"`
}

type rpcGetIntentSolutionsResponse []rpcGetIntentSolutionResult

type rpcGetIntentSolutionResult struct {
	SolutionID     string    `json:"solution_id"`
	SolverAddress  string    `json:"solver_address"`
	DappAddress    string    `json:"dapp_address"`
	IntentSolution []byte    `json:"intent_solution"`
	Timestamp      time.Time `json:"timestamp"`
}

func (h *handlerObj) handleSubmitIntent(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCSubmitIntentPayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	if err = validateSubmitIntentPayload(&params); err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	id, err := intent.GenerateIntentID(params.DappAddress, params.Intent)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}
	t := time.Now()
	intentMsg := bxmessage.NewIntent(id, params.DappAddress, params.SenderAddress, params.Hash, params.Signature, t, params.Intent)

	err = h.node.HandleMsg(intentMsg, nil, connections.RunBackground)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}

	h.intentsManager.IncIntentSubmissions()

	response := rpcIntentResponse{IntentID: id}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}
}

func (h *handlerObj) handleSubmitIntentSolution(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCSubmitIntentSolutionPayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	if err = validateSubmitIntentSolutionPayload(&params); err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	id, err := intent.GenerateSolutionID(params.IntentID, params.IntentSolution)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}
	firstSeen := time.Now()
	intentMsg := bxmessage.NewIntentSolution(id, params.SolverAddress, params.IntentID, params.Hash, params.Signature, firstSeen, params.IntentSolution)

	err = h.node.HandleMsg(intentMsg, nil, connections.RunBackground)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}

	h.intentsManager.IncSolutionSubmissions()

	response := rpcIntentSolutionResponse{SolutionID: id, FirstSeen: firstSeen}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}
}

func (h *handlerObj) handleGetIntentSolutions(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCGetIntentSolutionsPayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	err = validateGetIntentSolutionsPayload(&params)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	h.intentsManager.AddIntentOfInterest(params.IntentID)

	// broadcast the request to the connected relay nodes.
	// This will notify relay nodes that the client is interested in solutions for the intent
	err = h.node.HandleMsg(bxmessage.NewGetIntentSolutions(params.IntentID, params.DappOrSenderAddress, params.Hash, params.Signature),
		nil, connections.RunBackground)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}

	res := h.solutionsForIntent(params.IntentID, conn.DisconnectNotify())

	// send the solutions to the client even if the response is empty
	if err = conn.Reply(ctx, req.ID, res); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}
}

func (h *handlerObj) solutionsForIntent(intentID string, disconnectNotify <-chan struct{}) rpcGetIntentSolutionsResponse {
	// check if we already have solutions for the intent
	solutions := h.intentsManager.SolutionsForIntent(intentID)
	if len(solutions) == 0 {
		timer := time.NewTimer(solutionsWaitTime)
		ticker := time.NewTicker(solutionsPollInterval)
		defer timer.Stop()
		defer ticker.Stop()

	loop:
		for {
			select {
			case <-disconnectNotify:
				return nil
			case <-timer.C:
				break loop
			case <-ticker.C:
				solutions = h.intentsManager.SolutionsForIntent(intentID)
				if len(solutions) > 0 {
					break loop
				}
			}
		}
	}

	res := make([]rpcGetIntentSolutionResult, len(solutions))
	for i := range solutions {
		res[i] = rpcGetIntentSolutionResult{
			SolutionID:     solutions[i].ID,
			SolverAddress:  solutions[i].SolverAddress,
			DappAddress:    solutions[i].DappAddress,
			IntentSolution: solutions[i].Solution,
			Timestamp:      solutions[i].Timestamp,
		}
	}

	return res
}

func validateSubmitIntentPayload(payload *jsonrpc.RPCSubmitIntentPayload) error {
	if (payload.DappAddress != payload.SenderAddress) && !common.IsHexAddress(payload.DappAddress) {
		return ErrInvalidSenderAddress
	}

	if len(payload.Intent) == 0 {
		return ErrInvalidIntent
	}

	return intent.ValidateHashAndSignature(payload.SenderAddress, payload.Hash, payload.Signature, payload.Intent)
}

func validateSubmitIntentSolutionPayload(payload *jsonrpc.RPCSubmitIntentSolutionPayload) error {
	if len(payload.IntentSolution) == 0 {
		return ErrInvalidIntent
	}

	if len(payload.IntentID) == 0 {
		return ErrRequiredIntentID
	}

	return intent.ValidateHashAndSignature(payload.SolverAddress, payload.Hash, payload.Signature, payload.IntentSolution)
}

func validateGetIntentSolutionsPayload(payload *jsonrpc.RPCGetIntentSolutionsPayload) error {
	if len(payload.IntentID) == 0 {
		return ErrRequiredIntentID
	}

	data := []byte(payload.DappOrSenderAddress + payload.IntentID)

	return intent.ValidateHashAndSignature(payload.DappOrSenderAddress, payload.Hash, payload.Signature, data)
}
