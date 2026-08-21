package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	shieldedGroth16ProofMagic = "TKMG16V1"
	shieldedGroth16VKMagic    = "TKMG16VK1"
	shieldedPublicInputsV1    = 11
	shieldedPublicInputsV2    = 12

	shieldedDomainOutput   = uint64(1004)
	shieldedDomainOutputV2 = uint64(2004)
)

var ErrShieldedVerifyingKeyUnavailable = errors.New("shielded verifying key unavailable")

// ShieldedGroth16Proof is the on-chain proof format for one shielded spend.
// Points are gnark-crypto BN254 compressed encodings.
type ShieldedGroth16Proof struct {
	A []byte
	B []byte
	C []byte
}

// ShieldedGroth16VerifyingKey is the serialized consensus verifying key.
// IC must contain one constant term plus one G1 point per public input.
type ShieldedGroth16VerifyingKey struct {
	AlphaG1 []byte
	BetaG2  []byte
	GammaG2 []byte
	DeltaG2 []byte
	IC      [][]byte
}

type shieldedGroth16Verifier struct {
	vk *parsedShieldedGroth16VerifyingKey
}

type parsedShieldedGroth16VerifyingKey struct {
	alphaG1 bn254.G1Affine
	betaG2  bn254.G2Affine
	gammaG2 bn254.G2Affine
	deltaG2 bn254.G2Affine
	ic      []bn254.G1Affine
}

type shieldedGroth16VerifierCache struct {
	sync.RWMutex
	keyHash  common.Hash
	verifier ShieldedProofVerifier
	err      error
}

var (
	shieldedGroth16ConfigCache   shieldedGroth16VerifierCache
	shieldedGroth16UpgradeCache  shieldedGroth16VerifierCache
	shieldedGroth16RecoveryCache shieldedGroth16VerifierCache
	shieldedGroth16V2Cache       shieldedGroth16VerifierCache
)

// NewShieldedGroth16Verifier creates a production BN254 Groth16 verifier.
func NewShieldedGroth16Verifier(encodedVK []byte) (ShieldedProofVerifier, error) {
	vk, err := DecodeShieldedGroth16VerifyingKey(encodedVK)
	if err != nil {
		return nil, err
	}
	return &shieldedGroth16Verifier{vk: vk}, nil
}

// SetShieldedGroth16VerifyingKey installs the consensus Groth16 verifying key.
func SetShieldedGroth16VerifyingKey(encodedVK []byte) error {
	verifier, err := NewShieldedGroth16Verifier(encodedVK)
	if err != nil {
		SetShieldedProofVerifier(nil)
		return err
	}
	SetShieldedProofVerifier(verifier)
	return nil
}

func shieldedGroth16VerifierFromChainConfig(config *params.ChainConfig) ShieldedProofVerifier {
	if config == nil || len(config.ShieldedGroth16VerifyingKey) == 0 {
		return nil
	}
	return shieldedGroth16VerifierFromBytes(config.ShieldedGroth16VerifyingKey, &shieldedGroth16ConfigCache)
}

func upgradedShieldedGroth16VerifierFromParams(config *params.ChainConfig) ShieldedProofVerifier {
	if config == nil || config.ChainID == nil || params.MainnetChainConfig == nil || params.MainnetChainConfig.ChainID == nil {
		return nil
	}
	if config.ChainID.Cmp(params.MainnetChainConfig.ChainID) != 0 {
		return nil
	}
	if len(params.MainnetShieldedGroth16UpgradedVerifyingKey) == 0 {
		return nil
	}
	return shieldedGroth16VerifierFromBytes(params.MainnetShieldedGroth16UpgradedVerifyingKey, &shieldedGroth16UpgradeCache)
}

func recoveryShieldedGroth16VerifierFromParams(config *params.ChainConfig) ShieldedProofVerifier {
	if config == nil || config.ChainID == nil || params.MainnetChainConfig == nil || params.MainnetChainConfig.ChainID == nil {
		return nil
	}
	if config.ChainID.Cmp(params.MainnetChainConfig.ChainID) != 0 || len(params.MainnetShieldedGroth16RecoveryVerifyingKey) == 0 {
		return nil
	}
	return shieldedGroth16VerifierFromBytes(params.MainnetShieldedGroth16RecoveryVerifyingKey, &shieldedGroth16RecoveryCache)
}

