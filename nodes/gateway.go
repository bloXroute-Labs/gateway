package nodes

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/config"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/connections/handler"
	"github.com/bloXroute-Labs/gateway/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/logger"
	pb "github.com/bloXroute-Labs/gateway/protobuf"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/servers"
	"github.com/bloXroute-Labs/gateway/services"
	"github.com/bloXroute-Labs/gateway/services/loggers"
	"github.com/bloXroute-Labs/gateway/services/statistics"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/bloXroute-Labs/gateway/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"path"
	"reflect"
	"runtime"
	"strings"
	"time"
)

const (
	flasbotAuthHeader  = "X-Flashbots-Signature"
	timeToAvoidReEntry = 6 * time.Hour
)

type gateway struct {
	Bx
	pb.UnimplementedGatewayServer
	context context.Context
	cancel  context.CancelFunc

	sdn                connections.SDNHTTP
	accountID          types.AccountID
	bridge             blockchain.Bridge
	feedManager        *servers.FeedManager
	feedChan           chan types.Notification
	asyncMsgChannel    chan services.MsgInfo
	isBDN              bool
	bdnStats           *bxmessage.BdnPerformanceStats
	blockProcessor     services.BlockProcessor
	pendingTxs         services.HashHistory
	possiblePendingTxs services.HashHistory
	txTrace            loggers.TxTrace
	blockchainPeers    []types.NodeEndpoint
	stats              statistics.Stats
	bdnBlocks          services.HashHistory
	wsManager          blockchain.WSManager
	syncedWithRelay    atomic.Bool
	clock              utils.Clock
	timeStarted        time.Time
	burstLimiter       services.AccountBurstLimiter

	bestBlockHeight       int
	bdnBlocksSkipCount    int
	seenMEVMinerBundles   services.HashHistory
	seenMEVSearchers      services.HashHistory
	seenBlockConfirmation services.HashHistory

	mevClient    *http.Client
	gatewayPeers string
}

func generatePeers(peersInfo []network.PeerInfo) string {
	var result string
	if len(peersInfo) == 0 {
		return result
	}
	for _, peer := range peersInfo {
		result += fmt.Sprintf("%s+%s,", peer.Enode.String(), peer.EthWSURI)
	}
	result = result[:len(result)-1]
	return result
}

// NewGateway returns a new gateway node to send messages from a blockchain node to the relay network
func NewGateway(parent context.Context, bxConfig *config.Bx, bridge blockchain.Bridge, wsManager blockchain.WSManager,
	blockchainPeers []types.NodeEndpoint, peersInfo []network.PeerInfo) (Node, error) {
	ctx, cancel := context.WithCancel(parent)
	clock := utils.RealClock{}

	g := &gateway{
		Bx:                    NewBx(bxConfig, "datadir"),
		bridge:                bridge,
		isBDN:                 bxConfig.GatewayMode.IsBDN(),
		wsManager:             wsManager,
		context:               ctx,
		cancel:                cancel,
		blockchainPeers:       blockchainPeers,
		pendingTxs:            services.NewHashHistory("pendingTxs", 15*time.Minute),
		possiblePendingTxs:    services.NewHashHistory("possiblePendingTxs", 15*time.Minute),
		bdnBlocks:             services.NewHashHistory("bdnBlocks", 15*time.Minute),
		seenMEVMinerBundles:   services.NewHashHistory("mevMinerBundle", 30*time.Minute),
		seenMEVSearchers:      services.NewHashHistory("mevSearcher", 30*time.Minute),
		seenBlockConfirmation: services.NewHashHistory("blockConfirmation", 30*time.Minute),
		clock:                 clock,
		timeStarted:           clock.Now(),
		gatewayPeers:          generatePeers(peersInfo),
	}
	g.asyncMsgChannel = services.NewAsyncMsgChannel(g)
	g.mevClient = &http.Client{
		Transport: &http.Transport{
			MaxConnsPerHost:     100,
			MaxIdleConnsPerHost: 100,
			MaxIdleConns:        100,
			IdleConnTimeout:     0 * time.Second,
		},
		Timeout: 60 * time.Second,
	}

	// create tx store service pass to eth client
	assigner := services.NewEmptyShortIDAssigner()
	txStore := services.NewBxTxStore(30*time.Minute, 3*24*time.Hour, 10*time.Minute, assigner, services.NewHashHistory("seenTxs", 30*time.Minute), nil, timeToAvoidReEntry)
	g.TxStore = &txStore
	g.bdnStats = bxmessage.NewBDNStats()
	g.blockProcessor = services.NewRLPBlockProcessor(g.TxStore)
	g.burstLimiter = services.NewAccountBurstLimiter(g.clock)

	// set empty default stats, Run function will override it
	g.stats = statistics.NewStats(false, "127.0.0.1", "", nil, false)

	return g, nil
}

func (g *gateway) isSyncWithRelay() bool {
	return g.syncedWithRelay.Load()
}

func (g *gateway) setSyncWithRelay() {
	g.syncedWithRelay.Store(true)
}

