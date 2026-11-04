// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package shielded6

import (
	"crypto/hkdf"
	"crypto/sha512"
	"encoding/binary"
	"io"
	"math/big"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	shield6NoteVersion   = 1
	shield6NativeAssetID = AssetTKM
)

// OutputRecord is the wallet-side form of a ShieldedV6Output. The matching
// core envelope uses the same field encodings. EncryptedNote contains only
// versioned, authenticated ciphertext; amount and masks are never public.
type OutputRecord struct {
	OneTimeKey    [PointSize]byte
	EphemeralKey  [PointSize]byte
	Commitment    [PointSize]byte
	AmountLimbs   [2][PointSize]byte
	EncryptedNote []byte
}

// Note is a decrypted Shield6 output. SpendSecret is the one-time private key
// corresponding to OutputRecord.OneTimeKey. Call Clear after use.
type Note struct {
	ChainID     uint64
	AssetID     uint64
	Index       uint64
	Amount      *big.Int
	Witness     WideWitness
	SpendSecret [ScalarSize]byte
}

// SigningInput prepares a scanned note for CLSAG signing against a ring
// supplied by the wallet's canonical accumulator scanner. It verifies that
// the recovered secret and opening match the designated real ring member.
func (n *Note) SigningInput(ring [RingSize]RingMember, signerIndex uint64) (SigningInput128, error) {
	var input SigningInput128
	if n == nil || n.ChainID == 0 || n.AssetID != shield6NativeAssetID || n.Amount == nil || n.Amount.Sign() < 0 || n.Amount.BitLen() > 128 || signerIndex >= RingSize {
		return input, ErrInvalidWalletOutput
	}
	spend, err := decodeScalar(n.SpendSecret[:])
	if err != nil || spend.Equal(new(curve25519.Scalar)) == 1 {
		return input, ErrInvalidWalletOutput
	}
	defer spend.Set(new(curve25519.Scalar))
	spendPublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(spend)
	opening, err := Commitment128(n.Amount, n.Witness.Mask)
	if err != nil || !bytesEqual(spendPublic.Bytes(), ring[signerIndex].OneTimeKey[:]) || opening != ring[signerIndex].Commitment {
		return input, ErrInvalidWalletOutput
	}
	seen := make(map[[PointSize]byte]struct{}, RingSize)
	for _, member := range ring {
		if _, exists := seen[member.OneTimeKey]; exists || ValidatePublicKey(member.OneTimeKey[:]) != nil || ValidateCommitmentEncoding(member.Commitment[:]) != nil {
			return input, ErrInvalidWalletOutput
		}
		seen[member.OneTimeKey] = struct{}{}
	}
	input.Ring = ring
	input.SignerIndex = signerIndex
	input.SpendSecret = n.SpendSecret
	input.CommitmentMask = n.Witness.Mask
	input.Amount = new(big.Int).Set(n.Amount)
	return input, nil
}

func (n *Note) Clear() {
	if n == nil {
		return
	}
	clearBigInt(n.Amount)
	clear(n.Witness.Mask[:])
	clear(n.Witness.LimbMasks[0][:])
	clear(n.Witness.LimbMasks[1][:])
	clear(n.SpendSecret[:])
}

// clearBigInt wipes the backing words before resetting the value. SetInt64(0)
// alone may leave the former amount in the big.Int's allocated word slice.
func clearBigInt(value *big.Int) {
	if value == nil {
		return
	}
	words := value.Bits()
	clear(words)
	value.SetInt64(0)
}

// SumWideWitnessMasks adds the total output blinding masks modulo the curve
// scalar order for the CLSAG pseudo-output balancing equation.
func SumWideWitnessMasks(witnesses []WideWitness) ([ScalarSize]byte, error) {
	var result [ScalarSize]byte
	var sum curve25519.Scalar
	for i := range witnesses {
		mask, err := decodeScalar(witnesses[i].Mask[:])
		if err != nil {
			return result, ErrInvalidWalletOutput
		}
		sum.Add(&sum, mask)
		mask.Set(new(curve25519.Scalar))
	}
	result = scalarBytes(sum)
	sum.Set(new(curve25519.Scalar))
	return result, nil
}

type shield6NotePlaintext struct {
	Version        uint64
	ChainID        uint64
	AssetID        uint64
	OutputPosition uint64
	Amount         [16]byte
	Mask           [ScalarSize]byte
	LimbMasks      [2][ScalarSize]byte
}

type shield6EncryptedNote struct {
	Version    uint64
	Nonce      []byte
	Ciphertext []byte
}

type shield6OutputAAD struct {
	Domain       []byte
	ChainID      uint64
	Index        uint64
	OneTimeKey   [PointSize]byte
	EphemeralKey [PointSize]byte
	Commitment   [PointSize]byte
	AmountLimbs  [2][PointSize]byte
}

