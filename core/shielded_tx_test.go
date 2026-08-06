package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	shieldedcircuit "github.com/ethereum/go-ethereum/zk/shielded"
)

type testShieldedVerifier struct {
	err error
}

func (v testShieldedVerifier) VerifyShieldedSpend(ctx ShieldedProofContext, proof []byte) error {
	if v.err != nil {
		return v.err
	}
	if ctx.Nullifier == (common.Hash{}) || len(proof) == 0 {
		return ErrInvalidShieldedTx
	}
	return nil
}

func testShieldedEnvelope(t *testing.T, spends int) *ShieldedTransaction {
	t.Helper()
	tx := &ShieldedTransaction{
		Version:           shieldedTxVersion,
		BalanceCommitment: common.HexToHash("0xabc"),
		BindingSig:        common.BigToHash(big.NewInt(9)).Bytes(),
	}
	for i := 0; i < shieldedOutputSlots; i++ {
		tx.Outputs = append(tx.Outputs, ShieldedOutput{
			Commitment:       common.BigToHash(big.NewInt(int64(i + 1))),
			PayloadHash:      common.BigToHash(big.NewInt(int64(i + 100))),
			EncryptedPayload: make([]byte, shieldedMinEncryptedOutputBytes),
			Nonce:            make([]byte, shieldedMinNonceBytes),
		})
	}
	for i := 0; i < spends; i++ {
		tx.Spends = append(tx.Spends, ShieldedSpend{
			Nullifier:          common.BigToHash(big.NewInt(int64(i + 10))),
			Anchor:             common.HexToHash("0x3"),
			Proof:              []byte("proof"),
			EncryptedSpendData: []byte("spend"),
		})
	}
	return tx
}

func testShieldedTx(t *testing.T, envelope *ShieldedTransaction, value *big.Int) *types.Transaction {
	t.Helper()
	data, err := EncodeShieldedTransaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return types.NewTx(&types.LegacyTx{
		To:       &params.ShieldedPoolAddress,
		Value:    value,
		Gas:      100000,
		GasPrice: big.NewInt(1),
		Data:     data,
	})
}

func TestShieldedEnvelopeRoundTrip(t *testing.T) {
	want := testShieldedEnvelope(t, 1)
	data, err := EncodeShieldedTransaction(want)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := DecodeShieldedTransaction(data)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("shielded envelope not detected")
	}
	if got.Version != want.Version || len(got.Spends) != 1 || len(got.Outputs) != shieldedOutputSlots {
		t.Fatalf("decoded shielded envelope = %+v", got)
	}
}

func TestProcessShieldedSpendRequiresVerifier(t *testing.T) {
	SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	tx := testShieldedTx(t, testShieldedEnvelope(t, 1), new(big.Int))
	err = processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{}))
	if !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("processShieldedTransaction error = %v, want invalid shielded tx", err)
	}
	if !errors.Is(err, ErrShieldedVerifierUnavailable) {
		t.Fatalf("processShieldedTransaction error = %v, want verifier unavailable", err)
	}
}

func TestProcessTransparentTransactionPrivacyFork(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	to := common.HexToAddress("0x100")
	tx := types.NewTx(&types.LegacyTx{
		To:       &to,
		Value:    big.NewInt(1),
		Gas:      21000,
		GasPrice: big.NewInt(1),
	})
	if err := processShieldedTransaction(params.TestChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err != nil {
		t.Fatalf("transparent transaction rejected before privacy fork: %v", err)
	}
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("transparent transaction error = %v, want invalid shielded tx", err)
	}
}

func TestShieldedGroth16VerifierFormats(t *testing.T) {
	vkBytes, proofBytes := testShieldedGroth16Material(t)
	verifier, err := NewShieldedGroth16Verifier(vkBytes)
	if err != nil {
		t.Fatal(err)
	}
	err = verifier.VerifyShieldedSpend(ShieldedProofContext{
		ChainID:           big.NewInt(1),
		BlockNumber:       big.NewInt(2),
		TxHash:            common.HexToHash("0x01"),
		Nullifier:         common.HexToHash("0x02"),
		Anchor:            common.HexToHash("0x03"),
		BalanceCommitment: common.HexToHash("0x04"),
		PublicValue:       new(big.Int),
		OutputCommitments: []common.Hash{common.HexToHash("0x05"), common.HexToHash("0x06"), common.HexToHash("0x07"), common.HexToHash("0x08")},
		BindingSig:        common.BigToHash(big.NewInt(9)).Bytes(),
	}, proofBytes)
	if err == nil {
		t.Fatal("invalid Groth16 proof accepted")
	}
	if !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("Groth16 verifier error = %v, want invalid shielded tx", err)
	}
}

