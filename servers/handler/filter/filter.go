package filter

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/zhouzhuojie/conditions"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const txFromFilter = "from"

var (
	operators        = []string{"=", ">", "<", "!=", ">=", "<=", "in"}
	operands         = []string{"and", "or"}
	availableFilters = []string{"gas", "gas_price", "value", "to", "from", "method_id", "type", "chain_id", "max_fee_per_gas", "max_priority_fee_per_gas", "max_fee_per_blob_gas", "dapp_address"}
)

// IsFiltersSupportedByTxType This function is used to skip the evaluation of txs which are not supported by the filters.
// for example, if the client requested to filter by gas_price we will not evaluate dynamic fee txs, cause the
// evaluation result will not be correct, or cause the error.
// It helps us to proceed only to evaluation part with complex filters, like this one:
// (type == '0' && gas_price > 21000000000) || (type == '2' && max_priority_fee_per_gas > 21000000000)
//
// TODO: to get rid of this stuff, we need to move to another expression evaluation library (or forking existing one)
// to support short circuit behavior for the logical operators.
func IsFiltersSupportedByTxType(txType uint8, filters []string) bool {
	gasPriceExists := utils.Exists("gas_price", filters)
	maxFeePerGasExists := utils.Exists("max_fee_per_gas", filters)
	maxPriorityFeePerGasExists := utils.Exists("max_priority_fee_per_gas", filters)
	maxFeePerBlobGasExists := utils.Exists("max_fee_per_blob_gas", filters)

	switch txType {
	case ethtypes.BlobTxType:
		if gasPriceExists && !maxFeePerGasExists && !maxPriorityFeePerGasExists && !maxFeePerBlobGasExists {
			return false
		}
	case ethtypes.DynamicFeeTxType, ethtypes.SetCodeTxType:
		if gasPriceExists && !maxFeePerGasExists && !maxPriorityFeePerGasExists {
			return false
		}
	case ethtypes.AccessListTxType, ethtypes.LegacyTxType:
		if !gasPriceExists && (maxFeePerGasExists || maxPriorityFeePerGasExists) {
			return false
		}
	}

	return true
}

// ValidateFilters validate filters from request
func ValidateFilters(filters string, txFromFieldIncludable bool) (conditions.Expr, error) {
	_, expr, err := parseFilter(filters)
	if err != nil {
		return nil, fmt.Errorf("error parsing Filters: %v", err)
	}
	if expr == nil {
		return nil, nil
	}

	err = evaluateFilters(expr)
	if err != nil {
		return nil, fmt.Errorf("error evaluated Filters: %v", err)
	}

	if !txFromFieldIncludable && utils.Exists(txFromFilter, expr.Args()) {
		return nil, fmt.Errorf("error creating Filters: error evaluated Filters: argument: %s not found", txFromFilter)
	}

	log.Infof("GetTxContentAndFilters string - %s, GetTxContentAndFilters args - %s", expr, expr.Args())
	return expr, nil
}

// evaluateFilters - evaluating if the Filters provided by the user are ok
func evaluateFilters(expr conditions.Expr) error {
	// Evaluate if we should send the tx
	_, err := conditions.Evaluate(expr, types.EmptyFilteredTransactionMap)
	return err
}

// IsCorrectGasPriceFilters check if the gas price filters provided by the user are correct.
// Needed because of the way we are parsing the filters. All gas price filters configured
// at once without a type filter will not cause the error to be thrown.
//
// TODO: to get rid of this stuff, we need to move to another expression evaluation library (or forking existing one)
// to support short circuit behavior for the logical operators.
func IsCorrectGasPriceFilters(filters []string) bool {
	gasPriceExists := utils.Exists("gas_price", filters)
	maxFeePerGasExists := utils.Exists("max_fee_per_gas", filters)
	maxPriorityFeePerGasExists := utils.Exists("max_priority_fee_per_gas", filters)
	maxFeePerBlobGas := utils.Exists("max_fee_per_blob_gas", filters)
	txType := utils.Exists("type", filters)

	if gasPriceExists && (maxFeePerGasExists || maxPriorityFeePerGasExists || maxFeePerBlobGas) && !txType {
		return false
	}
	return true
}

