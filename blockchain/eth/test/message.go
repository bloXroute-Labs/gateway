package test

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"time"
)

// MsgReadWriter is a test implementation of the RW connection interface on RLP peers
type MsgReadWriter struct {
	ReadMessages  chan p2p.Msg
	WriteMessages []p2p.Msg

	// provide an alerting mechanism if write doesn't happen on main goroutine (writeChannelSize = -1 if this is not needed)
	writeAlertCh   chan bool
	writeToChannel bool
}

// NewMsgReadWriter returns a test read writer with the provided channel buffer size
func NewMsgReadWriter(readChannelSize, writeChannelSize int) *MsgReadWriter {
	rw := &MsgReadWriter{
		ReadMessages:   make(chan p2p.Msg, readChannelSize),
		WriteMessages:  make([]p2p.Msg, 0),
		writeToChannel: writeChannelSize != -1,
	}
	if rw.writeToChannel {
		rw.writeAlertCh = make(chan bool, writeChannelSize)
	}
	return rw
}

// ReadMsg pulls an encoded message off the read queue
func (t *MsgReadWriter) ReadMsg() (p2p.Msg, error) {
	msg := <-t.ReadMessages
	return msg, nil
}

// WriteMsg tracks all messages that are supposedly written to the RW peer
func (t *MsgReadWriter) WriteMsg(msg p2p.Msg) error {
	t.WriteMessages = append(t.WriteMessages, msg)
	if t.writeToChannel {
		t.writeAlertCh <- true
	}
	return nil
}

// QueueIncomingMessage simulates the peer sending a message to be read
func (t *MsgReadWriter) QueueIncomingMessage(code uint64, payload interface{}) {
	size, reader, err := rlp.EncodeToReader(payload)
	if err != nil {
		panic(err)
	}
	msg := p2p.Msg{
		Code:       code,
		Size:       uint32(size),
		Payload:    reader,
		ReceivedAt: time.Time{},
	}
	t.ReadMessages <- msg
}

// QueueIncomingMessageWithDelay simulates the peer sending a message to be read with delay
func (t *MsgReadWriter) QueueIncomingMessageWithDelay(code uint64, payload any, delay time.Duration) {
	time.Sleep(delay)
	size, reader, err := rlp.EncodeToReader(payload)
	if err != nil {
		panic(err)
	}
	msg := p2p.Msg{
		Code:       code,
		Size:       uint32(size),
		Payload:    reader,
		ReceivedAt: time.Time{},
	}
	t.ReadMessages <- msg
}

// ExpectWrite waits for up to the provided duration for writes to the channel. This method is useful if the message writing happens on a goroutine.
func (t *MsgReadWriter) ExpectWrite(d time.Duration) bool {
	if !t.writeToChannel {
		panic("cannot expect writes when write channel is not being used")
	}
	select {
	case <-t.writeAlertCh:
		return true
	case <-time.After(d):
		return false
	}
}

// PopWrittenMessage pops the first sent message from off the mesage queue and returns it
func (t *MsgReadWriter) PopWrittenMessage() p2p.Msg {
	msg := t.WriteMessages[0]
	t.WriteMessages = t.WriteMessages[1:]
	return msg
}
