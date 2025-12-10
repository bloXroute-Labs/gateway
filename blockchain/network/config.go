package network

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	ecdsaprysm "github.com/OffchainLabs/prysm/v7/crypto/ecdsa"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// PeerInfo contains the enode and websockets endpoint for an Ethereum peer
type PeerInfo struct {
	Enode        *enode.Node
	Multiaddr    *multiaddr.Multiaddr
	EthWSURI     string
	PrysmAddr    string
	BeaconAPIURI string
	Endpoint     types.NodeEndpoint
}

// EthConfig represents Ethereum network configuration settings (e.g. indicate Mainnet, Rinkeby, BSC, etc.). Most of this information will be exchanged in the status messages.
type EthConfig struct {
	StaticPeers    StaticPeers
	BootstrapNodes []*enode.Node
	PrivateKey     *ecdsa.PrivateKey

	ProgramName             string
	Network                 uint64
	TotalDifficulty         *big.Int
	TerminalTotalDifficulty *big.Int
	GenesisTime             uint64
	TTDOverrides            bool
	Head                    common.Hash
	Genesis                 common.Hash
	ExecutionLayerForks     []string
	BlockConfirmationsCount int
	SendBlockConfirmation   bool

	IgnoreBlockTimeout time.Duration
	IgnoreSlotCount    int
}

const (
	privateKeyLen          = 64
	invalidMultiNodeErrMsg = "unable to parse --multi-node argument node number %d. Expected format: enode[+eth-ws-uri],enr[+prysm://prysm-host:prysm-port],multiaddr[+prysm://prysm-host:prysm-port],beacon-api://ip:port"
)

