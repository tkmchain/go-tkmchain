package shielded6

import (
	"bytes"
	cryptorand "crypto/rand"
	"errors"
	"math/big"
	"testing"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct/clsag"
	"git.gammaspectra.live/P2Pool/consensus/v5/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func BenchmarkShield6MaxOutputRangeProof128(b *testing.B) {
	amount := new(big.Int)
	if _, ok := amount.SetString("5000000000000000000000000", 10); !ok { // 5,000,000 TKM in wei
		b.Fatal("invalid benchmark amount")
	}
	amounts := make([]*big.Int, MaxRangeOutputs)
	for i := range amounts {
		amounts[i] = new(big.Int).Set(amount)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		commitments, proof, err := ProveRangeProof128(amounts, cryptorand.Reader)
		if err != nil {
			b.Fatal(err)
		}
		if err := VerifyRangeProof128(commitments, proof); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkShield6MaxOutputRangeProofVerify isolates the consensus-side
// verifier cost. Proof construction is deliberately outside the timed region.
func BenchmarkShield6MaxOutputRangeProofVerify(b *testing.B) {
	amount := new(big.Int)
	if _, ok := amount.SetString("5000000000000000000000000", 10); !ok {
		b.Fatal("invalid benchmark amount")
	}
	amounts := make([]*big.Int, MaxRangeOutputs)
	for i := range amounts {
		amounts[i] = new(big.Int).Set(amount)
	}
	commitments, proof, err := ProveRangeProof128(amounts, cryptorand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := VerifyRangeProof128(commitments, proof); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkShield6MaxInputCLSAGVerify isolates validation of all 16
// fixed-size, 16-member CLSAG rings. Key generation and signing are untimed.
func BenchmarkShield6MaxInputCLSAGVerify(b *testing.B) {
	rng := crypto.NewDeterministicTestGenerator()
	inputs := make([]SigningInput128, MaxCLSAGInputs)
	for i := range inputs {
		input := &inputs[i]
		input.SignerIndex = uint64(i % RingSize)
		input.Amount = big.NewInt(int64(100_000 + i))
		for j := range RingSize {
			var spend, mask curve25519.Scalar
			curve25519.RandomScalar(&spend, rng)
			curve25519.RandomScalar(&mask, rng)
			amount := new(big.Int).Add(input.Amount, big.NewInt(int64(j+1)))
			if j == int(input.SignerIndex) {
				amount.Set(input.Amount)
				copy(input.SpendSecret[:], spend.Bytes())
				copy(input.CommitmentMask[:], mask.Bytes())
			}
			key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
			commitment, err := Commitment128(amount, scalarBytes(mask))
			if err != nil {
				b.Fatal(err)
			}
			copy(input.Ring[j].OneTimeKey[:], key.Bytes())
			input.Ring[j].Commitment = commitment
		}
	}
	var intent [32]byte
	copy(intent[:], []byte("TKM Shield6 maximum CLSAG verifier benchmark"))
	var outputMasks [ScalarSize]byte
	signed, err := SignCLSAGs128(intent, inputs, outputMasks, cryptorand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for i := range signed {
			if err := VerifyCLSAG(intent, inputs[i].Ring[:], signed[i].KeyImage, signed[i].PseudoOutput, signed[i].Signature[:]); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkShield6MaxInputCLSAGSignAndVerify(b *testing.B) {
	setupRNG := crypto.NewDeterministicTestGenerator()
	inputs := make([]SigningInput128, MaxCLSAGInputs)
	for i := range inputs {
		input := &inputs[i]
		input.SignerIndex = uint64(i % RingSize)
		input.Amount = big.NewInt(int64(100_000 + i))
		for j := range RingSize {
			var spend, mask curve25519.Scalar
			curve25519.RandomScalar(&spend, setupRNG)
			curve25519.RandomScalar(&mask, setupRNG)
			if j == int(input.SignerIndex) {
				copy(input.SpendSecret[:], spend.Bytes())
				copy(input.CommitmentMask[:], mask.Bytes())
			} else {
				inputAmount := new(big.Int).Add(input.Amount, big.NewInt(int64(j+1)))
				encodedMask := scalarBytes(mask)
				commitment, err := Commitment128(inputAmount, encodedMask)
				if err != nil {
					b.Fatal(err)
				}
				input.Ring[j].Commitment = commitment
			}
			key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
			copy(input.Ring[j].OneTimeKey[:], key.Bytes())
			if j == int(input.SignerIndex) {
				encodedMask := scalarBytes(mask)
				commitment, err := Commitment128(input.Amount, encodedMask)
				if err != nil {
					b.Fatal(err)
				}
				input.Ring[j].Commitment = commitment
			}
		}
	}
	var intent [32]byte
	copy(intent[:], []byte("TKM Shield6 maximum CLSAG benchmark"))
	var outputMasks [ScalarSize]byte
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		signed, err := SignCLSAGs128(intent, inputs, outputMasks, cryptorand.Reader)
		if err != nil {
			b.Fatal(err)
		}
		for i := range signed {
			if err := VerifyCLSAG(intent, inputs[i].Ring[:], signed[i].KeyImage, signed[i].PseudoOutput, signed[i].Signature[:]); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestCLSAGAndConfidentialAmountProofRoundTrip(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	var input SigningInput
	input.SignerIndex = 7
	input.Amount = 1000
	var spendSecret, inputMask curve25519.Scalar
	for i := range RingSize {
		var secret, mask curve25519.Scalar
		curve25519.RandomScalar(&secret, rng)
		curve25519.RandomScalar(&mask, rng)
		amount := uint64(1000 + i)
		if i == int(input.SignerIndex) {
			spendSecret, inputMask = secret, mask
			input.Amount = 1000
			amount = input.Amount
		}
		key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&secret)
		commitment := ringct.CalculateCommitment(new(curve25519.PublicKey[curve25519.VarTimeOperations]), ringct.LazyCommitment{Mask: mask, Amount: amount})
		copy(input.Ring[i].OneTimeKey[:], key.Bytes())
		copy(input.Ring[i].Commitment[:], commitment.Bytes())
	}
	copy(input.SpendSecret[:], spendSecret.Bytes())
	copy(input.CommitmentMask[:], inputMask.Bytes())
	var outputMask curve25519.Scalar
	curve25519.RandomScalar(&outputMask, rng)
	var outputMasks [ScalarSize]byte
	copy(outputMasks[:], outputMask.Bytes())
	var message [32]byte
	copy(message[:], []byte("TKM Shield6 test transaction domain"))

	signed, err := SignCLSAGs(message, []SigningInput{input}, outputMasks, rng)
	if err != nil {
		t.Fatalf("sign CLSAG: %v", err)
	}
	if err := VerifyCLSAG(message, input.Ring[:], signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); err != nil {
		t.Fatalf("verify generated CLSAG: %v", err)
	}
	derivedImage, err := DeriveKeyImage(input.Ring[input.SignerIndex].OneTimeKey[:], input.SpendSecret[:])
	if err != nil || derivedImage != signed[0].KeyImage {
		t.Fatalf("wallet-derived key image mismatch: err=%v", err)
	}
	if _, err := DeriveKeyImage(input.Ring[0].OneTimeKey[:], input.SpendSecret[:]); err == nil {
		t.Fatal("key-image derivation accepted a secret for a different output key")
	}
	if err := VerifyCLSAG([32]byte{1}, input.Ring[:], signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); err == nil {
		t.Fatal("CLSAG accepted a different transaction intent")
	}
	tampered := signed[0].Signature
	tampered[0] ^= 1
	if err := VerifyCLSAG(message, input.Ring[:], signed[0].KeyImage, signed[0].PseudoOutput, tampered[:]); err == nil {
		t.Fatal("CLSAG accepted a tampered response")
	}
	// These substitutions remain well-formed curve points and therefore
	// exercise transcript binding, rather than only the point decoder.
	if err := VerifyCLSAG(message, input.Ring[:], input.Ring[0].OneTimeKey, signed[0].PseudoOutput, signed[0].Signature[:]); err == nil {
		t.Fatal("CLSAG accepted a substituted valid key image")
	}
	if err := VerifyCLSAG(message, input.Ring[:], signed[0].KeyImage, input.Ring[0].Commitment, signed[0].Signature[:]); err == nil {
		t.Fatal("CLSAG accepted a substituted valid pseudo-output")
	}
	reorderedRing := append([]RingMember(nil), input.Ring[:]...)
	reorderedRing[0], reorderedRing[1] = reorderedRing[1], reorderedRing[0]
	if err := VerifyCLSAG(message, reorderedRing, signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); err == nil {
		t.Fatal("CLSAG accepted a reordered ring")
	}
	duplicateRing := append([]RingMember(nil), input.Ring[:]...)
	duplicateRing[RingSize-1] = duplicateRing[0]
	if err := VerifyCLSAG(message, duplicateRing, signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); !errors.Is(err, ErrInvalidRing) {
		t.Fatalf("duplicate ring key error = %v, want ErrInvalidRing", err)
	}

	commitments, proof, err := ProveRangeProof([]ringct.LazyCommitment{{Mask: outputMask, Amount: 990}}, rng)
	if err != nil {
		t.Fatalf("prove output amount range: %v", err)
	}
	if err := VerifyRangeProof(commitments, proof); err != nil {
		t.Fatalf("verify generated Bulletproofs+: %v", err)
	}
	badProof := append(append([]byte(nil), proof...), 0)
	if err := VerifyRangeProof(commitments, badProof); !errors.Is(err, ErrInvalidRangeProof) {
		t.Fatalf("trailing proof bytes error = %v, want ErrInvalidRangeProof", err)
	}
	badCommitments := append([][PointSize]byte(nil), commitments...)
	badCommitments[0][0] ^= 1
	if err := VerifyRangeProof(badCommitments, proof); err == nil {
		t.Fatal("Bulletproofs+ accepted a proof for a different amount commitment")
	}

	if err := VerifyCommitmentBalance([][PointSize]byte{signed[0].PseudoOutput}, commitments, 10); err != nil {
		t.Fatalf("valid commitment conservation equation rejected: %v", err)
	}
	if err := VerifyCommitmentBalance([][PointSize]byte{signed[0].PseudoOutput}, commitments, 9); !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("unbalanced commitment equation error = %v, want ErrUnbalanced", err)
	}
}

// TestShield6VerifierAcceptsPinnedUpstreamCLSAG checks our transaction-facing
// fixed-width decoder and verifier against a signature produced independently
// by the pinned upstream wallet signer. The normal Shield6 signer already
// checks its own result with the upstream verifier; this is the reverse
// interoperability direction.
func TestShield6VerifierAcceptsPinnedUpstreamCLSAG(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	const signerIndex = 9
	const amount = uint64(18_000_000_000)
	var spendSecret, realMask curve25519.Scalar
	var decoys ringct.Decoys[curve25519.VarTimeOperations]
	decoys.SignerIndex = signerIndex
	var members [RingSize]RingMember
	for i := range RingSize {
		var spend, mask curve25519.Scalar
		curve25519.RandomScalar(&spend, rng)
		curve25519.RandomScalar(&mask, rng)
		memberAmount := uint64(90_000 + i)
		if i == signerIndex {
			spendSecret, realMask = spend, mask
			memberAmount = amount
		}
		key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
		commitment := ringct.CalculateCommitment(
			new(curve25519.PublicKey[curve25519.VarTimeOperations]),
			ringct.LazyCommitment{Mask: mask, Amount: memberAmount},
		)
		decoys.Ring = append(decoys.Ring, [2]curve25519.PublicKey[curve25519.VarTimeOperations]{*key, *commitment})
		decoys.Offsets = append(decoys.Offsets, uint64(i+1))
		copy(members[i].OneTimeKey[:], key.Bytes())
		copy(members[i].Commitment[:], commitment.Bytes())
	}
	var intent [32]byte
	copy(intent[:], []byte("TKM Shield6 upstream CLSAG interoperability"))
	context, err := clsag.NewContext(decoys, ringct.LazyCommitment{Mask: realMask, Amount: amount})
	if err != nil {
		t.Fatalf("create upstream CLSAG context: %v", err)
	}
	keyPair := crypto.NewKeyPairFromPrivate[curve25519.VarTimeOperations](&spendSecret)
	var outputMask curve25519.Scalar
	curve25519.RandomScalar(&outputMask, rng)
	signed, err := clsag.Sign(
		types.Hash(intent),
		[]clsag.Input[curve25519.VarTimeOperations]{{KeyPair: *keyPair, Context: *context}},
		&outputMask, rng,
	)
	if err != nil || len(signed) != 1 {
		t.Fatalf("upstream CLSAG signer failed: signatures=%d err=%v", len(signed), err)
	}
	encoded, err := signed[0].Signature.AppendBinary(make([]byte, 0, CLSAGSize))
	if err != nil || len(encoded) != CLSAGSize {
		t.Fatalf("encode upstream CLSAG signature: bytes=%d err=%v", len(encoded), err)
	}
	var keyImage, pseudoOutput [PointSize]byte
	copy(keyImage[:], crypto.GetBiasedKeyImage(new(curve25519.PublicKey[curve25519.VarTimeOperations]), keyPair).Bytes())
	copy(pseudoOutput[:], signed[0].PseudoOut.Bytes())
	if err := VerifyCLSAG(intent, members[:], keyImage, pseudoOutput, encoded); err != nil {
		t.Fatalf("Shield6 verifier rejected pinned upstream signer output: %v", err)
	}
}

func TestShield6RangeProofConsensusVerificationIsDeterministic(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	var mask curve25519.Scalar
	curve25519.RandomScalar(&mask, rng)
	commitments, proof, err := ProveRangeProof([]ringct.LazyCommitment{{Mask: mask, Amount: 42}}, rng)
	if err != nil {
		t.Fatalf("prove amount: %v", err)
	}
	wide := newRangeProofVerifierReader(commitments, proof)
	second := newRangeProofVerifierReader(commitments, proof)
	firstBytes, secondBytes := make([]byte, 96), make([]byte, 96)
	if _, err := wide.Read(firstBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Read(secondBytes); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("same public range-proof statement produced different verifier coefficients")
	}
	for i := 0; i < 4; i++ {
		if err := VerifyRangeProof(commitments, proof); err != nil {
			t.Fatalf("valid proof rejected on verification %d: %v", i, err)
		}
	}
	badProof := append([]byte(nil), proof...)
	badProof[len(badProof)/2] ^= 1
	for i := 0; i < 4; i++ {
		if err := VerifyRangeProof(commitments, badProof); err == nil {
			t.Fatalf("tampered proof accepted on verification %d", i)
		}
	}
}

func TestShield6WideAmountRangeProof128(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	amounts := []*big.Int{
		new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 80), big.NewInt(12345)),
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1)),
	}
	outputs, proof, witnesses, err := ProveRangeProof128WithWitness(amounts, rng)
	if err != nil {
		t.Fatalf("prove 128-bit amount range: %v", err)
	}
	if err := VerifyRangeProof128(outputs, proof); err != nil {
		t.Fatalf("verify 128-bit amount range: %v", err)
	}
	for i := range outputs {
		commitment, err := Commitment128(amounts[i], witnesses[i].Mask)
		if err != nil || commitment != outputs[i].Commitment {
			t.Fatalf("128-bit witness commitment %d mismatch: err=%v", i, err)
		}
	}
	tampered := append([]WideCommitment(nil), outputs...)
	tampered[0].Limbs[1][0] ^= 1
	if err := VerifyRangeProof128(tampered, proof); err == nil {
		t.Fatal("wide range proof accepted a tampered high limb")
	}
	if _, _, err := ProveRangeProof128([]*big.Int{new(big.Int).Lsh(big.NewInt(1), 128)}, rng); !errors.Is(err, ErrInvalidRangeProof) {
		t.Fatalf("amount above 128 bits error = %v, want ErrInvalidRangeProof", err)
	}
}

func TestShield6MaximumOutputRangeProofBundle(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	amounts := make([]*big.Int, MaxRangeOutputs)
	for i := range amounts {
		amounts[i] = new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), uint(64+i%64)), big.NewInt(int64(i+1)))
	}
	outputs, bundle, _, err := ProveRangeProof128WithWitness(amounts, rng)
	if err != nil {
		t.Fatalf("prove maximum-output paired range proof: %v", err)
	}
	if len(outputs) != MaxRangeOutputs {
		t.Fatalf("output count = %d, want %d", len(outputs), MaxRangeOutputs)
	}
	if len(bundle) > MaxRangeProofBundleSize {
		t.Fatalf("proof bundle size = %d, max %d", len(bundle), MaxRangeProofBundleSize)
	}
	if err := VerifyRangeProof128(outputs, bundle); err != nil {
		t.Fatalf("verify maximum-output paired range proof: %v", err)
	}

	lowProof, highProof, err := decodeRangeProofBundle(bundle)
	if err != nil {
		t.Fatalf("decode generated proof bundle: %v", err)
	}
	lowProof[len(lowProof)-1] ^= 1
	badLow, err := encodeRangeProofBundle(lowProof, highProof)
	if err != nil {
		t.Fatalf("encode tampered low proof bundle: %v", err)
	}
	if err := VerifyRangeProof128(outputs, badLow); err == nil {
		t.Fatal("accepted tampered low-limb aggregate proof")
	}

	lowProof, highProof, err = decodeRangeProofBundle(bundle)
	if err != nil {
		t.Fatalf("decode generated proof bundle: %v", err)
	}
	highProof[len(highProof)-1] ^= 1
	badHigh, err := encodeRangeProofBundle(lowProof, highProof)
	if err != nil {
		t.Fatalf("encode tampered high proof bundle: %v", err)
	}
	if err := VerifyRangeProof128(outputs, badHigh); err == nil {
		t.Fatal("accepted tampered high-limb aggregate proof")
	}
	if err := VerifyRangeProof128(outputs, append(bundle, 0)); !errors.Is(err, ErrInvalidRangeProof) {
		t.Fatalf("trailing bundle bytes error = %v, want ErrInvalidRangeProof", err)
	}
	malformed, err := rlp.EncodeToBytes([][]byte{lowProof})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRangeProof128(outputs, malformed); !errors.Is(err, ErrInvalidRangeProof) {
		t.Fatalf("single-proof bundle error = %v, want ErrInvalidRangeProof", err)
	}
}

