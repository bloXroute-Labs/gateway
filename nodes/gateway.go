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
	pb "github.com/bloXroute-Labs/gateway/protobuf"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/servers"
	"github.com/bloXroute-Labs/gateway/services"
	baseservices "github.com/bloXroute-Labs/gateway/services"
	"github.com/bloXroute-Labs/gateway/services/loggers"
	"github.com/bloXroute-Labs/gateway/services/statistics"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/bloXroute-Labs/gateway/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	log "github.com/sirupsen/logrus"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
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
	timeToAvoidReEntry = 30 * time.Minute
)

type gateway struct {
	Bx
	pb.UnimplementedGatewayServer
	context context.Context
	cancel  context.CancelFunc

	sdn                *connections.SDNHTTP
	accountID          types.AccountID
	bridge             blockchain.Bridge
	feedChan           chan types.Notification
	asyncMsgChannel    chan services.MsgInfo
	bdnStats           *bxmessage.BdnPerformanceStats
	blockProcessor     services.BlockProcessor
	pendingTxs         services.HashHistory
	possiblePendingTxs services.HashHistory
	txTrace            loggers.TxTrace
	blockchainPeers    []types.NodeEndpoint
	stats              statistics.Stats
	bdnBlocks          services.HashHistory
	wsProvider         blockchain.WSProvider
	syncedWithRelay    atomic.Bool

	bestBlockHeight     int
	bdnBlocksSkipCount  int
	seenMEVMinerBundles baseservices.HashHistory
	seenMEVSearchers    baseservices.HashHistory
}

