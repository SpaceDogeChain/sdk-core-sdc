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

// Package sdcapi implements the general sdcereum API functions.
package sdcapi

import (
	"context"
	"math/big"
	"time"

	"github.com/sdcereum/go-sdcereum"
	"github.com/sdcereum/go-sdcereum/accounts"
	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/consensus"
	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/core/state"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/core/vm"
	"github.com/sdcereum/go-sdcereum/sdc/filters"
	"github.com/sdcereum/go-sdcereum/sdcdb"
	"github.com/sdcereum/go-sdcereum/event"
	"github.com/sdcereum/go-sdcereum/params"
	"github.com/sdcereum/go-sdcereum/rpc"
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	// General sdcereum API
	SyncProgress() sdcereum.SyncProgress

	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	FeeHistory(ctx context.Context, blockCount int, lastBlock rpc.BlockNumber, rewardPercentiles []float64) (*big.Int, [][]*big.Int, []*big.Int, []float64, error)
	ChainDb() sdcdb.Database
	AccountManager() *accounts.Manager
	ExtRPCEnabled() bool
	RPCGasCap() uint64            // global gas cap for sdc_call over rpc: DoS protection
	RPCEVMTimeout() time.Duration // global timeout for sdc_call over rpc: DoS protection
	RPCTxFeeCap() float64         // global tx fee cap for all transaction related APIs
	UnprotectedAllowed() bool     // allows only for EIP155 transactions.

	// Blockchain API
	Ssdcead(number uint64)
	HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error)
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error)
	CurrentHeader() *types.Header
	CurrentBlock() *types.Block
	BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error)
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error)
	StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error)
	StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error)
	PendingBlockAndReceipts() (*types.Block, types.Receipts)
	GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error)
	GetTd(ctx context.Context, hash common.Hash) *big.Int
	GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmConfig *vm.Config) (*vm.EVM, func() error, error)
	SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription
	SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription

	// Transaction pool API
	SendTx(ctx context.Context, signedTx *types.Transaction) error
	GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error)
	GetPoolTransactions() (types.Transactions, error)
	GetPoolTransaction(txHash common.Hash) *types.Transaction
	GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error)
	Stats() (pending int, queued int)
	TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions)
	TxPoolContentFrom(addr common.Address) (types.Transactions, types.Transactions)
	SubscribeNewTxsEvent(chan<- core.NewTxsEvent) event.Subscription

	ChainConfig() *params.ChainConfig
	Engine() consensus.Engine

	// sdc/filters needs to be initialized from this backend type, so msdcods needed by
	// it must also be included here.
	filters.Backend
}

func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	return []rpc.API{
		{
			Namespace: "sdc",
			Service:   NewsdcereumAPI(apiBackend),
		}, {
			Namespace: "sdc",
			Service:   NewBlockChainAPI(apiBackend),
		}, {
			Namespace: "sdc",
			Service:   NewTransactionAPI(apiBackend, nonceLock),
		}, {
			Namespace: "txpool",
			Service:   NewTxPoolAPI(apiBackend),
		}, {
			Namespace: "debug",
			Service:   NewDebugAPI(apiBackend),
		}, {
			Namespace: "sdc",
			Service:   NewsdcereumAccountAPI(apiBackend.AccountManager()),
		}, {
			Namespace: "personal",
			Service:   NewPersonalAccountAPI(apiBackend, nonceLock),
		},
	}
}
