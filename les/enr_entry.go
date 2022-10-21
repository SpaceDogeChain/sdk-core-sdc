// Copyright 2019 The go-sdcereum Authors
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

package les

import (
	"github.com/sdcereum/go-sdcereum/core/forkid"
	"github.com/sdcereum/go-sdcereum/p2p/dnsdisc"
	"github.com/sdcereum/go-sdcereum/p2p/enode"
	"github.com/sdcereum/go-sdcereum/rlp"
)

// lesEntry is the "les" ENR entry. This is set for LES servers only.
type lesEntry struct {
	// Ignore additional fields (for forward compatibility).
	VfxVersion uint
	Rest       []rlp.RawValue `rlp:"tail"`
}

func (lesEntry) ENRKey() string { return "les" }

// sdcEntry is the "sdc" ENR entry. This is redeclared here to avoid depending on package sdc.
type sdcEntry struct {
	ForkID forkid.ID
	Tail   []rlp.RawValue `rlp:"tail"`
}

func (sdcEntry) ENRKey() string { return "sdc" }

// setupDiscovery creates the node discovery source for the sdc protocol.
func (sdc *Lightsdcereum) setupDiscovery() (enode.Iterator, error) {
	it := enode.NewFairMix(0)

	// Enable DNS discovery.
	if len(sdc.config.sdcDiscoveryURLs) != 0 {
		client := dnsdisc.NewClient(dnsdisc.Config{})
		dns, err := client.NewIterator(sdc.config.sdcDiscoveryURLs...)
		if err != nil {
			return nil, err
		}
		it.AddSource(dns)
	}

	// Enable DHT.
	if sdc.udpEnabled {
		it.AddSource(sdc.p2pServer.DiscV5.RandomNodes())
	}

	forkFilter := forkid.NewFilter(sdc.blockchain)
	iterator := enode.Filter(it, func(n *enode.Node) bool { return nodeIsServer(forkFilter, n) })
	return iterator, nil
}

// nodeIsServer checks whsdcer n is an LES server node.
func nodeIsServer(forkFilter forkid.Filter, n *enode.Node) bool {
	var les lesEntry
	var sdc sdcEntry
	return n.Load(&les) == nil && n.Load(&sdc) == nil && forkFilter(sdc.ForkID) == nil
}
