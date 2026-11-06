package core

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"
	"time"

	monerocrypto "git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func TestShieldedV6IntentBindsRangeProofButNotCLSAGSignature(t *testing.T) {
	pool := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 1_000_000, To: &pool, Value: new(big.Int),
	})
	envelope := &ShieldedV6Transaction{
		Version: 6, AssetID: shielded6.AssetTKM,
		Inputs:     []ShieldedV6Input{{}},
		RangeProof: []byte{0xc1, 0x01},
	}
	base, err := ShieldedV6Intent(tx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	changedProof := *envelope
	changedProof.RangeProof = bytes.Clone(envelope.RangeProof)
	changedProof.RangeProof[1] ^= 1
	proofIntent, err := ShieldedV6Intent(tx, &changedProof)
	if err != nil {
		t.Fatal(err)
	}
	if proofIntent == base {
		t.Fatal("CLSAG intent did not bind the range proof bytes")
	}
	changedSignature := *envelope
	changedSignature.Inputs = append([]ShieldedV6Input(nil), envelope.Inputs...)
	changedSignature.Inputs[0].Signature[0] = 1
	signatureIntent, err := ShieldedV6Intent(tx, &changedSignature)
	if err != nil {
		t.Fatal(err)
	}
	if signatureIntent != base {
		t.Fatal("CLSAG intent unexpectedly depends on its signature bytes")
	}
}

func TestShieldedV6EnvelopeRoundTripAndGas(t *testing.T) {
	var signature [shielded6.CLSAGSize]byte
	signature[0] = 7
	e := &ShieldedV6Transaction{
		Version:    6,
		AssetID:    1,
		Inputs:     []ShieldedV6Input{{Signature: signature}},
		Outputs:    []ShieldedV6Output{{EncryptedNote: []byte{1, 2, 3}}},
		RangeProof: make([]byte, 100),
	}
	data, err := EncodeShieldedV6Transaction(e)
	if err != nil {
		t.Fatalf("encode Shield6: %v", err)
	}
	decoded, ok, err := DecodeShieldedV6Transaction(data)
	if err != nil || !ok || decoded.Version != 6 || decoded.Inputs[0].Signature[0] != 7 {
		t.Fatalf("decode Shield6: ok=%t err=%v envelope=%+v", ok, err, decoded)
	}
	clean, gas, err := ShieldedV6GasData(data)
	if err != nil {
		t.Fatalf("Shield6 gas data: %v", err)
	}
	wantGas := ShieldedV6BaseVerifyGas + ShieldedV6InputVerifyGas + ShieldedV6OutputVerifyGas + 10_000 +
		3*ShieldedV6StateWriteGas + (ShieldedV6OutputTreeDepth+6)*ShieldedV6StateWriteGas
	if gas != wantGas {
		t.Fatalf("Shield6 private transfer gas = %d, want %d including bounded state writes", gas, wantGas)
	}
	twoInputs := *e
	twoInputs.Inputs = append(append([]ShieldedV6Input(nil), e.Inputs...), ShieldedV6Input{Signature: signature})
	twoInputData, err := EncodeShieldedV6Transaction(&twoInputs)
	if err != nil {
		t.Fatal("encode two-input Shield6 gas case:", err)
	}
	_, twoInputGas, err := ShieldedV6GasData(twoInputData)
	if err != nil {
		t.Fatal("price two-input Shield6 transfer:", err)
	}
	wantTwoInputIncrement := ShieldedV6InputVerifyGas + 2*ShieldedV6StateWriteGas + ShieldedV6StateUpdateGas
	if got := twoInputGas - gas; got != wantTwoInputIncrement {
		t.Fatalf("second Shield6 input gas increment = %d, want %d including repeated count-slot update", got, wantTwoInputIncrement)
	}
	cleanEnvelope, ok, err := DecodeShieldedV6Transaction(clean)
	if err != nil || !ok {
		t.Fatalf("decode gas-cleaned envelope: ok=%t err=%v", ok, err)
	}
	if len(cleanEnvelope.RangeProof) != 0 || cleanEnvelope.Inputs[0].Signature != ([shielded6.CLSAGSize]byte{}) {
		t.Fatal("intrinsic gas encoding retained the proof or CLSAG signature bytes")
	}

	deposit := *e
	deposit.Deposit = true
	deposit.Inputs = nil
	depositData, err := EncodeShieldedV6Transaction(&deposit)
	if err != nil {
		t.Fatal("encode Shield6 deposit for gas check:", err)
	}
	_, depositGas, err := ShieldedV6GasData(depositData)
	if err != nil {
		t.Fatal("price Shield6 deposit:", err)
	}
	wantDepositGas := ShieldedV6BaseVerifyGas + ShieldedV6OutputVerifyGas + 10_000 +
		(ShieldedV6OutputTreeDepth+6)*ShieldedV6StateWriteGas + 3*ShieldedV6StateWriteGas
	if depositGas != wantDepositGas {
		t.Fatalf("Shield6 deposit gas = %d, want %d including backing-ledger writes", depositGas, wantDepositGas)
	}

	maxEnvelope := *e
	maxEnvelope.Inputs = make([]ShieldedV6Input, ShieldedV6MaxInputs)
	maxEnvelope.Outputs = make([]ShieldedV6Output, ShieldedV6MaxOutputs)
	maxEnvelope.RangeProof = make([]byte, shielded6.MaxRangeProofBundleSize)
	maxData, err := EncodeShieldedV6Transaction(&maxEnvelope)
	if err != nil {
		t.Fatal("encode maximum Shield6 envelope for gas check:", err)
	}
	_, maxGas, err := ShieldedV6GasData(maxData)
	if err != nil {
		t.Fatal("price maximum Shield6 envelope:", err)
	}
	if maxGas <= gas || maxGas > ShieldedV6BaseVerifyGas+30_000_000 {
		t.Fatalf("maximum Shield6 gas is not bounded as expected: %d", maxGas)
	}
}