func TestShield6WalletKeysAndPaymentCode(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	keys, err := DeriveWalletKeys(seed, 8979)
	if err != nil {
		t.Fatalf("derive Shield6 wallet keys: %v", err)
	}
	defer keys.Clear()
	again, err := DeriveWalletKeys(seed, 8979)
	if err != nil {
		t.Fatalf("derive deterministic Shield6 wallet keys: %v", err)
	}
	defer again.Clear()
	if keys.SpendSecret != again.SpendSecret || keys.ViewSecret != again.ViewSecret || keys.SpendPublic != again.SpendPublic || keys.ViewPublic != again.ViewPublic {
		t.Fatal("Shield6 keys are not deterministic for a seed and chain")
	}
	code, err := EncodePaymentCode(keys.Address())
	if err != nil {
		t.Fatalf("encode Shield6 payment code: %v", err)
	}
	decoded, err := DecodePaymentCode(code, 8979)
	if err != nil || decoded != keys.Address() {
		t.Fatalf("decode Shield6 payment code: err=%v", err)
	}
	if _, err := DecodePaymentCode(code, 8980); err == nil {
		t.Fatal("payment code accepted on the wrong chain")
	}
	tampered := []byte(code)
	tampered[len(tampered)-1] ^= 1
	if _, err := DecodePaymentCode(string(tampered), 8979); err == nil {
		t.Fatal("payment code accepted a checksum mutation")
	}
	otherChain, err := DeriveWalletKeys(seed, 8980)
	if err != nil {
		t.Fatalf("derive other-chain Shield6 keys: %v", err)
	}
	defer otherChain.Clear()
	if keys.SpendPublic == otherChain.SpendPublic || keys.ViewPublic == otherChain.ViewPublic {
		t.Fatal("Shield6 public keys are not chain-separated")
	}
	created, backupSeed, err := CreateWallet(crypto.NewDeterministicTestGenerator(), 8979)
	if err != nil {
		t.Fatalf("create Shield6 wallet from fresh entropy: %v", err)
	}
	defer created.Clear()
	if backupSeed == ([32]byte{}) {
		t.Fatal("wallet creation returned an empty backup seed")
	}
	restored, err := DeriveWalletKeys(backupSeed[:], 8979)
	if err != nil {
		t.Fatalf("restore newly created Shield6 wallet: %v", err)
	}
	defer restored.Clear()
	if created.Address() != restored.Address() {
		t.Fatal("wallet backup seed did not restore the created Shield6 address")
	}
	clear(backupSeed[:])
}

