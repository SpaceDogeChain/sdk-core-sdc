// Copyright 2014 The go-sdcereum Authors
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

// Package sdc implements the sdcereum protocol.
package sdc

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/sdcereum/go-sdcereum/accounts"
	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/common/hexutil"
	"github.com/sdcereum/go-sdcereum/consensus"
	"github.com/sdcereum/go-sdcereum/consensus/beacon"
	"github.com/sdcereum/go-sdcereum/consensus/clique"
	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/core/bloombits"
	"github.com/sdcereum/go-sdcereum/core/rawdb"
	"github.com/sdcereum/go-sdcereum/core/state/pruner"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/core/vm"
	"github.com/sdcereum/go-sdcereum/sdc/downloader"
	"github.com/sdcereum/go-sdcereum/sdc/sdcconfig"
	"github.com/sdcereum/go-sdcereum/sdc/gasprice"
	"github.com/sdcereum/go-sdcereum/sdc/protocols/sdc"
	"github.com/sdcereum/go-sdcereum/sdc/protocols/snap"
	"github.com/sdcereum/go-sdcereum/sdcdb"
	"github.com/sdcereum/go-sdcereum/event"
	"github.com/sdcereum/go-sdcereum/internal/sdcapi"
	"github.com/sdcereum/go-sdcereum/internal/shutdowncheck"
	"github.com/sdcereum/go-sdcereum/log"
	"github.com/sdcereum/go-sdcereum/miner"
	"github.com/sdcereum/go-sdcereum/node"
	"github.com/sdcereum/go-sdcereum/p2p"
	"github.com/sdcereum/go-sdcereum/p2p/dnsdisc"
	"github.com/sdcereum/go-sdcereum/p2p/enode"
	"github.com/sdcereum/go-sdcereum/params"
	"github.com/sdcereum/go-sdcereum/rlp"
	"github.com/sdcereum/go-sdcereum/rpc"
)

// Config contains the configuration options of the sdc protocol.
// Deprecated: use sdcconfig.Config instead.
type Config = sdcconfig.Config

// sdcereum implements the sdcereum full node service.
type sdcereum struct {
	config *sdcconfig.Config

	// Handlers
	txPool             *core.TxPool
	blockchain         *core.BlockChain
	handler            *handler
	sdcDialCandidates  enode.Iterator
	snapDialCandidates enode.Iterator
	merger             *consensus.Merger

	// DB interfaces
	chainDb sdcdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests     chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer      *core.ChainIndexer             // Bloom indexer operating during block imports
	closeBloomHandler chan struct{}

	APIBackend *sdcAPIBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	sdcerbase common.Address

	networkID     uint64
	netRPCService *sdcapi.NetAPI

	p2pServer *p2p.Server

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and sdcerbase)

	shutdownTracker *shutdowncheck.ShutdownTracker // Tracks if and when the node has shutdown ungracefully
}

