package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"io/ioutil"
	"os"
	"path"
)

// SSLCerts represents the required certificate files for interacting with the BDN.
// Private keys/certs are used for TLS socket connections, and registration only keys/certs
// are for creating a CSR for bxapi to return the signed private cert.
type SSLCerts struct {
	privateCertFile          string
	privateKeyFile           string
	registrationOnlyCertFile string
	registrationOnlyKeyFile  string

	privateCert               *x509.Certificate
	privateKey                ecdsa.PrivateKey
	privateKeyPair            *tls.Certificate
	registrationOnlyCert      x509.Certificate
	registrationOnlyCertBlock []byte
	registrationOnlyKey       ecdsa.PrivateKey
	registrationOnlyKeyPair   tls.Certificate
}

// NewSSLCerts returns and initializes new storage of SSL certificates.
// Registration only keys/certs are mandatory. If they cannot be loaded, this function will panic.
// Private keys and certs must match each other. If they do not, a new private key will be generated
// and written, pending loading of a new certificate.
func NewSSLCerts(registrationOnlyBaseURL, privateBaseURL, certName string) SSLCerts {
	var (
		privateDir          = path.Join(privateBaseURL, certName, "private")
		registrationOnlyDir = path.Join(registrationOnlyBaseURL, certName, "registration_only")
	)

	_ = os.MkdirAll(privateDir, 0755)
	_ = os.MkdirAll(registrationOnlyDir, 0755)

	var (
		privateCertFile          = path.Join(privateDir, fmt.Sprintf("%v_cert.pem", certName))
		privateKeyFile           = path.Join(privateDir, fmt.Sprintf("%v_key.pem", certName))
		registrationOnlyCertFile = path.Join(registrationOnlyDir, fmt.Sprintf("%v_cert.pem", certName))
		registrationOnlyKeyFile  = path.Join(registrationOnlyDir, fmt.Sprintf("%v_key.pem", certName))
	)

	registrationOnlyCertBlock, err := ioutil.ReadFile(registrationOnlyCertFile)
	if err != nil {
		panic(fmt.Errorf("could not read registration only cert from file (%v): %v", registrationOnlyCertFile, err))
	}
	registrationOnlyCert, err := parsePEMCert(registrationOnlyCertBlock)
	if err != nil {
		panic(fmt.Errorf("could not parse PEM data from registration only cert: %v", err))
	}

	registrationOnlyKeyBlock, err := ioutil.ReadFile(registrationOnlyKeyFile)
	if err != nil {
		panic(fmt.Errorf("could not read registration only key from file (%v): %v", registrationOnlyKeyFile, err))
	}
	registrationOnlyKey, err := parsePEMPrivateKey(registrationOnlyKeyBlock)
	if err != nil {
		panic(fmt.Errorf("could not parse PEM data from registration only key: %v", err))
	}
	registrationOnlyKeyPair, err := tls.X509KeyPair(registrationOnlyCertBlock, registrationOnlyKeyBlock)
	if err != nil {
		panic(fmt.Errorf("could not load registration only key pair: %v", err))
	}

	var privateKey *ecdsa.PrivateKey
	var privateCert *x509.Certificate
	var privateKeyPair *tls.Certificate

	privateCertBlock, certErr := ioutil.ReadFile(privateCertFile)
	if certErr != nil {
		privateCertBlock = nil
	}
	privateKeyBlock, keyErr := ioutil.ReadFile(privateKeyFile)
	if keyErr != nil {
		privateKeyBlock = nil
	}

	if privateKeyBlock == nil && privateCertBlock == nil {
		privateKey, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			panic(fmt.Errorf("could not generate private key: %v", err))
		}
		privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
		if err != nil {
			panic(fmt.Errorf("could not marshal generated private key: %v", err))
		}
		privateKeyBlock = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyBytes})
		err = ioutil.WriteFile(privateKeyFile, privateKeyBlock, 0644)
		if err != nil {
			panic(fmt.Errorf("could not write new private key to file (%v): %v", privateKeyFile, err))
		}
	} else if privateKeyBlock != nil && privateCertBlock != nil {
		privateKey, err = parsePEMPrivateKey(privateKeyBlock)
		if err != nil {
			panic(fmt.Errorf("could not parse private key from file (%v): %v", privateKeyFile, err))
		}
		privateCert, err = parsePEMCert(privateCertBlock)
		if err != nil {
			panic(fmt.Errorf("could not parse private cert from file (%v): %v", privateCert, err))
		}
		_privateKeyPair, err := tls.X509KeyPair(privateCertBlock, privateKeyBlock)
		privateKeyPair = &_privateKeyPair
		if err != nil {
			panic(fmt.Errorf("could not load private key pair: %v", err))
		}
	} else if privateKeyBlock != nil {
		privateKey, err = parsePEMPrivateKey(privateKeyBlock)
		if err != nil {
			panic(fmt.Errorf("could not parse private key from file (%v): %v", privateKeyFile, err))
		}
	} else {
		panic(fmt.Errorf("found a certificate with no matching private key –– delete the certificate at %v if it's not needed", privateCertFile))
	}

	return SSLCerts{
		privateCertFile: privateCertFile,
		privateKeyFile:  privateKeyFile,
		privateCert:     privateCert,
		privateKey:      *privateKey,
		privateKeyPair:  privateKeyPair,

		registrationOnlyCertFile:  registrationOnlyCertFile,
		registrationOnlyKeyFile:   registrationOnlyKeyFile,
		registrationOnlyCert:      *registrationOnlyCert,
		registrationOnlyCertBlock: registrationOnlyCertBlock,
		registrationOnlyKey:       *registrationOnlyKey,
		registrationOnlyKeyPair:   registrationOnlyKeyPair,
	}
}