func TestShield6NoteAndSigningInputClearBackingWords(t *testing.T) {
	amount := new(big.Int).Lsh(big.NewInt(1), 100)
	note := &Note{Amount: new(big.Int).Set(amount)}
	for i := range note.Witness.Mask {
		note.Witness.Mask[i] = 0xa5
		note.Witness.LimbMasks[0][i] = 0x5a
		note.Witness.LimbMasks[1][i] = 0x3c
		note.SpendSecret[i] = 0xc3
	}
	noteWords := note.Amount.Bits()
	input := &SigningInput128{Amount: new(big.Int).Set(amount)}
	for i := range input.SpendSecret {
		input.SpendSecret[i] = 0x96
		input.CommitmentMask[i] = 0x69
	}
	inputWords := input.Amount.Bits()

	note.Clear()
	input.Clear()
	if note.Amount.Sign() != 0 || input.Amount.Sign() != 0 {
		t.Fatal("cleared secret amount remains nonzero")
	}
	for i, word := range noteWords {
		if word != 0 {
			t.Fatalf("note amount backing word %d was not wiped", i)
		}
	}
	for i, word := range inputWords {
		if word != 0 {
			t.Fatalf("signing-input amount backing word %d was not wiped", i)
		}
	}
	if !bytes.Equal(note.Witness.Mask[:], make([]byte, ScalarSize)) ||
		!bytes.Equal(note.Witness.LimbMasks[0][:], make([]byte, ScalarSize)) ||
		!bytes.Equal(note.Witness.LimbMasks[1][:], make([]byte, ScalarSize)) ||
		!bytes.Equal(note.SpendSecret[:], make([]byte, ScalarSize)) ||
		!bytes.Equal(input.SpendSecret[:], make([]byte, ScalarSize)) ||
		!bytes.Equal(input.CommitmentMask[:], make([]byte, ScalarSize)) {
		t.Fatal("note or signing-input scalar bytes were not wiped")
	}
}

