package rpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	user       = "d2ab93ba-1a8b-4187-994e-c63896c936e3"
	secret     = "a61681a9456ca319a90036e8361dbc73"
	authHeader = "ZDJhYjkzYmEtMWE4Yi00MTg3LTk5NGUtYzYzODk2YzkzNmUzOmE2MTY4MWE5NDU2Y2EzMTlhOTAwMzZlODM2MWRiYzcz"
)

func TestEncodeUserSecret(t *testing.T) {
	result := EncodeUserSecret(user, secret)
	assert.Equal(t, authHeader, result)
}

func TestDecodeAuthHeader(t *testing.T) {
	decodedUser, decodedSecret, err := DecodeAuthHeader(authHeader)
	assert.Nil(t, err)
	assert.Equal(t, user, decodedUser)
	assert.Equal(t, secret, decodedSecret)
}
