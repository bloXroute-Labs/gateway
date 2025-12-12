package beacon

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	beaconParams "github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	prysmTypes "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ecdsaprysm "github.com/OffchainLabs/prysm/v7/crypto/ecdsa"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/libp2p/go-libp2p"
	mplex "github.com/libp2p/go-libp2p-mplex"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"
	fastssz "github.com/prysmaticlabs/fastssz"
	"google.golang.org/protobuf/proto"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

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
	gossipSubD   = 8 // topic stable mesh target count
	gossipSubDlo = 6 // topic stable mesh low watermark

	// gossip parameters
	gossipSubMcacheLen    = 6 // number of windows to retain full messages in cache for `IWANT` responses
	gossipSubMcacheGossip = 3 // number of windows to gossip about

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

var (
	errNoPeersFoundToBroadcast = errors.New("no peers found to broadcast")
)

var networkInitMapping = map[string]func(){
	// Mainnet is default and has required values
	"Mainnet": func() {},
	"Holesky": func() {
		cfg := beaconParams.HoleskyConfig().Copy()
		cfg.InitializeForkSchedule()
		beaconParams.OverrideBeaconConfig(cfg)
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

type topicSubscription struct {
	ctx          context.Context
	cancel       context.CancelFunc
	topic        *pubsub.Topic
	subscription *pubsub.Subscription
}

func (ts *topicSubscription) close() error {
	ts.subscription.Cancel() // cancel subscription
	err := ts.topic.Close()  // close topic
	ts.cancel()              // close subscription listener
	return err
}

// Node is beacon node
type Node struct {
	ctx   context.Context
	clock clock.Clock

	allForkDigests map[[4]byte]struct{}

	config      *network.EthConfig
	networkName string

	genesisState                state.BeaconState
	chain                       *Chain
	host                        host.Host
	pubSub                      *pubsub.PubSub
	bridge                      blockchain.Bridge
	staticPeers                 []libp2pPeer.AddrInfo
	peers                       peers
	trustedPeers                trustedPeers
	enableAnyIncomingConnection bool
	topicMap                    *syncmap.SyncMap[string, *topicSubscription]
	inboundLimit                int
	port                        int
	externalIP                  string
	enableQUIC                  bool

	encoding encoder.NetworkEncoding

	cancel func()

	log *log.Entry
}

// NodeParams are parameters for beacon node
type NodeParams struct {
	ParentContext        context.Context
	NetworkName          string
	EthConfig            *network.EthConfig
	TrustedPeersFilePath string
	GenesisFilePath      string
	Bridge               blockchain.Bridge
	Port                 int
	ExternalIP           string
	EnableQUIC           bool
	InboundLimit         int
}

// NewNode creates beacon node
func NewNode(params NodeParams) (*Node, error) {
	return newNode(params, &clock.RealClock{})
}

func newNode(params NodeParams, clock clock.Clock) (*Node, error) {
	logCtx := log.WithField("connType", "beacon")

	if err := initNetwork(params.NetworkName); err != nil {
		return nil, err
	}

	file, err := os.ReadFile(params.GenesisFilePath)
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

	genesisTimeUnix := genesisState.GenesisTime().Unix()
	if genesisTimeUnix < 0 {
		return nil, fmt.Errorf("genesis time is negative: %d", genesisTimeUnix)
	}
	if uint64(genesisTimeUnix) != params.EthConfig.GenesisTime {
		return nil, fmt.Errorf("inconsistent genesis time from genesis.ssz (%v) for network from --blockchain-network (%v)", genesisState.GenesisTime(), params.EthConfig.GenesisTime)
	}

	chain, err := NewChain(genesisState, uint64(beaconParams.BeaconConfig().SlotsPerEpoch))
	if err != nil {
		return nil, fmt.Errorf("could not create chain: %v", err)
	}

	staticPeers := make([]libp2pPeer.AddrInfo, 0, len(params.EthConfig.StaticPeers.BeaconNodes()))
	for _, multiaddr := range params.EthConfig.StaticPeers.BeaconNodes() {
		addrInfo, err := libp2pPeer.AddrInfoFromP2pAddr(*multiaddr)
		if err != nil {
			return nil, fmt.Errorf("could not convert multiaddr %v to addr info: %v", multiaddr, err)
		}

		staticPeers = append(staticPeers, *addrInfo)
	}

	ctx, cancel := context.WithCancel(params.ParentContext)

	n := &Node{
		ctx:                         ctx,
		clock:                       clock,
		config:                      params.EthConfig,
		networkName:                 params.NetworkName,
		genesisState:                genesisState,
		chain:                       chain,
		bridge:                      params.Bridge,
		staticPeers:                 staticPeers,
		peers:                       newPeers(),
		trustedPeers:                newTrustedPeers(),
		enableAnyIncomingConnection: params.TrustedPeersFilePath == "",
		topicMap:                    syncmap.NewStringMapOf[*topicSubscription](),
		inboundLimit:                params.InboundLimit,
		encoding:                    encoder.SszNetworkEncoder{},
		cancel:                      cancel,
		log:                         logCtx,
		port:                        params.Port,
		externalIP:                  params.ExternalIP,
		enableQUIC:                  params.EnableQUIC,
	}

	n.setAllForkDigests()

	if params.TrustedPeersFilePath != "" {
		n.reloadTrustedPeers(params.TrustedPeersFilePath)
	}

	opts, err := n.hostOptions(params.Port, params.EnableQUIC)
	if err != nil {
		return nil, fmt.Errorf("could not create host options: %v", err)
	}

	n.host, err = libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	psOpts := n.pubsubOptions()

	// TODO: put it into log file
	// if log.GetLevel() == log.TraceLevel {
	// 	logging.SetLogLevel("pubsub", "debug")
	// 	logging.SetLogLevel("p2p-config", "debug")
	// }

	n.pubSub, err = pubsub.NewGossipSub(ctx, n.host, psOpts...)
	if err != nil {
		return nil, err
	}

	n.host.Network().Notify(&libp2pNetwork.NotifyBundle{
		ConnectedF: func(net libp2pNetwork.Network, conn libp2pNetwork.Conn) {
			n.log.Tracef("peer %s connected", conn.RemoteMultiaddr())
			isInbound := conn.Stat().Direction == libp2pNetwork.DirInbound
			peer := n.peers.add(libp2pPeer.AddrInfo{
				ID:    conn.RemotePeer(),
				Addrs: []ma.Multiaddr{conn.RemoteMultiaddr()},
			}, isInbound)
			peerEndpoint := utils.MultiaddrToNodeEndpoint(conn.RemoteMultiaddr(), n.networkName)

			if isInbound {
				if peer.connect() {
					if err := n.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{
						PeerEndpoint: peerEndpoint,
						IsConnected:  true,
					}); err != nil {
						n.log.Errorf("could not send blockchain connection status: %v", err)
					}
				}
				return
			}

			// should be async
			go func() {
				if !peer.startHandshake() {
					n.log.Tracef("peer %v skipping handshake", peer)
					return
				}
				defer peer.finishHandshake()

				if err := n.handshake(peer, conn); err != nil {
					n.log.Infof("handshake with peer %v failed: %v", peer, err)

					if n.host.Network().Connectedness(conn.RemotePeer()) == libp2pNetwork.Connected {
						if err := n.host.Network().ClosePeer(conn.RemotePeer()); err != nil {
							n.log.Errorf("could not close peer %v: %v", peer, err)
						}
					}

					return
				}

				if peer.connect() {
					if err := n.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{
						PeerEndpoint: peerEndpoint,
						IsConnected:  true,
					}); err != nil {
						n.log.Errorf("could not send blockchain connection status: %v", err)
					}
				}

				n.log.Tracef("peer %v successed handshake", conn.RemotePeer())
			}()
		},
		DisconnectedF: func(net libp2pNetwork.Network, conn libp2pNetwork.Conn) {
			n.log.Tracef("peer %s disconnected", conn.RemoteMultiaddr())

			if peer := n.peers.get(conn.RemotePeer()); peer != nil && peer.disconnect() {
				if err := n.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{
					PeerEndpoint: utils.MultiaddrToNodeEndpoint(conn.RemoteMultiaddr(), n.networkName),
					IsConnected:  false,
				}); err != nil {
					n.log.Errorf("could not send blockchain connection status: %v", err)
				}
			}
		},
	})

	// todo: do we need to unsubscribe from these as well?
	n.subscribeRPC(p2p.RPCStatusTopicV2, n.statusRPCHandler)
	n.subscribeRPC(p2p.RPCGoodByeTopicV1, n.goodbyeRPCHandler)
	n.subscribeRPC(p2p.RPCPingTopicV1, n.pingRPCHandler)
	n.subscribeRPC(p2p.RPCMetaDataTopicV3, n.metadataRPCHandler)

	n.subscribeRPC(p2p.RPCBlobSidecarsByRangeTopicV1, func(stream libp2pNetwork.Stream) {
		defer n.closeStream(stream)

		SetRPCStreamDeadlines(stream)

		msg := new(ethpb.BlobSidecarsByRangeRequest)
		if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
			n.log.Errorf("could not decode blob by range request from peer %v: %v", stream.Conn().RemotePeer(), err)
			return
		}

		n.log.Debugf("Received blob by range request from peer %v: %v", stream.Conn().RemotePeer(), msg)
	})

	n.subscribeRPC(p2p.RPCBlobSidecarsByRootTopicV1, func(stream libp2pNetwork.Stream) {
		defer n.closeStream(stream)

		SetRPCStreamDeadlines(stream)

		msg := new(types.BlobSidecarsByRootReq)
		if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
			n.log.Errorf("could not decode blob by root request from peer %v: %v", stream.Conn().RemotePeer(), err)
			return
		}

		n.log.Debugf("Received blob by root request from peer %v: %v", stream.Conn().RemotePeer(), msg)
	})

	return n, nil
}