func TestShield6WideWitnessCleanup(t *testing.T) {
	witnesses := make([]WideWitness, 2)
	for i := range witnesses {
		for j := range witnesses[i].Mask {
			witnesses[i].Mask[j] = 0xa5
			witnesses[i].LimbMasks[0][j] = 0x5a
			witnesses[i].LimbMasks[1][j] = 0x3c
		}
	}
	clearWideWitnesses(witnesses)
	for i, witness := range witnesses {
		if witness.Mask != ([ScalarSize]byte{}) || witness.LimbMasks[0] != ([ScalarSize]byte{}) || witness.LimbMasks[1] != ([ScalarSize]byte{}) {
			t.Fatalf("wide witness %d was not completely cleared", i)
		}
	}
}

func TestShield6WalletOutputCreateAndScan(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(0xa0 + i)
	}
	keys, err := DeriveWalletKeys(seed, 8979)
	if err != nil {
		t.Fatalf("derive recipient wallet keys: %v", err)
	}
	defer keys.Clear()
	otherSeed := append([]byte(nil), seed...)
	otherSeed[0] ^= 0xff
	otherKeys, err := DeriveWalletKeys(otherSeed, 8979)
	if err != nil {
		t.Fatalf("derive non-recipient wallet keys: %v", err)
	}
	defer otherKeys.Clear()

	amount := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 80), big.NewInt(1234567))
	_, _, witnesses, err := ProveRangeProof128WithWitness([]*big.Int{amount}, rng)
	if err != nil {
		t.Fatalf("create amount witness: %v", err)
	}
	output, err := CreateOutput(keys.Address(), 0, 1, amount, witnesses[0], rng)
	if err != nil {
		t.Fatalf("create encrypted Shield6 output: %v", err)
	}
	if len(output.EncryptedNote) == 0 || len(output.EncryptedNote) > 4096 {
		t.Fatalf("encrypted note size out of bounds: %d", len(output.EncryptedNote))
	}
	if note, found, err := ScanOutput(otherKeys, 0, 19, output); err != nil || found || note != nil {
		t.Fatalf("non-recipient scan = (%v, %v, %v), want (nil, false, nil)", note, found, err)
	}
	note, found, err := ScanOutput(keys, 0, 19, output)
	if err != nil || !found || note == nil {
		t.Fatalf("recipient scan = (%v, %v, %v), want owned note", note, found, err)
	}
	defer note.Clear()
	if note.Index != 19 || note.AssetID != 1 || note.Amount.Cmp(amount) != 0 {
		t.Fatalf("recovered note metadata mismatch: index=%d asset=%d amount=%v", note.Index, note.AssetID, note.Amount)
	}
	spendSecret, err := decodeScalar(note.SpendSecret[:])
	if err != nil {
		t.Fatalf("decode recovered one-time spend key: %v", err)
	}
	spendPublic := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(spendSecret)
	if !bytesEqual(spendPublic.Bytes(), output.OneTimeKey[:]) {
		t.Fatal("recovered one-time spend key does not match output key")
	}
	badSpendKeys := *keys
	badSpendKeys.SpendSecret[0] ^= 1
	if note, found, err := ScanOutput(&badSpendKeys, 0, 19, output); err == nil || found || note != nil {
		t.Fatalf("inconsistent wallet spend key scan = (%v, %v, %v), want rejection", note, found, err)
	}
	var ring [RingSize]RingMember
	const signerIndex = 7
	for i := range RingSize {
		if i == signerIndex {
			ring[i] = RingMember{OneTimeKey: output.OneTimeKey, Commitment: output.Commitment}
			continue
		}
		var decoySpend, decoyMask curve25519.Scalar
		curve25519.RandomScalar(&decoySpend, rng)
		curve25519.RandomScalar(&decoyMask, rng)
		decoyKey := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&decoySpend)
		decoyCommitment := ringct.CalculateCommitment(new(curve25519.PublicKey[curve25519.VarTimeOperations]), ringct.LazyCommitment{Mask: decoyMask, Amount: amount.Uint64()})
		copy(ring[i].OneTimeKey[:], decoyKey.Bytes())
		copy(ring[i].Commitment[:], decoyCommitment.Bytes())
	}
	signingInput, err := note.SigningInput(ring, signerIndex)
	if err != nil {
		t.Fatalf("prepare scanned note for CLSAG: %v", err)
	}
	destinationCommitments, _, destinationWitnesses, err := ProveRangeProof128WithWitness([]*big.Int{amount}, rng)
	if err != nil {
		t.Fatalf("create destination output witness: %v", err)
	}
	var intent [32]byte
	copy(intent[:], []byte("Shield6 scanned-note spend test"))
	signed, err := SignCLSAGs128(intent, []SigningInput128{signingInput}, destinationWitnesses[0].Mask, rng)
	if err != nil {
		t.Fatalf("sign with scanned note: %v", err)
	}
	if err := VerifyCLSAG(intent, ring[:], signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); err != nil {
		t.Fatalf("verify scanned-note CLSAG: %v", err)
	}
	if err := VerifyCommitmentBalance([][PointSize]byte{signed[0].PseudoOutput}, [][PointSize]byte{destinationCommitments[0].Commitment}, 0); err != nil {
		t.Fatalf("verify scanned-note spend value conservation: %v", err)
	}

	tampered := output
	tampered.EncryptedNote = append([]byte(nil), output.EncryptedNote...)
	tampered.EncryptedNote[len(tampered.EncryptedNote)-1] ^= 1
	if _, found, err := ScanOutput(keys, 0, 19, tampered); err == nil || found {
		t.Fatalf("tampered encrypted note scan = (found=%v, err=%v), want authentication failure", found, err)
	}
	if note, found, err := ScanOutput(keys, 1, 19, output); err != nil || found || note != nil {
		t.Fatalf("output scan at wrong transaction position = (%v, %v, %v), want (nil, false, nil)", note, found, err)
	}
}

