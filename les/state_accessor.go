// Copyright 2021 The go-sdcereum Authors
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
	"context"
	"errors"
	"fmt"

	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/core/state"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/core/vm"
	"github.com/sdcereum/go-sdcereum/sdc/tracers"
	"github.com/sdcereum/go-sdcereum/light"
)

// noopReleaser is returned in case there is no operation expected
// for releasing state.
var noopReleaser = tracers.StateReleaseFunc(func() {})

// stateAtBlock retrieves the state database associated with a certain block.
func (lsdc *Lightsdcereum) stateAtBlock(ctx context.Context, block *types.Block, reexec uint64) (*state.StateDB, tracers.StateReleaseFunc, error) {
	return light.NewState(ctx, block.Header(), lsdc.odr), noopReleaser, nil
}

// stateAtTransaction returns the execution environment of a certain transaction.
func (lsdc *Lightsdcereum) stateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, *state.StateDB, tracers.StateReleaseFunc, error) {
	// Short circuit if it's genesis block.
	if block.NumberU64() == 0 {
		return nil, vm.BlockContext{}, nil, nil, errors.New("no transaction in genesis")
	}
	// Create the parent state database
	parent, err := lsdc.blockchain.GetBlock(ctx, block.ParentHash(), block.NumberU64()-1)
	if err != nil {
		return nil, vm.BlockContext{}, nil, nil, err
	}
	statedb, release, err := lsdc.stateAtBlock(ctx, parent, reexec)
	if err != nil {
		return nil, vm.BlockContext{}, nil, nil, err
	}
	if txIndex == 0 && len(block.Transactions()) == 0 {
		return nil, vm.BlockContext{}, statedb, release, nil
	}
	// Recompute transactions up to the target index.
	signer := types.MakeSigner(lsdc.blockchain.Config(), block.Number())
	for idx, tx := range block.Transactions() {
		// Assemble the transaction call message and return if the requested offset
		msg, _ := tx.AsMessage(signer, block.BaseFee())
		txContext := core.NewEVMTxContext(msg)
		context := core.NewEVMBlockContext(block.Header(), lsdc.blockchain, nil)
		statedb.Prepare(tx.Hash(), idx)
		if idx == txIndex {
			return msg, context, statedb, release, nil
		}
		// Not yet the searched for transaction, execute on top of the current state
		vmenv := vm.NewEVM(context, txContext, statedb, lsdc.blockchain.Config(), vm.Config{})
		if _, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(tx.Gas())); err != nil {
			return nil, vm.BlockContext{}, nil, nil, fmt.Errorf("transaction %#x failed: %v", tx.Hash(), err)
		}
		// Ensure any modifications are committed to the state
		// Only delete empty objects if EIP158/161 (a.k.a Spurious Dragon) is in effect
		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	return nil, vm.BlockContext{}, nil, nil, fmt.Errorf("transaction index %d out of range for block %#x", txIndex, block.Hash())
}
