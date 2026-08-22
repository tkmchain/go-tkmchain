package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	shieldedcircuit "github.com/ethereum/go-ethereum/zk/shielded"
	"github.com/holiman/uint256"
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

type shieldedVerifierFunc func(ctx ShieldedProofContext, proof []byte) error

func (f shieldedVerifierFunc) VerifyShieldedSpend(ctx ShieldedProofContext, proof []byte) error {
	return f(ctx, proof)
}

func testShieldedEnvelope(t *testing.T, spends int) *ShieldedTransaction {
	t.Helper()
	tx := &ShieldedTransaction{
		Version:           ShieldedTxVersionV1,
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

func testShieldedPQTkmTx(t *testing.T, envelope *ShieldedTransaction, algorithm string, publicKey, signature []byte) *types.Transaction {
	t.Helper()
	data, err := EncodeShieldedTransaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return types.NewTx(&types.PQTkmTx{
		ChainID:    big.NewInt(8979),
		To:         &params.ShieldedPoolAddress,
		Value:      new(big.Int),
		Gas:        100000,
		GasFeeCap:  big.NewInt(1),
		GasTipCap:  big.NewInt(1),
		Data:       data,
		Algorithm:  algorithm,
		PublicKey:  publicKey,
		Signature:  signature,
		AccessList: types.AccessList{},
	})
}

func markShieldedRootKnown(statedb *state.StateDB, root common.Hash) {
	statedb.SetState(params.ShieldedPoolAddress, ShieldedMerkleRootSlot(root), common.BigToHash(big.NewInt(1)))
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

func TestShieldedWithdrawalEnvelopeRoundTrip(t *testing.T) {
	want := testShieldedEnvelope(t, 1)
	want.WithdrawalRecipient = common.HexToAddress("0x1234")
	want.WithdrawalValue = big.NewInt(7)
	data, err := EncodeShieldedTransaction(want)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := DecodeShieldedTransaction(data)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.WithdrawalRecipient != want.WithdrawalRecipient || got.WithdrawalValue == nil || got.WithdrawalValue.Cmp(want.WithdrawalValue) != 0 {
		t.Fatalf("decoded shielded withdrawal = %+v", got)
	}
}

func TestShieldedGasSponsorEnvelopeRoundTripAndPreBalanceCost(t *testing.T) {
	want := testShieldedEnvelope(t, 1)
	want.Version = ShieldedTxVersionV2
	want.GasSponsorValue = big.NewInt(40000)
	tx := testShieldedPQTkmTx(t, want, "", nil, nil)
	got, ok, err := DecodeShieldedTransaction(tx.Data())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.GasSponsorValue == nil || got.GasSponsorValue.Cmp(want.GasSponsorValue) != 0 {
		t.Fatalf("decoded shielded gas sponsor = %+v", got)
	}
	if cost := ShieldedTransactionPreBalanceCost(tx); cost.Cmp(big.NewInt(60000)) != 0 {
		t.Fatalf("pre-balance cost = %s, want 60000", cost)
	}
}

func TestValidateShieldedTransactionState(t *testing.T) {
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 1)
	tx := testShieldedTx(t, envelope, new(big.Int))
	if err := ValidateShieldedTransactionState(statedb, tx); !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("unknown root error = %v, want %v", err, ErrInvalidShieldedTx)
	}

	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
	if err := ValidateShieldedTransactionState(statedb, tx); err != nil {
		t.Fatalf("known root rejected: %v", err)
	}

	statedb.SetState(params.ShieldedPoolAddress, shieldedNullifierSlot(envelope.Spends[0].Nullifier), common.HexToHash("0x1"))
	if err := ValidateShieldedTransactionState(statedb, tx); !errors.Is(err, ErrInvalidShieldedTx) {
		t.Fatalf("spent nullifier error = %v, want %v", err, ErrInvalidShieldedTx)
	}

	deposit := testShieldedEnvelope(t, 1)
	deposit.Spends[0].Nullifier = common.Hash{}
	deposit.Spends[0].Anchor = common.Hash{}
	depositTx := testShieldedTx(t, deposit, big.NewInt(1))
	if err := ValidateShieldedTransactionState(statedb, depositTx); err != nil {
		t.Fatalf("deposit rejected without root: %v", err)
	}
}

func TestShieldedGasSponsorValidation(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	envelope.Version = ShieldedTxVersionV2
	envelope.GasSponsorValue = big.NewInt(100000)
	tx := testShieldedPQTkmTx(t, envelope, "", nil, nil)
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedGasSponsorTime-1, tx); err == nil {
		t.Fatal("gas sponsorship accepted before activation")
	}
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedGasSponsorTime, tx); err != nil {
		t.Fatalf("valid full gas sponsorship rejected: %v", err)
	}
	if cost := ShieldedTransactionPreBalanceCost(tx); cost.Sign() != 0 {
		t.Fatalf("fully sponsored pre-balance cost = %s, want 0", cost)
	}

	envelope.GasSponsorValue = big.NewInt(100001)
	tx = testShieldedPQTkmTx(t, envelope, "", nil, nil)
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedGasSponsorTime, tx); err == nil {
		t.Fatal("gas sponsorship above maximum transaction cost accepted")
	}

	envelope.Version = ShieldedTxVersionV1
	envelope.GasSponsorValue = big.NewInt(1)
	tx = testShieldedPQTkmTx(t, envelope, "", nil, nil)
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedV2Time-1, tx); err == nil {
		t.Fatal("V1 gas sponsorship accepted")
	}
}

