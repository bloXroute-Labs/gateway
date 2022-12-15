package nodes

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"net/http"
	"os"
	"path"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/sirupsen/logrus"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/loggers"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/atomic"
)

const (
	flashbotAuthHeader = "X-Flashbots-Signature"
	timeToAvoidReEntry = 6 * time.Hour
)

type gateway struct {
	Bx
	pb.UnimplementedGatewayServer
	context context.Context
	cancel  context.CancelFunc

	sslCerts           *utils.SSLCerts
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

	mevClient        *http.Client
	gatewayPeers     string
	gatewayPublicKey string

	staticEnodesCount  int
	startupArgs        string
	validatorStatusMap *sync.Map              // validator addr -> online/offline
	validatorListMap   *sync.Map              // block height -> list of validators
	nextValidatorMap   *orderedmap.OrderedMap // next accessible validator
	validatorListReady bool
}

// GeneratePeers generate string peers separated by coma
func GeneratePeers(peersInfo []network.PeerInfo) string {
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
	blockchainPeers []types.NodeEndpoint, peersInfo []network.PeerInfo, gatewayPublicKeyStr string, sdn connections.SDNHTTP,
	sslCerts *utils.SSLCerts, staticEnodesCount int) (Node, error) {
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
		gatewayPeers:          GeneratePeers(peersInfo),
		gatewayPublicKey:      gatewayPublicKeyStr,
		staticEnodesCount:     staticEnodesCount,
		sdn:                   sdn,
		sslCerts:              sslCerts,
	}

	if bxConfig.BlockchainNetwork == bxgateway.BSCMainnet { // initialize validator map only for BSC now
		g.validatorListMap = &sync.Map{}
		g.validatorStatusMap = &sync.Map{}
		g.nextValidatorMap = orderedmap.New()
		g.validatorListReady = false
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
	g.bdnStats = bxmessage.NewBDNStats(blockchainPeers)
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

func (g *gateway) setupTxStore() {
	assigner := services.NewEmptyShortIDAssigner()
	g.TxStore = services.NewEthTxStore(g.clock, 30*time.Minute, 3*24*time.Hour, 10*time.Minute,
		assigner, services.NewHashHistory("seenTxs", 30*time.Minute), nil, *g.sdn.Networks(), services.NoOpBloomFilter{})
	g.blockProcessor = services.NewBlockProcessor(g.TxStore)
}

// InitSDN initialize SDN, get account model
func InitSDN(bxConfig *config.Bx, blockchainPeers []types.NodeEndpoint, gatewayPeers string, staticEnodesCount int) (*utils.SSLCerts, connections.SDNHTTP, error) {
	_, err := os.Stat(".dockerignore")
	isDocker := !os.IsNotExist(err)
	hostname, _ := os.Hostname()

	privateCertDir := path.Join(bxConfig.DataDir, "ssl")
	gatewayType := bxConfig.NodeType
	gatewayMode := bxConfig.GatewayMode
	privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile := utils.GetCertDir(bxConfig.RegistrationCertDir, privateCertDir, strings.ToLower(gatewayType.String()))
	sslCerts := utils.NewSSLCertsFromFiles(privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile)
	blockchainPeerEndpoint := types.NodeEndpoint{IP: "", Port: 0, PublicKey: ""}
	if len(blockchainPeers) > 0 {
		blockchainPeerEndpoint.IP = blockchainPeers[0].IP
		blockchainPeerEndpoint.Port = blockchainPeers[0].Port
		blockchainPeerEndpoint.PublicKey = blockchainPeers[0].PublicKey
	}

	nodeModel := sdnmessage.NodeModel{
		NodeType:        gatewayType.String(),
		GatewayMode:     string(gatewayMode),
		ExternalIP:      bxConfig.ExternalIP,
		ExternalPort:    bxConfig.ExternalPort,
		BlockchainIP:    blockchainPeerEndpoint.IP,
		BlockchainPeers: gatewayPeers,
		NodePublicKey:   blockchainPeerEndpoint.PublicKey,
		BlockchainPort:  blockchainPeerEndpoint.Port,
		ProgramName:     types.BloxrouteGoGateway,
		SourceVersion:   version.BuildVersion,
		IsDocker:        isDocker,
		Hostname:        hostname,
		OsVersion:       runtime.GOOS,
		ProtocolVersion: bxmessage.CurrentProtocol,
		IsGatewayMiner:  bxConfig.BlocksOnly,
		NodeStartTime:   time.Now().Format(bxgateway.TimeLayoutISO),
		StartupArgs:     strings.Join(os.Args[1:], " "),
	}

	sdn := connections.NewSDNHTTP(&sslCerts, bxConfig.SDNURL, nodeModel, bxConfig.DataDir)

	err = sdn.InitGateway(bxgateway.Ethereum, bxConfig.BlockchainNetwork)
	if err != nil {
		return nil, nil, err
	}

	accountModel := sdn.AccountModel()
	if bxConfig.MEVBuilderURI != "" && accountModel.MEVBuilder == "" {
		return nil, nil, fmt.Errorf(
			"account %v is not allowed for mev builder service, closing the gateway. Please contact support@bloxroute.com to enable running as mev builder",
			accountModel.AccountID,
		)
	}

	if bxConfig.MEVMinerURI != "" && accountModel.MEVMiner == "" {
		return nil, nil, fmt.Errorf(
			"account %v is not allowed for mev miner service, closing the gateway. Please contact support@bloxroute.com to enable running as mev miner",
			accountModel.AccountID,
		)
	}

	if uint64(staticEnodesCount) < uint64(accountModel.MinAllowedNodes.MsgQuota.Limit) {
		panic(fmt.Sprintf(
			"account %v is not allowed to run %d blockchain nodes. Minimum is %d",
			accountModel.AccountID,
			len(blockchainPeers),
			accountModel.MinAllowedNodes.MsgQuota.Limit,
		))
	}

	if uint64(staticEnodesCount) > uint64(accountModel.MaxAllowedNodes.MsgQuota.Limit) {
		panic(fmt.Sprintf(
			"account %v is not allowed to run %d blockchain nodes. Maximum is %d",
			accountModel.AccountID,
			len(blockchainPeers),
			accountModel.MaxAllowedNodes.MsgQuota.Limit,
		))
	}

	return &sslCerts, sdn, nil
}

func (g *gateway) Run() error {
	defer g.cancel()
	var err error

	g.accountID = g.sdn.NodeModel().AccountID
	// node ID might not be assigned to gateway yet, ok
	nodeID := g.sdn.NodeID()
	log.WithFields(log.Fields{
		"accountID": g.accountID,
		"nodeID":    nodeID,
	}).Infof("ssl certificate successfully loaded")

	blockchainPeerEndpoint := types.NodeEndpoint{IP: "", Port: 0, PublicKey: ""}
	if len(g.blockchainPeers) > 0 {
		blockchainPeerEndpoint.IP = g.blockchainPeers[0].IP
		blockchainPeerEndpoint.Port = g.blockchainPeers[0].Port
		blockchainPeerEndpoint.PublicKey = g.blockchainPeers[0].PublicKey
	}

	accountModel := g.sdn.AccountModel()

	// once we called InitGateway() we have the Networks so we can setup TxStore
	g.setupTxStore()

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

	networkNum := g.sdn.NetworkNum()

	err = g.pushBlockchainConfig()
	if err != nil {
		return fmt.Errorf("could process initial blockchain configuration: %v", err)
	}
	go g.handleBridgeMessages()
	go g.TxStore.Start()
	go g.updateValidatorStateMap()

	g.stats = statistics.NewStats(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, g.sdn.NodeID(), g.sdn.Networks(), g.BxConfig.LogNetworkContent)

	sslCert := g.sslCerts
	g.feedChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)

	blockchainNetwork, err := g.sdn.FindNetwork(networkNum)
	if err != nil {
		return fmt.Errorf("failed to find the blockchainNetwork with networkNum %v, %v", networkNum, err)
	}

	g.feedManager = servers.NewFeedManager(g.context, g, g.feedChan, networkNum, types.NetworkID(blockchainNetwork.DefaultAttributes.NetworkID),
		g.wsManager, accountModel, g.sdn.FetchCustomerAccountModel,
		sslCert.PrivateCertFile(), sslCert.PrivateKeyFile(), *g.BxConfig, g.stats, g.nextValidatorMap)
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled {
		clientHandler := servers.NewClientHandler(g.feedManager, nil, servers.NewHTTPServer(g.feedManager, g.BxConfig.HTTPPort), log.WithFields(log.Fields{
			"component": "gatewayClientHandler",
		}))
		go clientHandler.ManageWSServer(g.BxConfig.ManageWSServer)
		go clientHandler.ManageHTTPServer(g.context)
		g.feedManager.Start()
	}

	if err = log.InitFluentD(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, string(g.sdn.NodeID()), logrus.InfoLevel); err != nil {
		return err
	}

	go g.PingLoop()

	relayInstructions := make(chan connections.RelayInstruction)
	go g.updateRelayConnections(relayInstructions, *sslCert, networkNum)
	err = g.sdn.DirectRelayConnections(context.Background(), g.BxConfig.Relays, uint64(accountModel.RelayLimit.MsgQuota.Limit), relayInstructions, connections.AutoRelayTimeout)
	if err != nil {
		return err
	}

	go g.sendStatsOnInterval(15 * time.Minute)

	if g.BxConfig.GRPC.Enabled {
		grpcServer := newGatewayGRPCServer(g, g.BxConfig.Host, g.BxConfig.Port, g.BxConfig.User, g.BxConfig.Password)
		go grpcServer.Start()
	}

	go g.handleBlockchainConnectionStatusUpdate()

	return nil
}