// NewPresetEthConfigFromCLI builds a new EthConfig from the command line context. Selects a specific network configuration based on the provided startup flag.
func NewPresetEthConfigFromCLI(ctx *cli.Context, dataDir string) (*EthConfig, string, error) {
	blockchainNetwork := ctx.String(utils.BlockchainNetworkFlag.Name)

	preset, err := NewEthereumPreset(blockchainNetwork)
	if err != nil {
		return nil, "", err
	}
	preset.StaticPeers = make([]PeerInfo, 0)

	if ctx.IsSet(utils.MultiNode.Name) {
		switch {
		case ctx.IsSet(utils.EnodesFlag.Name), ctx.IsSet(utils.BeaconENRFlag.Name), ctx.IsSet(utils.BeaconMultiaddrFlag.Name), ctx.IsSet(utils.PrysmGRPCFlag.Name), ctx.IsSet(utils.EthWSUriFlag.Name), ctx.IsSet(utils.BeaconAPIUriFlag.Name):
			return nil, "", errors.New("parameters --multi-node and (--enodes, --enr, --multiaddr, --prysm-grpc-uri, --eth-ws-uri or --beacon-api-uri) should not be used simultaneously; if you would like to use multiple connections, use --multi-node")
		}

		if err := preset.parseMultiNode(ctx.String(utils.MultiNode.Name), ctx.String(utils.BlockchainNetworkFlag.Name)); err != nil {
			return nil, "", fmt.Errorf("unable to parse --multi-node argument: %v", err)
		}
	} else {

		if ctx.IsSet(utils.EnodesFlag.Name) {
			peer := PeerInfo{}

			enodeString := ctx.String(utils.EnodesFlag.Name)
			node, err := enode.Parse(enode.ValidSchemes, enodeString)
			if err != nil {
				return nil, "", err
			}
			peer.Enode = node
			peer.Endpoint = utils.EnodeToNodeEndpoint(node, blockchainNetwork)

			if ctx.IsSet(utils.EthWSUriFlag.Name) {
				ethWSURI := ctx.String(utils.EthWSUriFlag.Name)
				err = validateWSURI(ethWSURI)
				if err != nil {
					return nil, "", err
				}
				peer.EthWSURI = ethWSURI
			}

			preset.StaticPeers = append(preset.StaticPeers, peer)
		} else if ctx.IsSet(utils.EthWSUriFlag.Name) {
			ethWSURI := ctx.String(utils.EthWSUriFlag.Name)
			err = validateWSURI(ethWSURI)
			if err != nil {
				return nil, "", err
			}

			preset.StaticPeers = append(preset.StaticPeers, PeerInfo{
				EthWSURI: ethWSURI,
			})
		}

		if ctx.IsSet(utils.BeaconENRFlag.Name) || ctx.IsSet(utils.BeaconMultiaddrFlag.Name) {
			if ctx.IsSet(utils.BeaconENRFlag.Name) && ctx.IsSet(utils.BeaconMultiaddrFlag.Name) {
				return nil, "", fmt.Errorf("parameters --enr and --multiaddr should not be used simultaneously; if you would like to use multiple p2p connections, use --multi-node")
			}

			peer := PeerInfo{}
			if ctx.IsSet(utils.BeaconENRFlag.Name) {
				multiAddr, err := multiaddrFromEnodeStr(ctx.String(utils.BeaconENRFlag.Name))
				if err != nil {
					return nil, "", fmt.Errorf("unable to parse --enr argument: %v", err)
				}

				peer.Multiaddr = &multiAddr
				peer.Endpoint = utils.MultiaddrToNodeEndpoint(multiAddr, blockchainNetwork)
			} else if ctx.IsSet(utils.BeaconMultiaddrFlag.Name) {
				multiAddr, err := multiaddrFromStr(ctx.String(utils.BeaconMultiaddrFlag.Name))
				if err != nil {
					return nil, "", fmt.Errorf("unable to parse --multiaddr argument: %v", err)
				}

				peer.Multiaddr = &multiAddr
				peer.Endpoint = utils.MultiaddrToNodeEndpoint(multiAddr, blockchainNetwork)
			}

			preset.StaticPeers = append(preset.StaticPeers, peer)
		}

		if ctx.IsSet(utils.PrysmGRPCFlag.Name) {
			prysmGRPCURI := ctx.String(utils.PrysmGRPCFlag.Name)
			err = validatePrysmGRPC(prysmGRPCURI)
			if err != nil {
				return nil, "", err
			}

			endpoint, err := utils.CreatePrysmEndpoint(prysmGRPCURI, blockchainNetwork)
			if err != nil {
				return nil, "", fmt.Errorf("unable to parse --prysm-grpc-uri argument: %v", err)
			}

			preset.StaticPeers = append(preset.StaticPeers, PeerInfo{
				PrysmAddr: prysmGRPCURI,
				Endpoint:  endpoint,
			})
		}

		if ctx.IsSet(utils.BeaconAPIUriFlag.Name) {
			beaconAPIEndpointsArg := ctx.String(utils.BeaconAPIUriFlag.Name)
			beaconAPIURI := strings.TrimSpace(beaconAPIEndpointsArg)

			if err = validateBeaconAPIURI(beaconAPIURI); err != nil {
				return nil, "", fmt.Errorf("--beacon-api-uri: error in parsing %s endpoint: %v", beaconAPIEndpointsArg, err)
			}

			endpoint, err := utils.CreateAPIEndpoint(beaconAPIURI, blockchainNetwork)
			if err != nil {
				return nil, "", fmt.Errorf("unable to parse --beacon-api-uri argument: %v", err)
			}

			preset.StaticPeers = append(preset.StaticPeers, PeerInfo{
				BeaconAPIURI: beaconAPIURI,
				Endpoint:     endpoint,
			})
		}
	}

	var privateKey *ecdsa.PrivateKey

	if ctx.IsSet(utils.PrivateKeyFlag.Name) {
		privateKeyHexString := ctx.String(utils.PrivateKeyFlag.Name)
		if len(privateKeyHexString) != privateKeyLen {
			return nil, "", fmt.Errorf("incorrect private key length: expected length %v, actual length %v", privateKeyLen, len(privateKeyHexString))
		}
		privateKey, err = crypto.HexToECDSA(privateKeyHexString)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert hex string to ECDSA key (%v)", err)
		}
		err = os.WriteFile(path.Join(dataDir, ".gatewaykey"), []byte(privateKeyHexString), 0o644)
		if err != nil {
			return nil, "", fmt.Errorf("failed to save private key to file (%v)", err)
		}
	}

	// Try to load or generate a new private key if one wasn't found
	if privateKey == nil && len(dataDir) != 0 {
		privateKeyPath := path.Join(dataDir, ".gatewaykey")
		privateKeyFromFile, _, err := LoadOrGeneratePrivateKey(privateKeyPath)
		if err != nil {
			return nil, "", fmt.Errorf("couldn't load or generate a private key (%v)", err)
		}

		privateKey = privateKeyFromFile
	}

	preset.PrivateKey = privateKey

	// Generate the enode
	db, err := enode.OpenDB("")
	if err != nil {
		return nil, "", err
	}

	ln := enode.NewLocalNode(db, preset.PrivateKey)
	ln.SetFallbackIP(net.IP{127, 0, 0, 1})
	node := ln.Node().URLv4()

	if ctx.IsSet(utils.SendBlockConfirmation.Name) {
		sendBCF := ctx.Bool(utils.SendBlockConfirmation.Name)
		preset.SendBlockConfirmation = sendBCF
	}

	if ctx.IsSet(utils.TerminalTotalDifficulty.Name) {
		ttd, ok := big.NewInt(0).SetString(ctx.String(utils.TerminalTotalDifficulty.Name), 0)
		if !ok {
			return nil, "", fmt.Errorf("unable to parse terminal-total-difficulty %v", ctx.String(utils.TerminalTotalDifficulty.Name))
		}
		preset.TerminalTotalDifficulty.Set(ttd)
		preset.TTDOverrides = true
	}

	return &preset, node, nil
}