// BenchmarkShield6MaxPaymentEnvelope measures the wallet's maximum configured
// private-payment construction: 16 inputs, 16-member rings, and 16 outputs.
// Setup is excluded. The measured builder creates and verifies paired 128-bit
// Bulletproofs+, encrypts outputs, signs all CLSAGs, verifies those signatures,
// checks commitment conservation, and RLP-encodes the final envelope. It does
// not include the outer ML-DSA account signature, RPC scanning, or propagation.
func BenchmarkShield6MaxPaymentEnvelope(b *testing.B) {
	const chainID = uint64(8979)
	setupRNG := monerocrypto.NewDeterministicTestGenerator()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	recipient, err := shielded6.DeriveWalletKeys(seed, chainID)
	clear(seed)
	if err != nil {
		b.Fatal(err)
	}
	defer recipient.Clear()
	inputs := make([]ShieldedV6WalletInput, ShieldedV6MaxInputs)
	outputs := make([]ShieldedV6WalletOutput, ShieldedV6MaxOutputs)
	inputAmount := big.NewInt(1000)
	for i := range inputs {
		input := &inputs[i]
		input.SignerIndex = uint64(i % shielded6.RingSize)
		input.Note = &shielded6.Note{
			ChainID: chainID, AssetID: shielded6.AssetTKM,
			Index:  uint64(i*shielded6.RingSize) + input.SignerIndex,
			Amount: new(big.Int).Set(inputAmount),
		}
		var spendSecret, commitmentMask curve25519.Scalar
		curve25519.RandomScalar(&spendSecret, setupRNG)
		curve25519.RandomScalar(&commitmentMask, setupRNG)
		copy(input.Note.SpendSecret[:], spendSecret.Bytes())
		copy(input.Note.Witness.Mask[:], commitmentMask.Bytes())
		for j := range shielded6.RingSize {
			var spend, mask curve25519.Scalar
			if uint64(j) == input.SignerIndex {
				spend.Set(&spendSecret)
				mask.Set(&commitmentMask)
			} else {
				curve25519.RandomScalar(&spend, setupRNG)
				curve25519.RandomScalar(&mask, setupRNG)
			}
			key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
			copy(input.Ring[j].OneTimeKey[:], key.Bytes())
			amount := new(big.Int).Set(inputAmount)
			if uint64(j) != input.SignerIndex {
				amount.Add(amount, big.NewInt(int64(j+1)))
			}
			var encodedMask [shielded6.ScalarSize]byte
			copy(encodedMask[:], mask.Bytes())
			commitment, err := shielded6.Commitment128(amount, encodedMask)
			if err != nil {
				b.Fatal(err)
			}
			input.Ring[j].Commitment = commitment
			input.RingIndices[j] = uint64(i*shielded6.RingSize + j)
			mask.Set(new(curve25519.Scalar))
			spend.Set(new(curve25519.Scalar))
		}
		spendSecret.Set(new(curve25519.Scalar))
		commitmentMask.Set(new(curve25519.Scalar))
	}
	total := new(big.Int).Mul(inputAmount, big.NewInt(ShieldedV6MaxInputs))
	perOutput := new(big.Int).Quo(new(big.Int).Set(total), big.NewInt(ShieldedV6MaxOutputs))
	for i := range outputs {
		outputs[i] = ShieldedV6WalletOutput{Address: recipient.Address(), Amount: new(big.Int).Set(perOutput)}
	}
	to := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{
		ChainID: new(big.Int).SetUint64(chainID), To: &to, Value: new(big.Int),
		Gas: 8_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
		Algorithm: "ML-DSA-87", PublicKey: []byte{1},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := BuildShieldedV6Envelope(tx, inputs, outputs, cryptorand.Reader); err != nil {
			b.Fatal(err)
		}
	}
}

