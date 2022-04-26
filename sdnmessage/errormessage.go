package sdnmessage

// ErrorMessage represents error message from 400 Bad Request message details
type ErrorMessage struct {
	Message string `json:"message"`
	Details string `json:"details"`
}
