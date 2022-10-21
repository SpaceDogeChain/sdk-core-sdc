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

// Tests that setting the chain head backwards doesn't leave the database in some
// strange state with gaps in the chain, nor with block data dangling in the future.

package core

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/consensus/sdcash"
	"github.com/sdcereum/go-sdcereum/core/rawdb"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/core/vm"
	"github.com/sdcereum/go-sdcereum/params"
)

// rewindTest is a test case for chain rollback upon user request.
type rewindTest struct {
	canonicalBlocks int     // Number of blocks to generate for the canonical chain (heavier)
	sidechainBlocks int     // Number of blocks to generate for the side chain (lighter)
	freezsdcreshold uint64  // Block number until which to move things into the freezer
	commitBlock     uint64  // Block number for which to commit the state to disk
	pivotBlock      *uint64 // Pivot block number in case of fast sync

	ssdceadBlock       uint64 // Block number to set head back to
	expCanonicalBlocks int    // Number of canonical blocks expected to remain in the database (excl. genesis)
	expSidechainBlocks int    // Number of sidechain blocks expected to remain in the database (excl. genesis)
	expFrozen          int    // Number of canonical blocks expected to be in the freezer (incl. genesis)
	expHeadHeader      uint64 // Block number of the expected head header
	expHeadFastBlock   uint64 // Block number of the expected head fast sync block
	expHeadBlock       uint64 // Block number of the expected head full block
}

//nolint:unused
func (tt *rewindTest) dump(crash bool) string {
	buffer := new(strings.Builder)

	fmt.Fprint(buffer, "Chain:\n  G")
	for i := 0; i < tt.canonicalBlocks; i++ {
		fmt.Fprintf(buffer, "->C%d", i+1)
	}
	fmt.Fprint(buffer, " (HEAD)\n")
	if tt.sidechainBlocks > 0 {
		fmt.Fprintf(buffer, "  └")
		for i := 0; i < tt.sidechainBlocks; i++ {
			fmt.Fprintf(buffer, "->S%d", i+1)
		}
		fmt.Fprintf(buffer, "\n")
	}
	fmt.Fprintf(buffer, "\n")

	if tt.canonicalBlocks > int(tt.freezsdcreshold) {
		fmt.Fprint(buffer, "Frozen:\n  G")
		for i := 0; i < tt.canonicalBlocks-int(tt.freezsdcreshold); i++ {
			fmt.Fprintf(buffer, "->C%d", i+1)
		}
		fmt.Fprintf(buffer, "\n\n")
	} else {
		fmt.Fprintf(buffer, "Frozen: none\n")
	}
	fmt.Fprintf(buffer, "Commit: G")
	if tt.commitBlock > 0 {
		fmt.Fprintf(buffer, ", C%d", tt.commitBlock)
	}
	fmt.Fprint(buffer, "\n")

	if tt.pivotBlock == nil {
		fmt.Fprintf(buffer, "Pivot : none\n")
	} else {
		fmt.Fprintf(buffer, "Pivot : C%d\n", *tt.pivotBlock)
	}
	if crash {
		fmt.Fprintf(buffer, "\nCRASH\n\n")
	} else {
		fmt.Fprintf(buffer, "\nSsdcead(%d)\n\n", tt.ssdceadBlock)
	}
	fmt.Fprintf(buffer, "------------------------------\n\n")

	if tt.expFrozen > 0 {
		fmt.Fprint(buffer, "Expected in freezer:\n  G")
		for i := 0; i < tt.expFrozen-1; i++ {
			fmt.Fprintf(buffer, "->C%d", i+1)
		}
		fmt.Fprintf(buffer, "\n\n")
	}
	if tt.expFrozen > 0 {
		if tt.expFrozen >= tt.expCanonicalBlocks {
			fmt.Fprintf(buffer, "Expected in leveldb: none\n")
		} else {
			fmt.Fprintf(buffer, "Expected in leveldb:\n  C%d)", tt.expFrozen-1)
			for i := tt.expFrozen - 1; i < tt.expCanonicalBlocks; i++ {
				fmt.Fprintf(buffer, "->C%d", i+1)
			}
			fmt.Fprint(buffer, "\n")
			if tt.expSidechainBlocks > tt.expFrozen {
				fmt.Fprintf(buffer, "  └")
				for i := tt.expFrozen - 1; i < tt.expSidechainBlocks; i++ {
					fmt.Fprintf(buffer, "->S%d", i+1)
				}
				fmt.Fprintf(buffer, "\n")
			}
		}
	} else {
		fmt.Fprint(buffer, "Expected in leveldb:\n  G")
		for i := tt.expFrozen; i < tt.expCanonicalBlocks; i++ {
			fmt.Fprintf(buffer, "->C%d", i+1)
		}
		fmt.Fprint(buffer, "\n")
		if tt.expSidechainBlocks > tt.expFrozen {
			fmt.Fprintf(buffer, "  └")
			for i := tt.expFrozen; i < tt.expSidechainBlocks; i++ {
				fmt.Fprintf(buffer, "->S%d", i+1)
			}
			fmt.Fprintf(buffer, "\n")
		}
	}
	fmt.Fprintf(buffer, "\n")
	fmt.Fprintf(buffer, "Expected head header    : C%d\n", tt.expHeadHeader)
	fmt.Fprintf(buffer, "Expected head fast block: C%d\n", tt.expHeadFastBlock)
	if tt.expHeadBlock == 0 {
		fmt.Fprintf(buffer, "Expected head block     : G\n")
	} else {
		fmt.Fprintf(buffer, "Expected head block     : C%d\n", tt.expHeadBlock)
	}
	return buffer.String()
}

