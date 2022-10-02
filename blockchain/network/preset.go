package network

import (
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// EthMainnetChainID ethereum mainnet chain ID
const EthMainnetChainID = 1

var networkMapping = map[string]EthConfig{
	"Mainnet":         newEthereumMainnetConfig(),
	"BSC-Mainnet":     newBSCMainnetConfig(),
	"Polygon-Mainnet": newPolygonMainnetConfig(),
	"Kiln":            newEthereumKilnConfig(),
	"Ropsten":         newEthereumRopstenConfig(),
}

// NewEthereumPreset returns an Ethereum configuration for the given network name. For most of these presets, the client will present itself as only having the genesis block, but that shouldn't matter too much.
func NewEthereumPreset(network string) (EthConfig, error) {
	config, ok := networkMapping[network]
	if !ok {
		return unknownConfig(), fmt.Errorf("network %v did not have an available configuration", network)
	}
	return config, nil
}

func newEthereumMainnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("0400000000", 16)
	if !ok {
		panic("could not load Ethereum Mainnet configuration")
	}

	ttd, _ := big.NewInt(0).SetString("58750000000000000000000", 0)
	return EthConfig{
		Network:                 EthMainnetChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		Head:                    common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
		Genesis:                 common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
		IgnoreBlockTimeout:      150 * time.Second,
		IgnoreSlotCount:         10,
	}
}

func newEthereumRopstenConfig() EthConfig {
	td, ok := new(big.Int).SetString("0400000000", 16)
	if !ok {
		panic("could not load Ethereum Ropsten configuration")
	}

	return EthConfig{
		Network:                 3,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(50000000000000000),
		Head:                    common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d"),
		Genesis:                 common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d"),
		IgnoreBlockTimeout:      150 * time.Second,
		IgnoreSlotCount:         10,
	}
}

func newBSCMainnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load BSC Mainnet configuration")
	}

	return EthConfig{
		Network:                 56,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
		Head:                    common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		Genesis:                 common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		IgnoreBlockTimeout:      30 * time.Second,
	}
}

func newPolygonMainnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load BSC Mainnet configuration")
	}

	return EthConfig{
		Network:                 137,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
		Head:                    common.HexToHash("0xa9c28ce2141b56c474f1dc504bee9b01eb1bd7d1a507580d5519d4437a97de1b"),
		Genesis:                 common.HexToHash("0xa9c28ce2141b56c474f1dc504bee9b01eb1bd7d1a507580d5519d4437a97de1b"),
		IgnoreBlockTimeout:      30 * time.Second,
	}
}

func newEthereumKilnConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load Kiln configuration")
	}

	return EthConfig{
		Network:            1337802,
		TotalDifficulty:    td,
		Head:               common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		Genesis:            common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		IgnoreBlockTimeout: 150 * time.Second,
	}
}

func unknownConfig() EthConfig {
	return EthConfig{
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
	}
}
