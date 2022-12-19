package beacon

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/logger"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v3/async"
	"github.com/prysmaticlabs/prysm/v3/async/event"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v3/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/peers/peerdata"
	p2ptypes "github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/sync/genesis"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v3/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v3/network/forks"
	pb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

const forkDigestLength = 4

var (
	responseCodeSuccess        = byte(0x00)
	responseCodeInvalidRequest = byte(0x01)
	responseCodeServerError    = byte(0x02)
)

var networkInitMapping = map[string]func(){
	// Mainnet is default and has required values
	"Mainnet": func() {},
	"Prater": func() {
		params.UsePraterNetworkConfig()
		params.SetActive(params.PraterConfig())
	},
	"Ropsten": func() {
		params.UseRopstenNetworkConfig()
		params.SetActive(params.RopstenConfig())
	},
}

// Node is beacon node
type Node struct {
	ctx            context.Context
	config         *network.EthConfig
	genesisState   state.BeaconState
	db             db.Database
	stateFeed      *event.Feed
	p2p            *p2p.Service
	bridge         blockchain.Bridge
	blockProcessor *blockProcessor
	beaconBlock    bool

	lStatus *pb.Status
	m       sync.RWMutex

	cancel func()
	log    *logger.Entry
}

// NewNode creates beacon node
func NewNode(parent context.Context, networkName string, config *network.EthConfig, ethChain *eth.Chain, genesisInitializer genesis.Initializer, bridge blockchain.Bridge, dataDir, externalIP string, beaconBlock bool) (*Node, error) {
	return newNode(parent, networkName, config, ethChain, genesisInitializer, bridge, dataDir, externalIP, beaconBlock, new(utils.PublicIPResolver))
}

func newNode(parent context.Context, networkName string, config *network.EthConfig, ethChain *eth.Chain, genesisInitializer genesis.Initializer, bridge blockchain.Bridge, dataDir, externalIP string, beaconBlock bool, ipResolver utils.IPResolver) (*Node, error) {
	var err error

	if err := initNetwork(networkName); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)

	n := &Node{
		ctx:         ctx,
		config:      config,
		stateFeed:   new(event.Feed),
		bridge:      bridge,
		beaconBlock: beaconBlock,
		cancel:      cancel,
		log:         logger.WithField("connType", "beacon"),
	}

	// Store genesis state
	n.db, err = db.NewDB(ctx, dataDir)
	if err != nil {
		return nil, err
	}

	if err := genesisInitializer.Initialize(ctx, n.db); err != nil {
		// Skip if we already have genesis state
		if err != db.ErrExistingGenesisState {
			return nil, err
		}
	}

	n.genesisState, err = n.db.GenesisState(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not load genesis state: %v", err)
	}

	if externalIP == "" {
		externalIP, err = ipResolver.GetPublicIP()
		if err != nil {
			panic(fmt.Errorf("could not determine node's public ip: %v. consider specifying an --external-ip address", err))
		}
	}
	n.p2p, err = n.registerP2p(path.Join(dataDir, ".gatewaykey"), externalIP)
	if err != nil {
		return nil, err
	}

	// Do not limit reconnect tries
	n.p2p.Peers().Scorers().BadResponsesScorer().Params().Threshold = math.MaxInt
	n.p2p.Peers().Scorers().BadResponsesScorer().Params().DecayInterval = time.Second

	chainAdapter := newChainAdapter(beaconBlock, ethChain, NewChain(ctx, n.genesisState.GenesisTime(), config.IgnoreSlotCount))

	n.blockProcessor = newBlockProcessor(ctx, config, chainAdapter, bridge, n.BroadcastBlock, n.log)

	n.p2p.AddConnectionHandler(n.handshake, n.goodBye)
	n.p2p.AddDisconnectionHandler(func(_ context.Context, peerID peer.ID) error {
		// no-op
		return nil
	})
	n.p2p.AddPingMethod(n.sendPingRequest)

	n.subscribeRPC(p2p.RPCStatusTopicV1, n.statusRPCHandler)
	n.subscribeRPC(p2p.RPCGoodByeTopicV1, n.goodbyeRPCHandler)
	n.subscribeRPC(p2p.RPCPingTopicV1, n.pingRPCHandler)

	// TODO: check if Altair update
	n.subscribeRPC(p2p.RPCMetaDataTopicV2, n.metadataRPCHandler)

	go n.handleBDNBridge(ctx)

	return n, nil
}

