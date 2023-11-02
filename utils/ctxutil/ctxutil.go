package ctxutil

import "context"

// CtxSwitcher is a interface for context switcher
type CtxSwitcher interface {
	Context() context.Context
	Cancel()
	Switch() context.Context
}

// BaseCtxSwitcher is a base implementation of CtxSwitcher
type BaseCtxSwitcher struct {
	parent context.Context
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCtxSwitcher creates a new CtxSwitcher
func NewCtxSwitcher(ctx context.Context) *BaseCtxSwitcher {
	cs := new(BaseCtxSwitcher)
	cs.parent = ctx
	cs.ctx, cs.cancel = context.WithCancel(ctx)
	return cs
}

// Context returns current context
func (cs *BaseCtxSwitcher) Context() context.Context { return cs.ctx }

// Cancel cancels current context
func (cs *BaseCtxSwitcher) Cancel() { cs.cancel() }

// Switch switches current context
func (cs *BaseCtxSwitcher) Switch() context.Context {
	cs.cancel()
	cs.ctx, cs.cancel = context.WithCancel(cs.parent)
	return cs.ctx
}
