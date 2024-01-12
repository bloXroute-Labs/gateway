package types

// ErrorNotificationCode represents error notification code
type ErrorNotificationCode uint32

// MinErrorNotificationCode declares the min number since error codes begin
// errors before this const considered as warnings
const MinErrorNotificationCode = 1000

// codes below 1000 are warnings and do not lead to gateway shutdown
const (
	// WarnNotificationInvalidInput reports about invalid message from user
	WarnNotificationInvalidInput ErrorNotificationCode = iota + 1
	WarnNotificationInternalError
)

// flag constant values
const (
	ErrorNotificationCodeNotAuthorized ErrorNotificationCode = MinErrorNotificationCode << iota
	ErrorNotificationCodeForbiddenByFirewall
	ErrorNotificationUnsupportedCode
	ErrorNotificationDisabledTimeout
	ErrorNotificationDuplicateConnection
)

// CodeToReason converts code to reason
var CodeToReason = map[ErrorNotificationCode]string{
	ErrorNotificationCodeNotAuthorized:       "%v",
	ErrorNotificationCodeForbiddenByFirewall: "%v",
	ErrorNotificationUnsupportedCode:         "%v",
	ErrorNotificationDisabledTimeout:         "closing disabled connection %v",
	ErrorNotificationDuplicateConnection:     "%v",
	WarnNotificationInvalidInput:             "%v",
	WarnNotificationInternalError:            "%v",
}
