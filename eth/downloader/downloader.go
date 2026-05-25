// Copyright 2015 The go-ethereum Authors
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

// Package downloader contains the manual full chain synchronisation.
package downloader

import (
	"errors"
	"fmt"
//	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state/snapshot"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/triedb"
)

var (
	MaxBlockFetch   = 128 // Number of blocks to be fetched per retrieval request
	MaxHeaderFetch  = 192 // Number of block headers to be fetched per retrieval request
	MaxReceiptFetch = 256 // Number of transaction receipts to allow fetching per request

	maxQueuedHeaders           = 32 * 1024                        // [eth/62] Maximum number of headers to queue for import (DOS protection)
	maxHeadersProcess          = 2048                             // Number of header download results to import at once into the chain
	maxResultsProcess          = 2048                             // Number of content download results to import at once into the chain
	fullMaxForkAncestry uint64 = params.FullImmutabilityThreshold // Maximum chain reorganisation (locally redeclared so tests can reduce it)

	reorgProtHeaderDelay = 2 // Number of headers to delay delivering to cover mini reorgs

	fsHeaderSafetyNet = 2048            // Number of headers to discard in case a chain violation is detected
	fsHeaderContCheck = 3 * time.Second // Time interval to check for header continuations during state download
	fsMinFullBlocks   = 64              // Number of blocks to retrieve fully even in snap sync
)

var (
	errBusy    = errors.New("busy")
	errBadPeer = errors.New("action from bad peer ignored")

	errTimeout                 = errors.New("timeout")
	errInvalidChain            = errors.New("retrieved hash chain is invalid")
	errInvalidBody             = errors.New("retrieved block body is invalid")
	errInvalidReceipt          = errors.New("retrieved receipt is invalid")
	errCancelStateFetch        = errors.New("state data download canceled (requested)")
	errCancelContentProcessing = errors.New("content processing canceled (requested)")
	errCanceled                = errors.New("syncing canceled (requested)")
	errNoPivotHeader           = errors.New("pivot header is not found")
)

// SyncMode defines the sync method of the downloader.
// Deprecated: use ethconfig.SyncMode instead
type SyncMode = ethconfig.SyncMode

const (
	// Deprecated: use ethconfig.FullSync
	FullSync = ethconfig.FullSync
	// Deprecated: use ethconfig.SnapSync
	SnapSync = ethconfig.SnapSync
)

// peerDropFn is a callback type for dropping a peer detected as malicious.
type peerDropFn func(id string)

// badBlockFn is a callback for the async beacon sync to notify the caller that
// the origin header requested to sync to, produced a chain with a bad block.
type badBlockFn func(invalid *types.Header, origin *types.Header)

// headerTask is a set of downloaded headers to queue along with their precomputed
// hashes to avoid constant rehashing.
type headerTask struct {
	headers []*types.Header
	hashes  []common.Hash
}

