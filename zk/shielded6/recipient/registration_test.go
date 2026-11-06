package recipient

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/test"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func registrationFixture(t *testing.T) (RegistrationStatement, RegistrationWitness) {
	t.Helper()
	if !shielded3.NativeAvailable() {
		t.Skip("native Tip5 reference")
	}
	keys, err := shielded6.DeriveWalletKeys(bytes.Repeat([]byte{0x57}, 32), 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	owner := shielded3.Digest{1, 2, 3, 4, 5}
	commitment, err := BindingCommitment(8979, owner, keys.Address())
	if err != nil {
		t.Fatal(err)
	}
	return RegistrationStatement{ChainID: 8979, Owner: owner, Commitment: commitment, Intent: sha256.Sum256([]byte("registration intent"))}, RegistrationWitness{SpendSecret: keys.SpendSecret, ViewSecret: keys.ViewSecret}
}

func TestRecipientRegistrationConstrainsKeyControl(t *testing.T) {
	s, w := registrationFixture(t)
	defer w.Clear()
	assignment, err := NewRegistrationAssignment(s, w)
	if err != nil {
		t.Fatal(err)
	}
	if err := test.IsSolved(&RegistrationCircuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"spend", "view", "owner", "chain", "commitment", "intent"} {
		bad, err := NewRegistrationAssignment(s, w)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case "spend":
			set32(&bad.SpendSecret, [32]byte{})
		case "view":
			bad.ViewSecret = bad.SpendSecret
		case "owner":
			bad.Owner = digestAssignment(shielded3.Digest{99})
		case "chain":
			bad.ChainID = s.ChainID + 1
		case "commitment":
			bad.Commitment = digestAssignment(shielded3.Digest{99})
		case "intent":
			set32(&bad.Intent, [32]byte{})
		}
		if err := test.IsSolved(&RegistrationCircuit{}, bad, ecc.BN254.ScalarField()); err == nil {
			t.Fatalf("accepted registration %s substitution", name)
		}
	}
}

func TestRecipientRegistrationRealProof(t *testing.T) {
	if testing.Short() {
		t.Skip("ephemeral registration setup and real proof")
	}
	s, w := registrationFixture(t)
	defer w.Clear()
	started := time.Now()
	cs, hash, err := CompileRegistration()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("registration circuit: %d constraints; SHA256 %x", cs.GetNbConstraints(), hash)
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ephemeral registration compile and setup: %s", time.Since(started))
	var encoded bytes.Buffer
	if _, err := vk.WriteTo(&encoded); err != nil {
		t.Fatal(err)
	}
	v, err := NewVerifier(encoded.Bytes(), sha256.Sum256(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	proveStart := time.Now()
	proof, err := ProveRegistration(cs, pk, s, w)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("registration proof: %s, %d bytes", time.Since(proveStart), len(proof))
	if err := v.VerifyRegistration(s, proof); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"owner", "commitment", "chain", "intent"} {
		bad := s
		switch name {
		case "owner":
			bad.Owner[0]++
		case "commitment":
			bad.Commitment[0]++
		case "chain":
			bad.ChainID++
		case "intent":
			bad.Intent[0] ^= 1
		}
		if err := v.VerifyRegistration(bad, proof); err == nil {
			t.Fatalf("accepted replayed registration %s", name)
		}
	}
}
