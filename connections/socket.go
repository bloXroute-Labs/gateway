package connections

import (
	"crypto/tls"
	"net"
	"strconv"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// Socket represents an in between interface between connection objects and network sockets
type Socket interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	Close(string) error
	Equals(s Socket) bool

	SetReadDeadline(t time.Time) error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Properties() (utils.BxSSLProperties, error)
}

const dialTimeout = 30 * time.Second

// TLS wraps a raw TLS network connection to implement the Socket interface
type TLS struct {
	*tls.Conn
}

// NewTLS dials and creates a new TLS connection
func NewTLS(ip string, port int, certs *utils.SSLCerts) (*TLS, error) {
	config, err := certs.LoadPrivateConfig()
	if err != nil {
		log.Errorf("servers: loadkeys: %s", err)
		return nil, err
	}
	ipAddress := ip + ":" + strconv.Itoa(port)
	ipConn, err := net.DialTimeout("tcp", ipAddress, dialTimeout)
	if err != nil {
		return nil, err
	}

	tlsClient := tls.Client(ipConn, config)
	err = tlsClient.Handshake()
	if err != nil {
		return nil, err
	}

	return &TLS{Conn: tlsClient}, nil
}

// NewTLSFromConn creates a new TLS wrapper on an existing TLS connection
func NewTLSFromConn(conn *tls.Conn) *TLS {
	return &TLS{Conn: conn}
}

// Properties returns the SSL properties embedded in TLS certificates
func (t TLS) Properties() (utils.BxSSLProperties, error) {
	state := t.Conn.ConnectionState()
	var (
		err             error
		bxSSLExtensions utils.BxSSLProperties
	)
	for _, peerCertificate := range state.PeerCertificates {
		bxSSLExtensions, err = utils.ParseBxCertificate(peerCertificate)
		if err == nil {
			break
		}
	}
	return bxSSLExtensions, err
}

// Close closes the underlying TLS connection
func (t TLS) Close(reason string) error {
	return t.Conn.Close()
}

// Equals compares two connection IDs
func (t TLS) Equals(s Socket) bool {
	if s == nil {
		return false
	}

	tl, ok := s.(*TLS)
	if !ok {
		return false
	}

	if tl == nil {
		return false
	}

	return t == *tl
}
