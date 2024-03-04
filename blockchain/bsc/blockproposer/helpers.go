package blockproposer

import (
	"math/big"
	"time"
)

// getBlockNumberToProposeFor returns the block number to propose for
func getBlockNumberToProposeFor(number *big.Int) *big.Int {
	return new(big.Int).Add(number, big.NewInt(1))
}

// timeFromTimestamp returns the time from the timestamp
func timeFromTimestamp(ts uint64) time.Time {
	return time.Unix(int64(ts), 0).UTC()
}

// compareBlockReward compares x and y and returns:
//
// | -1 if x <  y
//
// |  0 if x == y
//
// | +1 if x >  y
func compareBlockReward(x, y *big.Int) int {
	switch {
	case x == nil && y == nil:
		return 0
	case x == nil:
		return -1
	case y == nil:
		return 1
	default:
		return x.Cmp(y)
	}
}
