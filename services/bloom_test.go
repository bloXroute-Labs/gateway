package services

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

const (
	bscMainnetBloomCap     = 35e6
	mainnetBloomCap        = 85e5
	polygonMainnetBloomCap = 225e5
)

func TestBloomStore(t *testing.T) {
	os.Remove(path.Join(bloomFilterDirName, currentBloomFileName))
	os.Remove(path.Join(bloomFilterDirName, previousBloomFileName))
	os.Remove(path.Join(bloomFilterDirName, counterBloomFileName))

	mockClock := utils.MockClock{}

	ctx := context.Background()
	bf, _ := NewBloomFilter(
		ctx, &mockClock, 1*time.Hour, "",
		bscMainnetBloomCap+mainnetBloomCap+polygonMainnetBloomCap)

	// fill the bloom
	for i := 0; i <= mainnetBloomCap/10000; i++ {
		iString := fmt.Sprintf("%v", i)
		bf.Add([]byte(iString))
	}

	mockClock.IncTime(1 * time.Hour)
	ctx.Done()
	// wait for the storeOnDisk to complete
	time.Sleep(1 * time.Second)

	// read the bloom filter
	bf2, err := NewBloomFilter(
		ctx, &mockClock, 1*time.Hour, "",
		bscMainnetBloomCap+mainnetBloomCap+polygonMainnetBloomCap)
	ctx.Done()
	assert.Nil(t, err)
	os.Remove(path.Join(bloomFilterDirName, currentBloomFileName))
	os.Remove(path.Join(bloomFilterDirName, previousBloomFileName))
	os.Remove(path.Join(bloomFilterDirName, counterBloomFileName))
	bfTest := bf.(*bloomFilter)
	bf2Test := bf2.(*bloomFilter)

	assert.Equal(t, len(bfTest.current.BitSet().Bytes()), len(bf2Test.current.BitSet().Bytes()))
	assert.Equal(t, cap(bfTest.current.BitSet().Bytes()), cap(bf2Test.current.BitSet().Bytes()))
}