func (g *gateway) updateRelayConnections(relayInstructions chan connections.RelayInstruction, sslCerts utils.SSLCerts, networkNum types.NetworkNum) {
	for {
		instruction := <-relayInstructions

		switch instruction.Type {
		case connections.Connect:
			g.connectRelay(instruction, sslCerts, networkNum)
		case connections.Disconnect:
			// disconnectRelay
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

	ttd := big.NewInt(math.MaxInt)

	if blockchainAttributes.TerminalTotalDifficulty != nil {
		mergeDifficulty, ok := blockchainAttributes.TerminalTotalDifficulty.(string)
		if !ok {
			return fmt.Errorf("could not parse terminal total difficulty: %v", blockchainAttributes.TerminalTotalDifficulty)
		}
		if ttd, ok = new(big.Int).SetString(mergeDifficulty, 10); !ok {
			return fmt.Errorf("could not parse terminal total difficulty: %v", blockchainAttributes.TerminalTotalDifficulty)
		}
	}

	genesis := common.HexToHash(blockchainAttributes.GenesisHash)
	ignoreBlockTimeout := time.Second * time.Duration(blockchainNetwork.BlockInterval*blockchainNetwork.IgnoreBlockIntervalCount)
	blockConfirmationsCount := blockchainNetwork.BlockConfirmationsCount
	ethConfig := network.EthConfig{
		Network:                 uint64(blockchainAttributes.NetworkID),
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		Head:                    genesis,
		Genesis:                 genesis,
		IgnoreBlockTimeout:      ignoreBlockTimeout,
		BlockConfirmationsCount: blockConfirmationsCount,
	}

	return g.bridge.UpdateNetworkConfig(ethConfig)
}

func (g *gateway) queryEpochBlock(height uint64) error {
	for _, wsProvider := range g.wsManager.Providers() {
		if wsProvider.IsOpen() {
			response, err := wsProvider.FetchBlock([]interface{}{fmt.Sprintf("0x%x", height), false}, blockchain.RPCOptions{RetryAttempts: bxgateway.MaxEthOnBlockCallRetries, RetryInterval: bxgateway.EthOnBlockCallRetrySleepInterval})
			if err == nil && response != nil {
				r := response.(map[string]interface{})
				ed, exist := r["extraData"]
				if !exist {
					return errors.New("extraData field doesn't exist when query previous epoch block")
				}
				data, err := hexutil.Decode(ed.(string))
				if err != nil {
					return err
				}
				return g.processExtraData(height, data)
			}
		}
	}
	return errors.New("failed to query blockchain node for previous epoch block")
}

func (g *gateway) processExtraData(blockHeight uint64, extraData []byte) error {
	if extraData == nil {
		return errors.New("cannot process empty extra data")
	}
	var ed eth.ExtraData
	err := ed.UnmarshalJSON(extraData)
	if err != nil {
		log.Errorf("can't extract extra data for block height %v", blockHeight)
	} else {
		// extraData contains list of validators now
		validatorInfo := blockchain.ValidatorListInfo{
			BlockHeight:   blockHeight,
			ValidatorList: ed.ValidatorList,
		}
		err = g.bridge.SendValidatorListInfo(&validatorInfo)
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *gateway) generateFutureValidatorInfo(blockHeight uint64) []*types.FutureValidatorInfo {
	if g.sdn.NetworkNum() != bxgateway.BSCMainnetNum {
		return nil
	}

	vi := make([]*types.FutureValidatorInfo, 2)
	for i := 1; i <= 2; i++ {
		vi[i-1] = &types.FutureValidatorInfo{
			BlockHeight: blockHeight + uint64(i),
			WalletID:    "nil",
			Accessible:  false,
		}
	}

	if g.validatorListMap == nil || g.validatorStatusMap == nil || g.nextValidatorMap == nil {
		return vi
	}

	// currentEpochBlockHeight will be the most recent block height that can be module by 200
	currentEpochBlockHeight := blockHeight / 200 * 200
	previousEpochBlockHeight := currentEpochBlockHeight - 200
	prevEpochValidatorList, exist := g.validatorListMap.Load(previousEpochBlockHeight)
	if !exist { // we need previous epoch validator list to calculate
		err := g.queryEpochBlock(previousEpochBlockHeight)
		prevEpochValidatorList, exist = g.validatorListMap.Load(previousEpochBlockHeight)
		if err != nil || !exist {
			g.validatorListReady = false
			return vi
		}
	}
	previousEpochValidatorList := prevEpochValidatorList.([]string)

	var currentEpochValidatorList []string
	currEpochValidatorList, exist := g.validatorListMap.Load(currentEpochBlockHeight)
	if !exist {
		err := g.queryEpochBlock(currentEpochBlockHeight)
		currEpochValidatorList, exist = g.validatorListMap.Load(currentEpochBlockHeight)
		if err != nil || !exist {
			g.validatorListReady = false
			return vi
		}
	}
	currentEpochValidatorList = currEpochValidatorList.([]string)
	if !g.validatorListReady {
		g.validatorListReady = true
		log.Info("The gateway has all the information to support next_validator transactions")
	}

	for i := 1; i <= 2; i++ {
		targetingBlockHeight := blockHeight + uint64(i)
		listIndex := targetingBlockHeight % uint64(len(currentEpochValidatorList)) // listIndex is the index for the validator list
		activationIndex := uint64((len(previousEpochValidatorList) + 1) / 2)       // activationIndex = ceiling[ N / 2 ] where N = the length of previous validator list, it marks a watershed. To the leftward we use previous validator list, to the rightward(inclusive) we use current validator list. Reference: https://github.com/bnb-chain/docs-site/blob/master/docs/smart-chain/guides/concepts/consensus.md
		index := targetingBlockHeight - currentEpochBlockHeight
		if index >= activationIndex {
			validatorAddr := currentEpochValidatorList[listIndex]
			vi[i-1].WalletID = strings.ToLower(validatorAddr)
		} else { // use list from previous epoch
			validatorAddr := previousEpochValidatorList[listIndex]
			vi[i-1].WalletID = strings.ToLower(validatorAddr)
		}

		g.nextValidatorMap.Set(targetingBlockHeight, strings.ToLower(vi[i-1].WalletID)) // nextValidatorMap is simulating a queue with height as expiration key. Regardless of the accessible status, next walletID will be appended to the queue
		accessible, exist := g.validatorStatusMap.Load(strings.ToLower(vi[i-1].WalletID))
		if exist {
			vi[i-1].Accessible = accessible.(bool)
		}
	}

	// remove all wallet address of existing blocks
	toRemove := make([]uint64, 0)
	for pair := g.nextValidatorMap.Oldest(); pair != nil; pair = pair.Next() {
		if int(blockHeight) >= int(pair.Key.(uint64)) {
			toRemove = append(toRemove, pair.Key.(uint64))
		}
	}

	for _, height := range toRemove {
		g.nextValidatorMap.Delete(height)
	}

	return vi
}

func (g *gateway) publishBlock(bxBlock *types.BxBlock, nodeSource *connections.Blockchain, info []*types.FutureValidatorInfo, isBlockchainBlock bool) error {
	// publishing a block means extracting the sender for all the block transactions which is heavy.
	// if there are no active block related feed subscribers we can skip this.
	if !g.feedManager.NeedBlocks() {
		return nil
	}

	// beaconBlockNotification is nil if not consensus layer block
	ethBlockNotification, beaconBlockNotification, err := g.bridge.BxBlockToCanonicFormat(bxBlock, info)
	if err != nil {
		return err
	}

	if isBlockchainBlock {
		if err := g.publishBlockchainBlock(bxBlock, ethBlockNotification, beaconBlockNotification, nodeSource); err != nil {
			return err
		}
	}

	// even though it is not from BDN, still sending this block in the feed in case the node sent the block first
	return g.publishBDNBlock(bxBlock, ethBlockNotification, beaconBlockNotification, nodeSource)
}

func (g *gateway) publishBlockchainBlock(bxBlock *types.BxBlock, ethBlockNotification, beaconBlockNotification types.BlockNotification, nodeSource *connections.Blockchain) error {
	blockHeight := int(bxBlock.Number.Int64())

	if !beaconBlockNotification.IsNil() {
		log.Debugf("Received block %v for %v source: %v, notify the feed", bxBlock, types.NewBeaconBlocksFeed, nodeSource)
		beaconBlockNotification.SetNotificationType(types.NewBeaconBlocksFeed)
		g.notify(beaconBlockNotification)
	}

	log.Debugf("Received block %v for %v source: %v, notify the feed", bxBlock, types.NewBlocksFeed, nodeSource)
	newBlockNotification := ethBlockNotification.Clone()
	newBlockNotification.SetNotificationType(types.NewBlocksFeed)
	g.notify(newBlockNotification)

	g.bestBlockHeight = blockHeight
	g.bdnBlocksSkipCount = 0

	onBlockNotification := ethBlockNotification.Clone()
	onBlockNotification.SetNotificationType(types.OnBlockFeed)
	if nodeSource != nil {
		sourceEndpoint := nodeSource.NodeEndpoint()
		onBlockNotification.SetSource(&sourceEndpoint)
	}
	log.Debugf("Received block %v for %v source: %v, notify the feed", bxBlock, types.OnBlockFeed, nodeSource)
	g.notify(onBlockNotification)

	txReceiptNotification := ethBlockNotification.Clone()
	txReceiptNotification.SetNotificationType(types.TxReceiptsFeed)
	if nodeSource != nil {
		sourceEndpoint := nodeSource.NodeEndpoint()
		txReceiptNotification.SetSource(&sourceEndpoint)
	}
	log.Debugf("Received block %v for %v source: %v notify the feed", bxBlock, types.TxReceiptsFeed, nodeSource)
	g.notify(txReceiptNotification)

	return nil
}

func (g *gateway) publishBDNBlock(bxBlock *types.BxBlock, ethBlockNotification, beaconBlockNotification types.BlockNotification, nodeSource *connections.Blockchain) error {
	isEthRequired := true
	sentBlockHash := bxBlock.Hash()
	if !bxBlock.BeaconHash().Empty() {
		sentBlockHash = bxBlock.BeaconHash()
		isEthRequired = g.bdnBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute)
	}

	if !g.bdnBlocks.SetIfAbsent(sentBlockHash.String(), 15*time.Minute) {
		log.Debugf("Block %v was already published with feed %v", bxBlock, types.BDNBlocksFeed)
		return nil
	}

	blockHeight := int(bxBlock.Number.Int64())

	// Beacon block has eth1 block inside so no need to send it twice
	if len(g.blockchainPeers) > 0 && blockHeight < g.bestBlockHeight {
		log.Debugf("block %v is too far behind best block height %v from node - not publishing to bdnBlocks", bxBlock, g.bestBlockHeight)
		return nil
	}
	if g.bestBlockHeight != 0 && utils.Abs(blockHeight-g.bestBlockHeight) > bxgateway.BDNBlocksMaxBlocksAway {
		if blockHeight > g.bestBlockHeight {
			g.bdnBlocksSkipCount++
		}
		if g.bdnBlocksSkipCount <= bxgateway.MaxOldBDNBlocksToSkipPublish {
			log.Debugf("block %v is too far away from best block height %v - not publishing to bdnBlocks", bxBlock, g.bestBlockHeight)
			return nil
		}
		log.Debugf("publishing block %v from BDN that is far away from current best block height %v - resetting bestBlockHeight to zero", bxBlock, g.bestBlockHeight)
		g.bestBlockHeight = 0
	}
	g.bdnBlocksSkipCount = 0
	if len(g.blockchainPeers) == 0 && blockHeight > g.bestBlockHeight {
		g.bestBlockHeight = blockHeight
	}

	if isEthRequired {
		log.Debugf("Received block %v for %v source: %v, notify the feed", bxBlock, types.BDNBlocksFeed, nodeSource)
		bdnBlockNotification := ethBlockNotification.Clone()
		bdnBlockNotification.SetNotificationType(types.BDNBlocksFeed)
		g.notify(bdnBlockNotification)
	}

	if !beaconBlockNotification.IsNil() {
		log.Debugf("Received block %v for %v source: %v, notify the feed", bxBlock, types.BDNBeaconBlocksFeed, nodeSource)
		bdnBeaconBlockNotification := beaconBlockNotification.Clone()
		bdnBeaconBlockNotification.SetNotificationType(types.BDNBeaconBlocksFeed)
		g.notify(bdnBeaconBlockNotification)
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
	// may have long proccess time so don't want to block other handlers
	go g.handleBlockFromBlockchain()

	var err error
	for {
		select {
		case txsFromNode := <-g.bridge.ReceiveNodeTransactions():
			traceIfSlow(func() {
				// if we are not yet synced with relay - ignore the transactions from the node
				if !g.isSyncWithRelay() {
					return
				}
				blockchainConnection := connections.NewBlockchainConn(txsFromNode.PeerEndpoint)
				for _, blockchainTx := range txsFromNode.Transactions {
					tx := bxmessage.NewTx(blockchainTx.Hash(), blockchainTx.Content(), g.sdn.NetworkNum(), types.TFLocalRegion, types.EmptyAccountID)
					g.processTransaction(tx, blockchainConnection)
				}
			}, "ReceiveNodeTransactions", txsFromNode.PeerEndpoint.String(), int64(len(txsFromNode.Transactions)))
		case txAnnouncement := <-g.bridge.ReceiveTransactionHashesAnnouncement():
			traceIfSlow(func() {
				// if we are not yet synced with relay - ignore the announcement from the node
				if !g.isSyncWithRelay() {
					return
				}
				// if announcement message has many transaction we are probably after reconnect with the node - we should ignore it in order not to over load the client feed
				if len(txAnnouncement.Hashes) > bxgateway.MaxAnnouncementFromNode {
					log.Debugf("skipped tx announcement of size %v", len(txAnnouncement.Hashes))
					return
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
					if !txAnnouncement.PeerEndpoint.IsInbound() {
						g.publishPendingTx(hash, bxTx, true)
					}
				}

				if len(requests) > 0 && txAnnouncement.PeerID != bxgateway.WSConnectionID {
					err = g.bridge.RequestTransactionsFromNode(txAnnouncement.PeerID, requests)
					if err == blockchain.ErrChannelFull {
						log.Warnf("transaction requests channel is full, skipping request for %v hashes", len(requests))
					} else if err != nil {
						panic(fmt.Errorf("could not request transactions over bridge: %v", err))
					}
				}
			}, "ReceiveTransactionHashesAnnouncement", txAnnouncement.PeerID, int64(len(txAnnouncement.Hashes)))
		case _ = <-g.bridge.ReceiveNoActiveBlockchainPeersAlert():
			if !g.sdn.AccountTier().IsElite() {
				// TODO should fix code to stop gateway appropriately
				log.Errorf("Gateway does not have an active blockchain connection. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
				log.Exit(0)
			}
		case confirmBlock := <-g.bridge.ReceiveConfirmedBlockFromNode():
			traceIfSlow(func() {
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
			}, "ReceiveConfirmedBlockFromNode", confirmBlock.PeerEndpoint.String(), 1)
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
	case *bxmessage.ValidatorUpdates:
		validatorMsg := msg.(*bxmessage.ValidatorUpdates)
		g.processValidatorUpdate(validatorMsg, source)
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
			g.TxStore.Add(txsItem.Hash, txsItem.Content, txsItem.ShortID, g.sdn.NetworkNum(), false, 0, time.Now(), 0, types.EmptySender)
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
				log.Debug("gateway is not sending block confirm message to relay")
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
	bxBlock, missingShortIDs, err := g.blockProcessor.BxBlockFromBroadcast(broadcastMsg)
	if err != nil {
		switch err {
		case services.ErrAlreadyProcessed:
			source.Log().Debugf("received duplicate %v skipping", broadcastMsg)

			var eventName string
			if broadcastMsg.IsBeaconBlock() {
				eventName = "GatewayProcessBeaconBlockFromBDNIgnoreSeen"
			} else {
				eventName = "GatewayProcessBlockFromBDNIgnoreSeen"
			}

			g.stats.AddGatewayBlockEvent(eventName, source, broadcastMsg.Hash(), broadcastMsg.BeaconHash(), broadcastMsg.GetNetworkNum(), 1, startTime, 0, 0, len(broadcastMsg.Block()), len(broadcastMsg.ShortIDs()), 0, 0, bxBlock)
		case services.ErrMissingShortIDs:
			source.Log().Debugf("%v from BDN is missing %v short IDs", broadcastMsg, len(missingShortIDs))

			if !g.isSyncWithRelay() {
				source.Log().Debugf("TxStore sync is in progress - Ignoring %v from bdn with unknown %v shortIDs", broadcastMsg, len(missingShortIDs))
				return
			}
			source.Log().Debugf("could not decompress %v, missing shortIDs count: %v", broadcastMsg, missingShortIDs)

			var eventName string
			if broadcastMsg.IsBeaconBlock() {
				eventName = "GatewayProcessBeaconBlockFromBDNRequiredRecovery"
			} else {
				eventName = "GatewayProcessBlockFromBDNRequiredRecovery"
			}

			g.stats.AddGatewayBlockEvent(eventName, source, broadcastMsg.Hash(), broadcastMsg.BeaconHash(), broadcastMsg.GetNetworkNum(), 1, startTime, 0, 0, len(broadcastMsg.Block()), len(broadcastMsg.ShortIDs()), 0, len(missingShortIDs), bxBlock)
		case services.ErrNotCompitableBeaconBlock:
			// Old relay version
			source.Log().Debugf("received incompitable beacon block %v skipping", broadcastMsg)
		default:
			source.Log().Errorf("could not decompress %v, err: %v", broadcastMsg, err)
			broadcastBlockHex := hex.EncodeToString(broadcastMsg.Block())
			source.Log().Debugf("could not decompress %v, err: %v, contents: %v", broadcastMsg, err, broadcastBlockHex)
		}

		return
	}

	source.Log().Infof("processing %v from BDN, block number: %v", broadcastMsg, bxBlock.Number)
	g.processBlockFromBDN(bxBlock)

	var eventName string
	if broadcastMsg.IsBeaconBlock() {
		eventName = "GatewayProcessBeaconBlockFromBDN"
	} else {
		eventName = "GatewayProcessBlockFromBDN"
	}

	g.stats.AddGatewayBlockEvent(eventName, source, broadcastMsg.Hash(), broadcastMsg.BeaconHash(), broadcastMsg.GetNetworkNum(), 1, startTime, 0, bxBlock.Size(), len(broadcastMsg.Block()), len(broadcastMsg.ShortIDs()), len(bxBlock.Txs), 0, bxBlock)
}

func (g *gateway) processValidatorUpdate(msg *bxmessage.ValidatorUpdates, source connections.Conn) {
	if g.sdn.NetworkNum() != bxgateway.BSCMainnetNum || g.sdn.NetworkNum() != msg.GetNetworkNum() {
		source.Log().Debugf("ignore validator update message for %v network number from relay, gateway is %v", msg.GetNetworkNum(), g.sdn.NetworkNum())
		return
	}

	g.validatorStatusMap = &sync.Map{}
	onlineList := msg.GetOnlineList()
	for _, addr := range onlineList {
		g.validatorStatusMap.Store(strings.ToLower(addr), true)
	}
}

func (g *gateway) processTransaction(tx *bxmessage.Tx, source connections.Conn) {
	startTime := time.Now()
	networkDuration := startTime.Sub(tx.Timestamp()).Microseconds()
	sentToBlockchainNode := false
	sentToBDN := false
	sourceInfo := source.Info()
	sourceEndpoint := types.NodeEndpoint{IP: sourceInfo.PeerIP, Port: int(sourceInfo.PeerPort), PublicKey: sourceInfo.PeerEnode}
	connEndpoint, ok := source.(connections.EndpointConn)
	if ok {
		sourceEndpoint = connEndpoint.NodeEndpoint()
	}
	var broadcastRes types.BroadcastResults

	// we add the transaction to TxStore with current time so we can measure time difference to node announcement/confirmation
	txResult := g.TxStore.Add(tx.Hash(), tx.Content(), tx.ShortID(), tx.GetNetworkNum(), !sourceInfo.IsRelay(), tx.Flags(), g.clock.Now(), 0, tx.Sender())
	eventName := "TxProcessedByGatewayFromPeerIgnoreSeen"

	switch {
	case txResult.FailedValidation:
		eventName = "TxValidationFailedStructure"
	case txResult.NewContent && txResult.Transaction.Flags().IsReuseSenderNonce() && tx.ShortID() == types.ShortIDEmpty:
		eventName = "TxReuseSenderNonce"
		log.Trace(txResult.DebugData)
	case txResult.AlreadySeen:
		log.Tracef("received already Seen transaction %v from %v:%v (%v) and account id %v", tx.Hash(), sourceInfo.PeerIP, sourceInfo.PeerPort, sourceInfo.NodeID, sourceInfo.AccountID)
	case txResult.NewContent || txResult.NewSID || txResult.Reprocess:
		eventName = "TxProcessedByGatewayFromPeer"
		if txResult.NewContent || txResult.Reprocess {
			validatorsOnlyTxFromCloudAPI := sourceInfo.ConnectionType == utils.CloudAPI && tx.Flags().IsValidatorsOnly()
			nextValidatorTxFromCloudAPI := sourceInfo.ConnectionType == utils.CloudAPI && tx.Flags().IsNextValidator()
			if txResult.NewContent && !tx.Flags().IsValidatorsOnly() && !tx.Flags().IsNextValidator() {
				newTxsNotification := types.CreateNewTransactionNotification(txResult.Transaction)
				g.notify(newTxsNotification)
				if !sourceEndpoint.IsInbound() {
					g.publishPendingTx(txResult.Transaction.Hash(), txResult.Transaction, sourceInfo.ConnectionType == utils.Blockchain)
				}
			}

			if !sourceInfo.IsRelay() {
				if sourceInfo.ConnectionType == utils.Blockchain {
					g.bdnStats.LogNewTxFromNode(sourceEndpoint)
				}

				paidTx := tx.Flags().IsPaid()
				var (
					allowed  bool
					behavior sdnmessage.BDNServiceBehaviorType
				)
				if sourceInfo.ConnectionType == utils.CloudAPI {
					allowed, behavior = g.burstLimiter.AllowTransaction(sourceInfo.AccountID, paidTx)
				} else {
					allowed, behavior = g.burstLimiter.AllowTransaction(g.accountID, paidTx)
				}
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
					//	source.Log().Warnf("account burst limits exceeded: your account is limited to %v paid txs per 5 seconds", g.burstLimiter.BurstLimit(sourceInfo.AccountID, paidTx))
					// } else {
					//	source.Log().Debugf("account burst limits exceeded: your account is limited to %v unpaid txs per 5 seconds", g.burstLimiter.BurstLimit(sourceInfo.AccountID, paidTx))
					// }
					fallthrough
				case sdnmessage.BehaviorNoAction:
					fallthrough
				default:
					allowed = true
				}

				if allowed {
					// set timestamp so relay can analyze communication delay
					tx.SetTimestamp(g.clock.Now())
					broadcastRes = g.broadcast(tx, source, utils.RelayTransaction)
					sentToBDN = true
				}
			}

			// if this gateway is the associated validator, need to send the transaction to the node
			if (tx.Flags().IsValidatorsOnly() || tx.Flags().IsNextValidator()) && g.sdn.AccountModel().Miner {
				tx.AddFlags(types.TFDeliverToNode)
			}

			// if source is relay, this means either this gateway is the associated validator, or the tx was sent after the fallback time
			if (tx.Flags().IsValidatorsOnly() || tx.Flags().IsNextValidator()) && !g.sdn.AccountModel().Miner {
				tx.RemoveFlags(types.TFDeliverToNode)
			}

			shouldSendTxFromNodeToOtherNodes := sourceInfo.ConnectionType == utils.Blockchain && len(g.blockchainPeers) > 1
			// send to node if all are true
			shouldSendTxFromBDNToNodes := len(g.blockchainPeers) > 0 && // Gateway is connected to nodes
				sourceInfo.ConnectionType != utils.Blockchain && // Transaction is not from blockchain node (transaction is from Relay or RPC)
				((!g.BxConfig.BlocksOnly && tx.Flags().ShouldDeliverToNode()) || // (Gateway didn't start with blocks only mode and DeliverToNode flag is on) OR
					(g.BxConfig.AllTransactions && !validatorsOnlyTxFromCloudAPI && !nextValidatorTxFromCloudAPI)) // OR (Gateway started with a flag to send all transactions and the tx is not sent from Cloud API for validators only)

			if shouldSendTxFromNodeToOtherNodes || shouldSendTxFromBDNToNodes {
				txsToDeliverToNodes := blockchain.Transactions{
					Transactions:   []*types.BxTransaction{txResult.Transaction},
					PeerEndpoint:   sourceEndpoint,
					ConnectionType: sourceInfo.ConnectionType,
				}
				err := g.bridge.SendTransactionsFromBDN(txsToDeliverToNodes)
				if err != nil {
					log.Errorf("failed to send transaction %v from BDN to bridge - %v", txResult.Transaction.Hash(), err)
				}
				sentToBlockchainNode = true
				if shouldSendTxFromNodeToOtherNodes {
					g.bdnStats.LogTxSentToAllNodesExceptSourceNode(sourceEndpoint)
				} else {
					g.bdnStats.LogTxSentToNodes()
				}
			}
			if sourceInfo.IsRelay() && !txResult.Reprocess {
				g.bdnStats.LogNewTxFromBDN()
			}

			g.txTrace.Log(tx.Hash(), source)
		}
	default:
		// duplicate transaction
		if txResult.Transaction.Flags().IsReuseSenderNonce() {
			eventName = "TxReuseSenderNonceIgnoreSeen"
			log.Trace(txResult.DebugData)
		}
		if sourceInfo.ConnectionType == utils.Blockchain {
			g.bdnStats.LogDuplicateTxFromNode(sourceEndpoint)
		}
	}

	statsStart := time.Now()
	g.stats.AddTxsByShortIDsEvent(eventName, source, txResult.Transaction, tx.ShortID(), sourceInfo.NodeID, broadcastRes.RelevantPeers, broadcastRes.SentGatewayPeers, startTime, tx.GetPriority(), txResult.DebugData)
	statsDuration := time.Since(statsStart)
	log.Tracef("msgTx: from %v, hash %v, nonce %v, flags %v, new Tx %v, new content %v, new shortid %v, event %v, sentToBDN: %v, sentToBlockchainNode: %v, handling duration %v, sender %v, networkDuration %v statsDuration %v, nextValidator enabled %v, validator wallet ID %v, fallback duration %v", source, tx.Hash(), txResult.Nonce, tx.Flags(), txResult.NewTx, txResult.NewContent, txResult.NewSID, eventName, sentToBDN, sentToBlockchainNode, time.Since(startTime), txResult.Transaction.Sender(), networkDuration, statsDuration, tx.Flags().IsNextValidator(), tx.WalletIDs(), tx.Fallback())
}

func (g *gateway) updateValidatorStateMap() {
	for newList := range g.bridge.ReceiveValidatorListInfo() {
		blockHeight := newList.BlockHeight
		g.validatorListMap.Delete(blockHeight - 400) // remove the validator list that doesn't need anymore
		g.validatorListMap.Store(blockHeight, newList.ValidatorList)
	}
}

func (g *gateway) handleBlockFromBlockchain() {
	for blockchainBlock := range g.bridge.ReceiveBlockFromNode() {
		traceIfSlow(func() {
			bxBlock := blockchainBlock.Block
			source := connections.NewBlockchainConn(blockchainBlock.PeerEndpoint)

			startTime := time.Now()

			g.bdnStats.LogNewBlockMessageFromNode(source.NodeEndpoint())

			validatorInfo := g.generateFutureValidatorInfo(bxBlock.Number.Uint64())

			if err := g.publishBlock(bxBlock, &source, validatorInfo, !blockchainBlock.PeerEndpoint.IsInbound()); err != nil {
				source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock.Hash(), err)
			}

			broadcastMessage, usedShortIDs, err := g.blockProcessor.BxBlockToBroadcast(bxBlock, g.sdn.NetworkNum(), g.sdn.MinTxAge())
			if err == services.ErrAlreadyProcessed {
				source.Log().Debugf("received duplicate block %v, skipping", bxBlock.Hash())

				eventName := "GatewayReceivedBlockFromBlockchainNodeIgnoreSeen"
				if bxBlock.IsBeaconBlock() {
					eventName = "GatewayReceivedBeaconBlockFromBlockchainNodeIgnoreSeen"
				}

				g.stats.AddGatewayBlockEvent(eventName, source, bxBlock.Hash(), bxBlock.BeaconHash(), g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), 0, 0, len(bxBlock.Txs), len(usedShortIDs), bxBlock)

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

			eventName := "GatewayReceivedBlockFromBlockchainNode"
			if bxBlock.IsBeaconBlock() {
				eventName = "GatewayReceivedBeaconBlockFromBlockchainNode"
			}

			g.stats.AddGatewayBlockEvent(eventName, source, bxBlock.Hash(), bxBlock.BeaconHash(), g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), int(broadcastMessage.Size(bxmessage.CurrentProtocol)), len(broadcastMessage.ShortIDs()), len(bxBlock.Txs), len(usedShortIDs), bxBlock)
		}, "handleBlockFromBlockchain", blockchainBlock.PeerEndpoint.String(), 1)
	}
}

func (g *gateway) processBlockFromBDN(bxBlock *types.BxBlock) {
	err := g.bridge.SendBlockToNode(bxBlock)
	if err != nil {
		log.Errorf("unable to send block from BDN to node: %v", err)
	}

	g.bdnStats.LogNewBlockFromBDN()
	validatorInfo := g.generateFutureValidatorInfo(bxBlock.Number.Uint64())
	err = g.publishBlock(bxBlock, nil, validatorInfo, false)
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

			if !g.BxConfig.MEVMaxProfitBuilder && mevSearcher.Frontrunning {
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

			req.Header.Add(flashbotAuthHeader, mevSearcherAuth)
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

// DisconnectInboundPeer disconnect inbound peer from gateway
func (g *gateway) DisconnectInboundPeer(_ context.Context, req *pb.DisconnectInboundPeerRequest) (*pb.DisconnectInboundPeerReply, error) {
	err := g.bridge.SendDisconnectEvent(types.NodeEndpoint{IP: req.PeerIp, Port: int(req.PeerPort), PublicKey: req.PublicKey})
	if err != nil {
		return &pb.DisconnectInboundPeerReply{Status: err.Error()}, err
	}
	return &pb.DisconnectInboundPeerReply{Status: fmt.Sprintf("Sent request to disconnect peer %v %v %v", req.PublicKey, req.PeerIp, req.PeerPort)}, err
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
			for _, peer := range status {
				connStatus := &pb.NodeConnStatus{
					Inbound: peer.IsInbound(),
				}

				nstat, ok := nodeStats[peer.IPPort()]
				if ok {
					connStatus.IsConnected = nstat.IsConnected
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

				mp[ipport(peer.IP, peer.Port)] = connStatus

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
			}

			// If a node was disconnected through the interval then they are not connected.
			// Let state this explicitly.
			for key, peer := range nodeStats {
				ipPort := strings.Replace(key, " ", ":", -1)
				if _, ok := mp[ipPort]; !ok {
					mp[ipPort] = &pb.NodeConnStatus{
						IsConnected: peer.IsConnected,
						Inbound:     peer.Inbound,
						NodePerformance: &pb.NodePerformance{
							Since:                                   g.bdnStats.StartTime().Format(time.RFC3339),
							NewBlocksReceivedFromBlockchainNode:     uint32(peer.NewBlocksReceivedFromBlockchainNode),
							NewBlocksReceivedFromBdn:                uint32(peer.NewBlocksReceivedFromBdn),
							NewBlocksSeen:                           peer.NewBlocksSeen,
							NewBlockMessagesFromBlockchainNode:      peer.NewBlockMessagesFromBlockchainNode,
							NewBlockAnnouncementsFromBlockchainNode: peer.NewBlockAnnouncementsFromBlockchainNode,
							NewTxReceivedFromBlockchainNode:         peer.NewTxReceivedFromBlockchainNode,
							NewTxReceivedFromBdn:                    peer.NewTxReceivedFromBdn,
							TxSentToNode:                            peer.TxSentToNode,
							DuplicateTxFromNode:                     peer.DuplicateTxFromNode,
						},
					}
				}
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
			Version:          version.BuildVersion,
			NodeId:           string(nodeModel.NodeID),
			IpAddress:        nodeModel.ExternalIP,
			TimeStarted:      g.timeStarted.Format(time.RFC3339),
			Continent:        nodeModel.Continent,
			Country:          nodeModel.Country,
			Network:          nodeModel.Network,
			StartupParams:    strings.Join(os.Args[1:], " "),
			GatewayPublicKey: g.gatewayPublicKey,
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

			broadcastRes := g.broadcast(closedIntervalBDNStatsMsg, nil, utils.Relay)

			closedIntervalBDNStatsMsg.Log()
			log.Tracef("sent bdnStats msg to relays, result: [%v]", broadcastRes)
		}
	}
}

func (g *gateway) handleBlockchainConnectionStatusUpdate() {
	for blockchainConnectionStatus := range g.bridge.ReceiveBlockchainConnectionStatus() {
		if !blockchainConnectionStatus.IsInbound {
			if blockchainConnectionStatus.IsConnected {
				g.sdn.SendNodeEvent(
					sdnmessage.NewBlockchainNodeConnEstablishedEvent(
						g.sdn.NodeID(),
						blockchainConnectionStatus.PeerEndpoint.IP,
						blockchainConnectionStatus.PeerEndpoint.Port,
						g.clock.Now().String(),
					),
					g.sdn.NodeID())
			} else {
				g.sdn.SendNodeEvent(
					sdnmessage.NewBlockchainNodeConnError(
						g.sdn.NodeID(),
						blockchainConnectionStatus.PeerEndpoint.IP,
						blockchainConnectionStatus.PeerEndpoint.Port,
						g.clock.Now().String(),
					),
					g.sdn.NodeID())
			}
		}
	}
}

func traceIfSlow(f func(), name string, from string, count int64) {
	startTime := time.Now()
	f()
	duration := time.Since(startTime)

	if duration > time.Millisecond {
		if count > 0 {
			log.Tracef("%s from %v spent %v processing %v entries (%v avg)", name, from, duration, count, duration.Microseconds()/count)
		} else {
			log.Tracef("%s from %v spent %v processing %v entries", name, from, duration, count)
		}
	}
}
