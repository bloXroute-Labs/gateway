package utils

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

var (
	errAuthHeaderNotBase65   = errors.New("auth header is not base64 encoded")
	errAuthHeaderWrongFormat = errors.New("account_id and hash could not be generated from auth header")
)

// GetAccountIDSecretHashFromHeader extracts accountID and secret values from an authorization header
func GetAccountIDSecretHashFromHeader(authHeader string) (types.AccountID, string, error) {
	payload, err := base64.StdEncoding.DecodeString(authHeader)
	if err != nil {
		return "", "", fmt.Errorf("%w:, %v", errAuthHeaderNotBase65, authHeader)
	}
	accountIDAndHash := strings.SplitN(string(payload), ":", 2)
	if len(accountIDAndHash) <= 1 {
		return "", "", fmt.Errorf("%w:, %v", errAuthHeaderWrongFormat, authHeader)
	}
	accountID := types.AccountID(accountIDAndHash[0])
	secretHash := accountIDAndHash[1]
	return accountID, secretHash, nil
}