// Tests a ssdcead for a short canonical chain where a recent block was already
// committed to disk and then the ssdcead called. In this case we expect the full
// chain to be rolled back to the committed block. Everything above the ssdcead
// point should be deleted. In between the committed block and the requested head
// the data can remain as "fast sync" data to avoid redownloading it.
func TestShortSsdcead(t *testing.T)              { testShortSsdcead(t, false) }
func TestShortSsdceadWithSnapshots(t *testing.T) { testShortSsdcead(t, true) }

func testShortSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 0,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain where the fast sync pivot point was
// already committed, after which ssdcead was called. In this case we expect the
// chain to behave like in full sync mode, rolling back to the committed block
// Everything above the ssdcead point should be deleted. In between the committed
// block and the requested head the data can remain as "fast sync" data to avoid
// redownloading it.
func TestShortSnapSyncedSsdcead(t *testing.T)              { testShortSnapSyncedSsdcead(t, false) }
func TestShortSnapSyncedSsdceadWithSnapshots(t *testing.T) { testShortSnapSyncedSsdcead(t, true) }

func testShortSnapSyncedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 0,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain where the fast sync pivot point was
// not yet committed, but ssdcead was called. In this case we expect the chain to
// detect that it was fast syncing and delete everything from the new head, since
// we can just pick up fast syncing from there. The head full block should be set
// to the genesis.
func TestShortSnapSyncingSsdcead(t *testing.T)              { testShortSnapSyncingSsdcead(t, false) }
func TestShortSnapSyncingSsdceadWithSnapshots(t *testing.T) { testShortSnapSyncingSsdcead(t, true) }

func testShortSnapSyncingSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//
	// Frozen: none
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 0,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a shorter side chain, where a
// recent block was already committed to disk and then ssdcead was called. In this
// test scenario the side chain is below the committed block. In this case we expect
// the canonical full chain to be rolled back to the committed block. Everything
// above the ssdcead point should be deleted. In between the committed block and
// the requested head the data can remain as "fast sync" data to avoid redownloading
// it. The side chain should be left alone as it was shorter.
func TestShortOldForkedSsdcead(t *testing.T)              { testShortOldForkedSsdcead(t, false) }
func TestShortOldForkedSsdceadWithSnapshots(t *testing.T) { testShortOldForkedSsdcead(t, true) }

func testShortOldForkedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 3,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a shorter side chain, where
// the fast sync pivot point was already committed to disk and then ssdcead was
// called. In this test scenario the side chain is below the committed block. In
// this case we expect the canonical full chain to be rolled back to the committed
// block. Everything above the ssdcead point should be deleted. In between the
// committed block and the requested head the data can remain as "fast sync" data
// to avoid redownloading it. The side chain should be left alone as it was shorter.
func TestShortOldForkedSnapSyncedSsdcead(t *testing.T) {
	testShortOldForkedSnapSyncedSsdcead(t, false)
}
func TestShortOldForkedSnapSyncedSsdceadWithSnapshots(t *testing.T) {
	testShortOldForkedSnapSyncedSsdcead(t, true)
}

func testShortOldForkedSnapSyncedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 3,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a shorter side chain, where
// the fast sync pivot point was not yet committed, but ssdcead was called. In this
// test scenario the side chain is below the committed block. In this case we expect
// the chain to detect that it was fast syncing and delete everything from the new
// head, since we can just pick up fast syncing from there. The head full block
// should be set to the genesis.
func TestShortOldForkedSnapSyncingSsdcead(t *testing.T) {
	testShortOldForkedSnapSyncingSsdcead(t, false)
}
func TestShortOldForkedSnapSyncingSsdceadWithSnapshots(t *testing.T) {
	testShortOldForkedSnapSyncingSsdcead(t, true)
}

func testShortOldForkedSnapSyncingSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen: none
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 3,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a shorter side chain, where a
// recent block was already committed to disk and then ssdcead was called. In this
// test scenario the side chain reaches above the committed block. In this case we
// expect the canonical full chain to be rolled back to the committed block. All
// data above the ssdcead point should be deleted. In between the committed block
// and the requested head the data can remain as "fast sync" data to avoid having
// to redownload it. The side chain should be truncated to the head set.
//
// The side chain could be left to be if the fork point was before the new head
// we are deleting to, but it would be exceedingly hard to detect that case and
// properly handle it, so we'll trade extra work in exchange for simpler code.
func TestShortNewlyForkedSsdcead(t *testing.T)              { testShortNewlyForkedSsdcead(t, false) }
func TestShortNewlyForkedSsdceadWithSnapshots(t *testing.T) { testShortNewlyForkedSsdcead(t, true) }

func testShortNewlyForkedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3->S4->S5->S6->S7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    10,
		sidechainBlocks:    8,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 7,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a shorter side chain, where
