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
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/sdcereum/go-sdcereum"
	"github.com/sdcereum/go-sdcereum/accounts"
	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/consensus"
	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/core/bloombits"
	"github.com/sdcereum/go-sdcereum/core/rawdb"
	"github.com/sdcereum/go-sdcereum/core/state"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/core/vm"
	"github.com/sdcereum/go-sdcereum/sdc/gasprice"
	"github.com/sdcereum/go-sdcereum/sdc/tracers"
	"github.com/sdcereum/go-sdcereum/sdcdb"
	"github.com/sdcereum/go-sdcereum/event"
	"github.com/sdcereum/go-sdcereum/miner"
	"github.com/sdcereum/go-sdcereum/params"
	"github.com/sdcereum/go-sdcereum/rpc"
)

// sdcAPIBackend implements sdcapi.Backend for full nodes
type sdcAPIBackend struct {
	extRPCEnabled       bool
	allowUnprotectedTxs bool
	sdc                 *sdcereum
	gpo                 *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *sdcAPIBackend) ChainConfig() *params.ChainConfig {
	return b.sdc.blockchain.Config()
}

func (b *sdcAPIBackend) CurrentBlock() *types.Block {
	return b.sdc.blockchain.CurrentBlock()
}

func (b *sdcAPIBackend) Ssdcead(number uint64) {
	b.sdc.handler.downloader.Cancel()
	b.sdc.blockchain.Ssdcead(number)
}

func (b *sdcAPIBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.sdc.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.sdc.blockchain.CurrentBlock().Header(), nil
	}
	if number == rpc.FinalizedBlockNumber {
		block := b.sdc.blockchain.CurrentFinalizedBlock()
		if block != nil {
			return block.Header(), nil
		}
		return nil, errors.New("finalized block not found")
	}
	if number == rpc.SafeBlockNumber {
		block := b.sdc.blockchain.CurrentSafeBlock()
		if block != nil {
			return block.Header(), nil
		}
		return nil, errors.New("safe block not found")
	}
	return b.sdc.blockchain.GsdceaderByNumber(uint64(number)), nil
}

func (b *sdcAPIBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.sdc.blockchain.GsdceaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.sdc.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *sdcAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.sdc.blockchain.GsdceaderByHash(hash), nil
}

func (b *sdcAPIBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.sdc.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.sdc.blockchain.CurrentBlock(), nil
	}
	if number == rpc.FinalizedBlockNumber {
		return b.sdc.blockchain.CurrentFinalizedBlock(), nil
	}
	if number == rpc.SafeBlockNumber {
		return b.sdc.blockchain.CurrentSafeBlock(), nil
	}
	return b.sdc.blockchain.GetBlockByNumber(uint64(number)), nil
}

func (b *sdcAPIBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.sdc.blockchain.GetBlockByHash(hash), nil
}

func (b *sdcAPIBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.sdc.blockchain.GsdceaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.sdc.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.sdc.blockchain.GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *sdcAPIBackend) PendingBlockAndReceipts() (*types.Block, types.Receipts) {
	return b.sdc.miner.PendingBlockAndReceipts()
}

func (b *sdcAPIBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		block, state := b.sdc.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.sdc.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *sdcAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.sdc.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.sdc.BlockChain().StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *sdcAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return b.sdc.blockchain.GetReceiptsByHash(hash), nil
}

func (b *sdcAPIBackend) GetLogs(ctx context.Context, hash common.Hash, number uint64) ([][]*types.Log, error) {
	return rawdb.ReadLogs(b.sdc.chainDb, hash, number, b.ChainConfig()), nil
}

func (b *sdcAPIBackend) GetTd(ctx context.Context, hash common.Hash) *big.Int {
	if header := b.sdc.blockchain.GsdceaderByHash(hash); header != nil {
		return b.sdc.blockchain.GetTd(hash, header.Number.Uint64())
	}
	return nil
}

