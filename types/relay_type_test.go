package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRelayType(t *testing.T) {
	backboneType, err := RelayTypeFromString("backbone")
	require.NoError(t, err)
	require.Equal(t, BackboneRelay, backboneType)

	edgeType, err := RelayTypeFromString("edge")
	require.NoError(t, err)
	require.Equal(t, EdgeRelay, edgeType)

	require.Equal(t, "backbone", backboneType.String())
	require.Equal(t, "edge", edgeType.String())
}

func TestRelayTypeInvalidRelayString(t *testing.T) {
	_, err := RelayTypeFromString("invalid")
	require.Error(t, err)
}

func TestRelayTypeJSON(t *testing.T) {
	type Target struct {
		T RelayType `json:"type"`
	}

	var backboneStruct = new(Target)
	err := json.Unmarshal([]byte(`{"type": "backbone"}`), backboneStruct)
	require.NoError(t, err)
	require.Equal(t, BackboneRelay, backboneStruct.T)

	var edgeStruct = new(Target)
	err = json.Unmarshal([]byte(`{"type": "edge"}`), edgeStruct)
	require.NoError(t, err)
	require.Equal(t, EdgeRelay, edgeStruct.T)

	var errStruct = new(Target)
	err = json.Unmarshal([]byte(`{"type": "invalid"}`), errStruct)
	require.Error(t, err)

	var nullStruct = new(Target)
	err = json.Unmarshal([]byte(`{"type": null}`), nullStruct)
	require.NoError(t, err)
	require.Equal(t, GenericRelay, nullStruct.T)

	var emptyStrStruct = new(Target)
	err = json.Unmarshal([]byte(`{"type": ""}`), emptyStrStruct)
	require.NoError(t, err)
	require.Equal(t, GenericRelay, emptyStrStruct.T)

	marshalBackboneStruct := Target{T: BackboneRelay}
	b, err := json.Marshal(marshalBackboneStruct)
	require.NoError(t, err)

	require.Equal(t, `{"type":"backbone"}`, string(b))
}