func (g *gateway) Run() error {
	defer g.cancel()

	var err error

	privateCertDir := path.Join(g.BxConfig.DataDir, "ssl")
	gatewayType := g.BxConfig.NodeType
	gatewayMode := g.BxConfig.GatewayMode

	privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile := utils.GetCertDir(g.BxConfig.RegistrationCertDir, privateCertDir, strings.ToLower(gatewayType.String()))
	sslCerts := utils.NewSSLCertsFromFiles(privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile)
	if g.accountID, err = sslCerts.GetAccountID(); err != nil {
		return err
	}

	// node ID might not be assigned to gateway yet, ok
	nodeID, _ := sslCerts.GetNodeID()
	log.WithFields(log.Fields{
		"accountID": g.accountID,
		"nodeID":    nodeID,
	}).Infof("ssl certificate successfully loaded")

	_, err = os.Stat(".dockerignore")
	isDocker := !os.IsNotExist(err)
	hostname, _ := os.Hostname()

	blockchainPeerEndpoint := types.NodeEndpoint{IP: "", Port: 0, PublicKey: ""}
	if len(g.blockchainPeers) > 0 {
		blockchainPeerEndpoint.IP = g.blockchainPeers[0].IP
		blockchainPeerEndpoint.Port = g.blockchainPeers[0].Port
		blockchainPeerEndpoint.PublicKey = g.blockchainPeers[0].PublicKey
	}

	nodeModel := sdnmessage.NodeModel{
		NodeType:        gatewayType.String(),
		GatewayMode:     string(gatewayMode),
		ExternalIP:      g.BxConfig.ExternalIP,
		ExternalPort:    g.BxConfig.ExternalPort,
		BlockchainIP:    blockchainPeerEndpoint.IP,
		BlockchainPeers: g.gatewayPeers,
		NodePublicKey:   blockchainPeerEndpoint.PublicKey,
		BlockchainPort:  blockchainPeerEndpoint.Port,
		ProgramName:     types.BloxrouteGoGateway,
		SourceVersion:   version.BuildVersion,
		IsDocker:        isDocker,
		Hostname:        hostname,
		OsVersion:       runtime.GOOS,
		ProtocolVersion: bxmessage.CurrentProtocol,
		IsGatewayMiner:  g.BxConfig.BlocksOnly,
		NodeStartTime:   time.Now().Format(bxgateway.TimeLayoutISO),
	}

	g.sdn = connections.NewSDNHTTP(&sslCerts, g.BxConfig.SDNURL, nodeModel, g.BxConfig.DataDir)
	var group errgroup.Group

	err = g.sdn.InitGateway(bxgateway.Ethereum, g.BxConfig.BlockchainNetwork)
	if err != nil {
		return err
	}

	if g.BxConfig.MEVBuilderURI != "" && g.sdn.AccountModel().MEVBuilder == "" {
		panic(fmt.Errorf("account %v is not allowed for mev builder service, closing the gateway. Please contact support@bloxroute.com to enable running as mev builder", g.sdn.AccountModel().AccountID))
	}

	if g.BxConfig.MEVMinerURI != "" && g.sdn.AccountModel().MEVMiner == "" {
		panic(fmt.Errorf(
			"account %v is not allowed for mev miner service, closing the gateway. Please contact support@bloxroute.com to enable running as mev miner",
			g.sdn.AccountModel().AccountID,
		))
	}

	accountModel := g.sdn.AccountModel()
	g.burstLimiter.Register(&accountModel)

	var txTraceLogger *log.Logger = nil
	if g.BxConfig.TxTraceLog.Enabled {
		txTraceLogger, err = log.CreateCustomLogger(
			g.BxConfig.AppName,
			int(g.BxConfig.ExternalPort),
			"txtrace",
			g.BxConfig.TxTraceLog.MaxFileSize,
			g.BxConfig.TxTraceLog.MaxBackupFiles,
			1,
			log.TraceLevel,
		)
		if err != nil {
			return fmt.Errorf("failed to create TxTrace logger: %v", err)
		}
	}
	g.txTrace = loggers.NewTxTrace(txTraceLogger)

	if g.sdn.AccountTier() != sdnmessage.ATierElite {
		if len(g.blockchainPeers) == 0 {
			return fmt.Errorf("no blockchain node specified. Enterprise-Elite account is required in order to run gateway without a blockchain node")
		}
		if len(g.blockchainPeers) > 1 {
			return fmt.Errorf("multiple blockchain nodes specfied. Enterprise-Elite account is required in order to run gateway with multiple blockchain nodes")
		}
	}

	networkNum := g.sdn.NetworkNum()

	err = g.pushBlockchainConfig()
	if err != nil {
		return fmt.Errorf("could process initial blockchain configuration: %v", err)
	}
	group.Go(g.handleBridgeMessages)
	group.Go(g.TxStore.Start)

	g.stats = statistics.NewStats(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, g.sdn.NodeID(), g.sdn.Networks(), g.BxConfig.LogNetworkContent)

	g.feedChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.feedManager = servers.NewFeedManager(g.context, g, g.feedChan, networkNum,
		g.wsManager, accountModel, g.sdn.FetchCustomerAccountModel,
		privateCertFile, privateKeyFile, *g.BxConfig, g.stats)
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled {
		group.Go(g.feedManager.Start)
	}

	if err = log.InitFluentD(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, string(g.sdn.NodeID())); err != nil {
		return err
	}

	go g.PingLoop()

	relayInstructions := make(chan connections.RelayInstruction)
	go g.updateRelayConnections(relayInstructions, sslCerts, networkNum)
	err = g.sdn.DirectRelayConnections(context.Background(), g.BxConfig.Relays, accountModel.RelayLimit.MsgQuota.Limit, relayInstructions, connections.AutoRelayTimeout)
	if err != nil {
		return err
	}

	go g.sendStatsOnInterval(15 * time.Minute)

	if g.BxConfig.GRPC.Enabled {
		grpcServer := newGatewayGRPCServer(g, g.BxConfig.Host, g.BxConfig.Port, g.BxConfig.User, g.BxConfig.Password)
		group.Go(grpcServer.Start)
	}

	err = group.Wait()
	if err != nil {
		return err
	}

	return nil
}

func (g *gateway) updateRelayConnections(relayInstructions chan connections.RelayInstruction, sslCerts utils.SSLCerts, networkNum types.NetworkNum) {
	for {
		instruction := <-relayInstructions

		switch instruction.Type {
		case connections.Connect:
			g.connectRelay(instruction, sslCerts, networkNum)
		case connections.Disconnect:
			// TODO disconnect relay, phase 2
		}
	}
}

func (g *gateway) connectRelay(instruction connections.RelayInstruction, sslCerts utils.SSLCerts, networkNum types.NetworkNum) {
	relay := handler.NewOutboundRelay(g, &sslCerts, instruction.IP, instruction.Port, g.sdn.NodeID(), utils.Relay,
		g.BxConfig.PrioritySending, g.sdn.Networks(), true, false, utils.RealClock{}, false, g.isBDN)
	relay.SetNetworkNum(networkNum)

	relay.Start()

	log.Infof("gateway %v (%v) starting, connecting to relay %v:%v", g.sdn.NodeID(), g.BxConfig.Environment, instruction.IP, instruction.Port)
}