// TestShield6MaxPaymentEnvelopeUnderTwoMinutes turns the maximum-capacity
// builder benchmark into a release gate. It includes paired 128-bit range
// proofs, 16 CLSAG inputs, 16 encrypted outputs and local verification, while
// deliberately excluding RPC scans and the outer ML-DSA signature.
func TestShield6MaxPaymentEnvelopeUnderTwoMinutes(t *testing.T) {
	result := testing.Benchmark(BenchmarkShield6MaxPaymentEnvelope)
	if result.N == 0 || result.NsPerOp() <= 0 {
		t.Fatalf("Shield6 max-payment benchmark produced no timing: %s", result)
	}
	if result.NsPerOp() > int64(2*time.Minute) {
		t.Fatalf("Shield6 maximum payment envelope took %s per build; exceeds the two-minute target (%s)", time.Duration(result.NsPerOp()), result)
	}
	t.Logf("Shield6 maximum payment envelope: %s per build (%s)", time.Duration(result.NsPerOp()), result)
}

func TestShieldedV6OutputAccumulatorIsAppendOnlyAndRevertible(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ShieldedV6OutputRoot(st), shieldedV6EmptyNodes()[ShieldedV6OutputTreeDepth]; got != want {
		t.Fatalf("empty Shield6 root = %s, want %s", got, want)
	}
	first := ShieldedV6Output{}
	first.OneTimeKey = shieldedV6TestKey(t, 1)
	first.Commitment = shieldedV6TestCommitment(t, 1, 1)
	firstIndex := st.Snapshot()
	if err := shieldedV6AppendOutput(st, first, common.Hash{1}); err != nil {
		t.Fatalf("append first Shield6 output: %v", err)
	}
	if ShieldedV6OutputCount(st) != 1 {
		t.Fatalf("Shield6 output count = %d, want 1", ShieldedV6OutputCount(st))
	}
	if got, err := shieldedV6ReadOutput(st, 0); err != nil || got != (shielded6.RingMember{OneTimeKey: first.OneTimeKey, Commitment: first.Commitment}) {
		t.Fatalf("read first Shield6 output: member=%x/%x err=%v", got.OneTimeKey, got.Commitment, err)
	}
	firstRoot := ShieldedV6OutputRoot(st)
	if firstRoot == (common.Hash{}) || firstRoot == shieldedV6EmptyNodes()[ShieldedV6OutputTreeDepth] {
		t.Fatal("Shield6 accumulator root did not advance")
	}
	if err := shieldedV6AppendOutput(st, first, common.Hash{2}); err == nil {
		t.Fatal("duplicate one-time key was appended")
	}
	second := ShieldedV6Output{}
	second.OneTimeKey = shieldedV6TestKey(t, 2)
	second.Commitment = shieldedV6TestCommitment(t, 2, 1)
	if err := shieldedV6AppendOutput(st, second, common.Hash{3}); err != nil {
		t.Fatalf("append second Shield6 output: %v", err)
	}
	if ShieldedV6OutputCount(st) != 2 || ShieldedV6OutputRoot(st) == firstRoot {
		t.Fatal("second Shield6 leaf did not update count and root")
	}
	st.RevertToSnapshot(firstIndex)
	if ShieldedV6OutputCount(st) != 0 || ShieldedV6OutputRoot(st) != shieldedV6EmptyNodes()[ShieldedV6OutputTreeDepth] {
		t.Fatal("Shield6 accumulator did not revert with the enclosing state snapshot")
	}
}

func TestShieldedV6AccumulatorRootMatchesSparseMerkleRecomputation(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	leaves := make([]common.Hash, 0, 40)
	zeroes := shieldedV6EmptyNodes()
	checkpoints := map[uint64]struct{}{
		1: {}, 2: {}, 3: {}, 4: {}, 5: {}, 7: {}, 8: {},
		15: {}, 16: {}, 17: {}, 31: {}, 32: {}, 33: {}, 39: {}, 40: {},
	}
	for i := uint64(0); i < 40; i++ {
		output := ShieldedV6Output{
			OneTimeKey: shieldedV6TestKey(t, byte(i+1)),
			Commitment: shieldedV6TestCommitment(t, i+1, byte(i+1)),
		}
		if err := shieldedV6AppendOutput(st, output, common.Hash{byte(i + 1)}); err != nil {
			t.Fatalf("append output %d: %v", i, err)
		}
		var index [8]byte
		binary.BigEndian.PutUint64(index[:], i)
		leaves = append(leaves, crypto.Keccak256Hash(
			[]byte("TKM_SHIELD6_ACCUMULATOR_LEAF_V1"), index[:], output.OneTimeKey[:], output.Commitment[:],
		))
		if _, check := checkpoints[i+1]; !check {
			continue
		}
		level := append([]common.Hash(nil), leaves...)
		for depth := uint32(0); depth < ShieldedV6OutputTreeDepth; depth++ {
			next := make([]common.Hash, (len(level)+1)/2)
			for parent := range next {
				left := level[parent*2]
				right := zeroes[depth]
				if parent*2+1 < len(level) {
					right = level[parent*2+1]
				}
				next[parent] = crypto.Keccak256Hash([]byte("TKM_SHIELD6_ACCUMULATOR_NODE_V1"), left[:], right[:])
			}
			level = next
		}
		if len(level) != 1 || ShieldedV6OutputRoot(st) != level[0] {
			t.Fatalf("accumulator root mismatch at output count %d: got %s, want %s", i+1, ShieldedV6OutputRoot(st), level[0])
		}
	}
}

