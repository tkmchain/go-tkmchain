package recipient

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	monerocrypto "git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/uints"
	"github.com/consensys/gnark/test"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func relationFixture(t *testing.T) (Statement, Witness) {
	t.Helper()
	if !shielded3.NativeAvailable() {
		t.Skip("requires native Tip5 reference")
	}
	keys, err := shielded6.DeriveWalletKeys(bytes.Repeat([]byte{0x45}, 32), 18979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	random := monerocrypto.NewDeterministicTestGenerator()
	amount := big.NewInt(7)
	_, _, openings, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, random)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clear(openings[0].Mask[:])
		clear(openings[0].LimbMasks[0][:])
		clear(openings[0].LimbMasks[1][:])
	}()
	output, secret, err := CreateOutput(keys.Address(), 3, amount, openings[0], random)
	if err != nil {
		t.Fatal(err)
	}
	// Demonstrate compatibility with the existing wallet, including HKDF's
	// first-accepted-candidate semantics and ciphertext authentication.
	note, owned, err := shielded6.ScanOutput(keys, 3, 0, output)
	if err != nil || !owned || note.Amount.Cmp(amount) != 0 {
		t.Fatalf("existing scanner rejected bound output: owned=%v err=%v", owned, err)
	}
	note.Clear()
	w := Witness{Owner: shielded3.Digest{1, 2, 3, 4, 5}, StampIndex: 0x12345678, BindingIndex: 0x87654321, Recipient: keys.Address(), EphemeralSecret: secret}
	s := Statement{ChainID: 18979, AssetID: shielded6.AssetTKM, OutputPosition: 3, StateAnchor: sha256.Sum256([]byte("state anchor")), Intent: sha256.Sum256([]byte("transaction intent")), Output: output}
	for i := 0; i < MerkleDepth; i++ {
		for j := 0; j < 5; j++ {
			w.StampPath[i][j] = uint64(i*5 + j + 11)
			w.BindingPath[i][j] = uint64(i*5 + j + 1011)
		}
	}
	stamp, err := shielded3.StampLeaf(s.ChainID, w.Owner)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindingLeaf(s.ChainID, w.Owner, w.Recipient)
	if err != nil {
		t.Fatal(err)
	}
	root := func(current shielded3.Digest, index uint32, path [MerkleDepth]shielded3.Digest) shielded3.Digest {
		for level, sibling := range path {
			if index>>level&1 == 0 {
				current, err = shielded3.HashPair(current, sibling)
			} else {
				current, err = shielded3.HashPair(sibling, current)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		return current
	}
	s.StampRoot = root(stamp, w.StampIndex, w.StampPath)
	s.BindingRoot = root(binding, w.BindingIndex, w.BindingPath)
	return s, w
}

func TestRecipientBindingRelation(t *testing.T) {
	s, w := relationFixture(t)
	defer w.Clear()
	assignment, err := NewAssignment(s, w)
	if err != nil {
		t.Fatal(err)
	}
	if err := test.IsSolved(&Circuit{}, assignment, ecc.BN254.ScalarField()); err != nil {
		t.Fatal(err)
	}
}

func TestRecipientBindingRejectsWitnessSubstitution(t *testing.T) {
	s, w := relationFixture(t)
	defer w.Clear()
	for _, name := range []string{"owner", "stamp-path", "binding-path", "stamp-index", "binding-index", "spend-key", "view-key", "spend-preimage", "view-preimage", "preimage-offcurve", "ephemeral-secret", "ephemeral-key", "one-time-key", "chain", "position", "anchor", "intent", "commitment", "limb", "ciphertext-hash", "stamp-root", "binding-root"} {
		t.Run(name, func(t *testing.T) {
			c, err := NewAssignment(s, w)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "owner":
				c.Owner[0] = emulated.ValueOf[gold](99)
			case "stamp-path":
				c.StampPath[0][0] = emulated.ValueOf[gold](99)
			case "binding-path":
				c.BindingPath[0][0] = emulated.ValueOf[gold](99)
			case "stamp-index":
				c.StampIndex = w.StampIndex ^ 1
			case "binding-index":
				c.BindingIndex = w.BindingIndex ^ 1
			case "spend-key":
				c.Spend = c.View
			case "view-key":
				c.View = c.Spend
			case "spend-preimage":
				c.SpendPreimage = c.ViewPreimage
			case "view-preimage":
				c.ViewPreimage = c.SpendPreimage
			case "preimage-offcurve":
				c.SpendPreimage = Point{X: emulated.ValueOf[edwardsBase](1), Y: emulated.ValueOf[edwardsBase](1)}
			case "ephemeral-secret":
				c.EphemeralSecret[0] = uints.NewU8(w.EphemeralSecret[0] ^ 1)
			case "ephemeral-key":
				c.Claim.EphemeralKey[0] = uints.NewU8(s.Output.EphemeralKey[0] ^ 1)
			case "one-time-key":
				c.Claim.OneTimeKey[0] = uints.NewU8(s.Output.OneTimeKey[0] ^ 1)
			case "chain":
				c.Claim.ChainID = s.ChainID + 1
			case "position":
				c.Claim.Position = s.OutputPosition + 1
			case "anchor":
				c.Claim.Anchor[0] = uints.NewU8(s.StateAnchor[0] ^ 1)
			case "intent":
				c.Claim.Intent[0] = uints.NewU8(s.Intent[0] ^ 1)
			case "commitment":
				c.Claim.Commitment[0] = uints.NewU8(s.Output.Commitment[0] ^ 1)
			case "limb":
				c.Claim.AmountLimbs[0][0] = uints.NewU8(s.Output.AmountLimbs[0][0] ^ 1)
			case "ciphertext-hash":
				c.Claim.CiphertextHash[0] = uints.NewU8(sha256.Sum256(s.Output.EncryptedNote)[0] ^ 1)
			case "stamp-root":
				c.Claim.StampRoot[0] = emulated.ValueOf[gold](99)
			case "binding-root":
				c.Claim.BindingRoot[0] = emulated.ValueOf[gold](99)
			}
			if err := test.IsSolved(&Circuit{}, c, ecc.BN254.ScalarField()); err == nil {
				t.Fatalf("accepted substituted %s", name)
			}
		})
	}
}

