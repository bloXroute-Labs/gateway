package nodes

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	pingInterval                 = 15 * time.Second
	connectionStatusConnected    = "connected"
	connectionStatusNotConnected = "not_connected"
)

// AccountsFetcher method for getting sdnmessage.Account.
type AccountsFetcher interface {
	GetAccount(accountID bxtypes.AccountID) *sdnmessage.Account
}

// Bx is a base struct for bloxroute nodes
type Bx struct {
	Abstract
	BxConfig        *config.Bx
	ConnectionsLock *sync.RWMutex
	Connections     connections.ConnList
	AccountsFetcher AccountsFetcher
	dataDir         string
	clock           clock.RealClock
}

// NewBx initializes a generic Bx node struct
func NewBx(bxConfig *config.Bx, dataDir string, accountsFetcher AccountsFetcher) Bx {
	return Bx{
		BxConfig:        bxConfig,
		Connections:     make(connections.ConnList, 0),
		ConnectionsLock: &sync.RWMutex{},
		dataDir:         dataDir,
		clock:           clock.RealClock{},
		AccountsFetcher: accountsFetcher,
	}
}

// OnConnEstablished - a callback function. Called when new connection is established
func (bn *Bx) OnConnEstablished(conn connections.Conn) error {
	conn.Log().Infof("connection established, gateway: %v, protocol version %v, network %v, on local port %v",
		connections.IsGateway(conn.GetConnectionType()), conn.Protocol(),
		conn.GetNetworkNum(), conn.GetLocalPort())
	bn.ConnectionsLock.Lock()
	bn.Connections = append(bn.Connections, conn)
	bn.ConnectionsLock.Unlock()
	return nil
}

// ValidateConnection - validates connection
func (bn *Bx) ValidateConnection(conn connections.Conn) error {
	return nil
}

// OnConnClosed - a callback function. Called when new connection is closed
func (bn *Bx) OnConnClosed(conn connections.Conn) error {
	bn.ConnectionsLock.Lock()
	defer bn.ConnectionsLock.Unlock()
	for idx, connection := range bn.Connections {
		if connection.ID() == conn.ID() {
			if conn.GetConnectionType()&bxtypes.RelayProxy != 0 && !bn.isThereOtherConnectedRelay(conn) {
				bn.TxStore.Clear()
			}
			bn.Connections = append(bn.Connections[:idx], bn.Connections[idx+1:]...)
			conn.Log().Debugf("connection closed and removed from connection pool")
			return nil
		}
	}
	err := fmt.Errorf("connection can't be removed from connection list - not found")
	conn.Log().Debug(err)
	return err
}

func (bn *Bx) isThereOtherConnectedRelay(conn connections.Conn) bool {
	for _, connection := range bn.Connections {
		if connection.ID() == conn.ID() {
			continue
		}
		if conn.GetConnectionType()&bxtypes.RelayProxy != 0 {
			return true
		}
	}
	return false
}