func (g *gateway) broadcast(msg bxmessage.Message, source connections.Conn, to utils.NodeType) types.BroadcastResults {
	g.ConnectionsLock.RLock()
	defer g.ConnectionsLock.RUnlock()
	results := types.BroadcastResults{}

	for _, conn := range g.Connections {
		// if connection type is not in target - skip
		if conn.Info().ConnectionType&to == 0 {
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

		if conn.Info().IsGateway() {
			results.SentGatewayPeers++
		}

		results.SentPeers++
	}
	return results
}

func (g *gateway) pushBlockchainConfig() error {
	blockchainNetwork, err := g.sdn.FindNetwork(g.sdn.NetworkNum())
	if err != nil {
		return err
	}

	blockchainAttributes := blockchainNetwork.DefaultAttributes
	chainDifficulty, ok := blockchainAttributes.ChainDifficulty.(string)
	if !ok {
		return fmt.Errorf("could not parse total difficulty: %v", blockchainAttributes.ChainDifficulty)
	}
	td, ok := new(big.Int).SetString(chainDifficulty, 16)
	if !ok {
		return fmt.Errorf("could not parse total difficulty: %v", blockchainAttributes.ChainDifficulty)
	}

	genesis := common.HexToHash(blockchainAttributes.GenesisHash)
	ignoreBlockTimeout := time.Second * time.Duration(blockchainNetwork.BlockInterval*blockchainNetwork.IgnoreBlockIntervalCount)
	blockConfirmationsCount := blockchainNetwork.BlockConfirmationsCount
	ethConfig := network.EthConfig{
		Network:                 uint64(blockchainAttributes.NetworkID),
		TotalDifficulty:         td,
		Head:                    genesis,
		Genesis:                 genesis,
		IgnoreBlockTimeout:      ignoreBlockTimeout,
		BlockConfirmationsCount: blockConfirmationsCount,
	}

	return g.bridge.UpdateNetworkConfig(ethConfig)
}

func (g *gateway) publishBlock(bxBlock *types.BxBlock, feedName types.FeedType, nodeSource *connections.Blockchain) error {
	// publishing a block means extracting the sender for all the block transactions which is heavy.
	// if there are no active block related feed subscribers we can skip this.
	if !g.feedManager.NeedBlocks() {
		return nil
	}

	blockHeight := int(bxBlock.Number.Int64())
	if feedName == types.BDNBlocksFeed {
		if !g.bdnBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute) {
			log.Debugf("Block %v with height %v was already published with feed %v", bxBlock.Hash(), bxBlock.Number, types.BDNBlocksFeed)
			return nil
		}
		if len(g.blockchainPeers) > 0 && blockHeight < g.bestBlockHeight {
			log.Debugf("block %v (%v) is too far behind best block height %v from node - not publishing to bdnBlocks", bxBlock.Number, bxBlock.Hash(), g.bestBlockHeight)
			return nil
		}
		if g.bestBlockHeight != 0 && utils.Abs(blockHeight-g.bestBlockHeight) > bxgateway.BDNBlocksMaxBlocksAway {
			if blockHeight > g.bestBlockHeight {
				g.bdnBlocksSkipCount++
			}
			if g.bdnBlocksSkipCount <= bxgateway.MaxOldBDNBlocksToSkipPublish {
				log.Debugf("block %v (%v) is too far away from best block height %v - not publishing to bdnBlocks", bxBlock.Number, bxBlock.Hash(), g.bestBlockHeight)
				return nil
			}
			log.Debugf("publishing block from BDN with height %v that is far away from current best block height %v - resetting bestBlockHeight to zero", blockHeight, g.bestBlockHeight)
			g.bestBlockHeight = 0
		}
		g.bdnBlocksSkipCount = 0
		if len(g.blockchainPeers) == 0 && blockHeight > g.bestBlockHeight {
			g.bestBlockHeight = blockHeight
		}
	}

	blockNotification, err := g.bridge.BxBlockToCanonicFormat(bxBlock)
	if err != nil {
		return err
	}
	blockNotification.SetNotificationType(feedName)
	log.Debugf("Received block for %v, block hash: %v, block height: %v, source: %v, notify the feed", feedName, bxBlock.Hash(), bxBlock.Number, nodeSource)
	g.notify(blockNotification)
	if feedName == types.NewBlocksFeed {
		g.bestBlockHeight = blockHeight
		g.bdnBlocksSkipCount = 0

		onBlockNotification := *blockNotification
		onBlockNotification.SetNotificationType(types.OnBlockFeed)
		if nodeSource != nil {
			sourceEndpoint := nodeSource.NodeEndpoint()
			onBlockNotification.SetSource(&sourceEndpoint)
		}
		log.Debugf("Received block for %v, block hash: %v, block height: %v, source: %v, notify the feed", types.OnBlockFeed, bxBlock.Hash(), bxBlock.Number, nodeSource)
		g.notify(&onBlockNotification)

		txReceiptNotification := *blockNotification
		txReceiptNotification.SetNotificationType(types.TxReceiptsFeed)
		if nodeSource != nil {
			sourceEndpoint := nodeSource.NodeEndpoint()
			txReceiptNotification.SetSource(&sourceEndpoint)
		}
		log.Debugf("Received block for %v, block hash: %v, block height: %v, source: %v notify the feed", types.TxReceiptsFeed, bxBlock.Hash(), bxBlock.Number, nodeSource)
		g.notify(&txReceiptNotification)
	}
	return nil
}

func (g *gateway) publishPendingTx(txHash types.SHA256Hash, bxTx *types.BxTransaction, fromNode bool) {
	if g.pendingTxs.Exists(txHash.String()) {
		return
	}

	if fromNode || g.possiblePendingTxs.Exists(txHash.String()) {
		if bxTx != nil && bxTx.HasContent() {
			// if already has tx content, tx is pending and notify it
			pendingTxsNotification := types.CreatePendingTransactionNotification(bxTx)
			g.notify(pendingTxsNotification)
			g.pendingTxs.Add(txHash.String(), 15*time.Minute)
		} else if fromNode {
			// not asking for tx content as we expect it to happen anyway
			g.possiblePendingTxs.Add(txHash.String(), 15*time.Minute)
		}
	}
}

