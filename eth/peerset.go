// Copyright 2020 The go-sdcereum Authors
// This file is part of the go-sdcereum library.
//
// The go-sdcereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-sdcereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-sdcereum library. If not, see <http://www.gnu.org/licenses/>.

package sdc

import (
	"errors"
	"math/big"
	"sync"

	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/sdc/protocols/sdc"
	"github.com/sdcereum/go-sdcereum/sdc/protocols/snap"
	"github.com/sdcereum/go-sdcereum/p2p"
)

var (
	// errPeerSetClosed is returned if a peer is attempted to be added or removed
	// from the peer set after it has been terminated.
	errPeerSetClosed = errors.New("peerset closed")

	// errPeerAlreadyRegistered is returned if a peer is attempted to be added
	// to the peer set, but one with the same id already exists.
	errPeerAlreadyRegistered = errors.New("peer already registered")

	// errPeerNotRegistered is returned if a peer is attempted to be removed from
	// a peer set, but no peer with the given id exists.
	errPeerNotRegistered = errors.New("peer not registered")

	// errSnapWithoutsdc is returned if a peer attempts to connect only on the
	// snap protocol without advertising the sdc main protocol.
	errSnapWithoutsdc = errors.New("peer connected on snap without compatible sdc support")
)

// peerSet represents the collection of active peers currently participating in
// the `sdc` protocol, with or without the `snap` extension.
type peerSet struct {
	peers     map[string]*sdcPeer // Peers connected on the `sdc` protocol
	snapPeers int                 // Number of `snap` compatible peers for connection prioritization

	snapWait map[string]chan *snap.Peer // Peers connected on `sdc` waiting for their snap extension
	snapPend map[string]*snap.Peer      // Peers connected on the `snap` protocol, but not yet on `sdc`

	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers:    make(map[string]*sdcPeer),
		snapWait: make(map[string]chan *snap.Peer),
		snapPend: make(map[string]*snap.Peer),
	}
}

// registerSnapExtension unblocks an already connected `sdc` peer waiting for its
// `snap` extension, or if no such peer exists, tracks the extension for the time
// being until the `sdc` main protocol starts looking for it.
func (ps *peerSet) registerSnapExtension(peer *snap.Peer) error {
	// Reject the peer if it advertises `snap` without `sdc` as `snap` is only a
	// satellite protocol meaningful with the chain selection of `sdc`
	if !peer.RunningCap(sdc.ProtocolName, sdc.ProtocolVersions) {
		return errSnapWithoutsdc
	}
	// Ensure nobody can double connect
	ps.lock.Lock()
	defer ps.lock.Unlock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.snapPend[id]; ok {
		return errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// Inject the peer into an `sdc` counterpart is available, otherwise save for later
	if wait, ok := ps.snapWait[id]; ok {
		delete(ps.snapWait, id)
		wait <- peer
		return nil
	}
	ps.snapPend[id] = peer
	return nil
}

// waitExtensions blocks until all satellite protocols are connected and tracked
// by the peerset.
func (ps *peerSet) waitSnapExtension(peer *sdc.Peer) (*snap.Peer, error) {
	// If the peer does not support a compatible `snap`, don't wait
	if !peer.RunningCap(snap.ProtocolName, snap.ProtocolVersions) {
		return nil, nil
	}
	// Ensure nobody can double connect
	ps.lock.Lock()

	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as existing ones
	}
	if _, ok := ps.snapWait[id]; ok {
		ps.lock.Unlock()
		return nil, errPeerAlreadyRegistered // avoid connections with the same id as pending ones
	}
	// If `snap` already connected, retrieve the peer from the pending set
	if snap, ok := ps.snapPend[id]; ok {
		delete(ps.snapPend, id)

		ps.lock.Unlock()
		return snap, nil
	}
	// Otherwise wait for `snap` to connect concurrently
	wait := make(chan *snap.Peer)
	ps.snapWait[id] = wait
	ps.lock.Unlock()

	return <-wait, nil
}

// registerPeer injects a new `sdc` peer into the working set, or returns an error
// if the peer is already known.
func (ps *peerSet) registerPeer(peer *sdc.Peer, ext *snap.Peer) error {
	// Start tracking the new peer
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errPeerSetClosed
	}
	id := peer.ID()
	if _, ok := ps.peers[id]; ok {
		return errPeerAlreadyRegistered
	}
	sdc := &sdcPeer{
		Peer: peer,
	}
	if ext != nil {
		sdc.snapExt = &snapPeer{ext}
		ps.snapPeers++
	}
	ps.peers[id] = sdc
	return nil
}

// unregisterPeer removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) unregisterPeer(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	peer, ok := ps.peers[id]
	if !ok {
		return errPeerNotRegistered
	}
	delete(ps.peers, id)
	if peer.snapExt != nil {
		ps.snapPeers--
	}
	return nil
}

// peer retrieves the registered peer with the given id.
func (ps *peerSet) peer(id string) *sdcPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// peersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes so it might be propagated to them.
func (ps *peerSet) peersWithoutBlock(hash common.Hash) []*sdcPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*sdcPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.KnownBlock(hash) {
			list = append(list, p)
		}
	}
	return list
}

// peersWithoutTransaction retrieves a list of peers that do not have a given
// transaction in their set of known hashes.
func (ps *peerSet) peersWithoutTransaction(hash common.Hash) []*sdcPeer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*sdcPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.KnownTransaction(hash) {
			list = append(list, p)
		}
	}
	return list
}

// len returns if the current number of `sdc` peers in the set. Since the `snap`
// peers are tied to the existence of an `sdc` connection, that will always be a
// subset of `sdc`.
func (ps *peerSet) len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// snapLen returns if the current number of `snap` peers in the set.
func (ps *peerSet) snapLen() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.snapPeers
}

// peerWithHighestTD retrieves the known peer with the currently highest total
// difficulty, but below the given PoS switchover threshold.
func (ps *peerSet) peerWithHighestTD() *sdc.Peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *sdc.Peer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p.Peer, td
		}
	}
	return bestPeer
}

// close disconnects all peers.
func (ps *peerSet) close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}
