package ptr

// New returns pointer to provided value.
func New[T any](val T) *T { return &val }