func (g *gateway) handleBridgeMessages() error {
	var err error
	for {
		select {
		case txsFromNode := <-g.bridge.ReceiveNodeTransactions():
			// if we are not yet synced with relay - ignore the transactions from the node
			if !g.isSyncWithRelay() {
				continue
			}
			blockchainConnection := connections.NewBlockchainConn(txsFromNode.PeerEndpoint)
			for _, blockchainTx := range txsFromNode.Transactions {
				tx := bxmessage.NewTx(blockchainTx.Hash(), blockchainTx.Content(), g.sdn.NetworkNum(), types.TFLocalRegion, types.EmptyAccountID)
				g.processTransaction(tx, blockchainConnection)
			}
		case txAnnouncement := <-g.bridge.ReceiveTransactionHashesAnnouncement():
			// if we are not yet synced with relay - ignore the announcement from the node
			if !g.isSyncWithRelay() {
				continue
			}
			// if announcement message has many transaction we are probably after reconnect with the node - we should ignore it in order not to over load the client feed
			if len(txAnnouncement.Hashes) > bxgateway.MaxAnnouncementFromNode {
				log.Debugf("skipped tx announcement of size %v", len(txAnnouncement.Hashes))
				continue
			}
			requests := make([]types.SHA256Hash, 0)
			for _, hash := range txAnnouncement.Hashes {
				bxTx, exists := g.TxStore.Get(hash)
				if !exists && !g.TxStore.Known(hash) {
					log.Tracef("msgTx: from Blockchain, hash %v, event TxAnnouncedByBlockchainNode, peerID: %v", hash, txAnnouncement.PeerID)
					requests = append(requests, hash)
				} else {
					var diffFromBDNTime int64
					var delivered bool
					var expected = "expected"
					if bxTx != nil {
						diffFromBDNTime = time.Since(bxTx.AddTime()).Microseconds()
						delivered = bxTx.Flags().ShouldDeliverToNode()
					}
					if delivered && txAnnouncement.PeerID != bxgateway.WSConnectionID {
						expected = "un-expected"
					}

					log.Tracef("msgTx: from Blockchain, hash %v, event TxAnnouncedByBlockchainNodeIgnoreSeen, peerID: %v, delivered %v, %v, diffFromBDNTime %v", hash, txAnnouncement.PeerID, delivered, expected, diffFromBDNTime)
					// if we delivered to node and got it from the node we were very late.
				}
				g.publishPendingTx(hash, bxTx, true)
			}

			if len(requests) > 0 && txAnnouncement.PeerID != bxgateway.WSConnectionID {
				err = g.bridge.RequestTransactionsFromNode(txAnnouncement.PeerID, requests)
				if err == blockchain.ErrChannelFull {
					log.Warnf("transaction requests channel is full, skipping request for %v hashes", len(requests))
				} else if err != nil {
					panic(fmt.Errorf("could not request transactions over bridge: %v", err))
				}
			}
		case blockFromNode := <-g.bridge.ReceiveBlockFromNode():
			blockchainConnection := connections.NewBlockchainConn(blockFromNode.PeerEndpoint)
			g.processBlockFromBlockchain(blockFromNode.Block, blockchainConnection)
		case _ = <-g.bridge.ReceiveNoActiveBlockchainPeersAlert():
			if g.sdn.AccountTier() != sdnmessage.ATierElite {
				// TODO should fix code to stop gateway appropriately
				log.Errorf("Gateway does not have an active blockchain connection. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
				log.Exit(0)
			}
		case confirmBlock := <-g.bridge.ReceiveConfirmedBlockFromNode():
			bcnfMsg := bxmessage.BlockConfirmation{}
			txList := make(types.SHA256HashList, 0, len(confirmBlock.Block.Txs))
			for _, tx := range confirmBlock.Block.Txs {
				txList = append(txList, tx.Hash())
			}

			bcnfMsg.Hashes = txList
			bcnfMsg.SetNetworkNum(g.sdn.NetworkNum())
			bcnfMsg.SetHash(confirmBlock.Block.Hash())
			blockchainConnection := connections.NewBlockchainConn(confirmBlock.PeerEndpoint)
			g.HandleMsg(&bcnfMsg, blockchainConnection, connections.RunBackground)
		}
	}
}

func (g *gateway) NodeStatus() connections.NodeStatus {
	var capabilities types.CapabilityFlags

	if g.BxConfig.MEVBuilderURI != "" {
		capabilities |= types.CapabilityMEVBuilder
	}

	if g.BxConfig.MEVMinerURI != "" {
		capabilities |= types.CapabilityMEVMiner
	}

	if g.BxConfig.GatewayMode.IsBDN() {
		capabilities |= types.CapabilityBDN
	}

	return connections.NodeStatus{
		Capabilities: capabilities,
		Version:      version.BuildVersion,
	}
}

func (g *gateway) HandleMsg(msg bxmessage.Message, source connections.Conn, background connections.MsgHandlingOptions) error {
	var err error
	if background {
		g.asyncMsgChannel <- services.MsgInfo{Msg: msg, Source: source}
		return nil
	}
	switch msg.(type) {
	case *bxmessage.Tx:
		tx := msg.(*bxmessage.Tx)
		g.processTransaction(tx, source)
	case *bxmessage.Broadcast:
		broadcastMsg := msg.(*bxmessage.Broadcast)
		// handle in a go-routing so tx flow from relay will not be delayed
		go g.processBroadcast(broadcastMsg, source)
	case *bxmessage.RefreshBlockchainNetwork:
		go g.sdn.FetchBlockchainNetwork() //nolint
	case *bxmessage.Txs:
		// TODO: check if this is the message type we want to use?
		txsMessage := msg.(*bxmessage.Txs)
		for _, txsItem := range txsMessage.Items() {
			g.TxStore.Add(txsItem.Hash, txsItem.Content, txsItem.ShortID, g.sdn.NetworkNum(), false, 0, time.Now(), 0)
		}
	case *bxmessage.SyncDone:
		g.setSyncWithRelay()
		err = g.Bx.HandleMsg(msg, source)
	case *bxmessage.MEVBundle:
		mevBundle := msg.(*bxmessage.MEVBundle)
		go g.handleMEVBundleMessage(*mevBundle, source)
	case *bxmessage.MEVSearcher:
		mevSearcher := msg.(*bxmessage.MEVSearcher)
		go g.handleMEVSearcherMessage(*mevSearcher, source)
	case *bxmessage.ErrorNotification:
		errorNotification := msg.(*bxmessage.ErrorNotification)
		source.Log().Errorf("received an error notification %v. terminating the gateway", errorNotification.Reason)
		// TODO should also close the gateway while notify the bridge and other go routine (web socket server, ...)
		log.Exit(0)
	case *bxmessage.Hello:
		source.Log().Tracef("received hello msg")

	case *bxmessage.BlockConfirmation:
		blockConfirmation := msg.(*bxmessage.BlockConfirmation)
		hashString := blockConfirmation.Hash().String()
		if g.seenBlockConfirmation.SetIfAbsent(hashString, 30*time.Minute) {
			if !g.BxConfig.SendConfirmation {
				log.Debug("gateway will not send block confirm message to relay without --send-block-confirmation flag")
			} else if source.Info().ConnectionType == utils.Blockchain {
				log.Tracef("gateway broadcasting block confirmation of block %v to relays", hashString)
				g.broadcast(blockConfirmation, source, utils.Relay)
			}
			_ = g.Bx.HandleMsg(msg, source)
		}

	default:
		err = g.Bx.HandleMsg(msg, source)
	}
	return err
}

func (g *gateway) processBroadcast(broadcastMsg *bxmessage.Broadcast, source connections.Conn) {
	startTime := time.Now()
	blockHash := broadcastMsg.Hash()
	bxBlock, missingShortIDsCount, err := g.blockProcessor.ProcessBroadcast(broadcastMsg)
	switch {
	case err == services.ErrAlreadyProcessed:
		source.Log().Debugf("received duplicate block %v, skipping", blockHash)
		g.stats.AddGatewayBlockEvent("GatewayProcessBlockFromBDNIgnoreSeen", source, blockHash, broadcastMsg.GetNetworkNum(), 1, startTime, 0, 0, len(broadcastMsg.Block()), len(broadcastMsg.ShortIDs()), 0, 0, bxBlock)
		return
	case err == services.ErrMissingShortIDs:
		if !g.isSyncWithRelay() {
			source.Log().Debugf("TxStore sync is in progress - Ignoring block %v from bdn with unknown %v shortIDs", blockHash, missingShortIDsCount)
			return
		}
		// TODO - list the missing shortIDs in trace.
		source.Log().Debugf("could not decompress block %v, missing shortIDs count: %v", blockHash, missingShortIDsCount)
		g.stats.AddGatewayBlockEvent("GatewayProcessBlockFromBDNRequiredRecovery", source, blockHash, broadcastMsg.GetNetworkNum(), 1, startTime, 0, 0, len(broadcastMsg.Block()), len(broadcastMsg.ShortIDs()), 0, missingShortIDsCount, bxBlock)
		return
	case err != nil:
		source.Log().Errorf("could not decompress block %v, err: %v", blockHash, err)
		broadcastBlockHex := hex.EncodeToString(broadcastMsg.Block())
		source.Log().Debugf("could not decompress block %v, err: %v, contents: %v", blockHash, err, broadcastBlockHex)
		return
	}

	source.Log().Infof("processing block %v from BDN, block number: %v, txs count: %v", blockHash, bxBlock.Number, len(bxBlock.Txs))
	g.processBlockFromBDN(bxBlock)
	g.stats.AddGatewayBlockEvent("GatewayProcessBlockFromBDN", source, blockHash, broadcastMsg.GetNetworkNum(), 1, startTime, 0, bxBlock.Size(), len(broadcastMsg.Block()), len(broadcastMsg.ShortIDs()), len(bxBlock.Txs), 0, bxBlock)
}

func (g *gateway) processTransaction(tx *bxmessage.Tx, source connections.Conn) {
	startTime := time.Now()
	networkDuration := startTime.Sub(tx.Timestamp()).Microseconds()
	sentToBlockchainNode := false
	sentToBDN := false
	var broadcastRes types.BroadcastResults
	// we add the transaction to TxStore with current time so we can measure time difference to node announcement/confirmation
	txResult := g.TxStore.Add(tx.Hash(), tx.Content(), tx.ShortID(), tx.GetNetworkNum(), false, tx.Flags(), g.clock.Now(), 0)
	eventName := "TxProcessedByGatewayFromPeerIgnoreSeen"
	if txResult.NewContent || txResult.NewSID || txResult.Reprocess {
		eventName = "TxProcessedByGatewayFromPeer"
	}

	if txResult.NewContent || txResult.Reprocess {
		if txResult.NewContent {
			newTxsNotification := types.CreateNewTransactionNotification(txResult.Transaction)
			g.notify(newTxsNotification)
			g.publishPendingTx(txResult.Transaction.Hash(), txResult.Transaction, source.Info().ConnectionType == utils.Blockchain)
		}

		if !source.Info().IsRelay() {
			if !txResult.Reprocess {
				g.bdnStats.LogNewTxFromNode(types.NodeEndpoint{IP: source.Info().PeerIP, Port: int(source.Info().PeerPort)})
			}

			paidTx := tx.Flags().IsPaid()
			allowed, behavior := g.burstLimiter.AllowTransaction(g.accountID, paidTx)
			if !allowed {
				if paidTx {
					g.bdnStats.LogBurstLimitedTransactionsPaid()
				} else {
					g.bdnStats.LogBurstLimitedTransactionsUnpaid()
				}
			}

			switch behavior {
			case sdnmessage.BehaviorBlock:
			case sdnmessage.BehaviorAlert:
				// disabled for now so users will not see any of these messages
				// if paidTx {
				//	source.Log().Warnf("account burst limits exceeded: your account is limited to %v paid txs per 5 seconds", g.burstLimiter.BurstLimit(g.accountID, paidTx))
				// } else {
				//	source.Log().Debugf("account burst limits exceeded: your account is limited to %v unpaid txs per 5 seconds", g.burstLimiter.BurstLimit(g.accountID, paidTx))
				// }
				fallthrough
			case sdnmessage.BehaviorNoAction:
				fallthrough
			default:
				allowed = true
			}

			tx.SetSender(txResult.Sender)

			if allowed {
				// set timestamp so relay can analyze communication delay
				tx.SetTimestamp(g.clock.Now())
				broadcastRes = g.broadcast(tx, source, utils.RelayTransaction)
				sentToBDN = true
			}
		}

		if source.Info().ConnectionType != utils.Blockchain {
			if (!g.BxConfig.BlocksOnly && tx.Flags()&types.TFDeliverToNode != 0) || g.BxConfig.AllTransactions {
				_ = g.bridge.SendTransactionsFromBDN([]*types.BxTransaction{txResult.Transaction})
				sentToBlockchainNode = true
				g.bdnStats.LogTxSentToNode()
			}
			if !txResult.Reprocess {
				g.bdnStats.LogNewTxFromBDN()
			}
		}

		g.txTrace.Log(tx.Hash(), source)
	} else if source.Info().ConnectionType == utils.Blockchain {
		g.bdnStats.LogDuplicateTxFromNode(types.NodeEndpoint{IP: source.Info().PeerIP, Port: int(source.Info().PeerPort), PublicKey: source.Info().PeerEnode})
	}

	log.Tracef("msgTx: from %v, hash %v, flags %v, new Tx %v, new content %v, new shortid %v, event %v, sentToBDN: %v, sentToBlockchainNode: %v, handling duration %v, sender %v, networkDuration %v", source, tx.Hash(), tx.Flags(), txResult.NewTx, txResult.NewContent, txResult.NewSID, eventName, sentToBDN, sentToBlockchainNode, time.Since(startTime), tx.Sender(), networkDuration)
	g.stats.AddTxsByShortIDsEvent(eventName, source, txResult.Transaction, tx.ShortID(), source.Info().NodeID, broadcastRes.RelevantPeers, broadcastRes.SentGatewayPeers, startTime, tx.GetPriority(), txResult.DebugData)
}

func (g *gateway) processBlockFromBlockchain(bxBlock *types.BxBlock, source connections.Blockchain) {
	startTime := time.Now()

	blockchainEndpoint := types.NodeEndpoint{IP: source.Info().PeerIP, Port: int(source.Info().PeerPort), PublicKey: source.Info().PeerEnode}
	g.bdnStats.LogNewBlockMessageFromNode(blockchainEndpoint)

	blockHash := bxBlock.Hash()
	// even though it is not from BDN, still sending this block in the feed in case the node sent the block first
	err := g.publishBlock(bxBlock, types.BDNBlocksFeed, &source)
	if err != nil {
		source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock.Hash(), err)
	}

	err = g.publishBlock(bxBlock, types.NewBlocksFeed, &source)
	if err != nil {
		source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock.Hash(), err)
	}

	broadcastMessage, usedShortIDs, err := g.blockProcessor.BxBlockToBroadcast(bxBlock, g.sdn.NetworkNum(), g.sdn.MinTxAge())
	if err == services.ErrAlreadyProcessed {
		source.Log().Debugf("received duplicate block %v, skipping", blockHash)
		g.stats.AddGatewayBlockEvent("GatewayReceivedBlockFromBlockchainNodeIgnoreSeen", source, blockHash, g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), 0, 0, len(bxBlock.Txs), len(usedShortIDs), bxBlock)
		return
	} else if err != nil {
		source.Log().Errorf("could not compress block: %v", err)
		return
	}

	// if not synced avoid sending to bdn (low compression rate block)
	if !g.isSyncWithRelay() {
		source.Log().Debugf("TxSync not completed. Not sending block %v to the bdn", bxBlock.Hash())
		return
	}

	source.Log().Debugf("compressed block from blockchain node: hash %v, compressed %v short IDs", bxBlock.Hash(), len(usedShortIDs))
	source.Log().Infof("propagating block %v from blockchain node to BDN, block number: %v, txs count: %v", bxBlock.Hash(), bxBlock.Number, len(bxBlock.Txs))

	_ = g.broadcast(broadcastMessage, source, utils.RelayBlock)

	g.bdnStats.LogNewBlockFromNode(source.NodeEndpoint())
	g.stats.AddGatewayBlockEvent("GatewayReceivedBlockFromBlockchainNode", source, blockHash, g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), int(broadcastMessage.Size()), len(broadcastMessage.ShortIDs()), len(bxBlock.Txs), len(usedShortIDs), bxBlock)
}

