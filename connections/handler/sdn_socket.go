package handler

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/cert"
	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/sdnp2p"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	sdnSocketConnectionTimeout = 10 * time.Second
	sdnPingInterval            = 15 * time.Second
	sdnReadDeadline            = 45 * time.Second
	sdnSubscribeRequestTimeout = 500 * time.Millisecond
)

// SDNSocketID is a placeholder NodeID represents the SDN connection.
const SDNSocketID bxtypes.NodeID = "sdnsocket"

// SDNSocket represents a socket connection to the SDN socket broker.
type SDNSocket struct {
	connections.Conn
	node                connections.BxListener
	subscriptionManager services.SubscriptionManager
	status              chan bool // for now, this emits true when the connection is established, and false when failed
	quitPing            chan bool
	messages            chan bxmessage.MessageBytes
	quitHandle          chan bool
	nodeModel           *sdnmessage.NodeModel
	proxyCerts          *cert.SSLCerts
	clock               clock.Clock
}

// NewSDNSocket returns a newly constructed connection the SDN socket broker.
func NewSDNSocket(node connections.BxListener, proxyCerts *cert.SSLCerts, sdnIP string, sdnPort int64, nodeModel *sdnmessage.NodeModel, clock clock.Clock) *SDNSocket {
	sdnsocket := &SDNSocket{
		Conn: connections.NewSSLConnection(
			func() (connections.Socket, error) {
				return connections.NewTLS(sdnIP, int(sdnPort), proxyCerts)
			},
			proxyCerts, sdnIP, sdnPort, bxmessage.EmptyProtocol, false, bxgateway.MaxConnectionBacklog, clock),
		node:                node,
		subscriptionManager: services.NewSubscriptionManager(clock),
		status:              make(chan bool, 10),
		quitPing:            make(chan bool),
		messages:            make(chan bxmessage.MessageBytes),
		quitHandle:          make(chan bool),
		nodeModel:           nodeModel,
		proxyCerts:          proxyCerts,
		clock:               clock,
	}
	return sdnsocket
}

// Start the SDNSocket event loop, blocking until the connection is ready.
func (sdn *SDNSocket) Start() error {
	if sdn.proxyCerts.NeedsPrivateCert() {
		return errors.New("cannot start SDN Socket without private proxy certificate")
	}
	go sdn.loop()
	return nil
}

// String provides a string representation of this connection
func (sdn *SDNSocket) String() string {
	return fmt.Sprintf("SDN %v", sdn.Conn)
}

func (sdn *SDNSocket) loop() {
	for {
		log.Trace("connecting to SDN....")

		err := sdn.Connect()
		if err != nil {
			log.Warnf("could not establish a socket connection to the SDN: %v, retrying in %v seconds...", err, sdnSocketConnectionTimeout/time.Second)
			time.Sleep(sdnSocketConnectionTimeout)
			continue
		}
		_ = sdn.node.OnConnEstablished(sdn) //nolint:errcheck

		// connection has been established
		go sdn.pingLoop()
		go sdn.handleMessages()

		sdn.initializeConn()

		for sdn.Conn.IsOpen() {
			n, err := sdn.ReadMessages(sdn.ProcessMessage, sdnReadDeadline, sdnp2p.SdnHeaderLen,
				func(b []byte) int {
					return int(binary.LittleEndian.Uint32(b[sdnp2p.SdnPayloadSizeOffset:]))
				})
			if err != nil {
				log.Errorf("connection to the SDN was broken because: %v, retrying.", err)
				_ = sdn.node.OnConnClosed(sdn) //nolint:errcheck
				break
			}
			log.Tracef("read %v bytes from SDN", n)
		}
		sdn.closeCurrent()
		sdn.clock.Sleep(sdnSocketConnectionTimeout)
	}
}

func (sdn *SDNSocket) pingLoop() {
	interval := sdn.clock.Ticker(sdnPingInterval)
	defer interval.Stop()

	for {
		select {
		case <-sdn.quitPing:
			sdn.quitPing <- true
			interval.Stop()
			return
		case <-interval.Alert():
			pm := sdnp2p.NewSdnPing()
			sdn.send(&pm, sdnp2p.SDNPingType)

			if sdn.node.NodeStatus().TransactionServiceSynced {
				sdn.SendTransactionSyncComplete()
			}
		}
	}
}