// NewGateway returns a new gateway node to send messages from a blockchain node to the relay network
func NewGateway(parent context.Context, bxConfig *config.Bx, bridge blockchain.Bridge, ws blockchain.WSProvider, blockchainPeers []types.NodeEndpoint) (Node, error) {
	ctx, cancel := context.WithCancel(parent)

	g := &gateway{
		Bx:                  NewBx(bxConfig, "datadir"),
		bridge:              bridge,
		wsProvider:          ws,
		context:             ctx,
		cancel:              cancel,
		blockchainPeers:     blockchainPeers,
		pendingTxs:          services.NewHashHistory("pendingTxs", 15*time.Minute),
		possiblePendingTxs:  services.NewHashHistory("possiblePendingTxs", 15*time.Minute),
		bdnBlocks:           services.NewHashHistory("bdnBlocks", 15*time.Minute),
		seenMEVMinerBundles: baseservices.NewHashHistory("mevMinerBundle", 30*time.Minute),
		seenMEVSearchers:    baseservices.NewHashHistory("mevSearcher", 30*time.Minute),
	}
	g.asyncMsgChannel = services.NewAsyncMsgChannel(g)

	// create tx store service pass to eth client
	assigner := services.NewEmptyShortIDAssigner()
	txStore := services.NewBxTxStore(30*time.Minute, 3*24*time.Hour, 10*time.Minute, assigner, services.NewHashHistory("seenTxs", 30*time.Minute), nil, timeToAvoidReEntry)
	g.TxStore = &txStore
	g.bdnStats = bxmessage.NewBDNStats()
	g.blockProcessor = services.NewRLPBlockProcessor(g.TxStore)

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

	privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile := utils.GetCertDir(g.BxConfig.RegistrationCertDir, privateCertDir, strings.ToLower(gatewayType.String()))
	sslCerts := utils.NewSSLCertsFromFiles(privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile)
	if g.accountID, err = sslCerts.GetAccountID(); err != nil {
		return err
	}
	log.Infof("ssl certificate is successfully loaded")

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
		ExternalIP:      g.BxConfig.ExternalIP,
		ExternalPort:    g.BxConfig.ExternalPort,
		BlockchainIP:    blockchainPeerEndpoint.IP,
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

	if g.BxConfig.MevBuilderURI != "" && g.sdn.AccountModel().MevBuilder == "" {
		panic(fmt.Errorf("account %v is not allowed for mev builder service, closing the gateway. Please contact support@bloxroute.com to enable running as mev builder", g.sdn.AccountModel().AccountID))
	}

	if g.BxConfig.MevMinerURI != "" && g.sdn.AccountModel().MevMiner == "" {
		panic(fmt.Errorf(
			"account %v is not allowed for mev miner service, closing the gateway. Please contact support@bloxroute.com to enable running as mev miner",
			g.sdn.AccountModel().AccountID,
		))
	}

	var txTraceLogger *log.Logger = nil
	if g.BxConfig.TxTrace.Enabled {
		txTraceLogger, err = CreateCustomLogger(
			g.BxConfig.AppName,
			int(g.BxConfig.ExternalPort),
			"txtrace",
			g.BxConfig.TxTrace.MaxFileSize,
			g.BxConfig.TxTrace.MaxBackupFiles,
			1,
			log.TraceLevel,
		)
		if err != nil {
			return fmt.Errorf("failed to create TxTrace logger: %v", err)
		}
	}
	g.txTrace = loggers.NewTxTrace(txTraceLogger)

	if g.sdn.AccountTier() != sdnmessage.ATierElite && len(g.blockchainPeers) == 0 {
		panic("No blockchain node specified. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
	}

	networkNum := g.sdn.NetworkNum()
	relayHost, relayPort, err := g.sdn.BestRelay(g.BxConfig)
	if err != nil {
		return err
	}

	err = g.pushBlockchainConfig()
	if err != nil {
		return fmt.Errorf("could process initial blockchain configuration: %v", err)
	}
	group.Go(g.handleBridgeMessages)
	group.Go(g.TxStore.Start)

	g.feedChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	accountModel := g.sdn.AccountModel()
	feedProvider := servers.NewFeedManager(g.context, g, g.feedChan,
		fmt.Sprintf(":%v", g.BxConfig.WebsocketPort), networkNum,
		g.wsProvider, g.BxConfig.ManageWSServer, accountModel, g.sdn.GetCustomerAccountModel,
		g.BxConfig.WebsocketTLSEnabled, privateCertFile, privateKeyFile)
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled {
		group.Go(feedProvider.Start)
	}

	usePQ := g.BxConfig.PrioritySending
	log.Infof("gateway %v (%v) starting, connecting to relay %v:%v", g.sdn.NodeID, g.BxConfig.Environment, relayHost, relayPort)
	relay := handler.NewOutboundRelay(g,
		&sslCerts, relayHost, relayPort, g.sdn.NodeID, utils.Relay, usePQ, &g.sdn.Networks, true, false, utils.RealClock{}, false)
	relay.SetNetworkNum(networkNum)

	if err := InitFluentD(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, g.sdn.NodeID); err != nil {
		return err
	}

	g.stats = statistics.NewStats(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, g.sdn.NodeID, &g.sdn.Networks, g.BxConfig.LogNetworkContent)

	go g.PingLoop()

	group.Go(relay.Start)

	go g.sendStatsOnInterval(15*time.Minute, relay.BxConn)

	if g.BxConfig.Enabled {
		grpcServer := newGatewayGRPCServer(g, g.BxConfig.Host, g.BxConfig.Port, g.BxConfig.User, g.BxConfig.Password)
		group.Go(grpcServer.Start)
	}

	err = group.Wait()
	if err != nil {
		return err
	}

	return nil
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
	ethConfig := network.EthConfig{
		Network:            uint64(blockchainAttributes.NetworkID),
		TotalDifficulty:    td,
		Head:               genesis,
		Genesis:            genesis,
		IgnoreBlockTimeout: ignoreBlockTimeout,
	}

	return g.bridge.UpdateNetworkConfig(ethConfig)
}

func (g *gateway) publishBlock(bxBlock *types.BxBlock, feedName types.FeedType) error {
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
	log.Debugf("Received block for %v, block hash: %v, block height: %v, notify the feed", feedName, bxBlock.Hash(), bxBlock.Number)
	g.notify(blockNotification)
	if feedName == types.NewBlocksFeed {
		g.bestBlockHeight = blockHeight
		g.bdnBlocksSkipCount = 0

		onBlockNotification := *blockNotification
		onBlockNotification.SetNotificationType(types.OnBlockFeed)
		log.Debugf("Received block for %v, block hash: %v, block height: %v, notify the feed", types.OnBlockFeed, bxBlock.Hash(), bxBlock.Number)
		g.notify(&onBlockNotification)

		txReceiptNotification := *blockNotification
		txReceiptNotification.SetNotificationType(types.TxReceiptsFeed)
		log.Debugf("Received block for %v, block hash: %v, block height: %v, notify the feed", types.TxReceiptsFeed, bxBlock.Hash(), bxBlock.Number)
		g.notify(&txReceiptNotification)
	}
	return nil
}

func (g *gateway) publishPendingTx(txHash types.SHA256Hash, bxTx *types.BxTransaction, fromNode bool) {
	if g.pendingTxs.Exists(txHash.String()) {
		return
	}

	if fromNode || g.possiblePendingTxs.Exists(txHash.String()) {
		if bxTx != nil {
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
				if !exists {
					log.Tracef("msgTx: from Blockchain, hash %v, event TxAnnouncedByBlockchainNode, peerID: %v", hash, txAnnouncement.PeerID)
					requests = append(requests, hash)
				} else {
					log.Tracef("msgTx: from Blockchain, hash %v, event TxAnnouncedByBlockchainNodeIgnoreSeen, peerID: %v", hash, txAnnouncement.PeerID)
				}
				g.publishPendingTx(hash, bxTx, true)
			}

			if len(requests) > 0 && txAnnouncement.PeerID != bxgateway.WSConnectionID {
				err = g.bridge.RequestTransactionsFromNode(txAnnouncement.PeerID, requests)
				if err == blockchain.ErrChannelFull {
					log.Warnf("transaction requests channel is full, skipping request for %v hashes", len(requests))
				} else if err != nil {
					log.Errorf("could not request transactions over bridge: %v", err)
					return err
				}
			}
		case blockFromNode := <-g.bridge.ReceiveBlockFromNode():
			blockchainConnection := connections.NewBlockchainConn(g.blockchainPeers[0])
			g.processBlockFromBlockchain(blockFromNode.Block, blockchainConnection)
		case _ = <-g.bridge.ReceiveNoActiveBlockchainPeersAlert():
			if g.sdn.AccountTier() != sdnmessage.ATierElite {
				panic("Gateway does not have an active blockchain connection. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
			}
		}
	}
}

func (g *gateway) NodeStatus() connections.NodeStatus {
	var capabilities types.CapabilityFlags
	if g.BxConfig.MevBuilderURI != "" {
		capabilities |= types.CapabilityMevBuilder
	}

	if g.BxConfig.MevMinerURI != "" {
		capabilities |= types.CapabilityMevMiner
	}

	return connections.NodeStatus{
		Capabilities: capabilities,
		Version:      version.BuildVersion,
	}
}

func (g *gateway) HandleMsg(msg bxmessage.Message, source connections.Conn, background connections.MsgHandlingOptions) error {
	startTime := time.Now()
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
		blockHash := broadcastMsg.Hash()

		bxBlock, missingShortIDsCount, err := g.blockProcessor.ProcessBroadcast(broadcastMsg)
		switch {
		case err == services.ErrAlreadyProcessed:
			source.Log().Debugf("received duplicate block %v, skipping", blockHash)
			g.stats.AddGatewayBlockEvent("GatewayProcessBlockFromBDNIgnoreSeen", source, blockHash, broadcastMsg.GetNetworkNum(), 1, startTime, 0, len(broadcastMsg.Block()), 0, len(broadcastMsg.ShortIDs()), 0, 0, bxBlock)
			return nil
		case err == services.ErrMissingShortIDs:
			if !g.isSyncWithRelay() {
				source.Log().Debugf("TxStore sync is in progress - Ignoring block %v from bdn with unknown %v shortIDs", blockHash, missingShortIDsCount)
				return nil
			}
			// TODO - list the missing shortIDs in trace.
			source.Log().Debugf("could not decompress block %v, missing shortIDs count: %v", blockHash, missingShortIDsCount)
			g.stats.AddGatewayBlockEvent("GatewayProcessBlockFromBDNRequiredRecovery", source, blockHash, broadcastMsg.GetNetworkNum(), 1, startTime, 0, len(broadcastMsg.Block()), 0, len(broadcastMsg.ShortIDs()), 0, missingShortIDsCount, bxBlock)
			return nil
		case err != nil:
			source.Log().Errorf("could not decompress block %v, err: %v", blockHash, err)
			broadcastBlockHex := hex.EncodeToString(broadcastMsg.Block())
			source.Log().Debugf("could not decompress block %v, err: %v, contents: %v", blockHash, err, broadcastBlockHex)
			return nil
		}

		source.Log().Infof("processing block %v from BDN, block number: %v, txs count: %v", blockHash, bxBlock.Number, len(bxBlock.Txs))
		g.processBlockFromBDN(bxBlock)
		// TODO decompress should not be 0 - calculate it in the BxBlock struct or add the original size in the broadcast msg
		g.stats.AddGatewayBlockEvent("GatewayProcessBlockFromBDN", source, blockHash, broadcastMsg.GetNetworkNum(), 1, startTime, 0, len(broadcastMsg.Block()), 0, len(broadcastMsg.ShortIDs()), len(bxBlock.Txs), 0, bxBlock)
	case *bxmessage.RefreshBlockchainNetwork:
		go g.sdn.GetBlockchainNetwork()
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

		if errorNotification.ErrorType == types.ErrorTypeTemporary {
			panic(fmt.Errorf("gateway received temporary error notification from relay %v with error %v", source, errorNotification.Reason))
		} else if errorNotification.ErrorType == types.ErrorTypePermanent {
			log.Errorf("received permanent error notification from relay %v, with error %v. closing the connection", source, errorNotification.Reason)
			// TODO should also close the gateway while notify the bridge and other go routine (web socket server, ...)
			os.Exit(0)
		}

	default:
		err = g.Bx.HandleMsg(msg, source)
	}
	return err
}

