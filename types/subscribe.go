package types

import bxtypes "github.com/bloXroute-Labs/bxcommon-go/v2/types"

// ClientInfo contains info about account and some meta info
type ClientInfo struct {
	RemoteAddress string
	AccountID     bxtypes.AccountID
	MetaInfo      map[string]string
}

// ReqOptions contains options for REQUEST
type ReqOptions struct {
	Filters  string
	Includes string
}
