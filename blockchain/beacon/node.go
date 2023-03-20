package beacon

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/pkg/errors"
	fastssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/blocks"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/encoder"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v3/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v3/crypto/ecdsa"
	"github.com/prysmaticlabs/prysm/v3/network/forks"
	ethpb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

var errPeerUnknown = errors.New("peer is unknown")

const (
	forkDigestLength = 4

	responseCodeSuccess     = byte(0x00)
	responseCodeServerError = byte(0x02)

	peerReconnectTimeout  = 1 * time.Second
	peerConnectionTimeout = 5 * time.Second
)

// libp2p settings
const (
	// overlay parameters
	gossipSubD   = 8  // topic stable mesh target count
	gossipSubDlo = 6  // topic stable mesh low watermark
	gossipSubDhi = 12 // topic stable mesh high watermark

	// gossip parameters
	gossipSubMcacheLen    = 6   // number of windows to retain full messages in cache for `IWANT` responses
	gossipSubMcacheGossip = 3   // number of windows to gossip about
	gossipSubSeenTTL      = 550 // number of heartbeat intervals to retain message IDs

	// heartbeat interval
	gossipSubHeartbeatInterval = 700 * time.Millisecond // frequency of heartbeat, milliseconds

	// It is set at this limit to handle the possibility
	// of double topic subscriptions at fork boundaries.
	// -> 64 Attestation Subnets * 2.
	// -> 4 Sync Committee Subnets * 2.
	// -> Block,Aggregate,ProposerSlashing,AttesterSlashing,Exits,SyncContribution * 2.
	pubsubSubscriptionRequestLimit = 200

	// pubsubQueueSize is the size that we assign to our validation queue and outbound message queue for
	// gossipsub.
	pubsubQueueSize = 600
)

