package ws

import (
	"sync"

	"github.com/fasthttp/websocket"
)

// Conn provides interface for websocket connection
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	WriteJSON(v interface{}) error
	GetRemoteAddr() string
	Close() error
}

type realWSConn struct {
	readLock  sync.Mutex
	writeLock sync.Mutex
	closed    bool

	realConn *websocket.Conn
}

func (c *realWSConn) ReadMessage() (messageType int, p []byte, err error) {
	c.readLock.Lock()
	defer c.readLock.Unlock()

	return c.realConn.ReadMessage()
}

func (c *realWSConn) WriteMessage(messageType int, data []byte) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	return c.realConn.WriteMessage(messageType, data)
}

func (c *realWSConn) WriteJSON(v interface{}) error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	return c.realConn.WriteJSON(v)
}

func (c *realWSConn) Close() error {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	c.closed = true

	return c.realConn.Close()
}

func (c *realWSConn) GetRemoteAddr() string {
	return c.realConn.RemoteAddr().String()
}

// NewConcurrentConn construct websocket connection which is safe to use in goroutines
func NewConcurrentConn(conn *websocket.Conn) Conn {
	return &realWSConn{realConn: conn}
}
