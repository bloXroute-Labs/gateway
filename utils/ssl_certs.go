package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/bloXroute-Labs/gateway/v2/test"
)

// TestCerts uses the test certs specified in constants to return an utils.SSLCerts object for connection testing
func TestCerts() SSLCerts {
	defer CleanupSSLCerts()
	SetupSSLFiles("test")
	return TestCertsWithoutSetup()
}

// TestCertsWithoutSetup uses the test certs specified in constants to return an utils.SSLCerts object for connection testing. This function does not do any setup/teardown of writing said files temporarily to disk.
func TestCertsWithoutSetup() SSLCerts {
	return NewSSLCerts(test.SSLTestPath, test.SSLTestPath, "test")
}

// SetupSSLFiles writes the fixed test certificates to disk for loading into an SSL context.
func SetupSSLFiles(certName string) {
	setupRegistrationFiles(certName)
	setupPrivateFiles(certName)
	SetupCAFiles()
}

func setupRegistrationFiles(certName string) {
	makeFolders(certName)
	writeCerts("registration_only", certName, test.RegistrationCert, test.RegistrationKey)
}

func setupPrivateFiles(certName string) {
	makeFolders(certName)
	writeCerts("private", certName, test.PrivateCert, test.PrivateKey)
}

// SetupCAFiles writes the CA files to disk for loading into an SSL context.
func SetupCAFiles() {
	err := os.MkdirAll(test.CACertFolder, 0755)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(test.CACertPath, []byte(test.CACert), 0644)
	if err != nil {
		panic(err)
	}
}

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

// CleanupSSLCerts clears the temporary SSL certs written to disk.
func CleanupSSLCerts() {
	_ = os.RemoveAll(test.SSLTestPath)
}