func (g *gateway) processTransaction(tx *bxmessage.Tx, source connections.Conn) {
	startTime := time.Now()
	sentToBlockchainNode := false
	sentToBDN := false
	var broadcastRes types.BroadcastResults
	txResult := g.TxStore.Add(tx.Hash(), tx.Content(), tx.ShortID(), tx.GetNetworkNum(), false, tx.Flags(), tx.Timestamp(), 0)
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
			broadcastRes = g.broadcast(tx, source, utils.RelayTransaction)
			sentToBDN = true
			if !txResult.Reprocess {
				g.bdnStats.LogNewTxFromNode(types.NodeEndpoint{IP: source.Info().PeerIP, Port: int(source.Info().PeerPort)})
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

	log.Tracef("msgTx: from %v, hash %v, flags %v, new Tx %v, new content %v, new shortid %v, event %v, sentToBDN: %v, sentToBlockchainNode: %v, handling duration %v", source, tx.Hash(), tx.Flags(), txResult.NewTx, txResult.NewContent, txResult.NewSID, eventName, sentToBDN, sentToBlockchainNode, time.Now().Sub(startTime))
	g.stats.AddTxsByShortIDsEvent(eventName, source, txResult.Transaction, tx.ShortID(), source.Info().NodeID, broadcastRes.RelevantPeers, broadcastRes.SentGatewayPeers, startTime, tx.GetPriority(), txResult.DebugData)
}

func (g *gateway) processBlockFromBlockchain(bxBlock *types.BxBlock, source connections.Blockchain) {
	startTime := time.Now()

	blockchainEndpoint := types.NodeEndpoint{IP: source.Info().PeerIP, Port: int(source.Info().PeerPort), PublicKey: source.Info().PeerEnode}
	g.bdnStats.LogNewBlockMessageFromNode(blockchainEndpoint)

	blockHash := bxBlock.Hash()
	// even though it is not from BDN, still sending this block in the feed in case the node sent the block first
	err := g.publishBlock(bxBlock, types.BDNBlocksFeed)
	if err != nil {
		source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock.Hash(), err)
	}

	err = g.publishBlock(bxBlock, types.NewBlocksFeed)
	if err != nil {
		source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock.Hash(), err)
	}

	broadcastMessage, usedShortIDs, err := g.blockProcessor.BxBlockToBroadcast(bxBlock, g.sdn.NetworkNum(), g.sdn.GetMinTxAge())
	if err == services.ErrAlreadyProcessed {
		source.Log().Debugf("received duplicate block %v, skipping", blockHash)
		// TODO same as line 378 - should calculate BxBlock size
		g.stats.AddGatewayBlockEvent("GatewayReceivedBlockFromBlockchainNodeIgnoreSeen", source, blockHash, g.sdn.NetworkNum(), 1, startTime, 0, 0, 0, 0, len(bxBlock.Txs), len(usedShortIDs), bxBlock)
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

	g.bdnStats.LogNewBlockFromNode(g.blockchainPeers[0])
	// TODO same as line 378 - should calculate BxBlock size
	g.stats.AddGatewayBlockEvent("GatewayReceivedBlockFromBlockchainNode", source, blockHash, g.sdn.NetworkNum(), 1, startTime, 0, 0, int(broadcastMessage.Size()), len(broadcastMessage.ShortIDs()), len(bxBlock.Txs), len(usedShortIDs), bxBlock)
}