// New creates a new sdcereum object (including the
// initialisation of the common sdcereum object)
func New(stack *node.Node, config *sdcconfig.Config) (*sdcereum, error) {
	// Ensure configuration values are compatible and sane
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run sdc.sdcereum in light sync mode, use les.Lightsdcereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if config.Miner.GasPrice == nil || config.Miner.GasPrice.Cmp(common.Big0) <= 0 {
		log.Warn("Sanitizing invalid miner gas price", "provided", config.Miner.GasPrice, "updated", sdcconfig.Defaults.Miner.GasPrice)
		config.Miner.GasPrice = new(big.Int).Set(sdcconfig.Defaults.Miner.GasPrice)
	}
	if config.NoPruning && config.TrieDirtyCache > 0 {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	// Assemble the sdcereum object
	chainDb, err := stack.OpenDatabaseWithFreezer("chaindata", config.DatabaseCache, config.DatabaseHandles, config.DatabaseFreezer, "sdc/db/chaindata/", false)
	if err != nil {
		return nil, err
	}
	if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb, stack.ResolvePath(config.TrieCleanCacheJournal)); err != nil {
		log.Error("Failed to recover state", "error", err)
	}
	// Transfer mining-related config to the sdcash config.
	sdcashConfig := config.sdcash
	sdcashConfig.NotifyFull = config.Miner.NotifyFull
	cliqueConfig, err := core.LoadCliqueConfig(chainDb, config.Genesis)
	if err != nil {
		return nil, err
	}
	engine := sdcconfig.CreateConsensusEngine(stack, &sdcashConfig, cliqueConfig, config.Miner.Notify, config.Miner.Noverify, chainDb)

	sdc := &sdcereum{
		config:            config,
		merger:            consensus.NewMerger(chainDb),
		chainDb:           chainDb,
		eventMux:          stack.EventMux(),
		accountManager:    stack.AccountManager(),
		engine:            engine,
		closeBloomHandler: make(chan struct{}),
		networkID:         config.NetworkId,
		gasPrice:          config.Miner.GasPrice,
		sdcerbase:         config.Miner.sdcerbase,
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      core.NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms),
		p2pServer:         stack.Server(),
		shutdownTracker:   shutdowncheck.NewShutdownTracker(chainDb),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Info("Initialising sdcereum protocol", "network", config.NetworkId, "dbversion", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Gsdc %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			if bcVersion != nil { // only print warning on upgrade, not on init
				log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			}
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	var (
		vmConfig = vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
		}
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanJournal:    stack.ResolvePath(config.TrieCleanCacheJournal),
			TrieCleanRejournal:  config.TrieCleanCacheRejournal,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
			SnapshotLimit:       config.SnapshotCache,
			Preimages:           config.Preimages,
		}
	)
	// Override the chain config with provided settings.
	var overrides core.ChainOverrides
	if config.OverrideTerminalTotalDifficulty != nil {
		overrides.OverrideTerminalTotalDifficulty = config.OverrideTerminalTotalDifficulty
	}
	if config.OverrideTerminalTotalDifficultyPassed != nil {
		overrides.OverrideTerminalTotalDifficultyPassed = config.OverrideTerminalTotalDifficultyPassed
	}
	sdc.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, config.Genesis, &overrides, sdc.engine, vmConfig, sdc.shouldPreserve, &config.TxLookupLimit)
	if err != nil {
		return nil, err
	}
	sdc.bloomIndexer.Start(sdc.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
	}
	sdc.txPool = core.NewTxPool(config.TxPool, sdc.blockchain.Config(), sdc.blockchain)

	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit + cacheConfig.SnapshotLimit
	checkpoint := config.Checkpoint
	if checkpoint == nil {
		checkpoint = params.TrustedCheckpoints[sdc.blockchain.Genesis().Hash()]
	}
	if sdc.handler, err = newHandler(&handlerConfig{
		Database:       chainDb,
		Chain:          sdc.blockchain,
		TxPool:         sdc.txPool,
		Merger:         sdc.merger,
		Network:        config.NetworkId,
		Sync:           config.SyncMode,
		BloomCache:     uint64(cacheLimit),
		EventMux:       sdc.eventMux,
		Checkpoint:     checkpoint,
		RequiredBlocks: config.RequiredBlocks,
	}); err != nil {
		return nil, err
	}

	sdc.miner = miner.New(sdc, &config.Miner, sdc.blockchain.Config(), sdc.EventMux(), sdc.engine, sdc.isLocalBlock)
	sdc.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	sdc.APIBackend = &sdcAPIBackend{stack.Config().ExtRPCEnabled(), stack.Config().AllowUnprotectedTxs, sdc, nil}
	if sdc.APIBackend.allowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.Miner.GasPrice
	}
	sdc.APIBackend.gpo = gasprice.NewOracle(sdc.APIBackend, gpoParams)

	// Setup DNS discovery iterators.
	dnsclient := dnsdisc.NewClient(dnsdisc.Config{})
	sdc.sdcDialCandidates, err = dnsclient.NewIterator(sdc.config.sdcDiscoveryURLs...)
	if err != nil {
		return nil, err
	}
	sdc.snapDialCandidates, err = dnsclient.NewIterator(sdc.config.SnapDiscoveryURLs...)
	if err != nil {
		return nil, err
	}

	// Start the RPC service
	sdc.netRPCService = sdcapi.NewNetAPI(sdc.p2pServer, config.NetworkId)

	// Register the backend on the node
	stack.RegisterAPIs(sdc.APIs())
	stack.RegisterProtocols(sdc.Protocols())
	stack.RegisterLifecycle(sdc)

	// Successful startup; push a marker and check previous unclean shutdowns.
	sdc.shutdownTracker.MarkStartup()

	return sdc, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"gsdc",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// APIs return the collection of RPC services the sdcereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *sdcereum) APIs() []rpc.API {
	apis := sdcapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "sdc",
			Service:   NewsdcereumAPI(s),
		}, {
			Namespace: "miner",
			Service:   NewMinerAPI(s),
		}, {
			Namespace: "sdc",
			Service:   downloader.NewDownloaderAPI(s.handler.downloader, s.eventMux),
		}, {
			Namespace: "admin",
			Service:   NewAdminAPI(s),
		}, {
			Namespace: "debug",
			Service:   NewDebugAPI(s),
		}, {
			Namespace: "net",
			Service:   s.netRPCService,
		},
	}...)
}

