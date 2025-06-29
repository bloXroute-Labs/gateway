package network

import (
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	ethparams "github.com/ethereum/go-ethereum/params"
)

// EthMainnetChainID ethereum mainnet chain ID
const EthMainnetChainID = 1

// BSCMainnetChainID BSC mainnet chain ID
const BSCMainnetChainID = 56

// BSCTestnetChainID BSC testnet chain ID
const BSCTestnetChainID = 97

// HoleskyChainID Holesky testnet chain ID
const HoleskyChainID = 17000

var networkMapping = map[string]EthConfig{
	"Mainnet":     newEthereumMainnetConfig(),
	"BSC-Mainnet": newBSCMainnetConfig(),
	"BSC-Testnet": newBSCTestnetConfig(),
	"Holesky":     newHoleskyConfig(),
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

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, ethparams.MainnetBootnodes)
	if err != nil {
		panic("could not set Ethereum Mainnet bootstrapNodes")
	}

	ttd, _ := big.NewInt(0).SetString("58750000000000000000000", 0)

	return EthConfig{
		Network:                 EthMainnetChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		GenesisTime:             1606824023,
		Head:                    common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
		Genesis:                 common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
		IgnoreBlockTimeout:      150 * time.Second,
		IgnoreSlotCount:         10,
		BootstrapNodes:          bootNodes,
		ProgramName:             "Geth/v1.15.11/linux-amd64/go1.24.2",
	}
}

func newBSCMainnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load BSC Mainnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://fe0bb07eae29e8cfaa5bb15b0db8c386a45b7da2c94e1dabd7ca58b6327eee0c27bdcea4f08db19ea07b9a1391e5496a28c675c6eee578154edae4fa44640c5d@54.228.2.74:30311",
		"enode://c307b4cddec0aea2188eafddedb0a076b9289402c63217b4c81eb7f34761c7cfaf6b075e93d7357169e226ff1bb4aa3bd71869b4c76cf261e2991005ddb4d4aa@3.81.81.182:30311",
		"enode://84a76ad1fab6164cbb00179dd07c96755141ffb75d5d387f45295e6ecfcc9e12a720f1f3dca8318449eeff768d13e9d49a414d2b522d1bcf2919aebf4852ab46@44.198.58.179:30311",
		"enode://41d57b0f00d83016e1bb4eccff0f3034aa49345301b7be96c6bb23a0a852b9b87b9ed11827c188ad409019fb0e578917d722f318665f198340b8a15ae8beff36@34.252.87.229:30311",
		"enode://accbc0a5af0af03e1ec3b5e80544bdceea48011a6928cd82d2c1a9c38b65fd48ec970ba17bd8c0b0ec21a28faec9efe1d1ce55134784b9207146e2f62d8932ba@54.162.32.1:30311",
		"enode://e333532e47a14dba7603c9ab0598e68be2c0822200855844edd45f50bfba481451ca5ee5247dbca2b54fe522e74a658edc15c8eed917360e1a289b3ab78ecf4c@3.250.36.7:30311",
		"enode://9f005be9111a6152884fd575abb55bddb1e7f726510c96cddde57a9bba84ffa4952a89d7632c9c9dd50d3750f83966a73a0f7ed793f253a3691b84a687b29b6c@3.88.177.211:30311",
		"enode://5451251a9902e658154456ea98ebdd93313e54496ce0a6ca2242fe4db882940d78d758c85a36485af54b0841270f2bdbff64d66c45976f3ed1dd912f7649c831@3.236.189.129:30311",
		"enode://a232f92d1e76447b93306ece2f6a55ac70ca4633fae0938d71a100757eaf8526e6bbf720aa70cba1e6d186be17291ad1ee851a35596ec6caa2fdf135ce4b6b68@107.20.124.16:30311",
		"enode://62c516645635f0389b4c851bfc4545720fac0607de74942e4ea7e923f4fa2ac0c438c146e2f0721c8ce06dca4e7f30f5c0136569d9f4b6a827c62b980fd53272@52.215.57.20:30311",
		"enode://c014bbf48209cdf8ca6d3bf3ff5cf2fade45104283dcfc079df6c64e0f4b65e4afe28040fa1731a0732bd9cbb90786cf78f0174b5de7bd5b303088e80d8e6a83@54.74.101.143:30311",
		"enode://710ed272e03b92c803cd165e5fa071da015815d312f17d107a43ad3b12b0f05c830c58ced2df7547294f5365fe76cdcf1a58f923ee5612d247a6d5b80cfe16a8@34.245.31.55:30311",
		"enode://768af449287561c0f17bb5dc5d98a1c6a4b1798cb41159bd0a7bfebdc179e39ad8076d7292caa9344eecb94a5f7499e632c29cc4edbdf2e8ada3f7c8c7b2a64b@3.95.173.72:30311",
		"enode://8428650e034341479d0ca3142bcd412f400ba47454bb7caeb88cfeb9bb60c21e45153eddf3e334d5d94ae67609ec2ac44816b346a2b3216d94a7c095883141e3@54.195.188.155:30311",
	})

	if err != nil {
		panic("could not set BSC Mainnet bootstrapNodes")
	}

	return EthConfig{
		Network:                 BSCMainnetChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
		Head:                    common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		Genesis:                 common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		IgnoreBlockTimeout:      30 * time.Second,
		BootstrapNodes:          bootNodes,
		ProgramName:             "Geth/v1.15.11/linux-amd64/go1.24.2",
	}
}

func newBSCTestnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load BSC Testnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://0637d1e62026e0c8685b1db0ca1c767c78c95c3fab64abc468d1a64b12ca4b530b46b8f80c915aec96f74f7ffc5999e8ad6d1484476f420f0c10e3d42361914b@52.199.214.252:30311",
		"enode://df1e8eb59e42cad3c4551b2a53e31a7e55a2fdde1287babd1e94b0836550b489ba16c40932e4dacb16cba346bd442c432265a299c4aca63ee7bb0f832b9f45eb@52.51.80.128:30311",
		"enode://ecd664250ca19b1074dcfbfb48576a487cc18d052064222a363adacd2650f8e08fb3db9de7a7aecb48afa410eaeb3285e92e516ead01fb62598553aed91ee15e@3.209.122.123:30311",
		"enode://665cf77ca26a8421cfe61a52ac312958308d4912e78ce8e0f61d6902e4494d4cc38f9b0dd1b23a427a7a5734e27e5d9729231426b06bb9c73b56a142f83f6b68@52.72.123.113:30311",
		"enode://428b12bcbbe4f607f6d83f91decbce549be5f0819d793ac32b0c7280f159dbb6125837b24d39ad1d568bc42d35e0754600429ea48044a44555e8af2113084ec7@18.181.52.189:30311",
		"enode://28daea97a03f0bff6f061c3fbb2e7b61d61b8683240eb03310dfa2fd1d56f3551f714bb09515c3e389bae6ff11bd85e45075460408696f5f9a782b9ffb66e1d1@34.242.33.165:30311",
	})

	if err != nil {
		panic("could not set BSC Testnet bootstrapNodes")
	}

	return EthConfig{
		Network:                 BSCTestnetChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
		Head:                    common.HexToHash("6d3c66c5357ec91d5c43af47e234a939b22557cbb552dc45bebbceeed90fbe34"),
		Genesis:                 common.HexToHash("6d3c66c5357ec91d5c43af47e234a939b22557cbb552dc45bebbceeed90fbe34"),
		IgnoreBlockTimeout:      30 * time.Second,
		BootstrapNodes:          bootNodes,
		ProgramName:             "Geth/v1.15.11/linux-amd64/go1.24.2",
	}
}

func newHoleskyConfig() EthConfig {
	td, ok := new(big.Int).SetString("1", 16)
	if !ok {
		panic("could not load Holesky configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://ac906289e4b7f12df423d654c5a962b6ebe5b3a74cc9e06292a85221f9a64a6f1cfdd6b714ed6dacef51578f92b34c60ee91e9ede9c7f8fadc4d347326d95e2b@146.190.13.128:30303",
		"enode://a3435a0155a3e837c02f5e7f5662a2f1fbc25b48e4dc232016e1c51b544cb5b4510ef633ea3278c0e970fa8ad8141e2d4d0f9f95456c537ff05fdf9b31c15072@178.128.136.233:30303"})
	if err != nil {
		panic("could not set Holesky bootstrapNodes")
	}

	ttd, _ := big.NewInt(0).SetString("0", 0)

	return EthConfig{
		Network:                 HoleskyChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		Head:                    common.HexToHash("0xb5f7f912443c940f21fd611f12828d75b534364ed9e95ca4e307729a4661bde4"),
		Genesis:                 common.HexToHash("0xb5f7f912443c940f21fd611f12828d75b534364ed9e95ca4e307729a4661bde4"),
		GenesisTime:             1695902400,
		IgnoreBlockTimeout:      30 * time.Second,
		BootstrapNodes:          bootNodes,
		IgnoreSlotCount:         10,
		ProgramName:             "Geth/v1.15.11/linux-amd64/go1.24.2",
	}
}

func unknownConfig() EthConfig {
	return EthConfig{
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
	}
}

func bootstrapNodes(validSchemes enr.IdentityScheme, nodes []string) ([]*enode.Node, error) {
	var enodes []*enode.Node
	for _, n := range nodes {
		node, err := enode.Parse(validSchemes, n)
		if err != nil {
			return nil, err
		}

		enodes = append(enodes, node)
	}

	return enodes, nil
}