func parsePEMPrivateKey(block []byte) (*ecdsa.PrivateKey, error) {
	decodedKeyBlock, _ := pem.Decode(block)
	keyBytes := decodedKeyBlock.Bytes
	return x509.ParseECPrivateKey(keyBytes)
}

func parsePEMCert(block []byte) (*x509.Certificate, error) {
	decodedCertBlock, _ := pem.Decode(block)
	certBytes := decodedCertBlock.Bytes
	return x509.ParseCertificate(certBytes)
}

// NeedsPrivateCert indicates if SSL storage has been populated with the private certificate
func (s SSLCerts) NeedsPrivateCert() bool {
	return s.privateCert == nil
}

// CreateCSR returns a PEM encoded x509.CertificateRequest, generated using the registration only cert template
// and signed with the private key
func (s SSLCerts) CreateCSR() ([]byte, error) {
	certRequest := x509.CertificateRequest{
		Subject:    s.registrationOnlyCert.Subject,
		Extensions: s.registrationOnlyCert.Extensions,
	}
	x509CR, err := x509.CreateCertificateRequest(rand.Reader, &certRequest, &s.privateKey)
	if err != nil {
		return nil, err
	}
	serializedCR := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: x509CR})
	return serializedCR, nil
}

// SerializeRegistrationCert returns the PEM encoded registration x509.Certificate
func (s SSLCerts) SerializeRegistrationCert() ([]byte, error) {
	return s.registrationOnlyCertBlock, nil
}

// SavePrivateCert saves the private certificate and updates the key pair.
func (s *SSLCerts) SavePrivateCert(privateCert string) error {
	privateCertBytes := []byte(privateCert)
	cert, err := parsePEMCert(privateCertBytes)
	if err != nil {
		return fmt.Errorf("could not parse private cert: %v", err)
	}
	s.privateCert = cert

	privateKeyBytes, err := x509.MarshalECPrivateKey(&s.privateKey)
	if err != nil {
		return fmt.Errorf("stored private key was somehow invalid: %v", err)
	}
	privateKeyBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyBytes})
	privateKeyPair, err := tls.X509KeyPair(privateCertBytes, privateKeyBlock)
	if err != nil {
		return fmt.Errorf("could not parse private key pair: %v", err)
	}
	s.privateKeyPair = &privateKeyPair

	return ioutil.WriteFile(s.privateCertFile, privateCertBytes, 0644)
}

// LoadPrivateConfig generates TLS config from the private certificates.
// The resulting config can be used for any bxapi or socket communications.
func (s SSLCerts) LoadPrivateConfig() (*tls.Config, error) {
	if s.privateKeyPair == nil {
		return nil, errors.New("private key pair has not been loaded")
	}
	config := &tls.Config{
		Certificates:       []tls.Certificate{*s.privateKeyPair},
		InsecureSkipVerify: true,
	}
	return config, nil
}

// LoadPrivateConfigWithCA generates TLS config from the private certificate.
// The resulting config can be used to configure a server that allows inbound connections.
func (s SSLCerts) LoadPrivateConfigWithCA(caPath string) (*tls.Config, error) {
	if s.privateKeyPair == nil {
		return nil, errors.New("private key pair has not been loaded")
	}

	caCertPEM, err := ioutil.ReadFile(caPath)
	if err != nil {
		return nil, err
	}

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(caCertPEM)
	if !ok {
		panic("failed to parse root certificate")
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{*s.privateKeyPair},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    roots,
	}
	return config, nil
}

// GetNodeID reads the node ID embedded in the private certificate storage
func (s SSLCerts) GetNodeID() (types.NodeID, error) {
	if s.privateCert == nil {
		return "", errors.New("private certificate has not been loaded")
	}

	sslProperties, err := ParseBxCertificate(s.privateCert)
	if err != nil {
		return "", err
	}

	return sslProperties.NodeID, nil
}

// GetAccountID reads the account ID embedded in the local certificates
func (s SSLCerts) GetAccountID() (types.AccountID, error) {
	sslProperties, err := ParseBxCertificate(&s.registrationOnlyCert)
	if err != nil {
		return "", err
	}
	return sslProperties.AccountID, nil
}

// LoadRegistrationConfig generates TLS config from the registration only certificate.
// The resulting config can only be used to register the node with bxapi, which will
// then return a private certificate for future use.
func (s SSLCerts) LoadRegistrationConfig() (*tls.Config, error) {
	config := &tls.Config{
		Certificates:       []tls.Certificate{s.registrationOnlyKeyPair},
		InsecureSkipVerify: true,
	}
	return config, nil
}
