package services

import (
	"testing"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestHashHistory_Set_Get(t *testing.T) {
	clock := &clock.MockClock{}
	// have to use date between 1678 and 2262 for UnixNano to work
	clock.SetTime(time.Date(2000, 01, 01, 00, 00, 00, 00, time.UTC))

	// clean duration more than in test, we will clean manually
	history := newHashHistory("", clock, 60*time.Minute)

	hash1 := types.SHA256Hash{1}
	hash2 := types.SHA256Hash{2}
	hash3 := types.SHA256Hash{3}
	hash4 := types.SHA256Hash{4}
	history.Add(string(hash1[:]), 10*time.Minute)
	history.Add(string(hash2[:]), 25*time.Minute)
	history.Add(string(hash3[:]), 45*time.Minute)
	assert.Equal(t, 3, history.Count())
	ok := history.Exists(string(hash3[:]))
	assert.True(t, ok)
	ok = history.Exists(string(hash4[:]))
	assert.False(t, ok)

	clock.IncTime(20 * time.Minute)
	ok = history.Exists(string(hash1[:]))
	assert.False(t, ok)
	assert.Equal(t, 3, history.Count())
	clock.IncTime(20 * time.Minute)
	ok = history.Exists(string(hash1[:]))
	assert.False(t, ok)
	ok = history.Exists(string(hash2[:]))
	assert.False(t, ok)

	assert.Equal(t, 3, history.Count())
	assert.Equal(t, 2, history.clean())
	assert.Equal(t, 1, history.Count())
}