func (s *sdcereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *sdcereum) sdcerbase() (eb common.Address, err error) {
	s.lock.RLock()
	sdcerbase := s.sdcerbase
	s.lock.RUnlock()

	if sdcerbase != (common.Address{}) {
		return sdcerbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			sdcerbase := accounts[0].Address

			s.lock.Lock()
			s.sdcerbase = sdcerbase
			s.lock.Unlock()

			log.Info("sdcerbase automatically configured", "address", sdcerbase)
			return sdcerbase, nil
		}
	}
	return common.Address{}, fmt.Errorf("sdcerbase must be explicitly specified")
}

// isLocalBlock checks whsdcer the specified block is mined
// by local miner accounts.
//
// We regard two types of accounts as local miner account: sdcerbase
// and accounts specified via `txpool.locals` flag.
func (s *sdcereum) isLocalBlock(header *types.Header) bool {
	author, err := s.engine.Author(header)
	if err != nil {
		log.Warn("Failed to retrieve block author", "number", header.Number.Uint64(), "hash", header.Hash(), "err", err)
		return false
	}
	// Check whsdcer the given address is sdcerbase.
	s.lock.RLock()
	sdcerbase := s.sdcerbase
	s.lock.RUnlock()
	if author == sdcerbase {
		return true
	}
	// Check whsdcer the given address is specified by `txpool.local`
	// CLI flag.
	for _, account := range s.config.TxPool.Locals {
		if account == author {
			return true
		}
	}
	return false
}

// shouldPreserve checks whsdcer we should preserve the given block
// during the chain reorg depending on whsdcer the author of block
// is a local account.
func (s *sdcereum) shouldPreserve(header *types.Header) bool {
	// The reason we need to disable the self-reorg preserving for clique
	// is it can be probable to introduce a deadlock.
	//
	// e.g. If there are 7 available signers
	//
	// r1   A
	// r2     B
	// r3       C
	// r4         D
	// r5   A      [X] F G
	// r6    [X]
	//
	// In the round5, the inturn signer E is offline, so the worst case
	// is A, F and G sign the block of round5 and reject the block of opponents
	// and in the round6, the last available signer B is offline, the whole
	// network is stuck.
	if _, ok := s.engine.(*clique.Clique); ok {
		return false
	}
	return s.isLocalBlock(header)
}

// Setsdcerbase sets the mining reward address.
func (s *sdcereum) Setsdcerbase(sdcerbase common.Address) {
	s.lock.Lock()
	s.sdcerbase = sdcerbase
	s.lock.Unlock()

	s.miner.Setsdcerbase(sdcerbase)
}

