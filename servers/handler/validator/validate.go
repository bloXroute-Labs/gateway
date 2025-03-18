package validator

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	txFromField = "tx_contents.from"
)

var (
	txContentFields = []string{"tx_contents.nonce", "tx_contents.tx_hash",
		"tx_contents.gas_price", "tx_contents.gas", "tx_contents.to", "tx_contents.value", "tx_contents.input",
		"tx_contents.v", "tx_contents.r", "tx_contents.s", "tx_contents.type", "tx_contents.access_list",
		"tx_contents.chain_id", "tx_contents.max_priority_fee_per_gas", "tx_contents.max_fee_per_gas",
		"tx_contents.max_fee_per_blob_gas", "tx_contents.blob_versioned_hashes", "tx_contents.y_parity", "tx_contents.authorization_list"}

	defaultTxParams = append(txContentFields, "tx_hash", "local_region", "time")

	txContentFieldsWithFrom = append(txContentFields, "tx_contents.from")

	validTxParams        = append(txContentFields, "tx_contents", "tx_contents.from", "tx_hash", "local_region", "time", "raw_tx")
	validBlockParams     = append(txContentFields, "tx_contents.from", "hash", "header", "transactions", "uncles", "future_validator_info", "withdrawals")
	validTxReceiptParams = []string{"block_hash", "block_number", "contract_address",
		"cumulative_gas_used", "effective_gas_price", "from", "gas_used", "logs", "logs_bloom",
		"status", "to", "transaction_hash", "transaction_index", "type", "txs_count", "blob_gas_used", "blob_gas_price"}
	validOnBlockParams     = []string{"name", "response", "block_height", "tag"}
	validBeaconBlockParams = []string{"hash", "header", "slot", "body"}

	validParamsMap = make(map[types.FeedType]map[string]struct{})
)

func init() {
	validParamsMap = map[types.FeedType]map[string]struct{}{
		types.NewTxsFeed:          stringSliceToSet(validTxParams),
		types.PendingTxsFeed:      stringSliceToSet(validTxParams),
		types.BDNBlocksFeed:       stringSliceToSet(validBlockParams),
		types.NewBlocksFeed:       stringSliceToSet(validBlockParams),
		types.OnBlockFeed:         stringSliceToSet(validOnBlockParams),
		types.TxReceiptsFeed:      stringSliceToSet(validTxReceiptParams),
		types.NewBeaconBlocksFeed: stringSliceToSet(validBeaconBlockParams),
		types.BDNBeaconBlocksFeed: stringSliceToSet(validBeaconBlockParams),
	}
}

// ValidateIncludeParam validate params from request
func ValidateIncludeParam(feed types.FeedType, include []string, txFromFieldIncludable bool) ([]string, error) {
	var requestedFields []string

	if len(include) == 0 {
		switch feed {
		case types.BDNBlocksFeed, types.NewBlocksFeed:
			requestedFields = validBlockParams
		case types.BDNBeaconBlocksFeed, types.NewBeaconBlocksFeed:
			requestedFields = validBeaconBlockParams
		case types.NewTxsFeed, types.PendingTxsFeed:
			requestedFields = defaultTxParams

			if txFromFieldIncludable {
				requestedFields = append(requestedFields, txFromField)
			}
		case types.OnBlockFeed:
			requestedFields = validOnBlockParams
		case types.TxReceiptsFeed:
			requestedFields = validTxReceiptParams
		}

		return requestedFields, nil
	}

	for _, param := range include {
		switch param {
		case "tx_contents":
			if txFromFieldIncludable {
				requestedFields = append(requestedFields, txContentFieldsWithFrom...)
			} else {
				requestedFields = append(requestedFields, txContentFields...)
			}
		case txFromField:
			if !txFromFieldIncludable {
				return nil, fmt.Errorf("got unsupported param '%s' for Feed '%s'", txFromField, feed)
			}
			requestedFields = append(requestedFields, txFromField)
		default:
			_, ok := validParamsMap[feed][param]
			if !ok {
				return nil, fmt.Errorf("got unsupported param '%v' for Feed '%v'", param, feed)
			}

			requestedFields = append(requestedFields, param)
		}
	}

	return requestedFields, nil
}

func stringSliceToSet(s []string) map[string]struct{} {
	m := make(map[string]struct{})
	for i := range s {
		m[s[i]] = struct{}{}
	}
	return m
}