func v2ShieldedGroth16VerifierFromParams(config *params.ChainConfig) ShieldedProofVerifier {
	if config == nil || config.ChainID == nil || params.MainnetChainConfig == nil || params.MainnetChainConfig.ChainID == nil {
		return nil
	}
	if config.ChainID.Cmp(params.MainnetChainConfig.ChainID) != 0 || len(params.MainnetShieldedGroth16V2VerifyingKey) == 0 {
		return nil
	}
	return shieldedGroth16VerifierFromBytes(params.MainnetShieldedGroth16V2VerifyingKey, &shieldedGroth16V2Cache)
}

func shieldedGroth16VerifierFromBytes(encoded []byte, cache *shieldedGroth16VerifierCache) ShieldedProofVerifier {
	keyHash := crypto.Keccak256Hash(encoded)
	cache.RLock()
	if cache.keyHash == keyHash {
		verifier := cache.verifier
		cache.RUnlock()
		return verifier
	}
	cache.RUnlock()

	cache.Lock()
	defer cache.Unlock()
	if cache.keyHash == keyHash {
		return cache.verifier
	}
	verifier, err := NewShieldedGroth16Verifier(encoded)
	cache.keyHash = keyHash
	cache.verifier = verifier
	cache.err = err
	if err != nil {
		return unavailableShieldedVerifier{}
	}
	return verifier
}

// EncodeShieldedGroth16Proof encodes one BN254 Groth16 proof for tx spend data.
func EncodeShieldedGroth16Proof(proof ShieldedGroth16Proof) ([]byte, error) {
	payload, err := rlp.EncodeToBytes(&proof)
	if err != nil {
		return nil, err
	}
	return append([]byte(shieldedGroth16ProofMagic), payload...), nil
}

// EncodeShieldedGroth16VerifyingKey encodes the consensus BN254 Groth16 verifying key.
func EncodeShieldedGroth16VerifyingKey(vk ShieldedGroth16VerifyingKey) ([]byte, error) {
	payload, err := rlp.EncodeToBytes(&vk)
	if err != nil {
		return nil, err
	}
	return append([]byte(shieldedGroth16VKMagic), payload...), nil
}

func DecodeShieldedGroth16VerifyingKey(encoded []byte) (*parsedShieldedGroth16VerifyingKey, error) {
	if !bytes.HasPrefix(encoded, []byte(shieldedGroth16VKMagic)) {
		return nil, fmt.Errorf("%w: missing Groth16 verifying key magic", ErrShieldedVerifyingKeyUnavailable)
	}
	var raw ShieldedGroth16VerifyingKey
	if err := rlp.DecodeBytes(encoded[len(shieldedGroth16VKMagic):], &raw); err != nil {
		return nil, fmt.Errorf("%w: malformed Groth16 verifying key: %v", ErrShieldedVerifyingKeyUnavailable, err)
	}
	if len(raw.IC) != shieldedPublicInputsV1+1 && len(raw.IC) != shieldedPublicInputsV2+1 {
		return nil, fmt.Errorf("%w: verifying key has %d IC points, want %d (V1) or %d (V2)", ErrShieldedVerifyingKeyUnavailable, len(raw.IC), shieldedPublicInputsV1+1, shieldedPublicInputsV2+1)
	}
	vk := &parsedShieldedGroth16VerifyingKey{ic: make([]bn254.G1Affine, len(raw.IC))}
	var err error
	if vk.alphaG1, err = decodeG1(raw.AlphaG1); err != nil {
		return nil, fmt.Errorf("%w: alpha_g1: %v", ErrShieldedVerifyingKeyUnavailable, err)
	}
	if vk.alphaG1.IsInfinity() {
		return nil, fmt.Errorf("%w: alpha_g1 is infinity", ErrShieldedVerifyingKeyUnavailable)
	}
	if vk.betaG2, err = decodeG2(raw.BetaG2); err != nil {
		return nil, fmt.Errorf("%w: beta_g2: %v", ErrShieldedVerifyingKeyUnavailable, err)
	}
	if vk.betaG2.IsInfinity() {
		return nil, fmt.Errorf("%w: beta_g2 is infinity", ErrShieldedVerifyingKeyUnavailable)
	}
	if vk.gammaG2, err = decodeG2(raw.GammaG2); err != nil {
		return nil, fmt.Errorf("%w: gamma_g2: %v", ErrShieldedVerifyingKeyUnavailable, err)
	}
	if vk.gammaG2.IsInfinity() {
		return nil, fmt.Errorf("%w: gamma_g2 is infinity", ErrShieldedVerifyingKeyUnavailable)
	}
	if vk.deltaG2, err = decodeG2(raw.DeltaG2); err != nil {
		return nil, fmt.Errorf("%w: delta_g2: %v", ErrShieldedVerifyingKeyUnavailable, err)
	}
	if vk.deltaG2.IsInfinity() {
		return nil, fmt.Errorf("%w: delta_g2 is infinity", ErrShieldedVerifyingKeyUnavailable)
	}
	for i, point := range raw.IC {
		if vk.ic[i], err = decodeG1(point); err != nil {
			return nil, fmt.Errorf("%w: ic[%d]: %v", ErrShieldedVerifyingKeyUnavailable, i, err)
		}
	}
	return vk, nil
}

