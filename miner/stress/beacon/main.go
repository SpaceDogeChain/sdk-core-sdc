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

// This file contains a miner stress test for the sdc1/2 transition
package main

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/sdcereum/go-sdcereum/accounts/keystore"
	"github.com/sdcereum/go-sdcereum/common"
	"github.com/sdcereum/go-sdcereum/common/fdlimit"
	"github.com/sdcereum/go-sdcereum/consensus/sdcash"
	"github.com/sdcereum/go-sdcereum/core"
	"github.com/sdcereum/go-sdcereum/core/beacon"
	"github.com/sdcereum/go-sdcereum/core/types"
	"github.com/sdcereum/go-sdcereum/crypto"
	"github.com/sdcereum/go-sdcereum/sdc"
	sdccatalyst "github.com/sdcereum/go-sdcereum/sdc/catalyst"
	"github.com/sdcereum/go-sdcereum/sdc/downloader"
	"github.com/sdcereum/go-sdcereum/sdc/sdcconfig"
	"github.com/sdcereum/go-sdcereum/les"
	lescatalyst "github.com/sdcereum/go-sdcereum/les/catalyst"
	"github.com/sdcereum/go-sdcereum/log"
	"github.com/sdcereum/go-sdcereum/miner"
	"github.com/sdcereum/go-sdcereum/node"
	"github.com/sdcereum/go-sdcereum/p2p"
	"github.com/sdcereum/go-sdcereum/p2p/enode"
	"github.com/sdcereum/go-sdcereum/params"
)

type nodetype int

const (
	legacyMiningNode nodetype = iota
	legacyNormalNode
	sdc2MiningNode
	sdc2NormalNode
	sdc2LightClient
)

func (typ nodetype) String() string {
	switch typ {
	case legacyMiningNode:
		return "legacyMiningNode"
	case legacyNormalNode:
		return "legacyNormalNode"
	case sdc2MiningNode:
		return "sdc2MiningNode"
	case sdc2NormalNode:
		return "sdc2NormalNode"
	case sdc2LightClient:
		return "sdc2LightClient"
	default:
		return "undefined"
	}
}

var (
	// transitionDifficulty is the target total difficulty for transition
	transitionDifficulty = new(big.Int).Mul(big.NewInt(20), params.MinimumDifficulty)

	// blockInterval is the time interval for creating a new sdc2 block
	blockInterval    = time.Second * 3
	blockIntervalInt = 3

	// finalizationDist is the block distance for finalizing block
	finalizationDist = 10
)

type sdcNode struct {
	typ        nodetype
	stack      *node.Node
	enode      *enode.Node
	api        *sdccatalyst.ConsensusAPI
	sdcBackend *sdc.sdcereum
	lapi       *lescatalyst.ConsensusAPI
	lesBackend *les.Lightsdcereum
}

func newNode(typ nodetype, genesis *core.Genesis, enodes []*enode.Node) *sdcNode {
	var (
		err        error
		api        *sdccatalyst.ConsensusAPI
		lapi       *lescatalyst.ConsensusAPI
		stack      *node.Node
		sdcBackend *sdc.sdcereum
		lesBackend *les.Lightsdcereum
	)
	// Start the node and wait until it's up
	if typ == sdc2LightClient {
		stack, lesBackend, lapi, err = makeLightNode(genesis)
	} else {
		stack, sdcBackend, api, err = makeFullNode(genesis)
	}
	if err != nil {
		panic(err)
	}
	for stack.Server().NodeInfo().Ports.Listener == 0 {
		time.Sleep(250 * time.Millisecond)
	}
	// Connect the node to all the previous ones
	for _, n := range enodes {
		stack.Server().AddPeer(n)
	}
	enode := stack.Server().Self()

	// Inject the signer key and start sealing with it
	stack.AccountManager().AddBackend(keystore.NewPlaintextKeyStore("beacon-stress"))
	store := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	if _, err := store.NewAccount(""); err != nil {
		panic(err)
	}
	return &sdcNode{
		typ:        typ,
		api:        api,
		sdcBackend: sdcBackend,
		lapi:       lapi,
		lesBackend: lesBackend,
		stack:      stack,
		enode:      enode,
	}
}

