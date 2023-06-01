package services

import "context"

// Runner is an interface for running a service.
type Runner interface {
	Run(ctx context.Context) error
}