// CreateOutput creates one Shield6 output. outputPosition is its index inside
// the transaction, not its eventual global accumulator index; the latter is
// assigned by consensus when the transaction is included. The supplied wide
// witness must be the witness used by ProveRangeProof128WithWitness for this
// amount; the function checks that its total and limb commitments agree.
func CreateOutput(address Address, outputPosition, assetID uint64, amount *big.Int, witness WideWitness, random io.Reader) (OutputRecord, error) {
	var output OutputRecord
	if err := address.Validate(); err != nil || assetID != shield6NativeAssetID || amount == nil || amount.Sign() < 0 || amount.BitLen() > 128 || random == nil {
		return output, ErrInvalidWalletOutput
	}
	var amountBytes [16]byte
	amount.FillBytes(amountBytes[:])
	defer clear(amountBytes[:])
	var limbs [2]uint64
	limbs[0] = binary.BigEndian.Uint64(amountBytes[8:])
	limbs[1] = binary.BigEndian.Uint64(amountBytes[:8])
	lowMask, err := decodeScalar(witness.LimbMasks[0][:])
	if err != nil {
		return output, ErrInvalidWalletOutput
	}
	defer lowMask.Set(new(curve25519.Scalar))
	highMask, err := decodeScalar(witness.LimbMasks[1][:])
	if err != nil {
		return output, ErrInvalidWalletOutput
	}
	defer highMask.Set(new(curve25519.Scalar))
	limbPoints := [2][PointSize]byte{}
	limbMasks := [2]curve25519.Scalar{*lowMask, *highMask}
	defer func() {
		for i := range limbMasks {
			limbMasks[i].Set(new(curve25519.Scalar))
		}
	}()
	for i, limb := range limbs {
		point := ringct.CalculateCommitment(new(curve25519.PublicKey[curve25519.ConstantTimeOperations]), ringct.LazyCommitment{Mask: limbMasks[i], Amount: limb})
		copy(limbPoints[i][:], point.Bytes())
	}
	combined, err := CombineAmountLimbs(limbPoints[0], limbPoints[1])
	if err != nil {
		return output, ErrInvalidWalletOutput
	}
	opening, err := Commitment128(amount, witness.Mask)
	if err != nil || opening != combined {
		return output, ErrInvalidWalletOutput
	}

	viewPublic, err := decodePointCT(address.ViewPublic[:], false)
	if err != nil {
		return output, ErrInvalidWalletOutput
	}
	var ephemeral curve25519.Scalar
	defer ephemeral.Set(new(curve25519.Scalar))
	curve25519.RandomScalar(&ephemeral, random)
	ephemeralPublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(&ephemeral)
	shared := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(&ephemeral, viewPublic)
	var identity curve25519.PublicKey[curve25519.ConstantTimeOperations]
	identity.Identity()
	if shared.Equal(&identity) == 1 {
		return output, ErrInvalidWalletOutput
	}
	sharedBytes := shared.Bytes()
	defer clear(sharedBytes)
	ephemeral.Set(new(curve25519.Scalar))
	copy(output.EphemeralKey[:], ephemeralPublic.Bytes())
	copy(output.Commitment[:], combined[:])
	output.AmountLimbs = limbPoints
	tweak, err := outputTweak(sharedBytes, address.ChainID, outputPosition)
	if err != nil {
		return OutputRecord{}, err
	}
	defer tweak.Set(new(curve25519.Scalar))
	spendPublic, err := decodePointCT(address.SpendPublic[:], false)
	if err != nil {
		return OutputRecord{}, ErrInvalidWalletOutput
	}
	tweakPublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(tweak)
	oneTimePublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).Add(spendPublic, tweakPublic)
	if err := ValidatePublicKey(oneTimePublic.Bytes()); err != nil {
		return OutputRecord{}, ErrInvalidWalletOutput
	}
	copy(output.OneTimeKey[:], oneTimePublic.Bytes())

	var plaintext shield6NotePlaintext
	defer clear(plaintext.Amount[:])
	plaintext.Version, plaintext.ChainID, plaintext.AssetID, plaintext.OutputPosition = shield6NoteVersion, address.ChainID, assetID, outputPosition
	copy(plaintext.Amount[:], amountBytes[:])
	plaintext.Mask = witness.Mask
	plaintext.LimbMasks = witness.LimbMasks
	encodedPlaintext, err := rlp.EncodeToBytes(plaintext)
	if err != nil {
		return OutputRecord{}, ErrInvalidWalletOutput
	}
	defer clear(encodedPlaintext)
	defer clear(plaintext.Mask[:])
	defer clear(plaintext.LimbMasks[0][:])
	defer clear(plaintext.LimbMasks[1][:])
	aad, err := outputAAD(address.ChainID, outputPosition, output)
	if err != nil {
		return OutputRecord{}, err
	}
	aead, err := noteAEAD(sharedBytes, address.ChainID, outputPosition)
	if err != nil {
		return OutputRecord{}, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return OutputRecord{}, err
	}
	ciphertext := aead.Seal(nil, nonce, encodedPlaintext, aad)
	encodedNote, err := rlp.EncodeToBytes(shield6EncryptedNote{Version: shield6NoteVersion, Nonce: nonce, Ciphertext: ciphertext})
	if err != nil || len(encodedNote) > 4096 {
		return OutputRecord{}, ErrInvalidWalletOutput
	}
	output.EncryptedNote = encodedNote
	return output, nil
}

