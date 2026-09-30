package simulated

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
)

// simulatedEngine executes contract tests without proof of work or monetary rewards.
// eth.NewSimulated confines it to an in-memory node without peers or public RPC.
type simulatedEngine struct{ consensus.Engine }

func (*simulatedEngine) CalcDifficulty(consensus.ChainHeaderReader, uint64, *types.Header) *big.Int {
	return big.NewInt(1)
}
func (*simulatedEngine) Finalize(_ consensus.ChainHeaderReader, header *types.Header, _ vm.StateDB, _ *types.Body) {
	setSimulatedMixDigest(header)
}
func (e *simulatedEngine) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, st *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	setSimulatedMixDigest(header)
	header.Root = st.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return types.NewBlock(header, body, receipts, trie.NewStackTrie(nil)), nil
}

// setSimulatedMixDigest supplies the proof field required by TKM's RandomX
// header validation. Simulated backends deliberately do not perform proof of
// work, so this is a deterministic fixture value rather than a mineable
// digest. Keeping it non-zero lets production verification reject empty
// digests without making contract-binding tests construct invalid blocks.
func setSimulatedMixDigest(header *types.Header) {
	if header == nil || header.Number == nil || header.Number.Sign() == 0 || header.MixDigest != (common.Hash{}) {
		return
	}
	header.MixDigest = crypto.Keccak256Hash(
		[]byte("tkm.simulated.randomx.mix"),
		header.ParentHash.Bytes(),
		header.Number.Bytes(),
	)
}
