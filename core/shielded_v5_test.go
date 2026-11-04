package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShieldedV5EnvelopeNullifierRules(t *testing.T) {
	envelope := &ShieldedV5Transaction{Version: 5, InputCount: 5, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)}
	for i := uint64(0); i < envelope.InputCount; i++ {
		n := shielded3.Digest{}
		n[0] = i + 1
		if i == 0 {
			envelope.Nullifier = n
		} else if i < shielded3.InputSlots {
			envelope.AdditionalNullifiers[i-1] = n
		} else {
			envelope.AdditionalNullifiersV5[i-shielded3.InputSlots] = n
		}
	}
	encoded, err := EncodeShieldedV5Transaction(envelope)
	if err != nil {
		t.Fatalf("encode V5 envelope: %v", err)
	}
	decoded, ok, err := DecodeShieldedV5Transaction(encoded)
	if err != nil || !ok {
		t.Fatalf("decode V5 envelope: ok=%t err=%v", ok, err)
	}
	nulls, err := ShieldedV5Nullifiers(decoded)
	if err != nil || len(nulls) != 5 {
		t.Fatalf("five-input envelope rejected: nullifiers=%d err=%v", len(nulls), err)
	}

	decoded.AdditionalNullifiersV5[0] = decoded.Nullifier
	if _, err := ShieldedV5Nullifiers(decoded); err == nil {
		t.Fatal("duplicate active nullifier accepted")
	}
	decoded.AdditionalNullifiersV5[0] = shielded3.Digest{}
	decoded.AdditionalNullifiersV5[1][0] = 9
	if _, err := ShieldedV5Nullifiers(decoded); err == nil {
		t.Fatal("nonzero inactive nullifier accepted")
	}
}
