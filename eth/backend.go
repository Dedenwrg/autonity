// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package eth implements the Ethereum protocol.
package eth

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/autonity/autonity/accounts"
	"github.com/autonity/autonity/accounts/abi/bind/backends"
	"github.com/autonity/autonity/common"
	"github.com/autonity/autonity/common/hexutil"
	"github.com/autonity/autonity/consensus"
	"github.com/autonity/autonity/consensus/tendermint/accountability"
	tendermintcore "github.com/autonity/autonity/consensus/tendermint/core"
	"github.com/autonity/autonity/consensus/tendermint/events"
	"github.com/autonity/autonity/core"
	"github.com/autonity/autonity/core/bloombits"
	"github.com/autonity/autonity/core/rawdb"
	"github.com/autonity/autonity/core/state/pruner"
	"github.com/autonity/autonity/core/types"
	"github.com/autonity/autonity/core/vm"
	"github.com/autonity/autonity/crypto"
	"github.com/autonity/autonity/eth/downloader"
	"github.com/autonity/autonity/eth/ethconfig"
	"github.com/autonity/autonity/eth/filters"
	"github.com/autonity/autonity/eth/gasprice"
	"github.com/autonity/autonity/eth/protocols/eth"
	"github.com/autonity/autonity/eth/protocols/snap"
	"github.com/autonity/autonity/ethdb"
	"github.com/autonity/autonity/event"
	"github.com/autonity/autonity/internal/ethapi"
	"github.com/autonity/autonity/internal/shutdowncheck"
	"github.com/autonity/autonity/log"
	"github.com/autonity/autonity/miner"
	"github.com/autonity/autonity/node"
	"github.com/autonity/autonity/p2p"
	"github.com/autonity/autonity/p2p/dnsdisc"
	"github.com/autonity/autonity/p2p/enode"
	"github.com/autonity/autonity/params"
	"github.com/autonity/autonity/rlp"
	"github.com/autonity/autonity/rpc"
)

const (
	maxFullMeshPeers = 20
)

// Config contains the configuration options of the ETH protocol.
// Deprecated: use ethconfig.Config instead.
type Config = ethconfig.Config

// Ethereum implements the Ethereum full node service.
type Ethereum struct {
	config *ethconfig.Config
	log    log.Logger
	// Handlers
	txPool             *core.TxPool
	blockchain         *core.BlockChain
	handler            *handler
	ethDialCandidates  enode.Iterator
	snapDialCandidates enode.Iterator

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests     chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer      *core.ChainIndexer             // Bloom indexer operating during block imports
	closeBloomHandler chan struct{}

	APIBackend *EthAPIBackend

	miner    *miner.Miner
	gasPrice *big.Int
	address  common.Address

	networkID     uint64
	netRPCService *ethapi.PublicNetAPI

	p2pServer        *p2p.Server
	topologySelector networkTopology

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and address)

	shutdownTracker *shutdowncheck.ShutdownTracker // Tracks if and when the node has shutdown ungracefully

	accountability *accountability.FaultDetector
}

