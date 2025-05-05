package beacon

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
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

func (p *peers) add(addrInfo libp2pPeer.AddrInfo, isDynamic bool) *peer {
	p.mu.Lock()
	defer p.mu.Unlock()

	if pr, ok := p.peersByID[addrInfo.ID]; ok {
		pr.addrInfo.Store(&addrInfo)
		return pr
	}

	pr := &peer{connected: &atomic.Bool{}}
	pr.addrInfo.Store(&addrInfo)
	pr.isDynamic.Store(isDynamic)

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
	addrInfo atomic.Pointer[libp2pPeer.AddrInfo]
	status   atomic.Pointer[ethpb.Status]

	handshaking    atomic.Bool
	connected      *atomic.Bool
	connectionTime atomic.Pointer[time.Time]
	isDynamic      atomic.Bool
}

// String implements Stringer interface
func (p *peer) String() string {
	addrInfo := p.addrInfo.Load()

	return fmt.Sprintf("{%v: %v}", addrInfo.ID, addrInfo.Addrs)
}

func (p *peer) startHandshake() bool {
	return p.handshaking.CompareAndSwap(false, true)
}

func (p *peer) finishHandshake() {
	p.handshaking.Store(false)
}

func (p *peer) setStatus(status *ethpb.Status) {
	p.status.Store(status)
}

func (p *peer) getStatus() *ethpb.Status {
	return p.status.Load()
}

func (p *peer) connect() bool {
	if p.connected.CompareAndSwap(false, true) {
		t := time.Now()
		p.connectionTime.Store(&t)
		return true
	}

	return false
}

func (p *peer) disconnect() bool {
	return p.connected.CompareAndSwap(true, false)
}

func (p *peer) connectedAt() *time.Time {
	return p.connectionTime.Load()
}
