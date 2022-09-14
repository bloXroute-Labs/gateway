package network

import (
	"crypto/ecdsa"
	"errors"
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
	IsBeacon  bool
	EthWSURI  string
	PrysmAddr string
}

// EthConfig represents Ethereum network configuration settings (e.g. indicate Mainnet, Rinkeby, BSC, etc.). Most of this information will be exchanged in the status messages.
type EthConfig struct {
	StaticPeers []PeerInfo
	PrivateKey  *ecdsa.PrivateKey
	Port        int

	Network                 uint64
	TotalDifficulty         *big.Int
	TerminalTotalDifficulty *big.Int
	TTDOverrides            bool
	Head                    common.Hash
	Genesis                 common.Hash
	BlockConfirmationsCount int
	SendBlockConfirmation   bool

	IgnoreBlockTimeout time.Duration
	IgnoreSlotCount    int
}

const privateKeyLen = 64
const invalidMultiNodeErrMsg = "unable to parse --multi-node argument node number %d. Expected format: enode[+eth-ws-uri],enr[+prysm://prysm-host:prysm-port]"

// NewPresetEthConfigFromCLI builds a new EthConfig from the command line context. Selects a specific network configuration based on the provided startup flag.
func NewPresetEthConfigFromCLI(ctx *cli.Context, dataDir string) (*EthConfig, string, error) {
	preset, err := NewEthereumPreset(ctx.String(utils.BlockchainNetworkFlag.Name))
	if err != nil {
		return nil, "", err
	}

	preset.Port = ctx.Int(utils.PortFlag.Name)

	peers := make([]PeerInfo, 0)
	if ctx.IsSet(utils.MultiNode.Name) {
		if ctx.IsSet(utils.EnodesFlag.Name) || ctx.IsSet(utils.BeaconENRFlag.Name) || ctx.IsSet(utils.PrysmGRPCFlag.Name) || ctx.IsSet(utils.EthWSUriFlag.Name) {
			return nil, "", errors.New("parameters --multi-node and (--enodes, --enr, --prysm-grpc-uri or --eth-ws-uri) should not be used simultaneously; if you would like to use multiple p2p or websockets connections, use --multi-node")
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
		if ctx.IsSet(utils.BeaconENRFlag.Name) {
			enodeString := ctx.String(utils.BeaconENRFlag.Name)
			enode, err := enode.Parse(enode.ValidSchemes, enodeString)
			if err != nil {
				return nil, "", err
			}

			peer := PeerInfo{
				Enode:    enode,
				IsBeacon: true,
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
	enode := ln.Node().URLv4()

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

	return &preset, enode, nil
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
				peer.IsBeacon = true
				fallthrough
			case "enode":
				enode, err := enode.Parse(enode.ValidSchemes, connURI)
				if err != nil {
					return nil, err
				}
				if peer.Enode != nil {
					return nil, fmt.Errorf(invalidMultiNodeErrMsg, i)
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

func validatePrysmGRPC(prysmGRPCUri string) error {
	if len(strings.Split(prysmGRPCUri, ":")) < 2 {
		return fmt.Errorf("unable to parse prysm-grpc-uri [%v]. Expected format: IP:PORT", prysmGRPCUri)
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
	ec.BlockConfirmationsCount = otherConfig.BlockConfirmationsCount
}

// StaticEnodes makes a list of enodes only from StaticPeers
func (ec *EthConfig) StaticEnodes() []*enode.Node {
	var enodesList []*enode.Node
	for _, peerInfo := range ec.StaticPeers {
		if !peerInfo.IsBeacon {
			enodesList = append(enodesList, peerInfo.Enode)
		}
	}
	return enodesList
}

// BeaconNodes makes a list of nodes for beacon
func (ec *EthConfig) BeaconNodes() []string {
	var beaconNodes []string
	for _, peer := range ec.StaticPeers {
		if peer.IsBeacon {
			beaconNodes = append(beaconNodes, peer.Enode.String())
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
