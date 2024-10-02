package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	"github.com/bloXroute-Labs/gateway/v2/utils/intent"
	"github.com/ethereum/go-ethereum/common"
	"github.com/sourcegraph/jsonrpc2"
)

// ErrInvalidQuote is returned when the quote is invalid
var ErrInvalidQuote = errors.New("quote is required")

type rpcQuoteResponse struct {
	QuoteID   string    `json:"quote_id"`
	FirstSeen time.Time `json:"first_seen,omitempty"`
}

func (h *handlerObj) handleSubmitQuote(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Params == nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, errParamsValueIsMissing, conn, req.ID)
		return
	}

	var params jsonrpc.RPCSubmitQuotePayload
	err := json.Unmarshal(*req.Params, &params)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, fmt.Sprintf("failed to unmarshal params for %v request: %v",
			jsonrpc.RPCTx, err), conn, req.ID)
		return
	}

	if err = validateSubmitQuotePayload(&params); err != nil {
		sendErrorMsg(ctx, jsonrpc.InvalidParams, err.Error(), conn, req.ID)
		return
	}

	id, err := intent.GenerateQuoteID(params.DappAddress, params.Quote)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}
	quoteMsg := bxmessage.NewQuote(id, params.DappAddress, params.SolverAddress, params.Hash, params.Signature, params.Quote, time.Now())

	err = h.node.HandleMsg(quoteMsg, nil, connections.RunBackground)
	if err != nil {
		sendErrorMsg(ctx, jsonrpc.InternalError, err.Error(), conn, req.ID)
		return
	}

	h.intentsManager.IncQuoteSubmissions()

	response := rpcQuoteResponse{QuoteID: id}

	if err = conn.Reply(ctx, req.ID, response); err != nil {
		h.log.Errorf("error replying to %v, method %v: %v", h.remoteAddress, req.Method, err)
		return
	}
}

func validateSubmitQuotePayload(payload *jsonrpc.RPCSubmitQuotePayload) error {
	if !common.IsHexAddress(payload.DappAddress) {
		return ErrInvalidSenderAddress
	}
	if len(payload.Quote) == 0 {
		return ErrInvalidQuote
	}

	return intent.ValidateHashAndSignature(payload.SolverAddress, payload.Hash, payload.Signature, payload.Quote)
}
