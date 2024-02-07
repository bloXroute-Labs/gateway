package services

import (
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

func TestBlockHistory_Set_Get(t *testing.T) {
	clock := &utils.MockClock{}
	// have to use date between 1678 and 2262 for UnixNano to work
	clock.SetTime(time.Date(2000, 01, 01, 00, 00, 00, 00, time.UTC))

	history := NewBlockHistory("", 60*time.Minute, clock)
	hash1 := types.SHA256Hash{1}
	hash2 := types.SHA256Hash{2}
	hash3 := types.SHA256Hash{3}
	hash4 := types.SHA256Hash{4}
	history.AddOrUpdate(string(hash1[:]), SeenFromRelay)
	history.AddOrUpdate(string(hash2[:]), SeenFromRelay)
	history.AddOrUpdate(string(hash3[:]), SeenFromNode)
	assert.Equal(t, 3, history.Count())
	status := history.Status(string(hash3[:]))
	assert.Equal(t, status, SeenFromNode)
	status = history.Status(string(hash4[:]))
	assert.Equal(t, status, FirstTimeSeen)
	status = history.Status(string(hash1[:]))
	assert.Equal(t, status, SeenFromRelay)
	status = history.Status(string(hash1[:]))
	assert.Equal(t, status, SeenFromRelay)
	status = history.Status(string(hash3[:]))
	assert.Equal(t, status, SeenFromNode)
	status = history.Status(string(hash4[:]))
	assert.Equal(t, status, FirstTimeSeen)
	history.AddOrUpdate(string(hash1[:]), SeenFromBoth)
	status = history.Status(string(hash1[:]))
	assert.Equal(t, status, SeenFromBoth)

	clock.IncTime(20 * time.Minute)
	status = history.Status(string(hash1[:]))
	assert.Equal(t, status, FirstTimeSeen)
	assert.Equal(t, 3, history.Count())
	clock.IncTime(20 * time.Minute)
	status = history.Status(string(hash1[:]))
	assert.Equal(t, status, FirstTimeSeen)
	status = history.Status(string(hash2[:]))
	assert.Equal(t, status, FirstTimeSeen)

	assert.Equal(t, 3, history.Count())
	assert.Equal(t, 3, history.clean())
	assert.Equal(t, 0, history.Count())
}

func TestBlockHistory_AddOrUpdate(t *testing.T) {
	clock := &utils.MockClock{}
	// have to use date between 1678 and 2262 for UnixNano to work
	clock.SetTime(time.Date(2000, 01, 01, 00, 00, 00, 00, time.UTC))
	history := NewBlockHistory("", 60*time.Minute, clock)
	hash1 := types.SHA256Hash{1}
	hash2 := types.SHA256Hash{2}
	hash3 := types.SHA256Hash{3}
	history.AddOrUpdate(string(hash1[:]), SeenFromRelay)
	history.AddOrUpdate(string(hash2[:]), FirstTimeSeen)
	history.AddOrUpdate(string(hash3[:]), SeenFromNode)
	history.AddOrUpdate(string(hash1[:]), SeenFromRelay)
	status := history.Status(string(hash1[:]))
	assert.Equal(t, status, SeenFromRelay)
	history.AddOrUpdate(string(hash1[:]), SeenFromNode)
	status = history.Status(string(hash1[:]))
	assert.Equal(t, status, SeenFromBoth)
	history.AddOrUpdate(string(hash2[:]), SeenFromNode)
	status = history.Status(string(hash2[:]))
	assert.Equal(t, status, SeenFromNode)
	history.AddOrUpdate(string(hash3[:]), SeenFromRelay)
	status = history.Status(string(hash3[:]))
	assert.Equal(t, status, SeenFromBoth)
}
