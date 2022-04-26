package network

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/urfave/cli/v2"
	"math/big"
	"strings"
	"time"
)

// PeerInfo contains the enode and websockets endpoint for an Ethereum peer
type PeerInfo struct {
	Enode    *enode.Node
	EthWSURI string
}

// EthConfig represents Ethereum network configuration settings (e.g. indicate Mainnet, Rinkeby, BSC, etc.). Most of this information will be exchanged in the status messages.
type EthConfig struct {
	StaticPeers []PeerInfo
	PrivateKey  *ecdsa.PrivateKey

	Network                 uint64
	TotalDifficulty         *big.Int
	Head                    common.Hash
	Genesis                 common.Hash
	BlockConfirmationsCount int
	SendBlockConfirmation   bool

	IgnoreBlockTimeout time.Duration
}

const privateKeyLen = 64

// NewPresetEthConfigFromCLI builds a new EthConfig from the command line context. Selects a specific network configuration based on the provided startup flag.
func NewPresetEthConfigFromCLI(ctx *cli.Context) (*EthConfig, error) {
	preset, err := NewEthereumPreset(ctx.String(utils.BlockchainNetworkFlag.Name))
	if err != nil {
		return nil, err
	}

	var peers []PeerInfo
	if ctx.IsSet(utils.MultiNode.Name) {
		if ctx.IsSet(utils.EnodesFlag.Name) {
			return nil, fmt.Errorf("parameters --multi-node and --enodes should not be used simultaneously; if you would like to use multiple p2p or websockets connections, use --multi-node")
		}
		peers, err = ParseMultiNode(ctx.String(utils.MultiNode.Name))
		if err != nil {
			return nil, err
		}
	} else if ctx.IsSet(utils.EnodesFlag.Name) {
		enodeString := ctx.String(utils.EnodesFlag.Name)
		peers = make([]PeerInfo, 0, 1)
		var peer PeerInfo

		enode, err := enode.Parse(enode.ValidSchemes, enodeString)
		if err != nil {
			return nil, err
		}
		peer.Enode = enode

		if ctx.IsSet(utils.EthWSUriFlag.Name) {
			ethWSURI := ctx.String(utils.EthWSUriFlag.Name)
			err = validateWSURI(ethWSURI)
			if err != nil {
				return nil, err
			}
			peer.EthWSURI = ethWSURI
		}

		peers = append(peers, peer)
	}

	preset.StaticPeers = peers

	if ctx.IsSet(utils.PrivateKeyFlag.Name) {
		privateKeyHexString := ctx.String(utils.PrivateKeyFlag.Name)
		if len(privateKeyHexString) != privateKeyLen {
			return nil, fmt.Errorf("incorrect private key length: expected length %v, actual length %v", privateKeyLen, len(privateKeyHexString))
		}
		privateKey, err := crypto.HexToECDSA(privateKeyHexString)
		if err != nil {
			return nil, err
		}
		preset.PrivateKey = privateKey
	}

	if ctx.IsSet(utils.SendBlockConfirmation.Name) {
		sendBCF := ctx.Bool(utils.SendBlockConfirmation.Name)
		preset.SendBlockConfirmation = sendBCF
	}

	return &preset, nil
}

// ParseMultiNode parses a string into list of PeerInfo, according to expected format of multi-eth-ws-uri parameter
func ParseMultiNode(multiEthWSURI string) ([]PeerInfo, error) {
	enodeWSPairs := strings.Split(multiEthWSURI, ",")
	peers := make([]PeerInfo, 0, len(enodeWSPairs))
	for _, pairStr := range enodeWSPairs {
		var peer PeerInfo
		enodeWSPair := strings.Split(pairStr, "+")
		if len(enodeWSPair) == 0 {
			return nil, fmt.Errorf("unable to parse --multi-node. Expected format: enode1+eth-ws-uri-1,enode2+,enode3+eth-ws-uri-3")
		}
		enode, err := enode.Parse(enode.ValidSchemes, enodeWSPair[0])
		if err != nil {
			return nil, err
		}
		peer.Enode = enode

		ethWSURI := ""
		if len(enodeWSPair) > 1 {
			ethWSURI = enodeWSPair[1]
			if ethWSURI != "" {
				err = validateWSURI(ethWSURI)
				if err != nil {
					return nil, err
				}
			}
		}
		peer.EthWSURI = ethWSURI
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
