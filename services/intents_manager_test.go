package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntentsManager(t *testing.T) {
	t.Run("SubscriptionMessages", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddIntentsSubscription("1", nil, nil)
		target.AddSolutionsSubscription("1", nil, nil)
		require.Len(t, target.SubscriptionMessages(), 2)
	})

	t.Run("AddIntentsSubscription", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddIntentsSubscription("1", nil, nil)
		_, ok := target.intentsSubscriptions["1"]
		require.True(t, ok)
	})

	t.Run("RmIntentsSubscription", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddIntentsSubscription("1", nil, nil)
		_, ok := target.intentsSubscriptions["1"]
		require.True(t, ok)

		target.RmIntentsSubscription("1")
		_, ok = target.intentsSubscriptions["1"]
		require.False(t, ok)
	})

	t.Run("IntentsSubscriptionExists", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddIntentsSubscription("1", nil, nil)
		require.True(t, target.IntentsSubscriptionExists("1"))

		target.RmIntentsSubscription("1")
		require.False(t, target.IntentsSubscriptionExists("1"))
	})

	t.Run("AddSolutionsSubscription", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddSolutionsSubscription("1", nil, nil)
		_, ok := target.solutionsSubscriptions["1"]
		require.True(t, ok)
	})

	t.Run("RmSolutionsSubscription", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddSolutionsSubscription("1", nil, nil)
		_, ok := target.solutionsSubscriptions["1"]
		require.True(t, ok)

		target.RmSolutionsSubscription("1")
		_, ok = target.solutionsSubscriptions["1"]
		require.False(t, ok)
	})

	t.Run("SolutionsSubscriptionExists", func(t *testing.T) {
		var target = NewIntentsManager()
		target.AddSolutionsSubscription("1", nil, nil)
		require.True(t, target.SolutionsSubscriptionExists("1"))

		target.RmSolutionsSubscription("1")
		require.False(t, target.SolutionsSubscriptionExists("1"))
	})
}