func (n *sdcNode) assembleBlock(parentHash common.Hash, parentTimestamp uint64) (*beacon.ExecutableDataV1, error) {
	if n.typ != sdc2MiningNode {
		return nil, errors.New("invalid node type")
	}
	timestamp := uint64(time.Now().Unix())
	if timestamp <= parentTimestamp {
		timestamp = parentTimestamp + 1
	}
	payloadAttribute := beacon.PayloadAttributesV1{
		Timestamp:             timestamp,
		Random:                common.Hash{},
		SuggestedFeeRecipient: common.HexToAddress("0xdeadbeef"),
	}
	fcState := beacon.ForkchoiceStateV1{
		HeadBlockHash:      parentHash,
		SafeBlockHash:      common.Hash{},
		FinalizedBlockHash: common.Hash{},
	}
	payload, err := n.api.ForkchoiceUpdatedV1(fcState, &payloadAttribute)
	if err != nil {
		return nil, err
	}
	return n.api.GetPayloadV1(*payload.PayloadID)
}

func (n *sdcNode) insertBlock(eb beacon.ExecutableDataV1) error {
	if !sdc2types(n.typ) {
		return errors.New("invalid node type")
	}
	switch n.typ {
	case sdc2NormalNode, sdc2MiningNode:
		newResp, err := n.api.NewPayloadV1(eb)
		if err != nil {
			return err
		} else if newResp.Status != "VALID" {
			return errors.New("failed to insert block")
		}
		return nil
	case sdc2LightClient:
		newResp, err := n.lapi.ExecutePayloadV1(eb)
		if err != nil {
			return err
		} else if newResp.Status != "VALID" {
			return errors.New("failed to insert block")
		}
		return nil
	default:
		return errors.New("undefined node")
	}
}

func (n *sdcNode) insertBlockAndSsdcead(parent *types.Header, ed beacon.ExecutableDataV1) error {
	if !sdc2types(n.typ) {
		return errors.New("invalid node type")
	}
	if err := n.insertBlock(ed); err != nil {
		return err
	}
	block, err := beacon.ExecutableDataToBlock(ed)
	if err != nil {
		return err
	}
	fcState := beacon.ForkchoiceStateV1{
		HeadBlockHash:      block.ParentHash(),
		SafeBlockHash:      common.Hash{},
		FinalizedBlockHash: common.Hash{},
	}
	switch n.typ {
	case sdc2NormalNode, sdc2MiningNode:
		if _, err := n.api.ForkchoiceUpdatedV1(fcState, nil); err != nil {
			return err
		}
		return nil
	case sdc2LightClient:
		if _, err := n.lapi.ForkchoiceUpdatedV1(fcState, nil); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("undefined node")
	}
}

type nodeManager struct {
	genesis      *core.Genesis
	genesisBlock *types.Block
	nodes        []*sdcNode
	enodes       []*enode.Node
	close        chan struct{}
}

func newNodeManager(genesis *core.Genesis) *nodeManager {
	return &nodeManager{
		close:        make(chan struct{}),
		genesis:      genesis,
		genesisBlock: genesis.ToBlock(),
	}
}

func (mgr *nodeManager) createNode(typ nodetype) {
	node := newNode(typ, mgr.genesis, mgr.enodes)
	mgr.nodes = append(mgr.nodes, node)
	mgr.enodes = append(mgr.enodes, node.enode)
}

func (mgr *nodeManager) getNodes(typ nodetype) []*sdcNode {
	var ret []*sdcNode
	for _, node := range mgr.nodes {
		if node.typ == typ {
			ret = append(ret, node)
		}
	}
	return ret
}

func (mgr *nodeManager) startMining() {
	for _, node := range append(mgr.getNodes(sdc2MiningNode), mgr.getNodes(legacyMiningNode)...) {
		if err := node.sdcBackend.StartMining(1); err != nil {
			panic(err)
		}
	}
}

func (mgr *nodeManager) shutdown() {
	close(mgr.close)
	for _, node := range mgr.nodes {
		node.stack.Close()
	}
}

