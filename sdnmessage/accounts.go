package sdnmessage

import (
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common/math"
	"strconv"
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
	ATierUltra        AccountTier = "Ultra"
	ATierElite        AccountTier = "EnterpriseElite"
	ATierEnterprise   AccountTier = "Enterprise"
	ATierProfessional AccountTier = "Professional"
	ATierDeveloper    AccountTier = "Developer"
	ATierIntroductory AccountTier = "Introductory"
)

// IsUltra indicates whether the account tier is ultra
func (at AccountTier) IsUltra() bool {
	return at == ATierUltra
}

// IsElite indicates whether the account tier is elite or ultra
func (at AccountTier) IsElite() bool {
	return at == ATierElite || at == ATierUltra
}

// IsEnterprise indicates whether the account tier is considered enterprise, elite or ultra
func (at AccountTier) IsEnterprise() bool {
	return at == ATierEnterprise || at == ATierElite || at == ATierUltra
}

// ReceivesUnpaidTxs indicates whether the account tier receives unpaid txs (only >= ATierProfessional)
func (at AccountTier) ReceivesUnpaidTxs() bool {
	return at == ATierUltra || at == ATierElite || at == ATierEnterprise || at == ATierProfessional
}

// IsValid indicates whether the account tier is valid
func (at AccountTier) IsValid() error {
	switch at {
	case ATierUltra, ATierElite, ATierEnterprise, ATierProfessional, ATierDeveloper, ATierIntroductory:
		return nil
	}
	return fmt.Errorf("unrecognized account tier: %v", at)
}

// TimeIntervalType represents an time interval type
type TimeIntervalType string

// BDNServiceLimit represents large integer that can hold number bigger than math.MaxBigInt and also negative numbers
type BDNServiceLimit int64

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
	// BehaviorNoAction means
	BehaviorNoAction BDNServiceBehaviorType = "NO_ACTION"
	// BehaviorBlock means to block transaction propagation
	BehaviorBlock BDNServiceBehaviorType = "BLOCK"
	// BehaviorAlert means issue customer alert
	BehaviorAlert BDNServiceBehaviorType = "ALERT"
	// BehaviorAuditLog means log audit entry
	BehaviorAuditLog BDNServiceBehaviorType = "AUDIT_LOG"
	// BehaviorBlockAlert means to block event and issue customer alert
	BehaviorBlockAlert BDNServiceBehaviorType = "BLOCK_ALERT"
	// BehaviorAuditAlert means
	BehaviorAuditAlert BDNServiceBehaviorType = "AUDIT_ALERT"
)

// BDNService represents a service model config
// This struct is roughly equivalent to 'BdnServiceModel' in Python
type BDNService struct {
	TimeInterval      TimeIntervalType       `json:"interval"`
	ServiceType       BDNServiceType         `json:"service_type"`
	Limit             BDNServiceLimit        `json:"limit"`
	BehaviorLimitOK   BDNServiceBehaviorType `json:"behavior_limit_ok"`
	BehaviorLimitFail BDNServiceBehaviorType `json:"behavior_limit_fail"`
}

// BDNQuotaService represents quota service model configs
type BDNQuotaService struct {
	MsgQuota       BDNService
	ExpireDateTime time.Time
}

// quotaService is a temporary struct for deserializing BDNQuotaService
type quotaService struct {
	ExpireDate string     `json:"expire_date"`
	MsgQuota   BDNService `json:"msg_quota"`
}

// UnmarshalJSON implements deserialization for BDNQuotaService type
func (i *BDNServiceLimit) UnmarshalJSON(b []byte) error {
	stringElement := json.Number(b)
	limit, err := stringElement.Int64()
	if err != nil && err.(*strconv.NumError).Err == strconv.ErrRange {
		*i = math.MaxInt64
	} else if err != nil {
		return err
	}
	*i = BDNServiceLimit(limit)
	return nil
}

// UnmarshalJSON implements deserialization for BDNQuotaService type
func (bdnQS *BDNQuotaService) UnmarshalJSON(b []byte) error {
	qs := quotaService{
		ExpireDate: bxgateway.ExpiredDate,
		MsgQuota:   BDNService{},
	}
	err := json.Unmarshal(b, &qs)
	if err != nil {
		return err
	}
	expireDateTime, err := time.Parse(bxgateway.TimeDateLayoutISO, qs.ExpireDate)
	bdnQS.ExpireDateTime = expireDateTime
	bdnQS.MsgQuota = qs.MsgQuota
	return nil
}

// MarshalJSON implements serialization for BDNQuotaService type
func (bdnQS BDNQuotaService) MarshalJSON() ([]byte, error) {
	qs := quotaService{
		ExpireDate: bdnQS.ExpireDateTime.Format(bxgateway.TimeDateLayoutISO),
		MsgQuota:   bdnQS.MsgQuota,
	}
	return json.Marshal(qs)
}

// FeedProperties represent feed in BDN service
type FeedProperties struct {
	AllowFiltering  bool     `json:"allow_filtering"`
	AvailableFields []string `json:"available_fields"`
}