// parseMultiNode parses a string into list of PeerInfo, according to expected format of multi-eth-ws-uri parameter
func (ec *EthConfig) parseMultiNode(multiNodeStr string, blockchainNetwork string) error {
	nodes := strings.Split(multiNodeStr, ",")
	for i, nodeStr := range nodes {
		// enode+eth-ws-uri only
		var combinedPeer *PeerInfo

		connURIs := strings.Split(nodeStr, "+")
		if len(connURIs) == 0 {
			return fmt.Errorf(invalidMultiNodeErrMsg, i)
		}

		for _, connURI := range connURIs {
			connURIParts := strings.Split(connURI, ":")

			if len(connURIParts) < 2 {
				return fmt.Errorf(invalidMultiNodeErrMsg, i)
			}

			connURIScheme := connURIParts[0]
			switch connURIScheme {
			case "enr":
				if combinedPeer != nil {
					return fmt.Errorf(invalidMultiNodeErrMsg, i)
				}

				multiaddr, err := multiaddrFromEnodeStr(connURI)
				if err != nil {
					return fmt.Errorf("invalid enr argument argument %d comma: %v", i, err)
				}

				ec.StaticPeers = append(ec.StaticPeers, PeerInfo{
					Multiaddr: &multiaddr,
					Endpoint:  utils.MultiaddrToNodeEndpoint(multiaddr, blockchainNetwork),
				})
			case "multiaddr":
				if combinedPeer != nil {
					return fmt.Errorf(invalidMultiNodeErrMsg, i)
				}

				multiaddr, err := multiaddrFromStr(connURIParts[1])
				if err != nil {
					return fmt.Errorf("invalid multiaddr argument after %d comma: %v", i, err)
				}

				ec.StaticPeers = append(ec.StaticPeers, PeerInfo{
					Multiaddr: &multiaddr,
					Endpoint:  utils.MultiaddrToNodeEndpoint(multiaddr, blockchainNetwork),
				})
			case "enode":
				if combinedPeer != nil {
					return fmt.Errorf(invalidMultiNodeErrMsg, i)
				}

				enode, err := enode.Parse(enode.ValidSchemes, connURI)
				if err != nil {
					return fmt.Errorf("invalid enode argument after %d comma: %v", i, err)
				}

				combinedPeer = &PeerInfo{
					Enode:    enode,
					Endpoint: utils.EnodeToNodeEndpoint(enode, blockchainNetwork),
				}
			case "ws", "wss":
				if combinedPeer == nil {
					ec.StaticPeers = append(ec.StaticPeers, PeerInfo{
						EthWSURI: connURI,
					})
				} else {
					combinedPeer.EthWSURI = connURI
				}
			case "prysm":
				prysmAddr := strings.TrimPrefix(connURI, "prysm://")

				endpoint, err := utils.CreatePrysmEndpoint(prysmAddr, blockchainNetwork)
				if err != nil {
					return fmt.Errorf("invalid prysm argument after %d comma: %v", i, err)
				}

				ec.StaticPeers = append(ec.StaticPeers, PeerInfo{
					PrysmAddr: prysmAddr,
					Endpoint:  endpoint,
				})
			case "beacon-api":
				beaconAPIUri := strings.TrimPrefix(connURI, "beacon-api://")

				if slices.ContainsFunc(ec.StaticPeers.BeaconAPIEndpoints(), func(peer PeerInfo) bool {
					return peer.BeaconAPIURI == beaconAPIUri
				}) {
					return fmt.Errorf("duplicated beacon-api argument after %d comma", i)
				}
				if err := validateBeaconAPIURI(beaconAPIUri); err != nil {
					return fmt.Errorf("invalid beacon-api argument after %d comma: %v", i, err)
				}

				endpoint, err := utils.CreateAPIEndpoint(beaconAPIUri, blockchainNetwork)
				if err != nil {
					return fmt.Errorf("invalid beacon-api argument after %d comma: %v", i, err)
				}

				ec.StaticPeers = append(ec.StaticPeers, PeerInfo{
					BeaconAPIURI: beaconAPIUri,
					Endpoint:     endpoint,
				})
			default:
				return fmt.Errorf(invalidMultiNodeErrMsg, i)
			}
		}

		if combinedPeer != nil {
			ec.StaticPeers = append(ec.StaticPeers, *combinedPeer)
		}
	}

	return nil
}