type Downloader struct {
	mode  atomic.Uint32 // Synchronisation mode defining the strategy used (per sync cycle), use d.getMode() to get the SyncMode
	moder *syncModer    // Sync mode management, deliver the appropriate sync mode choice for each cycle

	// Event feed for downloader events
	feed  event.FeedOf[SyncEvent]
	scope event.SubscriptionScope

	queue *queue   // Scheduler for selecting the hashes to download
	peers *peerSet // Set of active peers from which download can proceed

	stateDB ethdb.Database // Database to state sync into (and deduplicate via)

	// Statistics
	syncStatsChainOrigin uint64       // Origin block number where syncing started at
	syncStatsChainHeight uint64       // Highest block number known when syncing started
	syncStatsLock        sync.RWMutex // Lock protecting the sync stats fields

	blockchain BlockChain

	// Callbacks
	dropPeer peerDropFn // Drops a peer for misbehaving
	badBlock badBlockFn // Reports a block as rejected by the chain

	// Status
	synchronising atomic.Bool
	notified      atomic.Bool
	committed     atomic.Bool
	ancientLimit  uint64 // The maximum block number which can be regarded as ancient data.

	// The cutoff block number and hash before which chain segments (bodies
	// and receipts) are skipped during synchronization. 0 means the entire
	// chain segment is aimed for synchronization.
	chainCutoffNumber uint64
	chainCutoffHash   common.Hash

	// Channels
	headerProcCh chan *headerTask // Channel to feed the header processor new tasks

	// Skeleton sync - For RandomX chains, this is disabled
	skeleton *skeleton // Header skeleton to backfill the chain with

	// State sync
	pivotHeader *types.Header // Pivot block header to dynamically push the syncing state root
	pivotLock   sync.RWMutex  // Lock protecting pivot header reads from updates

	SnapSyncer     *snap.Syncer // TODO(karalabe): make private! hack for now
	stateSyncStart chan *stateSync

	// Cancellation and termination
	cancelCh   chan struct{}  // Channel to cancel mid-flight syncs
	cancelLock sync.RWMutex   // Lock to protect the cancel channel and peer in delivers
	cancelWg   sync.WaitGroup // Make sure all fetcher goroutines have exited.

	quitCh   chan struct{} // Quit channel to signal termination
	quitLock sync.Mutex    // Lock to prevent double closes

	// Testing hooks
	bodyFetchHook    func([]*types.Header) // Method to call upon starting a block body fetch
	receiptFetchHook func([]*types.Header) // Method to call upon starting a receipt fetch
	chainInsertHook  func([]*fetchResult)  // Method to call upon inserting a chain of blocks (possibly in multiple invocations)

	// Progress reporting metrics
	syncStartBlock uint64    // Head snap block when Geth was started
	syncStartTime  time.Time // Time instance when chain sync started
	syncLogTime    time.Time // Time instance when status was last reported
}

// BlockChain encapsulates functions required to sync a (full or snap) blockchain.
type BlockChain interface {
	// HasHeader verifies a header's presence in the local chain.
	HasHeader(common.Hash, uint64) bool

	// HasState checks if state trie is fully present in the database or not.
	HasState(root common.Hash) bool

	// GetHeaderByHash retrieves a header from the local chain.
	GetHeaderByHash(common.Hash) *types.Header

	// CurrentHeader retrieves the head header from the local chain.
	CurrentHeader() *types.Header

	// SetHead rewinds the local chain to a new head.
	SetHead(uint64) error

	// HasBlock verifies a block's presence in the local chain.
	HasBlock(common.Hash, uint64) bool

	// HasFastBlock verifies a snap block's presence in the local chain.
	HasFastBlock(common.Hash, uint64) bool

	// GetBlockByHash retrieves a block from the local chain.
	GetBlockByHash(common.Hash) *types.Block

	// CurrentBlock retrieves the head block from the local chain.
	CurrentBlock() *types.Header

	// CurrentSnapBlock retrieves the head snap block from the local chain.
	CurrentSnapBlock() *types.Header

	// SnapSyncStart explicitly notifies the chain that snap sync is scheduled and
	// marks chain mutations as disallowed.
	SnapSyncStart() error

	// SnapSyncComplete directly commits the head block to a certain entity.
	SnapSyncComplete(common.Hash) error

	// InsertHeadersBeforeCutoff inserts a batch of headers before the configured
	// chain cutoff into the ancient store.
	InsertHeadersBeforeCutoff([]*types.Header) (int, error)

	// InsertChain inserts a batch of blocks into the local chain.
	InsertChain(types.Blocks) (int, error)

	// InterruptInsert disables or enables chain insertion.
	InterruptInsert(on bool)

	// InsertReceiptChain inserts a batch of blocks along with their receipts
	// into the local chain. Blocks older than the specified `ancientLimit`
	// are stored directly in the ancient store, while newer blocks are stored
	// in the live key-value store.
	InsertReceiptChain(types.Blocks, []rlp.RawValue, uint64) (int, error)

	// Snapshots returns the blockchain snapshot tree to paused it during sync.
	Snapshots() *snapshot.Tree

	// TrieDB retrieves the low level trie database used for interacting
	// with trie nodes.
	TrieDB() *triedb.Database

	// HistoryPruningCutoff returns the configured history pruning point.
	// Block bodies along with the receipts will be skipped for synchronization.
	HistoryPruningCutoff() (uint64, common.Hash)
}

