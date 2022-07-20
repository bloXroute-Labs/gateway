package network

import (
	"crypto/ecdsa"
	"fmt"
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
	"github.com/urfave/cli/v2"
)

// PeerInfo contains the enode and websockets endpoint for an Ethereum peer
type PeerInfo struct {
	Enode     *enode.Node
	EthWSURI  string
	PrysmAddr string
}

// EthConfig represents Ethereum network configuration settings (e.g. indicate Mainnet, Rinkeby, BSC, etc.). Most of this information will be exchanged in the status messages.
type EthConfig struct {
	StaticPeers []PeerInfo
	PrivateKey  *ecdsa.PrivateKey

	Network                 uint64
	TotalDifficulty         *big.Int
	TerminalTotalDifficulty *big.Int
	TTDOverrides            bool
	Head                    common.Hash
	Genesis                 common.Hash
	BlockConfirmationsCount int
	SendBlockConfirmation   bool

	IgnoreBlockTimeout time.Duration
}

const privateKeyLen = 64

// NewPresetEthConfigFromCLI builds a new EthConfig from the command line context. Selects a specific network configuration based on the provided startup flag.
func NewPresetEthConfigFromCLI(ctx *cli.Context, dataDir string) (*EthConfig, string, error) {
	preset, err := NewEthereumPreset(ctx.String(utils.BlockchainNetworkFlag.Name))
	if err != nil {
		return nil, "", err
	}

	var peers []PeerInfo
	if ctx.IsSet(utils.MultiNode.Name) {
		if ctx.IsSet(utils.EnodesFlag.Name) {
			return nil, "", fmt.Errorf("parameters --multi-node and --enodes should not be used simultaneously; if you would like to use multiple p2p or websockets connections, use --multi-node")
		}
		peers, err = ParseMultiNode(ctx.String(utils.MultiNode.Name))
		if err != nil {
			return nil, "", err
		}
	} else if ctx.IsSet(utils.EnodesFlag.Name) {
		enodeString := ctx.String(utils.EnodesFlag.Name)
		peers = make([]PeerInfo, 0, 1)
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

		if ctx.IsSet(utils.PrysmHostFlag.Name) && ctx.IsSet(utils.PrysmPortFlag.Name) {
			peer.PrysmAddr = fmt.Sprintf("%s:%d", ctx.String(utils.PrysmHostFlag.Name), ctx.Int(utils.PrysmPortFlag.Name))
		}

		peers = append(peers, peer)
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
	enode := ln.Node().URLv4()

	if ctx.IsSet(utils.SendBlockConfirmation.Name) {
		sendBCF := ctx.Bool(utils.SendBlockConfirmation.Name)
		preset.SendBlockConfirmation = sendBCF
	}

	if ctx.IsSet(utils.TerminalTotalDifficulty.Name) {
		preset.TerminalTotalDifficulty = big.NewInt(int64(ctx.Int(utils.TerminalTotalDifficulty.Name)))
		preset.TTDOverrides = true
	}

	return &preset, enode, nil
}

// ParseMultiNode parses a string into list of PeerInfo, according to expected format of multi-eth-ws-uri parameter
func ParseMultiNode(multiNodeStr string) ([]PeerInfo, error) {
	nodes := strings.Split(multiNodeStr, ",")
	peers := make([]PeerInfo, 0, len(nodes))

	invalidMultiNodeErrMsg := "unable to parse --multi-node %d connection URI. Expected format: enode[+eth-ws-uri][+beacon:prysm-host:prysm-port]"

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
			case "enode":
				enode, err := enode.Parse(enode.ValidSchemes, connURI)
				if err != nil {
					return nil, err
				}
				peer.Enode = enode
			case "ws", "wss":
				peer.EthWSURI = connURI
			case "beacon":
				peer.PrysmAddr = strings.TrimPrefix(connURI, "beacon:")
			default:
				return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
			}
		}

		if peer.Enode == nil {
			return nil, fmt.Errorf("enode missed in --multi-node %d connection URI", i)
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

// Update updates properties of the EthConfig pushed down from server configuration
func (ec *EthConfig) Update(otherConfig EthConfig) {
	ec.Network = otherConfig.Network
	ec.TotalDifficulty = otherConfig.TotalDifficulty
	if !ec.TTDOverrides {
		ec.TerminalTotalDifficulty = otherConfig.TerminalTotalDifficulty
	}
	ec.Head = otherConfig.Head
	ec.Genesis = otherConfig.Genesis
	ec.BlockConfirmationsCount = otherConfig.BlockConfirmationsCount
}

// StaticEnodes makes a list of enodes only from StaticPeers
func (ec *EthConfig) StaticEnodes() []*enode.Node {
	enodesList := make([]*enode.Node, len(ec.StaticPeers))
	for i, peerInfo := range ec.StaticPeers {
		enodesList[i] = peerInfo.Enode
	}
	return enodesList
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