func TestShieldedV6PublicKeyImageIndexIsAppendOnlyAndRevertible(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := shielded6.CreateWallet(nil, 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	image, err := shielded6.DeriveKeyImage(keys.SpendPublic[:], keys.SpendSecret[:])
	if err != nil {
		t.Fatal(err)
	}
	snapshot := st.Snapshot()
	if err := shieldedV6AppendKeyImage(st, image, common.Hash{1}); err != nil {
		t.Fatalf("append canonical key image: %v", err)
	}
	if ShieldedV6KeyImageCount(st) != 1 {
		t.Fatalf("Shield6 key-image count = %d, want 1", ShieldedV6KeyImageCount(st))
	}
	if got, err := ShieldedV6KeyImageAt(st, 0); err != nil || got != image {
		t.Fatalf("Shield6 public key-image index = %x, err=%v; want %x", got, err, image)
	}
	if err := shieldedV6AppendKeyImage(st, image, common.Hash{2}); err == nil {
		t.Fatal("duplicate Shield6 key image was appended")
	}
	if ShieldedV6KeyImageCount(st) != 1 {
		t.Fatal("duplicate key-image rejection changed the public index")
	}
	st.RevertToSnapshot(snapshot)
	if ShieldedV6KeyImageCount(st) != 0 {
		t.Fatal("Shield6 public key-image index did not revert with state")
	}
}

func shieldedV6TestCommitment(t *testing.T, amount uint64, maskByte byte) [shielded6.PointSize]byte {
	t.Helper()
	var mask [shielded6.ScalarSize]byte
	mask[0] = maskByte
	commitment, err := shielded6.Commitment128(new(big.Int).SetUint64(amount), mask)
	if err != nil {
		t.Fatal(err)
	}
	return commitment
}

func TestShieldedV6AccumulatorAppendPreflightsFrontier(t *testing.T) {
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	for i := byte(1); i <= 6; i++ {
		output := ShieldedV6Output{
			OneTimeKey: shieldedV6TestKey(t, i),
			Commitment: shieldedV6TestCommitment(t, uint64(i), 1),
		}
		if err := shieldedV6AppendOutput(st, output, common.Hash{i}); err != nil {
			t.Fatalf("append output %d: %v", i, err)
		}
	}
	var lowLevel [4]byte
	var missingLevel [4]byte
	binary.BigEndian.PutUint32(missingLevel[:], 2)
	lowSlot := ShieldedV6StateSlot("accumulator-frontier", lowLevel[:])
	missingSlot := ShieldedV6StateSlot("accumulator-frontier", missingLevel[:])
	lowBefore := st.GetState(params.ShieldedPoolAddress, lowSlot)
	st.SetState(params.ShieldedPoolAddress, missingSlot, common.Hash{})
	countBefore := ShieldedV6OutputCount(st)
	rootBefore := ShieldedV6OutputRoot(st)
	output := ShieldedV6Output{
		OneTimeKey: shieldedV6TestKey(t, 7),
		Commitment: shieldedV6TestCommitment(t, 7, 1),
	}
	if err := shieldedV6AppendOutput(st, output, common.Hash{7}); err == nil {
		t.Fatal("append succeeded with a missing frontier node")
	}
	if got := st.GetState(params.ShieldedPoolAddress, lowSlot); got != lowBefore {
		t.Fatal("failed accumulator append modified a lower frontier node")
	}
	if ShieldedV6OutputCount(st) != countBefore || ShieldedV6OutputRoot(st) != rootBefore {
		t.Fatal("failed accumulator append changed count or root")
	}
}

func shieldedV6TestKey(t *testing.T, n byte) [shielded6.PointSize]byte {
	t.Helper()
	var raw [curve25519.PrivateKeySize]byte
	raw[0] = n
	scalar, err := new(curve25519.Scalar).SetCanonicalBytes(raw[:])
	if err != nil {
		t.Fatal(err)
	}
	point := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(scalar)
	var encoded [shielded6.PointSize]byte
	copy(encoded[:], point.Bytes())
	return encoded
}

func TestShieldedV6StateValidatesRingAgainstAccumulatorAndRejectsReplay(t *testing.T) {
	rng := monerocrypto.NewDeterministicTestGenerator()
	var signingInput shielded6.SigningInput128
	signingInput.SignerIndex = 7
	signingInput.Amount = new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 80), big.NewInt(1000))
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	for i := range shielded6.RingSize {
		var spend, mask curve25519.Scalar
		curve25519.RandomScalar(&spend, rng)
		curve25519.RandomScalar(&mask, rng)
		amount := new(big.Int).Add(signingInput.Amount, big.NewInt(int64(i+1)))
		if i == int(signingInput.SignerIndex) {
			amount = new(big.Int).Set(signingInput.Amount)
			copy(signingInput.SpendSecret[:], spend.Bytes())
			copy(signingInput.CommitmentMask[:], mask.Bytes())
		}
		pub := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
		commitment, err := shielded6.Commitment128(amount, func() [shielded6.ScalarSize]byte {
			var encoded [shielded6.ScalarSize]byte
			copy(encoded[:], mask.Bytes())
			return encoded
		}())
		if err != nil {
			t.Fatalf("construct ring commitment %d: %v", i, err)
		}
		copy(signingInput.Ring[i].OneTimeKey[:], pub.Bytes())
		signingInput.Ring[i].Commitment = commitment
		leaf := ShieldedV6Output{OneTimeKey: signingInput.Ring[i].OneTimeKey, Commitment: signingInput.Ring[i].Commitment}
		if err := shieldedV6AppendOutput(st, leaf, common.Hash{byte(i + 1)}); err != nil {
			t.Fatalf("seed accumulator output %d: %v", i, err)
		}
	}
	wideCommitments, rangeProof, outputWitnesses, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{signingInput.Amount}, rng)
	if err != nil {
		t.Fatal(err)
	}
	recipientSeed := make([]byte, 32)
	for i := range recipientSeed {
		recipientSeed[i] = byte(i + 33)
	}
	recipient, err := shielded6.DeriveWalletKeys(recipientSeed, 8979)
	if err != nil {
		t.Fatalf("derive Shield6 recipient: %v", err)
	}
	defer recipient.Clear()
	walletOutput, err := shielded6.CreateOutput(recipient.Address(), 0, 1, signingInput.Amount, outputWitnesses[0], rng)
	if err != nil {
		t.Fatalf("create Shield6 wallet output: %v", err)
	}
	output := ShieldedV6OutputFromWallet(walletOutput)
	if output.Commitment != wideCommitments[0].Commitment || output.AmountLimbs != wideCommitments[0].Limbs {
		t.Fatal("wallet output commitment differs from the range-proof commitments")
	}
	envelope := &ShieldedV6Transaction{Version: 6, AssetID: 1, Inputs: make([]ShieldedV6Input, 1), Outputs: []ShieldedV6Output{output}, RangeProof: rangeProof}
	for i := range envelope.Inputs[0].RingIndices {
		envelope.Inputs[0].RingIndices[i] = uint64(i)
	}
	var sumMasks [shielded6.ScalarSize]byte
	sumMasks = outputWitnesses[0].Mask
	to := params.ShieldedPoolAddress
	unsignedData, err := EncodeShieldedV6Transaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	makeTx := func(data []byte) *types.Transaction {
		return types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &to, Value: new(big.Int), Gas: 8_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87", PublicKey: []byte{1}, Data: data})
	}
	tx := makeTx(unsignedData)
	intent, err := ShieldedV6Intent(tx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := shielded6.SignCLSAGs128(intent, []shielded6.SigningInput128{signingInput}, sumMasks, rng)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Inputs[0].KeyImage = signed[0].KeyImage
	envelope.Inputs[0].PseudoOutput = signed[0].PseudoOutput
	envelope.Inputs[0].Signature = signed[0].Signature
	data, err := EncodeShieldedV6Transaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	tx = makeTx(data)
	if err := ValidateShieldedV6Proof(tx); err != nil {
		t.Fatalf("stateless Shield6 proof validation: %v", err)
	}
	opening, owned, err := shielded6.ScanOutput(recipient, 0, ShieldedV6OutputCount(st), ShieldedV6WalletRecord(envelope.Outputs[0]))
	if err != nil || !owned || opening == nil || opening.Amount.Cmp(signingInput.Amount) != 0 {
		t.Fatalf("scan output carried by consensus envelope: note=%v owned=%v err=%v", opening, owned, err)
	}
	opening.Clear()
	if err := validateShieldedV6State(st, tx, envelope); err != nil {
		t.Fatalf("stateful Shield6 CLSAG validation: %v", err)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV6StateSlot("key-image", signed[0].KeyImage[:]), tx.Hash())
	if err := validateShieldedV6State(st, tx, envelope); err == nil {
		t.Fatal("Shield6 replay with an already-spent key image was accepted")
	}
}