// ScanOutput checks whether output belongs to keys and, if it does, decrypts
// and validates its amount opening. A non-owned output returns (nil, false,
// nil); malformed owned ciphertext returns an error. accumulatorIndex is the
// output's position in the canonical Shield6 accumulator, supplied by the
// chain scanner; outputPosition is the position in the transaction envelope.
func ScanOutput(keys *WalletKeys, outputPosition, accumulatorIndex uint64, output OutputRecord) (*Note, bool, error) {
	if keys == nil || keys.ChainID == 0 || len(output.EncryptedNote) == 0 || len(output.EncryptedNote) > 4096 {
		return nil, false, ErrInvalidWalletOutput
	}
	ephemeralPublic, err := decodePointCT(output.EphemeralKey[:], false)
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	viewSecret, err := decodeScalar(keys.ViewSecret[:])
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	defer viewSecret.Set(new(curve25519.Scalar))
	shared := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(viewSecret, ephemeralPublic)
	var identity curve25519.PublicKey[curve25519.ConstantTimeOperations]
	identity.Identity()
	if shared.Equal(&identity) == 1 {
		return nil, false, ErrInvalidWalletOutput
	}
	sharedBytes := shared.Bytes()
	defer clear(sharedBytes)
	tweak, err := outputTweak(sharedBytes, keys.ChainID, outputPosition)
	if err != nil {
		return nil, false, err
	}
	defer tweak.Set(new(curve25519.Scalar))
	spendPublic, err := decodePointCT(keys.SpendPublic[:], false)
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	tweakPublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(tweak)
	oneTimePublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).Add(spendPublic, tweakPublic)
	if !bytesEqual(oneTimePublic.Bytes(), output.OneTimeKey[:]) {
		return nil, false, nil
	}
	if err := ValidateCommitmentEncoding(output.Commitment[:]); err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	aad, err := outputAAD(keys.ChainID, outputPosition, output)
	if err != nil {
		return nil, false, err
	}
	var encrypted shield6EncryptedNote
	if err := rlp.DecodeBytes(output.EncryptedNote, &encrypted); err != nil || encrypted.Version != shield6NoteVersion || len(encrypted.Nonce) != chacha20poly1305.NonceSizeX || len(encrypted.Ciphertext) < chacha20poly1305.Overhead {
		return nil, false, ErrInvalidWalletOutput
	}
	aead, err := noteAEAD(sharedBytes, keys.ChainID, outputPosition)
	if err != nil {
		return nil, false, err
	}
	plaintextBytes, err := aead.Open(nil, encrypted.Nonce, encrypted.Ciphertext, aad)
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	defer clear(plaintextBytes)
	var plaintext shield6NotePlaintext
	decodeErr := rlp.DecodeBytes(plaintextBytes, &plaintext)
	// Decoding can populate secret fields even when the encoded record is
	// malformed or has the wrong domain. Arrange the wipe before any early
	// return so rejected ciphertext does not leave plaintext in this frame.
	defer clear(plaintext.Amount[:])
	defer clear(plaintext.Mask[:])
	defer clear(plaintext.LimbMasks[0][:])
	defer clear(plaintext.LimbMasks[1][:])
	if decodeErr != nil || plaintext.Version != shield6NoteVersion || plaintext.ChainID != keys.ChainID || plaintext.OutputPosition != outputPosition || plaintext.AssetID != shield6NativeAssetID {
		return nil, false, ErrInvalidWalletOutput
	}
	amount := new(big.Int).SetBytes(plaintext.Amount[:])
	keepAmount := false
	defer func() {
		if !keepAmount {
			clearBigInt(amount)
		}
	}()
	commitment, err := Commitment128(amount, plaintext.Mask)
	if err != nil || commitment != output.Commitment {
		return nil, false, ErrInvalidWalletOutput
	}
	low, err := decodeScalar(plaintext.LimbMasks[0][:])
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	defer low.Set(new(curve25519.Scalar))
	high, err := decodeScalar(plaintext.LimbMasks[1][:])
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	defer high.Set(new(curve25519.Scalar))
	var amountLimbs [16]byte
	amount.FillBytes(amountLimbs[:])
	defer clear(amountLimbs[:])
	limbAmounts := [2]uint64{
		binary.BigEndian.Uint64(amountLimbs[8:]),
		binary.BigEndian.Uint64(amountLimbs[:8]),
	}
	limbMasks := [2]curve25519.Scalar{*low, *high}
	defer func() {
		for i := range limbMasks {
			limbMasks[i].Set(new(curve25519.Scalar))
		}
	}()
	for i := range limbAmounts {
		point := ringct.CalculateCommitment(new(curve25519.PublicKey[curve25519.ConstantTimeOperations]), ringct.LazyCommitment{Mask: limbMasks[i], Amount: limbAmounts[i]})
		if !bytesEqual(point.Bytes(), output.AmountLimbs[i][:]) {
			return nil, false, ErrInvalidWalletOutput
		}
	}
	combined, err := CombineAmountLimbs(output.AmountLimbs[0], output.AmountLimbs[1])
	if err != nil || combined != output.Commitment {
		return nil, false, ErrInvalidWalletOutput
	}
	spendSecret, err := decodeScalar(keys.SpendSecret[:])
	if err != nil {
		return nil, false, ErrInvalidWalletOutput
	}
	defer spendSecret.Set(new(curve25519.Scalar))
	oneTimeSecret := new(curve25519.Scalar).Add(spendSecret, tweak)
	if oneTimeSecret.Equal(new(curve25519.Scalar)) == 1 {
		return nil, false, ErrInvalidWalletOutput
	}
	defer oneTimeSecret.Set(new(curve25519.Scalar))
	derivedOneTimePublic := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(oneTimeSecret)
	if !bytesEqual(derivedOneTimePublic.Bytes(), output.OneTimeKey[:]) {
		return nil, false, ErrInvalidWalletOutput
	}
	spendKey := scalarBytes(*oneTimeSecret)
	keepAmount = true
	return &Note{
		ChainID: keys.ChainID, AssetID: plaintext.AssetID, Index: accumulatorIndex, Amount: amount,
		Witness:     WideWitness{Mask: plaintext.Mask, LimbMasks: plaintext.LimbMasks},
		SpendSecret: spendKey,
	}, true, nil
}

