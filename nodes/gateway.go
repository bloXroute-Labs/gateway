package nodes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"os"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/atomic"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon/bor"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/loggers"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/bundle"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/bloXroute-Labs/gateway/v2/version"
)

const (
	ignoreSeenEvent = "ignore seen"
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
	feedManagerChan    chan types.Notification
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
	newBlocks          services.HashHistory
	wsManager          blockchain.WSManager
	syncedWithRelay    atomic.Bool
	clock              utils.Clock
	timeStarted        time.Time
	burstLimiter       services.AccountBurstLimiter

	bestBlockHeight       int
	bdnBlocksSkipCount    int
	seenMEVBundles        services.HashHistory
	seenMEVMinerBundles   services.HashHistory
	seenMEVSearchers      services.HashHistory
	seenBlockConfirmation services.HashHistory

	mevClient           *http.Client
	mevBundleDispatcher *bundle.Dispatcher

	bscValidatorClient *http.Client

	bscTxClient      *http.Client
	gatewayPeers     string
	gatewayPublicKey string

	staticEnodesCount            int
	startupArgs                  string
	validatorStatusMap           *syncmap.SyncMap[string, bool]     // validator addr -> online/offline
	validatorListMap             *syncmap.SyncMap[uint64, []string] // block height -> list of validators
	nextValidatorMap             *orderedmap.OrderedMap             // next accessible validator
	validatorListReady           bool
	validatorInfoUpdateLock      sync.Mutex
	latestValidatorInfo          []*types.FutureValidatorInfo
	latestValidatorInfoHeight    int64
	transactionSlotStartDuration int
	transactionSlotEndDuration   int
	nextBlockTime                time.Time

	polygonValidatorInfoManager polygon.ValidatorInfoManager
	blockTime                   time.Duration

	grpcHandler   *servers.GrpcHandler
	txsQueue      services.MessageQueue
	txsOrderQueue services.MessageQueue
}

type proposedBlockArgs struct {
	MEVRelay      string          `json:"mevRelay,omitempty"`
	BlockNumber   rpc.BlockNumber `json:"blockNumber"`
	PrevBlockHash common.Hash     `json:"prevBlockHash"`
	BlockReward   *big.Int        `json:"blockReward"`
	GasLimit      uint64          `json:"gasLimit"`
	GasUsed       uint64          `json:"gasUsed"`
	Payload       []hexutil.Bytes `json:"payload"`
	UnReverted    []int           `json:"unReverted"`
}

// GeneratePeers generate string peers separated by coma
func GeneratePeers(peersInfo []network.PeerInfo) string {
	var result string
	if len(peersInfo) == 0 {
		return result
	}

	var peers []string
	for _, peer := range peersInfo {
		if peer.Enode != nil {
			peers = append(peers, fmt.Sprintf("%s+%s,", peer.Enode.String(), peer.EthWSURI))
		} else if peer.Multiaddr != nil {
			peers = append(peers, fmt.Sprintf("%s,", (*peer.Multiaddr).String()))
		}
	}
	return strings.Join(peers, ",")
}

// NewGateway returns a new gateway node to send messages from a blockchain node to the relay network
func NewGateway(parent context.Context, bxConfig *config.Bx, bridge blockchain.Bridge, wsManager blockchain.WSManager,
	blockchainPeers []types.NodeEndpoint, peersInfo []network.PeerInfo, recommendedPeers map[string]struct{}, gatewayPublicKeyStr string, sdn connections.SDNHTTP,
	sslCerts *utils.SSLCerts, staticEnodesCount int, polygonHeimdallEndpoint string, transactionSlotStartDuration int, transactionSlotEndDuration int) (Node, error) {
	ctx, cancel := context.WithCancel(parent)
	clock := utils.RealClock{}
	blockTime, _ := bxgateway.NetworkToBlockDuration[bxConfig.BlockchainNetwork]

	g := &gateway{
		Bx:                           NewBx(bxConfig, "datadir", nil),
		bridge:                       bridge,
		isBDN:                        bxConfig.GatewayMode.IsBDN(),
		wsManager:                    wsManager,
		context:                      ctx,
		cancel:                       cancel,
		blockchainPeers:              blockchainPeers,
		pendingTxs:                   services.NewHashHistory("pendingTxs", 15*time.Minute),
		possiblePendingTxs:           services.NewHashHistory("possiblePendingTxs", 15*time.Minute),
		bdnBlocks:                    services.NewHashHistory("bdnBlocks", 15*time.Minute),
		newBlocks:                    services.NewHashHistory("newBlocks", 15*time.Minute),
		seenMEVBundles:               services.NewHashHistory("mevBundle", 30*time.Minute),
		seenMEVMinerBundles:          services.NewHashHistory("mevMinerBundle", 30*time.Minute),
		seenMEVSearchers:             services.NewHashHistory("mevSearcher", 30*time.Minute),
		seenBlockConfirmation:        services.NewHashHistory("blockConfirmation", 30*time.Minute),
		clock:                        clock,
		timeStarted:                  clock.Now(),
		gatewayPeers:                 GeneratePeers(peersInfo),
		gatewayPublicKey:             gatewayPublicKeyStr,
		staticEnodesCount:            staticEnodesCount,
		transactionSlotStartDuration: transactionSlotStartDuration,
		transactionSlotEndDuration:   transactionSlotEndDuration,
		sdn:                          sdn,
		sslCerts:                     sslCerts,
		blockTime:                    blockTime,
	}

	if polygonHeimdallEndpoint != "" {
		g.polygonValidatorInfoManager = bor.NewSprintManager(parent, &g.wsManager, bor.NewHeimdallSpanner(parent, polygonHeimdallEndpoint))
	} else {
		g.polygonValidatorInfoManager = nil
	}

	if bxConfig.BlockchainNetwork == bxgateway.BSCMainnet || bxConfig.BlockchainNetwork == bxgateway.PolygonMainnet {
		g.validatorStatusMap = syncmap.NewStringMapOf[bool]()
		g.nextValidatorMap = orderedmap.New()
	}

	if bxConfig.BlockchainNetwork == bxgateway.BSCMainnet {
		g.validatorListMap = syncmap.NewIntegerMapOf[uint64, []string]()
		g.validatorListReady = false
		g.bscTxClient = &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost:     100,
				MaxIdleConnsPerHost: 100,
				MaxIdleConns:        100,
				IdleConnTimeout:     0 * time.Second,
			},
			Timeout: 60 * time.Second,
		}
	}

	g.asyncMsgChannel = services.NewAsyncMsgChannel(g)
	g.mevBundleDispatcher = bundle.NewDispatcher(bxConfig.MEVBuilders, bxConfig.MEVMaxProfitBuilder, bxConfig.ProcessMegaBundle)

	// create tx store service pass to eth client
	g.bdnStats = bxmessage.NewBDNStats(blockchainPeers, recommendedPeers)
	g.burstLimiter = services.NewAccountBurstLimiter(g.clock)

	// set empty default stats, Run function will override it
	g.stats = statistics.NewStats(false, "127.0.0.1", "", nil, false)
	g.txsQueue = services.NewMsgQueue(runtime.NumCPU()*2, bxgateway.TXQueueChannelSize, g.msgAdapter)
	g.txsOrderQueue = services.NewMsgQueue(1, bxgateway.TXQueueChannelSize, g.msgAdapter)

	return g, nil
}