func (sdn *SDNSocket) handleMessages() {
	for {
		select {
		case <-sdn.quitHandle:
			sdn.quitHandle <- true
			return
		case msg := <-sdn.messages:
			// msgType := sdnp2p.SDNMessageType(msg)
			// log.Tracef("read full message from SDN: type %v len %v", msgType, len(msg))
			sdn.ProcessMessage(msg)
		}
	}
}

func (sdn *SDNSocket) initializeConn() {
	// request private certificate for relay cert to present to gateways
	if sdn.proxyCerts.NeedsPrivateCert() {
		log.Debug("requesting private relay cert from SDN socket")
		err := sdn.registerNode(sdn.nodeModel, sdn.proxyCerts)
		if err != nil {
			panic(err)
		}
	}

	// request routing config
	sdn.sendSDNEvent(sdnp2p.SseRoutingConfigUpdate, nil)

	// request peer proxies (disabled for now: using static peer proxy peer file)
	// sdn.sendSDNEvent(sdnp2p.SseGetOutboundPeers, nil)

	// request mev builders for mev miners
	sdn.sendSDNEvent(sdnp2p.SseMEVBuilderUpdate, nil)
}

func (sdn *SDNSocket) registerNode(nodeModel *sdnmessage.NodeModel, certs *cert.SSLCerts) error {
	log.Debug("requesting private relay cert from SDN socket")

	roCert, err := certs.SerializeRegistrationCert()
	if err != nil {
		return fmt.Errorf("could not serialize registration only cert for relay cert on SDN socket connection: %v", err)
	}
	nodeModel.Cert = string(roCert)
	csr, err := certs.CreateCSR()
	if err != nil {
		return fmt.Errorf("could not create CSR for relay cert on SDN socket connection: %v", err)
	}
	nodeModel.Csr = string(csr)
	sdn.sendSDNEvent(sdnp2p.SseAddNode, nodeModel)
	return nil
}

// ProcessMessage handles all received messages from the SDN socket, delegating to the node object if necessary.
func (sdn *SDNSocket) ProcessMessage(msgBytes bxmessage.MessageBytes) {
	msgType := sdnp2p.SDNMessageType(msgBytes)
	msg := msgBytes.Raw()
	log.Tracef("read full message from SDN: type %v len %v", msgType, len(msg))
	switch msgType {
	case sdnp2p.SDNType:
		var sdnEvent sdnp2p.SDNEvent
		err := sdnEvent.Unpack(msg, bxmessage.EmptyProtocol)
		if err != nil {
			log.Warnf("could not unpack message from sdn: %v", err)
			return
		}

		log.Tracef("handling message from bxapi: %v", sdnEvent.SocketEvent)

		switch sdnEvent.SocketEvent {
		case sdnp2p.SseAddNode:
			nm := sdnEvent.Payload.(*sdnmessage.NodeModel)

			err := sdn.proxyCerts.SavePrivateCert(nm.Cert)
			// should pretty much never happen unless there are SDN problems, in which
			// case just abort on startup
			if err != nil {
				debug.PrintStack()
				panic(err)
			}
		case sdnp2p.SseSubscriptionPermission:
			permissionMsg := sdnEvent.Payload.(*types.SubscriptionPermissionMessage)
			sdn.subscriptionManager.ForwardPermissionResponse(permissionMsg)
		default:
			//nolint:errcheck
			_ = sdn.node.HandleMsg(&sdnEvent, sdn, connections.RunForeground)
		}
	case sdnp2p.SDNPingType:
		pm := sdnp2p.NewSdnPong()
		sdn.send(&pm, sdnp2p.SDNPongType)
	case sdnp2p.SDNPongType:
	default:
	}
}

// Type returns the connection peer type
func (sdn *SDNSocket) Type() bxtypes.NodeType {
	return bxtypes.APISocket
}

// GetNodeID return node ID
func (sdn *SDNSocket) GetNodeID() bxtypes.NodeID { return SDNSocketID }

