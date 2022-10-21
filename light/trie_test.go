// Copyright 2017 The go-sdcereum Authors
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

package light

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/sdcereum/go-sdcereum/consensus/sdcash"
	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/core/rawdb"
	"github.com/sdcereum/go-sdcereum/core/state"
	"github.com/sdcereum/go-sdcereum/core/vm"
	"github.com/sdcereum/go-sdcereum/params"
	"github.com/sdcereum/go-sdcereum/trie"
)

func TestNodeIterator(t *testing.T) {
	var (
		fulldb  = rawdb.NewMemoryDatabase()
		lightdb = rawdb.NewMemoryDatabase()
		gspec   = &core.Genesis{
			Config:  params.TestChainConfig,
			Alloc:   core.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
			BaseFee: big.NewInt(params.InitialBaseFee),
		}
	)
	blockchain, _ := core.NewBlockChain(fulldb, nil, gspec, nil, sdcash.NewFullFaker(), vm.Config{}, nil, nil)
	_, gchain, _ := core.GenerateChainWithGenesis(gspec, sdcash.NewFaker(), 4, testChainGen)
	if _, err := blockchain.InsertChain(gchain); err != nil {
		panic(err)
	}

	gspec.MustCommit(lightdb)
	ctx := context.Background()
	odr := &testOdr{sdb: fulldb, ldb: lightdb, serverState: blockchain.StateCache(), indexerConfig: TestClientIndexerConfig}
	head := blockchain.CurrentHeader()
	lightTrie, _ := NewStateDatabase(ctx, head, odr).OpenTrie(head.Root)
	fullTrie, _ := blockchain.StateCache().OpenTrie(head.Root)
	if err := diffTries(fullTrie, lightTrie); err != nil {
		t.Fatal(err)
	}
}

func diffTries(t1, t2 state.Trie) error {
	i1 := trie.NewIterator(t1.NodeIterator(nil))
	i2 := trie.NewIterator(t2.NodeIterator(nil))
	for i1.Next() && i2.Next() {
		if !bytes.Equal(i1.Key, i2.Key) {
			spew.Dump(i2)
			return fmt.Errorf("tries have different keys %x, %x", i1.Key, i2.Key)
		}
		if !bytes.Equal(i1.Value, i2.Value) {
			return fmt.Errorf("tries differ at key %x", i1.Key)
		}
	}
	switch {
	case i1.Err != nil:
		return fmt.Errorf("full trie iterator error: %v", i1.Err)
	case i2.Err != nil:
		return fmt.Errorf("light trie iterator error: %v", i2.Err)
	case i1.Next():
		return fmt.Errorf("full trie iterator has more k/v pairs")
	case i2.Next():
		return fmt.Errorf("light trie iterator has more k/v pairs")
	}
	return nil
}
