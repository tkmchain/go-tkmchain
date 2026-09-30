package verkle

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
)

type Peer struct {
	*p2p.Peer
	rw      p2p.MsgReadWriter
	version uint
	chain   *core.BlockChain
	mu      sync.Mutex
	pending map[uint64]pendingWitness
}

type witnessResult struct {
	witness *stateless.Witness
	err     error
}

type pendingWitness struct {
	blockHash common.Hash
	result    chan witnessResult
}

func NewPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) *Peer {
	return &Peer{Peer: p, rw: rw, version: version, pending: make(map[uint64]pendingWitness)}
}

func (p *Peer) Version() uint { return p.version }

// Handshake exchanges and validates the chain identity before witness data is
// accepted. Both peers send first, so two nodes cannot deadlock on startup.
func (p *Peer) Handshake(chain *core.BlockChain) error {
	if chain == nil || chain.Config() == nil || chain.Config().ChainID == nil || chain.Genesis() == nil {
		return errNetworkMismatch
	}
	p.chain = chain
	head := chain.CurrentHeader()
	status := &StatusPacket{Version: uint32(p.version), ChainID: new(big.Int).Set(chain.Config().ChainID), Genesis: chain.Genesis().Hash()}
	if head != nil {
		status.Latest, status.LatestHash, status.StateRoot = head.Number.Uint64(), head.Hash(), head.Root
	}
	if err := p2p.Send(p.rw, StatusMsg, status); err != nil {
		return err
	}
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	defer msg.Discard()
	if msg.Code != StatusMsg || msg.Size > maxMessageSize {
		return errInvalidMessage
	}
	var remote StatusPacket
	if err := msg.Decode(&remote); err != nil || remote.Version != uint32(p.version) || remote.ChainID == nil || remote.ChainID.Cmp(chain.Config().ChainID) != 0 || remote.Genesis != status.Genesis {
		return errNetworkMismatch
	}
	return nil
}

func (p *Peer) RequestWitness(id uint64, blockHash common.Hash, blockNumber uint64, maxBytes uint64) error {
	if maxBytes == 0 || maxBytes > maxWitnessSize {
		return fmt.Errorf("witness request size is outside the permitted range")
	}
	return p2p.Send(p.rw, GetWitnessMsg, &GetWitnessPacket{ID: id, BlockHash: blockHash, BlockNumber: blockNumber, MaxBytes: maxBytes})
}

// RequestWitnessAndWait registers a bounded request before sending it. The
// protocol read loop delivers the matching response, so callers never race a
// second goroutine against the peer's message handler.
func (p *Peer) RequestWitnessAndWait(ctx context.Context, id uint64, blockHash common.Hash, blockNumber uint64, maxBytes uint64) (*stateless.Witness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(chan witnessResult, 1)
	p.mu.Lock()
	if p.pending == nil {
		p.pending = make(map[uint64]pendingWitness)
	}
	if _, exists := p.pending[id]; exists {
		p.mu.Unlock()
		return nil, fmt.Errorf("duplicate witness request %d", id)
	}
	p.pending[id] = pendingWitness{blockHash: blockHash, result: result}
	p.mu.Unlock()
	if err := p.RequestWitness(id, blockHash, blockNumber, maxBytes); err != nil {
		p.mu.Lock()
		delete(p.pending, id)
		p.mu.Unlock()
		return nil, err
	}
	select {
	case response := <-result:
		return response.witness, response.err
	case <-ctx.Done():
		p.mu.Lock()
		delete(p.pending, id)
		p.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (p *Peer) deliverWitness(response WitnessPacket) {
	p.mu.Lock()
	pending, ok := p.pending[response.ID]
	if ok {
		delete(p.pending, response.ID)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	if response.Error != nil && len(response.Error) != 0 {
		pending.result <- witnessResult{err: fmt.Errorf("remote witness: %s", response.Error)}
		return
	}
	if response.BlockHash != pending.blockHash {
		pending.result <- witnessResult{err: errInvalidMessage}
		return
	}
	if len(response.Witness) == 0 || len(response.Witness) > maxWitnessSize {
		pending.result <- witnessResult{err: errInvalidMessage}
		return
	}
	var witness stateless.Witness
	if err := rlp.DecodeBytes(response.Witness, &witness); err != nil || len(witness.Headers) == 0 || witness.Headers[0].Root != response.StateRoot {
		pending.result <- witnessResult{err: errInvalidMessage}
		return
	}
	pending.result <- witnessResult{witness: &witness}
}

// ReadWitness validates a witness response against the requested block hash
// and the parent root carried by the witness before returning it to sync code.
func (p *Peer) ReadWitness(expectedID uint64, expectedHash common.Hash) (*stateless.Witness, error) {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return nil, err
	}
	defer msg.Discard()
	if msg.Code != WitnessMsg || msg.Size > maxMessageSize {
		return nil, errInvalidMessage
	}
	var response WitnessPacket
	if err := msg.Decode(&response); err != nil || response.ID != expectedID || response.BlockHash != expectedHash {
		return nil, errInvalidMessage
	}
	if len(response.Error) != 0 {
		return nil, fmt.Errorf("remote witness: %s", response.Error)
	}
	if len(response.Witness) == 0 || len(response.Witness) > maxWitnessSize {
		return nil, errInvalidMessage
	}
	var witness stateless.Witness
	if err := rlp.DecodeBytes(response.Witness, &witness); err != nil || len(witness.Headers) == 0 || witness.Headers[0].Root != response.StateRoot {
		return nil, errInvalidMessage
	}
	return &witness, nil
}

func (p *Peer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, pending := range p.pending {
		pending.result <- witnessResult{err: fmt.Errorf("verkle peer closed")}
		delete(p.pending, id)
	}
}
