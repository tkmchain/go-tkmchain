package randomx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type anchorTestChain struct {
	config  *params.ChainConfig
	headers map[common.Hash]*types.Header
}

func (c *anchorTestChain) Config() *params.ChainConfig  { return c.config }
func (c *anchorTestChain) CurrentHeader() *types.Header { return nil }
func (c *anchorTestChain) GetHeader(hash common.Hash, _ uint64) *types.Header {
	return c.headers[hash]
}
func (c *anchorTestChain) GetHeaderByNumber(number uint64) *types.Header {
	for _, header := range c.headers {
		if header.Number != nil && header.Number.Uint64() == number {
			return header
		}
	}
	return nil
}
func (c *anchorTestChain) GetHeaderByHash(hash common.Hash) *types.Header { return c.headers[hash] }

func TestVerifyBlockHashAnchorIsConsensusRequired(t *testing.T) {
	parent := &types.Header{Number: new(big.Int), Time: 0, Extra: []byte("genesis")}
	chain := &anchorTestChain{config: params.EgyptChainConfig, headers: map[common.Hash]*types.Header{}}
	chain.headers[parent.Hash()] = parent

	child := &types.Header{Number: big.NewInt(1), Time: 1, ParentHash: parent.Hash()}
	rolling := antartical.BlockHashAnchorCommitment(common.Hash{}, 0, parent.Hash())
	child.Extra, _ = antartical.AttachBlockHashAnchor(child.Extra, antartical.BlockHashAnchor{Height: 0, Hash: parent.Hash(), Rolling: rolling})
	if err := verifyBlockHashAnchor(chain, child, nil); err != nil {
		t.Fatalf("valid Egypt anchor rejected: %v", err)
	}
	child.Extra[len(child.Extra)-1] ^= 1
	if err := verifyBlockHashAnchor(chain, child, nil); err == nil {
		t.Fatal("conflicting rolling commitment accepted")
	}
	child.Extra = nil
	if err := verifyBlockHashAnchor(chain, child, nil); err == nil {
		t.Fatal("missing Antartical anchor accepted")
	}
}
