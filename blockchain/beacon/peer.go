package beacon

import (
	"sync"

	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
)

type peers struct {
	peersByID map[libp2pPeer.ID]*peer

	mu sync.RWMutex
}

func newPeers() peers {
	return peers{
		peersByID: make(map[libp2pPeer.ID]*peer),
	}
}

func (p *peers) add(addrInfo *libp2pPeer.AddrInfo, remoteAddr ma.Multiaddr) *peer {
	p.mu.Lock()
	defer p.mu.Unlock()

	peer := &peer{
		addrInfo:   addrInfo,
		remoteAddr: remoteAddr,
	}

	p.peersByID[addrInfo.ID] = peer

	return peer
}

func (p *peers) get(peerID libp2pPeer.ID) *peer {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.peersByID[peerID]
}

func (p *peers) rangeByID(f func(libp2pPeer.ID, *peer) bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for id, peer := range p.peersByID {
		p.mu.RUnlock()
		cont := f(id, peer)
		p.mu.RLock()

		if !cont {
			break
		}
	}
}

type peer struct {
	addrInfo   *libp2pPeer.AddrInfo
	remoteAddr ma.Multiaddr
	status     *ethpb.Status

	isHandshaking bool

	mu sync.Mutex
}

// String implements Stringer interface
func (p *peer) String() string {
	return p.remoteAddr.String()
}

func (p *peer) handshaking() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	connecting := p.isHandshaking

	p.isHandshaking = true

	return connecting
}

func (p *peer) finishedHandshaking() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.isHandshaking = false
}

func (p *peer) setStatus(status *ethpb.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = status
}