// BDNBasicService is a placeholder for service model configs
type BDNBasicService struct {
	ExpireDate string `json:"expire_date"`
}

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
	SecretHash                   string                 `json:"secret_hash"`
	FreeTransactions             BDNQuotaService        `json:"tx_free"`
	PaidTransactions             BDNQuotaService        `json:"tx_paid"`
	CloudAPI                     BDNBasicService        `json:"cloud_api"`
	NewTransactionStreaming      BDNFeedService         `json:"new_transaction_streaming"`
	NewBlockStreaming            BDNFeedService         `json:"new_block_streaming"`
	PendingTransactionStreaming  BDNFeedService         `json:"new_pending_transaction_streaming"`
	InternalTransactionStreaming BDNFeedService         `json:"new_internal_transaction_streaming"`
	TransactionStateFeed         BDNFeedService         `json:"transaction_state_feed"`
	OnBlockFeed                  BDNFeedService         `json:"on_block_feed"`
	TransactionReceiptFeed       BDNFeedService         `json:"transaction_receipts_feed"`
	PrivateRelay                 BDNPrivateRelayService `json:"private_relays"`
	PrivateTransaction           BDNQuotaService        `json:"private_transaction"`
	PrivateTransactionFee        BDNQuotaService        `json:"private_transaction_fee"`
	TxTraceRateLimit             BDNQuotaService        `json:"tx_trace_rate_limitation"`
	RelayLimit                   BDNQuotaService        `json:"relay_limit"`
	MinAllowedNodes              BDNQuotaService        `json:"min_allowed_nodes"`
	MaxAllowedNodes              BDNQuotaService        `json:"max_allowed_nodes"`

	// txs allowed per 5s
	UnpaidTransactionBurstLimit BDNQuotaService `json:"unpaid_tx_burst_limit"`
	PaidTransactionBurstLimit   BDNQuotaService `json:"paid_tx_burst_limit"`

	BoostMEVSearcher BDNBasicService `json:"boost_mevsearcher"`
}

// Validate verifies the response that the response from bxapi is well understood
func (a *Account) Validate() error {
	err := a.TierName.IsValid()
	if err != nil {
		a.TierName = ATierElite
		return err
	}
	return nil
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
	MEVBuilder         string          `json:"mev_builder"`
	MEVMiner           string          `json:"mev_miner"`
}

// DefaultEliteAccount default Elite account
var DefaultEliteAccount = Account{
	AccountInfo: AccountInfo{
		AccountID:          "",
		LogicalAccountID:   "",
		Certificate:        "",
		ExpireDate:         fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		BlockchainProtocol: "Ethereum",
		BlockchainNetwork:  "Mainnet",
		TierName:           ATierElite,
		Miner:              false,
	},
	FreeTransactions: BDNQuotaService{
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	PaidTransactions: BDNQuotaService{
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	CloudAPI: BDNBasicService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
	},
	NewTransactionStreaming: BDNFeedService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		Feed: FeedProperties{
			AllowFiltering:  true,
			AvailableFields: []string{"all"},
		},
	},
	NewBlockStreaming: BDNFeedService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		Feed: FeedProperties{
			AllowFiltering:  true,
			AvailableFields: []string{"all"},
		},
	},
	PendingTransactionStreaming: BDNFeedService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		Feed: FeedProperties{
			AllowFiltering:  true,
			AvailableFields: []string{"all"},
		},
	},
	InternalTransactionStreaming: BDNFeedService{
		ExpireDate: "2999-12-31",
		Feed: FeedProperties{
			AllowFiltering:  true,
			AvailableFields: []string{"all"},
		},
	},
	TransactionStateFeed: BDNFeedService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	OnBlockFeed: BDNFeedService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	TransactionReceiptFeed: BDNFeedService{
		ExpireDate: fmt.Sprintf("%s", time.Now().AddDate(0, 0, 1).Format("2006-01-02")),
		Feed: FeedProperties{
			AllowFiltering:  false,
			AvailableFields: nil,
		},
	},
	PrivateRelay: nil,
	PrivateTransaction: BDNQuotaService{
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	TxTraceRateLimit: BDNQuotaService{
		MsgQuota: BDNService{
			TimeInterval: TimeIntervalDaily,
			ServiceType:  BDNServiceMsgQuota,
			Limit:        1,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	UnpaidTransactionBurstLimit: BDNQuotaService{
		MsgQuota: BDNService{
			ServiceType:       BDNServiceMsgQuota,
			Limit:             20,
			BehaviorLimitOK:   BehaviorNoAction,
			BehaviorLimitFail: BehaviorNoAction,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	PaidTransactionBurstLimit: BDNQuotaService{
		MsgQuota: BDNService{
			ServiceType:       BDNServiceMsgQuota,
			Limit:             50,
			BehaviorLimitOK:   BehaviorNoAction,
			BehaviorLimitFail: BehaviorAlert,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	BoostMEVSearcher: BDNBasicService{
		ExpireDate: bxgateway.ExpiredDate,
	},
	RelayLimit: BDNQuotaService{
		MsgQuota: BDNService{
			ServiceType: BDNServicePermit,
			Limit:       2,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	MinAllowedNodes: BDNQuotaService{
		MsgQuota: BDNService{
			ServiceType: BDNServicePermit,
			Limit:       0,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	MaxAllowedNodes: BDNQuotaService{
		MsgQuota: BDNService{
			ServiceType: BDNServicePermit,
			Limit:       2,
		},
		ExpireDateTime: time.Now().Add(time.Hour),
	},
	SecretHash: "",
}
