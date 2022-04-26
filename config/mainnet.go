package config

// MainnetEnv is configuration for an instance running in testnet
var MainnetEnv = Env{
	SDNURL:              "https://bdn-api.blxrbdn.com",
	RegistrationCertDir: "ssl/",
	CACertURL:           "ssl/ca",
	DataDir:             "datadir",
	Environment:         "mainnet",
}
