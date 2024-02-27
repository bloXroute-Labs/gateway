package blockproposer

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_getBlockNumberToProposeFor(t *testing.T) {
	blockNumber := big.NewInt(12345678)
	blockNumberToProposeFor := getBlockNumberToProposeFor(blockNumber)

	require.Equal(t, blockNumber.Int64()+1, blockNumberToProposeFor.Int64())
}

func Test_timeFromTimestamp(t *testing.T) {
	nowTs := uint64(1707147163)
	gotTs := uint64(timeFromTimestamp(nowTs).Unix())

	require.Equal(t, strconv.FormatUint(nowTs, 10), strconv.FormatUint(gotTs, 10))
}

func Test_compareBlockReward(t *testing.T) {
	x := big.NewInt(12345678)
	y := big.NewInt(23456789)

	require.Equal(t, 0, compareBlockReward(nil, nil))
	require.Equal(t, 0, compareBlockReward(x, x))
	require.Equal(t, 0, compareBlockReward(y, y))
	require.Equal(t, -1, compareBlockReward(x, y))
	require.Equal(t, -1, compareBlockReward(nil, y))
	require.Equal(t, 1, compareBlockReward(y, x))
	require.Equal(t, 1, compareBlockReward(y, nil))
}