// HandleMsg - a callback function. Generic handling for common bloXroute messages
func (bn *Bx) HandleMsg(msg bxmessage.Message, source connections.Conn) error {
	switch msg.(type) {
	case *bxmessage.SyncTxsMessage:
		txs := msg.(*bxmessage.SyncTxsMessage)
		syncTxBuffer := strings.Builder{}
		syncTxCount := 0
		syncTxBuffer.WriteString("TxStore sync: ")
		for _, csi := range txs.ContentShortIds {
			var shortID types.ShortID
			var flags types.TxFlags
			if len(csi.ShortIDs) > 0 {
				shortID = csi.ShortIDs[0]
				flags = csi.ShortIDFlags[0]
			}
			result := bn.TxStore.Add(csi.Hash, csi.Content, shortID, txs.GetNetworkNum(), false, flags, csi.Timestamp(), 0, types.EmptySender)
			if result.NewTx || result.NewSID || result.NewContent {
				syncTxBuffer.WriteString(fmt.Sprintf("added hash %v newTx %v newContent %v newSid %v networkNum %v; ",
					hex.EncodeToString(csi.Hash[:]), result.NewTx, result.NewContent, result.NewSID, result.Transaction.NetworkNum()))
				syncTxCount++
				if syncTxCount == 1000 {
					source.Log().Trace(syncTxBuffer.String())
					syncTxBuffer.Reset()
					syncTxCount = 0
				}
			}
		}
		if syncTxCount != 0 {
			source.Log().Trace(syncTxBuffer.String())
		}

	case *bxmessage.SyncReq:
		syncReq := msg.(*bxmessage.SyncReq)
		txCount := 0
		sentCount := 0
		syncTxs := &bxmessage.SyncTxsMessage{}
		syncTxs.SetNetworkNum(syncReq.GetNetworkNum())
		priority := bxmessage.OnPongPriority
		if source.GetConnectionType()&bxtypes.RelayProxy != 0 || source.Protocol() >= bxmessage.MinProtocol {
			priority = bxmessage.NormalPriority
		}

		for txInfo := range bn.TxStore.Iter() {
			if txInfo.NetworkNum() != syncTxs.GetNetworkNum() {
				continue
			}
			if syncTxs.Add(txInfo) > bxgateway.SyncChunkSize {
				syncTxs.SetPriority(priority)
				// not checking error here as we have to finish the for loop to clear the Iterator goroutine
				_ = source.Send(syncTxs)
				sentCount += syncTxs.Count()
				syncTxs = &bxmessage.SyncTxsMessage{}
				syncTxs.SetNetworkNum(syncReq.GetNetworkNum())
			}
			txCount++
		}
		syncTxs.SetPriority(priority)
		err := source.Send(syncTxs)
		if err != nil {
			return err
		}
		sentCount += syncTxs.Count()
		syncDone := &bxmessage.SyncDone{}
		syncDone.SetNetworkNum(syncReq.GetNetworkNum())
		syncDone.SetPriority(priority)
		err = source.Send(syncDone)
		if err != nil {
			return err
		}

		source.Log().Debugf("TxStore sync: done sending %v out of %v entries for network %v", sentCount, txCount, syncReq.GetNetworkNum())

	case *bxmessage.SyncDone:
		source.Log().Infof("completed transaction sync (%v entries)", bn.TxStore.Count())

	case *bxmessage.TxCleanup:
		cleanup := msg.(*bxmessage.TxCleanup)
		sizeBefore := bn.TxStore.Count()
		startTime := time.Now()
		bn.TxStore.RemoveShortIDs(&cleanup.ShortIDs, services.FullReEntryProtection, "TxCleanup message")
		source.Log().Debugf("TxStore cleanup (go routine) by txcleanup message took %v. Size before %v, size after %v, shortIds %v",
			time.Since(startTime), sizeBefore, bn.TxStore.Count(), len(cleanup.ShortIDs))

	case *bxmessage.BlockConfirmation:
		blockConfirmation := msg.(*bxmessage.BlockConfirmation)
		sizeBefore := bn.TxStore.Count()
		startTime := time.Now()
		bn.TxStore.RemoveHashes(&blockConfirmation.Hashes, services.ShortReEntryProtection, "BlockConfirmation message")
		source.Log().Debugf("TxStore cleanup (go routine) by %v message took %v. Size before %v, size after %v, hashes %v",
			bxmessage.BlockConfirmationType, time.Since(startTime), sizeBefore, bn.TxStore.Count(), len(blockConfirmation.Hashes))
	case *bxmessage.ErrorNotification:
		errorNotification := msg.(*bxmessage.ErrorNotification)
		source.Log().Warnf("received unexpected error notification: code %v, reason %v", errorNotification.Code, errorNotification.Reason)
	default:
		source.Log().Errorf("unknown message type %v received: ", reflect.TypeOf(msg))
	}
	return nil
}

// DisconnectConn - disconnect a specific connection
func (bn *Bx) DisconnectConn(id bxtypes.NodeID) {
	bn.ConnectionsLock.Lock()
	for _, conn := range bn.Connections {
		if id == conn.GetNodeID() {
			// closing in a new go routine in order to avoid deadlock while Close method acquiring ConnectionsLock
			go conn.Close("disconnect requested by bxapi")
		}
	}
	bn.ConnectionsLock.Unlock()
}