var networkInitMapping = map[string]func(){
	// Mainnet is default and has required values
	"Mainnet": func() {},
	"Prater": func() {
		params.UsePraterNetworkConfig()
		params.SetActive(params.PraterConfig())
	},
	"Zhejiang": func() {
		cfg := params.MainnetConfig().Copy()
		cfg.MinGenesisTime = 1675263480
		cfg.GenesisDelay = 120
		cfg.ConfigName = "Zhejiang"
		cfg.GenesisForkVersion = []byte{0x00, 0x00, 0x00, 0x69}
		cfg.SecondsPerETH1Block = 14
		cfg.DepositChainID = 1337803
		cfg.DepositNetworkID = 1337803
		cfg.AltairForkEpoch = 0
		cfg.AltairForkVersion = []byte{0x0, 0x0, 0x00, 0x70}
		cfg.BellatrixForkEpoch = 0
		cfg.BellatrixForkVersion = []byte{0x0, 0x0, 0x00, 0x71}
		cfg.CapellaForkEpoch = 1350
		cfg.CapellaForkVersion = []byte{0x0, 0x0, 0x00, 0x72}
		cfg.TerminalTotalDifficulty = "0"
		cfg.DepositContractAddress = "0x4242424242424242424242424242424242424242"
		cfg.InitializeForkSchedule()
		params.SetActive(cfg)
		types.InitializeDataMaps()
	},
	"Goerli": func() {
		cfg := params.MainnetConfig().Copy()
		cfg.MinGenesisTime = 1614588812
		cfg.GenesisDelay = 1919188
		cfg.ConfigName = "Goerli"
		cfg.GenesisForkVersion = []byte{0x00, 0x00, 0x10, 0x20}
		cfg.SecondsPerETH1Block = 14
		cfg.DepositChainID = 5
		cfg.DepositNetworkID = 5
		cfg.AltairForkEpoch = 0
		cfg.AltairForkVersion = []byte{0x0, 0x0, 0x10, 0x20}
		cfg.BellatrixForkEpoch = 112260
		cfg.BellatrixForkVersion = []byte{0x02, 0x0, 0x10, 0x20}
		cfg.CapellaForkEpoch = 162304
		cfg.CapellaForkVersion = []byte{0x03, 0x0, 0x10, 0x20}
		cfg.TerminalTotalDifficulty = "10790000"
		cfg.DepositContractAddress = "0xff50ed3d0ec03aC01D4C79aAd74928BFF48a7b2b"
		cfg.InitializeForkSchedule()
		params.SetActive(cfg)
		types.InitializeDataMaps()
	},
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

// Node is beacon node
type Node struct {
	ctx   context.Context
	clock utils.Clock

	config      *network.EthConfig
	networkName string

	genesisState state.BeaconState
	host         host.Host
	pubSub       *pubsub.PubSub

	bridge         blockchain.Bridge
	blockProcessor *blockProcessor

	peers    peers
	topicMap sync.Map

	encoding encoder.NetworkEncoding

	status status

	cancel func()

	log *log.Entry
}

// NewNode creates beacon node
func NewNode(parent context.Context, networkName string, config *network.EthConfig, chain *Chain, genesisFilePath string, bridge blockchain.Bridge) (*Node, error) {
	return newNode(parent, networkName, config, chain, genesisFilePath, bridge, &utils.RealClock{})
}

func newNode(parent context.Context, networkName string, config *network.EthConfig, chain *Chain, genesisFilePath string, bridge blockchain.Bridge, clock utils.Clock) (*Node, error) {
	logCtx := log.WithField("connType", "beacon")

	if err := initNetwork(networkName); err != nil {
		return nil, err
	}

	encoder.SetMaxGossipSizeForBellatrix()
	encoder.SetMaxChunkSizeForBellatrix()

	file, err := os.ReadFile(genesisFilePath)
	if err != nil {
		return nil, err
	}
	st := &ethpb.BeaconState{}
	if err := st.UnmarshalSSZ(file); err != nil {
		return nil, err
	}
	genesisState, err := state_native.InitializeFromProtoPhase0(st)
	if err != nil {
		return nil, err
	}

	if genesisState.GenesisTime() != config.GenesisTime {
		return nil, fmt.Errorf("inconsistent genesis time from genesis.ssz (%v) for network from --blockchain-network (%v)", genesisState.GenesisTime(), config.GenesisTime)
	}

	ifaceKey, err := ecdsaprysm.ConvertToInterfacePrivkey(config.PrivateKey)
	if err != nil {
		return nil, err
	}
	host, err := libp2p.New(
		libp2p.UserAgent("Prysm/3.1.2/648ab9f2c249f1d06d0aad4328e8df429ceaf66c"),
		libp2p.Identity(ifaceKey),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.DefaultMuxers,
	)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(parent)

	n := &Node{
		ctx:          ctx,
		clock:        clock,
		config:       config,
		networkName:  networkName,
		genesisState: genesisState,
		host:         host,
		bridge:       bridge,
		peers:        newPeers(),
		encoding:     encoder.SszNetworkEncoder{},
		cancel:       cancel,
		log:          logCtx,
	}

	if err := n.addPeers(); err != nil {
		return nil, fmt.Errorf("could not add peers %v", err)
	}

	n.blockProcessor = newBlockProcessor(ctx, config, chain, bridge, n.BroadcastBlock, n.log)

	psOpts := n.pubsubOptions()
	n.pubSub, err = pubsub.NewGossipSub(ctx, host, psOpts...)
	if err != nil {
		return nil, err
	}

	n.host.Network().Notify(&libp2pNetwork.NotifyBundle{
		ConnectedF: func(net libp2pNetwork.Network, conn libp2pNetwork.Conn) {
			// should be async
			go func() {
				peer := n.peers.get(conn.RemotePeer())

				// Incoming connection
				if peer == nil {
					addrInfo, err := libp2pPeer.AddrInfoFromP2pAddr(conn.RemoteMultiaddr())
					if err != nil {
						n.log.Errorf("could not convert multiaddr %v for incoming connection: %v", conn.RemoteMultiaddr(), err)
						return
					}

					peer = n.peers.add(addrInfo, conn.RemoteMultiaddr())
				}

				if peer.handshaking() {
					n.log.Tracef("peer %v skipping handshake", peer)
					return
				}
				defer peer.finishedHandshaking()

				if err := n.handshake(conn); err != nil {
					n.log.Infof("handshake with peer %v failed: %v", peer, err)

					if err := n.host.Network().ClosePeer(conn.RemotePeer()); err != nil {
						n.log.Errorf("could not close peer %v: %v", peer, err)
					}

					return
				}

				n.log.Tracef("peer %v successed handshake", conn.RemotePeer())
			}()
		},
		DisconnectedF: func(net libp2pNetwork.Network, conn libp2pNetwork.Conn) {
			peer := n.peers.get(conn.RemotePeer())

			if err := n.host.Network().ClosePeer(conn.RemotePeer()); err != nil {
				n.log.Errorf("could not close peer %v: %v", peer, err)
			}

			n.log.Tracef("peer %v disconnected", peer)
		},
	})

	n.subscribeRPC(p2p.RPCStatusTopicV1, n.statusRPCHandler)
	n.subscribeRPC(p2p.RPCGoodByeTopicV1, n.goodbyeRPCHandler)
	n.subscribeRPC(p2p.RPCPingTopicV1, n.pingRPCHandler)

	// TODO: check if Altair update
	n.subscribeRPC(p2p.RPCMetaDataTopicV2, n.metadataRPCHandler)

	return n, nil
}

// Start starts beacon node
func (n *Node) Start() error {
	n.log.Infof("Starting P2P beacon node peer ID: p2p/%v", n.host.ID())

	go n.ensurePeerConnections()
	go n.handleBDNBridge()
	go n.sendStatusRequests()

	if err := n.subscribe(p2p.BlockSubnetTopicFormat, n.blockSubscriber); err != nil {
		return err
	}

	// Required to be on top gossip score rating and not be disconnected by prysm
	dontCare := func(msg *pubsub.Message) {}

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

func (n *Node) sendStatusRequests() {
	ticker := n.clock.Ticker(time.Second * time.Duration(params.BeaconConfig().SecondsPerSlot))

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.Alert():
			status, err := n.getStatus()
			if err != nil {
				n.log.Errorf("could not get status for request: %v", err)
				continue
			}

			n.peers.rangeByID(func(peerID libp2pPeer.ID, _ *peer) bool {
				// Skip non connected
				c := n.host.Network().ConnsToPeer(peerID)
				if len(c) == 0 {
					return true
				}

				ctx, cancel := context.WithTimeout(n.ctx, params.BeaconNetworkConfig().RespTimeout)
				defer cancel()

				stream, err := n.host.NewStream(ctx, libp2pPeer.ID(peerID), protocol.ID(p2p.RPCStatusTopicV1+n.encoding.ProtocolSuffix()))
				if err != nil {
					n.log.Errorf("could not create stream for status request: %v", err)
					return true
				}
				defer n.closeStream(stream)

				if _, err := n.encoding.EncodeWithMaxLength(stream, status); err != nil {
					n.log.Errorf("could not send status: %v", err)
					return true
				}

				b := make([]byte, 1)
				_, err = stream.Read(b)
				if err != nil {
					n.log.Errorf("could not read status response: %v", err)
					return true
				}

				if b[0] != responseCodeSuccess {
					n.log.Errorf("unexpected status code: %v", b[0])
					return true
				}

				msg := new(ethpb.Status)
				if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
					n.log.Errorf("could not decode message: %v", err)
					return true
				}

				n.updateStatus(msg)

				return true
			})
		}
	}
}

