// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT
//
// The signing transcript and CLSAG equations below follow the MIT-licensed
// P2Pool/consensus v5 implementation. This adapter constructs pseudo-outputs
// from 128-bit Pedersen amounts; the pinned upstream verifier checks the
// resulting canonical signature over the aggregate commitment points.
package shielded6

import (
	"fmt"
	"io"
	"math/big"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct"
	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/ringct/clsag"
	"git.gammaspectra.live/P2Pool/consensus/v5/types"
)

const MaxCLSAGInputs = 16

// SigningInput128 contains a wallet's opening for one output whose amount can
// occupy the full unsigned 128-bit range. The amount and masks are never
// serialized into the transaction.
type SigningInput128 struct {
	Ring           [RingSize]RingMember
	SignerIndex    uint64
	SpendSecret    [ScalarSize]byte
	CommitmentMask [ScalarSize]byte
	Amount         *big.Int
}

// Clear wipes the secret scalar and amount witness copied into a signing
// input. Call it after signing or after abandoning a prepared input.
func (input *SigningInput128) Clear() {
	if input == nil {
		return
	}
	clear(input.SpendSecret[:])
	clear(input.CommitmentMask[:])
	clearBigInt(input.Amount)
}

// DeriveKeyImage returns the CLSAG link tag for a wallet-owned one-time key.
// Wallet scanners use it to exclude confirmed and pending spends before
// offering a note for signing. The public key is checked against the secret
// first so callers cannot query arbitrary image values using an unrelated key.
func DeriveKeyImage(publicKey, secretKey []byte) ([PointSize]byte, error) {
	var result [PointSize]byte
	public, err := decodePoint(publicKey, false)
	if err != nil {
		return result, ErrInvalidRing
	}
	secret, err := decodeScalar(secretKey)
	if err != nil || secret.Equal(new(curve25519.Scalar)) == 1 {
		return result, ErrInvalidRing
	}
	defer secret.Set(new(curve25519.Scalar))
	derived := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(secret)
	if !bytesEqual(derived.Bytes(), public.Bytes()) {
		return result, ErrInvalidRing
	}
	hashPoint := crypto.BiasedHashToPoint(new(curve25519.PublicKey[curve25519.ConstantTimeOperations]), public.Bytes())
	image := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(secret, hashPoint)
	if err := ValidatePublicKey(image.Bytes()); err != nil {
		return result, ErrInvalidRing
	}
	copy(result[:], image.Bytes())
	return result, nil
}