func (g *gateway) msgAdapter(msg bxmessage.Message, source connections.Conn, waitingDuration time.Duration, workerChannelPosition int) {
	if tx, ok := msg.(*bxmessage.Tx); ok {
		tx.SetProcessingStats(waitingDuration, workerChannelPosition)
		g.processTransaction(tx, source)
	}
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
		NodeType:             gatewayType.String(),
		GatewayMode:          string(gatewayMode),
		ExternalIP:           bxConfig.ExternalIP,
		ExternalPort:         bxConfig.ExternalPort,
		BlockchainIP:         blockchainPeerEndpoint.IP,
		BlockchainPeers:      gatewayPeers,
		NodePublicKey:        blockchainPeerEndpoint.PublicKey,
		BlockchainPort:       blockchainPeerEndpoint.Port,
		ProgramName:          types.BloxrouteGoGateway,
		SourceVersion:        version.BuildVersion,
		IsDocker:             isDocker,
		Hostname:             hostname,
		OsVersion:            runtime.GOOS,
		ProtocolVersion:      bxmessage.CurrentProtocol,
		IsGatewayMiner:       bxConfig.BlocksOnly,
		NodeStartTime:        time.Now().Format(bxgateway.TimeLayoutISO),
		StartupArgs:          strings.Join(os.Args[1:], " "),
		BlockchainRPCEnabled: bxConfig.EnableBlockchainRPC,
	}

	sdn := connections.NewSDNHTTP(&sslCerts, bxConfig.SDNURL, nodeModel, bxConfig.DataDir)

	err = sdn.InitGateway(bxgateway.Ethereum, bxConfig.BlockchainNetwork)
	if err != nil {
		return nil, nil, err
	}

	accountModel := sdn.AccountModel()

	accountBuilders := make(map[string]bool)
	for _, builder := range accountModel.MEVBuilders {
		accountBuilders[builder] = true
	}

	// Check if the account is allowed to run the mev builder
	for builder := range bxConfig.MEVBuilders {
		if !accountBuilders[builder] {
			return nil, nil, fmt.Errorf("account %v is not allowed to run %v mev builder, closing the gateway. Please contact support@bloxroute.com to enable running this mev builder", accountModel.AccountID, builder)
		}
	}

	if uint64(staticEnodesCount) < uint64(accountModel.MinAllowedNodes.MsgQuota.Limit) {
		if staticEnodesCount == 0 {
			panic(fmt.Sprintf("Account %v is not allowed to run a gateway without node. Please check prior log entries for the reason the gateway is not connected to the node",
				accountModel.AccountID))
		}

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

	// start workers for handlingBridgeMessage per the number of dynamicDial connections + static connections
	// workers should be 75% of the connections + 1 worker to support block processing in parallel
	bridgeWorkers := int(math.Ceil(float64(g.staticEnodesCount)+float64(g.sdn.AccountModel().InboundNodeConnections.MsgQuota.Limit)*0.75)) + 1
	for i := 0; i < bridgeWorkers; i++ {
		go g.handleBridgeMessages()
	}

	go g.TxStore.Start()
	go g.updateValidatorStateMap()

	if g.BxConfig.NoStats {
		g.stats = statistics.NoStats{}
	} else {
		g.stats = statistics.NewStats(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, g.sdn.NodeID(), g.sdn.Networks(), g.BxConfig.LogNetworkContent)
	}

	sslCert := g.sslCerts
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)

	blockchainNetwork, err := g.sdn.FindNetwork(networkNum)
	if err != nil {
		return fmt.Errorf("failed to find the blockchainNetwork with networkNum %v, %v", networkNum, err)
	}

	g.feedManager = servers.NewFeedManager(g.context, g, g.feedManagerChan, services.NewNoOpSubscriptionServices(), networkNum,
		blockchainNetwork.DefaultAttributes.NetworkID, g.sdn.NodeModel().NodeID,
		g.wsManager, accountModel, g.sdn.FetchCustomerAccountModel,
		sslCert.PrivateCertFile(), sslCert.PrivateKeyFile(), *g.BxConfig, g.stats, g.nextValidatorMap, g.validatorStatusMap,
	)

	g.grpcHandler = servers.NewGrpcHandler(g.feedManager)

	// start feed manager if websocket or gRPC is enabled
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled {
		g.feedManager.Start()
	}

	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled {
		clientHandler := servers.NewClientHandler(g.feedManager, nil, servers.NewHTTPServer(g.feedManager, g.BxConfig.HTTPPort), g.BxConfig.EnableBlockchainRPC, g.sdn.GetQuotaUsage, log.WithFields(log.Fields{
			"component": "gatewayClientHandler",
		}), &g.BxConfig.PendingTxsSourceFromNode, g.authorize)
		go clientHandler.ManageWSServer(g.BxConfig.ManageWSServer)
		go clientHandler.ManageHTTPServer(g.context)
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

	if networkNum == bxgateway.PolygonMainnetNum && g.polygonValidatorInfoManager != nil {
		// running as goroutine to not block starting of node
		go func() {
			if retryErr := backoff.RetryNotify(
				g.polygonValidatorInfoManager.Run,
				bor.Retry(),
				func(err error, duration time.Duration) {
					log.Tracef("failed to start polygonValidatorInfoManager: %v, retry in %s", err, duration.String())
				},
			); retryErr != nil {
				log.Warnf("failed to start polygonValidatorInfoManager: %v", retryErr)
			}
		}()
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
	results := types.BroadcastResults{}

	g.ConnectionsLock.RLock()
	for _, conn := range g.Connections {
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
	g.ConnectionsLock.RUnlock()

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
		ExecutionLayerForks:     blockchainAttributes.ExecutionLayerForks,
		IgnoreBlockTimeout:      ignoreBlockTimeout,
		BlockConfirmationsCount: blockConfirmationsCount,
	}

	return g.bridge.UpdateNetworkConfig(ethConfig)
}

func (g *gateway) queryEpochBlock(height uint64) error {
	for _, wsProvider := range g.wsManager.Providers() {
		if wsProvider.IsOpen() {
			log.Debugf("blocking request to rpc")

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

func (g *gateway) cleanUpNextValidatorMap(currentHeight uint64) {
	// remove all wallet address of existing blocks
	toRemove := make([]uint64, 0)
	for pair := g.nextValidatorMap.Oldest(); pair != nil; pair = pair.Next() {
		if int(currentHeight) >= int(pair.Key.(uint64)) {
			toRemove = append(toRemove, pair.Key.(uint64))
		}
	}

	for _, height := range toRemove {
		g.nextValidatorMap.Delete(height)
	}
}

func (g *gateway) generateBSCValidator(blockHeight uint64) []*types.FutureValidatorInfo {
	vi := blockchain.DefaultValidatorInfo(blockHeight)

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
	previousEpochValidatorList := prevEpochValidatorList

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
	currentEpochValidatorList = currEpochValidatorList
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
			vi[i-1].Accessible = accessible
		}
	}

	g.cleanUpNextValidatorMap(blockHeight)
	go g.reevaluatePendingBSCNextValidatorTx()
	return vi
}

func (g *gateway) reevaluatePendingBSCNextValidatorTx() {
	now := time.Now()
	g.feedManager.LockPendingNextValidatorTxs()
	defer g.feedManager.UnlockPendingNextValidatorTxs()

	pendingNextValidatorTxsMap := g.feedManager.GetPendingNextValidatorTxs()
	for txHash, txInfo := range pendingNextValidatorTxsMap {
		fallback := time.Duration(uint64(txInfo.Fallback) * bxgateway.MillisecondsToNanosecondsMultiplier)
		// if fallback time is not reached or if fallback is 0 meaning we don't have fallback deadline
		if fallbackTimeNotReached := now.Before(txInfo.TimeOfRequest.Add(fallback)); fallbackTimeNotReached || txInfo.Fallback == 0 {
			timeSinceRequest := now.Sub(txInfo.TimeOfRequest)
			adjustedFallback := fallback - timeSinceRequest

			firstValidatorInaccessible, err := servers.ProcessNextValidatorTx(txInfo.Tx, uint16(adjustedFallback.Milliseconds()), g.nextValidatorMap, g.validatorStatusMap, txInfo.Tx.GetNetworkNum(), txInfo.Source, pendingNextValidatorTxsMap)
			delete(pendingNextValidatorTxsMap, txHash)

			if err != nil {
				log.Errorf("failed to reevaluate next validator tx %v: %v", txHash, err)
			}

			// send with adjusted fallback regardless of if validator is accessible
			if firstValidatorInaccessible {
				txInfo.Tx.RemoveFlags(types.TFNextValidator)
				txInfo.Tx.SetFallback(0)
			}
			err = g.HandleMsg(txInfo.Tx, txInfo.Source, connections.RunForeground)
			if err != nil {
				log.Errorf("failed to process reevaluated next validator tx %v: %v", txHash, err)
			}
			continue
		} else {
			// should have already been sent by gateway
			delete(pendingNextValidatorTxsMap, txHash)
		}
	}
}

func (g *gateway) generatePolygonValidator(bxBlock *types.BxBlock) []*types.FutureValidatorInfo {
	blockHeight := bxBlock.Number.Uint64()

	if g.validatorStatusMap == nil || g.wsManager == nil || g.polygonValidatorInfoManager == nil || !g.polygonValidatorInfoManager.IsRunning() {
		return blockchain.DefaultValidatorInfo(blockHeight)
	}

	blockSrc, err := g.bridge.BlockBDNtoBlockchain(bxBlock)
	if err != nil {
		log.Debugf("failed to convert BDN block (%v) to blockchain block: %v", bxBlock, err)

		return blockchain.DefaultValidatorInfo(blockHeight)
	}

	block, ok := blockSrc.(*eth.BlockInfo)
	if !ok {
		log.Debugf("failed to convert BDN block (%v) to blockchain block: %v", bxBlock, err)

		return blockchain.DefaultValidatorInfo(blockHeight)
	}

	validatorInfo := g.polygonValidatorInfoManager.FutureValidators(block.Block.Header())

	for _, info := range validatorInfo {
		if info.WalletID == "nil" {
			break
		}

		// nextValidatorMap is simulating a queue with height as expiration key.
		// Regardless of the accessible status, next walletID will be appended to the queue.
		g.nextValidatorMap.Set(info.BlockHeight, info.WalletID)

		accessible, exist := g.validatorStatusMap.Load(info.WalletID)
		if exist {
			info.Accessible = accessible
		}
	}

	g.cleanUpNextValidatorMap(blockHeight)

	return validatorInfo[:]
}

func (g *gateway) generateFutureValidatorInfo(block *types.BxBlock) []*types.FutureValidatorInfo {
	g.validatorInfoUpdateLock.Lock()
	defer g.validatorInfoUpdateLock.Unlock()

	if block.Number.Int64() <= g.latestValidatorInfoHeight {
		return g.latestValidatorInfo
	}
	g.latestValidatorInfoHeight = block.Number.Int64()

	switch g.sdn.NetworkNum() {
	case bxgateway.PolygonMainnetNum:
		g.latestValidatorInfo = g.generatePolygonValidator(block)
		return g.latestValidatorInfo
	case bxgateway.BSCMainnetNum:
		g.latestValidatorInfo = g.generateBSCValidator(block.Number.Uint64())
		return g.latestValidatorInfo
	default:
		return nil
	}
}

func (g *gateway) publishBlock(bxBlock *types.BxBlock, nodeSource *connections.Blockchain, info []*types.FutureValidatorInfo, isBlockchainBlock bool) error {

	// publishing a block means extracting the sender for all the block transactions which is heavy.
	// if there are no active block related feed subscribers we can skip this.
	if !g.feedManager.NeedBlocks() {
		return nil
	}

	// Check if received not stale block from BDN
	if !isBlockchainBlock {
		blockHeight := int(bxBlock.Number.Int64())
		if len(g.blockchainPeers) > 0 && blockHeight < g.bestBlockHeight {
			log.Debugf("block %v is too far behind best block height %v from node - not publishing", bxBlock, g.bestBlockHeight)
			return nil
		}
		if g.bestBlockHeight != 0 && utils.Abs(blockHeight-g.bestBlockHeight) > bxgateway.BDNBlocksMaxBlocksAway {
			if blockHeight > g.bestBlockHeight {
				g.bdnBlocksSkipCount++
			}
			if g.bdnBlocksSkipCount <= bxgateway.MaxOldBDNBlocksToSkipPublish {
				log.Debugf("block %v is too far away from best block height %v - not publishing", bxBlock, g.bestBlockHeight)
				return nil
			}
			log.Debugf("publishing block %v from BDN that is far away from current best block height %v - resetting bestBlockHeight to zero", bxBlock, g.bestBlockHeight)
			g.bestBlockHeight = 0
		}
		g.bdnBlocksSkipCount = 0
		if len(g.blockchainPeers) == 0 && blockHeight > g.bestBlockHeight {
			g.bestBlockHeight = blockHeight
		}
	}

	if err := g.notifyBlockFeeds(bxBlock, nodeSource, info, isBlockchainBlock); err != nil {
		return fmt.Errorf("cannot notify block feeds: %v", err)
	}

	return nil
}

func (g *gateway) notifyBlockFeeds(bxBlock *types.BxBlock, nodeSource *connections.Blockchain, info []*types.FutureValidatorInfo, isBlockchainBlock bool) error {
	// Not optimal. Block -> BxBlock -> Block
	block, err := g.bridge.BlockBDNtoBlockchain(bxBlock)
	if err != nil {
		return fmt.Errorf("cannot convert BxBlock to blockchain block: %v", err)
	}

	notifyEthBlockFeeds := func(block *ethtypes.Block, nodeSource *connections.Blockchain, info []*types.FutureValidatorInfo, isBlockchainBlock bool) error {
		ethNotification, err := types.NewEthBlockNotification(common.Hash(bxBlock.Hash()), block, info)
		if err != nil {
			return err
		}

		if g.bdnBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute) {
			// Send ETH notifications to BDN feed even if source is blockchain
			notification := ethNotification.Clone()
			notification.SetNotificationType(types.BDNBlocksFeed)
			g.notify(notification)

			// Waits response from node WS provider
			// Because it is in goroutine time will not be present in handleDuration
			go g.notifyTxReceiptsAndOnBlockFeeds(nodeSource, ethNotification)
		} else {
			log.Tracef("duplicate ETH block %v from %v for bdnBlocks", block.Hash(), nodeSource)
		}

		if isBlockchainBlock {
			if g.newBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute) {
				g.bestBlockHeight = int(block.Number().Int64())
				g.bdnBlocksSkipCount = 0

				notification := ethNotification.Clone()
				notification.SetNotificationType(types.NewBlocksFeed)
				g.notify(notification)
			} else {
				log.Tracef("duplicate ETH block %v from %v for newBlocks", block.Hash(), nodeSource)
			}
		}

		return nil
	}

	switch b := block.(type) {
	case interfaces.ReadOnlySignedBeaconBlock:
		beaconNotification, err := types.NewBeaconBlockNotification(b)
		if err != nil {
			return err
		}

		if g.bdnBlocks.SetIfAbsent(bxBlock.BeaconHash().String(), 15*time.Minute) {
			// Send beacon notifications to BDN feed even if source is blockchain
			notification := beaconNotification.Clone()
			notification.SetNotificationType(types.BDNBeaconBlocksFeed)
			g.notify(notification)
		}

		if isBlockchainBlock {
			if g.newBlocks.SetIfAbsent(bxBlock.BeaconHash().String(), 15*time.Minute) {
				notification := beaconNotification.Clone()
				notification.SetNotificationType(types.NewBeaconBlocksFeed)
				g.notify(notification)
			}
		} else {
			// Beacon sends both blocks, no reason to convert twice
			ethBlock, err := eth.BeaconBlockToEthBlock(b)
			if err != nil {
				return err
			}

			if err := notifyEthBlockFeeds(ethBlock, nodeSource, info, isBlockchainBlock); err != nil {
				return err
			}
		}
	case *eth.BlockInfo:
		if err := notifyEthBlockFeeds(b.Block, nodeSource, info, isBlockchainBlock); err != nil {
			return err
		}
	}

	return nil
}

