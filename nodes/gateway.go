package nodes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/sourcegraph/jsonrpc2"
	"golang.org/x/sync/errgroup"

	"github.com/bloXroute-Labs/bxcommon-go/cert"
	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/sdnsdk"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	bxcommoneth "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	"github.com/bloXroute-Labs/gateway/v2/metrics"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	handler2 "github.com/bloXroute-Labs/gateway/v2/servers/handler"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/account"
	"github.com/bloXroute-Labs/gateway/v2/services/feed"
	"github.com/bloXroute-Labs/gateway/v2/services/loggers"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/services/validator"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/version"
)

const (
	bscMainnetBloomCap = 259e6
	mainnetBloomCap    = 42e6

	bloomStoreInterval = time.Hour

	// https://github.com/bnb-chain/BEPs/blob/master/BEPs/BEP-524.md
	bscBlocksPerEpoch = 1000

	relayMapInterval     = 2 * time.Minute
	fastestRelayInterval = 12 * time.Hour

	addressLength      = 20
	bLSPublicKeyLength = 48

	// follow order in extra field https://github.com/bnb-chain/bsc/blob/5735d8a56540e8f2fb26d5585de0fa3959bb17b4/cmd/extradump/main.go#L21
	// |---Extra Vanity---|---Validators Number and Validators Bytes (or Empty)---|---Turn Length (or Empty)---|---Vote Attestation (or Empty)---|---Extra Seal---|
	extraVanityLength    = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	validatorNumberSize  = 1  // Fixed number of extra prefix bytes reserved for validator number after Luban
	validatorBytesLength = addressLength + bLSPublicKeyLength
	extraSealLength      = 65                                      // Fixed number of extra-data suffix bytes reserved for signer seal
	extraDataLength      = extraVanityLength + extraSealLength + 1 // 32 + 65 + 1
	oFACInterval         = 7 * 24 * time.Hour
)

var (
	errUnsupportedBlockType     = errors.New("block type is not supported")
	errIgnored                  = errors.New("ignored request")
	errExtraDataTooSmall        = errors.New("wrong extra data, too small")
	errNotAlignedValidatorsList = errors.New("parse validators failed, validator list is not aligned")
)

type gateway struct {
	Bx
	context context.Context

	sslCerts           *cert.SSLCerts
	sdn                sdnsdk.SDNHTTP
	accountID          bxtypes.AccountID
	bridge             blockchain.Bridge
	feedManager        *feed.Manager
	validatorsManager  *validator.Manager
	asyncMsgChannel    chan services.MsgInfo
	bdnStats           *bxmessage.BdnPerformanceStats
	blockProcessor     services.BlockProcessor
	blobProcessor      services.BlobProcessor
	pendingTxs         services.HashHistory
	possiblePendingTxs services.HashHistory
	txTrace            loggers.TxTrace
	blockchainPeers    []types.NodeEndpoint
	stats              statistics.Stats
	bdnBlocks          services.HashHistory
	newBlocks          services.HashHistory
	wsManager          blockchain.WSManager
	syncedWithRelay    atomic.Bool
	clock              clock.Clock
	timeStarted        time.Time
	burstLimiter       services.AccountBurstLimiter

	bestBlockHeight       int
	bdnBlocksSkipCount    int
	seenMEVSearchers      services.HashHistory
	seenBlockConfirmation services.HashHistory
	seenBeaconMessages    services.HashHistory

	bscTxClient      *http.Client
	gatewayPublicKey string

	staticEnodesCount            int
	startupArgs                  string
	validatorStatusMap           *syncmap.SyncMap[string, bool]           // validator addr -> online/offline
	validatorListMap             *syncmap.SyncMap[uint64, validator.List] // block height -> list of validators with turn length
	validatorListReady           bool
	validatorInfoUpdateLock      sync.Mutex
	latestValidatorInfo          []*types.FutureValidatorInfo
	latestValidatorInfoHeight    uint64
	transactionSlotStartDuration int
	transactionSlotEndDuration   int
	nextBlockTime                time.Time
	bloomFilter                  services.BloomFilter
	txIncludeSenderInFeed        bool
	ofacListEndpoint             string
	ofacBackupListEndpoint       string

	blockTime time.Duration

	txsQueue      *services.MessageQueue
	txsOrderQueue *services.MessageQueue

	clientHandler *servers.ClientHandler
	log           *log.Entry
	chainID       int64

	blobsManager    *beacon.BlobSidecarCacheManager
	ignoredRelays   *syncmap.SyncMap[string, bxtypes.RelayInfo]
	relaysToSwitch  *syncmap.SyncMap[string, bool]
	ofacMap         *types.OFACMap
	senderExtractor *services.SenderExtractor
}

func (g *gateway) startOFACUpdater() {
	log.Info("starting OFAC updater")
	for g.fetchOFACList() != nil {
		log.Error("failed to load OFAC data, retrying in 30s...")
		time.Sleep(30 * time.Second)
	}
	ticker := time.NewTicker(oFACInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Info("Weekly OFAC update triggered")
			// we don't care about the error, we can use an old/backup list
			_ = g.fetchOFACList() //nolint:errcheck
		case <-g.context.Done():
			log.Info("OFAC updater stopped")
			return
		}
	}
}

func (g *gateway) fetchOFACList() error {
	ofac, err := tryFetchComplianceList(g.context, g.ofacListEndpoint)
	if err == nil {
		g.ofacMap = ofac
		return nil
	}
	log.Errorf("failed to fetch ofac list from %v, err: %v", g.ofacListEndpoint, err)
	ofac, err = tryFetchComplianceList(g.context, g.ofacBackupListEndpoint)
	if err != nil {
		log.Errorf("failed to fetch ofac list from %v, err: %v", g.ofacBackupListEndpoint, err)
		return err
	}
	g.ofacMap = ofac
	return nil
}

func tryFetchComplianceList(ctx context.Context, url string) (*types.OFACMap, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}

	m := syncmap.NewStringMapOf[bool]()
	for addr := range raw {
		m.Store(addr, true)
	}

	return m, nil
}

