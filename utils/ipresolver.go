package utils

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"regexp"
)

const publicIPResolver = "http://checkip.dyndns.org/"

var ipRegex, _ = regexp.Compile("[0-9]+(?:\\.[0-9]+){3}")

// GetPublicIP fetches the publicly seen IP address of the currently running process.
func GetPublicIP() (string, error) {
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

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(ipRegex.Find(body)), nil
}
