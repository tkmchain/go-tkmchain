package eth

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/eth/protocols/verkle"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// verkleHandler serves execution witnesses from the canonical chain database.
// Witness construction is bounded by the remote request and uses the same
// state-recovery path as the debug RPC, so pruned nodes never send partial
// proofs.
type verkleHandler handler

func (h *verkleHandler) Chain() *core.BlockChain { return h.chain }

// verkleProtocolActive reports whether the canonical head has reached the
// Antartical activation boundary. P2P protocol capabilities are negotiated
// once when a connection is created, so a node that is still at genesis must
// not advertise Verkle yet: doing so makes the peer run the handler, which
// then rejects the connection before ordinary eth synchronization can catch
// the node up to the fork.
func verkleProtocolActive(chain *core.BlockChain) bool {
	if chain == nil || chain.Config() == nil {
		return false
	}
	head := chain.CurrentHeader()
	return head != nil && chain.Config().IsAntartical(head.Number, head.Time)
}

func (h *verkleHandler) RunPeer(peer *verkle.Peer, hand verkle.Handler) error {
	if h.chain == nil || h.chain.Config() == nil {
		return fmt.Errorf("verkle protocol unavailable without chain configuration")
	}
	if !verkleProtocolActive(h.chain) {
		return fmt.Errorf("verkle protocol is inactive before Antartical")
	}
	if err := peer.Handshake(h.chain); err != nil {
		return err
	}
	if !(*handler)(h).incHandlers() {
		return nil
	}
	defer (*handler)(h).decHandlers()
	return hand(peer)
}

func (h *verkleHandler) GetWitness(ctx context.Context, hash common.Hash, number uint64, maxBytes uint64) (*stateless.ExtWitness, error) {
	if maxBytes == 0 || maxBytes > 4*1024*1024 {
		return nil, fmt.Errorf("witness size limit is outside the permitted range")
	}
	if h.chain == nil || h.chain.Config() == nil {
		return nil, fmt.Errorf("verkle protocol unavailable without chain configuration")
	}
	if !verkleProtocolActive(h.chain) {
		return nil, fmt.Errorf("verkle protocol is inactive before Antartical")
	}
	block := h.chain.GetBlockByHash(hash)
	if block == nil || block.NumberU64() != number {
		return nil, fmt.Errorf("requested canonical block is unavailable")
	}
	witness, err := h.chain.BuildExecutionWitness(ctx, block, true)
	if err != nil {
		return nil, err
	}
	ext := witness.ToExtWitness()
	encoded, err := rlp.EncodeToBytes(ext)
	if err != nil {
		return nil, err
	}
	if uint64(len(encoded)) > maxBytes {
		return nil, fmt.Errorf("execution witness is %d bytes, request allows %d", len(encoded), maxBytes)
	}
	return ext, nil
}

func (h *verkleHandler) PeerInfo(id enode.ID) interface{} {
	return h.peers.peer(id.String()) != nil
}
