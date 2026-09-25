package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded4"
)

func TestShieldedV4EnvelopeRoundTrip(t *testing.T) {
	e := &ShieldedV4Transaction{
		Version:         4,
		Deposit:         false,
		Anchor:          shielded4.Digest{1},
		Nullifier:       shielded4.Digest{2},
		StampRoot:       shielded4.Digest{3},
		LinkTag:         shielded4.Digest{4},
		WithdrawalValue: new(big.Int),
		GasSponsorValue: new(big.Int),
		InputCount:      1,
	}
	data, err := EncodeShieldedV4Transaction(e)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok, err := DecodeShieldedV4Transaction(data)
	if err != nil || !ok || decoded.Version != 4 || decoded.LinkTag != e.LinkTag {
		t.Fatalf("Shield4 envelope round trip failed: ok=%v err=%v", ok, err)
	}
	if HasShieldedV3Prefix(data) {
		t.Fatal("Shield4 envelope was accepted as Shield3")
	}
}

func TestShieldedV4NullifiersAreDistinct(t *testing.T) {
	e := &ShieldedV4Transaction{Nullifier: shielded4.Digest{1}, AdditionalNullifiers: [3]shielded4.Digest{{2}, {3}, {4}}, InputCount: 4, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)}
	nullifiers, err := ShieldedV4Nullifiers(e)
	if err != nil || len(nullifiers) != 4 {
		t.Fatalf("valid Shield4 nullifiers rejected: %v", err)
	}
	e.AdditionalNullifiers[1] = e.Nullifier
	if _, err := ShieldedV4Nullifiers(e); !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("duplicate Shield4 nullifier accepted: %v", err)
	}
}

func TestShieldedV4BeforeAntarticalRejected(t *testing.T) {
	_, err := shieldedV4Basics(params.MainnetChainConfig, new(big.Int), 0, nil)
	if !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("pre-Antartical Shield4 transaction was not rejected: %v", err)
	}
	tx := types.NewTx(&types.LegacyTx{To: &params.ShieldedPoolAddress, Data: []byte(ShieldedV4Magic)})
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, new(big.Int), 0, tx); !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("pre-Antartical Shield4 envelope was not rejected: %v", err)
	}
}

func TestShieldedV4ReplayNamespaceIsSeparate(t *testing.T) {
	key := shielded4.Digest{9}.Bytes()
	if ShieldedV4StateSlot("link-tag", key) == ShieldedV3StateSlot("nullifier", key) {
		t.Fatal("Shield4 link-tag state shares the Shield3 replay slot")
	}
}