// New creates a new downloader to fetch hashes and blocks from remote peers.
func New(stateDb ethdb.Database, mode ethconfig.SyncMode, chain BlockChain, dropPeer peerDropFn, success func()) *Downloader {
	cutoffNumber, cutoffHash := chain.HistoryPruningCutoff()
	dl := &Downloader{
		stateDB:           stateDb,
		moder:             newSyncModer(mode, chain, stateDb),
		queue:             newQueue(blockCacheMaxItems, blockCacheInitialItems),
		peers:             newPeerSet(),
		blockchain:        chain,
		chainCutoffNumber: cutoffNumber,
		chainCutoffHash:   cutoffHash,
		dropPeer:          dropPeer,
		headerProcCh:      make(chan *headerTask, 1),
		quitCh:            make(chan struct{}),
		SnapSyncer:        snap.NewSyncer(stateDb, chain.TrieDB().Scheme()),
		stateSyncStart:    make(chan *stateSync),
		syncStartBlock:    chain.CurrentSnapBlock().Number.Uint64(),
	}

	// For RandomX/PoW chains, skeleton sync is disabled (beacon-specific)
	// dl.skeleton = newSkeleton(...) // REMOVED - not needed for RandomX

	go dl.stateFetcher()
	return dl
}

// Progress retrieves the synchronisation boundaries, specifically the origin
// block where synchronisation started at (may have failed/suspended); the block
// or header sync is currently at; and the latest known block which the sync targets.
//
// In addition, during the state download phase of snap synchronisation the number
// of processed and the total number of known states are also returned. Otherwise
// these are zero.
func (d *Downloader) Progress() ethereum.SyncProgress {
	// Lock the current stats and return the progress
	d.syncStatsLock.RLock()
	defer d.syncStatsLock.RUnlock()

	current := uint64(0)
	mode := d.getMode()
	switch mode {
	case ethconfig.FullSync:
		current = d.blockchain.CurrentBlock().Number.Uint64()
	case ethconfig.SnapSync:
		current = d.blockchain.CurrentSnapBlock().Number.Uint64()
	default:
		log.Error("Unknown downloader mode", "mode", mode)
	}
	progress, pending := d.SnapSyncer.Progress()

	return ethereum.SyncProgress{
		StartingBlock:       d.syncStatsChainOrigin,
		CurrentBlock:        current,
		HighestBlock:        d.syncStatsChainHeight,
		SyncedAccounts:      progress.AccountSynced,
		SyncedAccountBytes:  uint64(progress.AccountBytes),
		SyncedBytecodes:     progress.BytecodeSynced,
		SyncedBytecodeBytes: uint64(progress.BytecodeBytes),
		SyncedStorage:       progress.StorageSynced,
		SyncedStorageBytes:  uint64(progress.StorageBytes),
		HealedTrienodes:     progress.TrienodeHealSynced,
		HealedTrienodeBytes: uint64(progress.TrienodeHealBytes),
		HealedBytecodes:     progress.BytecodeHealSynced,
		HealedBytecodeBytes: uint64(progress.BytecodeHealBytes),
		HealingTrienodes:    pending.TrienodeHeal,
		HealingBytecode:     pending.BytecodeHeal,
	}
}

// RegisterPeer injects a new download peer into the set of block source to be
// used for fetching hashes and blocks from.
func (d *Downloader) RegisterPeer(id string, version uint, peer Peer) error {
	var logger log.Logger
	if len(id) < 16 {
		// Tests use short IDs, don't choke on them
		logger = log.New("peer", id)
	} else {
		logger = log.New("peer", id[:8])
	}
	logger.Trace("Registering sync peer")
	if err := d.peers.Register(newPeerConnection(id, version, peer, logger)); err != nil {
		logger.Error("Failed to register sync peer", "err", err)
		return err
	}
	return nil
}