func (v *shieldedGroth16Verifier) VerifyShieldedSpend(ctx ShieldedProofContext, encodedProof []byte) error {
	if v == nil || v.vk == nil {
		return ErrShieldedVerifyingKeyUnavailable
	}
	if err := validateShieldedProofContext(ctx); err != nil {
		return err
	}
	proof, err := decodeShieldedGroth16Proof(encodedProof)
	if err != nil {
		return err
	}
	var inputs []fr.Element
	switch len(v.vk.ic) {
	case shieldedPublicInputsV1 + 1:
		if ctx.Version != 0 && ctx.Version != ShieldedTxVersionV1 {
			return fmt.Errorf("%w: V1 verifying key cannot verify envelope version %d", ErrInvalidShieldedTx, ctx.Version)
		}
		v1 := shieldedProofPublicInputs(ctx)
		inputs = v1[:]
	case shieldedPublicInputsV2 + 1:
		if ctx.Version != ShieldedTxVersionV2 {
			return fmt.Errorf("%w: V2 verifying key cannot verify envelope version %d", ErrInvalidShieldedTx, ctx.Version)
		}
		inputs = shieldedProofPublicInputsV2(ctx)
	default:
		return ErrShieldedVerifyingKeyUnavailable
	}
	vkX := v.vk.ic[0]
	for i := range inputs {
		var term bn254.G1Affine
		term.ScalarMultiplication(&v.vk.ic[i+1], inputs[i].BigInt(new(big.Int)))
		vkX.Add(&vkX, &term)
	}
	negA := proof.a
	negA.Neg(&negA)
	ok, err := bn254.PairingCheck(
		[]bn254.G1Affine{negA, v.vk.alphaG1, vkX, proof.c},
		[]bn254.G2Affine{proof.b, v.vk.betaG2, v.vk.gammaG2, v.vk.deltaG2},
	)
	if err != nil {
		return fmt.Errorf("Groth16 pairing check failed: %w", err)
	}
	if !ok {
		return ErrInvalidShieldedTx
	}
	return nil
}

