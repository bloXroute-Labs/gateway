package utils

import (
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestSSLCerts_NoKeysProvided(t *testing.T) {
	setupRegistrationFiles("test")
	defer cleanupFiles()

	sslCerts := NewSSLCerts(test.SSLTestPath, test.SSLTestPath, "test")
	assert.True(t, sslCerts.NeedsPrivateCert())

	privateKey, _ := parsePEMPrivateKey([]byte(test.PrivateKey))
	sslCerts.privateKey = *privateKey
	err := sslCerts.SavePrivateCert(test.PrivateCert)

	assert.Nil(t, err)
	assert.NotNil(t, sslCerts.privateCert)
	assert.NotNil(t, sslCerts.privateKeyPair)
}

func TestSSLCerts_SerializeRegistrationCert(t *testing.T) {
	setupRegistrationFiles("test")
	defer cleanupFiles()
	sslCerts := NewSSLCerts(test.SSLTestPath, test.SSLTestPath, "test")

	registrationCert, err := sslCerts.SerializeRegistrationCert()
	assert.Nil(t, err)
	assert.Equal(t, test.RegistrationCert, string(registrationCert))
}

func TestSSLCerts_LoadCACert(t *testing.T) {
	setupRegistrationFiles("test")
	setupPrivateFiles("test")
	SetupCAFiles()
	defer cleanupFiles()

	sslCerts := NewSSLCerts(test.SSLTestPath, test.SSLTestPath, "test")

	tlsConfig, err := sslCerts.LoadPrivateConfigWithCA(test.CACertPath)
	assert.Nil(t, err)
	assert.NotNil(t, tlsConfig)
}

func cleanupFiles() {
	_ = os.RemoveAll(test.SSLTestPath)
}