// the fast sync pivot point was already committed to disk and then ssdcead was
// called. In this case we expect the canonical full chain to be rolled back to
// between the committed block and the requested head the data can remain as
// "fast sync" data to avoid having to redownload it. The side chain should be
// truncated to the head set.
//
// The side chain could be left to be if the fork point was before the new head
// we are deleting to, but it would be exceedingly hard to detect that case and
// properly handle it, so we'll trade extra work in exchange for simpler code.
func TestShortNewlyForkedSnapSyncedSsdcead(t *testing.T) {
	testShortNewlyForkedSnapSyncedSsdcead(t, false)
}
func TestShortNewlyForkedSnapSyncedSsdceadWithSnapshots(t *testing.T) {
	testShortNewlyForkedSnapSyncedSsdcead(t, true)
}

func testShortNewlyForkedSnapSyncedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3->S4->S5->S6->S7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    10,
		sidechainBlocks:    8,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 7,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a shorter side chain, where
// the fast sync pivot point was not yet committed, but ssdcead was called. In
// this test scenario the side chain reaches above the committed block. In this
// case we expect the chain to detect that it was fast syncing and delete
// everything from the new head, since we can just pick up fast syncing from
// there.
//
// The side chain could be left to be if the fork point was before the new head
// we are deleting to, but it would be exceedingly hard to detect that case and
// properly handle it, so we'll trade extra work in exchange for simpler code.
func TestShortNewlyForkedSnapSyncingSsdcead(t *testing.T) {
	testShortNewlyForkedSnapSyncingSsdcead(t, false)
}
func TestShortNewlyForkedSnapSyncingSsdceadWithSnapshots(t *testing.T) {
	testShortNewlyForkedSnapSyncingSsdcead(t, true)
}

func testShortNewlyForkedSnapSyncingSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8
	//
	// Frozen: none
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3->S4->S5->S6->S7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    10,
		sidechainBlocks:    8,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 7,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a longer side chain, where a
// recent block was already committed to disk and then ssdcead was called. In this
// case we expect the canonical full chain to be rolled back to the committed block.
// All data above the ssdcead point should be deleted. In between the committed
// block and the requested head the data can remain as "fast sync" data to avoid
// having to redownload it. The side chain should be truncated to the head set.
//
// The side chain could be left to be if the fork point was before the new head
// we are deleting to, but it would be exceedingly hard to detect that case and
// properly handle it, so we'll trade extra work in exchange for simpler code.
func TestShortReorgedSsdcead(t *testing.T)              { testShortReorgedSsdcead(t, false) }
func TestShortReorgedSsdceadWithSnapshots(t *testing.T) { testShortReorgedSsdcead(t, true) }

func testShortReorgedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3->S4->S5->S6->S7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    10,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 7,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a longer side chain, where
// the fast sync pivot point was already committed to disk and then ssdcead was
// called. In this case we expect the canonical full chain to be rolled back to
// the committed block. All data above the ssdcead point should be deleted. In
// between the committed block and the requested head the data can remain as
// "fast sync" data to avoid having to redownload it. The side chain should be
// truncated to the head set.
//
// The side chain could be left to be if the fork point was before the new head
// we are deleting to, but it would be exceedingly hard to detect that case and
// properly handle it, so we'll trade extra work in exchange for simpler code.
func TestShortReorgedSnapSyncedSsdcead(t *testing.T) {
	testShortReorgedSnapSyncedSsdcead(t, false)
}
func TestShortReorgedSnapSyncedSsdceadWithSnapshots(t *testing.T) {
	testShortReorgedSnapSyncedSsdcead(t, true)
}

func testShortReorgedSnapSyncedSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10
	//
	// Frozen: none
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3->S4->S5->S6->S7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    10,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 7,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a short canonical chain and a longer side chain, where
// the fast sync pivot point was not yet committed, but ssdcead was called. In
// this case we expect the chain to detect that it was fast syncing and delete
// everything from the new head, since we can just pick up fast syncing from
// there.
//
// The side chain could be left to be if the fork point was before the new head
// we are deleting to, but it would be exceedingly hard to detect that case and
// properly handle it, so we'll trade extra work in exchange for simpler code.
func TestShortReorgedSnapSyncingSsdcead(t *testing.T) {
	testShortReorgedSnapSyncingSsdcead(t, false)
}
func TestShortReorgedSnapSyncingSsdceadWithSnapshots(t *testing.T) {
	testShortReorgedSnapSyncingSsdcead(t, true)
}

func testShortReorgedSnapSyncingSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10
	//
	// Frozen: none
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(7)
	//
	// ------------------------------
	//
	// Expected in leveldb:
	//   G->C1->C2->C3->C4->C5->C6->C7
	//   └->S1->S2->S3->S4->S5->S6->S7
	//
	// Expected head header    : C7
	// Expected head fast block: C7
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    8,
		sidechainBlocks:    10,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       7,
		expCanonicalBlocks: 7,
		expSidechainBlocks: 7,
		expFrozen:          0,
		expHeadHeader:      7,
		expHeadFastBlock:   7,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks where a recent
// block - newer than the ancient limit - was already committed to disk and then
// ssdcead was called. In this case we expect the full chain to be rolled back
// to the committed block. Everything above the ssdcead point should be deleted.
// In between the committed block and the requested head the data can remain as
// "fast sync" data to avoid redownloading it.
func TestLongShallowSsdcead(t *testing.T)              { testLongShallowSsdcead(t, false) }
func TestLongShallowSsdceadWithSnapshots(t *testing.T) { testLongShallowSsdcead(t, true) }

func testLongShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks where a recent
// block - older than the ancient limit - was already committed to disk and then
// ssdcead was called. In this case we expect the full chain to be rolled back
// to the committed block. Since the ancient limit was underflown, everything
// needs to be deleted onwards to avoid creating a gap.
func TestLongDeepSsdcead(t *testing.T)              { testLongDeepSsdcead(t, false) }
func TestLongDeepSsdceadWithSnapshots(t *testing.T) { testLongDeepSsdcead(t, true) }

func testLongDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks where the fast
// sync pivot point - newer than the ancient limit - was already committed, after
// which ssdcead was called. In this case we expect the full chain to be rolled
// back to the committed block. Everything above the ssdcead point should be
// deleted. In between the committed block and the requested head the data can
// remain as "fast sync" data to avoid redownloading it.
func TestLongSnapSyncedShallowSsdcead(t *testing.T) {
	testLongSnapSyncedShallowSsdcead(t, false)
}
func TestLongSnapSyncedShallowSsdceadWithSnapshots(t *testing.T) {
	testLongSnapSyncedShallowSsdcead(t, true)
}

func testLongSnapSyncedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks where the fast
// sync pivot point - older than the ancient limit - was already committed, after
// which ssdcead was called. In this case we expect the full chain to be rolled
// back to the committed block. Since the ancient limit was underflown, everything
// needs to be deleted onwards to avoid creating a gap.
func TestLongSnapSyncedDeepSsdcead(t *testing.T)              { testLongSnapSyncedDeepSsdcead(t, false) }
func TestLongSnapSyncedDeepSsdceadWithSnapshots(t *testing.T) { testLongSnapSyncedDeepSsdcead(t, true) }

func testLongSnapSyncedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks where the fast
// sync pivot point - newer than the ancient limit - was not yet committed, but
// ssdcead was called. In this case we expect the chain to detect that it was fast
// syncing and delete everything from the new head, since we can just pick up fast
// syncing from there.
func TestLongSnapSyncingShallowSsdcead(t *testing.T) {
	testLongSnapSyncingShallowSsdcead(t, false)
}
func TestLongSnapSyncingShallowSsdceadWithSnapshots(t *testing.T) {
	testLongSnapSyncingShallowSsdcead(t, true)
}

func testLongSnapSyncingShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks where the fast
// sync pivot point - older than the ancient limit - was not yet committed, but
// ssdcead was called. In this case we expect the chain to detect that it was fast
// syncing and delete everything from the new head, since we can just pick up fast
// syncing from there.
func TestLongSnapSyncingDeepSsdcead(t *testing.T) {
	testLongSnapSyncingDeepSsdcead(t, false)
}
func TestLongSnapSyncingDeepSsdceadWithSnapshots(t *testing.T) {
	testLongSnapSyncingDeepSsdcead(t, true)
}

func testLongSnapSyncingDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4->C5->C6
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    0,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          7,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter side
// chain, where a recent block - newer than the ancient limit - was already committed
// to disk and then ssdcead was called. In this case we expect the canonical full
// chain to be rolled back to the committed block. Everything above the ssdcead point
// should be deleted. In between the committed block and the requested head the data
// can remain as "fast sync" data to avoid redownloading it. The side chain is nuked
// by the freezer.
func TestLongOldForkedShallowSsdcead(t *testing.T) {
	testLongOldForkedShallowSsdcead(t, false)
}
func TestLongOldForkedShallowSsdceadWithSnapshots(t *testing.T) {
	testLongOldForkedShallowSsdcead(t, true)
}

func testLongOldForkedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter side
// chain, where a recent block - older than the ancient limit - was already committed
// to disk and then ssdcead was called. In this case we expect the canonical full
// chain to be rolled back to the committed block. Since the ancient limit was
// underflown, everything needs to be deleted onwards to avoid creating a gap. The
// side chain is nuked by the freezer.
func TestLongOldForkedDeepSsdcead(t *testing.T)              { testLongOldForkedDeepSsdcead(t, false) }
func TestLongOldForkedDeepSsdceadWithSnapshots(t *testing.T) { testLongOldForkedDeepSsdcead(t, true) }

func testLongOldForkedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - newer than the ancient limit -
// was already committed to disk and then ssdcead was called. In this test scenario
// the side chain is below the committed block. In this case we expect the canonical
// full chain to be rolled back to the committed block. Everything above the
// ssdcead point should be deleted. In between the committed block and the
// requested head the data can remain as "fast sync" data to avoid redownloading
// it. The side chain is nuked by the freezer.
func TestLongOldForkedSnapSyncedShallowSsdcead(t *testing.T) {
	testLongOldForkedSnapSyncedShallowSsdcead(t, false)
}
func TestLongOldForkedSnapSyncedShallowSsdceadWithSnapshots(t *testing.T) {
	testLongOldForkedSnapSyncedShallowSsdcead(t, true)
}

func testLongOldForkedSnapSyncedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - older than the ancient limit -
// was already committed to disk and then ssdcead was called. In this test scenario
// the side chain is below the committed block. In this case we expect the canonical
// full chain to be rolled back to the committed block. Since the ancient limit was
// underflown, everything needs to be deleted onwards to avoid creating a gap. The
// side chain is nuked by the freezer.
func TestLongOldForkedSnapSyncedDeepSsdcead(t *testing.T) {
	testLongOldForkedSnapSyncedDeepSsdcead(t, false)
}
func TestLongOldForkedSnapSyncedDeepSsdceadWithSnapshots(t *testing.T) {
	testLongOldForkedSnapSyncedDeepSsdcead(t, true)
}

func testLongOldForkedSnapSyncedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4->C5->C6
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - newer than the ancient limit -
// was not yet committed, but ssdcead was called. In this test scenario the side
// chain is below the committed block. In this case we expect the chain to detect
// that it was fast syncing and delete everything from the new head, since we can
// just pick up fast syncing from there. The side chain is completely nuked by the
// freezer.
func TestLongOldForkedSnapSyncingShallowSsdcead(t *testing.T) {
	testLongOldForkedSnapSyncingShallowSsdcead(t, false)
}
func TestLongOldForkedSnapSyncingShallowSsdceadWithSnapshots(t *testing.T) {
	testLongOldForkedSnapSyncingShallowSsdcead(t, true)
}

func testLongOldForkedSnapSyncingShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - older than the ancient limit -
// was not yet committed, but ssdcead was called. In this test scenario the side
// chain is below the committed block. In this case we expect the chain to detect
// that it was fast syncing and delete everything from the new head, since we can
// just pick up fast syncing from there. The side chain is completely nuked by the
// freezer.
func TestLongOldForkedSnapSyncingDeepSsdcead(t *testing.T) {
	testLongOldForkedSnapSyncingDeepSsdcead(t, false)
}
func TestLongOldForkedSnapSyncingDeepSsdceadWithSnapshots(t *testing.T) {
	testLongOldForkedSnapSyncingDeepSsdcead(t, true)
}

func testLongOldForkedSnapSyncingDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4->C5->C6
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    3,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          7,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where a recent block - newer than the ancient limit - was already
// committed to disk and then ssdcead was called. In this test scenario the side
// chain is above the committed block. In this case the freezer will delete the
// sidechain since it's dangling, reverting to TestLongShallowSsdcead.
func TestLongNewerForkedShallowSsdcead(t *testing.T) {
	testLongNewerForkedShallowSsdcead(t, false)
}
func TestLongNewerForkedShallowSsdceadWithSnapshots(t *testing.T) {
	testLongNewerForkedShallowSsdcead(t, true)
}

func testLongNewerForkedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    12,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where a recent block - older than the ancient limit - was already
// committed to disk and then ssdcead was called. In this test scenario the side
// chain is above the committed block. In this case the freezer will delete the
// sidechain since it's dangling, reverting to TestLongDeepSsdcead.
func TestLongNewerForkedDeepSsdcead(t *testing.T) {
	testLongNewerForkedDeepSsdcead(t, false)
}
func TestLongNewerForkedDeepSsdceadWithSnapshots(t *testing.T) {
	testLongNewerForkedDeepSsdcead(t, true)
}

func testLongNewerForkedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    12,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - newer than the ancient limit -
// was already committed to disk and then ssdcead was called. In this test scenario
// the side chain is above the committed block. In this case the freezer will delete
// the sidechain since it's dangling, reverting to TestLongSnapSyncedShallowSsdcead.
func TestLongNewerForkedSnapSyncedShallowSsdcead(t *testing.T) {
	testLongNewerForkedSnapSyncedShallowSsdcead(t, false)
}
func TestLongNewerForkedSnapSyncedShallowSsdceadWithSnapshots(t *testing.T) {
	testLongNewerForkedSnapSyncedShallowSsdcead(t, true)
}

func testLongNewerForkedSnapSyncedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    12,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - older than the ancient limit -
// was already committed to disk and then ssdcead was called. In this test scenario
// the side chain is above the committed block. In this case the freezer will delete
// the sidechain since it's dangling, reverting to TestLongSnapSyncedDeepSsdcead.
func TestLongNewerForkedSnapSyncedDeepSsdcead(t *testing.T) {
	testLongNewerForkedSnapSyncedDeepSsdcead(t, false)
}
func TestLongNewerForkedSnapSyncedDeepSsdceadWithSnapshots(t *testing.T) {
	testLongNewerForkedSnapSyncedDeepSsdcead(t, true)
}

func testLongNewerForkedSnapSyncedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    12,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - newer than the ancient limit -
// was not yet committed, but ssdcead was called. In this test scenario the side
// chain is above the committed block. In this case the freezer will delete the
// sidechain since it's dangling, reverting to TestLongSnapSyncinghallowSsdcead.
func TestLongNewerForkedSnapSyncingShallowSsdcead(t *testing.T) {
	testLongNewerForkedSnapSyncingShallowSsdcead(t, false)
}
func TestLongNewerForkedSnapSyncingShallowSsdceadWithSnapshots(t *testing.T) {
	testLongNewerForkedSnapSyncingShallowSsdcead(t, true)
}

func testLongNewerForkedSnapSyncingShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    12,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a shorter
// side chain, where the fast sync pivot point - older than the ancient limit -
// was not yet committed, but ssdcead was called. In this test scenario the side
// chain is above the committed block. In this case the freezer will delete the
// sidechain since it's dangling, reverting to TestLongSnapSyncingDeepSsdcead.
func TestLongNewerForkedSnapSyncingDeepSsdcead(t *testing.T) {
	testLongNewerForkedSnapSyncingDeepSsdcead(t, false)
}
func TestLongNewerForkedSnapSyncingDeepSsdceadWithSnapshots(t *testing.T) {
	testLongNewerForkedSnapSyncingDeepSsdcead(t, true)
}

func testLongNewerForkedSnapSyncingDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4->C5->C6
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    12,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          7,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a longer side
// chain, where a recent block - newer than the ancient limit - was already committed
// to disk and then ssdcead was called. In this case the freezer will delete the
// sidechain since it's dangling, reverting to TestLongShallowSsdcead.
func TestLongReorgedShallowSsdcead(t *testing.T)              { testLongReorgedShallowSsdcead(t, false) }
func TestLongReorgedShallowSsdceadWithSnapshots(t *testing.T) { testLongReorgedShallowSsdcead(t, true) }

func testLongReorgedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12->S13->S14->S15->S16->S17->S18->S19->S20->S21->S22->S23->S24->S25->S26
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    26,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a longer side
// chain, where a recent block - older than the ancient limit - was already committed
// to disk and then ssdcead was called. In this case the freezer will delete the
// sidechain since it's dangling, reverting to TestLongDeepSsdcead.
func TestLongReorgedDeepSsdcead(t *testing.T)              { testLongReorgedDeepSsdcead(t, false) }
func TestLongReorgedDeepSsdceadWithSnapshots(t *testing.T) { testLongReorgedDeepSsdcead(t, true) }

func testLongReorgedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12->S13->S14->S15->S16->S17->S18->S19->S20->S21->S22->S23->S24->S25->S26
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : none
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    26,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         nil,
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a longer
// side chain, where the fast sync pivot point - newer than the ancient limit -
// was already committed to disk and then ssdcead was called. In this case the
// freezer will delete the sidechain since it's dangling, reverting to
// TestLongSnapSyncedShallowSsdcead.
func TestLongReorgedSnapSyncedShallowSsdcead(t *testing.T) {
	testLongReorgedSnapSyncedShallowSsdcead(t, false)
}
func TestLongReorgedSnapSyncedShallowSsdceadWithSnapshots(t *testing.T) {
	testLongReorgedSnapSyncedShallowSsdcead(t, true)
}

func testLongReorgedSnapSyncedShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12->S13->S14->S15->S16->S17->S18->S19->S20->S21->S22->S23->S24->S25->S26
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    26,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a longer
// side chain, where the fast sync pivot point - older than the ancient limit -
// was already committed to disk and then ssdcead was called. In this case the
// freezer will delete the sidechain since it's dangling, reverting to
// TestLongSnapSyncedDeepSsdcead.
func TestLongReorgedSnapSyncedDeepSsdcead(t *testing.T) {
	testLongReorgedSnapSyncedDeepSsdcead(t, false)
}
func TestLongReorgedSnapSyncedDeepSsdceadWithSnapshots(t *testing.T) {
	testLongReorgedSnapSyncedDeepSsdcead(t, true)
}

func testLongReorgedSnapSyncedDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12->S13->S14->S15->S16->S17->S18->S19->S20->S21->S22->S23->S24->S25->S26
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G, C4
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C4
	// Expected head fast block: C4
	// Expected head block     : C4
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    26,
		freezsdcreshold:    16,
		commitBlock:        4,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 4,
		expSidechainBlocks: 0,
		expFrozen:          5,
		expHeadHeader:      4,
		expHeadFastBlock:   4,
		expHeadBlock:       4,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a longer
// side chain, where the fast sync pivot point - newer than the ancient limit -
// was not yet committed, but ssdcead was called. In this case we expect the
// chain to detect that it was fast syncing and delete everything from the new
// head, since we can just pick up fast syncing from there. The side chain is
// completely nuked by the freezer.
func TestLongReorgedSnapSyncingShallowSsdcead(t *testing.T) {
	testLongReorgedSnapSyncingShallowSsdcead(t, false)
}
func TestLongReorgedSnapSyncingShallowSsdceadWithSnapshots(t *testing.T) {
	testLongReorgedSnapSyncingShallowSsdcead(t, true)
}

func testLongReorgedSnapSyncingShallowSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12->S13->S14->S15->S16->S17->S18->S19->S20->S21->S22->S23->S24->S25->S26
	//
	// Frozen:
	//   G->C1->C2
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2
	//
	// Expected in leveldb:
	//   C2)->C3->C4->C5->C6
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    18,
		sidechainBlocks:    26,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          3,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

// Tests a ssdcead for a long canonical chain with frozen blocks and a longer
// side chain, where the fast sync pivot point - older than the ancient limit -
// was not yet committed, but ssdcead was called. In this case we expect the
// chain to detect that it was fast syncing and delete everything from the new
// head, since we can just pick up fast syncing from there. The side chain is
// completely nuked by the freezer.
func TestLongReorgedSnapSyncingDeepSsdcead(t *testing.T) {
	testLongReorgedSnapSyncingDeepSsdcead(t, false)
}
func TestLongReorgedSnapSyncingDeepSsdceadWithSnapshots(t *testing.T) {
	testLongReorgedSnapSyncingDeepSsdcead(t, true)
}