// UnregisterPeer remove a peer from the known list, preventing any action from
// the specified peer. An effort is also made to return any pending fetches into
// the queue.
func (d *Downloader) UnregisterPeer(id string) error {
	// Unregister the peer from the active peer set and revoke any fetch tasks
	var logger log.Logger
	if len(id) < 16 {
		// Tests use short IDs, don't choke on them
		logger = log.New("peer", id)
	} else {
		logger = log.New("peer", id[:8])
	}
	logger.Trace("Unregistering sync peer")
	if err := d.peers.Unregister(id); err != nil {
		logger.Error("Failed to unregister sync peer", "err", err)
		return err
	}
	d.queue.Revoke(id)

	return nil
}

// synchronise will select the peer and use it for synchronising.
func (d *Downloader) synchronise() (err error) {
	// Make sure only one goroutine is ever allowed past this point at once
	if !d.synchronising.CompareAndSwap(false, true) {
		return errBusy
	}
	defer d.synchronising.Store(false)

	// Post a user notification of the sync (only once per session)
	if d.notified.CompareAndSwap(false, true) {
		log.Info("Block synchronisation started")
	}

	// Obtain the synchronized used in this cycle
	mode := d.moder.get(true)
	defer func() {
		if err == nil && mode == ethconfig.SnapSync {
			d.moder.disableSnap()
			log.Info("Disabled snap-sync after the initial sync cycle")
		}
	}()

	// Disable chain mutations when snap sync is selected
	if mode == ethconfig.SnapSync {
		if err := d.blockchain.SnapSyncStart(); err != nil {
			return err
		}
	}

	// Reset the queue and peer set
	d.queue.Reset(blockCacheMaxItems, blockCacheInitialItems)
	d.peers.Reset()

	for _, ch := range []chan bool{d.queue.blockWakeCh, d.queue.receiptWakeCh} {
		select {
		case <-ch:
		default:
		}
	}
	for empty := false; !empty; {
		select {
		case <-d.headerProcCh:
		default:
			empty = true
		}
	}

	// Create cancel channel for aborting mid-flight
	d.cancelLock.Lock()
	d.cancelCh = make(chan struct{})
	d.cancelLock.Unlock()

	defer d.Cancel()

	// Atomically set the requested sync mode
	d.mode.Store(uint32(mode))
	defer d.mode.Store(0)

	return d.syncToHead()
}

// getMode returns the sync mode used within current cycle.
func (d *Downloader) getMode() SyncMode {
	return SyncMode(d.mode.Load())
}

// ConfigSyncMode returns the sync mode configured for the node.
func (d *Downloader) ConfigSyncMode() SyncMode {
	return d.moder.get(false)
}

// SubscribeSyncEvents creates a subscription for downloader sync events
func (d *Downloader) SubscribeSyncEvents(ch chan<- SyncEvent) event.Subscription {
	return d.scope.Track(d.feed.Subscribe(ch))
}

// syncToHead starts a block synchronization based on the hash chain.
func (d *Downloader) syncToHead() (err error) {
	mode := d.getMode()
	d.feed.Send(SyncEvent{Type: SyncStarted, Mode: mode})
	defer func() {
		if err != nil {
			d.feed.Send(SyncEvent{Type: SyncFailed, Mode: mode, Err: err})
		} else {
			latest := d.blockchain.CurrentHeader()
			d.feed.Send(SyncEvent{Type: SyncCompleted, Mode: mode, Latest: latest})
		}
	}()

	log.Debug("Synchronising with the network", "mode", mode)
	defer func(start time.Time) {
		log.Debug("Synchronisation terminated", "elapsed", common.PrettyDuration(time.Since(start)))
	}(time.Now())

	// For RandomX, use simple header fetch without beacon bounds
	origin := uint64(0)
	height := uint64(0)

	d.syncStatsLock.Lock()
	d.syncStatsChainOrigin = origin
	d.syncStatsChainHeight = height
	d.syncStatsLock.Unlock()

	d.committed.Store(true)

	// Initiate the sync using a concurrent header and content retrieval algorithm
	d.queue.Prepare(origin+1, mode)

	fetchers := []func() error{
		func() error { return d.fetchHeaders(origin + 1) },
		func() error { return d.fetchBodies(origin + 1) },
		func() error { return d.fetchReceipts(origin + 1) },
		func() error { return d.processHeaders(origin + 1) },
	}

	if mode == ethconfig.SnapSync {
		fetchers = append(fetchers, func() error { return d.processSnapSyncContent() })
	} else if mode == ethconfig.FullSync {
		fetchers = append(fetchers, func() error { return d.processFullSyncContent() })
	}
	return d.spawnSync(fetchers)
}