// Stop stops beacon node
func (n *Node) Stop() {
	n.cancel()
}

// BroadcastBlock broadcasts block to peers
func (n *Node) BroadcastBlock(block interfaces.ReadOnlySignedBeaconBlock) error {
	msg, err := block.Proto()
	if err != nil {
		return err
	}

	return n.broadcast(p2p.BlockSubnetTopicFormat, msg)
}

// FilterIncomingSubscriptions is invoked for all RPCs containing subscription notifications.
// This method returns only the topics of interest and may return an error if the subscription
// request contains too many topics.
func (n *Node) FilterIncomingSubscriptions(_ libp2pPeer.ID, subs []*pubsubpb.RPC_SubOpts) ([]*pubsubpb.RPC_SubOpts, error) {
	if len(subs) > pubsubSubscriptionRequestLimit {
		return nil, pubsub.ErrTooManySubscriptions
	}

	return pubsub.FilterSubscriptions(subs, n.CanSubscribe), nil
}

// CanSubscribe returns true if the topic is of interest and we could subscribe to it.
func (n *Node) CanSubscribe(topic string) bool {
	parts := strings.Split(topic, "/")
	if len(parts) != 5 {
		return false
	}
	// The topic must start with a slash, which means the first part will be empty.
	if parts[0] != "" {
		return false
	}
	if parts[1] != "eth2" {
		return false
	}
	phase0ForkDigest, err := n.currentForkDigest()
	if err != nil {
		n.log.Errorf("Could not determine fork digest: %v", err)
		return false
	}
	altairForkDigest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().AltairForkEpoch, n.genesisState.GenesisValidatorsRoot())
	if err != nil {
		n.log.Errorf("Could not determine altair fork digest: %v", err)
		return false
	}
	bellatrixForkDigest, err := forks.ForkDigestFromEpoch(params.BeaconConfig().BellatrixForkEpoch, n.genesisState.GenesisValidatorsRoot())
	if err != nil {
		n.log.Errorf("Could not determine Bellatrix fork digest: %v", err)
		return false
	}

	switch parts[2] {
	case fmt.Sprintf("%x", phase0ForkDigest):
	case fmt.Sprintf("%x", altairForkDigest):
	case fmt.Sprintf("%x", bellatrixForkDigest):
	default:
		return false
	}

	return parts[4] == encoder.ProtocolSuffixSSZSnappy
}