func TestShield6CLSAGAllSignerPositions(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	amount := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 80), big.NewInt(77))
	var ring [RingSize]RingMember
	var spendSecrets [RingSize][ScalarSize]byte
	var inputMasks [RingSize][ScalarSize]byte
	for i := range RingSize {
		var spend, mask curve25519.Scalar
		curve25519.RandomScalar(&spend, rng)
		curve25519.RandomScalar(&mask, rng)
		copy(spendSecrets[i][:], spend.Bytes())
		copy(inputMasks[i][:], mask.Bytes())
		pub := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
		commitment, err := Commitment128(amount, scalarBytes(mask))
		if err != nil {
			t.Fatalf("make ring commitment: %v", err)
		}
		copy(ring[i].OneTimeKey[:], pub.Bytes())
		ring[i].Commitment = commitment
	}
	output, _, witness, err := ProveRangeProof128WithWitness([]*big.Int{amount}, rng)
	if err != nil {
		t.Fatalf("prove output: %v", err)
	}
	var message [32]byte
	copy(message[:], []byte("TKM Shield6 signer-position coverage"))
	for signer := range RingSize {
		input := SigningInput128{Ring: ring, SignerIndex: uint64(signer), SpendSecret: spendSecrets[signer], CommitmentMask: inputMasks[signer], Amount: amount}
		signed, err := SignCLSAGs128(message, []SigningInput128{input}, witness[0].Mask, rng)
		if err != nil {
			t.Fatalf("sign at ring position %d: %v", signer, err)
		}
		if err := VerifyCLSAG(message, ring[:], signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); err != nil {
			t.Fatalf("verify at ring position %d: %v", signer, err)
		}
		if err := VerifyCommitmentBalance([][PointSize]byte{signed[0].PseudoOutput}, [][PointSize]byte{output[0].Commitment}, 0); err != nil {
			t.Fatalf("balance at ring position %d: %v", signer, err)
		}
	}
}

