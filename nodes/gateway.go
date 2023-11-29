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

	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/cenkalti/backoff/v4"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/interfaces"
	"github.com/sirupsen/logrus"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller/rpc"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/polygon/bor"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	bxrpc "github.com/bloXroute-Labs/gateway/v2/rpc"
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

	bscMainnetBloomCap     = 35e6
	mainnetBloomCap        = 85e5
	polygonMainnetBloomCap = 225e5

	bloomStoreInterval = time.Hour
)

var (
	errUnsupportedBlockType = errors.New("block type is not supported")
)

type gateway struct {
	Bx
	pb.UnimplementedGatewayServer
	context context.Context

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

	blockProposer services.BlockProposer

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
	bloomFilter                  services.BloomFilter
	txIncludeSenderInFeed        bool

	polygonValidatorInfoManager polygon.ValidatorInfoManager
	blockTime                   time.Duration

	grpcHandler   *servers.GrpcHandler
	txsQueue      services.MessageQueue
	txsOrderQueue services.MessageQueue

	clientHandler *servers.ClientHandler
	grpcServer    *gatewayGRPCServer
	log           *log.Entry
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
func NewGateway(parent context.Context,
	bxConfig *config.Bx,
	bridge blockchain.Bridge,
	wsManager blockchain.WSManager,
	blockchainPeers []types.NodeEndpoint,
	peersInfo []network.PeerInfo,
	recommendedPeers map[string]struct{},
	gatewayPublicKeyStr string,
	sdn connections.SDNHTTP,
	sslCerts *utils.SSLCerts,
	staticEnodesCount int,
	polygonHeimdallEndpoints string,
	transactionSlotStartDuration int,
	transactionSlotEndDuration int,
	enableBloomFilter bool,
	blocksToCacheWhileProposing int,
	proposingInterval time.Duration,
	txIncludeSenderInFeed bool,
) (Node, error) {

	clock := utils.RealClock{}
	blockTime, _ := bxgateway.NetworkToBlockDuration[bxConfig.BlockchainNetwork]

	g := &gateway{
		Bx:                           NewBx(bxConfig, "datadir", nil),
		bridge:                       bridge,
		isBDN:                        bxConfig.GatewayMode.IsBDN(),
		wsManager:                    wsManager,
		context:                      parent,
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
		txIncludeSenderInFeed:        txIncludeSenderInFeed,
		log: log.WithFields(log.Fields{
			"component": "gateway",
		}),
	}

	g.blockProposer = services.NewNoopBlockProposer(&g.TxStore, log.WithField("service", "noop-block-proposer"))

	if polygonHeimdallEndpoints != "" {
		g.polygonValidatorInfoManager = bor.NewSprintManager(parent, &g.wsManager, bor.NewHeimdallSpanner(parent, polygonHeimdallEndpoints))
	} else {
		g.polygonValidatorInfoManager = nil
	}

	if bxConfig.BlockchainNetwork == bxgateway.BSCMainnet || bxConfig.BlockchainNetwork == bxgateway.PolygonMainnet || bxConfig.BlockchainNetwork == bxgateway.PolygonMumbai {
		g.validatorStatusMap = syncmap.NewStringMapOf[bool]()
		g.nextValidatorMap = orderedmap.New()
	}

	if bxConfig.BlockchainNetwork == bxgateway.BSCMainnet || bxConfig.BlockchainNetwork == bxgateway.BSCTestnet {
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

		g.blockProposer = bsc.NewBlockProposer(g.clock, &g.TxStore, blocksToCacheWhileProposing, log.WithField("service", "bsc-block-proposer"), rpc.NewManager(), proposingInterval)
	}

	g.asyncMsgChannel = services.NewAsyncMsgChannel(g)
	g.mevBundleDispatcher = bundle.NewDispatcher(bxConfig.MEVBuilders, bxConfig.MEVMaxProfitBuilder, bxConfig.ProcessMegaBundle)

	// create tx store service pass to eth client
	g.bdnStats = bxmessage.NewBDNStats(blockchainPeers, recommendedPeers)
	g.burstLimiter = services.NewAccountBurstLimiter(g.clock)

	// set empty default stats, Run function will override it
	g.stats = statistics.NewStats(false, "127.0.0.1", "", nil, false)
	g.txsQueue = services.NewMsgQueue(runtime.NumCPU()*2, bxgateway.ParallelQueueChannelSize, g.msgAdapter)
	g.txsOrderQueue = services.NewMsgQueue(1, bxgateway.ParallelQueueChannelSize, g.msgAdapter)

	if enableBloomFilter {
		var bloomCap uint32
		switch bxConfig.BlockchainNetwork {
		case bxgateway.Mainnet:
			bloomCap = mainnetBloomCap
		case bxgateway.BSCMainnet, bxgateway.BSCTestnet:
			bloomCap = bscMainnetBloomCap
		case bxgateway.PolygonMainnet, bxgateway.PolygonMumbai:
			bloomCap = polygonMainnetBloomCap
		default:
			// default to mainnet
			bloomCap = mainnetBloomCap
		}

		var err error
		g.bloomFilter, err = services.NewBloomFilter(context.Background(), clock, bloomStoreInterval, bxConfig.DataDir, bloomCap)
		if err != nil {
			return nil, err
		}
	} else {
		g.bloomFilter = services.NoOpBloomFilter{}
	}

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
		assigner, services.NewHashHistory("seenTxs", 30*time.Minute), nil, *g.sdn.Networks(), g.bloomFilter)
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
			staticEnodesCount,
			accountModel.MinAllowedNodes.MsgQuota.Limit,
		))
	}

	if uint64(staticEnodesCount) > uint64(accountModel.MaxAllowedNodes.MsgQuota.Limit) {
		panic(fmt.Sprintf(
			"account %v is not allowed to run %d blockchain nodes. Maximum is %d",
			accountModel.AccountID,
			staticEnodesCount,
			accountModel.MaxAllowedNodes.MsgQuota.Limit,
		))
	}

	return &sslCerts, sdn, nil
}