// Start starts beacon node
func (n *Node) Start() error {
	n.log.Infof("Starting P2P beacon node peer ID: p2p/%v", n.host.ID())

	internalAddresses, externalAddresses := n.serverAddresses()
	for _, addr := range internalAddresses {
		n.log.Infof("Node started p2p server: %s", addr)
	}
	for _, addr := range externalAddresses {
		n.log.Infof("Node started external p2p server: %s", addr)
	}

	go n.ensurePeerConnections()
	go n.sendStatusRequests()
	go n.bxStatusHandler()

	currentForkDigest, err := currentForkDigest(n.genesisState)
	if err != nil {
		return fmt.Errorf("could not get current fork digest: %v", err)
	}

	return n.subscribeAll(currentForkDigest)
}

func (n *Node) sendStatusRequests() {
	ticker := n.clock.Ticker(time.Second * time.Duration(beaconParams.BeaconConfig().SecondsPerSlot))

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.Alert():
			n.peers.rangeByID(func(peerID libp2pPeer.ID, peer *peer) bool {
				// skip non-connected
				c := n.host.Network().ConnsToPeer(peerID)
				if len(c) == 0 {
					return true
				}

				ctx, cancel := context.WithTimeout(n.ctx, beaconParams.BeaconConfig().RespTimeoutDuration())
				defer cancel()

				stream, err := n.host.NewStream(ctx, peerID, protocol.ID(p2p.RPCStatusTopicV2+n.encoding.ProtocolSuffix()))
				if err != nil {
					n.log.Errorf("could not create stream for status request: %v", err)
					return true
				}
				defer n.closeStream(stream)

				if _, err := n.encoding.EncodeWithMaxLength(stream, n.chain.Status(peer)); err != nil {
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

				msg := new(ethpb.StatusV2)
				if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
					n.log.Errorf("could not decode message: %v", err)
					return true
				}

				peer.setStatus(msg)

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

	if err := n.chain.AddBlock(block); err != nil {
		return fmt.Errorf("could not update status: %v", err)
	}

	digest, err := currentForkDigest(n.genesisState)
	if err != nil {
		return fmt.Errorf("could not get current fork digest: %v", err)
	}

	topic := fmt.Sprintf(p2p.BlockSubnetTopicFormat, digest)
	if err := n.broadcast(topic, msg); err != nil {
		return fmt.Errorf("could not broadcast block for topic %v: %v", topic, err)
	}

	return nil
}

func (n *Node) setAllForkDigests() {
	entries := beaconParams.SortedNetworkScheduleEntries()
	n.allForkDigests = make(map[[4]byte]struct{}, len(entries))
	for _, entry := range entries {
		n.allForkDigests[entry.ForkDigest] = struct{}{}
	}
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

// CanSubscribe returns true if the topic is of interest, and we can subscribe to it.
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

	var digest [4]byte
	dl, err := hex.Decode(digest[:], []byte(parts[2]))
	if err != nil {
		return false
	}
	if dl != 4 {
		return false
	}
	if _, ok := n.allForkDigests[digest]; !ok {
		return false
	}

	return parts[4] == encoder.ProtocolSuffixSSZSnappy
}

func (n *Node) unsubscribeAll(digest [4]byte) {
	n.topicMap.Range(func(k string, sub *topicSubscription) bool {
		// Skip if the topic does not contain the digest
		if !strings.Contains(k, fmt.Sprintf("%x", digest)) {
			return true
		}

		if err := sub.close(); err != nil {
			n.log.Warnf("could not close subscription, topic: %v: %v", k, err)
		} else {
			n.log.Infof("closed subscription, topic: %v", k)
		}

		return true
	})

	n.topicMap.Clear()
}

func dataColumnSubnetToTopic(subnet uint64, forkDigest [4]byte) string {
	return fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, forkDigest, subnet)
}

// BroadcastDataColumn broadcasts data column sidecar to peers
func (n *Node) BroadcastDataColumn(dataColumnSidecar *ethpb.DataColumnSidecar) error {
	digest, err := currentForkDigest(n.genesisState)
	if err != nil {
		return fmt.Errorf("could not get current fork digest: %v", err)
	}

	subnet := peerdas.ComputeSubnetForDataColumnSidecar(dataColumnSidecar.Index)

	topic := dataColumnSubnetToTopic(subnet, digest)
	err = n.broadcast(topic, dataColumnSidecar)
	if err == errNoPeersFoundToBroadcast {
		n.log.Tracef("no peers found to broadcast data column sidecar for topic %v", topic)
		return errNoPeersFoundToBroadcast
	}
	if err != nil {
		return fmt.Errorf("failed to broadcast blob: %v", err)
	}

	return err
}

func blobSubnetToTopic(subnet uint64, forkDigest [4]byte) string {
	return fmt.Sprintf(p2p.BlobSubnetTopicFormat, forkDigest, subnet)
}

func (n *Node) dataColumnSubscriber(msg *pubsub.Message) {
	dataColumnSidecar := new(ethpb.DataColumnSidecar)

	if err := n.encoding.DecodeGossip(msg.Data, dataColumnSidecar); err != nil {
		n.log.Errorf("could not decode data column: %v", err)
		return
	}

	bxSidecar, err := n.bridge.BeaconMessageToBDN(dataColumnSidecar)
	if err != nil {
		n.log.Errorf("could not convert beacon blob to BDN blob: %v", err)
		return
	}

	endpoint, err := n.loadNodeEndpointFromPeerID(msg.ReceivedFrom)
	if err != nil {
		n.log.Errorf("could not load peer endpoint: %v", err)

		return
	}

	if err := n.bridge.SendBeaconMessageToBDN(bxSidecar, *endpoint); err != nil {
		n.log.Errorf("could not send beacon message to BDN: %v", err)
		return
	}

	blockHash, err := dataColumnSidecar.SignedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		n.log.Errorf("could not get block hash: %v", err)
		return
	}

	n.log.Tracef("Received data column sidecar message from %v, index %v, block hash: %v, slot %v", msg.ReceivedFrom, dataColumnSidecar.Index, hex.EncodeToString(blockHash[:]), dataColumnSidecar.SignedBlockHeader.Header.Slot)
}