// GeneratePeers generate string peers separated by comma
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
	blobsManager *beacon.BlobSidecarCacheManager,
	blockchainPeers []types.NodeEndpoint,
	recommendedPeers map[string]struct{},
	gatewayPublicKeyStr string,
	sdn sdnsdk.SDNHTTP,
	sslCerts *cert.SSLCerts,
	staticEnodesCount int,
	transactionSlotStartDuration int,
	transactionSlotEndDuration int,
	enableBloomFilter bool,
	txIncludeSenderInFeed bool,
	oFACListEndpoint string,
	oFACBackupListEndpoint string,
) (Node, error) {
	clock := clock.RealClock{}
	blockTime := bxtypes.NetworkToBlockDuration(bxConfig.BlockchainNetwork)

	g := &gateway{
		Bx:                           NewBx(bxConfig, "datadir", nil),
		bridge:                       bridge,
		wsManager:                    wsManager,
		context:                      parent,
		blockchainPeers:              blockchainPeers,
		pendingTxs:                   services.NewHashHistory("pendingTxs", 15*time.Minute),
		possiblePendingTxs:           services.NewHashHistory("possiblePendingTxs", 15*time.Minute),
		bdnBlocks:                    services.NewHashHistory("bdnBlocks", 15*time.Minute),
		newBlocks:                    services.NewHashHistory("newBlocks", 15*time.Minute),
		seenMEVSearchers:             services.NewHashHistory("mevSearcher", 30*time.Minute),
		seenBlockConfirmation:        services.NewHashHistory("blockConfirmation", 30*time.Minute),
		seenBeaconMessages:           services.NewHashHistory("beaconMessages", 30*time.Minute),
		clock:                        clock,
		timeStarted:                  clock.Now(),
		gatewayPublicKey:             gatewayPublicKeyStr,
		staticEnodesCount:            staticEnodesCount,
		transactionSlotStartDuration: transactionSlotStartDuration,
		transactionSlotEndDuration:   transactionSlotEndDuration,
		sdn:                          sdn,
		sslCerts:                     sslCerts,
		blockTime:                    blockTime,
		txIncludeSenderInFeed:        txIncludeSenderInFeed,
		ofacListEndpoint:             oFACListEndpoint,
		ofacBackupListEndpoint:       oFACBackupListEndpoint,
		ignoredRelays:                syncmap.NewStringMapOf[bxtypes.RelayInfo](),
		relaysToSwitch:               syncmap.NewStringMapOf[bool](),
		ofacMap:                      syncmap.NewStringMapOf[bool](),
		senderExtractor:              services.NewSenderExtractor(),
		log: log.WithFields(log.Fields{
			"component": "gateway",
		}),
		blobsManager: blobsManager,
	}
	g.chainID = int64(bxtypes.NetworkNumToChainID[sdn.NetworkNum()])

	if bxConfig.BlockchainNetwork == bxtypes.BSCMainnet {
		g.validatorStatusMap = syncmap.NewStringMapOf[bool]()
	}

	if bxConfig.BlockchainNetwork == bxtypes.BSCMainnet || bxConfig.BlockchainNetwork == bxtypes.BSCTestnet {
		g.validatorListMap = syncmap.NewIntegerMapOf[uint64, validator.List]()
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

	if g.validatorStatusMap != nil {
		g.validatorsManager = validator.NewManager(g.validatorStatusMap, g.validatorListMap)
	}

	g.asyncMsgChannel = services.NewAsyncMsgChannel(g)

	// create tx store service pass to eth client
	g.bdnStats = bxmessage.NewBDNStats(blockchainPeers, recommendedPeers)
	g.burstLimiter = services.NewAccountBurstLimiter(g.clock)

	if g.BxConfig.NoStats {
		g.stats = statistics.NoStats{}
	} else {
		g.stats = statistics.NewStats(g.BxConfig.FluentDEnabled, g.BxConfig.FluentDHost, g.sdn.NodeID(), g.sdn.Networks(), g.BxConfig.LogNetworkContent)
	}

	g.txsQueue = services.NewMsgQueue(runtime.NumCPU()*2, bxgateway.ParallelQueueChannelSize, g.msgAdapter)
	g.txsOrderQueue = services.NewMsgQueue(1, bxgateway.ParallelQueueChannelSize, g.msgAdapter)

	if enableBloomFilter {
		var bloomCap uint32
		switch bxConfig.BlockchainNetwork {
		case bxtypes.Mainnet:
			bloomCap = mainnetBloomCap
		case bxtypes.BSCMainnet, bxtypes.BSCTestnet:
			bloomCap = bscMainnetBloomCap
		default:
			// default to mainnet
			bloomCap = mainnetBloomCap
		}

		var err error
		g.bloomFilter, err = services.NewBloomFilter(context.Background(), clock, bloomStoreInterval, bxConfig.DataDir, bloomCap, bxgateway.BloomFilterQueueSize)
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
	g.TxStore = services.NewEthTxStore(g.clock, 30*time.Minute, 10*time.Minute,
		assigner, services.NewHashHistory("seenTxs", 30*time.Minute), nil, *g.sdn.Networks(), g.bloomFilter, services.NewBlobCompressorStorage(), false, g.senderExtractor)
	g.blockProcessor = services.NewBlockProcessor(g.TxStore)
	g.blobProcessor = services.NewBlobProcessor(g.TxStore, g.blobsManager)
}

// InitSDN initialize SDN, get account model
func InitSDN(bxConfig *config.Bx, blockchainPeers []types.NodeEndpoint, gatewayPeers string, staticEnodesCount int) (*cert.SSLCerts, sdnsdk.SDNHTTP, error) {
	_, err := os.Stat(".dockerignore")
	isDocker := !os.IsNotExist(err)
	hostname, _ := os.Hostname()

	privateCertDir := path.Join(bxConfig.DataDir, "ssl")
	gatewayType := bxConfig.NodeType
	privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile := cert.GetCertDir(bxConfig.RegistrationCertDir, privateCertDir, strings.ToLower(gatewayType.String()))
	sslCerts := cert.NewSSLCertsFromFiles(privateCertFile, privateKeyFile, registrationOnlyCertFile, registrationOnlyKeyFile)
	blockchainPeerEndpoint := types.NodeEndpoint{IP: "", Port: 0, PublicKey: ""}
	if len(blockchainPeers) > 0 {
		blockchainPeerEndpoint.IP = blockchainPeers[0].IP
		blockchainPeerEndpoint.Port = blockchainPeers[0].Port
		blockchainPeerEndpoint.PublicKey = blockchainPeers[0].PublicKey
	}

	nodeModel := sdnmessage.NodeModel{
		NodeType:             gatewayType.String(),
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
		NodeStartTime:        time.Now().String(),
		StartupArgs:          strings.Join(os.Args[1:], " "),
		BlockchainRPCEnabled: bxConfig.EnableBlockchainRPC,
	}

	sdn := sdnsdk.NewSDNHTTP(&sslCerts, bxConfig.SDNURL, nodeModel, bxConfig.DataDir)

	err = sdn.InitGateway(bxtypes.EthereumProtocol, bxConfig.BlockchainNetwork)
	if err != nil {
		return nil, nil, err
	}

	accountModel := sdn.AccountModel()
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

		// close the file writer when the gateway is done
		defer txTraceLogger.Close()
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
	go g.senderExtractor.Run()

	sslCert := g.sslCerts

	blockchainNetwork, err := g.sdn.FindNetwork(networkNum)
	if err != nil {
		return fmt.Errorf("failed to find the blockchainNetwork with networkNum %v, %v", networkNum, err)
	}

	g.feedManager = feed.NewManager(g.sdn,
		services.NewNoOpSubscriptionServices(),
		accountModel,
		g.stats,
		g.sdn.NetworkNum(),
		g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled,
		&metrics.NoOpExporter{},
	)

	txFromFieldIncludable := blockchainNetwork.EnableCheckSenderNonce || g.txIncludeSenderInFeed

	if g.ofacListEndpoint != "" {
		group.Go(func() error {
			g.startOFACUpdater()
			return nil
		})
	}
	// start feed manager and servers if websocket or gRPC is enabled
	if g.BxConfig.WebsocketEnabled || g.BxConfig.WebsocketTLSEnabled || g.BxConfig.GRPC.Enabled {
		group.Go(func() error {
			g.feedManager.Start(ctx)
			return nil
		})
	}

	accService := account.NewService(g.sdn, g.log)

	g.clientHandler = servers.NewClientHandler(&g.Bx, g.BxConfig, g, g.sdn, accService, g.bridge,
		g.blockchainPeers, services.NewNoOpSubscriptionServices(), g.wsManager, g.bdnStats,
		g.timeStarted, g.gatewayPublicKey, g.feedManager, g.stats, g.TxStore,
		txFromFieldIncludable, sslCert.PrivateCertFile(), sslCert.PrivateKeyFile(), g.ofacMap, g.senderExtractor,
	)

	group.Go(func() error {
		return g.clientHandler.ManageServers(ctx, g.BxConfig.ManageWSServer)
	})

	go g.PingLoop()

	relayInstructions := make(chan sdnsdk.RelayInstruction)
	go g.updateRelayConnections(relayInstructions, *sslCert, networkNum)
	err = g.sdn.DirectRelayConnections(g.BxConfig.Relays, uint64(accountModel.RelayLimit.MsgQuota.Limit), relayInstructions, g.ignoredRelays)
	go g.checkForFastestRelays(relayInstructions)
	go g.cleanupRelayMap()
	if err != nil {
		return err
	}

	go g.sendStatsOnInterval(15 * time.Minute)

	go g.handleBlockchainConnectionStatusUpdate()

	return group.Wait()
}

func (g *gateway) Close() error {
	if g.clientHandler != nil {
		return g.clientHandler.Stop()
	}

	return nil
}

func (g *gateway) updateRelayConnections(relayInstructions chan sdnsdk.RelayInstruction, sslCerts cert.SSLCerts, networkNum bxtypes.NetworkNum) {
	for {
		instruction := <-relayInstructions

		switch instruction.Type {
		case sdnsdk.Connect:
			g.connectRelay(instruction, sslCerts, networkNum, relayInstructions)
		case sdnsdk.Switch:
			go g.switchRelay(instruction, relayInstructions, sslCerts, networkNum)
		case sdnsdk.Disconnect:
			// disconnectRelay
		}
	}
}

func (g *gateway) switchRelay(instruction sdnsdk.RelayInstruction, relayInstructions chan sdnsdk.RelayInstruction, sslCerts cert.SSLCerts, networkNum bxtypes.NetworkNum) {
	for _, newRelay := range instruction.RelaysToSwitch {
		// add relay to ignore relays so if we have 2 relays connected we will not connect to same relay
		if _, ok := g.ignoredRelays.LoadOrStore(newRelay.IP, bxtypes.RelayInfo{TimeAdded: time.Now(), IsConnected: true, Port: newRelay.Port}); ok {
			continue
		}
		relayInstruction := sdnsdk.RelayInstruction{IP: newRelay.IP, Port: newRelay.Port, Type: sdnsdk.Connect}
		isRelayConnected := g.tryToConnectToAutoRelay(relayInstruction, sslCerts, networkNum, relayInstructions)
		if isRelayConnected {
			err := g.disconnectRelay(instruction.IP)
			if err != nil {
				log.Errorf("error disconnecting relay: %v", err)
			}
			return
		}
		// if relay wasn't able to connect we will mark it as not connected
		g.ignoredRelays.Store(newRelay.IP, bxtypes.RelayInfo{TimeAdded: time.Now(), IsConnected: false, Port: newRelay.Port})
	}
}

func (g *gateway) disconnectRelay(relayIP string) error {
	for _, conn := range g.Connections {
		if conn.GetPeerIP() == relayIP {
			g.relaysToSwitch.Store(relayIP, true)
			return conn.Close("found faster relay")
		}
	}
	return nil
}

func (g *gateway) tryToConnectToAutoRelay(instruction sdnsdk.RelayInstruction, sslCerts cert.SSLCerts, networkNum bxtypes.NetworkNum, relayInstructions chan sdnsdk.RelayInstruction) bool {
	relay := handler.NewOutboundRelay(g, &sslCerts, instruction.IP, instruction.Port, g.sdn.NodeID(), bxtypes.RelayProxy,
		g.sdn.Networks(), true, false, clock.RealClock{}, false)
	relay.SetNetworkNum(networkNum)

	ctx, cancel := context.WithCancel(context.Background())
	go relay.Start(ctx)
	// let the relay connect for few secs
	time.Sleep(3 * time.Second)
	g.ConnectionsLock.Lock()
	defer g.ConnectionsLock.Unlock()
	for _, conn := range g.Connections {
		// if new relay was connected we need to disconnect old relay
		if conn.GetPeerIP() == instruction.IP {
			go g.monitorConnection(cancel, relay, relayInstructions)
			return true
		}
	}
	// if after 3 secs relay wasn't able to connect we will stop try and move to next relay
	cancel()
	return false
}

func (g *gateway) connectRelay(instruction sdnsdk.RelayInstruction, sslCerts cert.SSLCerts, networkNum bxtypes.NetworkNum, relayInstructions chan sdnsdk.RelayInstruction) {
	relay := handler.NewOutboundRelay(g, &sslCerts, instruction.IP, instruction.Port, g.sdn.NodeID(), bxtypes.RelayProxy,
		g.sdn.Networks(), true, false, clock.RealClock{}, false)
	relay.SetNetworkNum(networkNum)

	ctx := g.context
	// monitor and switch only auto relays
	if !instruction.IsStatic {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		go g.monitorConnection(cancel, relay, relayInstructions)
	}

	go relay.Start(ctx)

	g.log.WithFields(log.Fields{
		"gateway":   g.sdn.NodeID(),
		"relayIP":   instruction.IP,
		"relayPort": instruction.Port,
	}).Info("connecting to relay")
}

func (g *gateway) monitorConnection(cancel context.CancelFunc, relay *handler.Relay, relayInstructions chan sdnsdk.RelayInstruction) {
	ticker := time.NewTicker(types.RelayMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.context.Done():
			log.Tracef("Context canceled, stopping connection monitoring")
			cancel()
			return
		case <-ticker.C:
			if g.isRelaySwitched(relay.GetPeerIP()) {
				return
			}
			if !relay.BxConn.IsOpen() {
				log.Tracef("Relay %v connection down, waiting an additional %v to confirm.", relay.GetPeerIP(), types.RelayMonitorInterval)

				// Reset the timer to wait an additional interval for reconnection attempt
				<-time.After(types.RelayMonitorInterval)
				if g.isRelaySwitched(relay.GetPeerIP()) {
					return
				}

				if !relay.BxConn.IsOpen() {
					cancel()
					g.sdn.FindNewRelay(g.context, relay.GetPeerIP(), relay.GetPeerPort(), relayInstructions, g.ignoredRelays)
					return
				}
			}
		}
	}
}

func (g *gateway) isRelaySwitched(relayIP string) bool {
	if _, ok := g.relaysToSwitch.LoadAndDelete(relayIP); ok {
		log.Tracef("Relay %v is no longer active, stopping monitor", relayIP)
		g.ignoredRelays.Delete(relayIP)
		return true
	}
	return false
}

func (g *gateway) cleanupRelayMap() {
	ticker := time.NewTicker(relayMapInterval)
	defer ticker.Stop()
	for range ticker.C {
		g.ignoredRelays.Range(func(ip string, info bxtypes.RelayInfo) bool {
			if !info.IsConnected && time.Since(info.TimeAdded) > relayMapInterval {
				g.ignoredRelays.Delete(ip)
			}
			return true
		})
	}
}

func (g *gateway) checkForFastestRelays(relayInstructions chan<- sdnsdk.RelayInstruction) {
	ticker := time.NewTicker(fastestRelayInterval)
	defer ticker.Stop()
	for {
		select {
		case <-g.context.Done():
			log.Tracef("Context canceled, stopping check for fastest relay")
			return
		case <-ticker.C:
			g.sdn.FindFastestRelays(relayInstructions, g.ignoredRelays)
		}
	}
}

func (g *gateway) broadcast(msg bxmessage.Message, source connections.Conn, to bxtypes.NodeType) types.BroadcastResults {
	return g.Broadcast(msg, source, to)
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

				validatorList, err := bscExtractValidatorListFromBlock(data)
				if err != nil {
					return fmt.Errorf("failed to extract validator list from extraData: %v", err)
				}

				g.validatorListMap.Delete(height - bscBlocksPerEpoch*2) // remove the validator list that doesn't need anymore
				g.validatorListMap.Store(height, validatorList)

				return nil
			}
		}
	}

	return errors.New("failed to query blockchain node for previous epoch block")
}

