package txpool

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
)

type latestStateProviderTest struct {
	current *types.Header
	seen    []*types.Header
}

func (p *latestStateProviderTest) CurrentBlock() *types.Header { return p.current }

func (p *latestStateProviderTest) StateAt(header *types.Header) (*state.StateDB, error) {
	p.seen = append(p.seen, header)
	if header.Number.Cmp(big.NewInt(2)) != 0 {
		return nil, errors.New("stale state root")
	}
	return nil, nil
}

func TestStateAtWithLatestRetrySkipsStaleHead(t *testing.T) {
	requested := &types.Header{Number: big.NewInt(1)}
	latest := &types.Header{Number: big.NewInt(2)}
	provider := &latestStateProviderTest{current: latest}

	resolved, _, err := StateAtWithLatestRetry(provider, requested)
	if err != nil {
		t.Fatalf("state lookup failed: %v", err)
	}
	if resolved != latest {
		t.Fatalf("resolved head = %v, want latest head", resolved.Number)
	}
	if len(provider.seen) != 1 || provider.seen[0] != latest {
		t.Fatalf("lookups = %v, want one lookup at latest head", provider.seen)
	}
}
