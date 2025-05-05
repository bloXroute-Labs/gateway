package utils

import (
	"bytes"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	uuid "github.com/satori/go.uuid"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
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

// GetGID provide the goroutine ID
func GetGID() string {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, err := strconv.ParseUint(string(b), 10, 64)
	if err != nil {
		log.Errorf("can't extract GID - %v", err)
		return "go-routineID unknown"
	}
	return fmt.Sprintf("go-routineID %v", n)
}

// GenerateUUID generates random subscription ID
func GenerateUUID() string {
	newUUID, _ := uuid.NewV4()
	return newUUID.String()
}

// GenerateU128 generates random u128 string as subscription ID
func GenerateU128() (string, error) {
	u128 := make([]byte, 16)
	_, err := cryptorand.Read(u128)
	if err != nil {
		return "", err
	}

	hexEncoded := hex.EncodeToString(u128)
	return "0x" + hexEncoded, nil
}
