package utils

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
)

const publicIPResolver = "http://checkip.dyndns.org/"

var ipRegex, _ = regexp.Compile("[0-9]+(?:\\.[0-9]+){3}")

// IPResolverHolder
var (
	IPResolverHolder IPResolver
)

// IPResolver represents an interface
type IPResolver interface {
	GetPublicIP() (string, error)
}

// PublicIPResolver represents ip resolver struct
type PublicIPResolver struct{}

// GetPublicIP fetches the publicly seen IP address of the currently running process.
func (*PublicIPResolver) GetPublicIP() (string, error) {
	response, err := http.Get(publicIPResolver)
	if err != nil {
		return "", err
	}

	defer func() {
		err = response.Body.Close()
		if err != nil {
			log.Error(fmt.Errorf("unable to close response body %v error %v", response.Body, err))
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != 200 {
		return "", errors.New(string(body))
	}

	return string(ipRegex.Find(body)), nil
}
