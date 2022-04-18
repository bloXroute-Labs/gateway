package fixtures

// MEVBundlePayload valid payload for mev bundle
var MEVBundlePayload =
// Header
"fffefdfc6d657662756e646c650000005b000000fbed5e7084704a92ca28bb682843100a374707c5b25f83a15a674e3b0aa39e130000000000000000000000000000000000000000" +
	// mev miner method length
	"0E00" +
	// mev miner method
	"6574685f73656e6442756e646c65" +
	// number of mev miners
	"02" +
	// mev miner name length
	"0b00" +
	// mev miner name
	"74657374206d696e657231" +
	// mev miner name 2 length
	"0b00" +
	// mev miner name 2
	"74657374206d696e657232" +
	// Params
	"7b2274657374223a2274657374227d" +
	// Control digit
	"01"

// MEVSearcherPayload valid payload for MEVSearcher message
var MEVSearcherPayload =
// Header
"fffefdfc6d65767365617263686572005800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
	// mev miner method length
	"1200" +
	// mev miner method
	"6574685f73656e644d65676162756e646c65" +
	// mev builders
	"01" +
	// Miner name length
	"0900" +
	// Miner name
	"6e616d652074657374" +
	// Miner auth length
	"0900" +
	// Miner auth
	"617574682074657374" +
	// Params
	"7b2274657374223a2274657374227d" +
	// Control digit
	"01"
