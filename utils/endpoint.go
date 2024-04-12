package utils

import (
	"fmt"
	"strconv"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/multiformats/go-multiaddr"
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

// MultiaddrToNodeEndoint converts a multiaddr to a NodeEndpoint
func MultiaddrToNodeEndoint(ma multiaddr.Multiaddr, blockchainNetwork string) types.NodeEndpoint {
	var ip, dns, port, pubKey string
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
			return true
		case multiaddr.P_P2P:
			pubKey = c.Value()
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
	}
}
