package services

import "github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"

// ShortIDAssigner - an interface for short ID assigner struct
type ShortIDAssigner interface {
	Next() types.ShortID
}

type emptyShortIDAssigner struct {
}

func (empty *emptyShortIDAssigner) Next() types.ShortID {
	return types.ShortIDEmpty
}

// NewEmptyShortIDAssigner - create an assigner that never assign
func NewEmptyShortIDAssigner() ShortIDAssigner {
	return &emptyShortIDAssigner{}
}
