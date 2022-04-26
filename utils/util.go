package utils

import (
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"net"
	"strconv"
	"strings"
)

// Abs returns the absolute value of an integer
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GenerateValidEnode generates enode
func GenerateValidEnode(ip string, tcp int, udp int) *enode.Node {
	key, err := ecdsa.GenerateKey(crypto.S256(), cryptorand.Reader)
	if err != nil {
		panic(err)
	}
	splitIP := strings.Split(ip, ".")
	ipByte1, _ := strconv.Atoi(splitIP[0])
	ipByte2, _ := strconv.Atoi(splitIP[1])
	ipByte3, _ := strconv.Atoi(splitIP[2])
	ipByte4, _ := strconv.Atoi(splitIP[3])
	return enode.NewV4(&key.PublicKey, net.IPv4(byte(ipByte1), byte(ipByte2), byte(ipByte3), byte(ipByte4)), tcp, udp)
}

// GetIP checks the existence of and returns the IP address for a host name
func GetIP(host string) (string, error) {
	addr := net.ParseIP(host)
	if addr == nil {
		// If domain name provided instead of IP, convert it to an IP address
		ips, err := net.LookupHost(host)
		if err != nil {
			return "", fmt.Errorf("host provided %s is not valid - %v", host, err)
		}
		if len(ips) == 0 {
			return "", fmt.Errorf("host provided %s has no IPs behind the domain name", host)
		}

		_, err = net.LookupIP(ips[0])
		if err != nil {
			return "", fmt.Errorf("host provided %s is not valid - %v", host, err)
		}

		return ips[0], nil
	}
	return host, nil
}
