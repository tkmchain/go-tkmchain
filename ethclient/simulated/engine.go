package simulated

import (
	"math/big"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/trie"
)

// simulatedEngine executes contract tests without proof of work or monetary rewards.
// eth.NewSimulated confines it to an in-memory node without peers or public RPC.
type simulatedEngine struct{ consensus.Engine }

func (*simulatedEngine) CalcDifficulty(consensus.ChainHeaderReader, uint64, *types.Header) *big.Int {
	return big.NewInt(1)
}
func (*simulatedEngine) Finalize(consensus.ChainHeaderReader, *types.Header, vm.StateDB, *types.Body) {
}
func (e *simulatedEngine) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, st *state.StateDB, body *types.Body, receipts []*types.Receipt) (*types.Block, error) {
	header.Root = st.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return types.NewBlock(header, body, receipts, trie.NewStackTrie(nil)), nil
}