func (g *gateway) generateBSCValidator(blockHeight uint64) []*types.FutureValidatorInfo {
	vi := blockchain.DefaultValidatorInfo(blockHeight)

	if g.validatorsManager == nil {
		return vi
	}

	// currentEpochBlockHeight will be the most recent block height that can be module by amount of blocks in an epoch
	currentEpochBlockHeight := blockHeight / bscBlocksPerEpoch * bscBlocksPerEpoch
	previousEpochBlockHeight := currentEpochBlockHeight - bscBlocksPerEpoch

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

	var currentEpochValidatorList validator.List
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

		// see formula here https://github.com/bnb-chain/BEPs/blob/master/BEPs/BEP-341.md#421-priority-allocation
		listIndex := targetingBlockHeight / uint64(currentEpochValidatorList.TurnLength) % uint64(len(currentEpochValidatorList.Validators))

		// see formula here https://github.com/bnb-chain/BEPs/blob/master/BEPs/BEP-341.md#422-validator-set-switch
		activationIndex := uint64(len(previousEpochValidatorList.Validators)/2+1)*uint64(currentEpochValidatorList.TurnLength) - 1

		index := targetingBlockHeight - currentEpochBlockHeight
		if index >= activationIndex {
			validatorAddr := currentEpochValidatorList.Validators[listIndex]
			vi[i-1].WalletID = strings.ToLower(validatorAddr)
		} else { // use list from previous epoch
			validatorAddr := previousEpochValidatorList.Validators[listIndex]
			vi[i-1].WalletID = strings.ToLower(validatorAddr)
		}

		vi[i-1].Accessible = true
	}

	return vi
}