func TestShieldedProofPublicInputsMatchCircuitVector(t *testing.T) {
	vector := shieldedcircuit.DeterministicTestVectors().Valid
	outputs := make([]common.Hash, 0, len(vector.Private.OutputCommitment))
	for _, commitment := range vector.Private.OutputCommitment {
		outputs = append(outputs, hashFromDecimalString(t, commitment))
	}
	inputs := shieldedProofPublicInputs(ShieldedProofContext{
		ChainID:           bigFromDecimalString(t, vector.Public.ChainID),
		BlockNumber:       bigFromDecimalString(t, vector.Public.BlockNumber),
		TxHash:            hashFromDecimalLimbs(t, vector.Public.TxHashHi, vector.Public.TxHashLo),
		SpendIndex:        int(bigFromDecimalString(t, vector.Public.SpendIndex).Int64()),
		Nullifier:         hashFromDecimalString(t, vector.Public.Nullifier),
		Anchor:            hashFromDecimalString(t, vector.Public.Anchor),
		BalanceCommitment: hashFromDecimalString(t, vector.Public.BalanceCommitment),
		PublicValue:       bigFromDecimalString(t, vector.Public.PublicValue),
		OutputCommitments: outputs,
		BindingSig:        hashFromDecimalString(t, vector.Public.BindingSigHash).Bytes(),
	})
	want := []string{
		vector.Public.ChainID,
		vector.Public.BlockNumber,
		vector.Public.TxHashHi,
		vector.Public.TxHashLo,
		vector.Public.SpendIndex,
		vector.Public.Nullifier,
		vector.Public.Anchor,
		vector.Public.BalanceCommitment,
		vector.Public.PublicValue,
		vector.Public.OutputRoot,
		vector.Public.BindingSigHash,
	}
	for i, input := range inputs {
		if got := input.BigInt(new(big.Int)).String(); got != want[i] {
			t.Fatalf("public input %d = %s, want %s", i, got, want[i])
		}
	}
}

func bigFromDecimalString(t *testing.T, input string) *big.Int {
	t.Helper()
	out, ok := new(big.Int).SetString(input, 10)
	if !ok {
		t.Fatalf("invalid decimal %q", input)
	}
	return out
}

func hashFromDecimalString(t *testing.T, input string) common.Hash {
	t.Helper()
	return common.BigToHash(bigFromDecimalString(t, input))
}

func hashFromDecimalLimbs(t *testing.T, hi, lo string) common.Hash {
	t.Helper()
	hiValue := bigFromDecimalString(t, hi)
	loValue := bigFromDecimalString(t, lo)
	if hiValue.BitLen() > 128 || loValue.BitLen() > 128 {
		t.Fatalf("hash limb too large: hi=%s lo=%s", hi, lo)
	}
	out := new(big.Int).Lsh(hiValue, 128)
	out.Or(out, loValue)
	return common.BigToHash(out)
}

func testShieldedGroth16Material(t *testing.T) ([]byte, []byte) {
	t.Helper()
	_, _, g1, g2 := bn254.Generators()
	g1Bytes := g1.Bytes()
	g2Bytes := g2.Bytes()
	ic := make([][]byte, shieldedPublicInputs+1)
	for i := range ic {
		ic[i] = g1Bytes[:]
	}
	vkBytes, err := EncodeShieldedGroth16VerifyingKey(ShieldedGroth16VerifyingKey{
		AlphaG1: g1Bytes[:],
		BetaG2:  g2Bytes[:],
		GammaG2: g2Bytes[:],
		DeltaG2: g2Bytes[:],
		IC:      ic,
	})
	if err != nil {
		t.Fatal(err)
	}
	proofBytes, err := EncodeShieldedGroth16Proof(ShieldedGroth16Proof{
		A: g1Bytes[:],
		B: g2Bytes[:],
		C: g1Bytes[:],
	})
	if err != nil {
		t.Fatal(err)
	}
	return vkBytes, proofBytes
}