func (b *sdcAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmConfig *vm.Config) (*vm.EVM, func() error, error) {
	if vmConfig == nil {
		vmConfig = b.sdc.blockchain.GetVMConfig()
	}
	txContext := core.NewEVMTxContext(msg)
	context := core.NewEVMBlockContext(header, b.sdc.BlockChain(), nil)
	return vm.NewEVM(context, txContext, state, b.sdc.blockchain.Config(), *vmConfig), state.Error, nil
}

func (b *sdcAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.sdc.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *sdcAPIBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.sdc.miner.SubscribePendingLogs(ch)
}

func (b *sdcAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.sdc.BlockChain().SubscribeChainEvent(ch)
}

func (b *sdcAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.sdc.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *sdcAPIBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.sdc.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *sdcAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.sdc.BlockChain().SubscribeLogsEvent(ch)
}

func (b *sdcAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.sdc.txPool.AddLocal(signedTx)
}

func (b *sdcAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending := b.sdc.txPool.Pending(false)
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *sdcAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.sdc.txPool.Get(hash)
}

func (b *sdcAPIBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.sdc.ChainDb(), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *sdcAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.sdc.txPool.Nonce(addr), nil
}

func (b *sdcAPIBackend) Stats() (pending int, queued int) {
	return b.sdc.txPool.Stats()
}

func (b *sdcAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.sdc.TxPool().Content()
}

func (b *sdcAPIBackend) TxPoolContentFrom(addr common.Address) (types.Transactions, types.Transactions) {
	return b.sdc.TxPool().ContentFrom(addr)
}

func (b *sdcAPIBackend) TxPool() *core.TxPool {
	return b.sdc.TxPool()
}

func (b *sdcAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.sdc.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *sdcAPIBackend) SyncProgress() sdcereum.SyncProgress {
	return b.sdc.Downloader().Progress()
}

func (b *sdcAPIBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestTipCap(ctx)
}

func (b *sdcAPIBackend) FeeHistory(ctx context.Context, blockCount int, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (firstBlock *big.Int, reward [][]*big.Int, baseFee []*big.Int, gasUsedRatio []float64, err error) {
	return b.gpo.FeeHistory(ctx, blockCount, lastBlock, rewardPercentiles)
}

func (b *sdcAPIBackend) ChainDb() sdcdb.Database {
	return b.sdc.ChainDb()
}

func (b *sdcAPIBackend) EventMux() *event.TypeMux {
	return b.sdc.EventMux()
}

func (b *sdcAPIBackend) AccountManager() *accounts.Manager {
	return b.sdc.AccountManager()
}

func (b *sdcAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *sdcAPIBackend) UnprotectedAllowed() bool {
	return b.allowUnprotectedTxs
}

func (b *sdcAPIBackend) RPCGasCap() uint64 {
	return b.sdc.config.RPCGasCap
}

func (b *sdcAPIBackend) RPCEVMTimeout() time.Duration {
	return b.sdc.config.RPCEVMTimeout
}

func (b *sdcAPIBackend) RPCTxFeeCap() float64 {
	return b.sdc.config.RPCTxFeeCap
}

func (b *sdcAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.sdc.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *sdcAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.sdc.bloomRequests)
	}
}

func (b *sdcAPIBackend) Engine() consensus.Engine {
	return b.sdc.engine
}

func (b *sdcAPIBackend) CurrentHeader() *types.Header {
	return b.sdc.blockchain.CurrentHeader()
}

func (b *sdcAPIBackend) Miner() *miner.Miner {
	return b.sdc.Miner()
}

func (b *sdcAPIBackend) StartMining(threads int) error {
	return b.sdc.StartMining(threads)
}

func (b *sdcAPIBackend) StateAtBlock(ctx context.Context, block *types.Block, reexec uint64, base *state.StateDB, readOnly bool, preferDisk bool) (*state.StateDB, tracers.StateReleaseFunc, error) {
	return b.sdc.StateAtBlock(block, reexec, base, readOnly, preferDisk)
}

func (b *sdcAPIBackend) StateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, *state.StateDB, tracers.StateReleaseFunc, error) {
	return b.sdc.stateAtTransaction(block, txIndex, reexec)
}
