package http

import (
	"net/http"
	"time"
)

const (
	defaultClientTimeoutSec             = 11 // slightly less than slot duration
	defaultTransportTLSTimeoutSec       = 10
	defaultTransportIdleConnTimeoutSec  = 90
	defaultTransportMaxIdleConns        = 100
	defaultTransportMaxIdleConnsPerHost = 100
)

// Config for custom http.Client.
type Config struct {
	ClientTimeoutSec             int
	TransportTLSTimeoutSec       int
	TransportIdleConnTimeoutSec  int
	TransportMaxIdleConns        int
	TransportMaxIdleConnsPerHost int
}

// Client constructor for http.Client non-populated values are set to its defaults.
func Client(config *Config) *http.Client {
	if config == nil {
		config = new(Config)
	}

	var defaultInt = func(value, xdefault int) int {
		if value == 0 {
			return xdefault
		}
		return value
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSHandshakeTimeout: time.Duration(defaultInt(config.TransportTLSTimeoutSec, defaultTransportTLSTimeoutSec)) * time.Second,
			IdleConnTimeout:     time.Duration(defaultInt(config.TransportIdleConnTimeoutSec, defaultTransportIdleConnTimeoutSec)) * time.Second,
			MaxIdleConns:        defaultInt(config.TransportMaxIdleConns, defaultTransportMaxIdleConns),
			MaxIdleConnsPerHost: defaultInt(config.TransportMaxIdleConnsPerHost, defaultTransportMaxIdleConnsPerHost),
		},
		Timeout: time.Duration(defaultInt(config.ClientTimeoutSec, defaultClientTimeoutSec)) * time.Second,
	}
}
