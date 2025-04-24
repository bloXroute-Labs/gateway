package types

import (
	"encoding/json"
	"reflect"
	"testing"

	gateway "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/stretchr/testify/require"
)

func TestCallParamSliceFlagSet(t *testing.T) {
	tests := []struct {
		name          string
		args          string
		expectedValue CallParamSliceFlag
		err           error
	}{
		{
			name: "Single map",
			args: "key1:value1;key2:value2",
			expectedValue: CallParamSliceFlag{
				Values: []*gateway.CallParams{
					{
						Params: map[string]string{
							"key1": "value1",
							"key2": "value2",
						},
					},
				},
			},
		},
		{
			name: "Multiple maps",
			args: "key1:value1;key2:value2,key3:value3",
			expectedValue: CallParamSliceFlag{
				Values: []*gateway.CallParams{
					{
						Params: map[string]string{
							"key1": "value1",
							"key2": "value2",
						},
					},
					{
						Params: map[string]string{
							"key3": "value3",
						},
					},
				},
			},
		},
		{
			name: "Value has wrong format",
			args: "key1:value1;key2-value2,key3:value3",
			expectedValue: CallParamSliceFlag{
				Values: []*gateway.CallParams{},
			},
			err: errWrongFormat,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &CallParamSliceFlag{}

			err := f.Set(tt.args)
			require.ErrorIs(t, err, tt.err)

			for index, callParams := range f.Values {
				require.True(t, assertEqualMaps(callParams.Params, tt.expectedValue.Values[index].Params))
			}

		})
	}
}

func assertEqualMaps(map1, map2 map[string]string) bool {
	json1, _ := json.Marshal(map1)
	json2, _ := json.Marshal(map2)

	return reflect.DeepEqual(json1, json2)
}
