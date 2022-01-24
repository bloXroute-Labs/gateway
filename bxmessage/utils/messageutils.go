package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
)

// IPV4Prefix is the IPV4 address prefix in bytes
var IPV4Prefix = []byte("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff")

// IPAddrSizeInBytes is byte length of an IP Address
const IPAddrSizeInBytes = 16

// IPV4PrefixLength is the byte length of the IPV4 prefix
const IPV4PrefixLength = 12

// UnpackIPPort unpacks IPV4 address bytes into string format
func UnpackIPPort(buf []byte) (string, uint16, error) {
	port := binary.LittleEndian.Uint16(buf[IPAddrSizeInBytes:])
	if bytes.Compare(buf[:+IPV4PrefixLength], IPV4Prefix[:]) == 0 {
		ipBytes := buf[IPV4PrefixLength:IPAddrSizeInBytes]
		return ipv4ToStringFormat(ipBytes), port, nil
	}
	return "", 0, fmt.Errorf("IP address is not in IPV4 format")
}

// PackIPPort packs an IP address and port in IPV4 format
func PackIPPort(buf []byte, ip string, port uint16) {
	copy(buf[:], IPV4Prefix)
	offset := IPV4PrefixLength
	ipv4 := net.ParseIP(ip)
	ipv4Decimal := ipv4toInt(ipv4)
	binary.BigEndian.PutUint32(buf[offset:], uint32(ipv4Decimal))
	offset = IPAddrSizeInBytes
	binary.LittleEndian.PutUint16(buf[offset:], port)
}

func ipv4ToStringFormat(buf []byte) string {
	val := binary.LittleEndian.Uint32(buf)
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, val)
	return ip.String()
}

func ipv4toInt(ipv4Address net.IP) int64 {
	ipv4Int := big.NewInt(0)
	ipv4Int.SetBytes(ipv4Address.To4())
	return ipv4Int.Int64()
}