func (g *gateway) Run() error {
	group, ctx := errgroup.WithContext(g.context)

	g.accountID = g.sdn.NodeModel().AccountID
	// node ID might not be assigned to gateway yet, ok
	nodeID := g.sdn.NodeID()
	g.log.WithFields(log.Fields{
		"accountID": g.accountID,
		"nodeID":    nodeID,
	}).Info("ssl certificate successfully loaded")

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
	var err error
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
		group.Go(func() error {
			return g.handleBridgeMessages(ctx)
		})
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

	txFromFieldIncludable := blockchainNetwork.EnableCheckSenderNonce || g.txIncludeSenderInFeed

	g.grpcHandler = servers.NewGrpcHandler(g.feedManager, txFromFieldIncludable)

	// start feed manager if websocket or gRPC is enabled
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled {
		group.Go(func() error {
			return g.feedManager.Start(ctx)
		})
	}

	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled {
		g.clientHandler = servers.NewClientHandler(g.feedManager, nil, servers.NewHTTPServer(g.feedManager, g.BxConfig.HTTPPort), g.BxConfig.EnableBlockchainRPC, g.sdn.GetQuotaUsage, log.WithFields(log.Fields{
			"component": "gatewayClientHandler",
		}), &g.BxConfig.PendingTxsSourceFromNode, g.authorize, txFromFieldIncludable)

		group.Go(func() error {
			return g.clientHandler.ManageWSServer(ctx, g.BxConfig.ManageWSServer)
		})

		group.Go(func() error {
			return g.clientHandler.ManageHTTPServer()
		})
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
		g.grpcServer = newGatewayGRPCServer(g, g.BxConfig.Host, g.BxConfig.Port, g.BxConfig.User, g.BxConfig.Password)
		group.Go(func() error {
			return g.grpcServer.Start()
		})
	}

	go g.handleBlockchainConnectionStatusUpdate()

	if (networkNum == bxgateway.PolygonMainnetNum || networkNum == bxgateway.PolygonMumbaiNum) && g.polygonValidatorInfoManager != nil {
		// running as goroutine to not block starting of node
		log.Debugf("starting polygonValidatorInfoManager, networkNum=%d", networkNum)
		go func() {
			if retryErr := backoff.RetryNotify(
				g.polygonValidatorInfoManager.Run,
				bor.Retry(),
				func(err error, duration time.Duration) {
					g.log.Tracef("failed to start polygonValidatorInfoManager: %v, retry in %s", err, duration.String())
				},
			); retryErr != nil {
				g.log.Warnf("failed to start polygonValidatorInfoManager: %v", retryErr)
			}
		}()
	}

	if err = g.blockProposer.Run(ctx); err != nil {
		g.log.Warnf("failed to start blockProposer: %v", err)
	}

	return group.Wait()
}

func (g *gateway) Close() error {
	if g.grpcServer != nil {
		g.grpcServer.Stop()
	}

	if g.clientHandler != nil {
		return g.clientHandler.Stop()
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

	g.log.WithFields(log.Fields{
		"gateway":   g.sdn.NodeID(),
		"relayIP":   instruction.IP,
		"relayPort": instruction.Port,
	}).Info("connecting to relay")

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
			g.log.Debugf("blocking request to rpc")

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
		g.log.Errorf("can't extract extra data for block height %v", blockHeight)
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
		g.log.Info("The gateway has all the information to support next_validator transactions")
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
			l := g.log.WithFields(log.Fields{"txHash": txHash})

			if err != nil {
				l.Errorf("failed to reevaluate next validator tx, err %v", err)
			}

			// send with adjusted fallback regardless of if validator is accessible
			if firstValidatorInaccessible {
				txInfo.Tx.RemoveFlags(types.TFNextValidator)
				txInfo.Tx.SetFallback(0)
			}
			err = g.HandleMsg(txInfo.Tx, txInfo.Source, connections.RunForeground)
			if err != nil {
				l.Errorf("failed to process reevaluated next validator tx, err %v", err)
			}
			continue
		} else {
			// should have already been sent by gateway
			delete(pendingNextValidatorTxsMap, txHash)
		}
	}
}