func bscExtractValidatorListFromBlock(b []byte) (validator.List, error) {
	// 32 + 65 + 1
	if len(b) < extraDataLength {
		return validator.List{}, errExtraDataTooSmall
	}

	data := b[extraVanityLength : len(b)-extraSealLength]
	dataLength := len(data)

	// parse Validators and Vote Attestation
	if dataLength > 0 {
		// parse Validators
		if data[0] != '\xf8' { // rlp format of attestation begin with 'f8'
			validatorNum := int(data[0])
			validatorBytesTotalLength := validatorNumberSize + validatorNum*validatorBytesLength
			if dataLength < validatorBytesTotalLength {
				return validator.List{}, errNotAlignedValidatorsList
			}

			validatorList := make([]string, 0, validatorNum)
			data = data[validatorNumberSize:]
			for i := 0; i < validatorNum; i++ {
				validatorAddr := common.BytesToAddress(data[i*validatorBytesLength : i*validatorBytesLength+common.AddressLength])
				validatorList = append(validatorList, validatorAddr.String())
			}

			data = data[validatorBytesTotalLength-validatorNumberSize:]
			dataLength = len(data)

			turnLength := uint8(1)
			// parse TurnLength
			if dataLength > 0 && data[0] != '\xf8' {
				turnLength = data[0]
			}

			return validator.List{
				Validators: validatorList,
				TurnLength: turnLength,
			}, nil
		}
	}

	return validator.List{}, nil
}