// New creates a new Ethereum object (including the
// initialisation of the common Ethereum object)
func New(stack *node.Node, config *Config) (*Ethereum, error) {
	// Ensure configuration values are compatible and sane
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run eth.Ethereum in light sync mode, use les.LightEthereum")
	}

	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if config.Miner.GasPrice == nil || config.Miner.GasPrice.Cmp(common.Big0) <= 0 {
		stack.Logger().Warn("Sanitizing invalid miner gas price", "provided", config.Miner.GasPrice, "updated", ethconfig.Defaults.Miner.GasPrice)
		config.Miner.GasPrice = new(big.Int).Set(ethconfig.Defaults.Miner.GasPrice)
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
	stack.Logger().Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	// Transfer mining-related config to the ethash config.
	ethashConfig := config.Ethash
	ethashConfig.NotifyFull = config.Miner.NotifyFull

	// Assemble the Ethereum object
	chainDb, err := stack.OpenDatabaseWithFreezer("chaindata", config.DatabaseCache, config.DatabaseHandles, config.DatabaseFreezer, "eth/db/chaindata/", false)
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlockWithOverride(chainDb, config.Genesis, config.OverrideArrowGlacier, config.OverrideTerminalTotalDifficulty)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
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
	stack.Logger().Info("Initialised chain configuration", "config", chainConfig)

	if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb, stack.ResolvePath(config.TrieCleanCacheJournal)); err != nil {
		stack.Logger().Error("Failed to recover state", "error", err)
	}

	// The event mux is shared between the consensus engine and fault detector such that both of them will receive the
	// messages from p2p protocol manager layer.

	evMux := new(event.TypeMux)

	// single instance of msgStore shared by misbehaviour detector and omission fault detector.
	msgStore := tendermintcore.NewMsgStore()
	consensusEngine := ethconfig.CreateConsensusEngine(stack, chainConfig, config, config.Miner.Notify,
		config.Miner.Noverify, &vmConfig, evMux, msgStore)

	nodeKey, _ := stack.Config().AutonityKeys()
	eth := &Ethereum{
		config:            config,
		chainDb:           chainDb,
		log:               stack.Logger(),
		eventMux:          stack.EventMux(),
		accountManager:    stack.AccountManager(),
		engine:            consensusEngine,
		closeBloomHandler: make(chan struct{}),
		networkID:         config.NetworkID,
		gasPrice:          config.Miner.GasPrice,
		address:           crypto.PubkeyToAddress(nodeKey.PublicKey),
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      core.NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms),
		p2pServer:         stack.ExecutionServer(),
		topologySelector:  NewGraphTopology(maxFullMeshPeers),
		shutdownTracker:   shutdowncheck.NewShutdownTracker(chainDb),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	eth.log.Info("Initialising Autonity protocol", "network", config.NetworkID, "dbversion", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			if bcVersion != nil { // only print warning on upgrade, not on init
				eth.log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			}
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	senderCacher := core.NewTxSenderCacher()
	txSender := new(func(signedTx *types.Transaction) error)
	eth.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, chainConfig, eth.engine, vmConfig, eth.shouldPreserve,
		senderCacher, &config.TxLookupLimit, backends.NewInternalBackend(txSender), eth.log)

	if err != nil {
		return nil, err
	}

	// temporary solution
	if be, ok := consensusEngine.(interface {
		SetBlockchain(*core.BlockChain)
	}); ok {
		be.SetBlockchain(eth.blockchain)
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		eth.log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		eth.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	eth.bloomIndexer.Start(eth.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
	}
	eth.txPool = core.NewTxPool(config.TxPool, chainConfig, eth.blockchain, senderCacher)
	*txSender = eth.txPool.AddLocal
	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit + cacheConfig.SnapshotLimit
	checkpoint := config.Checkpoint
	if checkpoint == nil {
		checkpoint = params.TrustedCheckpoints[genesisHash]
	}
	if eth.handler, err = newHandler(&handlerConfig{
		Database:       chainDb,
		Chain:          eth.blockchain,
		TxPool:         eth.txPool,
		Network:        config.NetworkID,
		Sync:           config.SyncMode,
		BloomCache:     uint64(cacheLimit),
		EventMux:       eth.eventMux,
		Checkpoint:     checkpoint,
		RequiredBlocks: config.RequiredBlocks,
	}); err != nil {
		return nil, err
	}

	eth.miner = miner.New(eth, &config.Miner, chainConfig, eth.EventMux(), eth.engine, eth.isLocalBlock)
	eth.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	eth.APIBackend = &EthAPIBackend{stack.Config().ExtRPCEnabled(), stack.Config().AllowUnprotectedTxs, eth, nil}
	if eth.APIBackend.allowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.Miner.GasPrice
	}
	eth.APIBackend.gpo = gasprice.NewOracle(eth.APIBackend, gpoParams)

	// Once the chain is initialized, load accountability precompiled contracts in EVM environment before chain sync
	//start to apply accountability TXs if there were any, otherwise it would cause sync failure.
	accountability.LoadPrecompiles(eth.blockchain)
	// Create Fault Detector for each full node for the time being.
	//TODO: I think it would make more sense to move this into the tendermint backend if possible
	eth.accountability = accountability.NewFaultDetector(
		eth.blockchain,
		eth.address,
		evMux.Subscribe(events.MessageEvent{}, events.AccountabilityEvent{}, events.OldMessageEvent{}),
		msgStore, eth.txPool, eth.APIBackend, nodeKey,
		eth.blockchain.ProtocolContracts(),
		eth.log)

	// Setup DNS discovery iterators.
	dnsclient := dnsdisc.NewClient(dnsdisc.Config{})
	eth.ethDialCandidates, err = dnsclient.NewIterator(eth.config.EthDiscoveryURLs...)
	if err != nil {
		return nil, err
	}
	eth.snapDialCandidates, err = dnsclient.NewIterator(eth.config.SnapDiscoveryURLs...)
	if err != nil {
		return nil, err
	}

	// Start the RPC service
	eth.netRPCService = ethapi.NewPublicNetAPI(eth.p2pServer, config.NetworkID)

	// Register the backend on the node
	stack.RegisterAPIs(eth.APIs())
	stack.RegisterProtocols(eth.Protocols())
	stack.RegisterLifecycle(eth)

	// Successful startup; push a marker and check previous unclean shutdowns.
	eth.shutdownTracker.MarkStartup()

	return eth, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"autonity",
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