func (g *gateway) notifyTxReceiptsAndOnBlockFeeds(nodeSource *connections.Blockchain, ethNotification *types.EthBlockNotification) {
	var nodeEndpoint *types.NodeEndpoint
	if nodeSource != nil { // from blockchain node
		e := nodeSource.NodeEndpoint()
		nodeEndpoint = &e
	}

	wsProvider, ok := g.wsManager.ProviderWithBlock(nodeEndpoint, ethNotification.Header.GetNumber())
	if !ok {
		return
	}

	sourceEndpoint := wsProvider.BlockchainPeerEndpoint()

	notification := ethNotification.Clone()
	notification.SetNotificationType(types.OnBlockFeed)
	notification.SetSource(&sourceEndpoint)
	g.notify(notification)

	notification = ethNotification.Clone()
	notification.SetNotificationType(types.TxReceiptsFeed)
	notification.SetSource(&sourceEndpoint)
	g.notify(notification)
}

func (g *gateway) publishPendingTx(txHash types.SHA256Hash, bxTx *types.BxTransaction, fromNode bool) {
	// check if this transaction was seen before and has validators_only / next_validator flag, don't publish it to pending txs
	tx, ok := g.TxStore.Get(txHash)
	if ok && (tx.Flags().IsNextValidator() || tx.Flags().IsValidatorsOnly()) {
		return
	}

	strTxHash := txHash.String()

	if g.pendingTxs.Exists(strTxHash) {
		return
	}

	if fromNode || g.possiblePendingTxs.Exists(strTxHash) {
		if bxTx != nil && bxTx.HasContent() {
			// if already has tx content, tx is pending and notify it
			g.notify(types.CreatePendingTransactionNotification(bxTx))
			g.pendingTxs.Add(strTxHash, 15*time.Minute)
		} else if fromNode {
			// not asking for tx content as we expect it to happen anyway
			g.possiblePendingTxs.Add(strTxHash, 15*time.Minute)
		}
	}
}