func (g *gateway) generatePolygonValidator(bxBlock *types.BxBlock, blockInfo *eth.BlockInfo) []*types.FutureValidatorInfo {
	blockHeight := bxBlock.Number.Uint64()

	if g.validatorStatusMap == nil || g.wsManager == nil || g.polygonValidatorInfoManager == nil || !g.polygonValidatorInfoManager.IsRunning() || blockInfo == nil {
		return blockchain.DefaultValidatorInfo(blockHeight)
	}

	validatorInfo := g.polygonValidatorInfoManager.FutureValidators(blockInfo.Block.Header())

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

func (g *gateway) generateFutureValidatorInfo(block *types.BxBlock, blockInfo *eth.BlockInfo) []*types.FutureValidatorInfo {
	g.validatorInfoUpdateLock.Lock()
	defer g.validatorInfoUpdateLock.Unlock()

	if block.Number.Int64() <= g.latestValidatorInfoHeight {
		return g.latestValidatorInfo
	}
	g.latestValidatorInfoHeight = block.Number.Int64()

	switch g.sdn.NetworkNum() {
	case bxgateway.PolygonMainnetNum, bxgateway.PolygonMumbaiNum:
		g.latestValidatorInfo = g.generatePolygonValidator(block, blockInfo)
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
		l := g.log.WithFields(log.Fields{
			"blockHeight":     blockHeight,
			"bestBlockHeight": g.bestBlockHeight,
			"bxBlock":         bxBlock,
		})
		if len(g.blockchainPeers) > 0 && blockHeight < g.bestBlockHeight {
			l.Debug("block is too far behind best block height from node - not publishing")
			return nil
		}
		if g.bestBlockHeight != 0 && utils.Abs(blockHeight-g.bestBlockHeight) > bxgateway.BDNBlocksMaxBlocksAway {
			if blockHeight > g.bestBlockHeight {
				g.bdnBlocksSkipCount++
			}
			if g.bdnBlocksSkipCount <= bxgateway.MaxOldBDNBlocksToSkipPublish {
				l.Debug("block is too far away from best block height - not publishing")
				return nil
			}
			l.Debug("publishing block from BDN that is far away from current best block height - resetting bestBlockHeight to zero")
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
	l := g.log.WithFields(log.Fields{
		"bxBlock": bxBlock,
		"source":  nodeSource,
	})

	notifyEthBlockFeeds := func(block *ethtypes.Block, nodeSource *connections.Blockchain, info []*types.FutureValidatorInfo, isBlockchainBlock bool) error {
		ethNotification, err := types.NewEthBlockNotification(common.Hash(bxBlock.Hash()), block, info, g.txIncludeSenderInFeed)
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
			l.Trace("duplicate ETH block for bdnBlocks")
		}

		if isBlockchainBlock {
			if g.newBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute) {
				g.bestBlockHeight = int(block.Number().Int64())
				g.bdnBlocksSkipCount = 0

				notification := ethNotification.Clone()
				notification.SetNotificationType(types.NewBlocksFeed)
				g.notify(notification)
			} else {
				l.Trace("duplicate ETH block for newBlocks")
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
		}

		ethBlock, err := eth.BeaconBlockToEthBlock(b)
		if err != nil {
			return err
		}

		if err := notifyEthBlockFeeds(ethBlock, nodeSource, info, isBlockchainBlock); err != nil {
			return err
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
	notification.SetSource(&sourceEndpoint)

	if g.feedManager.SubscriptionTypeExists(types.TxReceiptsFeed) {
		receipts, err := servers.HandleTxReceipts(g.feedManager, notification.(*types.EthBlockNotification))
		if err != nil {
			log.Printf("failed to handle tx receipts: %v", err)
			return
		}
		if len(receipts) > 0 {
			g.notify(types.NewTxReceiptsNotification(receipts))
		}
	}
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

func (g *gateway) handleBridgeMessages(ctx context.Context) error {
	var err error
	for {
		select {
		case <-ctx.Done():
			return nil
		case txsFromNode := <-g.bridge.ReceiveNodeTransactions():
			g.traceIfSlow(func() {
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
			g.traceIfSlow(func() {
				// if we are not yet synced with relay - ignore the announcement from the node
				if !g.isSyncWithRelay() {
					return
				}
				// if announcement message has many transaction we are probably after reconnect with the node - we should ignore it in order not to over load the client feed
				if len(txAnnouncement.Hashes) > bxgateway.MaxAnnouncementFromNode {
					g.log.Debugf("skipped tx announcement of size %v", len(txAnnouncement.Hashes))
					return
				}
				requests := make([]types.SHA256Hash, 0)
				for _, hash := range txAnnouncement.Hashes {
					g.log.WithFields(log.Fields{
						"hash":   hash,
						"peerID": txAnnouncement.PeerID,
					})
					bxTx, exists := g.TxStore.Get(hash)
					if !exists && !g.TxStore.Known(hash) {
						g.log.Trace("msgTx: from Blockchain, event TxAnnouncedByBlockchainNode")
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
						g.log.WithFields(log.Fields{
							"delivered":       delivered,
							"expected":        expected,
							"diffFromBDNTime": diffFromBDNTime,
						}).Trace("msgTx: from Blockchain, event TxAnnouncedByBlockchainNodeIgnoreSeen")
						// if we delivered to node and got it from the node we were very late.
					}
					if !txAnnouncement.PeerEndpoint.IsDynamic() {
						g.publishPendingTx(hash, bxTx, true)
					}
				}
				if len(requests) > 0 && txAnnouncement.PeerID != bxgateway.WSConnectionID {
					err = g.bridge.RequestTransactionsFromNode(txAnnouncement.PeerID, requests)
					if err == blockchain.ErrChannelFull {
						g.log.Warningf("transaction requests channel is full, skipping request")
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
				g.log.Errorf("Gateway does not have an active blockchain connection. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
				log.Exit(0)
			}
		case confirmBlock := <-g.bridge.ReceiveConfirmedBlockFromNode():
			if !g.BxConfig.NoBlocks {
				g.traceIfSlow(func() {
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
						g.log.Errorf("Error handling block confirmation message: %v", err)
					}
				}, fmt.Sprintf("ReceiveConfirmedBlockFromNode hash=[%s]", confirmBlock.Block.Hash()), confirmBlock.PeerEndpoint.String(), 1)
			}
		case blockchainBlock := <-g.bridge.ReceiveBlockFromNode():
			if !g.BxConfig.NoBlocks {
				g.traceIfSlow(func() { g.handleBlockFromBlockchain(blockchainBlock) },
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
			source.Log().WithField("hash", typedMsg.Hash()).Errorf("discarding tx: %s", err)
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
				g.log.Debug("gateway is not sending block confirm message to relay")
			} else if source.GetConnectionType() == utils.Blockchain {
				g.log.Tracef("gateway broadcasting block confirmation of block %v to relays", hashString)
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
			g.log.Tracef("received status from %v:%v", status.IP, status.Port)
			return true
		case <-time.After(time.Second):
			return false
		}

	} else {
		g.log.Errorf("failed to send blockchain status request when received hello msg from relay: %v", err)
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

	var keysToRemove []string
	for _, key := range g.validatorStatusMap.Keys() {
		if !utils.Exists(key, onlineList) {
			keysToRemove = append(keysToRemove, key)
		}
	}

	for _, key := range keysToRemove {
		g.validatorStatusMap.Delete(key)
	}

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
	l := source.Log().WithFields(log.Fields{
		"hash":   tx.Hash(),
		"nodeID": nodeID,
	})

	switch {
	case txResult.FailedValidation:
		eventName = "TxValidationFailedStructure"
	case txResult.NewContent && txResult.Transaction.Flags().IsReuseSenderNonce() && tx.ShortID() == types.ShortIDEmpty:
		eventName = "TxReuseSenderNonce"
		l.Trace(txResult.DebugData)
	case txResult.AlreadySeen:
		l.Tracef("received already Seen transaction from %v:%v and account id %v, reason: %s", peerIP, peerPort, source.GetAccountID(), txResult.DebugData)
	case txResult.NewContent || txResult.NewSID || txResult.Reprocess:
		eventName = "TxProcessedByGatewayFromPeer"
		if txResult.NewContent || txResult.Reprocess {
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
			shouldSendTxFromBDNToNodes := g.shouldSendTxFromBDNToNodes(connectionType, tx, tx.Flags().IsValidatorsOnly(), tx.Flags().IsNextValidator()) && !g.BxConfig.NoTxsToBlockchain

			if source.GetNetworkNum() == bxgateway.BSCMainnetNum && (tx.Flags().IsValidatorsOnly() || tx.Flags().IsNextValidator()) {
				rawBytesString, err := getRawBytesStringFromTXMsg(tx)
				if err != nil {
					l.WithFields(log.Fields{
						"to": g.BxConfig.ForwardTransactionEndpoint,
					}).Errorf("failed to forward transaction, can't get the tx raw bytes, err %v", err)
				} else {
					go g.forwardBSCTx(g.bscTxClient, tx.Hash().String(), "0x"+rawBytesString, g.BxConfig.ForwardTransactionEndpoint, g.BxConfig.ForwardTransactionMethod)
				}
			} else if shouldSendTxFromNodeToOtherNodes || shouldSendTxFromBDNToNodes {
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
						l.WithFields(log.Fields{
							"slotStart":               slotBegin.String(),
							"nextBlockTime":           g.nextBlockTime.String(),
							"frontRunProtectionDelay": frontRunProtectionDelay.String(),
						}).Debug("received next validator tx, wait to prevent front running")
					} else if now.After(deadline) {
						sentToBlockchainNode = false
						l.WithFields(log.Fields{
							"deadline":      deadline.String(),
							"nextBlockTime": g.nextBlockTime.String(),
						}).Error("received next validator tx after slot deadline, will not send the tx")
						break
					}

					time.AfterFunc(frontRunProtectionDelay, func() {
						if source.GetNetworkNum() == bxgateway.BSCMainnetNum && (tx.Flags().IsNextValidator() || tx.Flags().IsValidatorsOnly()) {
							l.Debug("not sending tx to p2p node for bsc semiprivate tx")
							return
						}

						err := g.bridge.SendTransactionsFromBDN(txsToDeliverToNodes)
						if err != nil {
							l.Errorf("failed to send transaction from BDN to bridge: %v", err)
						}

						if shouldSendTxFromNodeToOtherNodes {
							g.bdnStats.LogTxSentToAllNodesExceptSourceNode(sourceEndpoint)
						}

						l.WithFields(log.Fields{
							"flags":                   tx.Flags(),
							"frontRunProtectionDelay": frontRunProtectionDelay.String(),
						}).Debug("tx sent to blockchain with front run protection delay")
					})
				} else {
					err := g.bridge.SendTransactionsFromBDN(txsToDeliverToNodes)
					if err != nil {
						l.Errorf("failed to send transaction from BDN to bridge: %v", err)
					}

					sentToBlockchainNode = true
					if shouldSendTxFromNodeToOtherNodes {
						g.bdnStats.LogTxSentToAllNodesExceptSourceNode(sourceEndpoint)
					}

					l.WithFields(log.Fields{
						"flags": tx.Flags(),
					}).Debug("tx sent to blockchain")
				}
			}

			if isRelay && !txResult.Reprocess {
				g.bdnStats.LogNewTxFromBDN()
			}

			if txResult.NewContent {
				g.txTrace.Log(tx.Hash(), source)
			}
		}
	default:
		// duplicate transaction
		if txResult.Transaction.Flags().IsReuseSenderNonce() {
			eventName = "TxReuseSenderNonceIgnoreSeen"
			g.log.Trace(txResult.DebugData)
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
	l = l.WithFields(log.Fields{
		"from":                    source,
		"nonce":                   txResult.Nonce,
		"flags":                   tx.Flags(),
		"newTx":                   txResult.NewTx,
		"newContent":              txResult.NewContent,
		"newShortid":              txResult.NewSID,
		"event":                   eventName,
		"sentToBDN":               sentToBDN,
		"sentPeersNum":            broadcastRes.SentPeers,
		"sentToBlockchainNode":    sentToBlockchainNode,
		"handlingDuration":        handlingTime,
		"sender":                  txResult.Transaction.Sender(),
		"networkDuration":         tx.ReceiveTime().Sub(tx.Timestamp()).Microseconds(),
		"statsDuration":           statsDuration,
		"nextValidatorEnabled":    tx.Flags().IsNextValidator(),
		"fallbackDuration":        tx.Fallback(),
		"nextValidatorFallback":   tx.Flags().IsNextValidatorRebroadcast(),
		"frontRunProtectionDelay": frontRunProtectionDelay,
		"waitingDuration":         tx.WaitDuration(),
		"txsInNetworkChannel":     tx.NetworkChannelPosition(),
		"txsInProcessingChannel":  tx.ProcessChannelPosition(),
		"msgLen":                  tx.Size(bxmessage.CurrentProtocol),
	})
	l.Trace("msgTx")
}

// shouldSendTxFromBDNToNodes send to node if all are true
func (g *gateway) shouldSendTxFromBDNToNodes(connectionType utils.NodeType, tx *bxmessage.Tx, validatorsOnly bool, nextValidatorTx bool) (send bool) {
	if connectionType == utils.Blockchain {
		return // Transaction is from blockchain node (transaction is not from Relay or RPC)
	}

	if len(g.blockchainPeers) == 0 {
		return // Gateway is not connected to nodes
	}

	if (!g.BxConfig.BlocksOnly && tx.Flags().ShouldDeliverToNode()) || // (Gateway didn't start with blocks only mode and DeliverToNode flag is on) OR
		(g.BxConfig.AllTransactions && !validatorsOnly && !nextValidatorTx) { // (Gateway started with a flag to send all transactions, and this is not validator only nor next validator tx
		send = true
	}

	return
}

func (g *gateway) updateValidatorStateMap() {
	for newList := range g.bridge.ReceiveValidatorListInfo() {
		if newList == nil {
			continue
		}

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

	blockInfo, err := g.bxBlockToBlockInfo(bxBlock)
	if err != nil && err != errUnsupportedBlockType {
		log.Errorf("failed to convert bx block %v to block info: %v", bxBlock, err)
	}

	g.onBlock(blockInfo)
	source := connections.NewBlockchainConn(blockchainBlock.PeerEndpoint)

	g.bdnStats.LogNewBlockMessageFromNode(source.NodeEndpoint())

	broadcastMessage, usedShortIDs, err := g.blockProcessor.BxBlockToBroadcast(bxBlock, g.sdn.NetworkNum(), g.sdn.MinTxAge())
	if err != nil {
		if err == services.ErrAlreadyProcessed {
			source.Log().Debugf("received duplicate block %v, skipping", bxBlock.Hash())
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

	validatorInfo := g.generateFutureValidatorInfo(bxBlock, blockInfo)
	if err := g.publishBlock(bxBlock, &source, validatorInfo, !blockchainBlock.PeerEndpoint.IsDynamic()); err != nil {
		source.Log().Errorf("Failed to publish block %v from blockchain node with %v", bxBlock, err)
	}
}

func (g *gateway) processBlockFromBDN(bxBlock *types.BxBlock) {
	blockInfo, err := g.bxBlockToBlockInfo(bxBlock)
	if err != nil && err != errUnsupportedBlockType {
		g.log.Errorf("failed to convert bx block %v to block info: %v", bxBlock, err)
	}

	g.onBlock(blockInfo)

	if err = g.bridge.SendBlockToNode(bxBlock); err != nil {
		g.log.Errorf("unable to send block %v from BDN to node: %v", bxBlock, err)
	}

	g.bdnStats.LogNewBlockFromBDN(g.BxConfig.BlockchainNetwork)
	validatorInfo := g.generateFutureValidatorInfo(bxBlock, blockInfo)
	err = g.publishBlock(bxBlock, nil, validatorInfo, false)
	if err != nil {
		g.log.Errorf("Failed to publish BDN block with %v, %v", err, bxBlock)
	}
}

func (g *gateway) notify(notification types.Notification) {
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled {
		select {
		case g.feedManagerChan <- notification:
		default:
			g.log.Warnf("gateway feed channel is full. Can't add %v without blocking. Ignoring hash %v", reflect.TypeOf(notification), notification.GetHash())
		}
	}
}

func (g *gateway) handleMEVBundleMessage(mevBundle bxmessage.MEVBundle, source connections.Conn) {
	start := time.Now()
	blockNumber, err := strconv.ParseInt(strings.TrimPrefix(mevBundle.BlockNumber, "0x"), 16, 64)
	if err != nil {
		g.log.Errorf("failed to parse block %v: %v", mevBundle, err)
	}

	fromRelay := connections.IsRelay(source.GetConnectionType())

	if !g.seenMEVBundles.SetIfAbsent(mevBundle.Hash().String(), time.Minute*30) {
		eventName := "GatewayReceivedBundleFromBDNIgnoreSeen"
		if !fromRelay {
			eventName = "GatewayReceivedBundleFromFeedIgnoreSeen"
		}

		source.Log().Tracef("ignoring %s duration: %v ms, time in network: %v ms", mevBundle, time.Since(start).Milliseconds(), start.Sub(mevBundle.PerformanceTimestamp).Milliseconds())
		g.stats.AddGatewayBundleEvent(eventName, source, start, mevBundle.BundleHash, mevBundle.GetNetworkNum(), mevBundle.Names(), mevBundle.Frontrunning, mevBundle.UUID, uint64(blockNumber), mevBundle.MinTimestamp, mevBundle.MaxTimestamp, mevBundle.BundlePrice, mevBundle.EnforcePayout)
		return
	}

	var event string
	if fromRelay {
		event = "GatewayReceivedBundleFromBDN"

		if err := g.mevBundleDispatcher.Dispatch(&mevBundle); err != nil {
			g.log.Errorf("failed to dispatch mev bundle %v: %v", mevBundle.BundleHash, err)
		}

		source.Log().Tracef("dispatching %s duration: %v ms, time in network: %v ms", mevBundle, time.Since(start).Milliseconds(), start.Sub(mevBundle.PerformanceTimestamp).Milliseconds())
	} else {
		event = "GatewayReceivedBundleFromFeed"

		// set timestamp as late as possible.
		mevBundle.PerformanceTimestamp = time.Now()
		broadcastRes := g.broadcast(&mevBundle, source, utils.RelayTransaction|utils.RelayProxy)

		source.Log().Tracef("broadcasting %s %s duration: %v ms, time in network: %v ms", mevBundle, broadcastRes, time.Since(start).Milliseconds(), start.Sub(mevBundle.PerformanceTimestamp).Milliseconds())
	}

	g.stats.AddGatewayBundleEvent(event, source, start, mevBundle.BundleHash, mevBundle.GetNetworkNum(), mevBundle.Names(), mevBundle.Frontrunning, mevBundle.UUID, uint64(blockNumber), mevBundle.MinTimestamp, mevBundle.MaxTimestamp, mevBundle.BundlePrice, mevBundle.EnforcePayout)
}

func retrieveAuthHeader(ctx context.Context, authFromRequestBody string) string {
	authHeader, err := bxrpc.ReadAuthMetadata(ctx)
	if err == nil {
		return authHeader
	}

	// deprecated
	return authFromRequestBody
}

func retrieveOriginalSenderAccountID(ctx context.Context, accountModel *sdnmessage.Account) (*types.AccountID, error) {
	accountID := accountModel.AccountID
	if accountModel.AccountID == types.BloxrouteAccountID {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok && len(md.Get(types.OriginalSenderAccountIDHeaderKey)) > 0 {
			accountID = types.AccountID(md.Get(types.OriginalSenderAccountIDHeaderKey)[0])
		} else {
			return nil, fmt.Errorf("request sent from cloud services and should include %v header", types.OriginalSenderAccountIDHeaderKey)
		}
	}
	return &accountID, nil
}

func (g *gateway) Peers(ctx context.Context, req *pb.PeersRequest) (*pb.PeersReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.Bx.Peers(ctx, req)
}

// DisconnectInboundPeer disconnect inbound peer from gateway
func (g *gateway) DisconnectInboundPeer(ctx context.Context, req *pb.DisconnectInboundPeerRequest) (*pb.DisconnectInboundPeerReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	err = g.bridge.SendDisconnectEvent(types.NodeEndpoint{IP: req.PeerIp, Port: int(req.PeerPort), PublicKey: req.PublicKey})
	if err != nil {
		return &pb.DisconnectInboundPeerReply{Status: err.Error()}, status.Error(codes.Internal, err.Error())
	}
	return &pb.DisconnectInboundPeerReply{Status: fmt.Sprintf("Sent request to disconnect peer %v %v %v", req.PublicKey, req.PeerIp, req.PeerPort)}, nil
}

const (
	connectionStatusConnected    = "connected"
	connectionStatusNotConnected = "not_connected"
)

func (g *gateway) validateAuthHeader(authHeader string, required bool, allowAccessToInternalGateway bool) (*sdnmessage.Account, error) {
	var err error
	if authHeader == "" {
		if required {
			return nil, fmt.Errorf("auth header is missing")
		}
		if g.sdn.AccountModel().AccountID == types.BloxrouteAccountID {
			return nil, fmt.Errorf("could not connect to internal gateway without auth header")
		}
		authHeader = g.getHeaderFromGateway()
	}
	accountID, secretHash, err := utils.GetAccountIDSecretHashFromHeader(authHeader)
	if err != nil {
		return nil, err
	}

	accountModel, err := g.authorize(accountID, secretHash, allowAccessToInternalGateway)
	if err != nil {
		return nil, err
	}
	return &accountModel, nil
}

func (g *gateway) validateAuthHeaderWithContext(ctx context.Context, authHeader string, required bool, allowAccessToInternalGateway bool) (*sdnmessage.Account, error) {
	accountModel, err := g.validateAuthHeader(authHeader, required, allowAccessToInternalGateway)
	if err != nil {
		return accountModel, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return accountModel, err
	}
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
	var err error
	connectionAccountModel := g.sdn.AccountModel()
	l := g.log.WithFields(log.Fields{
		"accountID":          g.sdn.AccountModel().AccountID,
		"requestedAccountID": accountID,
	})

	if accountID != connectionAccountModel.AccountID {
		if !allowAccessToInternalGateway {
			l.Errorf("account %v is not authorized to call this method directly", g.sdn.AccountModel().AccountID)
			return connectionAccountModel, fmt.Errorf("not authorized to call this method")
		}
		connectionAccountModel, err = g.sdn.FetchCustomerAccountModel(accountID)
		if err != nil {
			var invalidUserError error

			if strings.Contains(err.Error(), strconv.FormatInt(http.StatusUnauthorized, 10)) {
				invalidUserError = fmt.Errorf("account %v is not authorized to get other account %v information", g.sdn.AccountModel().AccountID, accountID)
			} else if strings.Contains(err.Error(), strconv.FormatInt(http.StatusNotFound, 10)) {
				invalidUserError = fmt.Errorf("account %v is not found", accountID)
			} else if strings.Contains(err.Error(), strconv.FormatInt(http.StatusBadRequest, 10)) {
				invalidUserError = fmt.Errorf("bad request for %v", accountID)
			}

			if invalidUserError != nil {
				log.Errorf(invalidUserError.Error())
				return connectionAccountModel, invalidUserError
			}

			l.Errorf("failed to get customer account model, connectionSecretHash: %v, error: %v", secretHash, err)
			connectionAccountModel = sdnmessage.GetDefaultEliteAccount(time.Now().UTC())
			connectionAccountModel.AccountID = accountID
			connectionAccountModel.SecretHash = secretHash
		}
		if !connectionAccountModel.TierName.IsEnterprise() {
			l.Warnf("customer must be enterprise / enterprise elite / ultra but it is %v", connectionAccountModel)
			return connectionAccountModel, fmt.Errorf("account must be enterprise / enterprise elite / ultra")
		}
	}

	if secretHash != connectionAccountModel.SecretHash && secretHash != "" {
		l.Error("account sent a different secret hash than set in the account model")
		return connectionAccountModel, fmt.Errorf("wrong value in the authorization header")
	}

	return connectionAccountModel, nil
}

func (g *gateway) Version(ctx context.Context, req *pb.VersionRequest) (*pb.VersionReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	resp := &pb.VersionReply{
		Version:   version.BuildVersion,
		BuildDate: version.BuildDate,
	}
	return resp, nil
}

const bdn = "BDN"

func (g *gateway) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
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
		if len(g.blockchainPeers) == 0 {
			return nil // Gateway is not connected to nodes
		}

		err := g.bridge.SendBlockchainStatusRequest()
		if err != nil {
			g.log.Errorf("failed to send blockchain status request: %v", err)
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
			g.log.Errorf("no blockchain status response from backend within 1sec timeout")
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

func (g *gateway) Subscriptions(ctx context.Context, req *pb.SubscriptionsRequest) (*pb.SubscriptionsReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, true)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.feedManager.GetGrpcSubscriptionReply(), nil
}

func (g *gateway) BlxrSubmitBundle(ctx context.Context, req *pb.BlxrSubmitBundleRequest) (*pb.BlxrSubmitBundleReply, error) {
	authHeader := retrieveAuthHeader(ctx, "")

	accountModel, err := g.validateAuthHeader(authHeader, true, false)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	mevBundleParams := &jsonrpc.RPCBundleSubmissionPayload{
		MEVBuilders:     req.MevBuilders,
		Frontrunning:    true,
		Transaction:     req.Transactions,
		BlockNumber:     req.BlockNumber,
		MinTimestamp:    int(req.MinTimestamp),
		MaxTimestamp:    int(req.MaxTimestamp),
		RevertingHashes: req.RevertingHashes,
		UUID:            req.Uuid,
		BundlePrice:     req.BundlePrice,
		EnforcePayout:   req.EnforcePayout,
	}

	grpc := connections.NewRPCConn(*accountID, servers.GetPeerAddr(ctx), g.sdn.NetworkNum(), utils.GRPC)
	bundleSubmitResult, _, err := servers.HandleMEVBundle(g.feedManager, grpc, *accountModel, mevBundleParams)
	if err != nil {
		// TODO need to refactor errors returned from HandleMEVBundle and then map them to jsonrpc and gRPC codes accordingly
		// TODO instead of returning protocol specific error codes
		return nil, err
	}

	return &pb.BlxrSubmitBundleReply{BundleHash: bundleSubmitResult.BundleHash}, nil
}

func (g *gateway) BlxrTx(ctx context.Context, req *pb.BlxrTxRequest) (*pb.BlxrTxReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, false)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	grpc := connections.NewRPCConn(*accountID, servers.GetPeerAddr(ctx), g.sdn.NetworkNum(), utils.GRPC)
	txHash, ok, err := servers.HandleSingleTransaction(g.feedManager, req.Transaction, nil, grpc,
		req.ValidatorsOnly, req.NextValidator, req.NodeValidation, req.FrontrunningProtection, uint16(req.Fallback),
		g.feedManager.GetNextValidatorMap(), g.feedManager.GetValidatorStatusMap())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if !ok {
		return nil, nil
	}

	log.Infof("grpc blxr_tx: Hash - 0x%v", txHash)
	return &pb.BlxrTxReply{TxHash: txHash}, nil
}

func (g *gateway) BlxrBatchTX(ctx context.Context, req *pb.BlxrBatchTXRequest) (*pb.BlxrBatchTXReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, false)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	var txHashes []*pb.TxIndex
	var txErrors []*pb.ErrorIndex
	transactionsAndSenders := req.GetTransactionsAndSenders()

	batchTxLimit := 10
	if len(transactionsAndSenders) > batchTxLimit {
		txError := fmt.Sprintf("blxr-batch-tx currently supports a maximum of %v transactions", batchTxLimit)
		txErrors = append(txErrors, &pb.ErrorIndex{Idx: 0, Error: txError})
		return &pb.BlxrBatchTXReply{TxErrors: txErrors}, nil
	}

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	grpc := connections.NewRPCConn(*accountID, servers.GetPeerAddr(ctx), g.sdn.NetworkNum(), utils.GRPC)

	for idx, transactionsAndSender := range transactionsAndSenders {
		tx := transactionsAndSender.GetTransaction()
		txHash, ok, err := servers.HandleSingleTransaction(g.feedManager, tx, transactionsAndSender.GetSender(), grpc,
			req.ValidatorsOnly, req.NextValidator, req.NodeValidation, req.FrontrunningProtection,
			uint16(req.Fallback), g.feedManager.GetNextValidatorMap(), g.feedManager.GetValidatorStatusMap())
		if err != nil {
			txErrors = append(txErrors, &pb.ErrorIndex{Idx: int32(idx), Error: err.Error()})
			continue
		}
		if !ok {
			continue
		}
		txHashes = append(txHashes, &pb.TxIndex{Idx: int32(idx), TxHash: txHash})
	}

	g.log.WithFields(log.Fields{
		"networkTime":    startTime.Sub(time.Unix(0, req.GetSendingTime())),
		"handleTime":     time.Now().Sub(startTime),
		"txsSuccess":     len(txHashes),
		"txsError":       len(txErrors),
		"validatorsOnly": req.ValidatorsOnly,
		"nextValidator":  req.NextValidator,
		"fallback":       req.Fallback,
		"nodeValidation": req.NodeValidation,
	}).Debug("blxr-batch-tx")

	return &pb.BlxrBatchTXReply{TxHashes: txHashes, TxErrors: txErrors}, nil
}

func (g *gateway) NewTxs(req *pb.TxsRequest, stream pb.Gateway_NewTxsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true)
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.grpcHandler.NewTxs(req, stream, *accountModel)
}

func (g *gateway) PendingTxs(req *pb.TxsRequest, stream pb.Gateway_PendingTxsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true)
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.grpcHandler.PendingTxs(req, stream, *accountModel)
}

func (g *gateway) NewBlocks(req *pb.BlocksRequest, stream pb.Gateway_NewBlocksServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true)
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.grpcHandler.NewBlocks(req, stream, *accountModel)
}

func (g *gateway) BdnBlocks(req *pb.BlocksRequest, stream pb.Gateway_BdnBlocksServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true)
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}

	return g.grpcHandler.BdnBlocks(req, stream, *accountModel)
}

func (g *gateway) EthOnBlock(req *pb.EthOnBlockRequest, stream pb.Gateway_EthOnBlockServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true)
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return g.grpcHandler.EthOnBlock(req, stream, *accountModel)
}

func (g *gateway) ShortIDs(ctx context.Context, req *pb.TxHashListRequest) (*pb.ShortIDListReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, false)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.blockProposer.ShortIDs(ctx, req)
}

func (g *gateway) ProposedBlock(ctx context.Context, req *pb.ProposedBlockRequest) (*pb.ProposedBlockReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, false)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.blockProposer.ProposedBlock(ctx, req)
}

func (g *gateway) BlockInfo(ctx context.Context, req *pb.BlockInfoRequest) (*pb.BlockInfoReply, error) {
	if _, err := g.validateAuthHeaderWithContext(ctx, req.AuthHeader, false, false); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.blockProposer.BlockInfo(ctx, req)
}

func (g *gateway) ProposedBlockStats(ctx context.Context, req *pb.ProposedBlockStatsRequest) (*pb.ProposedBlockStatsReply, error) {
	if _, err := g.validateAuthHeaderWithContext(ctx, req.AuthHeader, false, false); err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	return g.blockProposer.ProposedBlockStats(ctx, req)
}

func (g *gateway) bxBlockToBlockInfo(bxBlock *types.BxBlock) (*eth.BlockInfo, error) {
	// We support only types.BxBlockTypeEth. That means BSC or Polygon block
	if bxBlock.Type != types.BxBlockTypeEth {
		return nil, errUnsupportedBlockType
	}

	blockSrc, err := g.bridge.BlockBDNtoBlockchain(bxBlock)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to convert BDN block (%s) to blockchain block", bxBlock)
	}

	block, ok := blockSrc.(*eth.BlockInfo)
	if !ok {
		return nil, fmt.Errorf("failed to convert BDN block (%v) to blockchain block: %v", bxBlock, err)
	}

	return block, nil
}

func (g *gateway) onBlock(blockInfo *eth.BlockInfo) {
	if blockInfo == nil {
		return
	}

	if err := g.blockProposer.OnBlock(g.context, blockInfo.Block); err != nil {
		g.log.Debugf("failed to process block: %v", err)
		return
	}
}

func (g *gateway) TxReceipts(req *pb.TxReceiptsRequest, stream pb.Gateway_TxReceiptsServer) error {
	authHeader := retrieveAuthHeader(stream.Context(), req.AuthHeader)

	accountModel, err := g.validateAuthHeader(authHeader, true, true)
	if err != nil {
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return g.grpcHandler.TxReceipts(req, stream, *accountModel)
}

func (g *gateway) sendStats() {
	rss, err := utils.GetAppMemoryUsage()
	if err != nil {
		log.Tracef("Failed to get Process RSS size: %v", err)
	}
	g.bdnStats.SetMemoryUtilization(rss)

	closedIntervalBDNStatsMsg := g.bdnStats.CloseInterval()

	broadcastRes := g.broadcast(closedIntervalBDNStatsMsg, nil, utils.Relay)

	closedIntervalBDNStatsMsg.Log()
	g.log.Tracef("sent bdnStats msg to relays, result: [%v]", broadcastRes)
}

func (g *gateway) sendStatsOnInterval(interval time.Duration) {
	now := time.Now()

	// calculate the time until the next interval
	nextTime := now.Truncate(interval).Add(interval)
	timeUntilNextInterval := nextTime.Sub(now)
	// wait until the next interval and send it before the loop
	time.Sleep(timeUntilNextInterval)

	g.sendStats()
	ticker := g.clock.Ticker(interval)
	for {
		select {
		case <-ticker.Alert():
			g.sendStats()
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

func (g *gateway) traceIfSlow(f func(), name string, from string, count int64) {
	startTime := time.Now()
	f()
	duration := time.Since(startTime)

	if duration > time.Millisecond {
		if count > 0 {
			g.log.Tracef("%s from %v spent %v processing %v entries (%v avg)", name, from, duration, count, duration.Microseconds()/count)
		} else {
			g.log.Tracef("%s from %v spent %v processing %v entries", name, from, duration, count)
		}
	}
}

func (g *gateway) forwardBSCTx(conn *http.Client, txHash string, tx string, endpoint string, rpcType string) {
	if endpoint == "" || rpcType == "" || conn == nil {
		return
	}

	params, err := json.Marshal([]string{tx})
	l := g.log.WithFields(log.Fields{
		"endpoint": endpoint,
		"txHash":   txHash,
	})
	if err != nil {
		l.Errorf("failed to send tx, error for serializing request param, %v", err)
		return
	}

	httpReq := jsonrpc2.Request{
		Method: rpcType,
		Params: (*json.RawMessage)(&params),
	}

	reqBody, err := httpReq.MarshalJSON()
	if err != nil {
		l.Errorf("failed to send tx, error for serializing request, %v", err)
		return
	}

	resp, err := conn.Post(endpoint, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		l.Errorf("failed to send tx, error when send POST request, %v", err)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		l.Errorf("failed to read response, %v", err)
		return
	}
	l.WithFields(log.Fields{
		"rpcMethod":  httpReq.Method,
		"response":   string(body),
		"statusCode": resp.StatusCode,
	}).Info("transaction sent")
}

func (g *gateway) TxsFromShortIDs(ctx context.Context, req *pb.ShortIDListRequest) (*pb.TxListReply, error) {
	authHeader := retrieveAuthHeader(ctx, req.AuthHeader)

	_, err := g.validateAuthHeader(authHeader, false, false)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}

	shortIDs := req.GetShortIDs()
	if len(shortIDs) == 0 {
		return nil, errors.New("missing shortIDs")
	}

	txList := make([][]byte, 0, len(shortIDs))

	for _, shortID := range shortIDs {
		if shortID == 0 {
			txList = append(txList, []byte{})
			continue
		}
		txStoreTx, err := g.TxStore.GetTxByShortID(types.ShortID(shortID))
		if err != nil {
			return nil, errors.New("failed decompressing")
		}
		txList = append(txList, hexutil.Bytes(txStoreTx.Content()))
	}

	return &pb.TxListReply{
		Txs: txList,
	}, nil
}
