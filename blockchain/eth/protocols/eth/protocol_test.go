package eth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
)

// TestBSCNegotiatesETH68 runs a real devp2p handshake between the gateway's advertised BSC
// protocols and peers advertising the eth versions seen on BSC mainnet, and checks both sides
// settle on eth/68, the only version whose BSC handshake is supported.
func TestBSCNegotiatesETH68(t *testing.T) {
	peers := []struct {
		name     string
		versions []uint
	}{
		{"reth-bsc eth/66-69", []uint{66, 67, 68, 69}},
		{"bsc-geth v1.8 eth/68,70", []uint{68, 70}},
		{"bsc-geth v1.7 eth/68", []uint{68}},
	}

	for _, chainID := range []uint64{network.BSCMainnetChainID, network.BSCTestnetChainID} {
		for _, peer := range peers {
			t.Run(fmt.Sprintf("chain %d/%s", chainID, peer.name), func(t *testing.T) {
				gwVersion, peerVersion := negotiate(t, MakeProtocols(context.Background(), nil, chainID), peer.versions)
				require.Equal(t, uint(ETH68), gwVersion, "gateway side")
				require.Equal(t, uint(ETH68), peerVersion, "peer side")
			})
		}
	}
}

func negotiate(t *testing.T, gwProtocols []p2p.Protocol, peerVersions []uint) (uint, uint) {
	t.Helper()

	gwCh, peerCh := make(chan uint, 1), make(chan uint, 1)

	gw := make([]p2p.Protocol, 0, len(gwProtocols))
	for _, p := range gwProtocols {
		gw = append(gw, recordingProtocol(p.Name, p.Version, p.Length, gwCh))
	}
	peer := make([]p2p.Protocol, 0, len(peerVersions))
	for _, v := range peerVersions {
		peer = append(peer, recordingProtocol(ProtocolName, v, 18, peerCh))
	}

	gwSrv := startServer(t, gw)
	startServer(t, peer).AddPeer(gwSrv.Self())

	return awaitVersion(t, gwCh), awaitVersion(t, peerCh)
}

func recordingProtocol(name string, version uint, length uint64, ch chan<- uint) p2p.Protocol {
	return p2p.Protocol{
		Name:    name,
		Version: version,
		Length:  length,
		Run: func(_ *p2p.Peer, rw p2p.MsgReadWriter) error {
			select {
			case ch <- version:
			default:
			}
			_, err := rw.ReadMsg()
			return err
		},
	}
}

func startServer(t *testing.T, protocols []p2p.Protocol) *p2p.Server {
	t.Helper()

	key, err := crypto.GenerateKey()
	require.NoError(t, err)

	srv := &p2p.Server{Config: p2p.Config{
		PrivateKey:  key,
		MaxPeers:    10,
		NoDiscovery: true,
		ListenAddr:  "127.0.0.1:0",
		Protocols:   protocols,
		Name:        "test",
	}}
	require.NoError(t, srv.Start())
	t.Cleanup(srv.Stop)

	return srv
}

func awaitVersion(t *testing.T, ch <-chan uint) uint {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("eth protocol was not negotiated")
		return 0
	}
}
