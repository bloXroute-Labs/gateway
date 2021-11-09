package types

// BlockchainTransaction represents a generic blockchain transaction that allows filtering its fields
type BlockchainTransaction interface {
	WithFields(fields []string) BlockchainTransaction
	Filters(filters []string) map[string]interface{}
}
