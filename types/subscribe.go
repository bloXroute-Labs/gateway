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

// ClientInfo contains info about account and some meta info
type ClientInfo struct {
	RemoteAddress string
	Tier          string
	AccountID     AccountID
	MetaInfo      map[string]string
}

// ReqOptions contains options for REQUEST
type ReqOptions struct {
	Filters  string
	Includes string
}
