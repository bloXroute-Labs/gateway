package ticker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

func TestArgs(t *testing.T) {
	clock := new(utils.RealClock)

	invalidArgs := new(Args)

	validArgs := &Args{
		Clock: clock,
		Log:   logger.Discard(),
		Delays: []time.Duration{
			time.Second,
			time.Second * 2,
			time.Second * 3,
		},
		Interval: time.Second,
	}

	t.Run("Validate", func(t *testing.T) {
		t.Run("InvalidArgs", func(t *testing.T) {
			t.Run("LogIsNotInit", func(t *testing.T) {
				require.ErrorIs(t, invalidArgs.Validate(), ErrLogIsNotInit)
			})

			invalidArgs.Log = logger.Discard()

			t.Run("ClockIsNotInit", func(t *testing.T) {
				require.ErrorIs(t, invalidArgs.Validate(), ErrClockIsNotInit)
			})

			t.Run("InvalidInterval", func(t *testing.T) {
				invalidArgs.Clock = clock
				require.ErrorIs(t, invalidArgs.Validate(), ErrInvalidInterval)
			})

			t.Run("DelaysNotSetButIntervalIsSet", func(t *testing.T) {
				invalidArgs.Interval = time.Second
				require.NoError(t, invalidArgs.Validate())
			})

			t.Run("DelaysSetIncorrectlyAndIntervalNotSet", func(t *testing.T) {
				invalidArgs.Delays = []time.Duration{time.Second, time.Second}
				require.ErrorIs(t, invalidArgs.Validate(), ErrInvalidInterval)
			})
		})

		t.Run("ValidArgs", func(t *testing.T) {
			require.NoError(t, validArgs.Validate())
			require.Equal(t, []time.Duration{time.Second, time.Second, time.Second}, validArgs.getDelays())
		})
	})
}