func TestRecipientBindingWitnessOnlyFieldsStayPrivate(t *testing.T) {
	s, w := relationFixture(t)
	defer w.Clear()
	c, err := NewAssignment(s, w)
	if err != nil {
		t.Fatal(err)
	}
	public, err := frontend.NewWitness(c, ecc.BN254.ScalarField(), frontend.PublicOnly())
	if err != nil {
		t.Fatal(err)
	}
	// A public witness must contain only the 32 digest bytes, no owner, key,
	// scalar, or membership-path coordinates.
	if got := len(public.Vector().(fr.Vector)); got != 32 {
		t.Fatalf("public witness has %d elements, want digest bytes only", got)
	}
}

func TestRecipientBindingRealProof(t *testing.T) {
	if testing.Short() {
		t.Skip("full cryptographic proof and ephemeral test setup")
	}
	s, w := relationFixture(t)
	defer w.Clear()
	started := time.Now()
	cs, circuitHash, err := Compile()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("compiled %d constraints in %s, R1CS SHA256 %x", cs.GetNbConstraints(), time.Since(started), circuitHash)
	// Validate the compiled constraints before the expensive test setup.
	// IsSolved uses a testing commitment placeholder, not a proof. The actual
	// proof below must still use the genuine Groth16 commitment implementation.
	assignment, err := NewAssignment(s, w)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.IsSolved(witness); err != nil {
		t.Fatalf("compiled positive witness: %v", err)
	}
	for _, name := range []string{"owner", "scalar", "preimage", "context"} {
		bad, err := NewAssignment(s, w)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case "owner":
			bad.Owner[0] = emulated.ValueOf[gold](99)
		case "scalar":
			set32(&bad.EphemeralSecret, [32]byte{})
		case "preimage":
			bad.ViewPreimage = bad.SpendPreimage
		case "context":
			bad.Claim.Anchor[0] = uints.NewU8(s.StateAnchor[0] ^ 1)
		}
		encoded, err := frontend.NewWitness(bad, ecc.BN254.ScalarField())
		if err != nil {
			t.Fatal(err)
		}
		if err := cs.IsSolved(encoded); err == nil {
			t.Fatalf("compiled circuit accepted altered %s", name)
		}
	}
	t.Log("compiled positive witness and four malicious witnesses checked; beginning ephemeral test setup")
	setupStart := time.Now()
	// TEST ONLY. The toxic waste from this local setup is not a production
	// ceremony; no keys generated here are saved or configured on any network.
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ephemeral test setup: %s", time.Since(setupStart))
	var encodedKey bytes.Buffer
	if _, err := vk.WriteTo(&encodedKey); err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(encodedKey.Bytes())
	verifier, err := NewVerifier(encodedKey.Bytes(), pin)
	if err != nil {
		t.Fatal(err)
	}
	proveStart := time.Now()
	// Optional local performance evidence; never contains a serialized witness.
	var cpuProfile *os.File
	if path := os.Getenv("SHIELD6_PROVER_CPU_PROFILE"); path != "" {
		cpuProfile, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.StartCPUProfile(cpuProfile); err != nil {
			cpuProfile.Close()
			t.Fatal(err)
		}
	}
	proof, err := Prove(cs, pk, s, w)
	if cpuProfile != nil {
		pprof.StopCPUProfile()
		cpuProfile.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("proved in %s, %d proof bytes", time.Since(proveStart), len(proof))
	verifyStart := time.Now()
	if err := verifier.Verify(s, proof); err != nil {
		t.Fatal(err)
	}
	t.Logf("verified in %s", time.Since(verifyStart))
	for _, name := range []string{"chain", "asset", "position", "stamp-root", "binding-root", "anchor", "intent", "one-time-key", "ephemeral-key", "commitment", "limb", "ciphertext"} {
		changed := s
		switch name {
		case "chain":
			changed.ChainID++
		case "asset":
			changed.AssetID++
		case "position":
			changed.OutputPosition++
		case "stamp-root":
			changed.StampRoot[0]++
		case "binding-root":
			changed.BindingRoot[0]++
		case "anchor":
			changed.StateAnchor[0] ^= 1
		case "intent":
			changed.Intent[0] ^= 1
		case "one-time-key":
			changed.Output.OneTimeKey[0] ^= 1
		case "ephemeral-key":
			changed.Output.EphemeralKey[0] ^= 1
		case "commitment":
			changed.Output.Commitment[0] ^= 1
		case "limb":
			changed.Output.AmountLimbs[0][0] ^= 1
		case "ciphertext":
			changed.Output.EncryptedNote = append([]byte(nil), s.Output.EncryptedNote...)
			changed.Output.EncryptedNote[0] ^= 1
		}
		if err := verifier.Verify(changed, proof); err == nil {
			t.Fatalf("proof replay accepted for changed %s", name)
		}
	}
	for _, bad := range [][]byte{nil, proof[:len(proof)-1], append(append([]byte(nil), proof...), 0), make([]byte, MaxProofBytes+1)} {
		if err := verifier.Verify(s, bad); err == nil {
			t.Fatal("accepted malformed proof")
		}
	}
	wrongPin := pin
	wrongPin[0] ^= 1
	if _, err := NewVerifier(encodedKey.Bytes(), wrongPin); err == nil {
		t.Fatal("accepted unpinned key")
	}
	secondStart := time.Now()
	second, err := Prove(cs, pk, s, w)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(second, proof) {
		t.Fatal("proofs unexpectedly deterministic")
	}
	if err := verifier.Verify(s, second); err != nil {
		t.Fatal(err)
	}
	t.Logf("second randomized proof and verification: %s", time.Since(secondStart))
	t.Logf("full proof roundtrip and context substitution checks: %s", time.Since(started))
}