func (g *gateway) processBlockFromBDN(bxBlock *types.BxBlock) {
	err := g.bridge.SendBlockToNode(bxBlock)
	if err != nil {
		log.Errorf("unable to send block from BDN to node: %v", err)
	}
	g.bdnStats.LogNewBlockFromBDN()
	err = g.publishBlock(bxBlock, types.BDNBlocksFeed, nil)
	if err != nil {
		log.Errorf("Failed to publish BDN block with %v, block hash: %v, block height: %v", err, bxBlock.Hash(), bxBlock.Number)
	}
}

func (g *gateway) notify(notification types.Notification) {
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled {
		select {
		case g.feedChan <- notification:
		default:
			log.Warnf("gateway feed channel is full. Can't add %v without blocking. Ignoring hash %v", reflect.TypeOf(notification), notification.GetHash())
		}
	}
}

func (g gateway) handleMEVBundleMessage(mevBundle bxmessage.MEVBundle, source connections.Conn) {
	start := time.Now()
	event := "ignore seen"
	broadcastRes := types.BroadcastResults{}

	if g.seenMEVMinerBundles.SetIfAbsent(mevBundle.Hash().String(), time.Minute*30) {
		event = "broadcast"
		if source.Info().IsRelay() {
			if g.BxConfig.MEVMinerURI == "" {
				log.Warnf("received mevBundle message, but mev miner uri is empty. Message %v from %v in network %v", mevBundle.Hash(), mevBundle.SourceID(), mevBundle.GetNetworkNum())
				return
			}

			// we can override method name based on flag when gateway starts
			if mevBundle.Method == string(jsonrpc.RPCEthSendBundle) {
				mevBundle.Method = g.BxConfig.MevMinerSendBundleMethodName
			}
			if mevBundle.Method == string(jsonrpc.RPCEthSendMegaBundle) && !g.BxConfig.ProcessMegaBundle {
				source.Log().Debugf("received megaBundle message. Message %v from %v in network %v", mevBundle.Hash(), mevBundle.SourceID(), mevBundle.GetNetworkNum())
				return
			}

			// TODO: check the usage of mevBundle.ID and is it ok to set it to "1"
			mevBundle.ID, mevBundle.JSONRPC = "1", "2.0"
			mevRPC, err := json.Marshal(mevBundle)
			if err != nil {
				source.Log().Errorf("failed to create new mevBundle http request for: %v, hash: %v, %v", g.BxConfig.MEVMinerURI, mevBundle.Hash(), err)
				return
			}

			resp, err := g.mevClient.Post(g.BxConfig.MEVMinerURI, "application/json", bytes.NewReader(mevRPC))
			if err != nil {
				source.Log().Errorf("failed to perform mevBundle request for: %v, hash: %v, %v", g.BxConfig.MEVMinerURI, mevBundle.Hash(), err)
				return
			}

			defer resp.Body.Close()
			respBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				source.Log().Errorf("failed to read mevBundle response, hash: %v, %v", mevBundle.Hash(), err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				source.Log().Errorf("invalid mevBundle status code from: %v, hash: %v, code: %v", g.BxConfig.MEVMinerURI, mevBundle.Hash(), resp.StatusCode)
				return
			}

			source.Log().Tracef("%v mevBundle message hash %v to mev miner, duration: %v, response: %v", event, mevBundle.Hash(), time.Since(start).Milliseconds(), string(respBody))
			return
		}

		// sending message from mev-relay to BDN. Gateway set the hash and network that relays will use.
		broadcastRes = g.broadcast(&mevBundle, source, utils.RelayTransaction)
	}

	source.Log().Tracef("%v mevBundle message msg: %v in network %v to relays, result: [%v], duration: %v", event, mevBundle.Hash(), mevBundle.GetNetworkNum(), broadcastRes, time.Since(start).Milliseconds())
}

