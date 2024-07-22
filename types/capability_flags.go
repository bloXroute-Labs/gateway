package types

// CapabilityFlags represents various flags for capabilities in hello msg
type CapabilityFlags uint16

// flag constant values
const (
	_ CapabilityFlags = 1 << iota // Deprecated: CapabilityFastSync used only for unitests
	CapabilityMEVBuilder
	_ // Deprecated: CapabilityMEVMiner used for indication if the connection is a mev-miner gateway
	_ // Deprecated: CapabilityBDN used for indication if the connected gw is bdn/flashbot mode
	CapabilityBlockchainRPCEnabled
	CapabilityNoBlocks
)