func TestShield6WideCLSAGRoundTrip(t *testing.T) {
	rng := crypto.NewDeterministicTestGenerator()
	amount := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 80), big.NewInt(987654321))
	input := SigningInput128{SignerIndex: RingSize - 1, Amount: amount}
	var spendSecret, inputMask curve25519.Scalar
	for i := 0; i < RingSize; i++ {
		var spend, mask curve25519.Scalar
		curve25519.RandomScalar(&spend, rng)
		curve25519.RandomScalar(&mask, rng)
		memberAmount := new(big.Int).Add(amount, big.NewInt(int64(i+1)))
		if i == int(input.SignerIndex) {
			spendSecret, inputMask = spend, mask
			memberAmount = new(big.Int).Set(amount)
		}
		key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
		commitment, err := Commitment128(memberAmount, scalarBytes(mask))
		if err != nil {
			t.Fatalf("build ring commitment %d: %v", i, err)
		}
		copy(input.Ring[i].OneTimeKey[:], key.Bytes())
		input.Ring[i].Commitment = commitment
	}
	copy(input.SpendSecret[:], spendSecret.Bytes())
	copy(input.CommitmentMask[:], inputMask.Bytes())

	outputs, proof, witnesses, err := ProveRangeProof128WithWitness([]*big.Int{amount}, rng)
	if err != nil {
		t.Fatalf("prove output amount: %v", err)
	}
	if err := VerifyRangeProof128(outputs, proof); err != nil {
		t.Fatalf("verify output amount: %v", err)
	}
	var message [32]byte
	copy(message[:], []byte("TKM Shield6 wide CLSAG transcript"))
	signed, err := SignCLSAGs128(message, []SigningInput128{input}, witnesses[0].Mask, rng)
	if err != nil {
		t.Fatalf("sign 128-bit CLSAG: %v", err)
	}
	if err := VerifyCLSAG(message, input.Ring[:], signed[0].KeyImage, signed[0].PseudoOutput, signed[0].Signature[:]); err != nil {
		t.Fatalf("verify 128-bit CLSAG: %v", err)
	}
	if err := VerifyCommitmentBalance([][PointSize]byte{signed[0].PseudoOutput}, [][PointSize]byte{outputs[0].Commitment}, 0); err != nil {
		t.Fatalf("verify 128-bit commitment conservation: %v", err)
	}
}

