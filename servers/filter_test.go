package servers

import (
	"strings"
	"testing"

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
	s := &subscriptionOptions{}

	for pythonFormat, expectedGoFormat := range pythonFiltersToGoFilters {
		s.Filters = pythonFormat
		goFormatResult, exp, err := ParseFilter(s.Filters)
		assert.Equal(t, strings.ToLower(expectedGoFormat), strings.ToLower(goFormatResult))
		assert.Nil(t, err)
		assert.Nil(t, EvaluateFilters(exp))
	}

	for _, invalidFilters := range invalidPythonFilters {
		s.Filters = invalidFilters
		_, _, err := ParseFilter(s.Filters)
		assert.NotNil(t, err)
	}
}
