package types

// ErrorNotificationCode represents error notification code
type ErrorNotificationCode uint32

// flag constant values
const (
	ErrorNotificationCodeNotAuthorized ErrorNotificationCode = 1000 << iota
	ErrorNotificationCodeForbiddenByFirewall
	ErrorNotificationUnsupportedCode
	ErrorNotificationDisabledTimeout
)

// CodeToReason converts code to reason
var CodeToReason = map[ErrorNotificationCode]string{
	ErrorNotificationCodeNotAuthorized:       "%v",
	ErrorNotificationCodeForbiddenByFirewall: "%v",
	ErrorNotificationUnsupportedCode:         "%v",
	ErrorNotificationDisabledTimeout:         "closing disabled connection",
}
