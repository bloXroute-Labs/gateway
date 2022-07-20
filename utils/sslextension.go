package utils

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// BxSSLProperties represents extension data encoded in bloxroute SSL certificates
type BxSSLProperties struct {
	NodeType       NodeType
	NodeID         types.NodeID
	AccountID      types.AccountID
	NodePrivileges string // not currently used in Ethereum
}

// ErrNodeIDNotEmbedded indicates that the provided certificate does not have a node ID. This is an important error e.g. when establishing connections
var ErrNodeIDNotEmbedded = errors.New("node ID not embedded")

// Extension ID types encoded in TLS certificates
const (
	nodeTypeExtensionID       = "1.22.333.4444"
	nodeIDExtensionID         = "1.22.333.4445"
	accountIDExtensionID      = "1.22.333.4446"
	nodePrivilegesExtensionID = "1.22.333.4447"
)

// general extension IDs
const (
	subjectKeyID       = "2.5.29.14"
	keyUsageID         = "2.5.29.15"
	subjectAltNameID   = "2.5.29.17"
	basicConstraintsID = "2.5.29.19"
	authorityKeyID     = "2.5.29.35"
)

// ParseBxCertificate extracts bloXroute specific extension information from the SSL certificates
func ParseBxCertificate(certificate *x509.Certificate) (BxSSLProperties, error) {
	var (
		nodeType        NodeType
		nodeID          types.NodeID
		accountID       types.AccountID
		nodePrivileges  string
		err             error
		bxSSLExtensions BxSSLProperties
	)

	for _, extension := range certificate.Extensions {
		switch extension.Id.String() {
		case nodeTypeExtensionID:
			nodeType, err = DeserializeNodeType(extension.Value)
			if err != nil {
				return bxSSLExtensions, err
			}
		case nodeIDExtensionID:
			nodeID = types.NodeID(extension.Value)
		case accountIDExtensionID:
			accountID = types.AccountID(extension.Value)
		case nodePrivilegesExtensionID:
			nodePrivileges = string(extension.Value)
		case subjectKeyID:
		case keyUsageID:
		case subjectAltNameID:
		case basicConstraintsID:
		case authorityKeyID:
		default:
			log.Debugf("found an unexpected extension in TLS certificate: %v => %v", extension.Id, extension.Value)
		}
	}

	if nodeID == "" {
		err = ErrNodeIDNotEmbedded
	}
	bxSSLExtensions = BxSSLProperties{
		NodeType:       nodeType,
		NodeID:         nodeID,
		AccountID:      accountID,
		NodePrivileges: nodePrivileges,
	}
	return bxSSLExtensions, err
}

// GetAccountIDFromBxCertificate get account ID from cert
func GetAccountIDFromBxCertificate(extensions []pkix.Extension) (types.AccountID, error) {
	var accountID types.AccountID
	for _, extension := range extensions {
		if extension.Id.String() == accountIDExtensionID {
			accountID = types.AccountID(extension.Value)
			return accountID, nil
		}
	}

	return accountID, errors.New("extension not found in cert")
}