// BroadcastBlob broadcasts blob to peers
func (n *Node) BroadcastBlob(blobSidecar *ethpb.BlobSidecar) error {
	digest, err := currentForkDigest(n.genesisState)
	if err != nil {
		return fmt.Errorf("could not get current fork digest: %v", err)
	}

	err = n.broadcast(blobSubnetToTopic(blobSidecar.Index, digest), blobSidecar)
	if err != nil {
		return fmt.Errorf("failed to broadcast blob: %v", err)
	}

	return err
}

func (n *Node) blobSubscriber(msg *pubsub.Message) {
	blobSidecar := new(ethpb.BlobSidecar)

	if err := n.encoding.DecodeGossip(msg.Data, blobSidecar); err != nil {
		n.log.Errorf("could not decode blob: %v", err)
		return
	}

	bxSidecar, err := n.bridge.BeaconMessageToBDN(blobSidecar)
	if err != nil {
		n.log.Errorf("could not convert beacon blob to BDN blob: %v", err)
		return
	}

	endpoint, err := n.loadNodeEndpointFromPeerID(msg.ReceivedFrom)
	if err != nil {
		n.log.Errorf("could not load peer endpoint: %v", err)

		return
	}

	if err := n.bridge.SendBeaconMessageToBDN(bxSidecar, *endpoint); err != nil {
		n.log.Errorf("could not send beacon message to BDN: %v", err)
		return
	}

	blockHash, err := blobSidecar.SignedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		n.log.Errorf("could not get block hash: %v", err)
		return
	}

	n.log.Tracef("Received blob message from %v, index %v, block hash: %v, slot %v, kzg commitment: %v", msg.ReceivedFrom, blobSidecar.Index, hex.EncodeToString(blockHash[:]), blobSidecar.SignedBlockHeader.Header.Slot, hex.EncodeToString(blobSidecar.KzgCommitment))
}

