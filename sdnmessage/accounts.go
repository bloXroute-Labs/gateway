package sdnmessage

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"time"
)

// AccountRequest represents a request to bxapi for account details
// Either field is nullable, so fields must be pointers.
type AccountRequest struct {
	AccountID *types.AccountID `json:"account_id"`
	PeerID    *types.NodeID    `json:"peer_id"`
}

// AccountResponse represents a response from bxapi with account details
type AccountResponse struct {
	NodeID  *types.NodeID `json:"node_id"` // will be none if a pushed down update from bxapi
	Account *Account      `json:"account"`
}

// AccountTier represents a tier name
type AccountTier string

// AccountTier types enumeration
const (
	ATierElite        AccountTier = "EnterpriseElite"
	ATierEnterprise   AccountTier = "Enterprise"
	ATierProfessional AccountTier = "Professional"
	ATierDeveloper    AccountTier = "Developer"
	ATierIntroductory AccountTier = "Introductory"
	ATierDefault      AccountTier = "Default"
	ATierNotFound     AccountTier = "NotFound"
)

// IsElite indicates whether the account tier is elite
func (at AccountTier) IsElite() bool {
	return at == ATierElite
}

// IsEnterprise indicates whether the account tier is considered enterprise
func (at AccountTier) IsEnterprise() bool {
	return at == ATierEnterprise
}

// ReceivesUnpaidTxs indicates whether the account tier receives unpaid txs (only >= ATierProfessional)
func (at AccountTier) ReceivesUnpaidTxs() bool {
	return at == ATierElite || at == ATierEnterprise || at == ATierProfessional
}

// TimeIntervalType represents an time interval type
type TimeIntervalType string

// TimeIntervalType enumeration
const (
	TimeIntervalDaily   TimeIntervalType = "DAILY"
	TimeIntervalWithout TimeIntervalType = "WITHOUT_INTERVAL"
)

// BDNServiceType represents a BDN service type
type BDNServiceType string

// BDNServiceType enumeration
const (
	BDNServiceMsgQuota BDNServiceType = "MSG_QUOTA"
	BDNServicePermit   BDNServiceType = "PERMIT"
)

// BDNServiceBehaviorType represents various flags for service handling behaviors
type BDNServiceBehaviorType string

// BDNServiceBehaviorType enumeration
const (
	// NoAction means
	BehaviorNoAction BDNServiceBehaviorType = "NO_ACTION"
	// Block means to block transaction propagation
	BehaviorBlock BDNServiceBehaviorType = "BLOCK"
	// Alert means issue customer alert
	BehaviorAlert BDNServiceBehaviorType = "ALERT"
	// AuditLog means log audit entry
	BehaviorAuditLog BDNServiceBehaviorType = "AUDIT_LOG"
	// BehaviorBlockAlert means
	BlockAlert BDNServiceBehaviorType = "BLOCK_ALERT"
	// BehaviorAuditAlert means
	AuditAlert BDNServiceBehaviorType = "AUDIT_ALERT"
)

// BDNService represents a service model config
// This struct is roughly equivalent to 'BdnServiceModel' in Python
type BDNService struct {
	TimeInterval      TimeIntervalType       `json:"interval"`
	ServiceType       BDNServiceType         `json:"service_type"`
	Limit             int                    `json:"limit"`
	BehaviorLimitOK   BDNServiceBehaviorType `json:"behavior_limit_ok"`
	BehaviorLimitFail BDNServiceBehaviorType `json:"behavior_limit_fail"`
}

// BDNQuotaService represents quota service model configs
type BDNQuotaService struct {
	ExpireDate     string     `json:"expire_date"`
	MsgQuota       BDNService `json:"msg_quota"`
	ExpireDateTime time.Time
}

// FeedProperties represent feed in BDN service
type FeedProperties struct {
	AllowFiltering  bool     `json:"allow_filtering"`
	AvailableFields []string `json:"available_fields"`
}

// BDNBasicService is a placeholder for service model configs
type BDNBasicService interface{}

// BDNFeedService is a placeholder for service model configs
type BDNFeedService struct {
	ExpireDate string         `json:"expire_date"`
	Feed       FeedProperties `json:"feed"`
}

// BDNPrivateRelayService is a placeholder for service model configs
type BDNPrivateRelayService interface{}

// Account represents the account structure fetched from bxapi
type Account struct {
	AccountInfo
	SecretHash                  string                 `json:"secret_hash"`
	FreeTransactions            BDNQuotaService        `json:"tx_free"`
	PaidTransactions            BDNQuotaService        `json:"tx_paid"`
	CloudAPI                    BDNBasicService        `json:"cloud_api"`
	NewTransactionStreaming     BDNFeedService         `json:"new_transaction_streaming"`
	NewBlockStreaming           BDNFeedService         `json:"new_block_streaming"`
	PendingTransactionStreaming BDNFeedService         `json:"new_pending_transaction_streaming"`
	TransactionStateFeed        BDNFeedService         `json:"transaction_state_feed"`
	OnBlockFeed                 BDNFeedService         `json:"on_block_feed"`
	TransactionReceiptFeed      BDNFeedService         `json:"transaction_receipts_feed"`
	PrivateRelay                BDNPrivateRelayService `json:"private_relays"`
	PrivateTransaction          BDNQuotaService        `json:"private_transaction"`
}

// AccountInfo represents basic info about the account model
// This struct is roughly equivalent to `AccountTemplate` in Python
type AccountInfo struct {
	AccountID          types.AccountID `json:"account_id"`
	LogicalAccountID   string          `json:"logical_account_id"`
	Certificate        string          `json:"certificate"`
	ExpireDate         string          `json:"expire_date"`
	BlockchainProtocol string          `json:"blockchain_protocol"`
	BlockchainNetwork  string          `json:"blockchain_network"`
	TierName           AccountTier     `json:"tier_name"`
	Miner              bool            `json:"is_miner"`
}

// DefaultEnterpriseAccount default enterprise account
var DefaultEnterpriseAccount = Account{
	AccountInfo: AccountInfo{
		AccountID:          "",
		LogicalAccountID:   "",
		Certificate:        "",
		ExpireDate:         "2999-12-31",
		BlockchainProtocol: "Ethereum",
		BlockchainNetwork:  "Mainnet",
		TierName:           ATierEnterprise,
		Miner:              false,
	},
	FreeTransactions: BDNQuotaService{
		ExpireDate: "2999-12-31",
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	PaidTransactions: BDNQuotaService{
		ExpireDate: "2999-12-31",
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	CloudAPI: nil,
	NewTransactionStreaming: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	NewBlockStreaming: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	PendingTransactionStreaming: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	TransactionStateFeed: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	OnBlockFeed: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	TransactionReceiptFeed: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	PrivateRelay: nil,
	PrivateTransaction: BDNQuotaService{
		ExpireDate: "2999-12-31",
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	SecretHash: "",
}
