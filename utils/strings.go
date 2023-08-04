package utils

import (
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
)

// IsExpiredDate checks if the account is expired
// Date should have format: 2006-01-02
func IsExpiredDate(clock Clock, expireDate string) (bool, error) {
	expirationDate, err := time.Parse(bxgateway.TimeDateLayoutISO, expireDate)
	if err != nil {
		return false, fmt.Errorf("failed to parse expire date: %v", err)
	}

	return !expirationDate.After(clock.Now().UTC()), nil
}
