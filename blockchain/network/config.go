package network

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"math/big"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v4/crypto/ecdsa"
)

// PeerInfo contains the enode and websockets endpoint for an Ethereum peer
type PeerInfo struct {
	Enode     *enode.Node
	Multiaddr *multiaddr.Multiaddr
	EthWSURI  string
	PrysmAddr string
}

// EthConfig represents Ethereum network configuration settings (e.g. indicate Mainnet, Rinkeby, BSC, etc.). Most of this information will be exchanged in the status messages.
type EthConfig struct {
	StaticPeers    []PeerInfo
	BootstrapNodes []*enode.Node
	PrivateKey     *ecdsa.PrivateKey
	Port           int

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

const privateKeyLen = 64
const invalidMultiNodeErrMsg = "unable to parse --multi-node argument node number %d. Expected format: enode[+eth-ws-uri],enr[+prysm://prysm-host:prysm-port],multiaddr[+prysm://prysm-host:prysm-port]"

// NewPresetEthConfigFromCLI builds a new EthConfig from the command line context. Selects a specific network configuration based on the provided startup flag.
func NewPresetEthConfigFromCLI(ctx *cli.Context, dataDir string) (*EthConfig, string, error) {
	preset, err := NewEthereumPreset(ctx.String(utils.BlockchainNetworkFlag.Name))
	if err != nil {
		return nil, "", err
	}

	peers := make([]PeerInfo, 0)
	if ctx.IsSet(utils.MultiNode.Name) {
		if ctx.IsSet(utils.EnodesFlag.Name) || ctx.IsSet(utils.BeaconENRFlag.Name) || ctx.IsSet(utils.BeaconMultiaddrFlag.Name) || ctx.IsSet(utils.PrysmGRPCFlag.Name) || ctx.IsSet(utils.EthWSUriFlag.Name) {
			return nil, "", errors.New("parameters --multi-node and (--enodes, --enr, --multiaddr, --prysm-grpc-uri or --eth-ws-uri) should not be used simultaneously; if you would like to use multiple p2p or websockets connections, use --multi-node")
		}
		peers, err = ParseMultiNode(ctx.String(utils.MultiNode.Name))
		if err != nil {
			return nil, "", err
		}
	} else {
		if ctx.IsSet(utils.EnodesFlag.Name) {
			enodeString := ctx.String(utils.EnodesFlag.Name)
			var peer PeerInfo
			enode, err := enode.Parse(enode.ValidSchemes, enodeString)
			if err != nil {
				return nil, "", err
			}
			peer.Enode = enode
			if ctx.IsSet(utils.EthWSUriFlag.Name) {
				ethWSURI := ctx.String(utils.EthWSUriFlag.Name)
				err = validateWSURI(ethWSURI)
				if err != nil {
					return nil, "", err
				}
				peer.EthWSURI = ethWSURI
			}

			// Prysm is using enode address for source of the blocks in stats
			if ctx.IsSet(utils.PrysmGRPCFlag.Name) && !ctx.IsSet(utils.BeaconENRFlag.Name) {
				prysmGRPCURI := ctx.String(utils.PrysmGRPCFlag.Name)
				err = validatePrysmGRPC(prysmGRPCURI)
				if err != nil {
					return nil, "", err
				}
				peer.PrysmAddr = prysmGRPCURI
			}
			peers = append(peers, peer)
		}
		if ctx.IsSet(utils.BeaconENRFlag.Name) || ctx.IsSet(utils.BeaconMultiaddrFlag.Name) {
			if ctx.IsSet(utils.BeaconENRFlag.Name) && ctx.IsSet(utils.BeaconMultiaddrFlag.Name) {
				return nil, "", fmt.Errorf("parameters --enr and --multiaddr should not be used simultaneously; if you would like to use multiple p2p connections, use --multi-node")
			}

			peer := PeerInfo{}
			if ctx.IsSet(utils.BeaconENRFlag.Name) {
				multiaddr, err := multiaddrFromEnodeStr(ctx.String(utils.BeaconENRFlag.Name))
				if err != nil {
					return nil, "", fmt.Errorf("unable to parse --enr argument: %v", err)
				}

				peer.Multiaddr = &multiaddr
			} else if ctx.IsSet(utils.BeaconMultiaddrFlag.Name) {
				multiaddr, err := multiaddrFromStr(ctx.String(utils.BeaconMultiaddrFlag.Name))
				if err != nil {
					return nil, "", fmt.Errorf("unable to parse --multiaddr argument: %v", err)
				}

				peer.Multiaddr = &multiaddr
			}

			if ctx.IsSet(utils.PrysmGRPCFlag.Name) {
				prysmGRPCURI := ctx.String(utils.PrysmGRPCFlag.Name)
				err = validatePrysmGRPC(prysmGRPCURI)
				if err != nil {
					return nil, "", err
				}
				peer.PrysmAddr = prysmGRPCURI
			}

			peers = append(peers, peer)
		}
	}

	preset.StaticPeers = peers

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
		err = os.WriteFile(path.Join(dataDir, ".gatewaykey"), []byte(privateKeyHexString), 0644)
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

// ParseMultiNode parses a string into list of PeerInfo, according to expected format of multi-eth-ws-uri parameter
func ParseMultiNode(multiNodeStr string) ([]PeerInfo, error) {
	nodes := strings.Split(multiNodeStr, ",")
	peers := make([]PeerInfo, 0, len(nodes))

	for i, nodeStr := range nodes {
		var peer PeerInfo
		connURIs := strings.Split(nodeStr, "+")
		if len(connURIs) == 0 {
			return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
		}

		for _, connURI := range connURIs {
			connURIParts := strings.Split(connURI, ":")

			if len(connURIParts) < 2 {
				return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
			}

			connURIScheme := connURIParts[0]
			switch connURIScheme {
			case "enr":
				if peer.Enode != nil || peer.Multiaddr != nil {
					return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
				}

				multiaddr, err := multiaddrFromEnodeStr(connURI)
				if err != nil {
					return nil, fmt.Errorf("invalid enr argument argument %d comma: %v", i, err)
				}

				peer.Multiaddr = &multiaddr
			case "multiaddr":
				if peer.Enode != nil || peer.Multiaddr != nil {
					return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
				}

				multiaddr, err := multiaddrFromStr(connURIParts[1])
				if err != nil {
					return nil, fmt.Errorf("invalid multiaddr argument after %d comma: %v", i, err)
				}

				peer.Multiaddr = &multiaddr
			case "enode":
				if peer.Enode != nil || peer.Multiaddr != nil {
					return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
				}

				enode, err := enode.Parse(enode.ValidSchemes, connURI)
				if err != nil {
					return nil, fmt.Errorf("invalid enode argument after %d comma: %v", i, err)
				}

				peer.Enode = enode
			case "ws", "wss":
				peer.EthWSURI = connURI
			case "prysm":
				peer.PrysmAddr = strings.TrimPrefix(connURI, "prysm://")
			default:
				return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
			}
		}

		if peer.Enode == nil && peer.Multiaddr == nil {
			return nil, fmt.Errorf("none of enode/enr/multiaddr were specified in --multi-node %d connection URI", i)
		}

		peers = append(peers, peer)
	}
	return peers, nil
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
	var ip, port, pubKey string
	multiaddr.ForEach(multiAddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP6:
			ip = c.Value()
			return true
		case multiaddr.P_IP4:
			ip = c.Value()
			return true
		case multiaddr.P_TCP:
			port = c.Value()
			return true
		case multiaddr.P_P2P:
			pubKey = c.Value()
			return true
		}

		return false
	})

	if ip == "" {
		return errors.New("IP address is missing")
	}

	if port == "" {
		return errors.New("TCP port is missing")
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

// StaticEnodes makes a list of enodes only from StaticPeers
func (ec *EthConfig) StaticEnodes() []*enode.Node {
	var enodesList []*enode.Node
	for _, peerInfo := range ec.StaticPeers {
		if peerInfo.Multiaddr == nil {
			enodesList = append(enodesList, peerInfo.Enode)
		}
	}
	return enodesList
}

// BeaconNodes makes a list of nodes for beacon
func (ec *EthConfig) BeaconNodes() []*multiaddr.Multiaddr {
	var beaconNodes []*multiaddr.Multiaddr
	for _, peer := range ec.StaticPeers {
		if peer.Multiaddr != nil {
			beaconNodes = append(beaconNodes, peer.Multiaddr)
		}
	}

	return beaconNodes
}

// ValidWSAddr indicates whether a valid eth ws uri was parsed
func (ec *EthConfig) ValidWSAddr() bool {
	for _, peerInfo := range ec.StaticPeers {
		if peerInfo.EthWSURI != "" {
			return true
		}
	}
	return false
}

type keyWriteError struct {
	error
}

// LoadOrGeneratePrivateKey tries to load an ECDSA private key from the provided path. If this file does not exist, a new key is generated in its place.
func LoadOrGeneratePrivateKey(keyPath string) (privateKey *ecdsa.PrivateKey, generated bool, err error) {
	privateKey, err = crypto.LoadECDSA(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			dir, _ := path.Split(keyPath)
			if err = os.MkdirAll(dir, 0755); err != nil {
				err = keyWriteError{err}
				return
			}

			if privateKey, err = crypto.GenerateKey(); err != nil {
				return
			}

			if err = crypto.SaveECDSA(keyPath, privateKey); err != nil {
				err = keyWriteError{err}
				return
			}

			generated = true
		} else {
			return
		}
	}
	return
}
