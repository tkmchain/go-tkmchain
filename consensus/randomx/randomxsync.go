package randomx

import (
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Peer represents a connected node that can serve headers and blocks.
type Peer interface {
	ID() string
	Head() uint64
	RequestHeaders(from uint64, count int) ([]*types.Header, error)
	RequestBodies(hashes []common.Hash) ([]*types.Body, error)
	RequestReceipts(hashes []common.Hash) ([][]*types.Receipt, error)
}

// Chain inserts downloaded blocks and receipts.
type Chain interface {
	InsertBlock(block *types.Block, receipts []*types.Receipt) error
}

// Downloader coordinates syncing headers, bodies, and receipts.
type Downloader struct {
	peers    []Peer
	chain    Chain
	cancelCh chan struct{}
	head     uint64
}

// NewDownloader creates a new RandomX downloader.
func NewDownloader(peers []Peer, chain Chain) *Downloader {
	return &Downloader{
		peers:    peers,
		chain:    chain,
		cancelCh: make(chan struct{}),
		head:     0,
	}
}

// Sync runs the full sync loop.
func (d *Downloader) Sync() error {
	log.Println("Starting RandomX full sync…")
	for {
		select {
		case <-d.cancelCh:
			return nil
		default:
			if err := d.syncStep(); err != nil {
				log.Printf("Sync step failed: %v", err)
				time.Sleep(time.Second)
			}
		}
	}
}

// syncStep fetches headers, bodies, and receipts from the best peer.
func (d *Downloader) syncStep() error {
	peer := d.bestPeer()
	if peer == nil {
		log.Println("No peers available")
		time.Sleep(time.Second)
		return nil
	}

	// Fetch headers
	headers, err := peer.RequestHeaders(d.head+1, 128)
	if err != nil || len(headers) == 0 {
		return err
	}
	log.Printf("Fetched %d headers from %s", len(headers), peer.ID())

	// Validate headers (PoW, linkage)
	valid := []*types.Header{}
	for _, h := range headers {
		if validatePoW(h) && validateLinkage(valid, h) {
			valid = append(valid, h)
		}
	}
	if len(valid) == 0 {
		return nil
	}

	// Fetch bodies
	hashes := make([]common.Hash, len(valid))
	for i, h := range valid {
		hashes[i] = h.Hash()
	}
	bodies, err := peer.RequestBodies(hashes)
	if err != nil {
		return err
	}

	// Fetch receipts
	receipts, err := peer.RequestReceipts(hashes)
	if err != nil {
		return err
	}

	if len(bodies) != len(valid) || len(receipts) != len(valid) {
		return fmt.Errorf("mismatched block data lengths: headers=%d bodies=%d receipts=%d", len(valid), len(bodies), len(receipts))
	}

	// Commit blocks
	for i := range valid {
		block := types.NewBlockWithHeader(valid[i]).WithBody(*bodies[i])
		if err := d.commitBlock(block, receipts[i]); err != nil {
			return err
		}
		d.head = block.NumberU64()
	}
	return nil
}

// bestPeer selects the peer with the highest head.
func (d *Downloader) bestPeer() Peer {
	var best Peer
	for _, p := range d.peers {
		if best == nil || p.Head() > best.Head() {
			best = p
		}
	}
	return best
}

// validatePoW checks RandomX difficulty.
func validatePoW(h *types.Header) bool {
	// TODO: implement RandomX difficulty check
	_ = h
	return true
}

// validateLinkage ensures headers form a chain.
func validateLinkage(chain []*types.Header, h *types.Header) bool {
	if len(chain) == 0 {
		return true
	}
	return chain[len(chain)-1].Hash() == h.ParentHash
}

// commitBlock inserts the block into the local chain.
func (d *Downloader) commitBlock(b *types.Block, receipts []*types.Receipt) error {
	if d.chain == nil {
		log.Printf("Committed block #%d", b.NumberU64())
		return nil
	}
	return d.chain.InsertBlock(b, receipts)
}

