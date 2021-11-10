package services

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestHashHistory_Set_Get(t *testing.T) {
	clock := &bxmock.MockClock{}
	history := newHashHistory("", clock, 30*time.Minute)

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
