package eth

import (
	"errors"
	"sync"
)

type peerSet struct {
	peers map[string]*Peer
	lock  sync.RWMutex
}

func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*Peer),
	}
}

func (ps *peerSet) register(ep *Peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := ep.ID()
	if _, ok := ps.peers[id]; ok {
		return errors.New("peer already registered")
	}
	ps.peers[ep.ID()] = ep
	return nil
}

func (ps *peerSet) unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if _, ok := ps.peers[id]; !ok {
		return errors.New("peer does not exist")
	}

	delete(ps.peers, id)
	return nil
}

func (ps *peerSet) get(id string) (*Peer, bool) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	p, ok := ps.peers[id]
	return p, ok
}

func (ps *peerSet) getAll() []*Peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	peers := make([]*Peer, 0, len(ps.peers))
	for _, peer := range ps.peers {
		peers = append(peers, peer)
	}
	return peers
}