func (g *gateway) processBlockFromBDN(bxBlock *types.BxBlock) {
	err := g.bridge.SendBlockToNode(bxBlock)
	if err != nil {
		log.Errorf("unable to send block from BDN to node: %v", err)
	}
	g.bdnStats.LogNewBlockFromBDN()
	err = g.publishBlock(bxBlock, types.BDNBlocksFeed)
	if err != nil {
		log.Errorf("Failed to publish BDN block with %v, block hash: %v, block height: %v", err, bxBlock.Hash(), bxBlock.Number)
	}
}

func (g *gateway) notify(notification types.Notification) {
	if g.BxConfig.WebsocketEnabled {
		select {
		case g.feedChan <- notification:
		default:
			log.Warnf("gateway feed channel is full. Can't add %v without blocking. Ignoring hash %v", reflect.TypeOf(notification), notification.GetHash())
		}
	}
}

func (g gateway) handleMEVBundleMessage(mevBundle bxmessage.MEVBundle, source connections.Conn) {
	if source.Info().IsRelay() {
		if g.BxConfig.MevMinerURI == "" {
			log.Warnf("received mevBundle message, but mev miner uri is empty. Message %v from %v in network %v", mevBundle.Hash(), mevBundle.SourceID(), mevBundle.GetNetworkNum())
			return
		}
		if g.seenMEVMinerBundles.SetIfAbsent(mevBundle.Hash().String(), time.Minute*30) {
			mevBundle.ID, mevBundle.JSONRPC = "1", "2.0"
			mevRPC, err := json.Marshal(mevBundle)
			if err != nil {
				log.Errorf("failed to marshal mevBundle payload for: %v, hash: %v, params: %v, error: %v", g.BxConfig.MevMinerURI, mevBundle.Hash(), string(mevBundle.Params), err)
				return
			}

			httpClient := http.Client{}
			defer httpClient.CloseIdleConnections()

			req, err := http.NewRequest(http.MethodPost, g.BxConfig.MevMinerURI, bytes.NewReader(mevRPC))
			if err != nil {
				log.Errorf("failed to create new mevBundle http request for: %v, hash: %v, %v", g.BxConfig.MevMinerURI, mevBundle.Hash(), err)
				return
			}
			req.Header.Add("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			if err != nil {
				log.Errorf("failed to perform mevBundle request for: %v, hash: %v, %v", g.BxConfig.MevMinerURI, mevBundle.Hash(), err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				log.Errorf("invalid mevBundle status code from: %v, hash: %v,code: %v", g.BxConfig.MevMinerURI, mevBundle.Hash(), resp.StatusCode)
				return
			}
		}
		return
	}

	mevBundle.SetHash()
	mevBundle.SetNetworkNum(g.sdn.NetworkNum())
	broadcastRes := g.broadcast(&mevBundle, source, utils.RelayTransaction)
	log.Tracef("broadcast mevBundle message msg: %v in network %v to relays, result: [%v]", mevBundle.Hash(), mevBundle.GetNetworkNum(), broadcastRes)
}

// TODO: think about remove code duplication and merge this func with handleMEVBundleMessage
func (g gateway) handleMEVSearcherMessage(mevSearcher bxmessage.MEVSearcher, source connections.Conn) {
	if source.Info().IsRelay() {
		if g.BxConfig.MevBuilderURI == "" {
			log.Warnf("received mevSearcher message, but mev-builder-uri is empty. Message %v from %v in network %v", mevSearcher.Hash(), mevSearcher.SourceID(), mevSearcher.GetNetworkNum())
			return
		}
		if g.seenMEVSearchers.SetIfAbsent(mevSearcher.Hash().String(), time.Minute*30) {
			mevSearcher.ID, mevSearcher.JSONRPC = "1", "2.0"
			mevRPC, err := json.Marshal(mevSearcher)
			if err != nil {
				log.Errorf("failed to marshal mevSearcher payload for: %v, hash: %v, params: %v, error: %v", g.BxConfig.MevBuilderURI, mevSearcher.Hash(), string(mevSearcher.Params), err)
				return
			}

			httpClient := http.Client{}
			defer httpClient.CloseIdleConnections()

			req, err := http.NewRequest(http.MethodPost, g.BxConfig.MevBuilderURI, bytes.NewReader(mevRPC))
			if err != nil {
				log.Errorf("failed to create new mevSearcher http request for: %v, hash: %v, %v", g.BxConfig.MevBuilderURI, mevSearcher.Hash(), err)
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
			resp, err := httpClient.Do(req)
			if err != nil {
				log.Errorf("failed to perform mevSearcher request for: %v, hash: %v, %v", g.BxConfig.MevBuilderURI, mevSearcher.Hash(), err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				log.Errorf("invalid mevSearcher status code from: %v, hash: %v,code: %v", g.BxConfig.MevBuilderURI, mevSearcher.Hash(), resp.StatusCode)
				return
			}
		}
		return
	}

	mevSearcher.SetHash()
	mevSearcher.SetNetworkNum(g.sdn.NetworkNum())
	broadcastRes := g.broadcast(&mevSearcher, source, utils.RelayTransaction)
	log.Tracef("broadcast mevSearcher message msg: %v in network %v to relays, result: [%v]", mevSearcher.Hash(), mevSearcher.GetNetworkNum(), broadcastRes)
}

func (g *gateway) Peers(ctx context.Context, req *pb.PeersRequest) (*pb.PeersReply, error) {
	return g.Bx.Peers(ctx, req)
}

func (g *gateway) Version(_ context.Context, _ *pb.VersionRequest) (*pb.VersionReply, error) {
	resp := &pb.VersionReply{
		Version:   version.BuildVersion,
		BuildDate: version.BuildDate,
	}
	return resp, nil
}

func (g *gateway) BlxrTx(_ context.Context, req *pb.BlxrTxRequest) (*pb.BlxrTxReply, error) {
	tx := bxmessage.Tx{}
	tx.SetTimestamp(time.Now())

	txContent, err := hex.DecodeString(req.GetTransaction())
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

func (g *gateway) sendStatsOnInterval(interval time.Duration, relayConn *handler.BxConn) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			rss, err := utils.GetAppMemoryUsage()
			if err != nil {
				log.Tracef("Failed to get Process RSS size: %v", err)
			}
			g.bdnStats.SetMemoryUtilization(rss)

			closedIntervalBDNStatsMsg := g.bdnStats.CloseInterval()
			err = relayConn.Send(&closedIntervalBDNStatsMsg)
			if err != nil {
				log.Debugf("failed to send BDN performance stats: %v", err)
			}
			closedIntervalBDNStatsMsg.Log()
		}
	}
}