func TestShieldedV2EnvelopeActivationIsVersionExclusive(t *testing.T) {
	v1 := testShieldedEnvelope(t, 1)
	v1tx := testShieldedTx(t, v1, new(big.Int))
	before := params.MainnetShieldedV2Time - 1
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), before, v1tx); err != nil {
		t.Fatalf("V1 rejected before V2 activation: %v", err)
	}
	v2 := testShieldedEnvelope(t, 1)
	v2.Version = ShieldedTxVersionV2
	v2tx := testShieldedTx(t, v2, new(big.Int))
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), before, v2tx); err == nil {
		t.Fatal("V2 accepted before activation")
	}
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedV2Time, v1tx); err == nil {
		t.Fatal("new V1 envelope accepted after V2 activation")
	}
	if err := ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedV2Time, v2tx); err != nil {
		t.Fatalf("V2 rejected at activation: %v", err)
	}
}

func TestShieldedTransactionIntentHashExcludesFinalizedProofFields(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	tx := testShieldedTx(t, envelope, new(big.Int))
	intentHash, err := ShieldedTransactionIntentHash(tx, envelope)
	if err != nil {
		t.Fatal(err)
	}

	proofChanged := testShieldedEnvelope(t, 1)
	proofChanged.Spends[0].Proof = []byte("different proof bytes")
	proofChangedTx := testShieldedTx(t, proofChanged, new(big.Int))
	if tx.Hash() == proofChangedTx.Hash() {
		t.Fatal("full transaction hash did not change after proof byte change")
	}
	proofChangedIntentHash, err := ShieldedTransactionIntentHash(proofChangedTx, proofChanged)
	if err != nil {
		t.Fatal(err)
	}
	if proofChangedIntentHash != intentHash {
		t.Fatalf("intent hash changed after proof-only update: got %s want %s", proofChangedIntentHash, intentHash)
	}

	bindingChanged := testShieldedEnvelope(t, 1)
	bindingChanged.BindingSig = common.BigToHash(big.NewInt(12345)).Bytes()
	bindingChangedTx := testShieldedTx(t, bindingChanged, new(big.Int))
	if tx.Hash() == bindingChangedTx.Hash() {
		t.Fatal("full transaction hash did not change after binding hash change")
	}
	bindingChangedIntentHash, err := ShieldedTransactionIntentHash(bindingChangedTx, bindingChanged)
	if err != nil {
		t.Fatal(err)
	}
	if bindingChangedIntentHash != intentHash {
		t.Fatalf("intent hash changed after binding-only update: got %s want %s", bindingChangedIntentHash, intentHash)
	}

	unsignedPQ := testShieldedPQTkmTx(t, envelope, "", nil, nil)
	unsignedPQIntentHash, err := ShieldedTransactionIntentHash(unsignedPQ, envelope)
	if err != nil {
		t.Fatal(err)
	}
	signedPQ := testShieldedPQTkmTx(t, envelope, "ML-DSA-87", []byte("public-key"), []byte("signature"))
	signedPQIntentHash, err := ShieldedTransactionIntentHash(signedPQ, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if signedPQIntentHash != unsignedPQIntentHash {
		t.Fatalf("intent hash changed after PQ auth metadata update: got %s want %s", signedPQIntentHash, unsignedPQIntentHash)
	}

	nullifierChanged := testShieldedEnvelope(t, 1)
	nullifierChanged.Spends[0].Nullifier = common.BigToHash(big.NewInt(999))
	nullifierChangedTx := testShieldedTx(t, nullifierChanged, new(big.Int))
	nullifierChangedIntentHash, err := ShieldedTransactionIntentHash(nullifierChangedTx, nullifierChanged)
	if err != nil {
		t.Fatal(err)
	}
	if nullifierChangedIntentHash == intentHash {
		t.Fatal("intent hash did not change after nullifier update")
	}
}

func TestShieldedTransactionIntentHashBindsWithdrawal(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	envelope.WithdrawalRecipient = common.HexToAddress("0x1234")
	envelope.WithdrawalValue = big.NewInt(7)
	tx := testShieldedTx(t, envelope, new(big.Int))
	want, err := ShieldedTransactionIntentHash(tx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	changed := *envelope
	changed.WithdrawalRecipient = common.HexToAddress("0x5678")
	changedTx := testShieldedTx(t, &changed, new(big.Int))
	got, err := ShieldedTransactionIntentHash(changedTx, &changed)
	if err != nil {
		t.Fatal(err)
	}
	if got == want {
		t.Fatal("withdrawal recipient was not bound into the shielded intent hash")
	}
}

func TestProcessShieldedSpendUsesIntentHashForProofContext(t *testing.T) {
	SetShieldedProofVerifier(nil)
	defer SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 1)
	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
	tx := testShieldedTx(t, envelope, new(big.Int))
	intentHash, err := ShieldedTransactionIntentHash(tx, envelope)
	if err != nil {
		t.Fatal(err)
	}
	SetShieldedProofVerifier(shieldedVerifierFunc(func(ctx ShieldedProofContext, proof []byte) error {
		if ctx.TxHash != intentHash {
			t.Fatalf("proof context tx hash = %s, want intent hash %s", ctx.TxHash, intentHash)
		}
		if ctx.TxHash == tx.Hash() {
			t.Fatal("proof context used full transaction hash")
		}
		return nil
	}))
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err != nil {
		t.Fatal(err)
	}
}

func TestProcessShieldedSpendRequiresVerifier(t *testing.T) {
	SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 1)
	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
	tx := testShieldedTx(t, envelope, new(big.Int))
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

func TestMainnetRecoveryVerifierActivation(t *testing.T) {
	SetShieldedProofVerifier(nil)
	defer SetShieldedProofVerifier(nil)

	recovery := recoveryShieldedGroth16VerifierFromParams(params.MainnetChainConfig)
	if recovery == nil {
		t.Fatal("mainnet recovery verifier is unavailable")
	}
	before := activeShieldedProofVerifier(params.MainnetChainConfig, params.MainnetShieldedGroth16RecoveryTime-1, ShieldedTxVersionV1)
	if before == recovery {
		t.Fatal("recovery verifier activated before recovery timestamp")
	}
	after := activeShieldedProofVerifier(params.MainnetChainConfig, params.MainnetShieldedGroth16RecoveryTime, ShieldedTxVersionV1)
	if after != recovery {
		t.Fatal("recovery verifier did not activate at recovery timestamp")
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
	ic := make([][]byte, shieldedPublicInputsV1+1)
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
	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
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
	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
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

func TestProcessShieldedWithdrawalReleasesProvenPublicValue(t *testing.T) {
	recipient := common.HexToAddress("0x1234")
	SetShieldedProofVerifier(shieldedVerifierFunc(func(ctx ShieldedProofContext, proof []byte) error {
		if ctx.PublicValue.Cmp(big.NewInt(7)) != 0 {
			t.Fatalf("withdrawal public value = %s, want 7", ctx.PublicValue)
		}
		return nil
	}))
	defer SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 1)
	envelope.WithdrawalRecipient = recipient
	envelope.WithdrawalValue = big.NewInt(7)
	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
	statedb.AddBalance(params.ShieldedPoolAddress, uint256.NewInt(10), tracing.BalanceChangeUnspecified)
	tx := testShieldedTx(t, envelope, new(big.Int))
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err != nil {
		t.Fatal(err)
	}
	if got := statedb.GetBalance(params.ShieldedPoolAddress); !got.Eq(uint256.NewInt(3)) {
		t.Fatalf("shielded pool balance = %s, want 3", got)
	}
	if got := statedb.GetBalance(recipient); !got.Eq(uint256.NewInt(7)) {
		t.Fatalf("withdrawal recipient balance = %s, want 7", got)
	}
}

func TestProcessShieldedSpendFundsGasFromPrivateNote(t *testing.T) {
	key, err := pqcrypto.NewMLDSA87FromSeed(make([]byte, pqcrypto.MLDSA87SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	recipient := common.HexToAddress("0x1234")
	envelope := testShieldedEnvelope(t, 1)
	envelope.Version = ShieldedTxVersionV2
	envelope.WithdrawalRecipient = recipient
	envelope.WithdrawalValue = big.NewInt(7)
	envelope.GasSponsorValue = big.NewInt(9)
	data, err := EncodeShieldedTransaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	signer := types.MakeSigner(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedGasSponsorTime)
	tx, err := types.SignNewPQTkmTx(key, signer, &types.PQTkmTx{
		ChainID:   new(big.Int).Set(params.MainnetChainConfig.ChainID),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1),
		Gas:       100000,
		To:        &params.ShieldedPoolAddress,
		Value:     new(big.Int),
		Data:      data,
	})
	if err != nil {
		t.Fatal(err)
	}
	sender, err := types.Sender(signer, tx)
	if err != nil {
		t.Fatal(err)
	}
	SetShieldedProofVerifier(shieldedVerifierFunc(func(ctx ShieldedProofContext, proof []byte) error {
		if ctx.Sender != sender {
			t.Fatalf("proof sender = %s, want %s", ctx.Sender, sender)
		}
		if ctx.PublicValue.Cmp(big.NewInt(16)) != 0 {
			t.Fatalf("proof public value = %s, want 16", ctx.PublicValue)
		}
		return nil
	}))
	defer SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	markShieldedRootKnown(statedb, envelope.Spends[0].Anchor)
	statedb.AddBalance(params.ShieldedPoolAddress, uint256.NewInt(20), tracing.BalanceChangeUnspecified)
	if err := processShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetShieldedGasSponsorTime, statedb, tx, make(map[common.Hash]struct{})); err != nil {
		t.Fatal(err)
	}
	if got := statedb.GetBalance(params.ShieldedPoolAddress); !got.Eq(uint256.NewInt(4)) {
		t.Fatalf("shielded pool balance = %s, want 4", got)
	}
	if got := statedb.GetBalance(recipient); !got.Eq(uint256.NewInt(7)) {
		t.Fatalf("withdrawal recipient balance = %s, want 7", got)
	}
	if got := statedb.GetBalance(sender); !got.Eq(uint256.NewInt(9)) {
		t.Fatalf("gas-funded sender balance = %s, want 9", got)
	}
}

func TestShieldedWithdrawalRejectsInvalidPublicRelease(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	envelope.WithdrawalRecipient = common.HexToAddress("0x1234")
	envelope.WithdrawalValue = new(big.Int).Lsh(big.NewInt(1), 65)
	tx := testShieldedTx(t, envelope, new(big.Int))
	if err := ValidateShieldedTransactionBasics(params.EgyptChainConfig, big.NewInt(1), 0, tx); err == nil {
		t.Fatal("oversized shielded withdrawal accepted")
	}
	envelope.WithdrawalValue = nil
	tx = testShieldedTx(t, envelope, new(big.Int))
	if err := ValidateShieldedTransactionBasics(params.EgyptChainConfig, big.NewInt(1), 0, tx); err == nil {
		t.Fatal("shielded withdrawal recipient without value accepted")
	}
}

func TestProcessShieldedDepositStoresCommitmentAndMerklePath(t *testing.T) {
	SetShieldedProofVerifier(shieldedVerifierFunc(func(ctx ShieldedProofContext, proof []byte) error {
		if ctx.Nullifier != (common.Hash{}) {
			t.Fatalf("deposit nullifier = %s, want zero", ctx.Nullifier)
		}
		if ctx.Anchor != (common.Hash{}) {
			t.Fatalf("deposit anchor = %s, want zero", ctx.Anchor)
		}
		if ctx.PublicValue.Cmp(big.NewInt(7)) != 0 {
			t.Fatalf("deposit public value = %s, want 7", ctx.PublicValue)
		}
		if len(proof) == 0 {
			return ErrInvalidShieldedTx
		}
		return nil
	}))
	defer SetShieldedProofVerifier(nil)
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	envelope := testShieldedEnvelope(t, 0)
	envelope.Spends = append(envelope.Spends, ShieldedSpend{Proof: []byte("deposit-proof")})
	tx := testShieldedTx(t, envelope, big.NewInt(7))
	if err := processShieldedTransaction(params.EgyptChainConfig, big.NewInt(1), 0, statedb, tx, make(map[common.Hash]struct{})); err != nil {
		t.Fatal(err)
	}
	if got := statedb.GetState(params.ShieldedPoolAddress, shieldedNullifierSlot(common.Hash{})); got != (common.Hash{}) {
		t.Fatalf("deposit stored zero nullifier = %s", got)
	}
	if got := statedb.GetState(params.ShieldedPoolAddress, shieldedCommitmentSlot(envelope.Outputs[0].Commitment)); got != envelope.Outputs[0].PayloadHash {
		t.Fatalf("commitment state = %s, want %s", got, envelope.Outputs[0].PayloadHash)
	}
	witness, ok := ShieldedCommitmentPath(statedb, envelope.Outputs[0].Commitment)
	if !ok {
		t.Fatal("commitment path was not stored")
	}
	if witness.Index != 0 {
		t.Fatalf("commitment index = %d, want 0", witness.Index)
	}
	if len(witness.Path) != shieldedMerkleDepth || len(witness.PathIndex) != shieldedMerkleDepth {
		t.Fatalf("path lengths = %d/%d, want %d", len(witness.Path), len(witness.PathIndex), shieldedMerkleDepth)
	}
	if !shieldedMerkleRootKnown(statedb, witness.Root) {
		t.Fatalf("deposit root %s was not stored", witness.Root)
	}
	if got := ShieldedMerkleNextIndex(statedb); got != shieldedOutputSlots {
		t.Fatalf("next shielded index = %d, want %d", got, shieldedOutputSlots)
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
	if err := validateShieldedEnvelope(envelope, ShieldedTxVersionV1); err == nil {
		t.Fatal("non-canonical nullifier accepted")
	}

	envelope = testShieldedEnvelope(t, 1)
	envelope.Outputs[0].Commitment = nonCanonical
	if err := validateShieldedEnvelope(envelope, ShieldedTxVersionV1); err == nil {
		t.Fatal("non-canonical output commitment accepted")
	}

	envelope = testShieldedEnvelope(t, 1)
	envelope.BindingSig = []byte("short")
	if err := validateShieldedEnvelope(envelope, ShieldedTxVersionV1); err == nil {
		t.Fatal("short binding hash accepted")
	}
}

func TestValidateShieldedEnvelopeRequiresPaddedOutputs(t *testing.T) {
	envelope := testShieldedEnvelope(t, 1)
	envelope.Outputs = envelope.Outputs[:1]
	if err := validateShieldedEnvelope(envelope, ShieldedTxVersionV1); err == nil {
		t.Fatal("unpadded output set accepted")
	}
}