func (n *Node) subscribeAll(digest [4]byte) error {
	// Required to be on top gossip score rating and not be disconnected by prysm
	dontCare := func(msg *pubsub.Message) {}

	n.log.Infof("Subscribing to all topics with digest %x", digest)

	if err := n.subscribe(fmt.Sprintf(p2p.BlockSubnetTopicFormat, digest), n.blockSubscriber); err != nil {
		return err
	}

	// Lighthouse penalizes for not publishing this topic if subscribed
	// if err := n.subscribe(digest, p2p.AggregateAndProofSubnetTopicFormat, dontCare); err != nil {
	// 	return err
	// }

	if err := n.subscribe(fmt.Sprintf(p2p.ProposerSlashingSubnetTopicFormat, digest), dontCare); err != nil {
		return err
	}

	if err := n.subscribe(fmt.Sprintf(p2p.AttesterSlashingSubnetTopicFormat, digest), dontCare); err != nil {
		return err
	}

	if err := n.subscribe(fmt.Sprintf(p2p.SyncContributionAndProofSubnetTopicFormat, digest), dontCare); err != nil {
		return err
	}

	for subnet := range beaconParams.BeaconConfig().DataColumnSidecarSubnetCount {
		if err := n.subscribe(dataColumnSubnetToTopic(subnet, digest), n.dataColumnSubscriber); err != nil {
			return err
		}
	}

	return nil
}