// APIs return the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Ethereum) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	if _, ok := s.engine.(consensus.BFT); ok {
		apis = append(apis, rpc.API{
			Namespace: "aut",
			Version:   params.Version,
			Service:   NewAutonityContractAPI(s.BlockChain(), s.BlockChain().ProtocolContracts()),
			Public:    true,
		})
	}

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.handler.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.APIBackend, false, 5*time.Minute),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Ethereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Ethereum) Etherbase() (eb common.Address, err error) {
	s.lock.RLock()
	etherbase := s.address
	s.lock.RUnlock()

	if etherbase != (common.Address{}) {
		return etherbase, nil
	}
	return common.Address{}, fmt.Errorf("address must be explicitly specified")
}

// isLocalBlock checks whether the specified block is mined
// by local miner accounts.
//
// We regard two types of accounts as local miner account: address
// and accounts specified via `txpool.locals` flag.
func (s *Ethereum) isLocalBlock(header *types.Header) bool {
	author, err := s.engine.Author(header)
	if err != nil {
		log.Warn("Failed to retrieve block author", "number", header.Number.Uint64(), "hash", header.Hash(), "err", err)
		return false
	}
	// Check whether the given address is address.
	s.lock.RLock()
	etherbase := s.address
	s.lock.RUnlock()
	if author == etherbase {
		return true
	}
	// Check whether the given address is specified by `txpool.local`
	// CLI flag.
	for _, account := range s.config.TxPool.Locals {
		if account == author {
			return true
		}
	}
	return false
}

// shouldPreserve checks whether we should preserve the given block
// during the chain reorg depending on whether the author of block
// is a local account.
func (s *Ethereum) shouldPreserve(header *types.Header) bool {
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
	return s.isLocalBlock(header)
}

// StartMining starts the miner with the given number of CPU threads. If mining
// is already running, this method adjust the number of threads allowed to use
// and updates the minimum price required by the transaction pool.
// NOTE: this method bypasses the out-of-sync mining prevention check.
// The node will start mining even if not sure on whether he is synced with the chain head
func (s *Ethereum) StartMining(threads int) error {
	// Update the thread count within the consensus engine
	type threaded interface {
		SetThreads(threads int)
	}
	if th, ok := s.engine.(threaded); ok {
		s.log.Info("Updated mining threads", "threads", threads)
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
		if _, err := s.Etherbase(); err != nil {
			s.log.Error("Cannot start mining without address", "err", err)
			return fmt.Errorf("address missing: %v", err)
		}

		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.handler.acceptTxs, 1)

		go s.miner.ForceStart()
	}
	return nil
}