func testLongReorgedSnapSyncingDeepSsdcead(t *testing.T, snapshots bool) {
	// Chain:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8->C9->C10->C11->C12->C13->C14->C15->C16->C17->C18->C19->C20->C21->C22->C23->C24 (HEAD)
	//   └->S1->S2->S3->S4->S5->S6->S7->S8->S9->S10->S11->S12->S13->S14->S15->S16->S17->S18->S19->S20->S21->S22->S23->S24->S25->S26
	//
	// Frozen:
	//   G->C1->C2->C3->C4->C5->C6->C7->C8
	//
	// Commit: G
	// Pivot : C4
	//
	// Ssdcead(6)
	//
	// ------------------------------
	//
	// Expected in freezer:
	//   G->C1->C2->C3->C4->C5->C6
	//
	// Expected in leveldb: none
	//
	// Expected head header    : C6
	// Expected head fast block: C6
	// Expected head block     : G
	testSsdcead(t, &rewindTest{
		canonicalBlocks:    24,
		sidechainBlocks:    26,
		freezsdcreshold:    16,
		commitBlock:        0,
		pivotBlock:         uint64ptr(4),
		ssdceadBlock:       6,
		expCanonicalBlocks: 6,
		expSidechainBlocks: 0,
		expFrozen:          7,
		expHeadHeader:      6,
		expHeadFastBlock:   6,
		expHeadBlock:       0,
	}, snapshots)
}

func testSsdcead(t *testing.T, tt *rewindTest, snapshots bool) {
	// It's hard to follow the test case, visualize the input
	// log.Root().Ssdcandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
	// fmt.Println(tt.dump(false))

	// Create a temporary persistent database
	datadir := t.TempDir()

	db, err := rawdb.NewLevelDBDatabaseWithFreezer(datadir, 0, 0, datadir, "", false)
	if err != nil {
		t.Fatalf("Failed to create persistent database: %v", err)
	}
	defer db.Close()

	// Initialize a fresh chain
	var (
		gspec = &Genesis{
			BaseFee: big.NewInt(params.InitialBaseFee),
			Config:  params.AllsdcashProtocolChanges,
		}
		engine = sdcash.NewFullFaker()
		config = &CacheConfig{
			TrieCleanLimit: 256,
			TrieDirtyLimit: 256,
			TrieTimeLimit:  5 * time.Minute,
			SnapshotLimit:  0, // Disable snapshot
		}
	)
	if snapshots {
		config.SnapshotLimit = 256
		config.SnapshotWait = true
	}
	chain, err := NewBlockChain(db, config, gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create chain: %v", err)
	}
	defer chain.Stop()

	// If sidechain blocks are needed, make a light chain and import it
	var sideblocks types.Blocks
	if tt.sidechainBlocks > 0 {
		sideblocks, _ = GenerateChain(gspec.Config, gspec.ToBlock(), engine, rawdb.NewMemoryDatabase(), tt.sidechainBlocks, func(i int, b *BlockGen) {
			b.SetCoinbase(common.Address{0x01})
		})
		if _, err := chain.InsertChain(sideblocks); err != nil {
			t.Fatalf("Failed to import side chain: %v", err)
		}
	}
	canonblocks, _ := GenerateChain(gspec.Config, gspec.ToBlock(), engine, rawdb.NewMemoryDatabase(), tt.canonicalBlocks, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{0x02})
		b.SetDifficulty(big.NewInt(1000000))
	})
	if _, err := chain.InsertChain(canonblocks[:tt.commitBlock]); err != nil {
		t.Fatalf("Failed to import canonical chain start: %v", err)
	}
	if tt.commitBlock > 0 {
		chain.stateCache.TrieDB().Commit(canonblocks[tt.commitBlock-1].Root(), true, nil)
		if snapshots {
			if err := chain.snaps.Cap(canonblocks[tt.commitBlock-1].Root(), 0); err != nil {
				t.Fatalf("Failed to flatten snapshots: %v", err)
			}
		}
	}
	if _, err := chain.InsertChain(canonblocks[tt.commitBlock:]); err != nil {
		t.Fatalf("Failed to import canonical chain tail: %v", err)
	}
	// Manually dereference anything not committed to not have to work with 128+ tries
	for _, block := range sideblocks {
		chain.stateCache.TrieDB().Dereference(block.Root())
	}
	for _, block := range canonblocks {
		chain.stateCache.TrieDB().Dereference(block.Root())
	}
	// Force run a freeze cycle
	type freezer interface {
		Freeze(threshold uint64) error
		Ancients() (uint64, error)
	}
	db.(freezer).Freeze(tt.freezsdcreshold)

	// Set the simulated pivot block
	if tt.pivotBlock != nil {
		rawdb.WriteLastPivotNumber(db, *tt.pivotBlock)
	}
	// Set the head of the chain back to the requested number
	chain.Ssdcead(tt.ssdceadBlock)

	// Iterate over all the remaining blocks and ensure there are no gaps
	verifyNoGaps(t, chain, true, canonblocks)
	verifyNoGaps(t, chain, false, sideblocks)
	verifyCutoff(t, chain, true, canonblocks, tt.expCanonicalBlocks)
	verifyCutoff(t, chain, false, sideblocks, tt.expSidechainBlocks)

	if head := chain.CurrentHeader(); head.Number.Uint64() != tt.expHeadHeader {
		t.Errorf("Head header mismatch: have %d, want %d", head.Number, tt.expHeadHeader)
	}
	if head := chain.CurrentFastBlock(); head.NumberU64() != tt.expHeadFastBlock {
		t.Errorf("Head fast block mismatch: have %d, want %d", head.NumberU64(), tt.expHeadFastBlock)
	}
	if head := chain.CurrentBlock(); head.NumberU64() != tt.expHeadBlock {
		t.Errorf("Head block mismatch: have %d, want %d", head.NumberU64(), tt.expHeadBlock)
	}
	if frozen, err := db.(freezer).Ancients(); err != nil {
		t.Errorf("Failed to retrieve ancient count: %v\n", err)
	} else if int(frozen) != tt.expFrozen {
		t.Errorf("Frozen block count mismatch: have %d, want %d", frozen, tt.expFrozen)
	}
}

