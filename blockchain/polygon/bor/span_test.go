package bor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeimdallSpannerGetLatestSpan(t *testing.T) {
	t.Skip("Local check only.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	spanner := NewHeimdallSpanner(ctx, "http://0.0.0.0:1317")

	start := time.Now()
	latestSpan, err := spanner.GetLatestSpan()
	t.Log("LatestSpan:", time.Since(start).String())
	require.NoError(t, err)
	require.NotNil(t, latestSpan)

	start = time.Now()
	span, err := spanner.GetSpanByID(latestSpan.SpanID - 1)
	t.Log("CurrentSpan:", time.Since(start).String())
	require.NoError(t, err)
	require.NotNil(t, latestSpan)

	require.Equal(t, latestSpan.StartBlock-1, span.EndBlock)

	cancel()

	latestSpan, err = spanner.GetLatestSpan()
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, latestSpan)

	span, err = spanner.GetSpanByID(1)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, span)
}

func TestGetSpanIdByHeight(t *testing.T) {
	const (
		mainTargetSpanID uint64 = 6145
		startSpanHeight  uint64 = 39321856
		endSpanHeight    uint64 = 39328255
	)

	var resolvedSpanID uint64

	height := startSpanHeight - 1
	targetSpanID := mainTargetSpanID - 1

	resolvedSpanID = GetSpanIDByHeight(height)
	assert.Equalf(t, targetSpanID, resolvedSpanID, "expected: %d, actual: %d, height: %d", targetSpanID, resolvedSpanID, height)

	height = startSpanHeight
	targetSpanID = mainTargetSpanID

	resolvedSpanID = GetSpanIDByHeight(height)
	assert.Equalf(t, targetSpanID, resolvedSpanID, "expected: %d, actual: %d, height: %d", targetSpanID, resolvedSpanID, height)

	height = endSpanHeight + 1
	targetSpanID = mainTargetSpanID + 1

	resolvedSpanID = GetSpanIDByHeight(height)
	assert.Equalf(t, targetSpanID, resolvedSpanID, "expected: %d, actual: %d, height: %d", targetSpanID, resolvedSpanID, height)
}

func TestSpanStart(t *testing.T) {
	const (
		startBlock uint64 = 39321856
		endBlock   uint64 = 39341055
	)

	spanStarts := map[uint64]uint64{
		6145: 39321856,
		6146: 39328256,
		6147: 39334656,
	}

	for height := startBlock; height <= endBlock; height++ {
		spanStart := spanStarts[GetSpanIDByHeight(height)]
		recoveredSpanStart := SpanStart(height)

		require.Equal(t, spanStart, recoveredSpanStart, "expected: %d, actual: %d, height: %d", spanStart, recoveredSpanStart, height)
	}
}
