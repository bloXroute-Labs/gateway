package account

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/cache"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	sdnmessage "github.com/bloXroute-Labs/bxcommon-go/sdnsdk/message"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
)

const (
	accountsCacheManagerExpDur   = 5 * time.Minute
	accountsCacheManagerCleanDur = 30 * time.Minute
)

var (
	// ErrMethodNotAllowed is returned when the account is not authorized to call a method
	ErrMethodNotAllowed = errors.New("not authorized to call this method")
	// ErrTierTooLow is returned when the account is not enterprise / enterprise elite / ultra
	ErrTierTooLow = errors.New("account must be enterprise / enterprise elite / ultra")
	// ErrInvalidHeader is returned when the authorization header is invalid
	ErrInvalidHeader = errors.New("wrong value in the authorization header")
)

// Accounter declares the interface of the account service
type Accounter interface {
	Authorize(accountID bxtypes.AccountID, secretHash string, isWebsocket bool, remoteAddr string) (sdnmessage.Account, error)
}

type accountFetcher interface {
	FetchCustomerAccountModel(accountID bxtypes.AccountID) (sdnmessage.Account, error)
	AccountModel() sdnmessage.Account
}

// Service is a service for fetching and authorizing accounts
type Service struct {
	cacheMap *cache.Cache[bxtypes.AccountID, Result]
	sdn      accountFetcher
	log      *log.Entry
}

// Result is the result of fetching an account
// It contains the account or an error if the account is not authorized
type Result struct {
	Account          sdnmessage.Account
	NotAuthorizedErr error
}

// NewService creates a new account service
func NewService(sdn accountFetcher, log *log.Entry) *Service {
	c := &Service{
		sdn: sdn,
		log: log,
	}

	c.cacheMap = cache.NewCache[bxtypes.AccountID, Result](syncmap.AccountIDHasher, c.accountFetcher, accountsCacheManagerExpDur, accountsCacheManagerCleanDur)

	return c
}

// Authorize authorizes an account
func (g *Service) Authorize(accountID bxtypes.AccountID, secretHash string, allowAccessByOtherAccounts bool, ip string) (sdnmessage.Account, error) {
	// if gateway received request from a customer with a different account id, it should verify it with the SDN.
	// if the gateway does not have permission to verify account id (which mostly happen with external gateways),
	// SDN will return StatusUnauthorized and fail this connection. if SDN return any other error -
	// assuming the issue is with the SDN and set default enterprise account for the customer. in order to send request to the gateway,
	// customer must be enterprise / elite account
	connectionAccountModel := g.sdn.AccountModel()
	l := g.log.WithFields(log.Fields{
		"accountID":          connectionAccountModel.AccountID,
		"requestedAccountID": accountID,
		"ip":                 ip,
	})

	if accountID != connectionAccountModel.AccountID {
		if !allowAccessByOtherAccounts {
			l.Errorf("account %v is not authorized to call this method directly", g.sdn.AccountModel().AccountID)
			return connectionAccountModel, ErrMethodNotAllowed
		}

		accountRes, err := g.cacheMap.Get(accountID)
		switch {
		case err != nil:
			l.Errorf("failed to get customer account model, using default elite account: %v", err)
			connectionAccountModel = sdnmessage.GetDefaultEliteAccount(time.Now().UTC())
			connectionAccountModel.AccountID = accountID
			connectionAccountModel.SecretHash = secretHash

			// return early here since elite tier satisfies IsEnterprise()
			// and there is no need to check the hashes
			return connectionAccountModel, nil
		case accountRes.NotAuthorizedErr != nil:
			return accountRes.Account, accountRes.NotAuthorizedErr
		default:
			connectionAccountModel = accountRes.Account
		}

		if !connectionAccountModel.TierName.IsEnterprise() {
			l.Warnf("customer account %s must be enterprise / enterprise elite / ultra but it is %v",
				connectionAccountModel.AccountID, connectionAccountModel.TierName)
			return connectionAccountModel, ErrTierTooLow
		}
	}

	if secretHash != connectionAccountModel.SecretHash && secretHash != "" {
		l.Error("account sent a different secret hash than set in the account model")
		return connectionAccountModel, ErrInvalidHeader
	}

	return connectionAccountModel, nil
}

// accountFetcher fetches an account from the SDN
func (g *Service) accountFetcher(accountID bxtypes.AccountID) (*Result, error) {
	account, err := g.sdn.FetchCustomerAccountModel(accountID)
	if err != nil {
		if strings.Contains(err.Error(), strconv.FormatInt(http.StatusUnauthorized, 10)) {
			return &Result{
				NotAuthorizedErr: fmt.Errorf("account %v is not authorized to get other account %v information", g.sdn.AccountModel().AccountID, accountID),
			}, nil
		}

		if strings.Contains(err.Error(), strconv.FormatInt(http.StatusNotFound, 10)) {
			return &Result{
				NotAuthorizedErr: fmt.Errorf("account %v is not found", accountID),
			}, nil
		}

		if strings.Contains(err.Error(), strconv.FormatInt(http.StatusBadRequest, 10)) {
			return &Result{
				NotAuthorizedErr: fmt.Errorf("bad request for %v", accountID),
			}, nil
		}

		// other errors
		return nil, err
	}

	return &Result{
		Account: account,
	}, nil
}