func (g *gateway) handleBridgeMessages() error {
	var err error
	for {
		select {
		case txsFromNode := <-g.bridge.ReceiveNodeTransactions():
			traceIfSlow(func() {
				// if we are not yet synced with relay - ignore the transactions from the node
				if !g.isSyncWithRelay() {
					return
				}
				receiveTime := g.clock.Now()
				blockchainConnection := connections.NewBlockchainConn(txsFromNode.PeerEndpoint)
				for _, blockchainTx := range txsFromNode.Transactions {
					tx := bxmessage.NewTx(blockchainTx.Hash(), blockchainTx.Content(), g.sdn.NetworkNum(), types.TFLocalRegion, types.EmptyAccountID)
					tx.SetReceiveTime(receiveTime.Add(-time.Microsecond))
					tx.SetTimestamp(receiveTime)
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
					if !txAnnouncement.PeerEndpoint.IsDynamic() {
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
			// either gateway is none elite and running no active p2p blockchain connection
			// or gateway is not running with web3 bridge enabled (currently enterprise and above)
			if !g.sdn.AccountTier().IsElite() && !(g.BxConfig.EnableBlockchainRPC && g.sdn.AccountTier().IsEnterprise()) {
				// TODO should fix code to stop gateway appropriately
				log.Errorf("Gateway does not have an active blockchain connection. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
				log.Exit(0)
			}
		case confirmBlock := <-g.bridge.ReceiveConfirmedBlockFromNode():
			if !g.BxConfig.NoBlocks {
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
					if err = g.HandleMsg(&bcnfMsg, blockchainConnection, connections.RunBackground); err != nil {
						log.Errorf("Error handling block confirmation message: %v", err)
					}
				}, fmt.Sprintf("ReceiveConfirmedBlockFromNode hash=[%s]", confirmBlock.Block.Hash()), confirmBlock.PeerEndpoint.String(), 1)
			}
		case blockchainBlock := <-g.bridge.ReceiveBlockFromNode():
			if !g.BxConfig.NoBlocks {
				traceIfSlow(func() { g.handleBlockFromBlockchain(blockchainBlock) },
					fmt.Sprintf("handleBlockFromBlockchain hash=[%s]", blockchainBlock.Block.Hash()), blockchainBlock.PeerEndpoint.String(), 1)
			}
		}
	}
}

func (g *gateway) NodeStatus() connections.NodeStatus {
	var capabilities types.CapabilityFlags

	if len(g.BxConfig.MEVBuilders) > 0 {
		capabilities |= types.CapabilityMEVBuilder
	}

	if g.BxConfig.GatewayMode.IsBDN() {
		capabilities |= types.CapabilityBDN
	}

	if g.BxConfig.EnableBlockchainRPC {
		capabilities |= types.CapabilityBlockchainRPCEnabled
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

	switch typedMsg := msg.(type) {
	case *bxmessage.Tx:
		// insert to order queue if tx is flagged as send to node and no txs to blockchain is false, and we have static peers or dynamic peers
		if typedMsg.Flags().ShouldDeliverToNode() && !g.BxConfig.NoTxsToBlockchain && (g.staticEnodesCount > 0 || g.BxConfig.EnableDynamicPeers) {
			err = g.txsOrderQueue.Insert(typedMsg, source)
		} else {
			err = g.txsQueue.Insert(typedMsg, source)
		}
		if err != nil {
			log.Errorf("%v, discarding tx from %v, tx hash %v", err, source, typedMsg.Hash().String())
		}
	case *bxmessage.ValidatorUpdates:
		g.processValidatorUpdate(typedMsg, source)
	case *bxmessage.Broadcast:
		// handle in a go-routing so tx flow from relay will not be delayed
		if !g.BxConfig.NoBlocks {
			go g.processBroadcast(typedMsg, source)
		}
	case *bxmessage.RefreshBlockchainNetwork:
		go g.processBlockchainNetworkUpdate(source)
	case *bxmessage.Txs:
		// TODO: check if this is the message type we want to use?
		for _, txsItem := range typedMsg.Items() {
			g.TxStore.Add(txsItem.Hash, txsItem.Content, txsItem.ShortID, g.sdn.NetworkNum(), false, 0, time.Now(), 0, types.EmptySender)
		}
	case *bxmessage.SyncDone:
		g.setSyncWithRelay()
		err = g.Bx.HandleMsg(msg, source)
	case *bxmessage.MEVBundle:
		go g.handleMEVBundleMessage(*typedMsg, source)
	case *bxmessage.ErrorNotification:
		source.Log().Errorf("received an error notification %v. terminating the gateway", typedMsg.Reason)
		// TODO should also close the gateway while notify the bridge and other go routine (web socket server, ...)
		log.Exit(0)
	case *bxmessage.Hello:
		source.Log().Tracef("received hello msg")
		if connections.IsRelay(source.GetConnectionType()) && g.sdn.AccountModel().Miner {
			// check if it has blockchain connection
			go func() {
				if g.BxConfig.ForwardTransactionEndpoint != "" {
					g.sdn.SendNodeEvent(
						sdnmessage.NewAddAccessibleGatewayEvent(g.sdn.NodeID(), fmt.Sprintf(
							"gateway %v (%v) established connection with relay %v",
							g.sdn.NodeModel().ExternalIP, g.accountID, source.GetPeerIP()), g.clock.Now().String()),
						g.sdn.NodeID())
				} else if g.gatewayHasBlockchainConnection() {
					g.sdn.SendNodeEvent(
						sdnmessage.NewAddAccessibleGatewayEvent(g.sdn.NodeID(), fmt.Sprintf(
							"gateway %v (%v) established connection with relay %v:%v and connected to at least one node",
							g.sdn.NodeModel().ExternalIP, g.accountID, source.GetPeerIP(), source.GetPeerPort(),
						), g.clock.Now().String()),
						g.sdn.NodeID())
				} else {
					g.sdn.SendNodeEvent(
						sdnmessage.NewRemoveAccessibleGatewayEvent(g.sdn.NodeID(), fmt.Sprintf(
							"gateway %v (%v) established connection with relay %v:%v but not connected to any node",
							g.sdn.NodeModel().ExternalIP, g.accountID, source.GetPeerIP(), source.GetPeerPort(),
						), g.clock.Now().String()),
						g.sdn.NodeID())
				}
			}()
		}

	case *bxmessage.BlockConfirmation:
		hashString := typedMsg.Hash().String()
		if g.seenBlockConfirmation.SetIfAbsent(hashString, 30*time.Minute) {
			if !g.BxConfig.SendConfirmation {
				log.Debug("gateway is not sending block confirm message to relay")
			} else if source.GetConnectionType() == utils.Blockchain {
				log.Tracef("gateway broadcasting block confirmation of block %v to relays", hashString)
				g.broadcast(typedMsg, source, utils.Relay)
			}
			_ = g.Bx.HandleMsg(msg, source)
		}
	default:
		err = g.Bx.HandleMsg(msg, source)
	}

	return err
}

func (g *gateway) gatewayHasBlockchainConnection() bool {
	err := g.bridge.SendNodeConnectionCheckRequest()
	if err == nil {
		select {
		case status := <-g.bridge.ReceiveNodeConnectionCheckResponse():
			log.Tracef("received status from %v:%v", status.IP, status.Port)
			return true
		case <-time.After(time.Second):
			return false
		}

	} else {
		log.Errorf("failed to send blockchain status request when received hello msg from relay: %v", err)
	}

	return false
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

	// update the next block time
	g.nextBlockTime = startTime.Add(g.blockTime).Round(time.Second)

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

func (g *gateway) processBlockchainNetworkUpdate(source connections.Conn) {
	if err := g.sdn.FetchBlockchainNetwork(); err != nil {
		source.Log().Errorf("could not fetch blockchain network config: %v", err)
		return
	}

	if err := g.pushBlockchainConfig(); err != nil {
		source.Log().Errorf("could not push blockchain network config: %v", err)
		return
	}
}

func (g *gateway) processValidatorUpdate(msg *bxmessage.ValidatorUpdates, source connections.Conn) {
	if g.sdn.NetworkNum() != msg.GetNetworkNum() {
		source.Log().Debugf("ignore validator update message for %v network number from relay, gateway is %v", msg.GetNetworkNum(), g.sdn.NetworkNum())
		return
	}
	onlineList := msg.GetOnlineList()

	removeValidator := func(key string, value bool) bool {
		if !utils.Exists(key, onlineList) {
			g.validatorStatusMap.Delete(key)
		}
		return true
	}
	g.validatorStatusMap.Range(removeValidator)

	for _, addr := range onlineList {
		g.validatorStatusMap.Store(strings.ToLower(addr), true)
	}
}

func (g *gateway) processTransaction(tx *bxmessage.Tx, source connections.Conn) {
	var (
		sentToBDN            bool
		sentToBlockchainNode bool

		frontRunProtectionDelay time.Duration

		broadcastRes types.BroadcastResults

		eventName = "TxProcessedByGatewayFromPeerIgnoreSeen"
	)

	startTime := time.Now()
	txTime := tx.Timestamp()
	peerIP := source.GetPeerIP()
	peerPort := source.GetPeerPort()
	sourceEndpoint := types.NodeEndpoint{IP: peerIP, Port: int(peerPort), PublicKey: source.GetPeerEnode()}

	if connEndpoint, ok := source.(connections.EndpointConn); ok {
		sourceEndpoint = connEndpoint.NodeEndpoint()
	}

	connectionType := source.GetConnectionType()
	isRelay := connections.IsRelay(connectionType)

	sender := tx.Sender()
	// we add the transaction to TxStore with current time, so we can measure time difference to node announcement/confirmation
	txResult := g.TxStore.Add(tx.Hash(), tx.Content(), tx.ShortID(), tx.GetNetworkNum(), !(isRelay || (connections.IsGrpc(connectionType) && sender != types.EmptySender)), tx.Flags(), g.clock.Now(), 0, sender)

	nodeID := source.GetNodeID()

	switch {
	case txResult.FailedValidation:
		eventName = "TxValidationFailedStructure"
	case txResult.NewContent && txResult.Transaction.Flags().IsReuseSenderNonce() && tx.ShortID() == types.ShortIDEmpty:
		eventName = "TxReuseSenderNonce"
		log.Trace(txResult.DebugData)
	case txResult.AlreadySeen:
		log.Tracef("received already Seen transaction %v from %v:%v (%v) and account id %v", tx.Hash(), peerIP, peerPort, nodeID, source.GetAccountID())
	case txResult.NewContent || txResult.NewSID || txResult.Reprocess:
		eventName = "TxProcessedByGatewayFromPeer"
		if txResult.NewContent || txResult.Reprocess {
			validatorsOnlyTxFromCloudAPI := connectionType == utils.CloudAPI && tx.Flags().IsValidatorsOnly()
			nextValidatorTxFromCloudAPI := connectionType == utils.CloudAPI && tx.Flags().IsNextValidator()
			if txResult.NewContent && !tx.Flags().IsValidatorsOnly() && !tx.Flags().IsNextValidator() {
				newTxsNotification := types.CreateNewTransactionNotification(txResult.Transaction)
				g.notify(newTxsNotification)
				if !sourceEndpoint.IsDynamic() {
					g.publishPendingTx(txResult.Transaction.Hash(), txResult.Transaction, connectionType == utils.Blockchain)
				}
			}

			if !isRelay {
				if connectionType == utils.Blockchain {
					g.bdnStats.LogNewTxFromNode(sourceEndpoint)
				}

				paidTx := tx.Flags().IsPaid()
				var (
					allowed  bool
					behavior sdnmessage.BDNServiceBehaviorType
				)
				if connectionType == utils.CloudAPI {
					allowed, behavior = g.burstLimiter.AllowTransaction(source.GetAccountID(), paidTx)
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
					tx.SetSender(txResult.Transaction.Sender())
					// set timestamp so relay can analyze communication delay
					tx.SetTimestamp(g.clock.Now())
					broadcastRes = g.broadcast(tx, source, utils.RelayTransaction)
					sentToBDN = true
				}
			}

			// if this gateway is the associated validator or the tx was sent after the fallback time, need to send the transaction to the node
			if tx.Flags().IsValidatorsOnly() || tx.Flags().IsNextValidator() || tx.Flags().IsNextValidatorRebroadcast() {
				if g.sdn.AccountModel().Miner {
					tx.AddFlags(types.TFDeliverToNode)
				} else {
					tx.RemoveFlags(types.TFDeliverToNode)
				}
			}

			shouldSendTxFromNodeToOtherNodes := connectionType == utils.Blockchain && len(g.blockchainPeers) > 1 && !g.BxConfig.NoTxsToBlockchain
			shouldSendTxFromBDNToNodes := g.shouldSendTxFromBDNToNodes(connectionType, tx, validatorsOnlyTxFromCloudAPI, nextValidatorTxFromCloudAPI) && !g.BxConfig.NoTxsToBlockchain

			if shouldSendTxFromNodeToOtherNodes || shouldSendTxFromBDNToNodes {
				txsToDeliverToNodes := blockchain.Transactions{
					Transactions:   []*types.BxTransaction{txResult.Transaction},
					PeerEndpoint:   sourceEndpoint,
					ConnectionType: connectionType,
				}

				// Check for front run protection, if tx arrive too earlier, we send it with delay, if arrive too late, do not send to node
				if !tx.Flags().IsNextValidatorRebroadcast() && tx.Flags().IsFrontRunningProtection() && tx.Flags().IsNextValidator() {
					now := time.Now()
					deadline := g.nextBlockTime.Add(-time.Millisecond * time.Duration(g.transactionSlotEndDuration))
					slotBegin := g.nextBlockTime.Add(-time.Millisecond * time.Duration(g.transactionSlotStartDuration))
					if now.Before(slotBegin) {
						frontRunProtectionDelay = slotBegin.Sub(now)
						sentToBlockchainNode = true
						log.Debugf("received next validator tx %v before slot start (%v), next block time is %v, wait for %v to prevent front running", tx.String(), slotBegin.String(), g.nextBlockTime.String(), frontRunProtectionDelay.String())
					} else if now.After(deadline) {
						sentToBlockchainNode = false
						log.Errorf("received next validator tx %v from %v after slot deadline (%v), next block time is %v, will not send the tx", tx.String(), source.GetPeerIP(), deadline.String(), g.nextBlockTime.String())
						break
					}

					time.AfterFunc(frontRunProtectionDelay, func() {
						err := g.bridge.SendTransactionsFromBDN(txsToDeliverToNodes)
						if err != nil {
							log.Errorf("failed to send transaction %v from BDN to bridge - %v", txResult.Transaction.Hash(), err)
						}

						if shouldSendTxFromNodeToOtherNodes {
							g.bdnStats.LogTxSentToAllNodesExceptSourceNode(sourceEndpoint)
						}

						log.Debugf("tx %v sent to blockchain, flag %v, with front run protection delay %v", tx.Hash().String(), tx.Flags(), frontRunProtectionDelay.String())
					})
				} else {
					err := g.bridge.SendTransactionsFromBDN(txsToDeliverToNodes)
					if err != nil {
						log.Errorf("failed to send transaction %v from BDN to bridge - %v", txResult.Transaction.Hash(), err)
					}

					sentToBlockchainNode = true
					if shouldSendTxFromNodeToOtherNodes {
						g.bdnStats.LogTxSentToAllNodesExceptSourceNode(sourceEndpoint)
					}

					log.Debugf("tx %v sent to blockchain, flag %v", tx.Hash().String(), tx.Flags())
				}
			}

			if source.GetNetworkNum() == bxgateway.BSCMainnetNum && (tx.Flags().IsValidatorsOnly() || tx.Flags().IsNextValidator()) {
				rawBytesString, err := getRawBytesStringFromTXMsg(tx)
				if err != nil {
					log.Errorf("failed to forward transaction %v to %v due to rpl decoding error %v", txResult.Transaction.Hash(), g.BxConfig.ForwardTransactionEndpoint, err)
				} else {
					go forwardBSCTx(g.bscTxClient, tx.Hash().String(), "0x"+rawBytesString, g.BxConfig.ForwardTransactionEndpoint, g.BxConfig.ForwardTransactionMethod)
				}
			}

			if isRelay && !txResult.Reprocess {
				g.bdnStats.LogNewTxFromBDN()
			}

			// in case this is just a transaction with new short id,
			// it should not be logged
			if !txResult.NewSID {
				g.txTrace.Log(tx.Hash(), source)
			}
		}
	default:
		// duplicate transaction
		if txResult.Transaction.Flags().IsReuseSenderNonce() {
			eventName = "TxReuseSenderNonceIgnoreSeen"
			log.Trace(txResult.DebugData)
		}
		if connectionType == utils.Blockchain {
			g.bdnStats.LogDuplicateTxFromNode(sourceEndpoint)
		}
	}

	statsStart := time.Now()
	g.stats.AddTxsByShortIDsEvent(eventName, source, txResult.Transaction, tx.ShortID(), nodeID, broadcastRes.RelevantPeers, broadcastRes.SentGatewayPeers, startTime, tx.GetPriority(), txResult.DebugData)
	statsDuration := time.Since(statsStart)
	// usage of log.WithFields 7 times slower than usage of direct log.Tracef
	handlingTime := g.clock.Now().Sub(tx.ReceiveTime()).Microseconds() - tx.WaitDuration().Microseconds()
	log.Tracef(
		"msgTx: from %v, hash %v, nonce %v, flags %v, new Tx %v, new content %v, new shortid %v, event %v,"+
			" sentToBDN: %v, sentPeersNum %v, sentToBlockchainNode: %v, handling duration %v, sender %v,"+
			" networkDuration %v, statsDuration %v, nextValidator enabled %v,fallback duration %v,"+
			" next validator fallback %v, front run protection delay %v, waiting duration %v, txs in NetworkChannel %v, txs in processing channel %v, msg len %v",
		source, tx.Hash(), txResult.Nonce, tx.Flags(), txResult.NewTx, txResult.NewContent, txResult.NewSID, eventName,
		sentToBDN, broadcastRes.SentPeers, sentToBlockchainNode, handlingTime, txResult.Transaction.Sender(),
		tx.ReceiveTime().Sub(txTime).Microseconds(), statsDuration, tx.Flags().IsNextValidator(), tx.Fallback(),
		tx.Flags().IsNextValidatorRebroadcast(), frontRunProtectionDelay.String(), tx.WaitDuration(), tx.NetworkChannelPosition(), tx.ProcessChannelPosition(), tx.BufLen(),
	)
}

// shouldSendTxFromBDNToNodes send to node if all are true
func (g *gateway) shouldSendTxFromBDNToNodes(connectionType utils.NodeType, tx *bxmessage.Tx, validatorsOnlyTxFromCloudAPI bool, nextValidatorTxFromCloudAPI bool) (send bool) {
	if connectionType == utils.Blockchain {
		return // Transaction is from blockchain node (transaction is not from Relay or RPC)
	}

	if len(g.blockchainPeers) == 0 {
		return // Gateway is not connected to nodes
	}

	if (!g.BxConfig.BlocksOnly && tx.Flags().ShouldDeliverToNode()) || // (Gateway didn't start with blocks only mode and DeliverToNode flag is on) OR
		(g.BxConfig.AllTransactions && !validatorsOnlyTxFromCloudAPI && !nextValidatorTxFromCloudAPI) { // OR (Gateway started with a flag to send all transactions and the tx is not sent from Cloud API for validators only)
		send = true
	}

	return
}

func (g *gateway) updateValidatorStateMap() {
	for newList := range g.bridge.ReceiveValidatorListInfo() {
		blockHeight := newList.BlockHeight
		g.validatorListMap.Delete(blockHeight - 400) // remove the validator list that doesn't need anymore
		g.validatorListMap.Store(blockHeight, newList.ValidatorList)
	}
}

func getRawBytesStringFromTXMsg(tx *bxmessage.Tx) (string, error) {
	var ethTransaction ethtypes.Transaction
	err := rlp.DecodeBytes(tx.Content(), &ethTransaction)
	if err != nil {
		return "", err
	}

	b, err := ethTransaction.MarshalBinary()
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

func (g *gateway) handleBlockFromBlockchain(blockchainBlock blockchain.BlockFromNode) {
	startTime := time.Now()

	bxBlock := blockchainBlock.Block
	source := connections.NewBlockchainConn(blockchainBlock.PeerEndpoint)

	g.bdnStats.LogNewBlockMessageFromNode(source.NodeEndpoint())

	broadcastMessage, usedShortIDs, err := g.blockProcessor.BxBlockToBroadcast(bxBlock, g.sdn.NetworkNum(), g.sdn.MinTxAge())
	if err != nil {
		if err == services.ErrAlreadyProcessed {
			source.Log().Debugf("received duplicate block %v, skipping", bxBlock.Hash())

			eventName := "GatewayReceivedBlockFromBlockchainNodeIgnoreSeen"
			if bxBlock.IsBeaconBlock() {
				eventName = "GatewayReceivedBeaconBlockFromBlockchainNodeIgnoreSeen"
			}

			g.stats.AddGatewayBlockEvent(eventName, source, bxBlock.Hash(), bxBlock.BeaconHash(), g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), 0, 0, len(bxBlock.Txs), len(usedShortIDs), bxBlock)
		} else {
			source.Log().Errorf("could not compress block: %v", err)
		}
	} else {
		// if not synced avoid sending to bdn (low compression rate block)
		if !g.isSyncWithRelay() {
			source.Log().Debugf("TxSync not completed. Not sending block %v to the bdn", bxBlock.Hash())
		} else {
			source.Log().Debugf("compressed %v from blockchain node: compressed %v short IDs", bxBlock, len(usedShortIDs))
			source.Log().Infof("propagating %v from blockchain node to BDN", bxBlock)

			_ = g.broadcast(broadcastMessage, source, utils.RelayBlock)

			g.bdnStats.LogNewBlockFromNode(source.NodeEndpoint())

			eventName := "GatewayReceivedBlockFromBlockchainNode"
			if bxBlock.IsBeaconBlock() {
				eventName = "GatewayReceivedBeaconBlockFromBlockchainNode"
			}

			g.stats.AddGatewayBlockEvent(eventName, source, bxBlock.Hash(), bxBlock.BeaconHash(), g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), int(broadcastMessage.Size(bxmessage.CurrentProtocol)), len(broadcastMessage.ShortIDs()), len(bxBlock.Txs), len(usedShortIDs), bxBlock)
		}
	}

	validatorInfo := g.generateFutureValidatorInfo(bxBlock)
	if err := g.publishBlock(bxBlock, &source, validatorInfo, !blockchainBlock.PeerEndpoint.IsDynamic()); err != nil {
		source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock, err)
	}
}

func (g *gateway) processBlockFromBDN(bxBlock *types.BxBlock) {
	err := g.bridge.SendBlockToNode(bxBlock)
	if err != nil {
		log.Errorf("unable to send block from BDN to node: %v", err)
	}

	g.bdnStats.LogNewBlockFromBDN(g.BxConfig.BlockchainNetwork)
	validatorInfo := g.generateFutureValidatorInfo(bxBlock)
	err = g.publishBlock(bxBlock, nil, validatorInfo, false)
	if err != nil {
		log.Errorf("Failed to publish BDN block with %v, block hash: %v, block height: %v", err, bxBlock.Hash(), bxBlock.Number)
	}
}

func (g *gateway) notify(notification types.Notification) {
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled {
		select {
		case g.feedManagerChan <- notification:
		default:
			log.Warnf("gateway feed channel is full. Can't add %v without blocking. Ignoring hash %v", reflect.TypeOf(notification), notification.GetHash())
		}
	}
}

func (g *gateway) handleMEVBundleMessage(mevBundle bxmessage.MEVBundle, source connections.Conn) {
	start := time.Now()
	blockNumber, err := strconv.ParseInt(strings.TrimPrefix(mevBundle.BlockNumber, "0x"), 16, 64)
	if err != nil {
		log.Errorf("failed to parse block number %v: %v", mevBundle.BlockNumber, err)
	}

	fromRelay := connections.IsRelay(source.GetConnectionType())

	if !g.seenMEVBundles.SetIfAbsent(mevBundle.Hash().String(), time.Minute*30) {
		eventName := "GatewayReceivedBundleFromBDNIgnoreSeen"
		if !fromRelay {
			eventName = "GatewayReceivedBundleFromFeedIgnoreSeen"
		}

		source.Log().Tracef("ignoring %s duration: %v ms, time in network: %v ms", mevBundle, time.Since(start).Milliseconds(), start.Sub(mevBundle.PerformanceTimestamp).Milliseconds())
		g.stats.AddGatewayBundleEvent(eventName, source, start, mevBundle.BundleHash, mevBundle.GetNetworkNum(), mevBundle.Names(), mevBundle.Frontrunning, mevBundle.UUID, uint64(blockNumber), mevBundle.MinTimestamp, mevBundle.MaxTimestamp)
		return
	}

	var event string
	if fromRelay {
		event = "GatewayReceivedBundleFromBDN"

		if err := g.mevBundleDispatcher.Dispatch(&mevBundle); err != nil {
			log.Errorf("failed to dispatch mev bundle %v: %v", mevBundle.BundleHash, err)
		}

		source.Log().Tracef("dispatching %s duration: %v ms, time in network: %v ms", mevBundle, time.Since(start).Milliseconds(), start.Sub(mevBundle.PerformanceTimestamp).Milliseconds())
	} else {
		event = "GatewayReceivedBundleFromFeed"

		// set timestamp as late as possible.
		mevBundle.PerformanceTimestamp = time.Now()
		broadcastRes := g.broadcast(&mevBundle, source, utils.RelayTransaction|utils.RelayProxy)

		source.Log().Tracef("broadcasting %s %s duration: %v ms, time in network: %v ms", mevBundle, broadcastRes, time.Since(start).Milliseconds(), start.Sub(mevBundle.PerformanceTimestamp).Milliseconds())
	}

	g.stats.AddGatewayBundleEvent(event, source, start, mevBundle.BundleHash, mevBundle.GetNetworkNum(), mevBundle.Names(), mevBundle.Frontrunning, mevBundle.UUID, uint64(blockNumber), mevBundle.MinTimestamp, mevBundle.MaxTimestamp)
}

func (g *gateway) Peers(ctx context.Context, req *pb.PeersRequest) (*pb.PeersReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, true)
	if err != nil {
		return nil, err
	}

	return g.Bx.Peers(ctx, req)
}

// DisconnectInboundPeer disconnect inbound peer from gateway
func (g *gateway) DisconnectInboundPeer(_ context.Context, req *pb.DisconnectInboundPeerRequest) (*pb.DisconnectInboundPeerReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, true)
	if err != nil {
		return nil, err
	}

	err = g.bridge.SendDisconnectEvent(types.NodeEndpoint{IP: req.PeerIp, Port: int(req.PeerPort), PublicKey: req.PublicKey})
	if err != nil {
		return &pb.DisconnectInboundPeerReply{Status: err.Error()}, err
	}
	return &pb.DisconnectInboundPeerReply{Status: fmt.Sprintf("Sent request to disconnect peer %v %v %v", req.PublicKey, req.PeerIp, req.PeerPort)}, err
}

const (
	connectionStatusConnected    = "connected"
	connectionStatusNotConnected = "not_connected"
)

func (g *gateway) validateAuthHeader(authHeader string, required bool, allowAccessToInternalGateway bool) error {
	var err error
	if authHeader == "" {
		if required {
			return fmt.Errorf("auth header is missing")
		}
		if g.sdn.AccountModel().AccountID == types.BloxrouteAccountID {
			return fmt.Errorf("could not connect to internal gateway without auth header")
		}
		authHeader = g.getHeaderFromGateway()
	}
	accountID, secretHash, err := utils.GetAccountIDSecretHashFromHeader(authHeader)
	if err != nil {
		return err
	}

	_, err = g.authorize(accountID, secretHash, allowAccessToInternalGateway)
	if err != nil {
		return err
	}
	return nil
}

func (g *gateway) getHeaderFromGateway() string {
	accountID := g.sdn.AccountModel().AccountID
	secretHash := g.sdn.AccountModel().SecretHash
	accountIDAndHash := fmt.Sprintf("%s:%s", accountID, secretHash)
	return base64.StdEncoding.EncodeToString([]byte(accountIDAndHash))
}

func (g *gateway) authorize(accountID types.AccountID, secretHash string, allowAccessToInternalGateway bool) (sdnmessage.Account, error) {
	// if gateway received request from a customer with a different account id, it should verify it with the SDN.
	// if the gateway does not have permission to verify account id (which mostly happen with external gateways),
	// SDN will return StatusUnauthorized and fail this connection. if SDN return any other error -
	// assuming the issue is with the SDN and set default enterprise account for the customer. in order to send request to the gateway,
	// customer must be enterprise / elite account
	var connectionAccountModel sdnmessage.Account
	var err error
	if accountID != g.sdn.AccountModel().AccountID {
		if !allowAccessToInternalGateway {
			return connectionAccountModel, fmt.Errorf("accountID %v is different from the node accountID %v", accountID, g.sdn.AccountModel().AccountID)
		}
		connectionAccountModel, err = g.sdn.FetchCustomerAccountModel(accountID)
		if err != nil {
			if strings.Contains(strconv.FormatInt(http.StatusUnauthorized, 10), err.Error()) {
				log.Errorf("account %v is not authorized to get other account %v information", g.sdn.AccountModel().AccountID, accountID)
				return connectionAccountModel, fmt.Errorf("account is not authorized to get other accounts information")
			}
			log.Errorf("failed to get customer account model, account id: %v, connectionSecretHash: %v, error: %v",
				accountID, secretHash, err)
			connectionAccountModel = sdnmessage.GetDefaultEliteAccount(time.Now().UTC())
			connectionAccountModel.AccountID = accountID
			connectionAccountModel.SecretHash = secretHash
		}
		if !connectionAccountModel.TierName.IsEnterprise() {
			log.Warnf("customer account %v must be enterprise / enterprise elite / ultra but it is %v", accountID, connectionAccountModel.TierName)
			return connectionAccountModel, fmt.Errorf("account must be enterprise / enterprise elite / ultra")
		}
	} else {
		connectionAccountModel = g.sdn.AccountModel()
	}

	if secretHash != connectionAccountModel.SecretHash && secretHash != "" {
		log.Errorf("account %v sent a different secret hash than set in the account model", accountID)
		return connectionAccountModel, fmt.Errorf("wrong value in the authorization header")
	}

	return connectionAccountModel, nil
}

func (g *gateway) Version(_ context.Context, req *pb.VersionRequest) (*pb.VersionReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, true)
	if err != nil {
		return nil, err
	}

	resp := &pb.VersionReply{
		Version:   version.BuildVersion,
		BuildDate: version.BuildDate,
	}
	return resp, nil
}

