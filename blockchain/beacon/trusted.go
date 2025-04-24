package beacon

import (
	"bufio"
	"os"
	"sync"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/libp2p/go-libp2p/core/network"
	libp2pPeer "github.com/libp2p/go-libp2p/core/peer"
)

type trustedPeers struct {
	peers map[libp2pPeer.ID]struct{}
	mu    *sync.RWMutex
}

func newTrustedPeers() trustedPeers {
	return trustedPeers{
		peers: make(map[libp2pPeer.ID]struct{}),
		mu:    &sync.RWMutex{},
	}
}

func (t *trustedPeers) list() []libp2pPeer.ID {
	t.mu.RLock()
	defer t.mu.RUnlock()

	peers := make([]libp2pPeer.ID, 0, len(t.peers))
	for id := range t.peers {
		peers = append(peers, id)
	}

	return peers
}

func (t *trustedPeers) add(id libp2pPeer.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.peers[id] = struct{}{}
}

func (t *trustedPeers) remove(id libp2pPeer.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.peers, id)
}

func (t *trustedPeers) contains(id libp2pPeer.ID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	_, ok := t.peers[id]
	return ok
}

func (n *Node) reloadTrustedPeers(filename string) {
	utils.TriggerOnFileChanged(n.ctx, filename, func() {
		peerIDs, err := readPeerIDsFromFile(n.log, filename)
		if err != nil {
			log.Errorf("Failed to read peer IDs from file: %s", err)
			return
		}

		n.log.Infof("beacon trusted peers loaded: %v", peerIDs)

		add, remove := utils.CompareLists(peerIDs, n.trustedPeers.list())
		for _, id := range remove {
			n.log.Tracef("removing trusted peer: %s", id)

			// Remove only inbound connections in case peer is also added as static
			for _, conn := range n.host.Network().ConnsToPeer(id) {
				if conn.Stat().Direction == network.DirInbound {
					go func() {
						if err := conn.Close(); err != nil {
							n.log.Errorf("failed to close connection to peer %s: %s", id, err)
						}
					}()
				}
			}

			n.trustedPeers.remove(id)
		}

		for _, id := range add {
			n.log.Tracef("adding trusted peer: %s", id)

			n.trustedPeers.add(id)
		}
	})
}

func readPeerIDsFromFile(log *log.Entry, filename string) ([]libp2pPeer.ID, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var peerIDs []libp2pPeer.ID
	for scanner.Scan() {
		peerID, err := libp2pPeer.Decode(scanner.Text())
		if err != nil {
			log.Errorf("Failed to decode peer ID: %s", err)
			continue
		}
		peerIDs = append(peerIDs, peerID)

	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return peerIDs, nil
}
