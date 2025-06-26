package bsc

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
)

var (
	errRequestTimeout   = errors.New("request timeout")
	errPeerDisconnected = errors.New("peer disconnected")
	errUnknownRequestID = errors.New("unknown request ID on message")
)

// request represents a message request sent to a peer
type request struct {
	code      uint64
	want      uint64
	requestID uint64
	data      interface{}
	resCh     chan interface{}
	cancelCh  chan string
	timeout   time.Duration
}

// response represents a message response received from a peer
type response struct {
	code      uint64
	requestID uint64
	data      interface{}
}

// dispatcher handles message requests and responses
type dispatcher struct {
	peer     *Peer
	requests map[uint64]*request
	mu       sync.Mutex
}

// newDispatcher creates a new message dispatcher
func newDispatcher(peer *Peer) *dispatcher {
	return &dispatcher{
		peer:     peer,
		requests: make(map[uint64]*request),
	}
}

// dispatchRequest send the request, and block until the later response
func (d *dispatcher) dispatchRequest(req *request) (interface{}, error) {
	// record the request before sending
	req.resCh = make(chan interface{}, 1)
	req.cancelCh = make(chan string, 1)
	d.mu.Lock()
	d.requests[req.requestID] = req
	d.mu.Unlock()

	d.peer.log.Debugf("sending %T request", req.data)

	err := p2p.Send(d.peer.rw, req.code, req.data)
	if err != nil {
		return nil, err
	}

	// clean the requests when the request is done
	defer func() {
		d.mu.Lock()
		delete(d.requests, req.requestID)
		d.mu.Unlock()
	}()

	timeout := time.NewTimer(req.timeout)
	select {
	case res := <-req.resCh:
		return res, nil
	case <-timeout.C:
		req.cancelCh <- "timeout"
		return nil, errRequestTimeout
	case <-d.peer.ctx.Done():
		return nil, errPeerDisconnected
	}
}

// getRequestByResp get the request by the response, and delete the request if it is matched
func (d *dispatcher) getRequestByResp(res *response) (*request, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	req := d.requests[res.requestID]
	if req == nil {
		return nil, errUnknownRequestID
	}

	if req.want != res.code {
		return nil, fmt.Errorf("response mismatch: %d != %d", res.code, req.want)
	}

	delete(d.requests, req.requestID)
	return req, nil
}

// dispatchResponse dispatch the response to the request
func (d *dispatcher) dispatchResponse(res *response) error {
	req, err := d.getRequestByResp(res)
	if err != nil {
		return err
	}

	select {
	case req.resCh <- res.data:
		return nil
	case reason := <-req.cancelCh:
		return fmt.Errorf("request cancelled: %d, reason: %s", res.requestID, reason)
	case <-d.peer.ctx.Done():
		return errPeerDisconnected
	}
}

// genRequestID get requestID for packet
func genRequestID() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) //nolint:errcheck

	return binary.LittleEndian.Uint64(b[:])
}