const bdn = "BDN"

func (g *gateway) Status(_ context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, true)
	if err != nil {
		return nil, err
	}

	var bdnConn = func() map[string]*pb.BDNConnStatus {
		var mp = make(map[string]*pb.BDNConnStatus)

		g.ConnectionsLock.RLock()
		for _, conn := range g.Connections {
			connectionType := conn.GetConnectionType()

			if connectionType&utils.Relay == 0 {
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

			peerIP := conn.GetPeerIP()

			if !conn.IsOpen() {
				mp[peerIP] = &pb.BDNConnStatus{
					Status: connectionStatusNotConnected,
				}

				continue
			}

			mp[peerIP] = &pb.BDNConnStatus{
				Status:      connectionStatusConnected,
				ConnectedAt: conn.GetConnectedAt().Format(time.RFC3339),
				Latency:     connectionLatency,
			}
		}
		g.ConnectionsLock.RUnlock()

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
					Dynamic: peer.IsDynamic(),
					Version: int64(peer.Version),
					Name:    peer.Name,
				}

				nstat, ok := nodeStats[peer.IPPort()]
				if ok {
					connStatus.IsConnected = nstat.IsConnected
					connStatus.ConnectedAt = peer.ConnectedAt
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
						Dynamic:     peer.Dynamic,
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
		QueueStats: &pb.QueuesStats{
			TxsQueueCount:      g.txsQueue.TxsCount(),
			TxsOrderQueueCount: g.txsOrderQueue.TxsCount(),
		},
	}

	return rsp, nil
}