func validateShieldedProofContext(ctx ShieldedProofContext) error {
	if ctx.ChainID == nil || ctx.ChainID.Sign() < 0 || ctx.ChainID.BitLen() > 64 {
		return fmt.Errorf("%w: chain ID must be a 64-bit unsigned value", ErrInvalidShieldedTx)
	}
	if ctx.BlockNumber == nil || ctx.BlockNumber.Sign() < 0 || ctx.BlockNumber.BitLen() > 64 {
		return fmt.Errorf("%w: block number must be a 64-bit unsigned value", ErrInvalidShieldedTx)
	}
	if ctx.SpendIndex < 0 {
		return fmt.Errorf("%w: negative spend index", ErrInvalidShieldedTx)
	}
	if ctx.PublicValue == nil || ctx.PublicValue.Sign() < 0 || ctx.PublicValue.BitLen() > 64 {
		return fmt.Errorf("%w: public value must be a 64-bit unsigned value", ErrInvalidShieldedTx)
	}
	if ctx.Version == ShieldedTxVersionV2 && ctx.Sender == (common.Address{}) {
		return fmt.Errorf("%w: V2 proof context requires the recovered PQ sender", ErrInvalidShieldedTx)
	}
	if !isCanonicalShieldedFieldHash(ctx.Nullifier) {
		return fmt.Errorf("%w: nullifier is not a canonical BN254 field element", ErrInvalidShieldedTx)
	}
	if !isCanonicalShieldedFieldHash(ctx.Anchor) {
		return fmt.Errorf("%w: anchor is not a canonical BN254 field element", ErrInvalidShieldedTx)
	}
	if !isCanonicalShieldedFieldHash(ctx.BalanceCommitment) {
		return fmt.Errorf("%w: balance commitment is not a canonical BN254 field element", ErrInvalidShieldedTx)
	}
	if len(ctx.OutputCommitments) != shieldedOutputSlots {
		return fmt.Errorf("%w: proof context must carry exactly %d output commitments", ErrInvalidShieldedTx, shieldedOutputSlots)
	}
	for i, commitment := range ctx.OutputCommitments {
		if !isCanonicalShieldedFieldHash(commitment) {
			return fmt.Errorf("%w: output commitment %d is not a canonical BN254 field element", ErrInvalidShieldedTx, i)
		}
	}
	if len(ctx.BindingSig) != common.HashLength {
		return fmt.Errorf("%w: binding hash must be %d bytes", ErrInvalidShieldedTx, common.HashLength)
	}
	if !isCanonicalShieldedFieldHash(common.BytesToHash(ctx.BindingSig)) {
		return fmt.Errorf("%w: binding hash is not a canonical BN254 field element", ErrInvalidShieldedTx)
	}
	return nil
}

type parsedShieldedGroth16Proof struct {
	a bn254.G1Affine
	b bn254.G2Affine
	c bn254.G1Affine
}

func decodeShieldedGroth16Proof(encoded []byte) (parsedShieldedGroth16Proof, error) {
	if !bytes.HasPrefix(encoded, []byte(shieldedGroth16ProofMagic)) {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: missing Groth16 proof magic", ErrInvalidShieldedTx)
	}
	var raw ShieldedGroth16Proof
	if err := rlp.DecodeBytes(encoded[len(shieldedGroth16ProofMagic):], &raw); err != nil {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: malformed Groth16 proof: %v", ErrInvalidShieldedTx, err)
	}
	a, err := decodeG1(raw.A)
	if err != nil {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: proof A: %v", ErrInvalidShieldedTx, err)
	}
	if a.IsInfinity() {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: proof A is infinity", ErrInvalidShieldedTx)
	}
	b, err := decodeG2(raw.B)
	if err != nil {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: proof B: %v", ErrInvalidShieldedTx, err)
	}
	if b.IsInfinity() {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: proof B is infinity", ErrInvalidShieldedTx)
	}
	c, err := decodeG1(raw.C)
	if err != nil {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: proof C: %v", ErrInvalidShieldedTx, err)
	}
	if c.IsInfinity() {
		return parsedShieldedGroth16Proof{}, fmt.Errorf("%w: proof C is infinity", ErrInvalidShieldedTx)
	}
	return parsedShieldedGroth16Proof{a: a, b: b, c: c}, nil
}

func decodeG1(encoded []byte) (bn254.G1Affine, error) {
	var p bn254.G1Affine
	if len(encoded) == 0 {
		return p, io.ErrUnexpectedEOF
	}
	reader := bytes.NewReader(encoded)
	dec := bn254.NewDecoder(reader)
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	if reader.Len() != 0 {
		return p, fmt.Errorf("trailing G1 bytes: %d", reader.Len())
	}
	return p, nil
}

func decodeG2(encoded []byte) (bn254.G2Affine, error) {
	var p bn254.G2Affine
	if len(encoded) == 0 {
		return p, io.ErrUnexpectedEOF
	}
	reader := bytes.NewReader(encoded)
	dec := bn254.NewDecoder(reader)
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	if reader.Len() != 0 {
		return p, fmt.Errorf("trailing G2 bytes: %d", reader.Len())
	}
	return p, nil
}

