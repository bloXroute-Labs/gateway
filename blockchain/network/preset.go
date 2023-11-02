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

// ZhejiangChainID ethereum zhejiang (shanghai hard fork) chain ID
const ZhejiangChainID = 1337803

// GoerliChainID ethereum Goerli chain ID
const GoerliChainID = 5

// BSCMainnetChainID BSC mainnet chain ID
const BSCMainnetChainID = 56

// BSCTestnetChainID BSC testnet chain ID
const BSCTestnetChainID = 97

// PolygonMainnetChainID Polygon mainnet chain ID
const PolygonMainnetChainID = 137

var networkMapping = map[string]EthConfig{
	"Mainnet":         newEthereumMainnetConfig(),
	"BSC-Mainnet":     newBSCMainnetConfig(),
	"BSC-Testnet":     newBSCTestnetConfig(),
	"Polygon-Mainnet": newPolygonMainnetConfig(),
	"Zhejiang":        newZhejiangEthereumConfig(),
	"Goerli":          newGoerliConfig(),
}

func newGoerliConfig() EthConfig {
	td, ok := new(big.Int).SetString("10790000", 16)
	if !ok {
		panic("could not load Ethereum Mainnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://011f758e6552d105183b1761c5e2dea0111bc20fd5f6422bc7f91e0fabbec9a6595caf6239b37feb773dddd3f87240d99d859431891e4a642cf2a0a9e6cbb98a@51.141.78.53:30303",
		"enode://176b9417f511d05b6b2cf3e34b756cf0a7096b3094572a8f6ef4cdcb9d1f9d00683bf0f83347eebdf3b81c3521c2332086d9592802230bf528eaf606a1d9677b@13.93.54.137:30303",
		"enode://46add44b9f13965f7b9875ac6b85f016f341012d84f975377573800a863526f4da19ae2c620ec73d11591fa9510e992ecc03ad0751f53cc02f7c7ed6d55c7291@94.237.54.114:30313",
		"enode://b5948a2d3e9d486c4d75bf32713221c2bd6cf86463302339299bd227dc2e276cd5a1c7ca4f43a0e9122fe9af884efed563bd2a1fd28661f3b5f5ad7bf1de5949@18.218.250.66:30303",
	})
	if err != nil {
		panic("could not set Ethereum Mainnet bootstrapNodes")
	}

	ttd, _ := big.NewInt(0).SetString("10790000", 0)

	return EthConfig{
		Network:                 GoerliChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		GenesisTime:             1616508000,
		Head:                    common.HexToHash("0xbf7e331f7f7c1dd2e05159666b3bf8bc7a8a3a9eb1d518969eab529dd9b88c1a"),
		Genesis:                 common.HexToHash("0xbf7e331f7f7c1dd2e05159666b3bf8bc7a8a3a9eb1d518969eab529dd9b88c1a"),
		IgnoreBlockTimeout:      3 * time.Minute,
		IgnoreSlotCount:         10,
		BootstrapNodes:          bootNodes,
		ProgramName:             "Geth/v1.11.2-stable-67109427/linux-amd64/go1.18.4",
	}
}

func newZhejiangEthereumConfig() EthConfig {
	td, ok := new(big.Int).SetString("0400000000", 16) // todo: ?
	if !ok {
		panic("could not load Ethereum Mainnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{ // todo: ?
		"enode://f20741af2743d83e03d30c49801b19b0103db81667cd8482a428cb1807a790f78a6ed2045ac45694e84803c94b6e8e63a4fc151f7c676cace3d38fcd94e52c57@167.99.209.68:30303",
	})
	if err != nil {
		panic("could not set Ethereum Mainnet bootstrapNodes")
	}

	ttd, _ := big.NewInt(0).SetString("0", 0) // not changed

	return EthConfig{
		Network:                 ZhejiangChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		GenesisTime:             1680523260, // cfg.MinGenesisTime = 1680523200 + cfg.GenesisDelay = 60
		Head:                    common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
		Genesis:                 common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
		IgnoreBlockTimeout:      150 * time.Second,
		IgnoreSlotCount:         10,
		BootstrapNodes:          bootNodes,
		ProgramName:             "Geth/v1.10.21-stable-67109427/linux-amd64/go1.18.4",
	}
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
		ProgramName:             "Geth/v1.10.21-stable-67109427/linux-amd64/go1.18.4",
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
		ProgramName:             "Geth/v1.1.11-6073dbdf-20220626/linux-amd64/go1.18.4",
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
		ProgramName:             "Geth/v1.2.9-34b065ae-20230721/linux-amd64/go1.19.11",
	}
}

func newPolygonMainnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load Polygon Mainnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://0cb82b395094ee4a2915e9714894627de9ed8498fb881cec6db7c65e8b9a5bd7f2f25cc84e71e89d0947e51c76e85d0847de848c7782b13c0255247a6758178c@44.232.55.71:30303",
		"enode://88116f4295f5a31538ae409e4d44ad40d22e44ee9342869e7d68bdec55b0f83c1530355ce8b41fbec0928a7d75a5745d528450d30aec92066ab6ba1ee351d710@159.203.9.164:30303",
	})
	if err != nil {
		panic("could not set Polygon Mainnet bootstrapNodes")
	}

	return EthConfig{
		Network:                 137,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
		Head:                    common.HexToHash("0xa9c28ce2141b56c474f1dc504bee9b01eb1bd7d1a507580d5519d4437a97de1b"),
		Genesis:                 common.HexToHash("0xa9c28ce2141b56c474f1dc504bee9b01eb1bd7d1a507580d5519d4437a97de1b"),
		IgnoreBlockTimeout:      30 * time.Second,
		BootstrapNodes:          bootNodes,
		ProgramName:             "bor/v0.2.16-stable-f083705e/linux-amd64/go1.18.4",
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