func TestBuildShieldedV6EnvelopeFromScannedNote(t *testing.T) {
	rng := monerocrypto.NewDeterministicTestGenerator()
	const signerIndex = 4
	// Exercise both the private transfer and public withdrawal builders above
	// Shield3's 5,000,000 TKM ceiling. Shield6 retains its 128-bit bound.
	amount := new(big.Int).Add(shielded3.MaxSendWei(), big.NewInt(1))
	var seedA, seedB [32]byte
	for i := range seedA {
		seedA[i], seedB[i] = byte(i+1), byte(255-i)
	}
	sender, err := shielded6.DeriveWalletKeys(seedA[:], 8979)
	if err != nil {
		t.Fatalf("derive sender: %v", err)
	}
	defer sender.Clear()
	recipient, err := shielded6.DeriveWalletKeys(seedB[:], 8979)
	if err != nil {
		t.Fatalf("derive recipient: %v", err)
	}
	defer recipient.Clear()
	_, _, sourceWitnesses, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, rng)
	if err != nil {
		t.Fatal(err)
	}
	sourceOutput, err := shielded6.CreateOutput(sender.Address(), 0, shielded6.AssetTKM, amount, sourceWitnesses[0], rng)
	if err != nil {
		t.Fatalf("create source note: %v", err)
	}
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	var ring [shielded6.RingSize]shielded6.RingMember
	for i := range shielded6.RingSize {
		if i == signerIndex {
			ring[i] = shielded6.RingMember{OneTimeKey: sourceOutput.OneTimeKey, Commitment: sourceOutput.Commitment}
		} else {
			var spend, mask curve25519.Scalar
			curve25519.RandomScalar(&spend, rng)
			curve25519.RandomScalar(&mask, rng)
			key := new(curve25519.PublicKey[curve25519.VarTimeOperations]).ScalarBaseMult(&spend)
			commitment, err := shielded6.Commitment128(new(big.Int).Add(amount, big.NewInt(int64(i+1))), func() [shielded6.ScalarSize]byte {
				var encoded [shielded6.ScalarSize]byte
				copy(encoded[:], mask.Bytes())
				return encoded
			}())
			if err != nil {
				t.Fatalf("create decoy commitment %d: %v", i, err)
			}
			copy(ring[i].OneTimeKey[:], key.Bytes())
			ring[i].Commitment = commitment
		}
		leaf := ShieldedV6Output{OneTimeKey: ring[i].OneTimeKey, Commitment: ring[i].Commitment}
		if err := shieldedV6AppendOutput(st, leaf, common.Hash{byte(i + 1)}); err != nil {
			t.Fatalf("append ring leaf %d: %v", i, err)
		}
	}
	note, found, err := shielded6.ScanOutput(sender, 0, signerIndex, sourceOutput)
	if err != nil || !found || note == nil {
		t.Fatalf("scan source note: note=%v found=%v err=%v", note, found, err)
	}
	defer note.Clear()
	var input ShieldedV6WalletInput
	input.Note, input.Ring, input.SignerIndex = note, ring, signerIndex
	for i := range input.RingIndices {
		input.RingIndices[i] = uint64(i)
	}
	// Transaction ring order is randomized independently of accumulator age;
	// the real member must not retain a rank-correlated position.
	for i := shielded6.RingSize - 1; i > 0; i-- {
		selected, err := cryptorand.Int(rng, big.NewInt(int64(i+1)))
		if err != nil {
			t.Fatal(err)
		}
		j := int(selected.Int64())
		input.Ring[i], input.Ring[j] = input.Ring[j], input.Ring[i]
		input.RingIndices[i], input.RingIndices[j] = input.RingIndices[j], input.RingIndices[i]
		if int(input.SignerIndex) == i {
			input.SignerIndex = uint64(j)
		} else if int(input.SignerIndex) == j {
			input.SignerIndex = uint64(i)
		}
	}
	orderedByAge := true
	for i := 1; i < shielded6.RingSize; i++ {
		if input.RingIndices[i] <= input.RingIndices[i-1] {
			orderedByAge = false
			break
		}
	}
	if orderedByAge {
		t.Fatal("test ring order unexpectedly remained age-sorted")
	}
	to := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1},
	})
	data, err := BuildShieldedV6Envelope(tx, []ShieldedV6WalletInput{input}, []ShieldedV6WalletOutput{{Address: recipient.Address(), Amount: amount}}, rng)
	if err != nil {
		t.Fatalf("build Shield6 wallet envelope: %v", err)
	}
	e, recognized, err := DecodeShieldedV6Transaction(data)
	if err != nil || !recognized || e == nil {
		t.Fatalf("decode wallet-built envelope: recognized=%v err=%v", recognized, err)
	}
	tx = types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1}, Data: data,
	})
	active := *params.MainnetChainConfig
	active.AntarticalTime = new(uint64)
	active.PrivacyCommitmentTime = new(uint64)
	active.ConfidentialLedgerTime = new(uint64)
	active.Shield6Time = new(uint64)
	active.ShieldedOnlyTime = nil
	if _, err := shieldedV6Basics(&active, big.NewInt(1), 1, tx); err != nil {
		t.Fatalf("consensus rejected a randomized ring order: %v", err)
	}
	duplicateRing := *e
	duplicateRing.Inputs = append([]ShieldedV6Input(nil), e.Inputs...)
	duplicateRing.Inputs[0].RingIndices[1] = duplicateRing.Inputs[0].RingIndices[0]
	duplicateData, err := EncodeShieldedV6Transaction(&duplicateRing)
	if err != nil {
		t.Fatal(err)
	}
	duplicateTx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1}, Data: duplicateData,
	})
	if _, err := shieldedV6Basics(&active, big.NewInt(1), 1, duplicateTx); err == nil {
		t.Fatal("consensus accepted duplicate Shield6 accumulator indexes")
	}
	if err := ValidateShieldedV6Proof(tx); err != nil {
		t.Fatalf("verify wallet-built Shield6 transaction: %v", err)
	}
	if err := validateShieldedV6State(st, tx, e); err != nil {
		t.Fatalf("statefully verify wallet-built Shield6 transaction: %v", err)
	}
	// A full public exit is a supported migration boundary. Its amount and
	// destination are explicit, and the same canonical ring verifier applies.
	withdrawalTo := common.Address{0x42}
	withdrawalTx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1},
	})
	withdrawalData, err := BuildShieldedV6WithdrawalEnvelope(withdrawalTx, []ShieldedV6WalletInput{input}, nil, withdrawalTo, amount, rng)
	if err != nil {
		t.Fatalf("build full Shield6 exit: %v", err)
	}
	withdrawalTx = types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1}, Data: withdrawalData,
	})
	withdrawalEnvelope, ok, err := DecodeShieldedV6Transaction(withdrawalData)
	if err != nil || !ok || withdrawalEnvelope.WithdrawalRecipient != withdrawalTo || len(withdrawalEnvelope.Outputs) != 0 {
		t.Fatalf("decode full Shield6 exit: ok=%v err=%v envelope=%+v", ok, err, withdrawalEnvelope)
	}
	if err := ValidateShieldedV6Proof(withdrawalTx); err != nil {
		t.Fatalf("verify full Shield6 exit: %v", err)
	}
	if err := validateShieldedV6State(st, withdrawalTx, withdrawalEnvelope); err != nil {
		t.Fatalf("statefully verify full Shield6 exit: %v", err)
	}
	if _, err := shieldedV6Basics(&active, big.NewInt(1), 1, withdrawalTx); err != nil {
		t.Fatalf("validate Shield6 withdrawal above the legacy Shield3 limit: %v", err)
	}
}

