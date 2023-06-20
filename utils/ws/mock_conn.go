package ws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/sourcegraph/jsonrpc2"
)

type mockConn struct {
	responseBytes    []byte
	currentRequestID chan string
}

func (mockWS *mockConn) ReadMessage(ctx context.Context) ([]byte, error) {
	requestID, ok := <-mockWS.currentRequestID
	if !ok {
		return nil, errors.New("mock ws connection closed")
	}

	resp := jsonrpc2.Response{Result: (*json.RawMessage)(&mockWS.responseBytes), Error: nil, ID: jsonrpc2.ID{Str: requestID, IsString: true}}
	return resp.MarshalJSON()
}

func (mockWS *mockConn) WriteMessage(ctx context.Context, data []byte) error {
	req := jsonrpc2.Request{}
	err := json.Unmarshal(data, &req)
	if err != nil {
		return err
	}

	mockWS.currentRequestID <- req.ID.Str

	return nil
}

func (mockWS *mockConn) WriteJSON(ctx context.Context, v interface{}) error {
	req, ok := v.(jsonrpc2.Request)
	if !ok {
		return errors.New("not jsonrpc2 request")
	}

	mockWS.currentRequestID <- req.ID.Str

	return nil
}

func (mockWS *mockConn) ReadJSON(ctx context.Context, v interface{}) error {
	requestID, ok := <-mockWS.currentRequestID
	if !ok {
		return errors.New("mock ws connection closed")
	}

	resp := jsonrpc2.Response{Result: (*json.RawMessage)(&mockWS.responseBytes), Error: nil, ID: jsonrpc2.ID{Str: requestID, IsString: true}}
	data, err := resp.MarshalJSON()
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &v)
}

func (mockWS *mockConn) Close() error {
	close(mockWS.currentRequestID)

	return nil
}

func (mockWS *mockConn) GetRemoteAddr() string {
	return "127.0.0.1"
}

// NewMockConn construct a connection which returns response bytes back
func NewMockConn(responseBytes []byte) Conn {
	return &mockConn{responseBytes: responseBytes, currentRequestID: make(chan string, 1)}
}
