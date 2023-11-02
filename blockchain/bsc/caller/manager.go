package caller

import "context"

// Manager manages callers
type Manager interface {
	AddClient(ctx context.Context, addr string) (Caller, error)
	GetClient(ctx context.Context, addr string) (Caller, error)
	Close()
}