func validateBeaconAPIURI(uri string) error {
	parts := strings.Split(uri, ":")
	if len(parts) != 2 {
		return fmt.Errorf("--beacon-api-uri: invalid format, must be 'IP:PORT'")
	}

	ip := net.ParseIP(parts[0])
	if ip == nil {
		return fmt.Errorf("--beacon-api-uri: invalid IP address")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("--beacon-api-uri: invalid port number")
	}

	return nil
}

func validateWSURI(ethWSURI string) error {
	if !strings.HasPrefix(ethWSURI, "ws://") && !strings.HasPrefix(ethWSURI, "wss://") {
		return fmt.Errorf("unable to parse websockets URI [%v]. Expected format: ws(s)://IP:PORT", ethWSURI)
	}
	return nil
}

func validatePrysmGRPC(prysmGRPCUri string) error {
	if len(strings.Split(prysmGRPCUri, ":")) < 2 {
		return fmt.Errorf("unable to parse prysm-grpc-uri [%v]. Expected format: IP:PORT", prysmGRPCUri)
	}
	return nil
}

func parseEnodeMultiAddr(enodeStr string) (string, error) {
	node, err := enode.Parse(enode.ValidSchemes, enodeStr)
	if err != nil {
		return "", fmt.Errorf("could not parse enode: %v", err)
	}

	pubkey := node.Pubkey()
	assertedKey, err := ecdsaprysm.ConvertToInterfacePubkey(pubkey)
	if err != nil {
		return "", fmt.Errorf("could not get pubkey: %v", err)
	}
	id, err := libp2pPeer.IDFromPublicKey(assertedKey)
	if err != nil {
		return "", fmt.Errorf("could not get peer id: %v", err)
	}

	if node.TCP() == 0 {
		return "", errors.New("TCP port should be set")
	}

	if node.IP().To4() == nil && node.IP().To16() == nil {
		return "", fmt.Errorf("invalid ip address provided: %s", node.IP().String())
	}

	if node.IP().To4() != nil {
		return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", node.IP().String(), node.TCP(), id.String()), nil
	}

	return fmt.Sprintf("/ip6/%s/tcp/%d/p2p/%s", node.IP().String(), node.TCP(), id.String()), nil
}

func multiaddrFromEnodeStr(enodeStr string) (multiaddr.Multiaddr, error) {
	multiaddrStr, err := parseEnodeMultiAddr(enodeStr)
	if err != nil {
		return nil, err
	}

	return multiaddr.NewMultiaddr(multiaddrStr)
}

func multiaddrFromStr(multiAddr string) (multiaddr.Multiaddr, error) {
	multiaddr, err := multiaddr.NewMultiaddr(multiAddr)
	if err != nil {
		return nil, err
	}

	if err := validateMultiaddr(multiaddr); err != nil {
		return nil, err
	}

	return multiaddr, nil
}

