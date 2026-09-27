//go:build !cgo || !randomx

package randomx

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func TestFallbackRejectsEmptyMixDigest(t *testing.T) {
	rx := NewFaker()
	header := &types.Header{
		Number:     new(big.Int).SetUint64(44425),
		Difficulty: new(big.Int).Set(GenesisDifficulty),
		Nonce:      types.EncodeNonce(1),
	}
	for _, name := range []string{"mainnet", "testnet", "egypt"} {
		t.Run(name, func(t *testing.T) {
			if err := rx.VerifyHeader(nil, header); err == nil || !strings.Contains(err.Error(), "empty mix digest") {
				t.Fatalf("zero mix digest accepted on %s: %v", name, err)
			}
		})
	}
}
