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

package les

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/light"
)

func TestLightPruner(t *testing.T) {
	var (
		waitIndexers = func(cIndexer, bIndexer, btIndexer *core.ChainIndexer) {
			for {
				cs, _, _ := cIndexer.Sections()
				bts, _, _ := btIndexer.Sections()
				if cs >= 3 && bts >= 3 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		config    = light.TestClientIndexerConfig
		netconfig = testnetConfig{
			blocks:   int(3*config.ChtSize + config.ChtConfirms),
			protocol: 3,
			indexFn:  waitIndexers,
			connect:  true,
		}
	)
	server, client, tearDown := newClientServerEnv(t, netconfig)
	defer tearDown()

	// checkDB iterates the chain with given prefix, resolves the block number
	// with given callback and ensures this entry should exist or not.
	checkDB := func(from, to uint64, prefix []byte, resolve func(key, value []byte) *uint64, exist bool) bool {
		it := client.db.NewIterator(prefix, nil)
		defer it.Release()

		var next = from
		for it.Next() {
			number := resolve(it.Key(), it.Value())
			if number == nil || *number < from {
				continue
			} else if *number > to {
				return true
			}
			if exist {
				if *number != next {
					return false
				}
				next++
			} else {
				return false
			}
		}
		return true
	}
	// checkPruned checks and ensures the stale chain data has been pruned.
	checkPruned := func(from, to uint64) {
		// Iterate canonical hash
		if !checkDB(from, to, []byte("h"), func(key, value []byte) *uint64 {
			if len(key) == 1+8+1 && bytes.Equal(key[9:10], []byte("n")) {
				n := binary.BigEndian.Uint64(key[1:9])
				return &n
			}
			return nil
		}, false) {
			t.Fatalf("canonical hash mappings are not properly pruned")
		}
		// Iterate header
		if !checkDB(from, to, []byte("h"), func(key, value []byte) *uint64 {
			if len(key) == 1+8+32 {
				n := binary.BigEndian.Uint64(key[1:9])
				return &n
			}
			return nil
		}, false) {
			t.Fatalf("headers are not properly pruned")
		}
		// Iterate body
		if !checkDB(from, to, []byte("b"), func(key, value []byte) *uint64 {
			if len(key) == 1+8+32 {
				n := binary.BigEndian.Uint64(key[1:9])
				return &n
			}
			return nil
		}, false) {
			t.Fatalf("block bodies are not properly pruned")
		}
		// Iterate receipts
		if !checkDB(from, to, []byte("r"), func(key, value []byte) *uint64 {
			if len(key) == 1+8+32 {
				n := binary.BigEndian.Uint64(key[1:9])
				return &n
			}
			return nil
		}, false) {
			t.Fatalf("receipts are not properly pruned")
		}
		// Iterate td
		if !checkDB(from, to, []byte("h"), func(key, value []byte) *uint64 {
			if len(key) == 1+8+32+1 && bytes.Equal(key[41:42], []byte("t")) {
				n := binary.BigEndian.Uint64(key[1:9])
				return &n
			}
			return nil
		}, false) {
			t.Fatalf("tds are not properly pruned")
		}
	}
	// Start light pruner.
	time.Sleep(1500 * time.Millisecond) // Ensure light client has finished the syncing and indexing
	newPruner(client.db, client.chtIndexer, client.bloomTrieIndexer)

	time.Sleep(1500 * time.Millisecond) // Ensure pruner have enough time to prune data.
	checkPruned(1, config.ChtSize-1)

	// Ensure all APIs still work after pruning.
	var cases = []struct {
		from, to   uint64
		msdcodName string
		msdcod     func(uint64) bool
	}{
		{
			1, 10, "GsdceaderByNumber",
			func(n uint64) bool {
				_, err := light.GsdceaderByNumber(context.Background(), client.handler.backend.odr, n)
				return err == nil
			},
		},
		{
			11, 20, "GetCanonicalHash",
			func(n uint64) bool {
				_, err := light.GetCanonicalHash(context.Background(), client.handler.backend.odr, n)
				return err == nil
			},
		},
		{
			21, 30, "GetTd",
			func(n uint64) bool {
				_, err := light.GetTd(context.Background(), client.handler.backend.odr, server.handler.blockchain.GsdceaderByNumber(n).Hash(), n)
				return err == nil
			},
		},
		{
			31, 40, "GetBodyRLP",
			func(n uint64) bool {
				_, err := light.GetBodyRLP(context.Background(), client.handler.backend.odr, server.handler.blockchain.GsdceaderByNumber(n).Hash(), n)
				return err == nil
			},
		},
		{
			41, 50, "GetBlock",
			func(n uint64) bool {
				_, err := light.GetBlock(context.Background(), client.handler.backend.odr, server.handler.blockchain.GsdceaderByNumber(n).Hash(), n)
				return err == nil
			},
		},
		{
			51, 60, "GetBlockReceipts",
			func(n uint64) bool {
				_, err := light.GetBlockReceipts(context.Background(), client.handler.backend.odr, server.handler.blockchain.GsdceaderByNumber(n).Hash(), n)
				return err == nil
			},
		},
	}
	for _, c := range cases {
		for i := c.from; i <= c.to; i++ {
			if !c.msdcod(i) {
				t.Fatalf("rpc msdcod %s failed, number %d", c.msdcodName, i)
			}
		}
	}
	// Check GetBloombits
	_, err := light.GetBloomBits(context.Background(), client.handler.backend.odr, 0, []uint64{0})
	if err != nil {
		t.Fatalf("Failed to retrieve bloombits of pruned section: %v", err)
	}

	// Ensure the ODR cached data can be cleaned by pruner.
	newPruner(client.db, client.chtIndexer, client.bloomTrieIndexer)
	time.Sleep(50 * time.Millisecond) // Ensure pruner have enough time to prune data.
	checkPruned(1, config.ChtSize-1)  // Ensure all cached data(by odr) is cleaned.
}