// StartMining starts the miner with the given number of CPU threads. If mining
// is already running, this msdcod adjust the number of threads allowed to use
// and updates the minimum price required by the transaction pool.
func (s *sdcereum) StartMining(threads int) error {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := s.engine.(threaded); ok {
		log.Info("Updated mining threads", "threads", threads)
		if threads == 0 {
			threads = -1 // Disable the miner from within
		}
		th.SetThreads(threads)
	}
	// If the miner was not running, initialize it
	if !s.IsMining() {
		// Propagate the initial price point to the transaction pool
		s.lock.RLock()
		price := s.gasPrice
		s.lock.RUnlock()
		s.txPool.SetGasPrice(price)

		// Configure the local mining address
		eb, err := s.sdcerbase()
		if err != nil {
			log.Error("Cannot start mining without sdcerbase", "err", err)
			return fmt.Errorf("sdcerbase missing: %v", err)
		}
		var cli *clique.Clique
		if c, ok := s.engine.(*clique.Clique); ok {
			cli = c
		} else if cl, ok := s.engine.(*beacon.Beacon); ok {
			if c, ok := cl.InnerEngine().(*clique.Clique); ok {
				cli = c
			}
		}
		if cli != nil {
			wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
			if wallet == nil || err != nil {
				log.Error("sdcerbase account unavailable locally", "err", err)
				return fmt.Errorf("signer missing: %v", err)
			}
			cli.Authorize(eb, wallet.SignData)
		}
		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.handler.acceptTxs, 1)

		go s.miner.Start(eb)
	}
	return nil
}

// StopMining terminates the miner, both at the consensus engine level as well as
// at the block creation level.
func (s *sdcereum) StopMining() {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := s.engine.(threaded); ok {
		th.SetThreads(-1)
	}
	// Stop the block creating itself
	s.miner.Stop()
}

func (s *sdcereum) IsMining() bool      { return s.miner.Mining() }
func (s *sdcereum) Miner() *miner.Miner { return s.miner }

func (s *sdcereum) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *sdcereum) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *sdcereum) TxPool() *core.TxPool               { return s.txPool }
func (s *sdcereum) EventMux() *event.TypeMux           { return s.eventMux }
func (s *sdcereum) Engine() consensus.Engine           { return s.engine }
func (s *sdcereum) ChainDb() sdcdb.Database            { return s.chainDb }
func (s *sdcereum) IsListening() bool                  { return true } // Always listening
func (s *sdcereum) Downloader() *downloader.Downloader { return s.handler.downloader }
func (s *sdcereum) Synced() bool                       { return atomic.LoadUint32(&s.handler.acceptTxs) == 1 }
func (s *sdcereum) SetSynced()                         { atomic.StoreUint32(&s.handler.acceptTxs, 1) }
func (s *sdcereum) ArchiveMode() bool                  { return s.config.NoPruning }
func (s *sdcereum) BloomIndexer() *core.ChainIndexer   { return s.bloomIndexer }
func (s *sdcereum) Merger() *consensus.Merger          { return s.merger }
func (s *sdcereum) SyncMode() downloader.SyncMode {
	mode, _ := s.handler.chainSync.modeAndLocalHead()
	return mode
}

// Protocols returns all the currently configured
// network protocols to start.
func (s *sdcereum) Protocols() []p2p.Protocol {
	protos := sdc.MakeProtocols((*sdcHandler)(s.handler), s.networkID, s.sdcDialCandidates)
	if s.config.SnapshotCache > 0 {
		protos = append(protos, snap.MakeProtocols((*snapHandler)(s.handler), s.snapDialCandidates)...)
	}
	return protos
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// sdcereum protocol implementation.
func (s *sdcereum) Start() error {
	sdc.StartENRUpdater(s.blockchain, s.p2pServer.LocalNode())

	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(params.BloomBitsBlocks)

	// Regularly update shutdown marker
	s.shutdownTracker.Start()

	// Figure out a max peers count based on the server limits
	maxPeers := s.p2pServer.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= s.p2pServer.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, s.p2pServer.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.handler.Start(maxPeers)
	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// sdcereum protocol.
func (s *sdcereum) Stop() error {
	// Stop all the peer-related stuff first.
	s.sdcDialCandidates.Close()
	s.snapDialCandidates.Close()
	s.handler.Stop()

	// Then stop everything else.
	s.bloomIndexer.Close()
	close(s.closeBloomHandler)
	s.txPool.Stop()
	s.miner.Close()
	s.blockchain.Stop()
	s.engine.Close()

	// Clean shutdown marker as the last thing before closing db
	s.shutdownTracker.Stop()

	s.chainDb.Close()
	s.eventMux.Stop()

	return nil
}
