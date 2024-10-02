package beacon

import (
	"fmt"
	"sync"

	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

type peers struct {
	peersByID map[libp2pPeer.ID]*peer

	mu *sync.RWMutex
}

func newPeers() peers {
	return peers{
		peersByID: make(map[libp2pPeer.ID]*peer),
		mu:        &sync.RWMutex{},
	}
}

func (p *peers) add(addrInfo libp2pPeer.AddrInfo) *peer {
	p.mu.Lock()
	defer p.mu.Unlock()

	pr := &peer{
		addrInfo: addrInfo,
	}
	p.peersByID[addrInfo.ID] = pr

	return pr
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
	addrInfo libp2pPeer.AddrInfo
	status   *ethpb.Status

	handshaking bool

	mu sync.Mutex
}

// String implements Stringer interface
func (p *peer) String() string {
	return fmt.Sprintf("{%v: %v}", p.addrInfo.ID, p.addrInfo.Addrs)
}

func (p *peer) startHandshake() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	connecting := p.handshaking

	p.handshaking = true

	return connecting
}

func (p *peer) finishHandshake() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.handshaking = false
}

func (p *peer) setStatus(status *ethpb.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.status = status
}

func (p *peer) getStatus() *ethpb.Status {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.status
}
