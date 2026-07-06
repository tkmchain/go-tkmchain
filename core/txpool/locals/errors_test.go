package locals

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
)

func TestIsTemporaryRejectDoesNotHideUnderpriced(t *testing.T) {
	if IsTemporaryReject(txpool.ErrUnderpriced) {
		t.Fatalf("underpriced transaction rejection should be returned to caller")
	}
	if !IsTemporaryReject(legacypool.ErrTxPoolOverflow) {
		t.Fatalf("txpool overflow should remain retryable")
	}
}
