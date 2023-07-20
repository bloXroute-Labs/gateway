package types

import (
	"errors"
	"fmt"
	"strings"

	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
)

var errWrongFormat = errors.New("provided value is in a wrong format")

// CallParamSliceFlag is a custom flag type so that can be used from cli package
type CallParamSliceFlag struct {
	Values []*pb.CallParams
}

// Set method parses the provided value and creates the CallParamSliceFlag object
func (f *CallParamSliceFlag) Set(value string) error {
	multiParamSlice := strings.Split(value, ",")

	for _, paramSlice := range multiParamSlice {
		keyValuePairs := strings.Split(paramSlice, ";")
		var paramMap *pb.CallParams
		m := make(map[string]string)

		for _, kv := range keyValuePairs {
			pair := strings.Split(kv, ":")
			if len(pair) != 2 {
				return fmt.Errorf("%w: %v", errWrongFormat, kv)
			}

			key := strings.TrimSpace(pair[0])
			value = strings.TrimSpace(pair[1])
			m[key] = value

			paramMap = &pb.CallParams{Params: m}
		}

		f.Values = append(f.Values, paramMap)
	}
	return nil
}

// String method returns the CallParamSliceFlag as string
func (f *CallParamSliceFlag) String() string {
	var keyValuePairs []string
	for _, callParam := range f.Values {
		for k, v := range callParam.Params {
			keyValuePairs = append(keyValuePairs, fmt.Sprintf("%s:%s", k, v))
		}
	}

	return strings.Join(keyValuePairs, ",")
}
