package config

// TestnetEnv is configuration for an instance running in testnet
var TestnetEnv = Env{
	SDNURL:              "https://bdn-api.testnet.blxrbdn.com",
	RegistrationCertDir: "ssl/testnet",
	CACertURL:           "ssl/testnet/ca",
	DataDir:             "datadir",
	Environment:         "testnet",
}
