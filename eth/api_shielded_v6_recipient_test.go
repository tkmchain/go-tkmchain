package eth

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShield6RecipientRootRPCBoundsAndReorg(t *testing.T) {
	stamp, binding := shielded3.Digest{1}, shielded3.Digest{2}
	storage := map[common.Hash]common.Hash{
		core.ShieldedV3StateSlot("stamp/root", stamp.Bytes()):             common.Hash{1},
		core.ShieldedV3StateSlot("shield6-binding/root", binding.Bytes()): common.Hash{2},
	}
	_, node := newTestKingAPI(t, types.GenesisAlloc{params.ShieldedPoolAddress: {Storage: storage}})
	api := NewPrivacyAPI(node)
	chainID := node.blockchain.Config().ChainID.Uint64()
	roots := []core.ShieldedV6RecipientRoots{{StampRoot: stamp, BindingRoot: binding, Anchor: core.ShieldedV6RecipientAnchor(chainID, stamp, binding)}}
	known, err := api.ShieldedV6RecipientRootsKnown(roots)
	if err != nil || !known {
		t.Fatal("canonical recipient roots unavailable", err)
	}
	roots[0].Anchor[0] ^= 1
	if known, err := api.ShieldedV6RecipientRootsKnown(roots); err != nil || known {
		t.Fatal("arbitrary anchor accepted", err)
	}
	roots[0].BindingRoot[0]++
	roots[0].Anchor = core.ShieldedV6RecipientAnchor(chainID, stamp, roots[0].BindingRoot)
	if known, err := api.ShieldedV6RecipientRootsKnown(roots); err != nil || known {
		t.Fatal("noncanonical/reorganized root accepted", err)
	}
	for _, invalid := range [][]core.ShieldedV6RecipientRoots{nil, make([]core.ShieldedV6RecipientRoots, 17)} {
		if _, err := api.ShieldedV6RecipientRootsKnown(invalid); err == nil {
			t.Fatal("invalid root query size accepted")
		}
	}
}