// StopMining terminates the miner, both at the consensus engine level as well as
// at the block creation level.
func (s *Ethereum) StopMining() {
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

func (s *Ethereum) IsMining() bool      { return s.miner.Mining() }
func (s *Ethereum) Miner() *miner.Miner { return s.miner }

func (s *Ethereum) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *Ethereum) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *Ethereum) TxPool() *core.TxPool               { return s.txPool }
func (s *Ethereum) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Ethereum) Engine() consensus.Engine           { return s.engine }
func (s *Ethereum) FD() *accountability.FaultDetector  { return s.accountability }
func (s *Ethereum) ChainDb() ethdb.Database            { return s.chainDb }
func (s *Ethereum) IsListening() bool                  { return true } // Always listening
func (s *Ethereum) Downloader() *downloader.Downloader { return s.handler.downloader }
func (s *Ethereum) Synced() bool                       { return atomic.LoadUint32(&s.handler.acceptTxs) == 1 }
func (s *Ethereum) SetSynced()                         { atomic.StoreUint32(&s.handler.acceptTxs, 1) }
func (s *Ethereum) ArchiveMode() bool                  { return s.config.NoPruning }
func (s *Ethereum) BloomIndexer() *core.ChainIndexer   { return s.bloomIndexer }
func (s *Ethereum) SyncMode() downloader.SyncMode {
	mode, _ := s.handler.chainSync.modeAndLocalHead()
	return mode
}

