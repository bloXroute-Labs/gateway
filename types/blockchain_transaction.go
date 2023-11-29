package types

// todo: no need for BlockchainTransaction

// BlockchainTransaction represents a generic blockchain transaction that allows filtering its fields
type BlockchainTransaction interface {
	Filters(filters []string) map[string]interface{}
	Fields(fields []string) map[string]interface{}
	Sender() (Sender, error)
}
