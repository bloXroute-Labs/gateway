package beacon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p-core/host"
	p2pNetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	swarm "github.com/libp2p/go-libp2p-swarm"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/encoder"
	p2pTypes "github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	ecdsaprysm "github.com/prysmaticlabs/prysm/v3/crypto/ecdsa"
)

// special error constant types
var (
	ErrInvalidRequest        = errors.New("invalid request")
	ErrBodyNotFound          = errors.New("block body not stored")
	ErrAlreadySeen           = errors.New("already seen")
	ErrAncientHeaders        = errors.New("headers requested are ancient")
	ErrFutureHeaders         = errors.New("headers requested are in the future")
	ErrQueryAmountIsNotValid = errors.New("query amount is not valid")
)

// readStatusCode response from a RPC stream.
func readStatusCode(stream p2pNetwork.Stream, encoding encoder.NetworkEncoding) (uint8, string, error) {
	// Set ttfb deadline.
	SetStreamReadDeadline(stream, params.BeaconNetworkConfig().TtfbTimeout)
	b := make([]byte, 1)
	_, err := stream.Read(b)
	if err != nil {
		return 0, "", err
	}

	if b[0] == responseCodeSuccess {
		// Set response deadline on a successful response code.
		SetStreamReadDeadline(stream, params.BeaconNetworkConfig().TtfbTimeout)

		return 0, "", nil
	}

	// Set response deadline, when reading error message.
	SetStreamReadDeadline(stream, params.BeaconNetworkConfig().RespTimeout)
	msg := &p2pTypes.ErrorMessage{}
	if err := encoding.DecodeWithMaxLength(stream, msg); err != nil {
		return 0, "", err
	}

	return b[0], string(*msg), nil
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
				log.WithField("peer", peerInfo.ID).WithField("addrs", peerInfo.Addrs).Warnf("Failed to reconnect to peer: %v", err)

				// Try to reconnect as fast as possible again
				// https://github.com/libp2p/go-libp2p/blob/ddfb6f9240679b840d3663021e8b4433f51379a7/examples/relay/main.go#L90
				h.Network().(*swarm.Swarm).Backoff().Clear(peerInfo.ID)

				h.Network().Peerstore().RemovePeer(peerInfo.ID)
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
	var blockHex string
	if log.IsLevelEnabled(log.TraceLevel) {
		b, err := rlp.EncodeToBytes(bdnBlock)
		if err != nil {
			blockHex = fmt.Sprintf("bad block from BDN could not be encoded to RLP bytes: %v", err)
		} else {
			blockHex = hexutil.Encode(b)
		}
	}
	log.Errorf("could not convert block (hash: %v) from BDN to beacon block: %v. contents: %v", bdnBlock.Hash(), err, blockHex)
}

func replaceForkDigest(topic string) (string, error) {
	subStrings := strings.Split(topic, "/")
	if len(subStrings) != 4 {
		return "", errors.New("invalid topic")
	}
	subStrings[2] = "%x"

	return strings.Join(subStrings, "/"), nil
}

func currentSlot(genesisTime uint64) types.Slot {
	return types.Slot(uint64(time.Now().Unix()-int64(genesisTime)) / params.BeaconConfig().SecondsPerSlot)
}
