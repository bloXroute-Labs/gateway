package config

// LocalEnv is configuration for running in a local dev environment
// (i.e. running your own bxapi and relay instances)
var LocalEnv = Env{
	SDNURL:              "https://localhost:8080",
	RegistrationCertDir: "ssl/local",
	CACertURL:           "ssl/local/ca",
	DataDir:             "datadir",
	Environment:         "local",
}
