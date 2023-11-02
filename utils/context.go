package utils

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// ContextWithSignal returns a context that is cancelled when the process receives the given termination signal.
func ContextWithSignal(parent context.Context, s ...os.Signal) context.Context {
	if len(s) == 0 {
		s = []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	}
	ctx, cancel := context.WithCancel(parent)
	c := make(chan os.Signal, 1)
	signal.Notify(c, s...)

	go func() {
		// wait for either the signal, or for the context to be cancelled
		select {
		case <-c:
		case <-parent.Done():
		}

		// cancel the context
		// and stop waiting for more signals (next sigterm will terminate the process)
		cancel()
		signal.Stop(c)
	}()

	return ctx
}