// parseFilter parsing the filter
func parseFilter(filters string) (string, conditions.Expr, error) {
	// if the filters values are go-type filters, for example: {value}, parse the filters
	// if not go-type, convert it to go-type filters
	if strings.Contains(filters, "{") {
		p := conditions.NewParser(strings.NewReader(strings.ToLower(strings.Replace(filters, "'", "\"", -1))))
		expr, err := p.Parse()
		if err == nil {
			isEmptyValue := filtersHasEmptyValue(expr.String())
			if isEmptyValue != nil {
				return "", nil, errors.New("filter is empty")
			}
			if !IsCorrectGasPriceFilters(expr.Args()) {
				return "", nil, errors.New("invalid filters, add the type filter")
			}
		}
		return "", expr, err
	}

	// convert the string and add whitespace to separate elements
	tempFilters := strings.ReplaceAll(filters, "(", " ( ")
	tempFilters = strings.ReplaceAll(tempFilters, ")", " ) ")
	tempFilters = strings.ReplaceAll(tempFilters, "[", " [ ")
	tempFilters = strings.ReplaceAll(tempFilters, "]", " ] ")
	tempFilters = strings.ReplaceAll(tempFilters, ",", " , ")
	tempFilters = strings.ReplaceAll(tempFilters, "=", " = ")
	tempFilters = strings.ReplaceAll(tempFilters, "<", " < ")
	tempFilters = strings.ReplaceAll(tempFilters, ">", " > ")
	tempFilters = strings.ReplaceAll(tempFilters, "!", " ! ")
	tempFilters = strings.ReplaceAll(tempFilters, ",", " , ")
	tempFilters = strings.ReplaceAll(tempFilters, "<  =", "<=")
	tempFilters = strings.ReplaceAll(tempFilters, ">  =", ">=")
	tempFilters = strings.ReplaceAll(tempFilters, "!  =", "!=")
	tempFilters = strings.Trim(tempFilters, " ")
	tempFilters = strings.ToLower(tempFilters)
	filtersArr := strings.Split(tempFilters, " ")

	var newFilterString strings.Builder
	for _, elem := range filtersArr {
		switch {
		case elem == "":
		case elem == "(", elem == ")", elem == ",", elem == "]", elem == "[":
			newFilterString.WriteString(elem)
		case utils.Exists(elem, operators):
			newFilterString.WriteString(" ")
			if elem == "=" {
				newFilterString.WriteString(elem)
			}
			newFilterString.WriteString(elem + " ")
		case utils.Exists(elem, operands):
			newFilterString.WriteString(")")
			newFilterString.WriteString(" " + elem + " ")
		case utils.Exists(elem, availableFilters):
			newFilterString.WriteString("({" + elem + "}")
		default:
			isString := false
			if _, err := strconv.Atoi(elem); err != nil {
				isString = true
			}
			switch {
			case isString && len(elem) >= 2 && elem[0:2] != "0x":
				newFilterString.WriteString("'0x" + elem + "'")
			case isString && len(elem) >= 2 && elem[0:2] == "0x":
				newFilterString.WriteString("'" + elem + "'")
			default:
				newFilterString.WriteString(elem)
			}
		}
	}

	newFilterString.WriteString(")")

	p := conditions.NewParser(strings.NewReader(strings.ToLower(strings.Replace(newFilterString.String(), "'", "\"", -1))))
	expr, err := p.Parse()
	if err != nil {
		return "", nil, err
	}

	err = filtersHasEmptyValue(expr.String())
	if err != nil {
		return "", nil, err
	}

	return newFilterString.String(), expr, nil
}

var rex = regexp.MustCompile(`\(([^)]+)\)`)

func filtersHasEmptyValue(rawFilters string) error {
	out := rex.FindAllStringSubmatch(rawFilters, -1)
	for _, i := range out {
		for _, filter := range availableFilters {
			if i[1] == filter || filter == rawFilters {
				return fmt.Errorf("filter is empty: %v", i[1])
			}
		}
	}
	return nil
}
