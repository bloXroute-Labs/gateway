package services

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
)

// MessageQueue - queue of messages to process
type MessageQueue struct {
	queue    chan msgWithSource
	txsCount uint64
	ctx      context.Context
	cancel   context.CancelFunc
}

type msgWithSource struct {
	msg             bxmessage.Message
	source          connections.Conn
	waitStartTime   time.Time
	channelPosition int
}

// MessageQueueCallback - callback function for MessageQueue
type MessageQueueCallback func(msg bxmessage.Message, source connections.Conn, waitingDuration time.Duration, workerChannelPosition int)

// NewMsgQueue - create MessageQueue object, running workers and return MessageQueue
func NewMsgQueue(numOfWorkers int, queueSize int, callBack MessageQueueCallback) *MessageQueue {
	ctx, cancel := context.WithCancel(context.Background())
	msgQueue := &MessageQueue{
		queue:  make(chan msgWithSource, queueSize),
		ctx:    ctx,
		cancel: cancel,
	}
	for i := 0; i < numOfWorkers; i++ {
		go msgQueue.work(callBack)
	}

	return msgQueue
}

func (tq *MessageQueue) work(callBack MessageQueueCallback) {
	for {
		select {
		case tx := <-tq.queue:
			waitDuration := time.Since(tx.waitStartTime)
			callBack(tx.msg, tx.source, waitDuration, tx.channelPosition)
		case <-tq.ctx.Done():
			return
		}
	}
}

// Insert - insert msg to channel
func (tq *MessageQueue) Insert(msg bxmessage.Message, source connections.Conn) error {
	select {
	case <-tq.ctx.Done():
		return nil
	default:
		select {
		case tq.queue <- msgWithSource{source: source, msg: msg, waitStartTime: time.Now(), channelPosition: len(tq.queue)}:
			atomic.AddUint64(&tq.txsCount, 1)
		default:
			return errors.New("channel is full")
		}
	}

	return nil
}

// Len - return len of queue
func (tq *MessageQueue) Len() int {
	return len(tq.queue)
}

// TxsCount - return tx count that was in queue
func (tq *MessageQueue) TxsCount() uint64 {
	return atomic.LoadUint64(&tq.txsCount)
}

// Stop - stop queue
func (tq *MessageQueue) Stop() {
	tq.cancel()
}