// TODO: think about remove code duplication and merge this func with handleMEVBundleMessage
func (g gateway) handleMEVSearcherMessage(mevSearcher bxmessage.MEVSearcher, source connections.Conn) {
	start := time.Now()
	event := "ignore seen"
	broadcastRes := types.BroadcastResults{}

	if g.seenMEVSearchers.SetIfAbsent(mevSearcher.Hash().String(), time.Minute*30) {
		event = "broadcast"
		if source.Info().IsRelay() {
			if g.BxConfig.MEVBuilderURI == "" {
				log.Warnf("received mevSearcher message, but mev-builder-uri is empty. Message %v from %v in network %v", mevSearcher.Hash(), mevSearcher.SourceID(), mevSearcher.GetNetworkNum())
				return
			}

			mevSearcher.ID, mevSearcher.JSONRPC = "1", "2.0"
			mevRPC, err := json.Marshal(mevSearcher)
			if err != nil {
				log.Errorf("failed to marshal mevSearcher payload for: %v, hash: %v, params: %v, error: %v", g.BxConfig.MEVBuilderURI, mevSearcher.Hash(), string(mevSearcher.Params), err)
				return
			}

			req, err := http.NewRequest(http.MethodPost, g.BxConfig.MEVBuilderURI, bytes.NewReader(mevRPC))
			if err != nil {
				log.Errorf("failed to create new mevSearcher http request for: %v, hash: %v, %v", g.BxConfig.MEVBuilderURI, mevSearcher.Hash(), err)
				return
			}
			req.Header.Add("Content-Type", "application/json")

			// For this case we always have only 1 element
			var mevSearcherAuth string
			for _, auth := range mevSearcher.Auth() {
				mevSearcherAuth = auth
				break
			}

			req.Header.Add(flasbotAuthHeader, mevSearcherAuth)
			resp, err := g.mevClient.Do(req)
			if err != nil {
				log.Errorf("failed to perform mevSearcher request for: %v, hash: %v, %v", g.BxConfig.MEVBuilderURI, mevSearcher.Hash(), err)
				return
			}

			defer resp.Body.Close()
			respBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				source.Log().Errorf("failed to read mevSearcher response, hash: %v, %v", mevSearcher.Hash(), err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				log.Errorf("invalid mevSearcher status code from: %v, hash: %v, code: %v", g.BxConfig.MEVBuilderURI, mevSearcher.Hash(), resp.StatusCode)
				return
			}

			source.Log().Tracef("%v mevSearcher message hash %v to mev relay, duration: %v, response: %v", event, mevSearcher.Hash(), time.Since(start).Milliseconds(), string(respBody))
			return
		}

		// sending message from searcher to BDN. Gateway set the hash and network that relays will use.
		broadcastRes = g.broadcast(&mevSearcher, source, utils.RelayTransaction)
	}

	source.Log().Tracef("%v mevSearcher message msg: %v in network %v to relays, result: [%v], duration: %v", event, mevSearcher.Hash(), mevSearcher.GetNetworkNum(), broadcastRes, time.Since(start).Milliseconds())
}

