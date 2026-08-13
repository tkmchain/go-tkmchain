//go:build cgo && randomx
// +build cgo,randomx

package randomx

import (
	"math/big"
	"runtime"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestRandomXFlagCandidatesFallback(t *testing.T) {
	candidates := randomXFlagCandidates(0)
	if len(candidates) == 0 {
		t.Fatal("no RandomX flag candidates returned")
	}
	if candidates[len(candidates)-1] != 0 {
		t.Fatalf("last candidate = %d, want soft interpreter fallback 0", candidates[len(candidates)-1])
	}

	hasBase := false
	for _, flags := range candidates {
		if flags == randomXBaseFlags() {
			hasBase = true
			break
		}
	}
	if !hasBase {
		t.Fatalf("candidates %v do not include HARD_AES fallback %d", candidates, randomXBaseFlags())
	}

	fast := randomXFastFlags()
	if runtime.GOOS == "darwin" || runtime.GOARCH == "arm64" {
		if fast&RANDOMX_FLAG_JIT != 0 {
			t.Fatalf("fast flags enable JIT on %s/%s: %d", runtime.GOOS, runtime.GOARCH, fast)
		}
	} else if fast&RANDOMX_FLAG_JIT == 0 {
		t.Fatalf("fast flags disable JIT on %s/%s: %d", runtime.GOOS, runtime.GOARCH, fast)
	}
}

func TestMeetsMoneroDifficultyUsesLittleEndianHash(t *testing.T) {
	var low common.Hash
	low[0] = 1
	if !meetsMoneroDifficulty(low, big.NewInt(2)) {
		t.Fatal("little-endian hash value 1 should meet difficulty 2")
	}

	var boundary common.Hash
	boundary[31] = 1
	if meetsMoneroDifficulty(boundary, big.NewInt(256)) {
		t.Fatal("hash*difficulty equal to 2^256 must be rejected")
	}
	if !meetsMoneroDifficulty(boundary, big.NewInt(255)) {
		t.Fatal("hash below the multiplication boundary should be accepted")
	}
}

func TestVerifyMoneroProofBindsDigestAndAllowsZeroNonce(t *testing.T) {
	config := *params.RandomXChainConfig
	config.RandomXTxBlock = big.NewInt(0)
	config.RandomXMoneroBlock = big.NewInt(1)
	chain := verifySealTestChain{config: &config}

	rx, err := New(DefaultConfig(), 1, config.MainKingAddress, nil)
	if err != nil {
		t.Fatalf("new RandomX failed: %v", err)
	}
	defer rx.Close()

	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1)}
	vm, err := rx.getVM()
	if err != nil {
		t.Fatalf("get VM failed: %v", err)
	}
	_, header.MixDigest = rx.randomXHash(header, vm)
	vm.Close()

	if err := rx.VerifySeal(chain, header); err != nil {
		t.Fatalf("valid zero-nonce Monero proof rejected: %v", err)
	}
	mutated := types.CopyHeader(header)
	mutated.MixDigest[0] ^= 1
	if err := rx.VerifySeal(chain, mutated); err == nil || !strings.Contains(err.Error(), "mix digest mismatch") {
		t.Fatalf("unbound mix digest was not rejected: %v", err)
	}
}

func TestHistoricalRepairTailIsExactHashBound(t *testing.T) {
	want := map[uint64]common.Hash{
		20369: common.HexToHash("0xbe28029213fd28ad8393a3f2ee76982441d2e4de09b0b3d3b5e77cdfff4533e2"),
		20370: common.HexToHash("0xc53450d9108bbedf82a74dfa063277f0072b8320bb35fe7dedd34624d94d4434"),
		20371: common.HexToHash("0x582809dbdc5f1ea71c5565497e925e7af2fead9f19f4c81b795e11c82852f539"),
		20372: common.HexToHash("0x44c6076c7abbec0cf22742856df7ab9703b1649de99ce72b1701256b156d91bd"),
		20373: common.HexToHash("0x6386f2bea3d034883ce29af5777408caeb3ecc8e055f25af79c8098d8de3fcea"),
	}
	for number, hash := range want {
		if got := postPrivacyGapStoredMixDigestCompatCheckpoints[number]; got != hash {
			t.Fatalf("repair checkpoint %d = %s, want %s", number, got, hash)
		}
	}
}
