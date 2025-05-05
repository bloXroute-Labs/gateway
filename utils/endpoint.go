package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/multiformats/go-multiaddr"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// EnodeToNodeEndpoint converts an enode to a NodeEndpoint
func EnodeToNodeEndpoint(node *enode.Node, blockchainNetwork string) types.NodeEndpoint {
	pubKey := fmt.Sprintf("%x", crypto.FromECDSAPub(node.Pubkey())[1:])
	return types.NodeEndpoint{
		IP:                node.IP().String(),
		Port:              node.TCP(),
		PublicKey:         pubKey,
		IsBeacon:          false,
		BlockchainNetwork: blockchainNetwork,
	}
}

// MultiaddrToNodeEndpoint converts a multiaddr to a NodeEndpoint
func MultiaddrToNodeEndpoint(ma multiaddr.Multiaddr, blockchainNetwork string) types.NodeEndpoint {
	var ip, dns, port, pubKey, connType string
	multiaddr.ForEach(ma, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP6:
			ip = c.Value()
			return true
		case multiaddr.P_IP4:
			ip = c.Value()
			return true
		case multiaddr.P_DNS:
			dns = c.Value()
			return true
		case multiaddr.P_TCP:
			port = c.Value()
			connType = "tcp"
			return true
		case multiaddr.P_UDP:
			port = c.Value()
			connType = "udp"
			return true
		case multiaddr.P_P2P:
			pubKey = c.Value()
			return true
		case multiaddr.P_QUIC, multiaddr.P_QUIC_V1:
			connType = "quic"
			return true
		}

		return false
	})

	// Should be validated before
	p, _ := strconv.Atoi(port)

	return types.NodeEndpoint{
		IP:                ip,
		DNS:               dns,
		Port:              p,
		PublicKey:         pubKey,
		IsBeacon:          true,
		BlockchainNetwork: blockchainNetwork,
		ConnectionType:    connType,
	}
}

// CreatePrysmEndpoint creates prysm endpoint
func CreatePrysmEndpoint(prysmAddr, blockchainNetwork string) (types.NodeEndpoint, error) {
	parts := strings.Split(prysmAddr, ":")
	if len(parts) < 2 {
		return types.NodeEndpoint{}, fmt.Errorf("invalid addr format, addr %v", prysmAddr)
	}
	prysmPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return types.NodeEndpoint{}, fmt.Errorf("error getting port from prysm addr %v: %v", prysmAddr, err)
	}
	return types.NodeEndpoint{
		IP:                parts[0],
		Port:              prysmPort,
		IsBeacon:          true,
		BlockchainNetwork: blockchainNetwork,
		Name:              "Prysm",
	}, nil
}

// CreateAPIEndpoint creates NodeEndpoint object from uri:port string of beacon API endpoint
func CreateAPIEndpoint(url, blockchainNetwork string) (types.NodeEndpoint, error) {
	urlSplitted := strings.Split(url, ":")
	port, err := strconv.Atoi(urlSplitted[1])
	if err != nil {
		return types.NodeEndpoint{}, fmt.Errorf("failed to retrieve endpoint port: %v", err)
	}

	return types.NodeEndpoint{
		IP:                urlSplitted[0],
		Port:              port,
		PublicKey:         "BeaconAPI",
		IsBeacon:          true,
		BlockchainNetwork: blockchainNetwork,
		Name:              "BeaconAPI",
		ConnectedAt:       time.Now().Format(time.RFC3339),
	}, nil
}