func shieldedProofPublicInputs(ctx ShieldedProofContext) [shieldedPublicInputsV1]fr.Element {
	txHashHi, txHashLo := fieldElementsFromHashLimbs(ctx.TxHash)
	return [shieldedPublicInputsV1]fr.Element{
		fieldElementFromBig(ctx.ChainID),
		fieldElementFromBig(ctx.BlockNumber),
		txHashHi,
		txHashLo,
		fieldElementFromUint64(uint64(ctx.SpendIndex)),
		fieldElementFromHash(ctx.Nullifier),
		fieldElementFromHash(ctx.Anchor),
		fieldElementFromHash(ctx.BalanceCommitment),
		fieldElementFromBig(ctx.PublicValue),
		fieldElementFromHash(shieldedOutputCommitmentsRoot(ctx.OutputCommitments)),
		fieldElementFromHash(common.BytesToHash(ctx.BindingSig)),
	}
}

func shieldedProofPublicInputsV2(ctx ShieldedProofContext) []fr.Element {
	txHashHi, txHashLo := fieldElementsFromHashLimbs(ctx.TxHash)
	return []fr.Element{
		fieldElementFromBig(ctx.ChainID),
		fieldElementFromBig(ctx.BlockNumber),
		txHashHi,
		txHashLo,
		fieldElementFromUint64(uint64(ctx.SpendIndex)),
		fieldElementFromHash(ctx.Nullifier),
		fieldElementFromHash(ctx.Anchor),
		fieldElementFromHash(ctx.BalanceCommitment),
		fieldElementFromBig(ctx.PublicValue),
		fieldElementFromHash(shieldedOutputCommitmentsRootV2(ctx.OutputCommitments)),
		fieldElementFromHash(common.BytesToHash(ctx.BindingSig)),
		fieldElementFromBig(ctx.Sender.Big()),
	}
}

func shieldedOutputCommitmentsRoot(commitments []common.Hash) common.Hash {
	return shieldedOutputCommitmentsRootWithDomain(commitments, shieldedDomainOutput)
}

func shieldedOutputCommitmentsRootV2(commitments []common.Hash) common.Hash {
	return shieldedOutputCommitmentsRootWithDomain(commitments, shieldedDomainOutputV2)
}

func shieldedOutputCommitmentsRootWithDomain(commitments []common.Hash, outputDomain uint64) common.Hash {
	hasher := mimc.NewMiMC()
	domain := fieldElementFromUint64(outputDomain)
	domainBytes := domain.Bytes()
	hasher.Write(domainBytes[:])
	for _, commitment := range commitments {
		element := fieldElementFromHash(commitment)
		elementBytes := element.Bytes()
		hasher.Write(elementBytes[:])
	}
	for i := len(commitments); i < shieldedOutputSlots; i++ {
		var zero fr.Element
		zeroBytes := zero.Bytes()
		hasher.Write(zeroBytes[:])
	}
	sum := hasher.Sum(nil)
	var element fr.Element
	if err := element.SetBytesCanonical(sum); err != nil {
		panic(err)
	}
	var out common.Hash
	elementBytes := element.Bytes()
	copy(out[:], elementBytes[:])
	return out
}

func fieldElementFromHash(hash common.Hash) fr.Element {
	var out fr.Element
	if err := out.SetBytesCanonical(hash.Bytes()); err != nil {
		panic(err)
	}
	return out
}

func isCanonicalShieldedFieldHash(hash common.Hash) bool {
	var out fr.Element
	return out.SetBytesCanonical(hash.Bytes()) == nil
}

func fieldElementsFromHashLimbs(hash common.Hash) (fr.Element, fr.Element) {
	var hi, lo fr.Element
	hi.SetBytes(hash[:16])
	lo.SetBytes(hash[16:])
	return hi, lo
}

func fieldElementFromBig(v *big.Int) fr.Element {
	var out fr.Element
	if v != nil {
		out.SetBigInt(v)
	}
	return out
}

func fieldElementFromUint64(v uint64) fr.Element {
	var out fr.Element
	out.SetUint64(v)
	return out
}
