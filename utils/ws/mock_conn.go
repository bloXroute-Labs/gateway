package ws

import (
	"encoding/json"
	"errors"
	"github.com/sourcegraph/jsonrpc2"
)

type mockConn struct {
	responseBytes    []byte
	currentRequestID chan string
}

func (mockWS *mockConn) ReadMessage() (messageType int, p []byte, err error) {
	requestID, ok := <-mockWS.currentRequestID
	if !ok {
		return 0, nil, errors.New("mock ws connection closed")
	}

	resp := jsonrpc2.Response{Result: (*json.RawMessage)(&mockWS.responseBytes), Error: nil, ID: jsonrpc2.ID{Str: requestID, IsString: true}}
	json, err := resp.MarshalJSON()
	if err != nil {
		return 0, nil, err
	}
	return 1, json, nil
}

func (mockWS *mockConn) WriteMessage(messageType int, data []byte) error {
	req := jsonrpc2.Request{}
	err := json.Unmarshal(data, &req)
	if err != nil {
		return err
	}

	mockWS.currentRequestID <- req.ID.Str

	return nil
}

func (mockWS *mockConn) WriteJSON(v interface{}) error {
	req, ok := v.(jsonrpc2.Request)
	if !ok {
		return errors.New("not jsonrpc2 request")
	}

	mockWS.currentRequestID <- req.ID.Str

	return nil
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
