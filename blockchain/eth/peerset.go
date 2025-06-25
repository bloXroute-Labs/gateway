package eth

import (
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/eth/protocols/eth"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/bsc"
	eth2 "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/eth"
)

const (
	// extensionWaitTimeout is the maximum allowed time for the extension wait to
	// complete before dropping the connection as malicious.
	extensionWaitTimeout = 10 * time.Second
	tryWaitTimeout       = 100 * time.Millisecond
)

var (
	// errPeerAlreadyRegistered is returned if a peer is attempted to be added
	// to the peer set, but one with the same id already exists.
	errPeerAlreadyRegistered = errors.New("peer already registered")

	// errBscWithoutEth is returned if a peer attempts to connect only on the
	// bsc protocol without advertising the eth main protocol.
	errBscWithoutEth = errors.New("peer connected on bsc without compatible eth support")

	// errPeerWaitTimeout is returned if a peer waits extension for too long
	errPeerWaitTimeout = errors.New("peer wait timeout")
)

type peerSet struct {
	peers map[string]ethPeer // Peers connected on the `eth` protocol
	lock  sync.RWMutex

	bscWait map[string]chan bsc.Peer // Peers connected on `eth` waiting for their bsc extension
	bscPend map[string]bsc.Peer      // Peers connected on the `bsc` protocol, but not yet on `eth`
}

func newPeerSet() *peerSet {
	return &peerSet{
		peers:   make(map[string]ethPeer),
		bscWait: make(map[string]chan bsc.Peer),
		bscPend: make(map[string]bsc.Peer),
	}
}

// register injects a new `eth` peer into the working set, or returns an error
// if the peer is already known.
func (ps *peerSet) register(peer *eth2.Peer, bscExt *bsc.Peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered
	}

	ep := ethPeer{
		Peer: peer,
	}

	if bscExt != nil {
		ep.bscExt = &bscPeer{Peer: bscExt}
	}
	ps.peers[id] = ep

	return nil
}

// registerBscExtension unblocks an already connected `eth` peer waiting for its
// `bsc` extension, or if no such peer exists, tracks the extension for the time
// being until the `eth` main protocol starts looking for it.
func (ps *peerSet) registerBscExtension(peer *bsc.Peer) error {
	// reject the peer if it advertises `bsc` without `eth` as `bsc` is only a
	// satellite protocol meaningful with the chain selection of `eth`
	if !peer.RunningCap(eth.ProtocolName, eth.ProtocolVersions) {
		return errBscWithoutEth
	}

	// ensure nobody can double-connect
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.bscPend[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}

	// inject the peer into an `eth` counterpart is available, otherwise save for later
	if wait, ok := ps.bscWait[id]; ok {
		delete(ps.bscWait, id)
		wait <- *peer
		return nil
	}

	ps.bscPend[id] = *peer

	return nil
}

// waitBscExtension blocks until all satellite protocols are connected and tracked
// by the peerset.
func (ps *peerSet) waitBscExtension(peer *eth2.Peer) (*bsc.Peer, error) {
	// if the peer does not support a compatible `bsc`, don't wait
	if !peer.RunningCap(bsc.ProtocolName, bsc.ProtocolVersions) {
		return nil, nil
	}
	// Ensure nobody can double-connect
	ps.lock.Lock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.bscWait[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// If `bsc` already connected, retrieve the peer from the pending set
	if bsc, ok := ps.bscPend[id]; ok {
		delete(ps.bscPend, id)

		ps.lock.Unlock()
		return &bsc, nil
	}
	// Otherwise wait for `bsc` to connect concurrently
	wait := make(chan bsc.Peer)
	ps.bscWait[id] = wait
	ps.lock.Unlock()

	select {
	case peer := <-wait:
		return &peer, nil
	case <-time.After(extensionWaitTimeout):
		// could be deadlock, so we use TryLock to avoid it.
		if ps.lock.TryLock() {
			delete(ps.bscWait, id)
			ps.lock.Unlock()
			return nil, errPeerWaitTimeout
		}
		// if TryLock failed, we wait for a while and try again.
		for {
			select {
			case <-wait:
				// discard the peer, even though the peer arrived.
				return nil, errPeerWaitTimeout
			case <-time.After(tryWaitTimeout):
				if ps.lock.TryLock() {
					delete(ps.bscWait, id)
					ps.lock.Unlock()
					return nil, errPeerWaitTimeout
				}
			}
		}
	}
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

func (ps *peerSet) get(id string) (*ethPeer, bool) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	p, ok := ps.peers[id]

	return &p, ok
}

func (ps *peerSet) getAll() []ethPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	peers := make([]ethPeer, 0, len(ps.peers))
	for _, peer := range ps.peers {
		peers = append(peers, peer)
	}
	return peers
}