func outputTweak(shared []byte, chainID, index uint64) (*curve25519.Scalar, error) {
	if len(shared) != PointSize || chainID == 0 {
		return nil, ErrInvalidWalletOutput
	}
	var salt [16]byte
	binary.BigEndian.PutUint64(salt[:8], chainID)
	binary.BigEndian.PutUint64(salt[8:], index)
	for counter := uint32(0); ; counter++ {
		var counterBytes [4]byte
		binary.BigEndian.PutUint32(counterBytes[:], counter)
		info := string(append([]byte("TKM_SHIELD6_OUTPUT_TWEAK_V1/"), counterBytes[:]...))
		material, err := hkdf.Key(sha512.New, shared, salt[:], info, ScalarSize)
		if err != nil {
			return nil, err
		}
		tweak, parseErr := new(curve25519.Scalar).SetCanonicalBytes(material)
		clear(material)
		if parseErr == nil && tweak.Equal(new(curve25519.Scalar)) != 1 {
			return tweak, nil
		}
		if counter == ^uint32(0) {
			return nil, ErrInvalidWalletOutput
		}
	}
}

func noteAEAD(shared []byte, chainID, index uint64) (cipherAEAD, error) {
	var salt [16]byte
	binary.BigEndian.PutUint64(salt[:8], chainID)
	binary.BigEndian.PutUint64(salt[8:], index)
	key, err := hkdf.Key(sha512.New, shared, salt[:], "TKM_SHIELD6_NOTE_XCHACHA20POLY1305_V1", chacha20poly1305.KeySize)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	return chacha20poly1305.NewX(key)
}

func outputAAD(chainID, index uint64, output OutputRecord) ([]byte, error) {
	return rlp.EncodeToBytes(shield6OutputAAD{
		Domain: []byte("TKM_SHIELD6_OUTPUT_AAD_V1"), ChainID: chainID,
		Index: index, OneTimeKey: output.OneTimeKey,
		EphemeralKey: output.EphemeralKey, Commitment: output.Commitment,
		AmountLimbs: output.AmountLimbs,
	})
}

// cipherAEAD keeps this file's helpers independent of the concrete AEAD type.
type cipherAEAD interface {
	NonceSize() int
	Overhead() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}
