package bor

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	maxRetries = 10

	backoffInterval = 100 * time.Millisecond
)

// Retry returns preconfigured backoff instance.
func Retry() backoff.BackOff {
	return backoff.WithMaxRetries(backoff.NewConstantBackOff(backoffInterval), maxRetries)
}