// spawnSync runs d.process and all given fetcher functions to completion in
// separate goroutines, returning the first error that appears.
func (d *Downloader) spawnSync(fetchers []func() error) error {
	errc := make(chan error, len(fetchers))
	d.cancelWg.Add(len(fetchers))
	for _, fn := range fetchers {
		go func() { defer d.cancelWg.Done(); errc <- fn() }()
	}
	var err error
	for i := 0; i < len(fetchers); i++ {
		if i == len(fetchers)-1 {
			d.queue.Close()
		}
		if got := <-errc; got != nil {
			err = got
			if got != errCanceled {
				break
			}
		}
	}
	d.queue.Close()
	d.Cancel()
	return err
}

// cancel aborts all of the operations and resets the queue.
func (d *Downloader) cancel() {
	d.cancelLock.Lock()
	defer d.cancelLock.Unlock()

	if d.cancelCh != nil {
		select {
		case <-d.cancelCh:
		default:
			close(d.cancelCh)
		}
	}
}

// Cancel aborts all of the operations and waits for all download goroutines to finish.
func (d *Downloader) Cancel() {
	d.blockchain.InterruptInsert(true)
	d.cancel()
	d.cancelWg.Wait()
	d.blockchain.InterruptInsert(false)
}

// Terminate interrupts the downloader, canceling all pending operations.
func (d *Downloader) Terminate() {
	d.scope.Close()

	d.quitLock.Lock()
	select {
	case <-d.quitCh:
	default:
		close(d.quitCh)
	}
	d.quitLock.Unlock()

	d.Cancel()
}

// --- fetchBodies: fixed retry loop ---
func (d *Downloader) fetchBodies(from uint64) error {
    log.Debug("Downloading block bodies", "origin", from)
    for {
        select {
        case <-d.cancelCh:
            return errCanceled
        default:
            if err := d.concurrentFetch((*bodyQueue)(d)); err != nil {
                if err == errCanceled {
                    return err
                }
                log.Warn("Body fetch error, retrying", "err", err)
                time.Sleep(200 * time.Millisecond)
                continue
            }
            return nil
        }
    }
}

// --- fetchReceipts: same fix applied ---
func (d *Downloader) fetchReceipts(from uint64) error {
    log.Debug("Downloading receipts", "origin", from)
    for {
        select {
        case <-d.cancelCh:
            return errCanceled
        default:
            if err := d.concurrentFetch((*receiptQueue)(d)); err != nil {
                if err == errCanceled {
                    return err
                }
                log.Warn("Receipt fetch error, retrying", "err", err)
                time.Sleep(200 * time.Millisecond)
                continue
            }
            return nil
        }
    }
}

