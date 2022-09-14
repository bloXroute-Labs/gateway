package beacon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/host"
	p2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/encoder"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v3/crypto/ecdsa"
)

// special error constant types
var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrBodyNotFound   = errors.New("block body not stored")
	ErrAlreadySeen    = errors.New("already seen")
	ErrAncientHeaders = errors.New("headers requested are ancient")
	ErrFutureHeaders  = errors.New("headers requested are in the future")
)

// readStatusCode response from a RPC stream.
func readStatusCode(stream p2pNetwork.Stream, encoding encoder.NetworkEncoding) error {
	b := make([]byte, 1)
	_, err := stream.Read(b)
	if err != nil {
		return err
	}

	if b[0] != responseCodeSuccess {
		return errResponseNotOK
	}

	return nil
}

func parseMultiAddr(addr ma.Multiaddr) (string, int, error) {
	var ip, port string

	ma.ForEach(addr, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_IP6:
			ip = c.Value()
			return true
		case ma.P_IP4:
			ip = c.Value()
			return true
		case ma.P_TCP:
			port = c.Value()
			return true
		}

		return false
	})

	p, err := strconv.Atoi(port)
	if err != nil {
		return "", 0, fmt.Errorf("could not convert port to int: %v", err)
	}

	return ip, p, nil
}

// ensurePeerConnections will attempt to reestablish connection to the peers
// if there are currently no connections to that peer.
func ensurePeerConnections(ctx context.Context, h host.Host, peers ...string) {
	if len(peers) == 0 {
		return
	}
	for _, p := range peers {
		if p == "" {
			continue
		}

		addr, err := parseEnodeMultiAddr(p)
		if err != nil {
			log.Errorf("Could not parse multiAddr: %v", err)
			continue
		}

		peerInfo, err := p2p.MakePeer(addr)
		if err != nil {
			log.Errorf("Could not make peer %v: %v", addr, err)
			continue
		}

		c := h.Network().ConnsToPeer(peerInfo.ID)
		if len(c) == 0 {
			if err := connectWithTimeout(ctx, h, peerInfo); err != nil {
				log.WithField("peer", peerInfo.ID).WithField("addrs", peerInfo.Addrs).Errorf("Failed to reconnect to peer: %v", err)
				continue
			}
		}
	}
}

func parseEnodeMultiAddr(enodeStr string) (string, error) {
	node, err := enode.Parse(enode.ValidSchemes, enodeStr)
	if err != nil {
		return "", fmt.Errorf("could not parse enode: %v", err)
	}

	pubkey := node.Pubkey()
	assertedKey, err := ecdsaprysm.ConvertToInterfacePubkey(pubkey)
	if err != nil {
		return "", fmt.Errorf("could not get pubkey: %v", err)
	}
	id, err := peer.IDFromPublicKey(assertedKey)
	if err != nil {
		return "", fmt.Errorf("could not get peer id: %v", err)
	}

	parsedIP := net.ParseIP(node.IP().String())
	if parsedIP.To4() != nil {
		return fmt.Sprintf("/ip4/%s/%s/%d/p2p/%s", node.IP().String(), "tcp", uint(node.TCP()), id.String()), nil
	}
	if parsedIP.To16() != nil {
		return fmt.Sprintf("/ip6/%s/%s/%d/p2p/%s", node.IP().String(), "tcp", uint(node.TCP()), id.String()), nil
	}
	return "", fmt.Errorf("invalid ip address provided: %s", node.IP().String())
}

func connectWithTimeout(ctx context.Context, h host.Host, peer *peer.AddrInfo) error {
	log.WithField("peer", peer.ID).Debug("No connections to peer, reconnecting")
	ctx, cancel := context.WithTimeout(ctx, params.BeaconNetworkConfig().RespTimeout)
	defer cancel()
	return h.Connect(ctx, *peer)
}

func logBlockConverterFailure(err error, bdnBlock *bxTypes.BxBlock) {
	blockHex := "<omitted>"
	if log.IsLevelEnabled(log.TraceLevel) {
		b, err := rlp.EncodeToBytes(bdnBlock)
		if err != nil {
			log.Error("bad block from BDN could not be encoded to RLP bytes")
			return
		}
		blockHex = hexutil.Encode(b)
	}
	log.Errorf("could not convert block (hash: %v) from BDN to Ethereum block: %v. contents: %v", bdnBlock.Hash(), err, blockHex)
}