func (n *Node) subscribe(topic string, handler func(msg *pubsub.Message)) error {
	digest, err := n.currentForkDigest()
	if err != nil {
		return err
	}

	pbTopic, err := n.pubSub.Join(fmt.Sprintf(topic+n.encoding.ProtocolSuffix(), digest))
	if err != nil {
		return err
	}

	n.topicMap.Store(topic, pbTopic)

	sub, err := pbTopic.Subscribe()
	if err != nil {
		return err
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

				// do not receive message from yourself
				if msg.ReceivedFrom == n.host.ID() {
					continue
				}

				// do not block subscription
				go handler(msg)
			}
		}
	}()

	return nil
}

func (n *Node) blockSubscriber(msg *pubsub.Message) {
	endpoint, err := n.loadNodeEndpointFromPeerID(msg.ReceivedFrom)
	if err != nil {
		if err == errPeerUnknown {
			n.log.Debugf("skipping block, the peer ID %v that broadcasted the block is not trusted", msg.ReceivedFrom)
		} else {
			n.log.Errorf("could not load peer endpoint: %v", err)
		}
		return
	}

	logCtx := n.log.WithField("remoteAddr", fmt.Sprintf("%v:%v", endpoint.IP, endpoint.Port))

	if msg.Data == nil {
		logCtx.Errorf("msg is nil from peer: %v", msg.ReceivedFrom)
		return
	}

	fDigest, err := p2p.ExtractGossipDigest(*msg.Topic)
	if err != nil {
		logCtx.Errorf("extraction failed for topic %v", err)
		return
	}

	blk, err := extractBlockDataType(fDigest[:], n.genesisState.GenesisValidatorsRoot())
	if err != nil {
		logCtx.Errorf("could not extract block data type: %v", err)
		return
	}

	if err := n.encoding.DecodeGossip(msg.Data, blk); err != nil {
		logCtx.Errorf("could not decode block: %v", err)
		return
	}

	blockHash, err := blk.Block().HashTreeRoot()
	if err != nil {
		logCtx.Errorf("could not get block[slot=%d] hash: %v", blk.Block().Slot(), err)
		return
	}
	blockHashHex := ethcommon.BytesToHash(blockHash[:]).String()

	execution, err := blk.Block().Body().Execution()
	if err != nil {
		logCtx.Errorf("could not get block[slot=%d,hash=%s] execution: %v", blk.Block().Slot(), blockHashHex, err)
		return
	}

	// If it pre-merge state execution is empty
	if execution.BlockNumber() == 0 {
		logCtx.Tracef("skip eth1 block[slot=%d,hash=%s] for pre-merge", blk.Block().Slot(), blockHashHex)
		return
	}

	if err := n.blockProcessor.ProcessBlockchainBlock(logCtx, *endpoint, blk); err != nil {
		logCtx.Errorf("could not process block[slot=%d,hash=%s]: %v", blk.Block().Slot(), blockHashHex, err)
		return
	}

	logCtx.Debugf("eth2 p2p block[slot=%d,hash=%s] sent to BDN", blk.Block().Slot(), blockHashHex)
}