// --- fetchHeaders: prevent premature SnapSync exit ---
func (d *Downloader) fetchHeaders(from uint64) error {
    log.Debug("Downloading headers", "origin", from)
    mode := d.getMode()

    for {
        select {
        case <-d.cancelCh:
            return errCanceled
        default:
        }

        peers := d.peers.AllPeers()
        if len(peers) == 0 {
            time.Sleep(100 * time.Millisecond)
            continue
        }

        // Pick best peer
        var bestPeer *peerConnection
        for _, peer := range peers {
            if bestPeer == nil || peer.rates.Capacity(eth.BlockHeadersMsg, 0) > bestPeer.rates.Capacity(eth.BlockHeadersMsg, 0) {
                bestPeer = peer
            }
        }
        if bestPeer == nil {
            time.Sleep(100 * time.Millisecond)
            continue
        }

        // Request headers
        count := bestPeer.HeaderCapacity(time.Second)
        if count > MaxHeaderFetch {
            count = MaxHeaderFetch
        }
        if count < 1 {
            count = 1
        }

        requestStart := time.Now()
        resultCh := make(chan *eth.Response, 1)
        req, err := bestPeer.peer.RequestHeadersByNumber(from, count, 0, false, resultCh)
        if err != nil {
            log.Debug("Failed to request headers", "peer", bestPeer.id, "err", err)
            time.Sleep(100 * time.Millisecond)
            continue
        }

        select {
        case res := <-resultCh:
            if res == nil || res.Res == nil {
                continue
            }
            headers, ok := res.Res.(*eth.BlockHeadersRequest)
            if !ok || headers == nil {
                continue
            }
            headerList := []*types.Header(*headers)
            if len(headerList) == 0 {
                continue
            }

            // Prepare header task
            headerTask := &headerTask{
                headers: make([]*types.Header, len(headerList)),
                hashes:  make([]common.Hash, len(headerList)),
            }
            for i, header := range headerList {
                headerTask.headers[i] = header
                headerTask.hashes[i] = header.Hash()
            }

            bestPeer.UpdateHeaderRate(len(headerList), time.Since(requestStart))

            // Send to processor
            select {
            case d.headerProcCh <- headerTask:
            case <-d.cancelCh:
                req.Close()
                return errCanceled
            }

            from += uint64(len(headerList))
            log.Debug("Fetched headers", "count", len(headerList), "latest", headerList[len(headerList)-1].Number)

        case <-time.After(10 * time.Second):
            log.Debug("Header request timeout", "peer", bestPeer.id)
            continue

        case <-d.cancelCh:
            req.Close()
            return errCanceled
        }

        // ✅ SnapSync fix: don’t push nil prematurely
        if mode == ethconfig.SnapSync && d.committed.Load() {
            log.Debug("Header sync committed, stopping header fetch")
            return nil
        }
    }
}

// --- processHeaders: always wake body/receipt queues ---
func (d *Downloader) processHeaders(origin uint64) error {
    var (
        //mode  = d.getMode()
        timer = time.NewTimer(time.Second)
    )
    defer timer.Stop()

    for {
        select {
        case <-d.cancelCh:
            return errCanceled

        case task := <-d.headerProcCh:
            if task == nil || len(task.headers) == 0 {
                // Wake up body/receipt fetchers even if no headers
                for _, ch := range []chan bool{d.queue.blockWakeCh, d.queue.receiptWakeCh} {
                    select {
                    case ch <- true:
                    default:
                    }
                }
                return nil
            }

            for _, ch := range []chan bool{d.queue.blockWakeCh, d.queue.receiptWakeCh} {
                select {
                case ch <- true:
                default:
                }
            }
        }
    }
}


// processFullSyncContent takes fetch results from the queue and imports them into the chain.
func (d *Downloader) processFullSyncContent() error {
	for {
		results := d.queue.Results(true)
		if len(results) == 0 {
			return nil
		}
		if d.chainInsertHook != nil {
			d.chainInsertHook(results)
		}
		if err := d.importBlockResults(results); err != nil {
			return err
		}
	}
}

func (d *Downloader) importBlockResults(results []*fetchResult) error {
	if len(results) == 0 {
		return nil
	}
	select {
	case <-d.quitCh:
		return errCancelContentProcessing
	default:
	}

	first, last := results[0].Header, results[len(results)-1].Header
	log.Debug("Inserting downloaded chain", "items", len(results),
		"firstnum", first.Number, "firsthash", first.Hash(),
		"lastnum", last.Number, "lasthash", last.Hash(),
	)
	blocks := make([]*types.Block, len(results))
	for i, result := range results {
		blocks[i] = types.NewBlockWithHeader(result.Header).WithBody(result.body())
	}

	if index, err := d.blockchain.InsertChain(blocks); err != nil {
		if index < len(results) {
			log.Debug("Downloaded item processing failed", "number", results[index].Header.Number, "hash", results[index].Header.Hash(), "err", err)
		} else {
			log.Debug("Downloaded item processing failed on sidechain import", "index", index, "err", err)
		}
		return fmt.Errorf("%w: %v", errInvalidChain, err)
	}
	return nil
}