func (g *gateway) Peers(ctx context.Context, req *pb.PeersRequest) (*pb.PeersReply, error) {
	return g.Bx.Peers(ctx, req)
}

const (
	connectionStatusConnected    = "connected"
	connectionStatusNotConnected = "not_connected"
)

func (g *gateway) Version(_ context.Context, _ *pb.VersionRequest) (*pb.VersionReply, error) {
	resp := &pb.VersionReply{
		Version:   version.BuildVersion,
		BuildDate: version.BuildDate,
	}
	return resp, nil
}

const bdn = "BDN"

func (g *gateway) Status(context.Context, *pb.StatusRequest) (*pb.StatusResponse, error) {
	var bdnConn = func() map[string]*pb.BDNConnStatus {
		g.ConnectionsLock.RLock()
		defer g.ConnectionsLock.RUnlock()

		var mp = make(map[string]*pb.BDNConnStatus)
		for _, conn := range g.Connections {
			connInfo := conn.Info()

			if connInfo.ConnectionType&utils.Relay == 0 {
				continue
			}

			var connectionLatency *pb.ConnectionLatency
			if bxConn, ok := conn.(*handler.BxConn); ok {
				minMsFromPeer, minMsToPeer, slowTrafficCount, minMsRoundTrip := bxConn.GetMinLatencies()
				connectionLatency = &pb.ConnectionLatency{
					MinMsFromPeer:    minMsFromPeer,
					MinMsToPeer:      minMsToPeer,
					SlowTrafficCount: slowTrafficCount,
					MinMsRoundTrip:   minMsRoundTrip,
				}
			}

			if !conn.IsOpen() {
				mp[connInfo.PeerIP] = &pb.BDNConnStatus{
					Status: connectionStatusNotConnected,
				}

				continue
			}

			mp[connInfo.PeerIP] = &pb.BDNConnStatus{
				Status:      connectionStatusConnected,
				ConnectedAt: connInfo.ConnectedAt.Format(time.RFC3339),
				Latency:     connectionLatency,
			}
		}

		if len(mp) == 0 {
			// set "BDN: NOT_CONNECTED" in case of missing connections to any relay
			mp[bdn] = &pb.BDNConnStatus{
				Status: connectionStatusNotConnected,
			}
		}

		return mp
	}

	var nodeConn = func() map[string]*pb.NodeConnStatus {
		err := g.bridge.SendBlockchainStatusRequest()
		if err != nil {
			log.Errorf("failed to send blockchain status request: %v", err)
			return nil
		}

		var wsProviders = g.wsManager.Providers()

		select {
		case status := <-g.bridge.ReceiveBlockchainStatusResponse():
			var mp = make(map[string]*pb.NodeConnStatus)
			var nodeStats = g.bdnStats.NodeStats()
			for _, peer := range g.blockchainPeers {
				connStatus := &pb.NodeConnStatus{
					ConnStatus: connectionStatusNotConnected,
				}

				for _, peerStatus := range status {
					if peer.IP != peerStatus.IP {
						continue
					}

					connStatus = &pb.NodeConnStatus{
						ConnStatus: connectionStatusConnected,
					}

					nstat, ok := nodeStats[peer.IPPort()]
					if ok {
						connStatus.NodePerformance = &pb.NodePerformance{
							Since:                                   g.bdnStats.StartTime().Format(time.RFC3339),
							NewBlocksReceivedFromBlockchainNode:     uint32(nstat.NewBlocksReceivedFromBlockchainNode),
							NewBlocksReceivedFromBdn:                uint32(nstat.NewBlocksReceivedFromBdn),
							NewBlocksSeen:                           nstat.NewBlocksSeen,
							NewBlockMessagesFromBlockchainNode:      nstat.NewBlockMessagesFromBlockchainNode,
							NewBlockAnnouncementsFromBlockchainNode: nstat.NewBlockAnnouncementsFromBlockchainNode,
							NewTxReceivedFromBlockchainNode:         nstat.NewTxReceivedFromBlockchainNode,
							NewTxReceivedFromBdn:                    nstat.NewTxReceivedFromBdn,
							TxSentToNode:                            nstat.TxSentToNode,
							DuplicateTxFromNode:                     nstat.DuplicateTxFromNode,
						}
					}

					wsPeer, ok := wsProviders[peer.IPPort()]
					if !ok {
						continue
					}

					connStatus.WsConnection = &pb.WsConnStatus{
						Addr: wsPeer.Addr(),
						ConnStatus: func() string {
							if wsPeer.IsOpen() {
								return connectionStatusConnected
							}
							return connectionStatusNotConnected
						}(),
						SyncStatus: strings.ToLower(string(wsPeer.SyncStatus())),
					}

					break
				}

				mp[ipport(peer.IP, peer.Port)] = connStatus
			}

			return mp
		case <-time.After(time.Second):
			log.Errorf("no blockchain status response from backend within 1sec timeout")
			return nil
		}
	}

	var (
		nodeModel    = g.sdn.NodeModel()
		accountModel = g.sdn.AccountModel()
	)

	rsp := &pb.StatusResponse{
		GatewayInfo: &pb.GatewayInfo{
			Version:       version.BuildVersion,
			NodeId:        string(nodeModel.NodeID),
			IpAddress:     nodeModel.ExternalIP,
			TimeStarted:   g.timeStarted.Format(time.RFC3339),
			Continent:     nodeModel.Continent,
			Country:       nodeModel.Country,
			Network:       nodeModel.Network,
			StartupParams: strings.Join(os.Args[1:], " "),
		},
		Nodes:  nodeConn(),
		Relays: bdnConn(),
		AccountInfo: &pb.AccountInfo{
			AccountId:  string(accountModel.AccountID),
			ExpireDate: accountModel.ExpireDate,
		},
	}

	return rsp, nil
}