func TestShieldedGroth16VerifierRejectsInvalidEncoding(t *testing.T) {
	if _, err := NewShieldedGroth16Verifier([]byte("bad")); !errors.Is(err, ErrShieldedVerifyingKeyUnavailable) {
		t.Fatalf("NewShieldedGroth16Verifier error = %v, want verifying key unavailable", err)
	}
	if _, err := decodeShieldedGroth16Proof([]byte("bad")); !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("decodeShieldedGroth16Proof error = %v, want invalid shielded tx", err)
	}
}

func TestProcessShieldedSpendUsesConfiguredGroth16Verifier(t *testing.T) {
	SetShieldedProofVerifier(nil)
	vkBytes, proofBytes := testShieldedGroth16Material(t)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	config := *params.EgyptChainConfig
	config.ShieldedGroth16VerifyingKey = vkBytes
	envelope := testShieldedEnvelope(t, 1)
	envelope.Spends[0].Proof = proofBytes
	tx := testShieldedTx(t, envelope, new(big.Int))
	err = processShieldedTransaction(&config, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{}))
	if err == nil {
		t.Fatal("invalid configured Groth16 proof accepted")
	}
	if errors.Is(err, ErrShieldedVerifierUnavailable) || errors.Is(err, ErrShieldedVerifyingKeyUnavailable) {
		t.Fatalf("configured Groth16 verifier was not used: %v", err)
	}
	if !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("processShieldedTransaction error = %v, want invalid shielded tx", err)
	}
}

func TestProcessShieldedSpendStoresNullifierAndCommitment(t *testing.T) {
	SetShieldedProofVerifier(testShieldedVerifier{})
	defer SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 1)
	tx := testShieldedTx(t, envelope, new(big.Int))
	err = processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := statedb.GetState(params.ShieldedPoolAddress, shieldedNullifierSlot(envelope.Spends[0].Nullifier)); got != tx.Hash() {
		t.Fatalf("nullifier state = %s, want %s", got, tx.Hash())
	}
	if got := statedb.GetState(params.ShieldedPoolAddress, shieldedCommitmentSlot(envelope.Outputs[0].Commitment)); got != envelope.Outputs[0].PayloadHash {
		t.Fatalf("commitment state = %s, want %s", got, envelope.Outputs[0].PayloadHash)
	}
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("duplicate nullifier accepted")
	}
}

func TestProcessShieldedTxRejectsTransparentValue(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	tx := testShieldedTx(t, testShieldedEnvelope(t, 0), new(big.Int))
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("output-only shielded transaction accepted")
	}
	tx = testShieldedTx(t, testShieldedEnvelope(t, 0), big.NewInt(1))
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("shielded transaction with transparent value accepted")
	}
}

func TestValidateShieldedEnvelopeRejectsNonCanonicalFieldInputs(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	nonCanonical := common.Hash{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	}
	envelope.Spends[0].Nullifier = nonCanonical
	if err := validateShieldedEnvelope(envelope); err == nil {
		t.Fatal("non-canonical nullifier accepted")
	}

	envelope = testShieldedEnvelope(t, 1)
	envelope.Outputs[0].Commitment = nonCanonical
	if err := validateShieldedEnvelope(envelope); err == nil {
		t.Fatal("non-canonical output commitment accepted")
	}

	envelope = testShieldedEnvelope(t, 1)
	envelope.BindingSig = []byte("short")
	if err := validateShieldedEnvelope(envelope); err == nil {
		t.Fatal("short binding hash accepted")
	}
}

func TestValidateShieldedEnvelopeRequiresPaddedOutputs(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	envelope.Outputs = envelope.Outputs[:1]
	if err := validateShieldedEnvelope(envelope); err == nil {
		t.Fatal("unpadded output set accepted")
	}
}
