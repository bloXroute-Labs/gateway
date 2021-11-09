package utils

import (
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/test"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestSSLCerts_NoKeysProvided(t *testing.T) {
	setupRegistrationFiles()
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
	setupRegistrationFiles()
	defer cleanupFiles()

	sslCerts := NewSSLCerts(test.SSLTestPath, test.SSLTestPath, "test")
	registrationCert, err := sslCerts.SerializeRegistrationCert()
	assert.Nil(t, err)
	assert.Equal(t, test.RegistrationCert, string(registrationCert))
}

func TestSSLCerts_LoadCACert(t *testing.T) {
	setupRegistrationFiles()
	setupPrivateFiles()
	setupCAFiles()
	defer cleanupFiles()

	sslCerts := NewSSLCerts(test.SSLTestPath, test.SSLTestPath, "test")

	tlsConfig, err := sslCerts.LoadPrivateConfigWithCA(test.CACertPath)
	assert.Nil(t, err)
	assert.NotNil(t, tlsConfig)
}

// code below this point is duplicated with bxmock/ssl_certs.go to avoid circular imports

func makeFolders(name string) {
	privatePath := path.Join(test.SSLTestPath, name, "private")
	registrationPath := path.Join(test.SSLTestPath, name, "registration_only")
	err := os.MkdirAll(privatePath, 0755)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(registrationPath, 0755)
	if err != nil {
		panic(err)
	}
}

func setupRegistrationFiles() {
	makeFolders("test")
	writeCerts("registration_only", "test", test.RegistrationCert, test.RegistrationKey)
}

func setupPrivateFiles() {
	makeFolders("test")
	writeCerts("private", "test", test.PrivateCert, test.PrivateKey)
}

func setupCAFiles() {
	err := os.MkdirAll(test.CACertFolder, 0755)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(test.CACertPath, []byte(test.CACert), 0644)
	if err != nil {
		panic(err)
	}
}

func writeCerts(folder, name, cert, key string) {
	p := path.Join(test.SSLTestPath, name, folder)
	keyPath := path.Join(p, fmt.Sprintf("%v_cert.pem", name))
	certPath := path.Join(p, fmt.Sprintf("%v_key.pem", name))

	err := ioutil.WriteFile(keyPath, []byte(cert), 0644)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(certPath, []byte(key), 0644)
	if err != nil {
		panic(err)
	}
}

func cleanupFiles() {
	_ = os.RemoveAll(test.SSLTestPath)
}
