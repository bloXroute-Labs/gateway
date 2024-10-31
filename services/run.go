package services

import (
	"context"
	"errors"
	"fmt"
)

// RunState is a state of a service.
type RunState uint8

const (
	// StateIdle is a state of a service that is not running.
	StateIdle = RunState(iota)

	// StateBooting is a state of a service that is booting.
	StateBooting

	// StateRunning is a state of a service that is running.
	StateRunning
)

func (s RunState) String() string {
	return []string{"idle", "booting", "running"}[s]
}

// ErrWrongState is returned when a service is not in the correct state for an operation.
var ErrWrongState = errors.New("service is not in the correct state for this operation")

// Runner is an interface for running a service.
type Runner interface {
	Run(ctx context.Context) error
}

// RequireState returns an error if the state is not the required one.
func RequireState(state *RunState, required RunState) error {
	if state == nil {
		return fmt.Errorf("required(%s) <> actual(nil): %w", required, ErrWrongState)
	}

	if *state != required {
		return fmt.Errorf("required(%s) <> actual(%s): %w", required, *state, ErrWrongState)
	}

	return nil
}
