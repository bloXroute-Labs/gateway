package bor

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type WSResp[T any] struct {
	Result T `json:"result"`
}

type SprinterPayloadDTO struct {
	Current  WSResp[*SpanInfo]
	Next     WSResp[*SpanInfo]
	Snapshot WSResp[*Snapshot]
}

func newSprinterPayloadDTO(t require.TestingT, currentSpanPath, nextSpanPath, snapshotPath string) *SprinterPayloadDTO {
	sprinterPayloadDTO := new(SprinterPayloadDTO)

	data, err := os.ReadFile(currentSpanPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &sprinterPayloadDTO.Current))

	data, err = os.ReadFile(nextSpanPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &sprinterPayloadDTO.Next))

	data, err = os.ReadFile(snapshotPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &sprinterPayloadDTO.Snapshot))

	return sprinterPayloadDTO
}

func Benchmark_getSprintValidatorsMap(b *testing.B) {
	sprinterPayload := newSprinterPayloadDTO(
		b,
		"./testdata/span_6145.json",
		"./testdata/span_6146.json",
		"./testdata/snap_39321856.json",
	)

	for n := 0; n < b.N; n++ {
		_, _ = getSprintValidatorsMap(
			sprinterPayload.Current.Result,
			sprinterPayload.Next.Result,
			sprinterPayload.Snapshot.Result,
		)
	}
}

func BenchmarkSnapshot_IncrementSprint(b *testing.B) {
	sprinterPayload := newSprinterPayloadDTO(
		b,
		"./testdata/span_6145.json",
		"./testdata/span_6146.json",
		"./testdata/snap_39321856.json",
	)

	for n := 0; n < b.N; n++ {
		_, _ = sprinterPayload.Snapshot.Result.IncrementSprint(
			sprinterPayload.Current.Result,
			sprinterPayload.Next.Result,
		)
	}
}