// GetConnectionType returns type of the connection
func (sdn *SDNSocket) GetConnectionType() bxtypes.NodeType { return bxtypes.APISocket }

// GetConnectionState returns state of the connection
func (sdn *SDNSocket) GetConnectionState() string { return "todo" }

// shutdowns goroutines for current connection; allows retry
func (sdn *SDNSocket) closeCurrent() {
	sdn.quitPing <- true
	<-sdn.quitPing

	sdn.quitHandle <- true
	<-sdn.quitHandle
}

func (sdn *SDNSocket) send(msg bxmessage.Message, msgType string) {
	err := sdn.Send(msg)
	if err != nil {
		log.Warnf("could not send %v on SDN socket: %v", msgType, err)
	} else {
		log.Tracef("sent a %v message on SDN socket", msgType)
	}
}

func (sdn *SDNSocket) sendSDNEvent(socketEvent sdnp2p.SDNSocketEvent, payload interface{}) {
	msg := sdnp2p.NewSDNEvent(sdn.nodeModel.NodeID, socketEvent, payload)
	log.Tracef("sending a %v SDN event on socket", socketEvent)
	sdn.send(&msg, sdnp2p.SDNType)
}

// SendPeerConnectionEvent submits a node event indicating that a node has connected
// to the relay proxy.
func (sdn *SDNSocket) SendPeerConnectionEvent(peerID bxtypes.NodeID, networkNum bxtypes.NetworkNum) {
	nodeEvent := sdnmessage.NewNodeConnectionEvent(peerID, networkNum)
	sdn.sendSDNEvent(sdnp2p.SseRelayInboundConn, nodeEvent)
	log.Debugf("sending peer connection event to SDN: %v", peerID)
}

// SendPeerDisconnectionEvent submits a node event indicating that a node has
// disconnected from the relay proxy.
func (sdn *SDNSocket) SendPeerDisconnectionEvent(peerID bxtypes.NodeID) {
	nodeEvent := sdnmessage.NewNodeDisconnectionEvent(peerID)
	sdn.sendSDNEvent(sdnp2p.SseRelayInboundConn, nodeEvent)
	log.Debugf("sending peer disconnection event to SDN: %v", peerID)
}

// SendPeerDisabledEvent submits a node event indicating that a peer connection has been disabled.
func (sdn *SDNSocket) SendPeerDisabledEvent(peerID bxtypes.NodeID, reason string) {
	nodeEvent := sdnmessage.NewNodeDisabledEvent(peerID, reason)
	sdn.sendSDNEvent(sdnp2p.SseRelayInboundConn, nodeEvent)
	log.Debugf("sending peer disabled event wtih reason [] to SDN: %v [%v]", peerID, reason)
}

// SendTransactionSyncComplete indicates to the SDN that this node has completed transaction sync.
func (sdn *SDNSocket) SendTransactionSyncComplete() {
	sdn.sendSDNEvent(sdnp2p.SseTxSyncComplete, nil)
}

// SendConnectedPeers sends the SDN the requested connection pool
func (sdn *SDNSocket) SendConnectedPeers(connectedPeers sdnmessage.ConnectedPeers) {
	sdn.sendSDNEvent(sdnp2p.SseGetConnectedPeers, connectedPeers)
}

// SendTransactionSyncCheck sends the SDN the nodes sync status
func (sdn *SDNSocket) SendTransactionSyncCheck(requestID string) {
	syncCheck := sdnmessage.TransactionSyncCheck{
		RequestID: requestID,
		IsSynced:  sdn.node.NodeStatus().TransactionServiceSynced,
	}
	sdn.sendSDNEvent(sdnp2p.SseTransactionSyncCheck, syncCheck)
}

// SendAuditCountersUpdate sends the SDN an audit counters update
func (sdn *SDNSocket) SendAuditCountersUpdate(auditCountersUpdate *sdnmessage.AuditCountersUpdate) {
	sdn.sendSDNEvent(sdnp2p.SseAuditCountersRelayUpdate, *auditCountersUpdate)
}

