// Copyright 2015 The go-sdcereum Authors
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
	"math/big"

	"github.com/sdcereum/go-sdcereum/sdc/protocols/sdc"
	"github.com/sdcereum/go-sdcereum/sdc/protocols/snap"
)

// sdcPeerInfo represents a short summary of the `sdc` sub-protocol metadata known
// about a connected peer.
type sdcPeerInfo struct {
	Version    uint     `json:"version"`    // sdcereum protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // Hex hash of the peer's best owned block
}

// sdcPeer is a wrapper around sdc.Peer to maintain a few extra metadata.
type sdcPeer struct {
	*sdc.Peer
	snapExt *snapPeer // Satellite `snap` connection
}

// info gathers and returns some `sdc` protocol metadata known about a peer.
func (p *sdcPeer) info() *sdcPeerInfo {
	hash, td := p.Head()

	return &sdcPeerInfo{
		Version:    p.Version(),
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// snapPeerInfo represents a short summary of the `snap` sub-protocol metadata known
// about a connected peer.
type snapPeerInfo struct {
	Version uint `json:"version"` // Snapshot protocol version negotiated
}

// snapPeer is a wrapper around snap.Peer to maintain a few extra metadata.
type snapPeer struct {
	*snap.Peer
}

// info gathers and returns some `snap` protocol metadata known about a peer.
func (p *snapPeer) info() *snapPeerInfo {
	return &snapPeerInfo{
		Version: p.Version(),
	}
}