// SignCLSAGs128 signs the input set while balancing pseudo-output masks to the
// aggregate 128-bit output commitment masks. Its output is verified by the
// pinned consensus/v5 CLSAG verifier through VerifyCLSAG.
func SignCLSAGs128(message [32]byte, inputs []SigningInput128, sumOutputMasks [ScalarSize]byte, random io.Reader) ([]SignedInput, error) {
	if len(inputs) == 0 || len(inputs) > MaxCLSAGInputs || random == nil {
		return nil, ErrInvalidRing
	}
	sumOutputs, err := decodeScalar(sumOutputMasks[:])
	if err != nil {
		return nil, fmt.Errorf("sum of output masks: %w", err)
	}
	prepared := make([]preparedInput128, len(inputs))
	var sumPseudoMasks curve25519.Scalar
	defer func() {
		sumOutputs.Set(new(curve25519.Scalar))
		sumPseudoMasks.Set(new(curve25519.Scalar))
		for i := range prepared {
			if prepared[i].spend != nil {
				prepared[i].spend.Set(new(curve25519.Scalar))
			}
			if prepared[i].inputMask != nil {
				prepared[i].inputMask.Set(new(curve25519.Scalar))
			}
			if prepared[i].pseudoMask != nil {
				prepared[i].pseudoMask.Set(new(curve25519.Scalar))
			}
		}
	}()
	for i, input := range inputs {
		if len(input.Ring) != RingSize || input.SignerIndex >= RingSize || input.Amount == nil || input.Amount.Sign() < 0 || input.Amount.BitLen() > 128 {
			return nil, fmt.Errorf("input %d: %w", i, ErrInvalidRing)
		}
		spend, err := decodeScalar(input.SpendSecret[:])
		if err != nil || spend.Equal(new(curve25519.Scalar)) == 1 {
			return nil, fmt.Errorf("input %d spend secret is invalid", i)
		}
		defer spend.Set(new(curve25519.Scalar))
		inputMask, err := decodeScalar(input.CommitmentMask[:])
		if err != nil {
			return nil, fmt.Errorf("input %d commitment mask is invalid", i)
		}
		defer inputMask.Set(new(curve25519.Scalar))
		members := make(ringct.CommitmentRing[curve25519.ConstantTimeOperations], RingSize)
		for j, member := range input.Ring {
			key, err := decodePointCT(member.OneTimeKey[:], false)
			if err != nil {
				return nil, fmt.Errorf("input %d ring member %d key: %w", i, j, err)
			}
			commitment, err := decodePointCT(member.Commitment[:], true)
			if err != nil {
				return nil, fmt.Errorf("input %d ring member %d commitment: %w", i, j, err)
			}
			members[j] = [2]curve25519.PublicKey[curve25519.ConstantTimeOperations]{*key, *commitment}
		}
		secretPair := crypto.NewKeyPairFromPrivate[curve25519.ConstantTimeOperations](spend)
		matchesSigner := secretPair.PublicKey.Equal(&members[input.SignerIndex][0]) != 0
		// NewKeyPairFromPrivate copies the secret scalar. Keep only its public
		// key after the consistency check; the original scalar is separately
		// cleared by the signer cleanup below.
		secretPair.PrivateKey.Set(new(curve25519.Scalar))
		if !matchesSigner {
			return nil, fmt.Errorf("input %d secret does not match selected ring member", i)
		}
		// The input opening must match the selected real member. This also
		// prevents a wallet from signing with an amount unrelated to its note.
		var maskBytes [ScalarSize]byte
		copy(maskBytes[:], inputMask.Bytes())
		opening, err := Commitment128(input.Amount, maskBytes)
		clear(maskBytes[:])
		if err != nil || opening != input.Ring[input.SignerIndex].Commitment {
			return nil, fmt.Errorf("input %d amount/mask do not open selected commitment", i)
		}
		var pseudoMask curve25519.Scalar
		if i == len(inputs)-1 {
			pseudoMask.Subtract(sumOutputs, &sumPseudoMasks)
		} else {
			curve25519.RandomScalar(&pseudoMask, random)
			sumPseudoMasks.Add(&sumPseudoMasks, &pseudoMask)
		}
		pseudoOutBytes, err := Commitment128(input.Amount, scalarBytes(pseudoMask))
		if err != nil {
			return nil, err
		}
		pseudoOut, err := decodePointCT(pseudoOutBytes[:], false)
		if err != nil {
			return nil, err
		}
		prepared[i] = preparedInput128{
			members: members, signer: int(input.SignerIndex), spend: spend,
			inputMask: inputMask, pseudoMask: &pseudoMask, pseudoOut: pseudoOut,
		}
	}
	result := make([]SignedInput, len(prepared))
	for i := range prepared {
		signed, image, err := signOneCLSAG128(types.Hash(message), &prepared[i], random)
		if err != nil {
			return nil, fmt.Errorf("input %d CLSAG: %w", i, err)
		}
		encoded, err := signed.AppendBinary(make([]byte, 0, signed.BufferLength()))
		if err != nil || len(encoded) != CLSAGSize {
			return nil, ErrInvalidSignature
		}
		copy(result[i].Signature[:], encoded)
		copy(result[i].PseudoOutput[:], prepared[i].pseudoOut.Bytes())
		copy(result[i].KeyImage[:], image.Bytes())
	}
	return result, nil
}

type preparedInput128 struct {
	members    ringct.CommitmentRing[curve25519.ConstantTimeOperations]
	signer     int
	spend      *curve25519.Scalar
	inputMask  *curve25519.Scalar
	pseudoMask *curve25519.Scalar
	pseudoOut  *curve25519.PublicKey[curve25519.ConstantTimeOperations]
}

func scalarBytes(s curve25519.Scalar) [ScalarSize]byte {
	var result [ScalarSize]byte
	copy(result[:], s.Bytes())
	return result
}

func signOneCLSAG128(message types.Hash, input *preparedInput128, random io.Reader) (clsag.Signature[curve25519.ConstantTimeOperations], *curve25519.PublicKey[curve25519.ConstantTimeOperations], error) {
	var signature clsag.Signature[curve25519.ConstantTimeOperations]
	keyImageGenerator := crypto.BiasedHashToPoint(new(curve25519.PublicKey[curve25519.ConstantTimeOperations]), input.members[input.signer][0].Bytes())
	keyImage := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(input.spend, keyImageGenerator)
	maskDelta := new(curve25519.Scalar).Subtract(input.inputMask, input.pseudoMask)
	defer maskDelta.Set(new(curve25519.Scalar))
	D := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(maskDelta, keyImageGenerator)
	var nonce curve25519.Scalar
	defer nonce.Set(new(curve25519.Scalar))
	curve25519.RandomScalar(&nonce, random)
	A := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarBaseMult(&nonce)
	AH := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(&nonce, keyImageGenerator)
	transcript, err := clsagCoreSign128(message, input.members, keyImage, input.pseudoOut, D, input.signer, A, AH, random)
	if err != nil {
		return signature, nil, err
	}
	signature = transcript.signature
	challengeSpend := new(curve25519.Scalar).Multiply(&transcript.cMuP, input.spend)
	challengeMask := new(curve25519.Scalar).Multiply(&transcript.cMuC, maskDelta)
	defer challengeSpend.Set(new(curve25519.Scalar))
	defer challengeMask.Set(new(curve25519.Scalar))
	signature.S[input.signer] = *new(curve25519.Scalar).Subtract(&nonce, new(curve25519.Scalar).Add(challengeSpend, challengeMask))
	if err := signature.Verify(message, input.members, keyImage, input.pseudoOut); err != nil {
		return clsag.Signature[curve25519.ConstantTimeOperations]{}, nil, err
	}
	return signature, keyImage, nil
}

