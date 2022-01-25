package utils

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// GetWSIPPort parses websocket URI and returns IP and port
func GetWSIPPort(uri string) (string, int, error) {
	parsedWSURI, err := url.Parse(uri)
	if err != nil {
		panic(fmt.Errorf("invalid websocket URI parameter %v provided: %v", uri, err))
	}
	wsIPPort := strings.Split(parsedWSURI.Host, ":")
	if len(wsIPPort) < 2 {
		panic(fmt.Errorf("invalid websocket URI parameter %v provided: %v", uri, err))
	}
	IP := wsIPPort[0]
	port, err := strconv.Atoi(wsIPPort[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid websocket URI parameter %v provided: %v. Unable to convert port to integer", uri, err)
	}
	return IP, port, nil
}

// Abs returns the absolute value of an integer
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
