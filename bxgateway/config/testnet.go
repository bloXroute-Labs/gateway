package config

// TestnetEnv is configuration for a instance running on a relay instance in testnet
var TestnetEnv = Env{
	SDNURL:              "https://bdn-api.testnet.blxrbdn.com",
	RegistrationCertDir: "ssl/",
	CACertURL:           "ssl/ca",
	DataDir:             "datadir",
	Environment:         "testnet",
}
