package network

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/urfave/cli/v2"
	"math/big"
	"strings"
)

// EthConfig represents Ethereum network configuration settings (e.g. indicate Mainnet, Rinkeby, BSC, etc.). Most of this information will be exchanged in the status messages.
type EthConfig struct {
	StaticPeers []*enode.Node
	PrivateKey  *ecdsa.PrivateKey

	Network         uint64
	TotalDifficulty *big.Int
	Head            common.Hash
	Genesis         common.Hash
}

const privateKeyLen = 64

// NewPresetEthConfigFromCLI builds a new EthConfig from the command line context. Selects a specific network configuration based on the provided startup flag.
func NewPresetEthConfigFromCLI(ctx *cli.Context) (*EthConfig, error) {
	preset, err := NewEthereumPreset(ctx.String(utils.BlockchainNetworkFlag.Name))
	if err != nil {
		return nil, err
	}

	var peers []*enode.Node

	if ctx.IsSet(utils.EnodesFlag.Name) {
		enodeStrings := strings.Split(ctx.String(utils.EnodesFlag.Name), ",")
		peers = make([]*enode.Node, 0, len(enodeStrings))

		for _, enodeString := range enodeStrings {
			enode, err := enode.Parse(enode.ValidSchemes, enodeString)
			if err != nil {
				return nil, err
			}
			peers = append(peers, enode)
		}

		if len(enodeStrings) > 1 {
			return nil, errors.New("blockchain client does not currently support more than a single connection")
		}
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

	return &preset, nil
}

// Update updates properties of the EthConfig pushed down from server configuration
func (ec *EthConfig) Update(otherConfig EthConfig) {
	ec.Network = otherConfig.Network
	ec.TotalDifficulty = otherConfig.TotalDifficulty
	ec.Head = otherConfig.Head
	ec.Genesis = otherConfig.Genesis
}
