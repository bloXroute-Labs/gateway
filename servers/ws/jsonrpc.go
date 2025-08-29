package ws

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
)

// ErrClosed indicates that the WebSocket connection is closed.
var ErrClosed = errors.New("ws: connection is closed")

type conn struct {
	wsConn     *websocket.Conn
	closed     bool
	disconnect chan struct{}

	lock        sync.RWMutex
	sendingLock sync.Mutex
}

// Handler interface defines the methods that a handler must implement to process JSON-RPC requests.
type Handler interface {
	Handle(ctx context.Context, c *conn, req Request)
}

func newConn(ctx context.Context, wsConn *websocket.Conn, handler Handler) *conn {
	c := &conn{
		wsConn:     wsConn,
		disconnect: make(chan struct{}),
	}

	go c.handleRoutine(ctx, handler)

	return c
}

func (c *conn) handleRoutine(ctx context.Context, handler Handler) {
	for {
		msgType, r, err := c.wsConn.NextReader()
		if err != nil {
			if _, ok := err.(*websocket.CloseError); !ok {
				log.Errorf("ws: error reading from connection: %v", err)
			}
			c.Close()
			return

		}
		if msgType != websocket.TextMessage {
			log.Errorf("ws: unexpected message type, expected TextMessage, actual: %v", msgType)
			c.Close()
			return
		}

		var req Request
		if err := jsoniter.ConfigCompatibleWithStandardLibrary.NewDecoder(r).Decode(&req); err != nil {
			log.Errorf("ws: error decoding request: %v", err)
			c.Close()
			return
		}

		go handler.Handle(ctx, c, req)
	}
}

// ReplyWithError sends a JSON-RPC response with an error.
func (c *conn) ReplyWithError(ctx context.Context, id ID, respErr Error) error {
	return c.send(ctx, Response{
		ID:    id,
		Error: respErr,
	})
}

// Reply sends a JSON-RPC response with a result.
func (c *conn) Reply(ctx context.Context, id ID, result any) error {
	return c.send(ctx, Response{
		ID:     id,
		Result: result,
	})
}

// Notify sends a JSON-RPC notification without expecting a response.
func (c *conn) Notify(ctx context.Context, method string, params any) error {
	return c.send(ctx, Notification{
		Method: method,
		Params: params,
	})
}

// Close closes the WebSocket connection and notifies any listeners that the connection is closed.
func (c *conn) Close() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.closed {
		return ErrClosed
	}

	c.closed = true
	close(c.disconnect)
	return c.wsConn.Close()
}

// DisconnectNotify returns a channel that is closed when the connection is closed.
func (c *conn) DisconnectNotify() <-chan struct{} {
	return c.disconnect
}

func (c *conn) send(_ context.Context, msg any) error {
	c.sendingLock.Lock()
	defer c.sendingLock.Unlock()

	c.lock.RLock()
	if c.closed {
		c.lock.RUnlock()
		return ErrClosed
	}
	c.lock.RUnlock()

	wr, err := c.wsConn.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}
	defer wr.Close()

	st := jsoniter.ConfigCompatibleWithStandardLibrary.BorrowStream(wr)
	defer jsoniter.ConfigCompatibleWithStandardLibrary.ReturnStream(st)

	st.WriteVal(msg)
	if _, err := wr.Write(st.Buffer()); err != nil {
		c.Close()
		return err
	}

	return nil
}

// ID represents a JSON-RPC request or response identifier.
type ID struct {
	Num      uint64
	Str      string
	IsString bool
}

// MarshalJSON implements json.Marshaler.
func (id ID) MarshalJSON() ([]byte, error) {
	if id.IsString {
		return jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(id.Str)
	}
	return jsoniter.ConfigCompatibleWithStandardLibrary.Marshal(id.Num)
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *ID) UnmarshalJSON(data []byte) error {
	// Support both uint64 and string IDs.
	var num uint64
	if err := jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(data, &num); err == nil {
		*id = ID{Num: num, Str: strconv.FormatUint(num, 10)}
		return nil
	}
	var str string
	if err := jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(data, &str); err != nil {
		return err
	}
	*id = ID{Str: str, IsString: true}
	return nil
}

// String returns the string representation of the ID.
func (id ID) String() string {
	return id.Str
}

// IsZero checks if the ID is zero-valued.
func (id ID) IsZero() bool {
	return id.Num == 0 && id.Str == "" && !id.IsString
}

// Notification represents a JSON-RPC notification message.
type Notification struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// Request represents a JSON-RPC request message.
type Request struct {
	ID     ID               `json:"id,omitempty,omitzero"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC response message.
type Response struct {
	ID     ID    `json:"id,omitempty,omitzero"`
	Result any   `json:"result,omitempty"`
	Error  Error `json:"error,omitempty,omitzero"`
}

// Error represents a JSON-RPC error message.
type Error struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// IsZero checks if the Error is zero-valued.
func (e Error) IsZero() bool {
	return e.Code == 0 && e.Message == "" && e.Data == nil
}
