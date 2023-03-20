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

//BSCMainnetChainID BSC mainnet chain ID
const BSCMainnetChainID = 56

//PolygonMainnetChainID Polygon mainnet chain ID
const PolygonMainnetChainID = 137

var networkMapping = map[string]EthConfig{
	"Mainnet":         newEthereumMainnetConfig(),
	"BSC-Mainnet":     newBSCMainnetConfig(),
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
	td, ok := new(big.Int).SetString("0400000000", 16)
	if !ok {
		panic("could not load Ethereum Mainnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://691c66d0ce351633b2ef8b4e4ef7db9966915ca0937415bd2b408df22923f274873b4d4438929e029a13a680140223dcf701cabe22df7d8870044321022dfefa@64.225.78.1:30303",
		"enode://89347b9461727ee1849256d78e84d5c86cc3b4c6c5347650093982b726d71f3d08027e280b399b7b6604ceeda863283dcfe1a01e93728b4883114e9f8c7cc8ef@146.190.238.212:30303",
		"enode://c2892072efe247f21ed7ebea6637ade38512a0ae7c5cffa1bf0786d5e3be1e7f40ff71252a21b36aa9de54e49edbcfc6962a98032adadfa29c8524262e484ad3@165.232.84.160:30303",
		"enode://71e862580d3177a99e9837bd9e9c13c83bde63d3dba1d5cea18e89eb2a17786bbd47a8e7ae690e4d29763b55c205af13965efcaf6105d58e118a5a8ed2b0f6d0@68.183.13.170:30303",
		"enode://2f6cf7f774e4507e7c1b70815f9c0ccd6515ee1170c991ce3137002c6ba9c671af38920f5b8ab8a215b62b3b50388030548f1d826cb6c2b30c0f59472804a045@161.35.147.98:30303",
	})
	if err != nil {
		panic("could not set Ethereum Mainnet bootstrapNodes")
	}

	ttd, _ := big.NewInt(0).SetString("0", 0)

	return EthConfig{
		Network:                 ZhejiangChainID,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: ttd,
		GenesisTime:             1675263600,
		Head:                    common.HexToHash("0x40891180a021e1f86644f42721a9badb5bc8fc04036ca847c0d6fe1c440d4261"),
		Genesis:                 common.HexToHash("0x40891180a021e1f86644f42721a9badb5bc8fc04036ca847c0d6fe1c440d4261"),
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
		"enode://1cc4534b14cfe351ab740a1418ab944a234ca2f702915eadb7e558a02010cb7c5a8c295a3b56bcefa7701c07752acd5539cb13df2aab8ae2d98934d712611443@52.71.43.172:30311",
		"enode://28b1d16562dac280dacaaf45d54516b85bc6c994252a9825c5cc4e080d3e53446d05f63ba495ea7d44d6c316b54cd92b245c5c328c37da24605c4a93a0d099c4@34.246.65.14:30311",
		"enode://5a7b996048d1b0a07683a949662c87c09b55247ce774aeee10bb886892e586e3c604564393292e38ef43c023ee9981e1f8b335766ec4f0f256e57f8640b079d5@35.73.137.11:30311",
	})
	//bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
	//	"enode://45146a7cb02cd127d21cf3c37f533623c5caf4dea31aecc619d5e47ccc38f8377f08be8ce60f9c3429efaa50825c435498880a46b4634ccdc3ed8eb72ba054ae@3.209.235.131:30311",
	//	"enode://eaa9a4d781fc7f1666bb15b8cb5fd6128ce9b92ab3824294e1fde8074a646d1d94e9f8558f6111a714ff28aed0f06dfb3958212b21ac943904747719c758b4d1@100.25.250.190:30311",
	//	"enode://5a7b996048d1b0a07683a949662c87c09b55247ce774aeee10bb886892e586e3c604564393292e38ef43c023ee9981e1f8b335766ec4f0f256e57f8640b079d5@13.114.81.214:30311",
	//	"enode://20d04257749893d7193b8e3ed619d46384d28b350508bef163b52ee9dc60efc4f562aee00c7fde5cfa83e4e9723b0e90d6422d9031b6069734bd7e24a9ed8e73@107.21.209.99:30311",
	//	"enode://98bf45137866d17ac544cdfd43408d18146db32e2b70a9be8b9499ed9ce47c914f5adb4b94f4216ce0e8779b3331f3400342ae5cef721e4a6dadb9cc09e03baa@35.76.210.163:30311",
	//	"enode://37d548bc46315eb66f95bca51e5db3f77c1dbe254eca48f81d539fe87076c7a20fd8f119a27e21edba475f2edeb156742f2908e92a7eefbc2e421dc5f3812b19@54.178.99.222:30311",
	//	"enode://0fe2af9a5e6fdaa1782eb1025d516984395197699bf677a78021fa6158660f88bcfe577bb5d05c7e1b5bf8ff5a6bf60d9174437c2268bec74cb16221e37ab075@52.210.159.54:30311",
	//	"enode://f420209bac5324326c116d38d83edfa2256c4101a27cd3e7f9b8287dc8526900f4137e915df6806986b28bc79b1e66679b544a1c515a95ede86f4d809bd65dab@54.178.62.117:30311",
	//	"enode://d4a12107e316ccaf5a2d0fa95efdbc4d15e3fa7df60a38e767bba3b7d7e9d345fb8fe991fbc0319e5ed2582e21e557c60e5780af8942fce3fff78c34db4607f7@54.178.125.71:30311",
	//	"enode://82efe88c5070a94e1cb7ec218f3e916a1621eac64067c6973e3103492aebc40d8febf810e12a786e316b8f2f02f35bf93ceaca69bb1852fbf8d5f345cd75f04e@35.75.44.49:30311",
	//	"enode://514334110fb750cf9c09d048815f2a39e2d869e4040c89f9a4c74dc08dd61556e596477ca3edd82ab5b4e41d8b23eb5c0da9fc521edb81ec0e062c57c4ce9700@34.248.99.52:30311",
	//	"enode://3a1efd25c06f925e05eed5418534e033ee285abbdc898e64c6747e327abbf9369db846ad0532ce94149e9e3d35c1f9a6d700e99c9cfbc02de099988a4ab1049e@3.215.208.84:30311",
	//	"enode://2149f76dabe7c1711e3180b0b8358c55ec3b37b0b0e3e00b4dc1fe994c74d4ee7012ffc9eec768f601e0809e99b8d1650dbf0d91ffb62f6b2e809fbd3411ae2f@46.137.9.186:30311",
	//	"enode://1841077024720c251f58e6eeb10c2a3846db3610b2f4e8210e7035d0623f4ab6caef94c3bf215cb548e7c7e41d2755da33b63685de425e07aeb5cef017ea8cb5@52.51.36.24:30311",
	//})
	if err != nil {
		panic("could not set Ethereum Mainnet bootstrapNodes")
	}

	return EthConfig{
		Network:                 56,
		TotalDifficulty:         td,
		TerminalTotalDifficulty: big.NewInt(math.MaxInt),
		Head:                    common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		Genesis:                 common.HexToHash("0x0d21840abff46b96c84b2ac9e10e4f5cdaeb5693cb665db62a2f3b02d2d57b5b"),
		IgnoreBlockTimeout:      30 * time.Second,
		BootstrapNodes:          bootNodes,
		ProgramName:             "Geth/v1.1.11-6073dbdf-20220626/linux-amd64/go1.18.4",
	}
}

func newPolygonMainnetConfig() EthConfig {
	td, ok := new(big.Int).SetString("40000", 16)
	if !ok {
		panic("could not load BSC Mainnet configuration")
	}

	var err error
	var bootNodes []*enode.Node

	bootNodes, err = bootstrapNodes(enode.ValidSchemes, []string{
		"enode://0cb82b395094ee4a2915e9714894627de9ed8498fb881cec6db7c65e8b9a5bd7f2f25cc84e71e89d0947e51c76e85d0847de848c7782b13c0255247a6758178c@44.232.55.71:30303",
		"enode://88116f4295f5a31538ae409e4d44ad40d22e44ee9342869e7d68bdec55b0f83c1530355ce8b41fbec0928a7d75a5745d528450d30aec92066ab6ba1ee351d710@159.203.9.164:30303",
	})
	if err != nil {
		panic("could not set Ethereum Mainnet bootstrapNodes")
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
