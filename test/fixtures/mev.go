package fixtures

var mevParamsWithControlDigit =
// Params
"7b2274657374223a2274657374227d" +
	// Control digit
	"01"

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
	mevParamsWithControlDigit

// MEVSearcherPayload valid payload for MEVSearcher message
var MEVSearcherPayload =
// Header
"fffefdfc6d65767365617263686572006c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
	// mev miner method length
	"0f00" +
	// mev miner method
	// 	"6574685f73656e644d65676162756e646c65" +
	"6574685f73656e644d657373616765" +
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

// MEVSearcherPayloadWithUUID valid payload for MEVSearcher message with uuid
var MEVSearcherPayloadWithUUID =
// Header
"fffefdfc6d65767365617263686572006c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
	// mev miner method length
	"0f00" +
	// mev miner method
	"6574685f73656e644d657373616765" +
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
	// uuid
	"c40df8ec844d4887812927bb80812680" +
	mevParamsWithControlDigit

// MEVSearcherPayloadWithEmptyUUID valid payload for MEVSearcher message with empty uuid
var MEVSearcherPayloadWithEmptyUUID =
// Header
"fffefdfc6d65767365617263686572006c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
	// mev miner method length
	"0f00" +
	// mev miner method
	"6574685f73656e644d657373616765" +
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
	// uuid
	"00000000000000000000000000000000" +
	mevParamsWithControlDigit

// MEVSearcherPayloadWithMEVMaxProfitBuilder valid payload for MEVSearcher message with maxProfitBuilder
var MEVSearcherPayloadWithMEVMaxProfitBuilder =
// Header
"fffefdfc6d65767365617263686572006c00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000" +
	// mev miner method length
	"0f00" +
	// mev miner method
	"6574685f73656e644d657373616765" +
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
	// uuid
	"00000000000000000000000000000000" +
	// Frontrunning
	"01" +
	// EffectiveGasPriceLen
	"0900" +
	// EffectiveGasPrice
	"0a6f840a934d058000" +
	// CoinbaseProfitLen
	"0900" +
	// CoinbaseProfit
	"0a6f840a934d058000" +
	mevParamsWithControlDigit
