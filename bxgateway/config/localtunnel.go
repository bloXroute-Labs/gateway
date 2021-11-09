package config

// LocalTunnelEnv is configuration for running in a local dev environment,
// but connecting to the testnet SDN socket broker over a SSH tunnel, e.g. with
// ssh 54.174.28.236 -fNL 1800:bxapi-s.testnet-v176-16.testnet.blxrbdn.com:1800 -M -S sdn-socket
var LocalTunnelEnv = Env{
	SDNURL:              "https://bdn-api.testnet.blxrbdn.com",
	RegistrationCertDir: "ssl/",
	CACertURL:           "ssl/ca",
	DataDir:             "datadir",
	Environment:         "localTunnel",
}
