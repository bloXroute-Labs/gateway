package nodes

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	bxgateway "github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pbbase "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	pingInterval = 15 * time.Second
)

// AccountsFetcher method for getting sdnmessage.Account.
type AccountsFetcher interface {
	GetAccount(accountID types.AccountID) *sdnmessage.Account
}

// Bx is a base struct for bloxroute nodes
type Bx struct {
	Abstract
	BxConfig        *config.Bx
	ConnectionsLock *sync.RWMutex
	Connections     connections.ConnList
	AccountsFetcher AccountsFetcher
	dataDir         string
	clock           utils.RealClock
}

// NewBx initializes a generic Bx node struct
func NewBx(bxConfig *config.Bx, dataDir string, accountsFetcher AccountsFetcher) Bx {
	return Bx{
		BxConfig:        bxConfig,
		Connections:     make(connections.ConnList, 0),
		ConnectionsLock: &sync.RWMutex{},
		dataDir:         dataDir,
		clock:           utils.RealClock{},
		AccountsFetcher: accountsFetcher,
	}
}

// OnConnEstablished - a callback function. Called when new connection is established
func (bn *Bx) OnConnEstablished(conn connections.Conn) error {
	conn.Log().Infof("connection established, gateway: %v, bdn: %v protocol version %v, network %v, on local port %v",
		connections.IsGateway(conn.GetConnectionType()), connections.IsBDN(conn.GetCapabilities()), conn.Protocol(),
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
			if conn.GetConnectionType()&utils.RelayTransaction != 0 {
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
					source.Log().Tracef(syncTxBuffer.String())
					syncTxBuffer.Reset()
					syncTxCount = 0
				}
			}
		}
		if syncTxCount != 0 {
			source.Log().Tracef(syncTxBuffer.String())
		}

	case *bxmessage.SyncReq:
		syncReq := msg.(*bxmessage.SyncReq)
		txCount := 0
		sentCount := 0
		syncTxs := &bxmessage.SyncTxsMessage{}
		syncTxs.SetNetworkNum(syncReq.GetNetworkNum())
		priority := bxmessage.OnPongPriority
		if source.GetConnectionType()&utils.RelayProxy != 0 || source.Protocol() >= bxmessage.MinFastSyncProtocol {
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

	default:
		source.Log().Errorf("unknown message type %v received", reflect.TypeOf(msg))
	}
	return nil
}

// DisconnectConn - disconnect a specific connection
func (bn *Bx) DisconnectConn(id types.NodeID) {
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
func (bn *Bx) Peers(_ context.Context, req *pbbase.PeersRequest) (*pbbase.PeersReply, error) {
	var nodeType utils.NodeType = -1 // all types
	switch req.Type {
	case "gw", "gateway":
		nodeType = utils.Gateway
	case "relay":
		nodeType = utils.Relay
	}
	resp := &pbbase.PeersReply{}
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

		peer := &pbbase.Peer{
			Ip:         conn.GetPeerIP(),
			NodeId:     string(conn.GetNodeID()),
			Type:       connType,
			State:      conn.GetConnectionState(),
			Network:    uint32(conn.GetNetworkNum()),
			Initiator:  conn.IsInitiator(),
			AccountId:  string(accountID),
			Port:       conn.GetLocalPort(),
			Disabled:   conn.IsDisabled(),
			Capability: uint32(conn.GetCapabilities()),
			Trusted:    trusted,
		}
		if bxConn, ok := conn.(*handler.BxConn); ok {
			peer.MinMsFromPeer, peer.MinMsToPeer, peer.SlowTrafficCount, peer.MinMsRoundTrip = bxConn.GetMinLatencies()
		}
		resp.Peers = append(resp.Peers, peer)

	}
	return resp, nil
}

// PingLoop send a ping request every pingInterval to gateway, relays, and proxies. Can't use broadcast due to geo constrains
func (bn *Bx) PingLoop() {
	pingTicker := bn.clock.Ticker(pingInterval)
	ping := &bxmessage.Ping{}
	to := utils.Gateway | utils.RelayProxy | utils.Relay
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