func (mgr *nodeManager) run() {
	if len(mgr.nodes) == 0 {
		return
	}
	chain := mgr.nodes[0].sdcBackend.BlockChain()
	sink := make(chan core.ChainHeadEvent, 1024)
	sub := chain.SubscribeChainHeadEvent(sink)
	defer sub.Unsubscribe()

	var (
		transitioned bool
		parentBlock  *types.Block
		waitFinalise []*types.Block
	)
	timer := time.NewTimer(0)
	defer timer.Stop()
	<-timer.C // discard the initial tick

	// Handle the by default transition.
	if transitionDifficulty.Sign() == 0 {
		transitioned = true
		parentBlock = mgr.genesisBlock
		timer.Reset(blockInterval)
		log.Info("Enable the transition by default")
	}

	// Handle the block finalization.
	checkFinalise := func() {
		if parentBlock == nil {
			return
		}
		if len(waitFinalise) == 0 {
			return
		}
		oldest := waitFinalise[0]
		if oldest.NumberU64() > parentBlock.NumberU64() {
			return
		}
		distance := parentBlock.NumberU64() - oldest.NumberU64()
		if int(distance) < finalizationDist {
			return
		}
		nodes := mgr.getNodes(sdc2MiningNode)
		nodes = append(nodes, mgr.getNodes(sdc2NormalNode)...)
		nodes = append(nodes, mgr.getNodes(sdc2LightClient)...)
		for _, node := range nodes {
			fcState := beacon.ForkchoiceStateV1{
				HeadBlockHash:      oldest.Hash(),
				SafeBlockHash:      common.Hash{},
				FinalizedBlockHash: oldest.Hash(),
			}
			// TODO(rjl493456442) finalization doesn't work properly, FIX IT
			_ = fcState
			_ = node
			//node.api.ForkchoiceUpdatedV1(fcState, nil)
		}
		log.Info("Finalised sdc2 block", "number", oldest.NumberU64(), "hash", oldest.Hash())
		waitFinalise = waitFinalise[1:]
	}

	for {
		checkFinalise()
		select {
		case <-mgr.close:
			return

		case ev := <-sink:
			if transitioned {
				continue
			}
			td := chain.GetTd(ev.Block.Hash(), ev.Block.NumberU64())
			if td.Cmp(transitionDifficulty) < 0 {
				continue
			}
			transitioned, parentBlock = true, ev.Block
			timer.Reset(blockInterval)
			log.Info("Transition difficulty reached", "td", td, "target", transitionDifficulty, "number", ev.Block.NumberU64(), "hash", ev.Block.Hash())

		case <-timer.C:
			producers := mgr.getNodes(sdc2MiningNode)
			if len(producers) == 0 {
				continue
			}
			hash, timestamp := parentBlock.Hash(), parentBlock.Time()
			if parentBlock.NumberU64() == 0 {
				timestamp = uint64(time.Now().Unix()) - uint64(blockIntervalInt)
			}
			ed, err := producers[0].assembleBlock(hash, timestamp)
			if err != nil {
				log.Error("Failed to assemble the block", "err", err)
				continue
			}
			block, _ := beacon.ExecutableDataToBlock(*ed)

			nodes := mgr.getNodes(sdc2MiningNode)
			nodes = append(nodes, mgr.getNodes(sdc2NormalNode)...)
			nodes = append(nodes, mgr.getNodes(sdc2LightClient)...)
			for _, node := range nodes {
				if err := node.insertBlockAndSsdcead(parentBlock.Header(), *ed); err != nil {
					log.Error("Failed to insert block", "type", node.typ, "err", err)
				}
			}
			log.Info("Create and insert sdc2 block", "number", ed.Number)
			parentBlock = block
			waitFinalise = append(waitFinalise, block)
			timer.Reset(blockInterval)
		}
	}
}