func initNetwork(networkName string) error {
	init, ok := networkInitMapping[networkName]
	if !ok {
		return fmt.Errorf("network %v is not supported for beacon node", networkName)
	}

	// Influences on fork version and therefore on digest
	init()

	return nil
}

// BroadcastBlock broadcasts block to peers
func (n *Node) BroadcastBlock(block interfaces.SignedBeaconBlock) error {
	proto, err := block.Proto()
	if err != nil {
		return err
	}

	return n.p2p.Broadcast(n.ctx, proto)
}

// Start starts beacon node
func (n *Node) Start() error {
	n.log.Infof("Starting P2P beacon node: %v, peer ID: p2p/%v", n.p2p.Host().Addrs(), n.p2p.PeerID())

	// Attempt to reconnect if disconnected
	async.RunEvery(n.ctx, params.BeaconNetworkConfig().TtfbTimeout, func() {
		ensurePeerConnections(n.ctx, n.p2p.Host(), n.config.BeaconNodes()...)
	})
	// Update and send status
	async.RunEvery(n.ctx, time.Duration(time.Second*time.Duration(params.BeaconConfig().SecondsPerSlot)), func() {
		for _, pid := range n.p2p.Peers().Connected() {
			_, err := n.sendStatusRequest(n.ctx, pid)
			if err != nil {
				n.log.Warnf("could not send status: %v", err)
			}
		}
	})
	// Prune output peer list if disconnected to not receive "no good addresses"
	async.RunEvery(n.ctx, time.Minute, n.p2p.Peers().Prune)

	go n.p2p.Start()

	// We need wait a bit for p2p subscribe to event
	time.Sleep(time.Millisecond * 10)

	// Signalizing that we are done to synhronize(skipped)
	n.stateFeed.Send(&feed.Event{
		Type: statefeed.Initialized,
		Data: &statefeed.InitializedData{
			StartTime:             time.Unix(int64(n.genesisState.GenesisTime()), 0),
			GenesisValidatorsRoot: n.genesisState.GenesisValidatorsRoot(),
		},
	})

	if err := n.subscribe(p2p.BlockSubnetTopicFormat, n.blockSubscriber); err != nil {
		return err
	}

	// Required to be on top gossip score rating and not be disconnected by prysm

	// TODO: replace with broadcast when tested
	// broadcast should perform validation, for example check slot
	dontCare := func(msg *pubsub.Message) {}

	if err := n.subscribe(p2p.AggregateAndProofSubnetTopicFormat, dontCare); err != nil {
		return err
	}

	if err := n.subscribe(p2p.AggregateAndProofSubnetTopicFormat, dontCare); err != nil {
		return err
	}

	if err := n.subscribe(p2p.ProposerSlashingSubnetTopicFormat, dontCare); err != nil {
		return err
	}

	if err := n.subscribe(p2p.AttesterSlashingSubnetTopicFormat, dontCare); err != nil {
		return err
	}

	if err := n.subscribe(p2p.SyncContributionAndProofSubnetTopicFormat, dontCare); err != nil {
		return err
	}

	return nil
}

// Stop stops beacon node
func (n *Node) Stop() {
	n.cancel()
	n.p2p.Stop()
}

// StateFeed returns state feed
func (n *Node) StateFeed() *event.Feed {
	return n.stateFeed
}