func ipport(ip string, port int) string { return fmt.Sprintf("%s:%d", ip, port) }

func (g *gateway) Subscriptions(_ context.Context, _ *pb.SubscriptionsRequest) (*pb.SubscriptionsReply, error) {
	return g.feedManager.GetGrpcSubscriptionReply(), nil
}

func (g *gateway) BlxrTx(_ context.Context, req *pb.BlxrTxRequest) (*pb.BlxrTxReply, error) {
	tx := bxmessage.Tx{}
	tx.SetTimestamp(time.Now())

	txContent, err := types.DecodeHex(req.GetTransaction())
	if err != nil {
		log.Errorf("failed to decode transaction %v sent via GRPC blxrtx: %v", req.GetTransaction(), err)
		return &pb.BlxrTxReply{}, err
	}
	tx.SetContent(txContent)

	hashAsByteArr := crypto.Keccak256(txContent)
	var hash types.SHA256Hash
	copy(hash[:], hashAsByteArr)
	tx.SetHash(hash)

	// TODO: take the account ID from the authentication meta data
	tx.SetAccountID(g.sdn.NodeModel().AccountID)
	tx.SetNetworkNum(g.sdn.NetworkNum())

	grpc := connections.NewRPCConn(g.accountID, "", g.sdn.NetworkNum(), utils.GRPC)
	g.HandleMsg(&tx, grpc, connections.RunForeground)
	return &pb.BlxrTxReply{TxHash: tx.Hash().String()}, nil
}

func (g *gateway) sendStatsOnInterval(interval time.Duration) {
	ticker := g.clock.Ticker(interval)
	for {
		select {
		case <-ticker.Alert():
			rss, err := utils.GetAppMemoryUsage()
			if err != nil {
				log.Tracef("Failed to get Process RSS size: %v", err)
			}
			g.bdnStats.SetMemoryUtilization(rss)

			closedIntervalBDNStatsMsg := g.bdnStats.CloseInterval()

			broadcastRes := g.broadcast(&closedIntervalBDNStatsMsg, nil, utils.Relay)

			closedIntervalBDNStatsMsg.Log()
			log.Tracef("sent bdnStats msg to relays, result: [%v]", broadcastRes)
		}
	}
}
