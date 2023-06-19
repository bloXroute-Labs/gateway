package utils

import (
	"bytes"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"crypto/rand"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	uuid "github.com/satori/go.uuid"
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

// SerializeStruct returns string representation of struct with denoted pointers
func SerializeStruct(v interface{}) string {
	return serializeStruct(reflect.ValueOf(v))
}

func serializeStruct(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Invalid:
		return "nil"
	case reflect.Struct:
		t := v.Type()
		out := getTypeString(t) + "{"
		for i := 0; i < v.NumField(); i++ {
			if i > 0 {
				out += ", "
			}
			fieldValue := v.Field(i)
			field := t.Field(i)
			out += fmt.Sprintf("%s: %s", field.Name, serializeStruct(fieldValue))
		}
		out += "}"
		return out
	case reflect.Interface, reflect.Ptr:
		if v.IsZero() {
			return fmt.Sprintf("(%s)(nil)", getTypeString(v.Type()))
		}
		return "&" + serializeStruct(v.Elem())
	case reflect.Slice:
		out := getTypeString(v.Type())
		if v.IsZero() {
			out += "(nil)"
		} else {
			out += "{"
			for i := 0; i < v.Len(); i++ {
				if i > 0 {
					out += ", "
				}
				out += serializeStruct(v.Index(i))
			}
			out += "}"
		}
		return out
	default:
		return fmt.Sprintf("%#v", v)
	}
}

func getTypeString(t reflect.Type) string {
	if t.PkgPath() == "main" {
		return t.Name()
	}
	return t.String()
}

// GenerateUUID generates random subscription ID
func GenerateUUID() string {
	newUUID, _ := uuid.NewV4()
	return newUUID.String()
}

// GenerateU128 generates random u128 string as subscription ID
func GenerateU128() (string, error) {
	u128 := make([]byte, 16)
	_, err := rand.Read(u128)
	if err != nil {
		return "", err
	}

	hexEncoded := hex.EncodeToString(u128)
	return "0x" + hexEncoded, nil
}