func (n *Node) loadNodeEndpointFromPeerID(peerID libp2pPeer.ID) (*bxTypes.NodeEndpoint, error) {
	addr := n.peers.get(peerID)
	if addr == nil {
		return nil, errPeerUnknown
	}

	multiaddr := utils.MultiaddrToNodeEndoint(addr.remoteAddr, n.networkName)
	return &multiaddr, nil
}

func (n *Node) broadcast(topic string, msg proto.Message) error {
	t, ok := n.topicMap.Load(topic)
	if !ok {
		return errors.New("not started")
	}

	pbTopic := t.(*pubsub.Topic)

	if len(pbTopic.ListPeers()) == 0 {
		n.log.Warnf("no peers to broadcast")
		return nil
	}

	castMsg, ok := msg.(fastssz.Marshaler)
	if !ok {
		return errors.Errorf("message of %T does not support marshaller interface", msg)
	}

	buf := new(bytes.Buffer)
	if _, err := n.encoding.EncodeGossip(buf, castMsg); err != nil {
		return fmt.Errorf("could not encode gossip: %v", err)
	}

	return pbTopic.Publish(n.ctx, buf.Bytes())
}

func (n *Node) handleBDNBridge() {
	for {
		select {
		case bdnBlock := <-n.bridge.ReceiveBeaconBlockFromBDN():
			n.blockProcessor.ProcessBDNBlock(bdnBlock)
		case <-n.ctx.Done():
			return
		}
	}
}

func (n *Node) addPeers() error {
	for _, multiaddr := range n.config.BeaconNodes() {
		addrInfo, err := libp2pPeer.AddrInfoFromP2pAddr(*multiaddr)
		if err != nil {
			return fmt.Errorf("could not convert multiaddr %v to addr info: %v", multiaddr, err)
		}

		n.peers.add(addrInfo, *multiaddr)
	}

	return nil
}

func (n *Node) ensurePeerConnections() {
	ticker := n.clock.Ticker(peerReconnectTimeout)

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.Alert():
			n.peers.rangeByID(func(peerID libp2pPeer.ID, peer *peer) bool {
				c := n.host.Network().ConnsToPeer(peerID)
				if len(c) > 0 {
					return true
				}

				ctx, cancel := context.WithTimeout(n.ctx, peerConnectionTimeout)
				defer cancel()

				if err := n.host.Connect(ctx, *peer.addrInfo); err != nil {
					// Try to reconnect as fast as possible again
					// https://github.com/libp2p/go-libp2p/blob/ddfb6f9240679b840d3663021e8b4433f51379a7/examples/relay/main.go#L90
					n.host.Network().(*swarm.Swarm).Backoff().Clear(peerID)

					if err := n.host.Network().ClosePeer(peerID); err != nil {
						n.log.Errorf("could not close peer %v: %v", peer.remoteAddr.String(), err)
					}

					n.log.Warnf("could not connect peer %v: %v", peer.remoteAddr.String(), err)
				}

				return true
			})
		}
	}
}