func validateMultiaddr(multiAddr multiaddr.Multiaddr) error {
	var ip, dns, port, pubKey string
	multiaddr.ForEach(multiAddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP6:
			ip = c.Value()
			return true
		case multiaddr.P_IP4:
			ip = c.Value()
			return true
		case multiaddr.P_DNS:
			dns = c.Value()
			return true
		case multiaddr.P_TCP, multiaddr.P_UDP:
			port = c.Value()
			return true
		case multiaddr.P_P2P:
			pubKey = c.Value()
			return true
		case multiaddr.P_QUIC, multiaddr.P_QUIC_V1:
			return true
		}

		return false
	})

	if ip == "" && dns == "" {
		return errors.New("IP or DNS address is missing")
	}

	if dns != "" {
		_, err := net.ResolveIPAddr("", dns)
		if err != nil {
			return fmt.Errorf("DNS address '%s' is not valid: %v", dns, err)
		}
	}

	if port == "" {
		return errors.New("TCP or UDP port is missing")
	}

	if pubKey == "" {
		return errors.New("public key is missing")
	}

	return nil
}

// Update updates properties of the EthConfig pushed down from server configuration
func (ec *EthConfig) Update(otherConfig EthConfig) {
	ec.Network = otherConfig.Network
	ec.TotalDifficulty = otherConfig.TotalDifficulty
	if !ec.TTDOverrides {
		ec.TerminalTotalDifficulty.Set(otherConfig.TerminalTotalDifficulty)
	}
	ec.Head = otherConfig.Head
	ec.Genesis = otherConfig.Genesis
	ec.ExecutionLayerForks = otherConfig.ExecutionLayerForks
	ec.BlockConfirmationsCount = otherConfig.BlockConfirmationsCount
}

// StaticPeers is a list of peers to connect to
type StaticPeers []PeerInfo

// Enodes makes a list of enodes
func (peers *StaticPeers) Enodes() []*enode.Node {
	var enodesList []*enode.Node
	for _, peerInfo := range *peers {
		if peerInfo.Multiaddr == nil && peerInfo.Enode != nil {
			enodesList = append(enodesList, peerInfo.Enode)
		}
	}
	return enodesList
}

// BeaconNodes makes a list of nodes for beacon
func (peers *StaticPeers) BeaconNodes() []*multiaddr.Multiaddr {
	var beaconNodes []*multiaddr.Multiaddr
	for _, peer := range *peers {
		if peer.Multiaddr != nil {
			beaconNodes = append(beaconNodes, peer.Multiaddr)
		}
	}

	return beaconNodes
}

// BeaconAPIEndpoints makes a list of endpoints which supports beacon API
func (peers *StaticPeers) BeaconAPIEndpoints() []PeerInfo {
	return utils.Filter(*peers, func(peer PeerInfo) bool {
		return peer.BeaconAPIURI != ""
	})
}

// PrysmAddrs makes a list of prysm addresses
func (peers *StaticPeers) PrysmAddrs() []PeerInfo {
	return utils.Filter(*peers, func(peer PeerInfo) bool {
		return peer.PrysmAddr != ""
	})
}

// ValidWSAddr indicates whether a valid eth ws uri was parsed
func (peers *StaticPeers) ValidWSAddr() bool {
	for _, peerInfo := range *peers {
		if peerInfo.EthWSURI != "" {
			return true
		}
	}
	return false
}

// Endpoints makes a list of endpoints
func (peers *StaticPeers) Endpoints() []types.NodeEndpoint {
	endpoints := make([]types.NodeEndpoint, len(*peers))
	for i, peer := range *peers {
		endpoints[i] = peer.Endpoint
	}

	return endpoints
}

// KeyWriteError is an error type that indicates an issue with writing the private key to disk.
type KeyWriteError struct {
	error
}

// LoadOrGeneratePrivateKey tries to load an ECDSA private key from the provided path. If this file does not exist, a new key is generated in its place.
func LoadOrGeneratePrivateKey(keyPath string) (privateKey *ecdsa.PrivateKey, generated bool, err error) {
	privateKey, err = crypto.LoadECDSA(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			dir, _ := path.Split(keyPath)
			if err = os.MkdirAll(dir, 0o755); err != nil {
				err = KeyWriteError{err}
				return
			}

			if privateKey, err = crypto.GenerateKey(); err != nil {
				return
			}

			if err = crypto.SaveECDSA(keyPath, privateKey); err != nil {
				err = KeyWriteError{err}
				return
			}

			generated = true
		} else {
			return
		}
	}
	return
}