func TestBuildShieldedV6PublicDepositEnvelope(t *testing.T) {
	rng := monerocrypto.NewDeterministicTestGenerator()
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 19)
	}
	wallet, err := shielded6.DeriveWalletKeys(seed[:], 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer wallet.Clear()
	// Shield6 has 128-bit confidential amounts and must not inherit Shield3's
	// historical 5,000,000 TKM transfer ceiling.
	amount := new(big.Int).Add(shielded3.MaxSendWei(), big.NewInt(1))
	to := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int).Set(amount), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1},
	})
	data, err := BuildShieldedV6DepositEnvelope(tx, []ShieldedV6WalletOutput{{Address: wallet.Address(), Amount: amount}}, rng)
	if err != nil {
		t.Fatalf("build Shield6 deposit: %v", err)
	}
	tx = types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int).Set(amount), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1}, Data: data,
	})
	e, ok, err := DecodeShieldedV6Transaction(data)
	if err != nil || !ok || e == nil || !e.Deposit || len(e.Inputs) != 0 || len(e.Outputs) != 1 {
		t.Fatalf("decode Shield6 deposit: ok=%v err=%v envelope=%+v", ok, err, e)
	}
	if err := ValidateShieldedV6Proof(tx); err != nil {
		t.Fatalf("verify Shield6 deposit proof: %v", err)
	}
	if _, err := shieldedOnlyFields(tx.Type(), tx.To(), tx.Value(), tx.Data()); err != nil {
		t.Fatalf("shielded-only policy rejected explicit Shield6 deposit: %v", err)
	}
	active := *params.MainnetChainConfig
	active.AntarticalTime = new(uint64)
	active.PrivacyCommitmentTime = new(uint64)
	active.Shield6Time = new(uint64)
	active.ConfidentialLedgerTime = new(uint64)
	active.ShieldedOnlyTime = nil
	if _, err := shieldedV6Basics(&active, big.NewInt(1), 1, tx); err != nil {
		t.Fatalf("validate Shield6 deposit above the legacy Shield3 limit: %v", err)
	}
	tooLarge := new(big.Int).Lsh(big.NewInt(1), 128)
	tooLargeTx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: tooLarge, Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87", PublicKey: []byte{1},
	})
	if _, err := BuildShieldedV6DepositEnvelope(tooLargeTx, []ShieldedV6WalletOutput{{Address: wallet.Address(), Amount: tooLarge}}, rng); err == nil {
		t.Fatal("Shield6 accepted a deposit exceeding its 128-bit amount encoding")
	}
	withoutLedger := active
	withoutLedger.ConfidentialLedgerTime = nil
	if _, err := shieldedV6Basics(&withoutLedger, big.NewInt(1), 1, tx); err == nil {
		t.Fatal("Shield6 public deposit was accepted without the confidential backing ledger")
	}
	bad := *e
	bad.DepositMask[0] ^= 1
	badData, err := EncodeShieldedV6Transaction(&bad)
	if err != nil {
		t.Fatal(err)
	}
	badTx := types.NewTx(&types.PQTkmTx{
		ChainID: big.NewInt(8979), To: &to, Value: new(big.Int).Set(amount), Gas: 8_000_000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), Algorithm: "ML-DSA-87",
		PublicKey: []byte{1}, Data: badData,
	})
	if err := ValidateShieldedV6Proof(badTx); err == nil {
		t.Fatal("Shield6 deposit with an altered aggregate mask was accepted")
	}
}