// RequestAccount dispatches a request to the SDN to send account information
func (sdn *SDNSocket) RequestAccount(nodeID bxtypes.NodeID, accountID bxtypes.AccountID) {
	var peerIDPtr *bxtypes.NodeID
	var accountIDPtr *bxtypes.AccountID

	if nodeID != "" {
		peerIDPtr = &nodeID
	}

	if accountID != "" {
		accountIDPtr = &accountID
	}

	log.Debugf("requesting account details for nodeID: %v account ID: %v", nodeID, accountID)
	sdn.sendSDNEvent(sdnp2p.SseAccountInfo, sdnmessage.AccountRequest{
		AccountID: accountIDPtr,
		PeerID:    peerIDPtr,
	})
}

// NodeID is not expected to be called on the SDN socket
func (sdn *SDNSocket) NodeID() bxtypes.NodeID {
	return SDNSocketID
}

// SendSubscribeNotification sends a subscription request to SDN and returns permission status
func (sdn *SDNSocket) SendSubscribeNotification(subModel *types.SubscriptionModel) (bool, string, chan *types.SubscriptionPermissionMessage) {
	if !services.SubscriptionLimitsEnforced(subModel.FeedType) {
		return true, "", nil
	}
	blacklisted, secondsRemaining := sdn.subscriptionManager.IsAccountBlacklisted(subModel.AccountID)
	if blacklisted {
		return false, fmt.Sprintf("subscription request from your account was rejected too recently, try again in %v seconds.", secondsRemaining), nil
	}

	permissionResponseCh := sdn.subscriptionManager.RecordSubscriptionRequest(subModel.SubscriptionID)

	sdn.sendSDNEvent(sdnp2p.SseSubscriptionNotification, types.SubscriptionNotification{
		Type:          types.SubscriptionNotificationTypeSubscribe,
		NodeID:        string(sdn.nodeModel.NodeID),
		Subscriptions: []types.SubscriptionModel{*subModel},
	})

	timeoutTimer := sdn.clock.Timer(sdnSubscribeRequestTimeout)
	defer timeoutTimer.Stop()
	for {
		select {
		case permissionResponse := <-permissionResponseCh:
			if len(permissionResponseCh) > 0 {
				// if there are more permission messages in the channel, this one is old
				continue
			}
			if !permissionResponse.Allowed {
				sdn.subscriptionManager.EndSubscriptionManagement(subModel.SubscriptionID)
			}
			return permissionResponse.Allowed, permissionResponse.ErrorReason, permissionResponseCh
		case <-timeoutTimer.Alert():
			log.Debugf("failed to get response for SDN subscribe request event for SubscriptionID %v, allowing subscription until otherwise directed by SDN", subModel.SubscriptionID)
			return true, "", permissionResponseCh
		}
	}
}

// SendUnsubscribeNotification sends feed unsubscribe notification to SDN
func (sdn *SDNSocket) SendUnsubscribeNotification(sub *types.SubscriptionModel) {
	if !services.SubscriptionLimitsEnforced(sub.FeedType) {
		return
	}
	sdn.subscriptionManager.EndSubscriptionManagement(sub.SubscriptionID)
	sdn.sendSDNEvent(sdnp2p.SseSubscriptionNotification, types.SubscriptionNotification{
		Type:          types.SubscriptionNotificationTypeUnsubscribe,
		NodeID:        string(sdn.nodeModel.NodeID),
		Subscriptions: []types.SubscriptionModel{*sub},
	})
}

// SendSubscriptionResetNotification sends notification to SDN containing all of node's active subscriptions
func (sdn *SDNSocket) SendSubscriptionResetNotification(subscriptions []types.SubscriptionModel) {
	sdn.sendSDNEvent(sdnp2p.SseSubscriptionNotification, types.SubscriptionNotification{
		Type:          types.SubscriptionNotificationTypeReset,
		NodeID:        string(sdn.nodeModel.NodeID),
		Subscriptions: subscriptions,
	})
}

// GenerateSubscriptionID generates and returns a new uuid to represent a subscription
func (sdn *SDNSocket) GenerateSubscriptionID(ethSubscribe bool) string {
	if ethSubscribe {
		//nolint:errcheck
		u128, _ := utils.GenerateU128()
		return u128
	}

	return utils.GenerateUUID()
}
