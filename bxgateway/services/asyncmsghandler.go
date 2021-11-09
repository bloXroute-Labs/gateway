package services

import (
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/bxmessage"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/connections"
	log "github.com/sirupsen/logrus"
)

// MsgInfo is a struct that stores a msg and its source connection
type MsgInfo struct {
	Msg    bxmessage.Message
	Source connections.Conn
}

// AsyncMsgHandler is a struct that handles messages asynchronously
type AsyncMsgHandler struct {
	AsyncMsgChannel chan MsgInfo
	listener        connections.BxListener
}

// NewAsyncMsgChannel returns a new instance of AsyncMsgHandler
func NewAsyncMsgChannel(listener connections.BxListener) chan MsgInfo {
	handler := &AsyncMsgHandler{
		AsyncMsgChannel: make(chan MsgInfo, bxgateway.AsyncMsgChannelSize),
		listener:        listener,
	}
	go handler.HandleMsgAsync()
	return handler.AsyncMsgChannel
}

// HandleMsgAsync handles messages pushed onto the channel of AsyncMsgHandler
func (amh AsyncMsgHandler) HandleMsgAsync() {
	for {
		messageInfo, ok := <-amh.AsyncMsgChannel
		if !ok {
			log.Error("unexpected termination of AsyncMsgHandler. AsyncMsgChannel was closed.")
			return
		}
		log.Tracef("async handling of %v from %v", messageInfo.Msg, messageInfo.Source.ID().RemoteAddr())
		_ = amh.listener.HandleMsg(messageInfo.Msg, messageInfo.Source, connections.RunForeground)
	}
}