func ipport(ip string, port int) string { return fmt.Sprintf("%s:%d", ip, port) }

func (g *gateway) Subscriptions(_ context.Context, req *pb.SubscriptionsRequest) (*pb.SubscriptionsReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, true)
	if err != nil {
		return nil, err
	}

	return g.feedManager.GetGrpcSubscriptionReply(), nil
}

func (g *gateway) BlxrTx(_ context.Context, req *pb.BlxrTxRequest) (*pb.BlxrTxReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, true, false)
	if err != nil {
		return nil, err
	}

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

func (g *gateway) BlxrBatchTX(_ context.Context, req *pb.BlxrBatchTXRequest) (*pb.BlxrBatchTXReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, true, false)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	var txHashes []*pb.TxIndex
	var txErrors []*pb.ErrorIndex
	transactions := req.GetTransactionsAndSenders()
	if len(transactions) > 2 {
		txError := "blxr-batch-tx currently supports a maximum of two transactions"
		txErrors = append(txErrors, &pb.ErrorIndex{Idx: 0, Error: txError})
		return &pb.BlxrBatchTXReply{TxErrors: txErrors}, nil
	}
	grpc := connections.NewRPCConn(g.accountID, "", g.sdn.NetworkNum(), utils.GRPC)

	blockchainNetwork, err := g.sdn.Networks().FindNetwork(g.sdn.NetworkNum())
	if err != nil {
		txErrors = append(txErrors, &pb.ErrorIndex{Idx: 0, Error: err.Error()})
	} else {
		networkNum := g.sdn.NetworkNum()
		accountID := g.sdn.AccountModel().AccountID
		for idx, txAndSender := range transactions {
			tx := txAndSender.GetTransaction()
			txContent, err := types.DecodeHex(tx)
			if err != nil {
				txErrors = append(txErrors, &pb.ErrorIndex{Idx: int32(idx), Error: fmt.Sprintf("failed to decode transaction %v sent via GRPC blxrtx: %v", tx, err)})
				continue
			}
			g.feedManager.LockPendingNextValidatorTxs()
			validTx, pendingReevaluation, err := servers.ValidateTxFromExternalSource(tx, txContent, req.ValidatorsOnly, blockchainNetwork.DefaultAttributes.NetworkID, req.NextValidator, uint16(req.Fallback), g.nextValidatorMap, g.validatorStatusMap, networkNum, accountID, req.NodeValidation, g.wsManager, grpc, g.feedManager.GetPendingNextValidatorTxs(), false)
			g.feedManager.UnlockPendingNextValidatorTxs()
			if err != nil {
				txErrors = append(txErrors, &pb.ErrorIndex{Idx: int32(idx), Error: err.Error()})
				continue
			}
			if pendingReevaluation {
				continue
			}
			var sender types.Sender
			copy(sender[:], txAndSender.GetSender()[:])
			validTx.SetSender(sender)
			g.HandleMsg(validTx, grpc, connections.RunForeground)
			txHashes = append(txHashes, &pb.TxIndex{Idx: int32(idx), TxHash: validTx.HashString(true)})
		}
	}

	log.Debugf("blxr-batch-tx network time %v, handle time: %v, txs success: %v, txs error: %v, validatorsOnly: %v, nextValidator %v fallback %v ms, nodeValidation %v", startTime.Sub(time.Unix(0, req.GetSendingTime())), time.Now().Sub(startTime), len(txHashes), len(txErrors), req.ValidatorsOnly, req.NextValidator, req.Fallback, req.NodeValidation)
	return &pb.BlxrBatchTXReply{TxHashes: txHashes, TxErrors: txErrors}, nil
}