func main() {
	log.Root().Ssdcandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
	fdlimit.Raise(2048)

	// Generate a batch of accounts to seal and fund with
	faucets := make([]*ecdsa.PrivateKey, 16)
	for i := 0; i < len(faucets); i++ {
		faucets[i], _ = crypto.GenerateKey()
	}
	// Pre-generate the sdcash mining DAG so we don't race
	sdcash.MakeDataset(1, filepath.Join(os.Getenv("HOME"), ".sdcash"))

	// Create an sdcash network based off of the Ropsten config
	genesis := makeGenesis(faucets)
	manager := newNodeManager(genesis)
	defer manager.shutdown()

	manager.createNode(sdc2NormalNode)
	manager.createNode(sdc2MiningNode)
	manager.createNode(legacyMiningNode)
	manager.createNode(legacyNormalNode)
	manager.createNode(sdc2LightClient)

	// Iterate over all the nodes and start mining
	time.Sleep(3 * time.Second)
	if transitionDifficulty.Sign() != 0 {
		manager.startMining()
	}
	go manager.run()

	// Start injecting transactions from the faucets like crazy
	time.Sleep(3 * time.Second)
	nonces := make([]uint64, len(faucets))
	for {
		// Pick a random mining node
		nodes := manager.getNodes(sdc2MiningNode)

		index := rand.Intn(len(faucets))
		node := nodes[index%len(nodes)]

		// Create a self transaction and inject into the pool
		tx, err := types.SignTx(types.NewTransaction(nonces[index], crypto.PubkeyToAddress(faucets[index].PublicKey), new(big.Int), 21000, big.NewInt(100000000000+rand.Int63n(65536)), nil), types.HomesteadSigner{}, faucets[index])
		if err != nil {
			panic(err)
		}
		if err := node.sdcBackend.TxPool().AddLocal(tx); err != nil {
			panic(err)
		}
		nonces[index]++

		// Wait if we're too saturated
		if pend, _ := node.sdcBackend.TxPool().Stats(); pend > 2048 {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// makeGenesis creates a custom sdcash genesis block based on some pre-defined
// faucet accounts.
func makeGenesis(faucets []*ecdsa.PrivateKey) *core.Genesis {
	genesis := core.DefaultRopstenGenesisBlock()
	genesis.Difficulty = params.MinimumDifficulty
	genesis.GasLimit = 25000000

	genesis.BaseFee = big.NewInt(params.InitialBaseFee)
	genesis.Config = params.AllsdcashProtocolChanges
	genesis.Config.TerminalTotalDifficulty = transitionDifficulty

	genesis.Alloc = core.GenesisAlloc{}
	for _, faucet := range faucets {
		genesis.Alloc[crypto.PubkeyToAddress(faucet.PublicKey)] = core.GenesisAccount{
			Balance: new(big.Int).Exp(big.NewInt(2), big.NewInt(128), nil),
		}
	}
	return genesis
}

func makeFullNode(genesis *core.Genesis) (*node.Node, *sdc.sdcereum, *sdccatalyst.ConsensusAPI, error) {
	// Define the basic configurations for the sdcereum node
	datadir, _ := os.MkdirTemp("", "")

	config := &node.Config{
		Name:    "gsdc",
		Version: params.Version,
		DataDir: datadir,
		P2P: p2p.Config{
			ListenAddr:  "0.0.0.0:0",
			NoDiscovery: true,
			MaxPeers:    25,
		},
		UseLightweightKDF: true,
	}
	// Create the node and configure a full sdcereum node on it
	stack, err := node.New(config)
	if err != nil {
		return nil, nil, nil, err
	}
	econfig := &sdcconfig.Config{
		Genesis:         genesis,
		NetworkId:       genesis.Config.ChainID.Uint64(),
		SyncMode:        downloader.FullSync,
		DatabaseCache:   256,
		DatabaseHandles: 256,
		TxPool:          core.DefaultTxPoolConfig,
		GPO:             sdcconfig.Defaults.GPO,
		sdcash:          sdcconfig.Defaults.sdcash,
		Miner: miner.Config{
			GasFloor: genesis.GasLimit * 9 / 10,
			GasCeil:  genesis.GasLimit * 11 / 10,
			GasPrice: big.NewInt(1),
			Recommit: 10 * time.Second, // Disable the recommit
		},
		LightServ:        100,
		LightPeers:       10,
		LightNoSyncServe: true,
	}
	sdcBackend, err := sdc.New(stack, econfig)
	if err != nil {
		return nil, nil, nil, err
	}
	_, err = les.NewLesServer(stack, sdcBackend, econfig)
	if err != nil {
		log.Crit("Failed to create the LES server", "err", err)
	}
	err = stack.Start()
	return stack, sdcBackend, sdccatalyst.NewConsensusAPI(sdcBackend), err
}

func makeLightNode(genesis *core.Genesis) (*node.Node, *les.Lightsdcereum, *lescatalyst.ConsensusAPI, error) {
	// Define the basic configurations for the sdcereum node
	datadir, _ := os.MkdirTemp("", "")

	config := &node.Config{
		Name:    "gsdc",
		Version: params.Version,
		DataDir: datadir,
		P2P: p2p.Config{
			ListenAddr:  "0.0.0.0:0",
			NoDiscovery: true,
			MaxPeers:    25,
		},
		UseLightweightKDF: true,
	}
	// Create the node and configure a full sdcereum node on it
	stack, err := node.New(config)
	if err != nil {
		return nil, nil, nil, err
	}
	lesBackend, err := les.New(stack, &sdcconfig.Config{
		Genesis:         genesis,
		NetworkId:       genesis.Config.ChainID.Uint64(),
		SyncMode:        downloader.LightSync,
		DatabaseCache:   256,
		DatabaseHandles: 256,
		TxPool:          core.DefaultTxPoolConfig,
		GPO:             sdcconfig.Defaults.GPO,
		sdcash:          sdcconfig.Defaults.sdcash,
		LightPeers:      10,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	err = stack.Start()
	return stack, lesBackend, lescatalyst.NewConsensusAPI(lesBackend), err
}

func sdc2types(typ nodetype) bool {
	if typ == sdc2LightClient || typ == sdc2NormalNode || typ == sdc2MiningNode {
		return true
	}
	return false
}
