package types

// CapabilityFlags represents various flags for capabilities in hello msg
type CapabilityFlags uint16

// flag constant values
const (
	CapabilityFastSync CapabilityFlags = 1 << iota
	CapabilityMevBuilder
	CapabilityMevMiner
)