func (g *gateway) NewTxs(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer) error {
	err := g.validateAuthHeader(req.AuthHeader, true, true)
	if err != nil {
		return err
	}

	return g.grpcHandler.NewTxs(req, stream, g.sdn.AccountModel())
}

func (g *gateway) PendingTxs(req *pb.TxsRequest, stream pb.Gateway_PendingTxsServer) error {
	err := g.validateAuthHeader(req.AuthHeader, true, true)
	if err != nil {
		return err
	}

	return g.grpcHandler.PendingTxs(req, stream, g.sdn.AccountModel())
}

func (g *gateway) NewBlocks(req *pb.BlocksRequest, stream pb.Gateway_NewBlocksServer) error {
	err := g.validateAuthHeader(req.AuthHeader, true, true)
	if err != nil {
		return err
	}

	return g.grpcHandler.NewBlocks(req, stream, g.sdn.AccountModel())
}

func (g *gateway) BdnBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer) error {
	err := g.validateAuthHeader(req.AuthHeader, true, true)
	if err != nil {
		return err
	}

	return g.grpcHandler.BdnBlocks(req, stream, g.sdn.AccountModel())
}

func (g *gateway) EthOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer) error {
	err := g.validateAuthHeader(req.AuthHeader, true, true)
	if err != nil {
		return err
	}
	return g.grpcHandler.EthOnBlock(req, stream, g.sdn.AccountModel())
}