// Peers provides a list of current peers for the requested type
func (bn *Bx) Peers(peerType string) []bxmessage.PeerInfo {
	var nodeType bxtypes.NodeType = -1 // all types
	switch peerType {
	case "gw", "gateway":
		nodeType = bxtypes.Gateway
	case "relay":
		nodeType = bxtypes.RelayProxy
	}

	peers := make([]bxmessage.PeerInfo, 0, len(bn.Connections))

	bn.ConnectionsLock.RLock()
	defer bn.ConnectionsLock.RUnlock()

	for _, conn := range bn.Connections {
		connectionType := conn.GetConnectionType()
		if connectionType&nodeType == 0 {
			continue
		}
		connType := connectionType.String()

		var trusted string

		accountID := conn.GetAccountID()
		if bn.AccountsFetcher != nil {
			if accountModel := bn.AccountsFetcher.GetAccount(accountID); accountModel != nil {
				trusted = strconv.FormatBool(accountModel.IsTrusted())
			}
		}

		peer := bxmessage.PeerInfo{
			IP:         conn.GetPeerIP(),
			NodeID:     string(conn.GetNodeID()),
			Protocol:   uint32(conn.Protocol()),
			Type:       connType,
			State:      conn.GetConnectionState(),
			Network:    uint32(conn.GetNetworkNum()),
			Initiator:  conn.IsInitiator(),
			AccountID:  string(accountID),
			Port:       conn.GetLocalPort(),
			Disabled:   conn.IsDisabled(),
			Capability: uint32(conn.GetCapabilities()),
			Trusted:    trusted,
		}
		if bxConn, ok := conn.(*handler.BxConn); ok {
			peer.MinUsFromPeer, peer.MinUsToPeer, peer.SlowTrafficCount, peer.MinUsRoundTrip = bxConn.GetMinLatencies()
		}
		peers = append(peers, peer)

	}

	return peers
}

// Relays provides a list of current relays
func (bn *Bx) Relays() map[string]bxmessage.RelayConnectionInfo {
	bn.ConnectionsLock.RLock()
	defer bn.ConnectionsLock.RUnlock()

	mp := make(map[string]bxmessage.RelayConnectionInfo)

	for _, conn := range bn.Connections {
		connectionType := conn.GetConnectionType()

		if connectionType&bxtypes.RelayProxy == 0 {
			continue
		}

		peerIP := conn.GetPeerIP()

		if !conn.IsOpen() {
			mp[peerIP] = bxmessage.RelayConnectionInfo{
				Status: connectionStatusNotConnected,
			}

			continue
		}

		var connectionLatency *bxmessage.ConnectionLatency
		if bxConn, ok := conn.(*handler.BxConn); ok {
			minMsFromPeer, minMsToPeer, slowTrafficCount, minMsRoundTrip := bxConn.GetMinLatencies()
			connectionLatency = &bxmessage.ConnectionLatency{
				MinMsFromPeer:    minMsFromPeer,
				MinMsToPeer:      minMsToPeer,
				SlowTrafficCount: slowTrafficCount,
				MinMsRoundTrip:   minMsRoundTrip,
			}
		}

		mp[peerIP] = bxmessage.RelayConnectionInfo{
			Status:      connectionStatusConnected,
			ConnectedAt: conn.GetConnectedAt().Format(time.RFC3339),
			Latency:     connectionLatency,
		}
	}

	return mp
}

// PingLoop send a ping request every pingInterval to gateway, relays, and proxies. Can't use broadcast due to geo constrains
func (bn *Bx) PingLoop() {
	pingTicker := bn.clock.Ticker(pingInterval)
	ping := &bxmessage.Ping{}
	to := bxtypes.Gateway | bxtypes.RelayProxy
	for {
		select {
		case <-pingTicker.Alert():
			count := 0
			bn.ConnectionsLock.RLock()
			for _, conn := range bn.Connections {
				if conn.GetConnectionType()&to != 0 {
					err := conn.Send(ping)
					if err != nil {
						conn.Log().Errorf("error sending ping message: %v", err)
						continue
					}
					count++
				}
			}
			bn.ConnectionsLock.RUnlock()
			log.Tracef("ping message sent to %v connections", count)
		}
	}
}

// Broadcast sends a message to all connections of the specified type
func (bn *Bx) Broadcast(msg bxmessage.Message, source connections.Conn, to bxtypes.NodeType) types.BroadcastResults {
	results := types.BroadcastResults{}

	bn.ConnectionsLock.RLock()
	defer bn.ConnectionsLock.RUnlock()

	for _, conn := range bn.Connections {
		connectionType := conn.GetConnectionType()

		// if connection type is not in target - skip
		if connectionType&to == 0 {
			continue
		}

		results.RelevantPeers++
		if !conn.IsOpen() || source != nil && conn.ID() == source.ID() {
			results.NotOpenPeers++
			continue
		}

		err := conn.Send(msg)
		if err != nil {
			conn.Log().Errorf("error writing to connection, closing")
			results.ErrorPeers++
			continue
		}

		if connections.IsGateway(connectionType) {
			results.SentGatewayPeers++
		}

		results.SentPeers++
	}

	return results
}