func TestShield6LegacyMigrationAllowsOnlySelfExitAfterActivation(t *testing.T) {
	owner := common.Address{0x21}
	pool := params.ShieldedPoolAddress
	for _, version := range []uint64{3, 4, 5} {
		t.Run(fmt.Sprintf("shield%d", version), func(t *testing.T) {
			encode := func(deposit bool) ([]byte, error) {
				switch version {
				case 3:
					return EncodeShieldedV3Transaction(&ShieldedV3Transaction{Version: 3, Deposit: deposit, WithdrawalRecipient: owner, WithdrawalValue: big.NewInt(7), GasSponsorValue: new(big.Int)})
				case 4:
					return EncodeShieldedV4Transaction(&ShieldedV4Transaction{Version: 4, Deposit: deposit, WithdrawalRecipient: owner, WithdrawalValue: big.NewInt(7), GasSponsorValue: new(big.Int)})
				default:
					return EncodeShieldedV5Transaction(&ShieldedV5Transaction{Version: 5, Deposit: deposit, WithdrawalRecipient: owner, WithdrawalValue: big.NewInt(7), GasSponsorValue: new(big.Int)})
				}
			}
			data, err := encode(false)
			if err != nil {
				t.Fatal(err)
			}
			tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &pool, Value: new(big.Int), Data: data})
			if !isShieldedLegacySelfExit(tx, owner) {
				t.Fatalf("Shield%d owner-directed withdrawal was not recognized as a migration exit", version)
			}
			if isShieldedLegacySelfExit(tx, common.Address{0x22}) {
				t.Fatalf("Shield%d withdrawal to a different address was accepted as self-exit", version)
			}
			data, err = encode(true)
			if err != nil {
				t.Fatal(err)
			}
			tx = types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &pool, Value: new(big.Int), Data: data})
			if isShieldedLegacySelfExit(tx, owner) {
				t.Fatalf("Shield%d legacy deposit was accepted as a migration exit", version)
			}
		})
	}
}

