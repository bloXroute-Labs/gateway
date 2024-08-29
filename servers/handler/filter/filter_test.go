package filter

import (
	"strings"
	"testing"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// pythonFiltersToGoFilters - contains available filters in python format and theirs go format filters
var pythonFiltersToGoFilters = map[string]string{
	// {value}
	"value<=10000":   "({value} <= 10000)",
	"value<= 10000":  "({value} <= 10000)",
	"value!=10000":   "({value} != 10000)",
	"value <= 10000": "({value} <= 10000)",
	"value >= 10000": "({value} >= 10000)",
	"value != 10000": "({value} != 10000)",
	"value > 1000000000000000000 and value < 4000000000000000000":        "({value} > 1000000000000000000) and ({value} < 4000000000000000000)",
	"( ( value > 1000000000000000000 ) and value < 4000000000000000000)": "((({value} > 1000000000000000000)) and ({value} < 4000000000000000000))",
	"( (value > 1000000000000000000 ) and value < 4000000000000000000)":  "((({value} > 1000000000000000000)) and ({value} < 4000000000000000000))",
	// {to}
	"to = 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2":                                                  "({to} == '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2')",
	"from in [0x6671799F031059e017bBc9E9FCbE6721cc2Bd798, 0x09eDBC6ed492C6D4274810E257A690a11d71ce43]": "({from} in ['0x6671799F031059e017bBc9E9FCbE6721cc2Bd798','0x09eDBC6ed492C6D4274810E257A690a11d71ce43'])",
	// {gas_price}
	"gas_price > 183000000000": "({gas_price} > 183000000000)",
	"gas_price> 100000000000":  "({gas_price} > 100000000000)",
	// {method_id}
	"method_id != aa":      "({method_id} != '0xaa')",
	"method_id = a9059cbb": "({method_id} == '0xa9059cbb')",
	"method_id= a9059cbb":  "({method_id} == '0xa9059cbb')",
	// {chain_id}
	"chain_id = 1": "({chain_id} == 1)",
	// {max_fee_per_gas}
	"max_fee_per_gas = 1": "({max_fee_per_gas} == 1)",
	// {max_fee_per_gas}
	"max_priority_fee_per_gas = 1": "({max_priority_fee_per_gas} == 1)",
	// address list with or without white spaces
	"from in[0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f]": "({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f'])",
	"from in [0xaa, 0xbb,0xcc, 0xdd]":                     "({from} in ['0xaa','0xbb','0xcc','0xdd'])",
	"to in [0xaa,0xbb,0xcc,0xdd]":                         "({to} in ['0xaa','0xbb','0xcc','0xdd'])",
	"method_id in [aa, bb,cc, dd]":                        "({method_id} in ['0xaa','0xbb','0xcc','0xdd'])",
	"method_id in [aa, bb, cc,dd]":                        "({method_id} in ['0xaa','0xbb','0xcc','0xdd'])",
	"from in [0xaa, 0xbb,0xcc, 0xdd] and value < 4000000000000000000 and to in [0xaa,0xbb, 0xcc, 0xdd]": "({from} in ['0xaa','0xbb','0xcc','0xdd']) and ({value} < 4000000000000000000) and ({to} in ['0xaa','0xbb','0xcc','0xdd'])",
	"from in [0xaa, 0xbb, 0xcc, 0xdd] and to in [0xaa, 0xbb, 0xcc, 0xdd]":                               "({from} in ['0xaa','0xbb','0xcc','0xdd']) and ({to} in ['0xaa','0xbb','0xcc','0xdd'])",
	"from in [0xaa, 0xbb,0xcc, 0xdd] and to in [0xaa,0xbb, 0xcc, 0xdd]":                                 "({from} in ['0xaa','0xbb','0xcc','0xdd']) and ({to} in ['0xaa','0xbb','0xcc','0xdd'])",
	// complex filters with different number of parenthesis
	"from = 0xaa and ((value > 1000 or value < 500) and method_id in [aa, bb, cc] and (to = 0xabb or gas_price = 5))":                                                                                                         "({from} == '0xaa') and ((({value} > 1000) or ({value} < 500)) and ({method_id} in ['0xaa','0xbb','0xcc']) and (({to} == '0xabb') or ({gas_price} == 5)))",
	"from = 0xaa and ((((value > 1000 or value < 500) and method_id in [aa, bb, cc] and (to = 0xabb or gas_price = 5))))":                                                                                                     "({from} == '0xaa') and ((((({value} > 1000) or ({value} < 500)) and ({method_id} in ['0xaa','0xbb','0xcc']) and (({to} == '0xabb') or ({gas_price} == 5)))))",
	"from = 0xaa and value > 1000 or value < 500 and (method_id in [aa, bb, cc] and (to = 0xabb or gas_price = 5))":                                                                                                           "({from} == '0xaa') and ({value} > 1000) or ({value} < 500) and (({method_id} in ['0xaa','0xbb','0xcc']) and (({to} == '0xabb') or ({gas_price} == 5)))",
	"to = 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2 and ((value > 1000000000000000000 and value < 4000000000000000000) or from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f, 0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288])": "({to} == '0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2') and ((({value} > 1000000000000000000) and ({value} < 4000000000000000000)) or ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f','0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288']))",
	"method_id = a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f,0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288] and gas_price > 100000000000":                                                                   "({method_id} == '0xa9059cbb') and ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f','0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288']) and ({gas_price} > 100000000000)",
	"method_id = a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f,   0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288,0x77e2b7268911954c16b37fbcf1b0b1d395a0e288] and gas_price > 100000000000":                     "({method_id} == '0xa9059cbb') and ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f','0x77e2b72689fc954c16b37fbcf1b0b1d395a0e288','0x77e2b7268911954c16b37fbcf1b0b1d395a0e288']) and ({gas_price} > 100000000000)",
	"method_id = a9059cbb and from in [0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f] and gas_price > 100000000000":                                                                                                              "({method_id} == '0xa9059cbb') and ({from} in ['0x8fdc5df186c58cdc2c22948beee12b1ae1406c6f']) and ({gas_price} > 100000000000)",
}

// invalidPythonFilters - invalid python format filters
var invalidPythonFilters = []string{
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
}

func TestFilter(t *testing.T) {
	for pythonFormat, expectedGoFormat := range pythonFiltersToGoFilters {
		goFormatResult, exp, err := parseFilter(pythonFormat)
		assert.Equal(t, strings.ToLower(expectedGoFormat), strings.ToLower(goFormatResult))
		assert.NoError(t, err)
		assert.Nil(t, evaluateFilters(exp))
	}

	for _, invalidFilters := range invalidPythonFilters {
		_, _, err := parseFilter(invalidFilters)
		assert.NotNil(t, err)
	}
}

func TestIntentsFilter(t *testing.T) {
	pythonFormat := "dapp_address = 0x0df00"
	expectedGoFormat := "({dapp_address} == '0x0df00')"

	goFormatResult, exp, err := parseFilter(pythonFormat)
	assert.Equal(t, strings.ToLower(expectedGoFormat), strings.ToLower(goFormatResult))
	assert.NoError(t, err)
	assert.Nil(t, evaluateIntentsFilters(exp))
}

func TestIsCorrectGasPriceFilters(t *testing.T) {
	tests := []struct {
		name     string
		filters  []string
		expected bool
	}{
		{
			name:     "gas_price and max_fee_per_gas exist, txType does not",
			filters:  []string{"gas_price", "max_fee_per_gas"},
			expected: false,
		},
		{
			name:     "gas_price and max_priority_fee_per_gas exist, txType does not",
			filters:  []string{"gas_price", "max_priority_fee_per_gas"},
			expected: false,
		},
		{
			name:     "gas_price and max_priority_fee_per_gas exist, txType does not",
			filters:  []string{"gas_price", "max_priority_fee_per_gas", "type"},
			expected: true,
		},
		{
			name:     "gas_price exists, max_fee_per_gas and txType do not",
			filters:  []string{"gas_price"},
			expected: true,
		},
		{
			name:     "gas_price exists, max_priority_fee_per_gas and txType do not",
			filters:  []string{"gas_price"},
			expected: true,
		},
		{
			name:     "gas_price and txType exist, max_fee_per_gas does not",
			filters:  []string{"gas_price", "type"},
			expected: true,
		},
		{
			name:     "gas_price and txType exist, max_priority_fee_per_gas does not",
			filters:  []string{"gas_price", "type"},
			expected: true,
		},
		{
			name:     "no gas price filters",
			filters:  []string{"type"},
			expected: true,
		},
		{
			name:     "empty filters",
			filters:  []string{},
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCorrectGasPriceFilters(tt.filters); got != tt.expected {
				t.Errorf("IsCorrectGasPriceFilters() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestIsFiltersSupportedByTxType(t *testing.T) {
	tests := []struct {
		name     string
		txType   uint8
		filters  []string
		expected bool
	}{
		{
			name:     "DynamicFeeTxType with gas_price filter",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price"},
			expected: false,
		},
		{
			name:     "DynamicFeeTxType with gas_price and max_fee_per_gas filters",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price", "max_fee_per_gas"},
			expected: true,
		},
		{
			name:     "DynamicFeeTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "DynamicFeeTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price", "max_fee_per_gas", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "BlobTxType with gas_price filter",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price"},
			expected: false,
		},
		{
			name:     "BlobTxType with gas_price and max_fee_per_gas filters",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price", "max_fee_per_gas"},
			expected: true,
		},
		{
			name:     "BlobTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "BlobTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price", "max_fee_per_gas", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "Non-DynamicFeeTxType with max_fee_per_gas filter",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{"max_fee_per_gas"},
			expected: false,
		},
		{
			name:     "Non-DynamicFeeTxType with max_priority_fee_per_gas filter",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{"max_priority_fee_per_gas"},
			expected: false,
		},
		{
			name:     "Non-DynamicFeeTxType with no filters",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{},
			expected: true,
		},
		{
			name:     "Non-DynamicFeeTxType with gasPrice",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{"gas_price"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFiltersSupportedByTxType(tt.txType, tt.filters); got != tt.expected {
				t.Errorf("IsFiltersSupportedByTxType() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
