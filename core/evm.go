// Copyright 2016 The go-sdcereum Authors
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

package core

import (
	"math/big"

	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/consensus"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/core/vm"
)

// ChainContext supports retrieving headers and consensus parameters from the
// current blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the chain's consensus engine.
	Engine() consensus.Engine

	// Gsdceader returns the header corresponding to the hash/number argument pair.
	Gsdceader(common.Hash, uint64) *types.Header
}

// NewEVMBlockContext creates a new context for use in the EVM.
func NewEVMBlockContext(header *types.Header, chain ChainContext, author *common.Address) vm.BlockContext {
	var (
		beneficiary common.Address
		baseFee     *big.Int
		random      *common.Hash
	)

	// If we don't have an explicit author (i.e. not mining), extract from the header
	if author == nil {
		beneficiary, _ = chain.Engine().Author(header) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}
	if header.BaseFee != nil {
		baseFee = new(big.Int).Set(header.BaseFee)
	}
	if header.Difficulty.Cmp(common.Big0) == 0 {
		random = &header.MixDigest
	}
	return vm.BlockContext{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
		Gsdcash:     GsdcashFn(header, chain),
		Coinbase:    beneficiary,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).SetUint64(header.Time),
		Difficulty:  new(big.Int).Set(header.Difficulty),
		BaseFee:     baseFee,
		GasLimit:    header.GasLimit,
		Random:      random,
	}
}

// NewEVMTxContext creates a new transaction context for a single transaction.
func NewEVMTxContext(msg Message) vm.TxContext {
	return vm.TxContext{
		Origin:   msg.From(),
		GasPrice: new(big.Int).Set(msg.GasPrice()),
	}
}

// GsdcashFn returns a GsdcashFunc which retrieves header hashes by number
func GsdcashFn(ref *types.Header, chain ChainContext) func(n uint64) common.Hash {
	// Cache will initially contain [refHash.parent],
	// Then fill up with [refHash.p, refHash.pp, refHash.ppp, ...]
	var cache []common.Hash

	return func(n uint64) common.Hash {
		if ref.Number.Uint64() <= n {
			// This situation can happen if we're doing tracing and using
			// block overrides.
			return common.Hash{}
		}
		// If there's no hash cache yet, make one
		if len(cache) == 0 {
			cache = append(cache, ref.ParentHash)
		}
		if idx := ref.Number.Uint64() - n - 1; idx < uint64(len(cache)) {
			return cache[idx]
		}
		// No luck in the cache, but we can start iterating from the last element we already know
		lastKnownHash := cache[len(cache)-1]
		lastKnownNumber := ref.Number.Uint64() - uint64(len(cache))

		for {
			header := chain.Gsdceader(lastKnownHash, lastKnownNumber)
			if header == nil {
				break
			}
			cache = append(cache, header.ParentHash)
			lastKnownHash = header.ParentHash
			lastKnownNumber = header.Number.Uint64() - 1
			if n == lastKnownNumber {
				return lastKnownHash
			}
		}
		return common.Hash{}
	}
}

// CanTransfer checks whsdcer there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas in to account to make the transfer valid.
func CanTransfer(db vm.StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}