func (g *gateway) generateFutureValidatorInfo(block *types.BxBlock, blockInfo *core.BlockInfo) []*types.FutureValidatorInfo {
	g.validatorInfoUpdateLock.Lock()
	defer g.validatorInfoUpdateLock.Unlock()

	blockHeight := block.Number.Uint64()

	if (g.sdn.NetworkNum() == bxtypes.BSCMainnetNum || g.sdn.NetworkNum() == bxtypes.BSCTestnetNum) && blockHeight%bscBlocksPerEpoch == 0 {
		validatorList, err := bscExtractValidatorListFromBlock(blockInfo.Block.Extra())
		if err != nil {
			g.log.Errorf("failed to extract validator list from extra data: %v", err)
		} else {
			g.validatorListMap.Delete(blockHeight - bscBlocksPerEpoch) // remove the validator list that doesn't need anymore
			g.validatorListMap.Store(blockHeight, validatorList)
		}
	}

	if blockHeight <= g.latestValidatorInfoHeight {
		return g.latestValidatorInfo
	}
	g.latestValidatorInfoHeight = blockHeight

	switch g.sdn.NetworkNum() {
	case bxtypes.BSCMainnetNum, bxtypes.BSCTestnetNum:
		g.latestValidatorInfo = g.generateBSCValidator(blockHeight)
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
	// Why not to check already seen before converting to blockchain block?
	block, err := g.bridge.BlockBDNtoBlockchain(bxBlock)
	if err != nil {
		return fmt.Errorf("cannot convert BxBlock to blockchain block: %v", err)
	}
	l := g.log.WithFields(log.Fields{
		"bxBlock": bxBlock,
		"source":  nodeSource,
	})
	var addedNewBlock, addedBdnBlock bool

	notifyEthBlockFeeds := func(block *bxcommoneth.Block, nodeSource *connections.Blockchain, info []*types.FutureValidatorInfo, isBlockchainBlock bool) error {
		ethNotification, err := types.NewEthBlockNotification(common.Hash(bxBlock.ExecutionHash()), block, info)
		if err != nil {
			return err
		}

		if addedBdnBlock {
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
			if addedNewBlock {
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

		addedBdnBlock = g.bdnBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute)
		if addedBdnBlock {
			// Send beacon notifications to BDN feed even if source is blockchain
			notification := beaconNotification.Clone()
			notification.SetNotificationType(types.BDNBeaconBlocksFeed)
			g.notify(notification)
		}

		if isBlockchainBlock {
			addedNewBlock = g.newBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute)
			if addedNewBlock {
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
	case *core.BlockInfo:
		addedBdnBlock = g.bdnBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute)
		if isBlockchainBlock {
			addedNewBlock = g.newBlocks.SetIfAbsent(bxBlock.Hash().String(), 15*time.Minute)
		}
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

	wsProvider, err := g.wsManager.ProviderWithBlock(nodeEndpoint, ethNotification.Header.GetNumber())
	if err != nil {
		log.Warn(err)
		g.notifyError(feed.ErrorNotification{ErrorMsg: err.Error(), FeedType: types.TxReceiptsFeed})
		g.notifyError(feed.ErrorNotification{ErrorMsg: err.Error(), FeedType: types.OnBlockFeed})
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
		receipts, err := handler2.HandleTxReceipts(g.wsManager, notification.(*types.EthBlockNotification))
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
		case request := <-g.bridge.ReceiveTransactionHashesRequestFromNode():
			txs := make([]*types.BxTransaction, 0)
			for _, hash := range request.Hashes {
				bxTx, exists := g.TxStore.Get(hash)
				if !exists {
					continue
				}

				txs = append(txs, bxTx)
			}

			err = g.bridge.SendRequestedTransactionsToNode(request.RequestID, txs)
			if errors.Is(err, blockchain.ErrChannelFull) {
				g.log.Warnf("requested transactions channel is full, skipping request")
			} else if err != nil {
				panic(fmt.Errorf("could not send requested transactions over bridge: %v", err))
			}
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
						"hash":   hash.String(),
						"peerID": txAnnouncement.PeerID,
					})
					bxTx, exists := g.TxStore.Get(hash)
					if !exists && !g.TxStore.Known(hash) {
						g.log.Trace("msgTx: from Blockchain, event TxAnnouncedByBlockchainNode")
						requests = append(requests, hash)
					} else {
						var diffFromBDNTime int64
						var delivered bool
						expected := "expected"
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
					if errors.Is(err, blockchain.ErrChannelFull) {
						g.log.Warnf("transaction requests channel is full, skipping request")
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
				g.log.Fatalf("Gateway does not have an active blockchain connection. Enterprise-Elite account is required in order to run gateway without a blockchain node.")
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
		case beaconMessage := <-g.bridge.ReceiveBeaconMessageFromNode():
			g.traceIfSlow(func() {
				g.handleBeaconMessageFromBlockchain(&beaconMessage)
			},
				fmt.Sprintf("handleBeaconMessageFromBlockchain hash=[%s]", beaconMessage.Message.Hash), beaconMessage.PeerEndpoint.String(), 1)
		}
	}
}

func (g *gateway) handleBeaconMessageFromBlockchain(beaconMessageFromNode *blockchain.BeaconMessageFromNode) {
	startTime := g.clock.Now()
	if !g.seenBeaconMessages.SetIfAbsent(beaconMessageFromNode.Message.Hash.String(), 30*time.Minute) {
		g.log.Tracef("skipping beacon message %v, already seen", beaconMessageFromNode.Message.Hash.String())
		return
	}

	beaconMessage, err := g.blobProcessor.BxBeaconMessageToBeaconMessage(beaconMessageFromNode.Message, g.sdn.NetworkNum())
	if err != nil {
		g.log.Errorf("failed to convert beaconMessage from BxBeaconMessage: %v", err)
		return
	}

	source := connections.NewBlockchainConn(beaconMessageFromNode.PeerEndpoint)

	results := g.broadcast(beaconMessage, source, bxtypes.RelayProxy)

	eventName := "GatewayReceivedBeaconMessageFromBlockchain"
	g.stats.AddBlobEvent(eventName, beaconMessage.Hash().String(), source.GetNodeID(), beaconMessage.GetNetworkNum(),
		startTime, g.clock.Now(), len(beaconMessageFromNode.Message.Data), len(beaconMessage.Data()), beaconMessage.Index(),
		beaconMessage.BlockHash().String())

	g.log.Tracef("broadcasted beacon message from Blockchain to BDN %v, from %v to peers[%v]", beaconMessage, source, results)
}

func (g *gateway) NodeStatus() connections.NodeStatus {
	var capabilities types.CapabilityFlags

	if g.BxConfig.EnableBlockchainRPC {
		capabilities |= types.CapabilityBlockchainRPCEnabled
	}

	if g.BxConfig.NoBlocks {
		capabilities |= types.CapabilityNoBlocks
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
		if typedMsg.Flags().IsForBuilders() {
			_ = g.broadcast(typedMsg, source, bxtypes.RelayProxy)
			return nil
		}
		// insert to order queue if tx is flagged as send to node and no txs to blockchain is false, and we have static peers or dynamic peers
		if typedMsg.Flags().ShouldDeliverToNode() && !g.BxConfig.NoTxsToBlockchain && (g.staticEnodesCount > 0 || g.BxConfig.EnableDynamicPeers) {
			err = g.txsOrderQueue.Insert(typedMsg, source)
		} else {
			err = g.txsQueue.Insert(typedMsg, source)
		}
		if err != nil {
			source.Log().WithField("hash", typedMsg.Hash().String()).Errorf("discarding tx: %s", err)
		}
	case *bxmessage.ValidatorUpdates:
		// accessible wallets are not supported anymore
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
			g.TxStore.Add(txsItem.Hash, txsItem.Content, txsItem.ShortID, g.sdn.NetworkNum(), false, 0, time.Now(), g.chainID, types.EmptySender)
		}
	case *bxmessage.SyncDone:
		g.setSyncWithRelay()
		err = g.Bx.HandleMsg(msg, source)
	case *bxmessage.MEVBundle:
	// Do nothing. Bundle propagation no longer accepted as of
	// https://bloxroute.atlassian.net/browse/BP-3153
	case *bxmessage.ErrorNotification:
		if typedMsg.Code < types.MinErrorNotificationCode {
			source.Log().Warnf("received a warn notification %v.", typedMsg.Reason)
		} else {
			source.Log().Fatalf("received an error notification %v. terminating the gateway", typedMsg.Reason)
			// TODO should also close the gateway while notify the bridge and other go routine (web socket server, ...)
		}
	case *bxmessage.Hello:
		source.Log().Tracef("received hello msg")
	case *bxmessage.BlockConfirmation:
		hashString := typedMsg.Hash().String()
		if g.seenBlockConfirmation.SetIfAbsent(hashString, 30*time.Minute) {
			if !g.BxConfig.SendConfirmation {
				g.log.Debug("gateway is not sending block confirm message to relay")
			} else if source.GetConnectionType() == bxtypes.Blockchain {
				g.log.Tracef("gateway broadcasting block confirmation of block %v to relays", hashString)
				g.broadcast(typedMsg, source, bxtypes.RelayProxy)
			}
			_ = g.Bx.HandleMsg(msg, source)
		}
	case *bxmessage.BeaconMessage:
		go g.processBeaconMessage(typedMsg, source)
	default:
		err = g.Bx.HandleMsg(msg, source)
	}

	return err
}

func (g *gateway) processBeaconMessage(beaconMessage *bxmessage.BeaconMessage, source connections.Conn) {
	start := g.clock.Now()
	if !g.isSyncWithRelay() {
		g.log.Tracef("skipping beacon message %v, gateway is not yet synced with relay", beaconMessage.Hash())
		return
	}
	if !g.seenBeaconMessages.SetIfAbsent(beaconMessage.Hash().String(), 30*time.Minute) {
		g.log.Tracef("skipping beacon message %v, already seen", beaconMessage.Hash())
		return
	}

	bxBeaconMessage, err := g.blobProcessor.BeaconMessageToBxBeaconMessage(beaconMessage)
	if err != nil {
		g.log.Errorf("failed to process beaconMessage from broadcast: %v", err)
		return
	}

	eventName := "GatewayReceivedBeaconMessageFromBDN"
	g.stats.AddBlobEvent(eventName, beaconMessage.Hash().String(), source.GetNodeID(), beaconMessage.GetNetworkNum(),
		start, g.clock.Now(), len(bxBeaconMessage.Data), len(beaconMessage.Data()), beaconMessage.Index(),
		beaconMessage.BlockHash().String())

	if err := g.bridge.SendBeaconMessageToBlockchain(bxBeaconMessage); err != nil {
		g.log.Errorf("could not send beacon message %v to blockchain: %v", beaconMessage, err)
		return
	}

	source.Log().Tracef("sent beacon message %v to blockchain", beaconMessage)
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
		var errAlreadyProcessed *services.ErrAlreadyProcessed
		if errors.As(err, &errAlreadyProcessed) {
			source.Log().Debugf("received duplicate %v skipping", broadcastMsg)
			return
		}
		switch {
		case errors.Is(err, services.ErrMissingShortIDs):
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
		case errors.Is(err, services.ErrNotCompatibleBeaconBlock):
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
	txResult := g.TxStore.Add(tx.Hash(), tx.Content(), tx.ShortID(), tx.GetNetworkNum(), !(isRelay || (connections.IsGrpc(connectionType) && sender != types.EmptySender)), tx.Flags(), g.clock.Now(), g.chainID, sender)

	// some flags can be changed during the process of adding transaction to the store
	// so we need to update the flags of the initial transaction as well to keep them in sync
	tx.SetFlags(txResult.Transaction.Flags())

	nodeID := source.GetNodeID()
	l := source.Log().WithFields(log.Fields{
		"hash":   tx.Hash().String(),
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
			if txResult.NewContent {
				newTxsNotification := types.CreateNewTransactionNotification(txResult.Transaction)
				g.notify(newTxsNotification)
				if !sourceEndpoint.IsDynamic() {
					g.publishPendingTx(txResult.Transaction.Hash(), txResult.Transaction, connectionType == bxtypes.Blockchain)
				}
			}

			if !isRelay {
				if connectionType == bxtypes.Blockchain {
					g.bdnStats.LogNewTxFromNode(sourceEndpoint)
				}

				paidTx := tx.Flags().IsPaid()
				var (
					allowed  bool
					behavior sdnmessage.BDNServiceBehaviorType
				)
				if connectionType == bxtypes.CloudAPI {
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
					broadcastRes = g.broadcast(tx, source, bxtypes.RelayProxy)
					sentToBDN = true
				}
			}

			shouldSendTxFromNodeToOtherNodes := connectionType == bxtypes.Blockchain && len(g.blockchainPeers) > 1 && !g.BxConfig.NoTxsToBlockchain
			shouldSendTxFromBDNToNodes := g.shouldSendTxFromBDNToNodes(connectionType, tx) && !g.BxConfig.NoTxsToBlockchain

			if shouldSendTxFromNodeToOtherNodes || shouldSendTxFromBDNToNodes {
				txsToDeliverToNodes := blockchain.Transactions{
					Transactions:   []*types.BxTransaction{txResult.Transaction},
					PeerEndpoint:   sourceEndpoint,
					ConnectionType: connectionType,
				}
				err := g.bridge.SendTransactionsFromBDN(txsToDeliverToNodes)
				if err != nil {
					l.Errorf("failed to send transaction from BDN to bridge: %v", err)
				}

				sentToBlockchainNode = true
				if shouldSendTxFromNodeToOtherNodes {
					g.bdnStats.LogTxSentToAllNodesExceptSourceNode(sourceEndpoint)
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
		if connectionType == bxtypes.Blockchain {
			g.bdnStats.LogDuplicateTxFromNode(sourceEndpoint)
		}
	}

	txSender := txResult.Transaction.Sender()

	statsStart := time.Now()
	g.stats.AddTxsByShortIDsEvent(eventName, source, txResult.Transaction, tx.ShortID(), nodeID, broadcastRes.RelevantPeers, broadcastRes.SentGatewayPeers, startTime, tx.GetPriority(), txResult.DebugData)
	statsDuration := time.Since(statsStart)
	// usage of log.WithFields 7 times slower than usage of direct log.Tracef
	handlingTime := g.clock.Now().Sub(tx.ReceiveTime()).Microseconds() - tx.WaitDuration().Microseconds()
	l.WithFields(log.Fields{
		"from":                    fmt.Sprintf("%s", source),
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
		"sender":                  txSender,
		"networkDuration":         tx.ReceiveTime().Sub(tx.Timestamp()).Microseconds(),
		"statsDuration":           statsDuration,
		"frontRunProtectionDelay": frontRunProtectionDelay,
		"waitingDuration":         tx.WaitDuration(),
		"txsInNetworkChannel":     tx.NetworkChannelPosition(),
		"txsInProcessingChannel":  tx.ProcessChannelPosition(),
		"msgLen":                  tx.Size(bxmessage.CurrentProtocol),
	}).Trace("msgTx")
}

// shouldSendTxFromBDNToNodes send to node if all are true
func (g *gateway) shouldSendTxFromBDNToNodes(connectionType bxtypes.NodeType, tx *bxmessage.Tx) (send bool) {
	if connectionType == bxtypes.Blockchain {
		return // Transaction is from blockchain node (transaction is not from Relay or RPC)
	}

	if len(g.blockchainPeers) == 0 {
		return // Gateway is not connected to nodes
	}

	if (!g.BxConfig.BlocksOnly && tx.Flags().ShouldDeliverToNode()) || // (Gateway didn't start with blocks only mode and DeliverToNode flag is on) OR
		g.BxConfig.AllTransactions { // (Gateway started with a flag to send all transactions, and this is not validator only
		send = true
	}

	return
}

const maxBlockAgeSinceNow = time.Minute * 10

func (g *gateway) handleBlockFromBlockchain(blockchainBlock blockchain.BlockFromNode) {
	startTime := time.Now()

	bxBlock := blockchainBlock.Block

	blockInfo, err := g.bxBlockToBlockInfo(bxBlock)
	if err != nil && !errors.Is(err, errUnsupportedBlockType) {
		log.Errorf("failed to convert bx block %v to block info: %v", bxBlock, err)
	}

	source := connections.NewBlockchainConn(blockchainBlock.PeerEndpoint)

	if blockInfo != nil {
		blockTime := time.Unix(int64(blockInfo.Block.Time()), 0)
		if !blockTime.IsZero() && startTime.Sub(blockTime) > maxBlockAgeSinceNow {
			source.Log().Warnf("received block %v from blockchain node with time %v, which is older than %v", bxBlock.Hash(), blockTime, maxBlockAgeSinceNow)
			return
		}
	}

	g.bdnStats.LogNewBlockMessageFromNode(source.NodeEndpoint())

	broadcastMessage, usedShortIDs, err := g.blockProcessor.BxBlockToBroadcast(bxBlock, g.sdn.NetworkNum(), g.sdn.MinTxAge())
	if err != nil {
		var processedErr *services.ErrAlreadyProcessed
		if errors.As(err, &processedErr) {
			switch processedErr.Status() {
			case services.SeenFromRelay:
				source.Log().Infof("received duplicate block %v from blockchain, block already seen from relay", bxBlock.Hash())
			case services.SeenFromNode:
				source.Log().Infof("received duplicate block %v from blockchain, block already seen from blockchain node, skipping", bxBlock.Hash())
				return
			case services.SeenFromBoth:
				source.Log().Infof("received duplicate block %v from blockchain, block already seen from blockchain node and from relay, skipping", bxBlock.Hash())
				return
			default:
				source.Log().Errorf("unexpected status %v", err)
			}
		} else {
			source.Log().Errorf("could not compress block: %v", err)
		}
	} else {
		// if not synced - avoid sending to bdn (low compression rate block)
		if !g.isSyncWithRelay() {
			source.Log().Debugf("TxSync not completed. Not sending block %v to the bdn", bxBlock.Hash())
		} else {
			source.Log().Debugf("compressed %v from blockchain node: compressed %v short IDs", bxBlock, len(usedShortIDs))
			source.Log().Infof("propagating %v from blockchain node to BDN", bxBlock)

			_ = g.broadcast(broadcastMessage, source, bxtypes.RelayProxy)

			g.bdnStats.LogNewBlockFromNode(source.NodeEndpoint())

			g.stats.AddGatewayBlockEvent(gatewayBlockEventName(source.NodeEndpoint().Name, bxBlock.IsBeaconBlock()), source, bxBlock.Hash(), bxBlock.BeaconHash(), g.sdn.NetworkNum(), 1, startTime, 0, bxBlock.Size(), int(broadcastMessage.Size(bxmessage.CurrentProtocol)), len(broadcastMessage.ShortIDs()), len(bxBlock.Txs), len(usedShortIDs), bxBlock)
		}
	}

	validatorInfo := g.generateFutureValidatorInfo(bxBlock, blockInfo)

	source.Log().Debugf("generated validator info: %v", validatorInfo)

	if err = g.publishBlock(bxBlock, &source, validatorInfo, !blockchainBlock.PeerEndpoint.IsDynamic()); err != nil {
		source.Log().Errorf("failed to publish block %v from blockchain node: %v", bxBlock, err)
	}
}

func gatewayBlockEventName(nodeName string, isBeaconBlock bool) string {
	eventName := "GatewayReceivedBlockFromBlockchainNode"
	if isBeaconBlock {
		eventName = "GatewayReceivedBeaconBlockFromBlockchainNode"
	}
	return eventName
}

func (g *gateway) processBlockFromBDN(bxBlock *types.BxBlock) {
	blockInfo, err := g.bxBlockToBlockInfo(bxBlock)
	if err != nil && !errors.Is(err, errUnsupportedBlockType) {
		g.log.Errorf("failed to convert bx block %v to block info: %v", bxBlock, err)
	}

	if err = g.bridge.SendBlockToNode(bxBlock); err != nil {
		g.log.Errorf("unable to send block %v from BDN to node: %v", bxBlock, err)
	}

	g.bdnStats.LogNewBlockFromBDN(g.BxConfig.BlockchainNetwork)
	validatorInfo := g.generateFutureValidatorInfo(bxBlock, blockInfo)
	err = g.publishBlock(bxBlock, nil, validatorInfo, false)
	if err != nil {
		g.log.Errorf("failed to publish BDN block with %v, %v", err, bxBlock)
	}
}

func (g *gateway) notify(notification types.Notification) {
	g.feedManager.Notify(notification)
}

func (g *gateway) notifyError(errorMsg feed.ErrorNotification) {
	g.feedManager.NotifyError(errorMsg)
}

func (g *gateway) getHeaderFromGateway() string {
	accountID := g.sdn.AccountModel().AccountID
	secretHash := g.sdn.AccountModel().SecretHash
	accountIDAndHash := fmt.Sprintf("%s:%s", accountID, secretHash)
	return base64.StdEncoding.EncodeToString([]byte(accountIDAndHash))
}

func (g *gateway) bxBlockToBlockInfo(bxBlock *types.BxBlock) (*core.BlockInfo, error) {
	// We support only types.BxBlockTypeEth. That means BSC block
	if bxBlock.Type != types.BxBlockTypeEth {
		return nil, errUnsupportedBlockType
	}

	blockSrc, err := g.bridge.BlockBDNtoBlockchain(bxBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to convert BDN block (%s) to blockchain block: %w", bxBlock, err)
	}

	block, ok := blockSrc.(*core.BlockInfo)
	if !ok {
		return nil, fmt.Errorf("failed to convert BDN block (%v) to blockchain block: %v", bxBlock, err)
	}

	return block, nil
}

func (g *gateway) sendStats() {
	rss, err := utils.GetAppMemoryUsage()
	if err != nil {
		log.Tracef("Failed to get Process RSS size: %v", err)
	}
	g.bdnStats.SetMemoryUtilization(rss)

	closedIntervalBDNStatsMsg := g.bdnStats.CloseInterval()

	broadcastRes := g.broadcast(closedIntervalBDNStatsMsg, nil, bxtypes.RelayProxy)

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
		g.bdnStats.SetBlockchainConnectionStatus(blockchainConnectionStatus)

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
			continue
		}

		g.sdn.SendNodeEvent(
			sdnmessage.NewBlockchainNodeConnError(g.sdn.NodeID(), blockchainIP, blockchainPort, g.clock.Now().String()),
			g.sdn.NodeID(),
		)
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
	defer resp.Body.Close()

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
