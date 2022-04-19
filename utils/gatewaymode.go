package utils

import (
	"fmt"
	"strings"
)

// GatewayMode represents gateway mode
type GatewayMode string

const (
	// GatewayModeFlashbots is gateway flashbots mode
	GatewayModeFlashbots GatewayMode = "flashbots"
	// GatewayModeBDN is gateway BDN mode
	GatewayModeBDN GatewayMode = "bdn"
)

func (m GatewayMode) split() []string {
	return strings.Split(strings.TrimSpace(string(m)), ",")
}

// FromStringToGatewayMode returns GatewayMode from string name
func FromStringToGatewayMode(s string) (GatewayMode, error) {
	mode := GatewayMode(s)

	for _, m := range mode.split() {
		switch m {
		case string(GatewayModeFlashbots):
		case string(GatewayModeBDN):
		default:
			return "", fmt.Errorf("incorrect mode %s", m)
		}
	}

	return mode, nil
}

// IsBDN returns true if gateway has BDN mode
func (m GatewayMode) IsBDN() bool {
	for _, mode := range m.split() {
		if mode == string(GatewayModeBDN) {
			return true
		}
	}

	return false
}
