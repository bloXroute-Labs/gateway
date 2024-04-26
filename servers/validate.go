package servers

import (
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	txFromFilter = "from"
	txFromField  = "tx_contents.from"
)

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
