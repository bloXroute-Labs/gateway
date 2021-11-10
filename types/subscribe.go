package types

// SubscriptionResponse struct that represent subscription response from the node
type SubscriptionResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  string `json:"result"`
	Method  string `json:"method"`
	Params  struct {
		Subscription string      `json:"subscription"`
		Result       interface{} `json:"result"`
	} `json:"params"`
}
