package core

import (
	"testing"

	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func TestShield6DecoderAdmitsMaximumConfiguredFields(t *testing.T) {
	e := &ShieldedV6Transaction{Version: ShieldedV6BoundVersion, AssetID: 1, ValidUntil: 3600, Inputs: make([]ShieldedV6Input, 16), Outputs: make([]ShieldedV6Output, 16), RangeProof: make([]byte, shielded6.MaxRangeProofBundleSize), Recipients: &ShieldedV6RecipientProofs{Roots: make([]ShieldedV6RecipientRoots, 16), Proofs: make([][]byte, 16)}}
	for i := range e.Outputs {
		e.Outputs[i].EncryptedNote = make([]byte, ShieldedV6MaxCiphertext)
		e.Recipients.Proofs[i] = make([]byte, ShieldedV6MaxRecipientProof)
	}
	data, err := EncodeShieldedV6Transaction(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > ShieldedV6MaxTxSize {
		t.Fatalf("configured fields exceed consensus size: %d", len(data))
	}
	decoded, recognized, err := DecodeShieldedV6Transaction(data)
	if err != nil || !recognized || len(decoded.Outputs) != 16 || decoded.ValidUntil != 3600 {
		t.Fatal("bounded maximum could not decode", err)
	}
}

func TestShield6DecoderBoundsBeforeAllocating(t *testing.T) {
	for _, name := range []string{"proof-count", "root-count", "output-count", "proof-bytes", "registration-bytes"} {
		e := &ShieldedV6Transaction{Version: ShieldedV6BoundVersion, AssetID: 1, Outputs: []ShieldedV6Output{{EncryptedNote: []byte{1}}}, Recipients: &ShieldedV6RecipientProofs{Roots: []ShieldedV6RecipientRoots{{}}, Proofs: [][]byte{{1}}}}
		switch name {
		case "proof-count":
			e.Recipients.Proofs = make([][]byte, 4096)
		case "root-count":
			e.Recipients.Roots = make([]ShieldedV6RecipientRoots, 17)
		case "output-count":
			e.Outputs = make([]ShieldedV6Output, 17)
		case "proof-bytes":
			e.Recipients.Proofs[0] = make([]byte, ShieldedV6MaxRecipientProof+1)
		case "registration-bytes":
			e.Version = ShieldedV6BindingVersion
			e.Recipients = nil
			e.Outputs = nil
			e.Binding = &ShieldedV6BindingRegistration{Owner: shielded3.Digest{1}, Commitment: shielded3.Digest{2}, Proof: make([]byte, ShieldedV6MaxRecipientProof+1)}
		}
		data, err := EncodeShieldedV6Transaction(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, recognized, err := DecodeShieldedV6Transaction(data); !recognized || err == nil {
			t.Fatalf("decoder accepted unbounded %s", name)
		}
	}
}

func FuzzShield6EnvelopeDecoder(f *testing.F) {
	f.Add([]byte(ShieldedV6Magic))
	f.Add(append([]byte(ShieldedV6Magic), 0xc0))
	f.Fuzz(func(t *testing.T, data []byte) {
		e, recognized, err := DecodeShieldedV6Transaction(data)
		if recognized && err == nil && (e == nil || len(e.Inputs) > ShieldedV6MaxInputs || len(e.Outputs) > ShieldedV6MaxOutputs || len(data) > ShieldedV6MaxTxSize) {
			t.Fatal("unbounded decoded envelope")
		}
	})
}
