// Copyright 2026 The go-ethereum Authors
// SPDX-License-Identifier: MIT

package recipient

import (
	"crypto/hkdf"
	cryptorand "crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"io"
	"math/big"

	"git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto/curve25519"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

const maxEphemeralAttempts = 4096

var ErrNoBoundedTweak = errors.New("recipient relation: cannot sample bounded output tweak")

// CreateOutput samples an output accepted by Circuit and readable by the
// existing Shield6 ScanOutput. Its private ephemeral scalar is returned solely
// for the proof witness and must be cleared after use. No recipient secret is
// required or returned. Entropy failures never fall back to weaker randomness.
func CreateOutput(address shielded6.Address, position uint64, amount *big.Int, witness shielded6.WideWitness, random io.Reader) (shielded6.OutputRecord, [32]byte, error) {
	if address.Validate() != nil || position >= 16 || amount == nil || amount.Sign() < 0 || amount.BitLen() > 128 {
		return shielded6.OutputRecord{}, [32]byte{}, shielded6.ErrInvalidWalletOutput
	}
	if random == nil {
		random = cryptorand.Reader
	}
	view, err := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).SetBytes(address.ViewPublic[:])
	if err != nil {
		return shielded6.OutputRecord{}, [32]byte{}, err
	}
	// Bound even a malicious/randomly failing reader. With uniform entropy the
	// failure probability of all bounded independent resamplings is negligible.
	for attempt := 0; attempt < maxEphemeralAttempts; attempt++ {
		var entropy [64]byte
		if _, err := io.ReadFull(random, entropy[:]); err != nil {
			clear(entropy[:])
			return shielded6.OutputRecord{}, [32]byte{}, err
		}
		scalar, err := new(curve25519.Scalar).SetUniformBytes(entropy[:])
		clear(entropy[:])
		if err != nil {
			return shielded6.OutputRecord{}, [32]byte{}, err
		}
		if scalar.Equal(new(curve25519.Scalar)) == 1 {
			scalar.Set(new(curve25519.Scalar))
			continue
		}
		shared := new(curve25519.PublicKey[curve25519.ConstantTimeOperations]).ScalarMult(scalar, view).Bytes()
		ok, err := hasBoundedTweak(shared, address.ChainID, position)
		clear(shared)
		if err != nil {
			scalar.Set(new(curve25519.Scalar))
			return shielded6.OutputRecord{}, [32]byte{}, err
		}
		if !ok {
			scalar.Set(new(curve25519.Scalar))
			continue
		}
		var secret [32]byte
		copy(secret[:], scalar.Bytes())
		scalar.Set(new(curve25519.Scalar))
		output, err := shielded6.CreateOutputWithEphemeral(address, position, shielded6.AssetTKM, amount, witness, secret, random)
		if err != nil {
			clear(secret[:])
			return shielded6.OutputRecord{}, [32]byte{}, err
		}
		return output, secret, nil
	}
	return shielded6.OutputRecord{}, [32]byte{}, ErrNoBoundedTweak
}
func hasBoundedTweak(shared []byte, chain, position uint64) (bool, error) {
	var salt [16]byte
	binary.BigEndian.PutUint64(salt[:8], chain)
	binary.BigEndian.PutUint64(salt[8:], position)
	for i := uint32(0); i < TweakCandidates; i++ {
		info := append([]byte("TKM_SHIELD6_OUTPUT_TWEAK_V1/"), 0, 0, 0, byte(i))
		material, err := hkdf.Key(sha512.New, shared, salt[:], string(info), 32)
		if err != nil {
			return false, err
		}
		scalar, parseErr := new(curve25519.Scalar).SetCanonicalBytes(material)
		clear(material)
		if parseErr == nil {
			valid := scalar.Equal(new(curve25519.Scalar)) != 1
			scalar.Set(new(curve25519.Scalar))
			if valid {
				return true, nil
			}
		}
	}
	return false, nil
}
