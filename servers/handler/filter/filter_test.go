package filter

import (
	"encoding/hex"
	"testing"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

var (
	pythonFilters = []string{
		"value<=10000",
		"value<= 10000",
		"value!=10000",
		"value <= 10000",
		"value >= 10000",
		"value != 10000",
		"value > 1000000000000000000 and value < 4000000000000000000",
		"value > 1000000000000000000 AND value < 4000000000000000000",
		"( ( value > 1000000000000000000 ) and value < 4000000000000000000)",
		"( (value > 1000000000000000000 ) and value < 4000000000000000000)",
		"to = 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
		"from in [0x6671799F031059e017bBc9E9FCbE6721cc2Bd798, 0x09eDBC6ed492C6D4274810E257A690a11d71ce43]",
		"gas_price > 183000000000",
		"gas_price> 100000000000",
		"method_id != aa",
		"method_id = a9059cbb",
		"method_id= a9059cbb",
		"chain_id = 1",
		"max_fee_per_gas = 1",
		"max_priority_fee_per_gas = 1",
		"from in[0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f]",
		"from in [0xaa, 0xbb,0xcc, 0xdd]",
		"to in [0xaa,0xbb,0xcc,0xdd]",
		"method_id in [aa, bb,cc, dd]",
		"method_id in [aa, bb, cc,dd]",
		"method_id IN [aa, bb, cc,dd]",
		"method_id IN[aa, bb, cc,dd]",
		"from in [0xaa, 0xbb,0xcc, 0xdd] and value < 4000000000000000000 and to in [0xaa,0xbb, 0xcc, 0xdd]",
		"from in [0xaa, 0xbb, 0xcc, 0xdd] and to in [0xaa, 0xbb, 0xcc, 0xdd]",
		"from in [0xaa, 0xbb,0xcc, 0xdd] and to in [0xaa,0xbb, 0xcc, 0xdd]",
		"from = 0xaa and ((value > 1000 or value < 500) and method_id in [aa, bb, cc] and (to = 0xabb or gas_price = 5))",
		"from = 0xaa and ((((value > 1000 or value < 500) and method_id in [aa, bb, cc] and (to = 0xabb or gas_price = 5))))",
		"from = 0xaa and value > 1000 or value < 500 and (method_id in [aa, bb, cc] and (to = 0xabb or gas_price = 5))",
		"to = 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2 and ((value > 1000000000000000000 and value < 4000000000000000000) or from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f, 0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288])",
		"method_id = a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f,0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288] and gas_price > 100000000000",
		"method_id = a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f,   0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288,0x77e2b7268911954c16b37fbcf1b0b1d395a0e288] and gas_price > 100000000000",
		"method_id = a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f] and gas_price > 100000000000",
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (type == '0' && gas_price > 21000000000) || (type == '2' && max_priority_fee_per_gas > 21000000000)",
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (type == '3') || (type == '2' && max_priority_fee_per_gas > 21000000000)",
		// max_priority_fee_per_gas used without the type, but it's || with the type
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (type == '3') || (max_priority_fee_per_gas > 21000000000)",
		// gas_price used without the type, but it's || with the type
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (gas_price > 21000000000) || (type == '2' && max_priority_fee_per_gas > 21000000000)",
		// max_priority_fee_per_gas used with the type, but it's || with the type
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (type == '0' && gas_price > 21000000000) || (max_priority_fee_per_gas > 21000000000)",
	}

	goFiltersValues = []string{
		"({value} <= 10000)",
		"({value} <= 10000)",
		"({value} != 10000)",
		"({value} <= 10000)",
		"({value} >= 10000)",
		"({value} != 10000)",
		"({value} > 1000000000000000000) and ({value} < 4000000000000000000)",
		"((({value} > 1000000000000000000)) and ({value} < 4000000000000000000))",
		"((({value} > 1000000000000000000)) and ({value} < 4000000000000000000))",
		"({value} > 1000000000000000000) AND ({value} < 2000000000000000000)",
		"({value} > 1000000000000000000)AND({value} < 2000000000000000000)",
		"({to} == '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2')",
		"({from} in ['0x6671799F031059e017bBc9E9FCbE6721cc2Bd798','0x09eDBC6ed492C6D4274810E257A690a11d71ce43'])",
		"({gas_price} > 183000000000)",
		"({gas_price} > 100000000000)",
		"({method_id} != '0xaa')",
		"({method_id} == '0xa9059cbb')",
		"({method_id} == '0xa9059cbb')",
		"({chain_id} == 1)",
		"({max_fee_per_gas} == 1)",
		"({max_priority_fee_per_gas} == 1)",
		"({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f'])",
		"({from} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({to} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({method_id} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({method_id} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({method_id} IN ['0xaa','0xbb','0xcc','0xdd'])",
		"({from} in ['0xaa','0xbb','0xcc','0xdd']) and ({value} < 4000000000000000000) and ({to} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({from} in ['0xaa','0xbb','0xcc','0xdd']) and ({to} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({from} in ['0xaa','0xbb','0xcc','0xdd']) and ({to} in ['0xaa','0xbb','0xcc','0xdd'])",
		"({from} == '0xaa') and ((({value} > 1000) or ({value} < 500)) and ({method_id} in ['0xaa','0xbb','0xcc']) and (({to} == '0xabb') or ({gas_price} == 5)))",
		"({from} == '0xaa') and ((((({value} > 1000) or ({value} < 500)) and ({method_id} in ['0xaa','0xbb','0xcc']) and (({to} == '0xabb') or ({gas_price} == 5)))))",
		"({from} == '0xaa') and ({value} > 1000) or ({value} < 500) and (({method_id} in ['0xaa','0xbb','0xcc']) and (({to} == '0xabb') or ({gas_price} == 5)))",
		"({to} == '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2') and ((({value} > 1000000000000000000) and ({value} < 4000000000000000000)) or ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f','0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288']))",
		"({method_id} == '0xa9059cbb') and ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f','0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288']) and ({gas_price} > 100000000000)",
		"({method_id} == '0xa9059cbb') and ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f','0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288','0x77e2b7268911954c16b37fbcf1b0b1d395a0e288']) and ({gas_price} > 100000000000)",
		"({method_id} == '0xa9059cbb') and ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f']) and ({gas_price} > 100000000000)",
	}

	invalidFilters = []string{
		"from",        // empty filter
		"value in []", // empty array
		"from in [0xaa, 0xbb,0xcc, 0xdd] and value < 4000000000000000000 and to in []", // empty array
		"from value",
		"hello = 1000",
		"(from = (0xaa",
		"(from = 0xaa",
		"from = (0xaa and to = ) 1000",
		"from = (0xaa and to = ) 1000)",
		"value ! =  10000",
		"value > = 10000",
		"value < = 10000",
		"value < = 10000 and gas_price != 1500",
		"gas_price != 1500 and value < = 10000",
		"gas_price => 100000000000",
		"gas_price =< 100000000000",
		"gas_price != 1500 and gas_price =< 100000000000",
		"method_id = (a9059cbb and from in ([0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f,0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288] and gas_price > 100000000000)",
		"method_id != a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f,   0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288,0x77e2b7268911954c16b37fbcf1b0b1d395a0e288] and gas_price => 100000000000",
		// gas_price used with invalid type
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (type == '3' && gas_price > 21000000000) || (type == '2' && max_priority_fee_per_gas > 21000000000)",
		// max_priority_fee_per_gas used with invalid type
		"from in [0xaa, 0xbb,0xcc, 0xdd] && (type == '0' && gas_price > 21000000000) && (max_priority_fee_per_gas > 21000000000)",
	}
)

func TestValidateFilters(t *testing.T) {
	log.SetLevel(log.ErrorLevel)

	for _, filter := range pythonFilters {
		_, err := NewDefaultExpression(filter, true)
		require.NoError(t, err, filter)
	}
	for _, filter := range goFiltersValues {
		_, err := NewDefaultExpression(filter, true)
		require.NoError(t, err, filter)
	}
	for _, filter := range invalidFilters {
		_, err := NewDefaultExpression(filter, true)
		require.Error(t, err, "expected error for filter: %s", filter)
	}
}

func TestNoFilter(t *testing.T) {
	log.SetLevel(log.ErrorLevel)

	filter := "from = 0xaa"

	expression, err := NewDefaultExpression(filter, true)
	require.NoError(t, err, "failed to create and validate filter: %s", filter)

	_, err = expression.Evaluate(nil)
	require.Error(t, err)
}

func TestFilterTransaction(t *testing.T) {
	log.SetLevel(log.ErrorLevel)

	hashString := fixtures.LegacyTransactionHash
	txString := fixtures.LegacyTransaction

	hash, err := types.NewSHA256HashFromString(hashString)
	require.NoError(t, err)

	content, err := hex.DecodeString(txString)
	require.NoError(t, err)

	tx := types.NewBxTransaction(hash, bxtypes.NetworkNum(5), types.TFPaidTx, time.Now())
	tx.SetContent(content)

	blockchainTx, err := tx.MakeAndSetEthTransaction(types.EmptySender)
	require.NoError(t, err)

	filters := blockchainTx.Filters()

	var testCases = []struct {
		filter   string
		expected bool
	}{
		{
			filter:   "from = 0xaa",
			expected: false, // 'from' is not allowed in this context
		},
		{
			filter:   "to = 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2",
			expected: false, // 'to' does not match the transaction's 'to' field
		},
		{
			filter:   "value > 100000000000",
			expected: true, // 'value' matches the transaction's value
		},
		{
			filter:   "type == '0' and gas_price > 21000000000",
			expected: true, // 'type' and 'gas_price' match the transaction's filters
		},
		{
			filter:   "value < 100000000000",
			expected: false, // 'value' does not match the transaction's value
		},
		{
			filter:   "({to} IN ['0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D','0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48'])",
			expected: true, // case-insensitive match for 'to' address
		},
		{
			filter:   "({to} IN ['0x7a250d5630b4cf539739df2c5dacb4c659f2488d','0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48'])",
			expected: true, // case-insensitive match for 'to' address
		},
		{
			filter:   "to IN [0x7a250d5630b4cf539739df2c5dacb4c659f2488d,0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48]",
			expected: true, // case-insensitive match for 'to' address, non-go syntax
		},
		{
			filter:   "to IN [0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48,0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f]",
			expected: false,
		},
	}

	for _, tc := range testCases {
		expression, err := NewDefaultExpression(tc.filter, true)
		require.NoError(t, err, "failed to create and validate filter: %s", tc.filter)

		result, err := expression.Evaluate(filters)
		require.NoError(t, err, "failed to evaluate expression: %s", tc.filter)
		assert.Equal(t, tc.expected, result)
	}
}