func (n *Node) subscribe(topic string, handler func(msg *pubsub.Message)) error {
	pbTopic, err := n.pubSub.Join(topic + n.encoding.ProtocolSuffix())
	if err != nil {
		return err
	}

	sub, err := pbTopic.Subscribe()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(n.ctx)
	n.topicMap.Store(topic, &topicSubscription{
		ctx:          ctx,
		cancel:       cancel,
		topic:        pbTopic,
		subscription: sub,
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-n.ctx.Done():
				return
			default:
				msg, err := sub.Next(n.ctx)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}

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

func (n *Node) replaceForkDigest(topic string) (string, error) {
	subStrings := strings.Split(topic, "/")
	if len(subStrings) != 4 {
		return "", p2p.ErrInvalidTopic
	}
	subStrings[2] = "%x"
	return strings.Join(subStrings, "/"), nil
}

func (n *Node) blockSubscriber(msg *pubsub.Message) {
	endpoint, err := n.loadNodeEndpointFromPeerID(msg.ReceivedFrom)
	if err != nil {
		n.log.Errorf("could not load peer endpoint: %v", err)

		return
	}

	var remoteAddr string
	if endpoint.DNS != "" {
		remoteAddr = fmt.Sprintf("%v:%v", endpoint.DNS, endpoint.Port)
	} else {
		remoteAddr = fmt.Sprintf("%v:%v", endpoint.IP, endpoint.Port)
	}

	logCtx := n.log.WithFields(log.Fields{
		"remoteAddr":   remoteAddr,
		"connProtocol": endpoint.ConnectionType,
	})

	if msg.Data == nil {
		logCtx.Errorf("msg is nil from peer: %v", msg.ReceivedFrom)
		return
	}

	fDigest, err := p2p.ExtractGossipDigest(*msg.Topic)
	if err != nil {
		logCtx.Errorf("extraction failed for topic %v", err)
		return
	}

	topic := strings.TrimSuffix(*msg.Topic, n.encoding.ProtocolSuffix())
	topic, err = n.replaceForkDigest(topic)
	if err != nil {
		n.log.Errorf("could not replace fork digest in topic %v: %v", *msg.Topic, err)

		return
	}

	block, err := extractValidDataTypeFromTopic(topic, fDigest[:])
	if err != nil {
		logCtx.Errorf("could not extract block data type: %v", err)
		return
	}
	blk, ok := block.(interfaces.ReadOnlySignedBeaconBlock)
	if !ok {
		logCtx.Errorf("extracted block is not of type ReadOnlySignedBeaconBlock")
		return
	}

	if err := n.encoding.DecodeGossip(msg.Data, blk); err != nil {
		logCtx.Errorf("could not decode block: %v", err)
		return
	}

	genesisTimeUnix := n.genesisState.GenesisTime().Unix()
	if genesisTimeUnix < 0 {
		logCtx.Errorf("genesis time is negative: %d", genesisTimeUnix)
		return
	}
	//nolint:gosec
	// G115: genesisTimeUnix is checked to be >= 0 above
	if blk.Block().Slot() <= currentSlot(uint64(genesisTimeUnix))-prysmTypes.Slot(n.config.IgnoreSlotCount) {
		logCtx.Errorf("block slot=%d is too old to process", blk.Block().Slot())
		return
	}

	wrappedBlock := NewWrappedReadOnlySignedBeaconBlock(blk)
	blockHash, err := wrappedBlock.HashTreeRoot()
	if err != nil {
		logCtx.Errorf("could not get block[slot=%d] hash: %v", blk.Block().Slot(), err)
		return
	}
	blockHashHex := ethcommon.BytesToHash(blockHash[:]).String()

	logCtx.Tracef("received beacon block [slot=%d, hash=%s]", blk.Block().Slot(), blockHashHex)

	if err := SendBlockToBDN(n.clock, n.log, wrappedBlock, n.bridge, *endpoint); err != nil {
		logCtx.Errorf("could not process block[slot=%d,hash=%s]: %v", blk.Block().Slot(), blockHashHex, err)
		return
	}

	if err := n.chain.AddBlock(blk); err != nil {
		logCtx.Errorf("could not update status: %v", err)
		return
	}
}

func (n *Node) loadNodeEndpointFromPeerID(peerID libp2pPeer.ID) (*bxTypes.NodeEndpoint, error) {
	conns := n.host.Network().ConnsToPeer(peerID)
	if len(conns) == 0 {
		return nil, fmt.Errorf("no connection to peer %v", peerID)
	}

	multiaddr := utils.MultiaddrToNodeEndpoint(conns[0].RemoteMultiaddr(), n.networkName)

	return &multiaddr, nil
}

func (n *Node) broadcast(topic string, msg proto.Message) error {
	pbTopic, ok := n.topicMap.Load(topic)
	if !ok {
		return errors.New("not started")
	}

	if len(pbTopic.topic.ListPeers()) == 0 {
		return errNoPeersFoundToBroadcast
	}

	castMsg, ok := msg.(fastssz.Marshaler)
	if !ok {
		return fmt.Errorf("message of %T does not support marshaller interface", msg)
	}

	buf := new(bytes.Buffer)
	if _, err := n.encoding.EncodeGossip(buf, castMsg); err != nil {
		return fmt.Errorf("could not encode gossip: %v", err)
	}

	return pbTopic.topic.Publish(n.ctx, buf.Bytes())
}

func (n *Node) ensurePeerConnections() {
	ticker := n.clock.Ticker(peerReconnectTimeout)

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.Alert():
			for _, peer := range n.staticPeers {
				c := n.host.Network().ConnsToPeer(peer.ID)
				if len(c) > 0 {
					continue
				}

				n.reconnectOrClose(peer)
			}
		}
	}
}

