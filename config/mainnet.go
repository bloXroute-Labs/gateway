package config

// MainnetEnv is configuration for a instance running on a relay instance in testnet
var MainnetEnv = Env{
	SDNURL:              "https://bdn-api.blxrbdn.com",
	RegistrationCertDir: "ssl/",
	CACertURL:           "ssl/ca",
	DataDir:             "datadir",
	Environment:         "mainnet",
}
