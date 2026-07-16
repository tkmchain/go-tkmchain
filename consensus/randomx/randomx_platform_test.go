//go:build cgo && randomx
// +build cgo,randomx

package randomx

import (
	"runtime"
	"testing"
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
