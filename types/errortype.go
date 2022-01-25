package types

// ErrorType represents error type
type ErrorType uint16

// flag constant values
const (
	ErrorTypeTemporary ErrorType = 0
	ErrorTypePermanent ErrorType = 1
)
