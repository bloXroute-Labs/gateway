package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
)

func BenchmarkBlockBDNtoBlockchain(b *testing.B) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(100)
	bxBlock, err := c.BlockBlockchainToBDN(core.NewBlockInfo(block, td))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := c.BlockBDNtoBlockchain(bxBlock); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockBlockchainToBDN(b *testing.B) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(100)
	blockInfo := core.NewBlockInfo(block, td)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := c.BlockBlockchainToBDN(blockInfo); err != nil {
			b.Fatal(err)
		}
	}
}