func (n *Node) reconnectOrClose(peer libp2pPeer.AddrInfo) {
	ctx, cancel := context.WithTimeout(n.ctx, peerConnectionTimeout)
	defer cancel()

	if err := n.host.Connect(ctx, peer); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		n.log.Warnf("could not connect peer %s: %v", peer, err)

		// Try to reconnect as fast as possible again
		// https://github.com/libp2p/go-libp2p/blob/ddfb6f9240679b840d3663021e8b4433f51379a7/examples/relay/main.go#L90
		n.host.Network().(*swarm.Swarm).Backoff().Clear(peer.ID)

		if err = n.host.Network().ClosePeer(peer.ID); err != nil {
			n.log.Errorf("could not close peer %s: %v", peer, err)
		}
	}
}

func (n *Node) handshake(peer *peer, conn libp2pNetwork.Conn) error {
	ctx, cancel := context.WithTimeout(n.ctx, beaconParams.BeaconConfig().RespTimeoutDuration())
	defer cancel()

	stream, err := n.host.NewStream(ctx, conn.RemotePeer(), protocol.ID(p2p.RPCStatusTopicV2+n.encoding.ProtocolSuffix()))
	if err != nil {
		return fmt.Errorf("could not create stream: %v", err)
	}
	defer n.closeStream(stream)

	if _, err := n.encoding.EncodeWithMaxLength(stream, n.chain.Status(peer)); err != nil {
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

	msg := new(ethpb.StatusV2)
	if err := n.encoding.DecodeWithMaxLength(stream, msg); err != nil {
		return fmt.Errorf("could not decode message: %v", err)
	}

	peer.setStatus(msg)

	return nil
}

func (n *Node) closeStream(stream libp2pNetwork.Stream) {
	if err := stream.Close(); err != nil && !strings.Contains(err.Error(), "close called for canceled stream") {
		n.log.Errorf("could not close peer %v stream for topic '%s': %v", stream.Conn().RemotePeer(), stream.Protocol(), err)
	}
}

func (n *Node) hostOptions(port int, enableQUIC bool) ([]libp2p.Option, error) {
	ifaceKey, err := ecdsaprysm.ConvertToInterfacePrivkey(n.config.PrivateKey)
	if err != nil {
		return nil, err
	}

	opts := []libp2p.Option{
		libp2p.UserAgent("Prysm/6.0.2/2ec1ef53dcb114da22698e8ccd9bc1e3aa8e3870"),
		libp2p.Identity(ifaceKey),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.DefaultMuxers,
	}

	if port != 0 {
		opts = append(opts,
			libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
			libp2p.ConnectionGater(n),
		)
		if enableQUIC {
			opts = append(opts,
				libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port)),
			)
		}
	}

	return opts, nil
}