type clsagTranscript128 struct {
	signature clsag.Signature[curve25519.ConstantTimeOperations]
	cMuP      curve25519.Scalar
	cMuC      curve25519.Scalar
}

func clsagCoreSign128(message types.Hash, ring ringct.CommitmentRing[curve25519.ConstantTimeOperations], image, pseudoOut, D *curve25519.PublicKey[curve25519.ConstantTimeOperations], signer int, A, AH *curve25519.PublicKey[curve25519.ConstantTimeOperations], random io.Reader) (clsagTranscript128, error) {
	const prefix = "CLSAG_"
	const agg0 = "agg_0"
	const round = "round"
	invEight := new(curve25519.Scalar).Invert((&curve25519.PrivateKeyBytes{8}).Scalar())
	DInvEight := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(invEight, D)
	data := make([]byte, 0, ((2*len(ring))+5)*curve25519.PublicKeySize)
	data = append(data, prefix...)
	data = append(data, agg0...)
	data = append(data, make([]byte, curve25519.PublicKeySize-len(prefix)-len(agg0))...)
	P, C := make([]curve25519.PublicKey[curve25519.ConstantTimeOperations], len(ring)), make([]curve25519.PublicKey[curve25519.ConstantTimeOperations], len(ring))
	for i := range ring {
		P[i] = ring[i][0]
		data = append(data, ring[i][0].Bytes()...)
	}
	for i := range ring {
		C[i].Subtract(&ring[i][1], pseudoOut)
		data = append(data, ring[i][1].Bytes()...)
	}
	data = append(data, image.Bytes()...)
	data = append(data, DInvEight.Bytes()...)
	data = append(data, pseudoOut.Bytes()...)
	muP := crypto.ScalarDeriveLegacy(new(curve25519.Scalar), data)
	data[len(prefix)+len(agg0)-1] = '1'
	muC := crypto.ScalarDeriveLegacy(new(curve25519.Scalar), data)
	data = data[:((2*len(ring))+1)*curve25519.PublicKeySize]
	copy(data[len(prefix):], round)
	data = append(data, pseudoOut.Bytes()...)
	data = append(data, message[:]...)
	data = append(data, A.Bytes()...)
	data = append(data, AH.Bytes()...)
	challenge := crypto.ScalarDeriveLegacy(new(curve25519.Scalar), data)
	var c1 curve25519.Scalar
	c1.Set(challenge)
	responses := make([]curve25519.Scalar, len(ring))
	for i := range responses {
		curve25519.RandomScalar(&responses[i], random)
	}
	var cP, cC curve25519.Scalar
	var L, PH, R, tmp curve25519.PublicKey[curve25519.ConstantTimeOperations]
	for j := signer + 1; j < signer+len(ring); j++ {
		i := j % len(ring)
		cP.Multiply(muP, challenge)
		cC.Multiply(muC, challenge)
		L.ScalarBaseMult(&responses[i])
		L.Add(&L, tmp.DoubleScalarMult(&cP, &P[i], &cC, &C[i]))
		crypto.BiasedHashToPoint(&PH, P[i].Bytes())
		R.ScalarMult(&responses[i], &PH)
		R.Add(&R, tmp.DoubleScalarMult(&cP, image, &cC, D))
		data = data[:((2*len(ring))+3)*curve25519.PublicKeySize]
		data = append(data, L.Bytes()...)
		data = append(data, R.Bytes()...)
		crypto.ScalarDeriveLegacy(challenge, data)
		if i == len(ring)-1 {
			c1.Set(challenge)
		}
	}
	return clsagTranscript128{
		signature: clsag.Signature[curve25519.ConstantTimeOperations]{D: DInvEight.AsBytes(), S: responses, C1: c1},
		cMuP:      *new(curve25519.Scalar).Multiply(challenge, muP),
		cMuC:      *new(curve25519.Scalar).Multiply(challenge, muC),
	}, nil
}