// Protocols returns all the currently configured
// network protocols to start.
func (s *Ethereum) Protocols() []p2p.Protocol {
	protos := eth.MakeProtocols((*ethHandler)(s.handler), s.networkID, s.ethDialCandidates)
	if s.config.SnapshotCache > 0 {
		protos = append(protos, snap.MakeProtocols((*snapHandler)(s.handler), s.snapDialCandidates)...)
	}
	return protos
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *Ethereum) Start() error {
	go s.accountability.Start()

	go func() {
		header := s.blockchain.CurrentHeader()
		if header.Number.BitLen() == 0 && header.Time > uint64(time.Now().Unix()) {
			s.genesisCountdown()
		}
		s.validatorController()
	}()

	eth.StartENRUpdater(s.blockchain, s.p2pServer.LocalNode())
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

// This routine is responsible to communicate to devp2p who are the other consensus members
// if the local node is part of the consensus committee or not. It also control the miner start/stop functions.
// todo(youssef): listen to new epoch events instead
func (s *Ethereum) validatorController() {
	chainHeadCh := make(chan core.ChainHeadEvent)
	chainHeadSub := s.blockchain.SubscribeChainHeadEvent(chainHeadCh)

	updateConsensusEnodes := func(block *types.Block) {
		state, err := s.blockchain.StateAt(block.Header().Root)
		if err != nil {
			s.log.Error("Could not retrieve state at head block", "err", err)
			return
		}
		committee, err := s.blockchain.ProtocolContracts().CommitteeEnodes(block, state, false)
		if err != nil {
			s.log.Error("Could not retrieve consensus whitelist at head block", "err", err)
			return
		}

		index := s.topologySelector.MyIndex(committee.List, s.p2pServer.LocalNode())
		s.p2pServer.UpdateConsensusEnodes(s.topologySelector.RequestSubset(committee.List, index), committee.List)
	}
	wasValidating := false
	currentBlock := s.blockchain.CurrentBlock()
	if currentBlock.Header().CommitteeMember(s.address) != nil {
		updateConsensusEnodes(currentBlock)
		s.miner.Start()
		s.log.Info("Starting node as validator")
		wasValidating = true
	}

	for {
		select {
		case ev := <-chainHeadCh:
			// current block number is cached in server
			s.p2pServer.SetCurrentBlockNumber(ev.Block.NumberU64())
			header := ev.Block.Header()
			// check if the local node belongs to the consensus committee.
			if header.CommitteeMember(s.address) == nil {
				// if the local node was part of the committee set for the previous block
				// there is no longer the need to retain the full connections and the
				// consensus engine enabled.
				if wasValidating {
					s.log.Info("Local node no longer detected part of the consensus committee, mining stopped")
					s.miner.Stop()
					s.p2pServer.UpdateConsensusEnodes(nil, nil)
					wasValidating = false
				}
				continue
			}
			updateConsensusEnodes(ev.Block)
			// if we were not committee in the past block we need to enable the mining engine.
			if !wasValidating {
				s.log.Info("Local node detected part of the consensus committee, mining started")
				s.miner.Start()
			}
			wasValidating = true
		// Err() channel will be closed when unsubscribing.
		case <-chainHeadSub.Err():
			return
		}
	}
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *Ethereum) Stop() error {
	// Stop AFD first,
	s.accountability.Stop()
	s.engine.Close()
	// Stop all the peer-related stuff then.
	s.ethDialCandidates.Close()
	s.snapDialCandidates.Close()
	s.handler.Stop()
	// Then stop everything else.
	s.bloomIndexer.Close()
	close(s.closeBloomHandler)
	s.txPool.Stop()
	s.miner.Close()
	s.blockchain.Stop()

	// Clean shutdown marker as the last thing before closing db
	s.shutdownTracker.Stop()

	s.chainDb.Close()
	s.eventMux.Stop()

	return nil
}

func (s *Ethereum) genesisCountdown() {
	genesisTime := time.Unix(int64(s.blockchain.Genesis().Time()), 0)
	s.log.Info(fmt.Sprintf("Chain genesis time: %v", genesisTime))
	if s.blockchain.Genesis().Header().CommitteeMember(s.address) != nil {
		s.log.Warn("**************************************************************")
		s.log.Warn("Local node is detected GENESIS VALIDATOR")
		s.log.Warn("Please remain tuned to our Telegram channel for announcements")
		s.log.Warn("**************************************************************")
	}
	var (
		lastDays   = 0
		lastHours  = 0
		lastMinute = 0
		lastSecond = 0
	)
	for {
		now := time.Now()
		duration := genesisTime.Sub(now)

		if duration <= 0 {
			s.log.Warn("Launch!")
			go func() {
				time.Sleep(3 * time.Second)
				if s.blockchain.Genesis().Number().Cmp(common.Big0) > 0 {
					s.log.Warn("🚀🚀🚀 LAUNCH SUCCESS 🚀🚀🚀")
				}
			}()
			break
		}
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		minutes := int(duration.Minutes()) % 60
		seconds := int(duration.Seconds()) % 60

		switch {
		case days > 0:
			if days != lastDays {
				lastDays = days
				s.log.Info(fmt.Sprintf("%d day(s) remaining before genesis", days))
			}
		case (hours == 1 || hours == 2 || hours == 6 || hours == 12) && days == 0:
			if hours != lastHours {
				lastHours = hours
				s.log.Info(fmt.Sprintf("%d hour(s) remaining before genesis", hours))
			}
		case (minutes == 1 || minutes == 5 || minutes == 15 || minutes == 30 || minutes == 45) && hours == 0 && days == 0 && seconds == 0:
			if minutes != lastMinute {
				lastMinute = minutes
				s.log.Info(fmt.Sprintf("%d minute(s) remaining before genesis", minutes))
			}
		case (seconds < 10 || seconds == 30 || seconds == 45) && days == 0 && hours == 0 && minutes == 0:
			if seconds != lastSecond {
				lastSecond = seconds
				s.log.Info(fmt.Sprintf("%d second(s) before genesis", seconds))
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Ethereum) Logger() log.Logger {
	return s.log
}