func (n *Node) pubsubOptions() []pubsub.Option {
	psOpts := []pubsub.Option{
		pubsub.WithFloodPublish(true), // send blocks to all peers regardless connection number
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithNoAuthor(),
		pubsub.WithMessageIdFn(func(pmsg *pubsubpb.Message) string {
			return p2p.MsgID(n.genesisState.GenesisValidatorsRoot(), pmsg)
		}),
		pubsub.WithSubscriptionFilter(n),
		pubsub.WithPeerOutboundQueueSize(pubsubQueueSize),
		pubsub.WithMaxMessageSize(int(beaconParams.BeaconConfig().MaxPayloadSize)), //nolint
		pubsub.WithValidateQueueSize(pubsubQueueSize),
		pubsub.WithGossipSubParams(pubsubGossipParam()),
	}
	return psOpts
}

func (n *Node) bxStatusHandler() {
	statusBridge := n.bridge.SubscribeStatus(true)

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-statusBridge.ReceiveBlockchainStatusRequest():
			endpoints := make([]*bxTypes.NodeEndpoint, 0)
			n.peers.rangeByID(func(peerID libp2pPeer.ID, peer *peer) bool {
				c := n.host.Network().ConnsToPeer(peerID)
				if len(c) == 0 {
					return true
				}

				endpoint := utils.MultiaddrToNodeEndpoint(peer.addrInfo.Load().Addrs[0], n.networkName)
				endpoint.ConnectedAt = peer.connectedAt().Format(time.RFC3339)
				endpoint.ID = peerID.String()
				endpoint.Dynamic = peer.isDynamic.Load()
				endpoints = append(endpoints, &endpoint)

				return true
			})

			internalAddresses, externalAddresses := n.serverAddresses()

			status := blockchain.BxStatus{
				Endpoints:       endpoints,
				ServerAddresses: append(internalAddresses, externalAddresses...),
			}

			if err := statusBridge.SendBlockchainStatusResponse(status); err != nil {
				n.log.Errorf("could not send blockchain status response: %v", err)
			}
		case <-statusBridge.ReceiveTrustedPeerRequest():
			if err := statusBridge.SendTrustedPeerResponse(n.trustedPeers.list()); err != nil {
				n.log.Errorf("could not send trusted peer response: %v", err)
			}
		}
	}
}

func (n *Node) serverAddresses() (internalAddresses, externalAddresses []string) {
	hostID := n.host.ID().String()

	internalAddresses = n.internalNetworkAddrs()

	externalAddresses = append(externalAddresses, fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", n.externalIP, n.port, hostID))
	if n.enableQUIC {
		externalAddresses = append(externalAddresses, fmt.Sprintf("/ip4/%s/udp/%d/quic-v1/p2p/%s", n.externalIP, n.port, hostID))
	}

	return
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

func (n *Node) internalNetworkAddrs() []string {
	hostID := n.host.ID().String()
	addrs := n.host.Network().ListenAddresses()
	res := make([]string, 0, len(addrs))

	for i := range addrs {
		if !(strings.Contains(addrs[i].String(), "/ip4/") || strings.Contains(addrs[i].String(), "/ip6/")) {
			continue
		}

		res = append(res, fmt.Sprintf("%s/p2p/%s", addrs[i].String(), hostID))
	}

	return res
}