// verifyNoGaps checks that there are no gaps after the initial set of blocks in
// the database and errors if found.
func verifyNoGaps(t *testing.T, chain *BlockChain, canonical bool, inserted types.Blocks) {
	t.Helper()

	var end uint64
	for i := uint64(0); i <= uint64(len(inserted)); i++ {
		header := chain.GsdceaderByNumber(i)
		if header == nil && end == 0 {
			end = i
		}
		if header != nil && end > 0 {
			if canonical {
				t.Errorf("Canonical header gap between #%d-#%d", end, i-1)
			} else {
				t.Errorf("Sidechain header gap between #%d-#%d", end, i-1)
			}
			end = 0 // Reset for further gap detection
		}
	}
	end = 0
	for i := uint64(0); i <= uint64(len(inserted)); i++ {
		block := chain.GetBlockByNumber(i)
		if block == nil && end == 0 {
			end = i
		}
		if block != nil && end > 0 {
			if canonical {
				t.Errorf("Canonical block gap between #%d-#%d", end, i-1)
			} else {
				t.Errorf("Sidechain block gap between #%d-#%d", end, i-1)
			}
			end = 0 // Reset for further gap detection
		}
	}
	end = 0
	for i := uint64(1); i <= uint64(len(inserted)); i++ {
		receipts := chain.GetReceiptsByHash(inserted[i-1].Hash())
		if receipts == nil && end == 0 {
			end = i
		}
		if receipts != nil && end > 0 {
			if canonical {
				t.Errorf("Canonical receipt gap between #%d-#%d", end, i-1)
			} else {
				t.Errorf("Sidechain receipt gap between #%d-#%d", end, i-1)
			}
			end = 0 // Reset for further gap detection
		}
	}
}

// verifyCutoff checks that there are no chain data available in the chain after
// the specified limit, but that it is available before.
func verifyCutoff(t *testing.T, chain *BlockChain, canonical bool, inserted types.Blocks, head int) {
	t.Helper()

	for i := 1; i <= len(inserted); i++ {
		if i <= head {
			if header := chain.Gsdceader(inserted[i-1].Hash(), uint64(i)); header == nil {
				if canonical {
					t.Errorf("Canonical header   #%2d [%x...] missing before cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				} else {
					t.Errorf("Sidechain header   #%2d [%x...] missing before cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				}
			}
			if block := chain.GetBlock(inserted[i-1].Hash(), uint64(i)); block == nil {
				if canonical {
					t.Errorf("Canonical block    #%2d [%x...] missing before cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				} else {
					t.Errorf("Sidechain block    #%2d [%x...] missing before cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				}
			}
			if receipts := chain.GetReceiptsByHash(inserted[i-1].Hash()); receipts == nil {
				if canonical {
					t.Errorf("Canonical receipts #%2d [%x...] missing before cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				} else {
					t.Errorf("Sidechain receipts #%2d [%x...] missing before cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				}
			}
		} else {
			if header := chain.Gsdceader(inserted[i-1].Hash(), uint64(i)); header != nil {
				if canonical {
					t.Errorf("Canonical header   #%2d [%x...] present after cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				} else {
					t.Errorf("Sidechain header   #%2d [%x...] present after cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				}
			}
			if block := chain.GetBlock(inserted[i-1].Hash(), uint64(i)); block != nil {
				if canonical {
					t.Errorf("Canonical block    #%2d [%x...] present after cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				} else {
					t.Errorf("Sidechain block    #%2d [%x...] present after cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				}
			}
			if receipts := chain.GetReceiptsByHash(inserted[i-1].Hash()); receipts != nil {
				if canonical {
					t.Errorf("Canonical receipts #%2d [%x...] present after cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				} else {
					t.Errorf("Sidechain receipts #%2d [%x...] present after cap %d", inserted[i-1].Number(), inserted[i-1].Hash().Bytes()[:3], head)
				}
			}
		}
	}
}

// uint64ptr is a weird helper to allow 1-line constant pointer creation.
func uint64ptr(n uint64) *uint64 {
	return &n
}
