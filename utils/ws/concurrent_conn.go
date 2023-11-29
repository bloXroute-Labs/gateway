package ws

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
)

const (
	messageSizeLimit = 15 * 1024 * 1024
	msgCHanSize      = 1000
	writeTimeout     = 30 * time.Second
)

// DialOptions represents Dial's options.
type DialOptions struct {
	HandshakeTimeout time.Duration
	TLSClientConfig  *tls.Config
	HTTPHeader       http.Header
}

// ErrAlreadyClosed is returned when trying to read/write from/to closed connection
var ErrAlreadyClosed = &websocket.CloseError{
	Code: websocket.CloseAbnormalClosure,
	Text: "websocket connection is closed",
}

// Conn provides interface for websocket connection
type Conn interface {
	ReadMessage(context.Context) ([]byte, error)
	ReadJSON(ctx context.Context, v interface{}) error
	WriteMessage(context.Context, []byte) error
	WriteJSON(ctx context.Context, v interface{}) error
	GetRemoteAddr() string
	Close() error
}

type realWSConn struct {
	mu            sync.RWMutex
	remoteAddress string
	closed        chan struct{}
	msgsRaw       chan msgRawResponse
	msgsJSON      chan msgJSONResponse

	realConn *websocket.Conn
}

type msgRawResponse struct {
	msgsRaw []byte
	ctx     context.Context
}

type msgJSONResponse struct {
	msgsJSON interface{}
	ctx      context.Context
}

// Upgrade construct websocket connection which is safe to use in goroutines
func Upgrade(w http.ResponseWriter, req *http.Request) (Conn, error) {
	u := websocket.Upgrader{}
	conn, err := u.Upgrade(w, req, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to accept websocket connection: %w", err)
	}

	conn.SetReadLimit(messageSizeLimit)

	c := &realWSConn{
		realConn:      conn,
		remoteAddress: req.RemoteAddr,
		closed:        make(chan struct{}),
		msgsRaw:       make(chan msgRawResponse, msgCHanSize),
		msgsJSON:      make(chan msgJSONResponse, msgCHanSize),
	}

	go c.write()

	return c, nil
}

// Dial dials websocket connection which is safe to use in goroutines
func Dial(ctx context.Context, url string, opts *DialOptions) (Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: opts.HandshakeTimeout,
		TLSClientConfig:  opts.TLSClientConfig,
	}

	conn, _, err := dialer.DialContext(ctx, url, opts.HTTPHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to dial websocket connection: %w", err)
	}

	conn.SetReadLimit(messageSizeLimit)

	c := &realWSConn{
		realConn:      conn,
		remoteAddress: url,
		closed:        make(chan struct{}),
		msgsRaw:       make(chan msgRawResponse, msgCHanSize),
		msgsJSON:      make(chan msgJSONResponse, msgCHanSize),
	}

	go c.write()

	return c, nil
}

// ReadJSON reads JSON message from websocket connection
func (c *realWSConn) ReadJSON(ctx context.Context, v interface{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return ErrAlreadyClosed
	default:
	}

	err := c.ReadJSONHelper(&v)
	if err != nil && IsWSClosedError(err) {
		return c.Close()
	}

	return err
}

func (c *realWSConn) ReadJSONHelper(v interface{}) error {
	_, r, err := c.realConn.NextReader()
	if err != nil {
		return err
	}

	processingStart := time.Now()
	err = json.NewDecoder(r).Decode(v)
	if err == io.EOF {
		// One value is expected in the message.
		err = io.ErrUnexpectedEOF
	}

	log.Tracef("ws request preprocessing payload decoding duration is %v ms", time.Since(processingStart).Milliseconds())
	return err
}

// ReadMessage reads text message from websocket connection
func (c *realWSConn) ReadMessage(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, ErrAlreadyClosed
	default:
	}

	_, data, err := c.realConn.ReadMessage()
	if err != nil && IsWSClosedError(err) {
		return nil, c.Close()
	}

	return data, err
}

// WriteJSON writes JSON message to websocket connection
func (c *realWSConn) WriteJSON(ctx context.Context, v interface{}) error {
	select {
	case <-c.closed:
		return ErrAlreadyClosed
	case <-ctx.Done():
		return nil
	case c.msgsJSON <- msgJSONResponse{msgsJSON: v, ctx: ctx}:
	default:
		// in case the channel is full, we close the connection
		return c.Close()
	}

	return nil
}

// WriteMessage writes text message to websocket connection
func (c *realWSConn) WriteMessage(ctx context.Context, data []byte) error {
	select {
	case <-c.closed:
		return ErrAlreadyClosed
	case <-ctx.Done():
		return nil
	case c.msgsRaw <- msgRawResponse{msgsRaw: data, ctx: ctx}:
	default:
		// in case the channel is full, we close the connection
		return c.Close()
	}

	return nil
}

func (c *realWSConn) write() {
	defer func() {
		err := c.Close()
		if err != nil {
			log.Error(err)
		}
	}()

	for {
		select {
		case <-c.closed:
			return
		case msg := <-c.msgsRaw:
			select {
			case <-msg.ctx.Done():
				return
			default:
				err := c.writeTimeoutRaw(msg.ctx, msg.msgsRaw)
				if err != nil && IsWSClosedError(err) {
					return
				}
			}
		case msg := <-c.msgsJSON:
			select {
			case <-msg.ctx.Done():
				return
			default:
				err := c.writeTimeoutJSON(msg.ctx, msg.msgsJSON)
				if err != nil && IsWSClosedError(err) {
					return
				}
			}
		}
	}
}

// Close closes websocket connection
// It is safe to call Close multiple times
func (c *realWSConn) Close() error {
	// we saw we have raise condition
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
	}

	err := c.realConn.Close()
	if err != nil {
		if IsWSClosedError(err) {
			return nil
		}
		return fmt.Errorf("failed to close websocket connection: %w", err)
	}

	return nil
}

// GetRemoteAddr returns remote address of websocket connection
func (c *realWSConn) GetRemoteAddr() string {
	return c.remoteAddress
}

// IsWSClosedError checks if error is websocket close error
func IsWSClosedError(err error) bool {
	if err == nil {
		return false
	}

	var websocketCloseErr *websocket.CloseError
	if errors.As(err, &websocketCloseErr) ||
		errors.Is(err, ErrAlreadyClosed) ||
		errors.Is(err, io.EOF) ||
		strings.Contains(err.Error(), "already wrote close") ||
		strings.Contains(err.Error(), "EOF") ||
		strings.Contains(err.Error(), "broken pipe") ||
		strings.Contains(err.Error(), "connection reset by peer") {
		return true
	}

	return false
}

// writeTimeoutRaw writes raw message to websocket connection with timeout
// should not be used for concurrent writes
func (c *realWSConn) writeTimeoutRaw(_ context.Context, msg []byte) error {
	return c.realConn.WriteMessage(websocket.TextMessage, msg)
}

// writeTimeoutJSON writes JSON message to websocket connection with timeout
// should not be used for concurrent writes
func (c *realWSConn) writeTimeoutJSON(_ context.Context, msg interface{}) error {
	return c.realConn.WriteJSON(msg)
}
