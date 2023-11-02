package caller

import "context"

// Caller is a caller interface
type Caller interface {
	CallContext(ctx context.Context, result any, method string, args ...any) error
	Close()
}