func TestShieldedV6ForkGateAndOrdering(t *testing.T) {
	activation := params.MainnetAntarticalTime + 7200
	config := *params.MainnetChainConfig
	config.Shield6Time = new(uint64)
	*config.Shield6Time = activation
	if config.IsShield6(big.NewInt(0), activation-1) {
		t.Fatal("Shield6 active before configured activation")
	}
	if !config.IsShield6(big.NewInt(0), activation) {
		t.Fatal("Shield6 inactive at configured activation")
	}
	if err := config.CheckConfigForkOrder(); err != nil {
		t.Fatalf("valid Shield6 fork ordering rejected: %v", err)
	}
	*config.Shield6Time = params.MainnetAntarticalTime - 1
	if err := config.CheckConfigForkOrder(); err == nil {
		t.Fatal("Shield6 scheduled before Antartical was accepted")
	}
}

func TestShieldedV6RejectsBeforeActivation(t *testing.T) {
	config := *params.MainnetChainConfig
	data, err := EncodeShieldedV6Transaction(&ShieldedV6Transaction{Version: 6, AssetID: 1})
	if err != nil {
		t.Fatal(err)
	}
	to := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{ChainID: config.ChainID, To: &to, Data: data, Value: new(big.Int)})
	if err := ValidateShieldedTransactionBasics(&config, big.NewInt(1), params.MainnetAntarticalTime+1, tx); err == nil {
		t.Fatal("pre-activation Shield6 transaction was accepted")
	}
}
