package utils

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

var ipPortBytes = []byte("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\x7f6\x03\x01@\x1f")

func TestUnpackIPPort(t *testing.T) {
	testIPV4 := "127.54.3.1"
	testPort := uint16(8000)
	ip, port, err := UnpackIPPort(ipPortBytes)
	assert.Equal(t, ip, testIPV4)
	assert.Equal(t, port, testPort)
	assert.Nil(t, err)
}

func TestPackIPPort(t *testing.T) {
	testIPV4 := "127.54.3.1"
	testPort := uint16(8000)

	buf := make([]byte, IPAddrSizeInBytes+types.UInt16Len)
	PackIPPort(buf, testIPV4, testPort)
	assert.Equal(t, ipPortBytes, buf)
}