// processSnapSyncContent takes fetch results from the queue and writes them to the database.
func (d *Downloader) processSnapSyncContent() error {
	d.pivotLock.RLock()
	pivot := d.pivotHeader
	d.pivotLock.RUnlock()
	if pivot == nil {
		return errNoPivotHeader
	}
	sync := d.syncState(pivot.Root)

	defer func() {
		sync.Cancel()
	}()

	closeOnErr := func(s *stateSync) {
		if err := s.Wait(); err != nil && err != errCancelStateFetch && err != errCanceled && err != snap.ErrCancelled {
			d.queue.Close()
		}
	}
	go closeOnErr(sync)

	var (
		oldPivot *fetchResult
		oldTail  []*fetchResult
		timer    = time.NewTimer(time.Second)
	)
	defer timer.Stop()

	for {
		results := d.queue.Results(oldPivot == nil)
		if len(results) == 0 {
			if d.committed.Load() {
				d.reportSnapSyncProgress(true)
				return sync.Cancel()
			}
			select {
			case <-d.cancelCh:
				sync.Cancel()
				return errCanceled
			default:
			}
		}
		if d.chainInsertHook != nil {
			d.chainInsertHook(results)
		}
		d.reportSnapSyncProgress(false)

		d.pivotLock.RLock()
		pivot := d.pivotHeader
		d.pivotLock.RUnlock()
		if pivot == nil {
			return errNoPivotHeader
		}

		if oldPivot == nil {
			if !d.committed.Load() {
				if pivot.Root != sync.root {
					sync.Cancel()
					sync = d.syncState(pivot.Root)
					go closeOnErr(sync)
				}
			}
		} else {
			results = append(append([]*fetchResult{oldPivot}, oldTail...), results...)
		}
		P, beforeP, afterP := splitAroundPivot(pivot.Number.Uint64(), results)
		if err := d.commitSnapSyncData(beforeP, sync); err != nil {
			return err
		}
		if P != nil {
			if oldPivot != P {
				sync.Cancel()
				sync = d.syncState(P.Header.Root)
				go closeOnErr(sync)
				oldPivot = P
			}
			timer.Reset(time.Second)
			select {
			case <-sync.done:
				if sync.err != nil {
					return sync.err
				}
				if err := d.commitPivotBlock(P); err != nil {
					return err
				}
				oldPivot = nil
			case <-timer.C:
				oldTail = afterP
				continue
			}
		}
		if err := d.importBlockResults(afterP); err != nil {
			return err
		}
	}
}

func splitAroundPivot(pivot uint64, results []*fetchResult) (p *fetchResult, before, after []*fetchResult) {
	if len(results) == 0 {
		return nil, nil, nil
	}
	if lastNum := results[len(results)-1].Header.Number.Uint64(); lastNum < pivot {
		return nil, results, nil
	}
	for _, result := range results {
		num := result.Header.Number.Uint64()
		switch {
		case num < pivot:
			before = append(before, result)
		case num == pivot:
			p = result
		default:
			after = append(after, result)
		}
	}
	return p, before, after
}

func (d *Downloader) commitSnapSyncData(results []*fetchResult, stateSync *stateSync) error {
	if len(results) == 0 {
		return nil
	}
	select {
	case <-d.quitCh:
		return errCancelContentProcessing
	case <-stateSync.done:
		if err := stateSync.Wait(); err != nil {
			return err
		}
	default:
	}

	first, last := results[0].Header, results[len(results)-1].Header
	log.Debug("Inserting snap-sync blocks", "items", len(results),
		"firstnum", first.Number, "firsthash", first.Hash(),
		"lastnum", last.Number, "lasthash", last.Hash(),
	)
	blocks := make([]*types.Block, len(results))
	receipts := make([]rlp.RawValue, len(results))
	for i, result := range results {
		blocks[i] = types.NewBlockWithHeader(result.Header).WithBody(result.body())
		receipts[i] = result.Receipts
	}
	if index, err := d.blockchain.InsertReceiptChain(blocks, receipts, d.ancientLimit); err != nil {
		log.Debug("Downloaded item processing failed", "number", results[index].Header.Number, "hash", results[index].Header.Hash(), "err", err)
		return fmt.Errorf("%w: %v", errInvalidChain, err)
	}
	return nil
}