func (n *Node) handleBDNBridge(ctx context.Context) {
	for {
		select {
		case bdnBlock := <-n.bridge.ReceiveBeaconBlockFromBDN():
			n.blockProcessor.ProcessBDNBlock(bdnBlock)
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) broadcast(msg *pubsub.Message) {
	topic := strings.TrimSuffix(*msg.Topic, n.p2p.Encoding().ProtocolSuffix())

	var err error
	topic, err = replaceForkDigest(topic)
	if err != nil {
		n.log.Errorf("could not replace topic: %v", err)
		return
	}

	base := p2p.GossipTopicMappings(topic, 0)
	if base == nil {
		n.log.Errorf("could not get basic")
		return
	}
	m, ok := proto.Clone(base).(ssz.Unmarshaler)
	if !ok {
		n.log.Errorf("message of %T does not support marshaller interface", base)
		return
	}

	if err := n.p2p.Encoding().DecodeGossip(msg.Data, m); err != nil {
		n.log.Errorf("could not encode gossip: %v", err)
		return
	}

	if err := n.p2p.Broadcast(n.ctx, m.(proto.Message)); err != nil {
		n.log.Errorf("could not broadcast: %v", err)
		return
	}
}

func (n *Node) digest() ([4]byte, error) {
	genesisTime := time.Unix(int64(n.genesisState.GenesisTime()), 0)
	genesisValidatorsRoot := n.genesisState.GenesisValidatorsRoot()

	return forks.CreateForkDigest(genesisTime, genesisValidatorsRoot)
}

func (n *Node) subscribe(topic string, f func(msg *pubsub.Message)) error {
	digest, err := n.digest()
	if err != nil {
		return fmt.Errorf("could not load digest: %v", err)
	}

	sub, err := n.p2p.SubscribeToTopic(fmt.Sprintf(topic, digest) + n.p2p.Encoding().ProtocolSuffix())
	if err != nil {
		return fmt.Errorf("could not subscribe to topic %v: %v", topic, err)
	}

	go func() {
		for {
			select {
			case <-n.ctx.Done():
			default:
				msg, err := sub.Next(n.ctx)
				if err != nil {
					n.log.Errorf("could not get message: %v", err)
					continue
				}

				if msg.ReceivedFrom == n.p2p.PeerID() {
					continue
				}

				// do not block subscription
				go f(msg)
			}
		}
	}()

	return nil
}

func (n *Node) blockSubscriber(msg *pubsub.Message) {
	if msg.ReceivedFrom == n.p2p.PeerID() {
		return
	}

	endpoint, err := n.loadNodeEndpointFromPeerID(msg.ReceivedFrom)
	if err != nil {
		if err == peerdata.ErrPeerUnknown {
			n.log.Debugf("skipping block, the peer ID %v that broadcasted the block is not trusted", msg.ReceivedFrom)
		} else {
			n.log.Errorf("could not load peer endpoint: %v", err)
		}
		return
	}

	log := n.log.WithField("remoteAddr", fmt.Sprintf("%v:%v", endpoint.IP, endpoint.Port))

	if msg.Data == nil {
		log.Errorf("msg is nil from peer: %v", msg.ReceivedFrom)
		return
	}

	fDigest, err := p2p.ExtractGossipDigest(*msg.Topic)
	if err != nil {
		log.Errorf("extraction failed for topic %v", err)
		return
	}

	blk, err := extractBlockDataType(fDigest[:], n.genesisState.GenesisValidatorsRoot())
	if err != nil {
		log.Errorf("could not extract block data type: %v", err)
		return
	}

	if err := n.p2p.Encoding().DecodeGossip(msg.Data, blk); err != nil {
		log.Errorf("could not decode block: %v", err)
		return
	}

	blockHash, err := blk.Block().HashTreeRoot()
	if err != nil {
		log.Errorf("could not get block[slot=%d] hash: %v", blk.Block().Slot(), err)
		return
	}
	blockHashHex := ethcommon.BytesToHash(blockHash[:]).String()

	execution, err := blk.Block().Body().Execution()
	if err != nil {
		log.Errorf("could not get block[slot=%d,hash=%s] execution: %v", blk.Block().Slot(), blockHashHex, err)
		return
	}

	// If it pre-merge state execution is empty
	if !n.beaconBlock && execution.BlockNumber() == 0 {
		log.Tracef("skip eth1 block[slot=%d,hash=%s] for pre-merge", blk.Block().Slot(), blockHashHex)
		return
	}

	if err := n.blockProcessor.ProcessBlockchainBlock(log, *endpoint, blk); err != nil {
		log.Errorf("could not process block[slot=%d,hash=%s]: %v", blk.Block().Slot(), blockHashHex, err)
		return
	}

	log.Debugf("eth2 p2p block[slot=%d,hash=%s] sent to BDN", blk.Block().Slot(), blockHashHex)
}

func extractBlockDataType(digest []byte, vRoot []byte) (interfaces.SignedBeaconBlock, error) {
	if len(digest) == 0 {
		bFunc, ok := p2ptypes.BlockMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
		if !ok {
			return nil, errors.New("no block type exists for the genesis fork version")
		}
		return bFunc()
	}
	if len(digest) != forkDigestLength {
		return nil, fmt.Errorf("invalid digest returned, wanted a length of %d but received %d", forkDigestLength, len(digest))
	}
	for k, blkFunc := range p2ptypes.BlockMap {
		rDigest, err := signing.ComputeForkDigest(k[:], vRoot[:])
		if err != nil {
			return nil, err
		}
		if rDigest == bytesutil.ToBytes4(digest) {
			return blkFunc()
		}
	}
	return nil, errors.New("no valid digest matched")
}

func (n *Node) loadNodeEndpointFromPeerID(peerID peer.ID) (*bxTypes.NodeEndpoint, error) {
	addr, err := n.p2p.Peers().Address(peerID)
	if err != nil {
		return nil, err
	}

	ip, port, err := parseMultiAddr(addr)
	if err != nil {
		return nil, err
	}

	return &bxTypes.NodeEndpoint{IP: ip, Port: port, PublicKey: peerID.String()}, nil
}

func (n *Node) registerP2p(privateKey, externalIP string) (*p2p.Service, error) {
	return p2p.NewService(n.ctx, &p2p.Config{
		NoDiscovery:       true,
		StaticPeers:       n.config.BeaconNodes(),
		BootstrapNodeAddr: nil,
		RelayNodeAddr:     "",
		DataDir:           "",
		LocalIP:           "",
		HostAddress:       externalIP,
		HostDNS:           "",
		PrivateKey:        privateKey,
		MetaDataDir:       "",
		TCPPort:           13000,
		UDPPort:           0,
		MaxPeers:          uint(len(n.config.BeaconNodes())),
		AllowListCIDR:     "",
		DenyListCIDR:      nil,
		EnableUPnP:        false,
		StateNotifier:     n,
		DB:                n.db,
	})
}

func (n *Node) updateLastStatus(status *pb.Status) {
	n.m.Lock()
	defer n.m.Unlock()

	// Use latest slot
	if n.lStatus != nil && status.HeadSlot <= n.lStatus.HeadSlot {
		return
	}

	n.lStatus = status
}

func (n *Node) lastStatus() *pb.Status {
	n.m.RLock()
	defer n.m.RUnlock()

	return n.lStatus
}

func (n *Node) status(ctx context.Context) (*pb.Status, error) {
	if status := n.lastStatus(); status != nil {
		return status, nil
	}

	digest, err := n.digest()
	if err != nil {
		return nil, err
	}

	stateRoot, err := n.genesisState.HashTreeRoot(ctx)
	if err != nil {
		return nil, err
	}
	genesisBlk := blocks.NewGenesisBlock(stateRoot[:])
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	return &pb.Status{
		ForkDigest:     digest[:],
		FinalizedRoot:  params.BeaconConfig().ZeroHash[:],
		FinalizedEpoch: 0,
		HeadRoot:       genesisBlkRoot[:],
		HeadSlot:       0,
	}, nil
}

func (n *Node) handshake(ctx context.Context, id peer.ID) error {
	n.log.WithField("peer", id).Debug("new peer")

	_, err := n.sendStatusRequest(ctx, id)
	if err != nil {
		return fmt.Errorf("could not send status: %v", err)
	}

	// Do not return an error for ping requests.
	if err := n.sendPingRequest(ctx, id); err != nil {
		return fmt.Errorf("could not ping peer: %v", err)
	}

	return nil
}

func (n *Node) goodBye(ctx context.Context, id peer.ID) error {
	n.log.WithField("peer", id).Debug("peer disconnected")

	return n.p2p.Disconnect(id)
}

// validates the peer's sequence number.
func (n *Node) validateSequenceNum(seq types.SSZUint64, id peer.ID) (bool, error) {
	md, err := n.p2p.Peers().Metadata(id)
	if err != nil {
		return false, err
	}
	if md == nil || md.IsNil() {
		return false, nil
	}
	// Return error on invalid sequence number.
	if md.SequenceNumber() > uint64(seq) {
		return false, p2ptypes.ErrInvalidSequenceNum
	}
	return md.SequenceNumber() == uint64(seq), nil
}