func (n *Node) handshake(conn libp2pNetwork.Conn) error {
	ctx, cancel := context.WithTimeout(n.ctx, params.BeaconNetworkConfig().RespTimeout)
	defer cancel()

	stream, err := n.host.NewStream(ctx, conn.RemotePeer(), protocol.ID(p2p.RPCStatusTopicV1+n.encoding.ProtocolSuffix()))
	if err != nil {
		return fmt.Errorf("could not create stream: %v", err)
	}
	defer n.closeStream(stream)

	status, err := n.getStatus()
	if err != nil {
		return fmt.Errorf("could not get status: %v", err)
	}

	if _, err := n.encoding.EncodeWithMaxLength(stream, status); err != nil {
		return fmt.Errorf("could not send status: %v", err)
	}

	b := make([]byte, 1)
	_, err = stream.Read(b)
	if err != nil {
		return fmt.Errorf("could not read status response: %v", err)
	}

	if b[0] != responseCodeSuccess {
		return fmt.Errorf("unexpected status code: %v", b[0])
	}

	msg := new(ethpb.Status)
	if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
		return fmt.Errorf("could not decode message: %v", err)
	}

	n.updateStatus(msg)

	return nil
}

type status struct {
	status *ethpb.Status

	mu sync.RWMutex
}

func (s *status) load() *ethpb.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

func (s *status) store(status *ethpb.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = status
}

func (n *Node) updateStatus(status *ethpb.Status) {
	// Use latest slot
	oldStatus := n.status.load()
	if oldStatus != nil && status.HeadSlot <= oldStatus.HeadSlot {
		return
	}

	n.status.store(status)
}

func (n *Node) getStatus() (*ethpb.Status, error) {
	if status := n.status.load(); status != nil {
		return status, nil
	}

	digest, err := n.currentForkDigest()
	if err != nil {
		return nil, err
	}

	stateRoot, err := n.genesisState.HashTreeRoot(n.ctx)
	if err != nil {
		return nil, err
	}
	genesisBlk := blocks.NewGenesisBlock(stateRoot[:])
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	// return empty status if nobody have gave us yet
	return &ethpb.Status{
		ForkDigest:     digest[:],
		FinalizedRoot:  params.BeaconConfig().ZeroHash[:],
		FinalizedEpoch: 0,
		HeadRoot:       genesisBlkRoot[:],
		HeadSlot:       0,
	}, nil
}

func (n *Node) currentForkDigest() ([4]byte, error) {
	genesisTime := time.Unix(int64(n.genesisState.GenesisTime()), 0)
	genesisValidatorsRoot := n.genesisState.GenesisValidatorsRoot()

	return forks.CreateForkDigest(genesisTime, genesisValidatorsRoot)
}

func (n *Node) closeStream(stream libp2pNetwork.Stream) {
	if err := stream.Close(); err != nil {
		n.log.Errorf("could not close peer %v stream: %v", stream.Conn().RemotePeer(), err)
	}
}

func (n *Node) pubsubOptions() []pubsub.Option {
	psOpts := []pubsub.Option{
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(),
		pubsub.WithMessageIdFn(func(pmsg *pubsubpb.Message) string {
			return p2p.MsgID(n.genesisState.GenesisValidatorsRoot(), pmsg)
		}),
		pubsub.WithSubscriptionFilter(n),
		pubsub.WithPeerOutboundQueueSize(pubsubQueueSize),
		pubsub.WithMaxMessageSize(int(params.BeaconNetworkConfig().GossipMaxSizeBellatrix)),
		pubsub.WithValidateQueueSize(pubsubQueueSize),
		pubsub.WithGossipSubParams(pubsubGossipParam()),
	}
	return psOpts
}

// creates a custom gossipsub parameter set.
func pubsubGossipParam() pubsub.GossipSubParams {
	gParams := pubsub.DefaultGossipSubParams()
	gParams.Dlo = gossipSubDlo
	gParams.D = gossipSubD
	gParams.HeartbeatInterval = gossipSubHeartbeatInterval
	gParams.HistoryLength = gossipSubMcacheLen
	gParams.HistoryGossip = gossipSubMcacheGossip
	return gParams
}