func (g *gateway) ShortIDs(_ context.Context, req *pb.TxHashListRequest) (*pb.ShortIDListReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, false)
	if err != nil {
		return nil, err
	}

	result := pb.ShortIDListReply{
		ShortIDs: make([]uint32, len(req.GetTxHashes())),
	}

	for i, txHashBytes := range req.GetTxHashes() {
		txHash, err := types.NewSHA256Hash(txHashBytes)
		if err != nil {
			log.Errorf("txhash from builder is not correct position %v, txhash %v err %v", i, hex.EncodeToString(txHashBytes), err)
			continue
		}
		tx, exists := g.TxStore.Get(txHash)
		if exists && len(tx.ShortIDs()) > 0 {
			result.ShortIDs[i] = uint32(tx.ShortIDs()[0])
		}
	}
	return &result, nil
}

func (g *gateway) ProposedBlock(ctx context.Context, req *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error) {
	err := g.validateAuthHeader(req.AuthHeader, false, false)
	if err != nil {
		return nil, err
	}

	blockReward, ok := new(big.Int).SetString(req.BlockReward, 10)
	if !ok {
		return nil, errors.New("failed converting blockReward")
	}
	proposedBlock := []proposedBlockArgs{
		{
			MEVRelay:      "bloXroute",
			BlockNumber:   rpc.BlockNumber(req.BlockNumber),
			PrevBlockHash: common.HexToHash(req.PrevBlockHash),
			BlockReward:   blockReward,
			GasLimit:      req.GasLimit,
			GasUsed:       req.GasUsed,
			Payload:       make([]hexutil.Bytes, 0, len(req.GetPayload())),
		},
	}

	var txStoreTx *types.BxTransaction
	for _, proposedBlockTx := range req.GetPayload() {
		if proposedBlockTx.GetShortID() == 0 {
			proposedBlock[0].Payload = append(proposedBlock[0].Payload, proposedBlockTx.GetRawData())
		} else {
			txStoreTx, err = g.TxStore.GetTxByShortID(types.ShortID(proposedBlockTx.GetShortID()))
			if err != nil {
				return nil, errors.New("failed decompressing")
			}
			proposedBlock[0].Payload = append(proposedBlock[0].Payload, hexutil.Bytes(txStoreTx.Content()))
		}
	}
	if g.bscValidatorClient == nil {
		g.bscValidatorClient = &http.Client{
			Transport: &http.Transport{
				MaxConnsPerHost:     10,
				MaxIdleConnsPerHost: 10,
				MaxIdleConns:        10,
				IdleConnTimeout:     0 * time.Second,
			},
			Timeout: 60 * time.Second,
		}
	}

	params, err := json.Marshal(proposedBlock)
	if err != nil {
		log.Errorf("error serializing ProposedBlock params %v, params %+v", err, proposedBlock)
		return nil, fmt.Errorf("failed serializing request param, %v", err)
	}

	httpReqJSON := jsonrpc2.Request{
		Method: fmt.Sprintf("%v_proposedBlock", req.Namespace),
		Params: (*json.RawMessage)(&params),
	}

	reqBody, err := httpReqJSON.MarshalJSON()
	if err != nil {
		log.Errorf("error serializing ProposedBlock request %v, message %+v", err, httpReqJSON)
		return nil, fmt.Errorf("failed serializing request message, %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.ValidatorHttpAddress, bytes.NewReader(reqBody))
	if err != nil {
		log.Errorf("error preparing the httpRequest of ProposedBlock %v, message %v", err, string(reqBody))
		return nil, fmt.Errorf("error preparing httpRequest, %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.bscValidatorClient.Do(httpReq)
	if err != nil {
		log.Errorf("error sending ProposedBlock %v, message %v", err, string(reqBody))
		return nil, fmt.Errorf("failed sending message to validator, %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("failed to read validator response for ProposedBlock %v, message, %v", err, string(reqBody))
		return nil, fmt.Errorf("failed to read response, %v", err)
	}
	var jsonResp jsonrpc2.Response
	err = json.Unmarshal(body, &jsonResp)
	if err != nil {
		log.Errorf("failed to send compress ProposedBlock to %v, error when serializing response, %v", req.ValidatorHttpAddress, err)
		return nil, fmt.Errorf("failed to serializing response, %v", err)
	}
	if jsonResp.Error != nil {
		log.Errorf("validator returned error for ProposedBlock %v, message %v", jsonResp.Error.Error(), string(reqBody))
		return nil, fmt.Errorf("error from validator, %v", jsonResp.Error.Message)
	}

	log.Infof("validator response to ProposedBlock %v - %v", resp.Status, string(body))
	return &pb.ProposedBlockReply{
		ValidatorReply:     string(body),
		ValidatorReplyTime: g.clock.Now().UnixMilli(),
	}, nil
}

func (g *gateway) TxReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer) error {
	err := g.validateAuthHeader(req.AuthHeader, true, true)
	if err != nil {
		return err
	}
	return g.grpcHandler.TxReceipts(req, stream, g.sdn.AccountModel())
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
		if _, ok := g.bdnStats.NodeStats()[blockchainConnectionStatus.PeerEndpoint.IPPort()]; ok {
			g.bdnStats.NodeStats()[blockchainConnectionStatus.PeerEndpoint.IPPort()].IsConnected = blockchainConnectionStatus.IsConnected
		}

		if blockchainConnectionStatus.IsDynamic {
			continue
		}

		blockchainIP := blockchainConnectionStatus.PeerEndpoint.IP
		blockchainPort := blockchainConnectionStatus.PeerEndpoint.Port

		// check if gateway is connected to a relay
		if blockchainConnectionStatus.IsConnected {
			g.sdn.SendNodeEvent(
				sdnmessage.NewBlockchainNodeConnEstablishedEvent(
					g.sdn.NodeID(),
					blockchainIP,
					blockchainPort,
					g.clock.Now().String(),
					blockchainConnectionStatus.PeerEndpoint.Version,
					blockchainConnectionStatus.PeerEndpoint.Name,
				),
				g.sdn.NodeID(),
			)
			// check if gateway has relay connection
			if !g.sdn.AccountModel().Miner {
				continue
			}
			var isConnectedRelay bool
			for _, conn := range g.Connections {
				if connections.IsRelay(conn.GetConnectionType()) {
					isConnectedRelay = true
				}
			}
			if isConnectedRelay {
				g.sdn.SendNodeEvent(
					sdnmessage.NewAddAccessibleGatewayEvent(g.sdn.NodeID(), fmt.Sprintf(
						"gateway %v (%v) from account %v established connection with node %v:%v",
						g.sdn.NodeModel().ExternalIP, g.sdn.NodeID(), g.accountID, blockchainIP, int64(blockchainPort),
					), g.clock.Now().String()),
					g.sdn.NodeID())
			} else {
				g.sdn.SendNodeEvent(
					sdnmessage.NewRemoveAccessibleGatewayEvent(g.sdn.NodeID(), fmt.Sprintf(
						"gateway %v (%v) from account %v established connection with node %v:%v but not connected to any relay",
						g.sdn.NodeModel().ExternalIP, g.sdn.NodeID(), g.accountID, blockchainIP, int64(blockchainPort),
					), g.clock.Now().String()),
					g.sdn.NodeID())
			}

			continue
		}

		g.sdn.SendNodeEvent(
			sdnmessage.NewBlockchainNodeConnError(g.sdn.NodeID(), blockchainIP, blockchainPort, g.clock.Now().String()),
			g.sdn.NodeID(),
		)
		if g.sdn.AccountModel().Miner && !g.gatewayHasBlockchainConnection() {
			go func() {
				// check if gateway doesn't have any other node connections
				g.sdn.SendNodeEvent(
					sdnmessage.NewRemoveAccessibleGatewayEvent(g.sdn.NodeID(), fmt.Sprintf(
						"gateway %v (%v) from account %v is not connected to any node",
						g.sdn.NodeModel().ExternalIP, g.sdn.NodeID(), g.accountID,
					), g.clock.Now().String()),
					g.sdn.NodeID())

			}()
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

func forwardBSCTx(conn *http.Client, txHash string, tx string, endpoint string, rpcType string) {
	if endpoint == "" || rpcType == "" || conn == nil {
		return
	}

	params, err := json.Marshal([]string{tx})
	if err != nil {
		log.Errorf("failed to send tx to %v, error for serializing request param, %v", endpoint, err)
		return
	}

	httpReq := jsonrpc2.Request{
		Method: rpcType,
		Params: (*json.RawMessage)(&params),
	}

	reqBody, err := httpReq.MarshalJSON()
	if err != nil {
		log.Errorf("failed to send tx %v to %v, error for serializing request, %v", txHash, endpoint, err)
		return
	}

	resp, err := conn.Post(endpoint, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		log.Errorf("failed to send tx %v to %v, error when send POST request, %v", txHash, endpoint, err)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("failed to read response from %v , %v, tx hash %v", endpoint, err, txHash)
		return
	}

	log.Infof("transaction %v has been sent to %v, rpc method %v, got response: %s, status code: %v", txHash, endpoint, httpReq.Method, string(body), resp.StatusCode)
}
