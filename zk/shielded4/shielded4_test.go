package shielded4

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShield4RejectsMissingProof(t *testing.T) {
	statement := Statement{
		Statement: shielded3.Statement{
			ChainID:    8979,
			AssetID:    AssetTKM,
			Anchor:     Digest{1},
			Nullifier:  Digest{2},
			StampRoot:  Digest{3},
			InputCount: 1,
		},
		LinkTag: Digest{4},
	}
	if err := (NativeBackend{}).Verify(context.Background(), statement, nil); !errors.Is(err, ErrInvalidProof) {
		t.Fatalf("missing proof was not rejected: %v", err)
	}
}