func TestShield6RejectsMalformedInputsBeforeCurveWork(t *testing.T) {
	var message [32]byte
	var image, pseudo [PointSize]byte
	if err := VerifyCLSAG(message, make([]RingMember, RingSize-1), image, pseudo, make([]byte, CLSAGSize)); !errors.Is(err, ErrInvalidRing) {
		t.Fatalf("short ring error = %v, want ErrInvalidRing", err)
	}
	if err := VerifyCLSAG(message, make([]RingMember, RingSize), image, pseudo, make([]byte, CLSAGSize-1)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("short signature error = %v, want ErrInvalidSignature", err)
	}
	if err := VerifyRangeProof(nil, []byte{1}); !errors.Is(err, ErrInvalidRangeProof) {
		t.Fatalf("empty range proof error = %v, want ErrInvalidRangeProof", err)
	}
}

func TestSelectRingShufflesIndicesAndIncludesRealOutput(t *testing.T) {
	keyRNG := crypto.NewDeterministicTestGenerator()
	candidates := make([]IndexedRingMember, 64)
	var realIndex uint64
	for i := range candidates {
		var spend, mask curve25519.Scalar
		curve25519.RandomScalar(&spend, keyRNG)
		curve25519.RandomScalar(&mask, keyRNG)
		key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
		commitment := ringct.CalculateCommitment(new(curve25519.PublicKey[curve25519.VarTimeOperations]), ringct.LazyCommitment{Mask: mask, Amount: uint64(i + 1)})
		candidates[i] = IndexedRingMember{Index: uint64(i*3 + 2)}
		copy(candidates[i].Member.OneTimeKey[:], key.Bytes())
		copy(candidates[i].Member.Commitment[:], commitment.Bytes())
	}
	realPosition := 37
	realIndex = candidates[realPosition].Index
	rng1, rng2 := crypto.NewDeterministicTestGenerator(), crypto.NewDeterministicTestGenerator()
	ring1, indices1, signer1, err := SelectRing(realIndex, candidates, rng1)
	if err != nil {
		t.Fatalf("select canonical ring: %v", err)
	}
	ring2, indices2, signer2, err := SelectRing(realIndex, candidates, rng2)
	if err != nil {
		t.Fatalf("repeat canonical ring selection: %v", err)
	}
	if ring1 != ring2 || indices1 != indices2 || signer1 != signer2 {
		t.Fatal("fixed local entropy did not reproduce ring selection")
	}
	if signer1 >= RingSize || indices1[signer1] != realIndex || ring1[signer1] != candidates[realPosition].Member {
		t.Fatal("selected ring did not retain the real output and signer position")
	}
	orderedByAge := true
	seenIndices := make(map[uint64]struct{}, RingSize)
	for i := range RingSize {
		if i > 0 && indices1[i] <= indices1[i-1] {
			orderedByAge = false
		}
		if _, duplicate := seenIndices[indices1[i]]; duplicate {
			t.Fatal("selected ring repeats an accumulator index")
		}
		seenIndices[indices1[i]] = struct{}{}
		found := false
		for _, candidate := range candidates {
			if candidate.Index == indices1[i] && candidate.Member == ring1[i] {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("ring member does not correspond to its selected canonical index")
		}
	}
	if orderedByAge {
		t.Fatal("signed ring order still follows accumulator age order")
	}
}

func TestSelectRingRejectsInsufficientOrNoncanonicalCandidates(t *testing.T) {
	if _, _, _, err := SelectRing(0, make([]IndexedRingMember, RingSize-1), crypto.NewDeterministicTestGenerator()); err == nil {
		t.Fatal("ring selection accepted fewer than RingSize candidates")
	}
	candidates := make([]IndexedRingMember, RingSize)
	for i := range candidates {
		candidates[i].Index = uint64(i)
	}
	candidates[RingSize-1].Index = candidates[RingSize-2].Index
	if _, _, _, err := SelectRing(0, candidates, crypto.NewDeterministicTestGenerator()); err == nil {
		t.Fatal("ring selection accepted repeated accumulator indexes")
	}
}