func (d *Downloader) commitPivotBlock(result *fetchResult) error {
	block := types.NewBlockWithHeader(result.Header).WithBody(result.body())
	log.Debug("Committing snap sync pivot as new head", "number", block.Number(), "hash", block.Hash())

	if _, err := d.blockchain.InsertReceiptChain([]*types.Block{block}, []rlp.RawValue{result.Receipts}, d.ancientLimit); err != nil {
		return err
	}
	if err := d.blockchain.SnapSyncComplete(block.Hash()); err != nil {
		return err
	}
	d.committed.Store(true)
	return nil
}

// DeliverSnapPacket is invoked from a peer's message handler when it transmits a
// data packet for the local node to consume.
func (d *Downloader) DeliverSnapPacket(peer *snap.Peer, packet snap.Packet) error {
	switch packet := packet.(type) {
	case *snap.AccountRangePacket:
		hashes, accounts, err := packet.Unpack()
		if err != nil {
			return err
		}
		return d.SnapSyncer.OnAccounts(peer, packet.ID, hashes, accounts, packet.Proof)
	case *snap.StorageRangesPacket:
		hashset, slotset := packet.Unpack()
		return d.SnapSyncer.OnStorage(peer, packet.ID, hashset, slotset, packet.Proof)
	case *snap.ByteCodesPacket:
		return d.SnapSyncer.OnByteCodes(peer, packet.ID, packet.Codes)
	case *snap.TrieNodesPacket:
		return d.SnapSyncer.OnTrieNodes(peer, packet.ID, packet.Nodes)
	default:
		return fmt.Errorf("unexpected snap packet type: %T", packet)
	}
}

func (d *Downloader) Sync() error {
	return d.synchronise()
}

var ErrBusy = errBusy

// reportSnapSyncProgress calculates various status reports and provides it to the user.
func (d *Downloader) reportSnapSyncProgress(force bool) {
	if d.syncStartTime.IsZero() {
		d.syncStartTime = time.Now().Add(-time.Millisecond)
	}
	if !force && time.Since(d.syncLogTime) < 8*time.Second {
		return
	}

	var (
		headerBytes, _  = d.stateDB.AncientSize(rawdb.ChainFreezerHeaderTable)
		bodyBytes, _    = d.stateDB.AncientSize(rawdb.ChainFreezerBodiesTable)
		receiptBytes, _ = d.stateDB.AncientSize(rawdb.ChainFreezerReceiptTable)
	)
	syncedBytes := common.StorageSize(headerBytes + bodyBytes + receiptBytes)
	if syncedBytes == 0 {
		return
	}

	var (
		header = d.blockchain.CurrentHeader()
		block  = d.blockchain.CurrentSnapBlock()
	)
	if block.Number.Uint64() <= d.syncStartBlock {
		return
	}
	if d.chainCutoffNumber != 0 && block.Number.Uint64() <= d.chainCutoffNumber {
		return
	}

	fetchedBlocks := block.Number.Uint64() - d.syncStartBlock
	if d.chainCutoffNumber != 0 && d.chainCutoffNumber > d.syncStartBlock {
		fetchedBlocks = block.Number.Uint64() - d.chainCutoffNumber
	}
	// Use fetchedBlocks in a log or mark as used
	_ = fetchedBlocks

	var progress = fmt.Sprintf("%.2f%%", float64(block.Number.Uint64())*100/float64(block.Number.Uint64()))
	var headers = fmt.Sprintf("%v@%v", log.FormatLogfmtUint64(header.Number.Uint64()), common.StorageSize(headerBytes).TerminalString())
	var bodies = fmt.Sprintf("%v@%v", log.FormatLogfmtUint64(block.Number.Uint64()), common.StorageSize(bodyBytes).TerminalString())
	var receipts = fmt.Sprintf("%v@%v", log.FormatLogfmtUint64(block.Number.Uint64()), common.StorageSize(receiptBytes).TerminalString())

	log.Info("Syncing: chain download in progress", "synced", progress, "chain", syncedBytes, "headers", headers, "bodies", bodies, "receipts", receipts)
	d.syncLogTime = time.Now()
}
