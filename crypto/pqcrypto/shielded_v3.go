// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"bytes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	ShieldedV3Version          = 3
	ShieldedV3KEMPublicKeySize = mlkem.EncapsulationKeySize1024
	ShieldedV3ViewKeySize      = mlkem.SeedSize
	ShieldedV3MaxPlaintextSize = 4096
	shieldedV3HeaderSize       = 4 + 1 + 1 + 1 + 8 + sha512.Size
	shieldedV3PaddedSize       = 2 + ShieldedV3MaxPlaintextSize
	ShieldedV3CiphertextSize   = shieldedV3HeaderSize + mlkem.CiphertextSize1024 + chacha20poly1305.NonceSizeX + shieldedV3PaddedSize + chacha20poly1305.Overhead
	shieldedV3Suite            = 1 // ML-KEM-1024 / HKDF-SHA-512 / XChaCha20-Poly1305
)

// ShieldedV3Purpose separates notes, stamps and selected-payment auditor capsules.
// Use separate keys for each purpose; a stamp key must not reveal note contents.
type ShieldedV3Purpose byte

const (
	ShieldedV3Incoming   ShieldedV3Purpose = 1
	ShieldedV3Outgoing   ShieldedV3Purpose = 2
	ShieldedV3Stamp      ShieldedV3Purpose = 3
	ShieldedV3Disclosure ShieldedV3Purpose = 4
)

var (
	ErrInvalidShieldedV3Context    = errors.New("invalid Shield3 encryption context")
	ErrInvalidShieldedV3Ciphertext = errors.New("invalid Shield3 ciphertext")
	ErrShieldedV3PlaintextTooLarge = errors.New("Shield3 plaintext exceeds fixed payload size")
)

// ShieldedV3Context is public associated data authenticated by the encryption.
// Commitment must be a hiding commitment with fresh secret randomness, never a
// hash of an amount, name or country alone. It is not a transaction identifier:
// including a hash of this ciphertext would introduce a circular dependency.
type ShieldedV3Context struct {
	ChainID    uint64
	Purpose    ShieldedV3Purpose
	Commitment [sha512.Size]byte
}

// GenerateShieldedV3ViewKey returns a secret 64-byte ML-KEM seed and its public
// encapsulation key. Publishing the public key does not disclose the seed.
func GenerateShieldedV3ViewKey() (seed, publicKey []byte, err error) {
	key, err := mlkem.GenerateKey1024()
	if err != nil {
		return nil, nil, err
	}
	return key.Bytes(), key.EncapsulationKey().Bytes(), nil
}

// DeriveShieldedV3ViewKey derives independent incoming, outgoing, stamp or auditor seeds
// from wallet secret material. A full view key contains both incoming and
// outgoing seeds; neither seed grants spending authority. Security remains
// limited by the entropy of the wallet secret. Callers must protect and clear
// all returned seeds just as they protect wallet backup material.
func DeriveShieldedV3ViewKey(walletSecret []byte, chainID uint64, purpose ShieldedV3Purpose) ([]byte, error) {
	if len(walletSecret) < MLDSA87SeedSize {
		return nil, ErrInvalidPrivateKey
	}
	if !validShieldedV3Purpose(purpose) || chainID == 0 {
		return nil, ErrInvalidShieldedV3Context
	}
	var salt [9]byte
	binary.BigEndian.PutUint64(salt[:8], chainID)
	salt[8] = byte(purpose)
	return hkdf.Key(sha512.New, walletSecret, salt[:], "TKM_SHIELD3_MLKEM1024_VIEW_SEED_V1", mlkem.SeedSize)
}

// ShieldedV3ViewPublicKey reconstructs a public ML-KEM-1024 key from its seed.
func ShieldedV3ViewPublicKey(seed []byte) ([]byte, error) {
	key, err := mlkem.NewDecapsulationKey1024(seed)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}
	return key.EncapsulationKey().Bytes(), nil
}

// SealShieldedV3 encapsulates a fresh ML-KEM secret and encrypts a fixed-size
// padded payload. No recipient public key, amount or viewing tag appears in the
// resulting envelope. This protects payload confidentiality; it does not hide
// outer transaction metadata or replace the consensus spend proof.
func SealShieldedV3(publicKey, plaintext []byte, context ShieldedV3Context) ([]byte, error) {
	header, err := shieldedV3Header(context)
	if err != nil {
		return nil, err
	}
	if len(plaintext) > ShieldedV3MaxPlaintextSize {
		return nil, ErrShieldedV3PlaintextTooLarge
	}
	ek, err := mlkem.NewEncapsulationKey1024(publicKey)
	if err != nil {
		return nil, ErrInvalidShieldedViewKey
	}
	shared, kemCiphertext := ek.Encapsulate()
	defer clear(shared)
	aead, err := shieldedV3AEAD(shared, publicKey, header, kemCiphertext)
	if err != nil {
		return nil, err
	}
	padded := make([]byte, shieldedV3PaddedSize)
	defer clear(padded)
	binary.BigEndian.PutUint16(padded[:2], uint16(len(plaintext)))
	copy(padded[2:], plaintext)
	result := make([]byte, 0, ShieldedV3CiphertextSize)
	result = append(result, header...)
	result = append(result, kemCiphertext...)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	result = append(result, nonce...)
	// Authenticate the complete header, KEM ciphertext and nonce.
	return aead.Seal(result, nonce, padded, result), nil
}

// OpenShieldedV3 decrypts only with a private ML-KEM seed and the exact expected
// context. All malformed envelopes, wrong keys and authentication failures use
// the same ciphertext error. ML-KEM implicit rejection alone does not signal a
// wrong key: the AEAD authentication check is mandatory.
func OpenShieldedV3(seed, envelope []byte, context ShieldedV3Context) ([]byte, error) {
	header, err := shieldedV3Header(context)
	if err != nil {
		return nil, err
	}
	if len(envelope) != ShieldedV3CiphertextSize || !bytes.Equal(envelope[:shieldedV3HeaderSize], header) {
		return nil, ErrInvalidShieldedV3Ciphertext
	}
	dk, err := mlkem.NewDecapsulationKey1024(seed)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}
	kemEnd := shieldedV3HeaderSize + mlkem.CiphertextSize1024
	kemCiphertext := envelope[shieldedV3HeaderSize:kemEnd]
	shared, err := dk.Decapsulate(kemCiphertext)
	if err != nil {
		return nil, ErrInvalidShieldedV3Ciphertext
	}
	defer clear(shared)
	aead, err := shieldedV3AEAD(shared, dk.EncapsulationKey().Bytes(), header, kemCiphertext)
	if err != nil {
		return nil, err
	}
	return openShieldedV3Payload(aead, envelope)
}

func openShieldedV3Payload(aead cipher.AEAD, envelope []byte) ([]byte, error) {
	kemEnd := shieldedV3HeaderSize + mlkem.CiphertextSize1024
	payloadStart := kemEnd + chacha20poly1305.NonceSizeX
	padded, err := aead.Open(nil, envelope[kemEnd:payloadStart], envelope[payloadStart:], envelope[:payloadStart])
	if err != nil {
		return nil, ErrInvalidShieldedV3Ciphertext
	}
	defer clear(padded)
	size := int(binary.BigEndian.Uint16(padded[:2]))
	if size > ShieldedV3MaxPlaintextSize {
		return nil, ErrInvalidShieldedV3Ciphertext
	}
	for _, b := range padded[2+size:] {
		if b != 0 {
			return nil, ErrInvalidShieldedV3Ciphertext
		}
	}
	return bytes.Clone(padded[2 : 2+size]), nil
}

func validShieldedV3Purpose(purpose ShieldedV3Purpose) bool {
	return purpose >= ShieldedV3Incoming && purpose <= ShieldedV3Disclosure
}

func shieldedV3Header(context ShieldedV3Context) ([]byte, error) {
	if context.ChainID == 0 || !validShieldedV3Purpose(context.Purpose) || context.Commitment == ([sha512.Size]byte{}) {
		return nil, ErrInvalidShieldedV3Context
	}
	header := make([]byte, shieldedV3HeaderSize)
	copy(header, "TKPQ")
	header[4], header[5], header[6] = ShieldedV3Version, shieldedV3Suite, byte(context.Purpose)
	binary.BigEndian.PutUint64(header[7:15], context.ChainID)
	copy(header[15:], context.Commitment[:])
	return header, nil
}

func shieldedV3RecordKey(shared, publicKey, header, kemCiphertext []byte) ([]byte, error) {
	// Bind the key derivation to the recipient key and complete KEM transcript,
	// in addition to authenticating the public transcript as associated data.
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_ENCRYPTION_TRANSCRIPT_V1"))
	h.Write(publicKey)
	h.Write(header)
	h.Write(kemCiphertext)
	key, err := hkdf.Key(sha512.New, shared, h.Sum(nil), "TKM_SHIELD3_XCHACHA20POLY1305_KEY_V1", chacha20poly1305.KeySize)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func shieldedV3AEAD(shared, publicKey, header, kemCiphertext []byte) (cipher.AEAD, error) {
	key, err := shieldedV3RecordKey(shared, publicKey, header, kemCiphertext)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	return chacha20poly1305.NewX(key)
}

// ValidateShieldedV3CiphertextContext performs public framing checks for
// consensus. AEAD authentication still requires the recipient's private key.
func ValidateShieldedV3CiphertextContext(data []byte, expected ShieldedV3Context) error {
	if !validShieldedV3Purpose(expected.Purpose) || expected.ChainID == 0 || len(data) != ShieldedV3CiphertextSize || !bytes.Equal(data[:4], []byte("TKPQ")) || data[4] != ShieldedV3Version || data[5] != shieldedV3Suite || data[6] != byte(expected.Purpose) || binary.BigEndian.Uint64(data[7:15]) != expected.ChainID || !bytes.Equal(data[15:79], expected.Commitment[:]) {
		return ErrInvalidShieldedV3Ciphertext
	}
	return nil
}

// ShieldedV3RecordKey exports a key for one authenticated ciphertext only.
// It never exports the wallet's reusable ML-KEM viewing seed.
func ShieldedV3RecordKey(seed, envelope []byte, context ShieldedV3Context) ([]byte, error) {
	if err := ValidateShieldedV3CiphertextContext(envelope, context); err != nil {
		return nil, err
	}
	dk, err := mlkem.NewDecapsulationKey1024(seed)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}
	kem := envelope[shieldedV3HeaderSize : shieldedV3HeaderSize+mlkem.CiphertextSize1024]
	shared, err := dk.Decapsulate(kem)
	if err != nil {
		return nil, ErrInvalidShieldedV3Ciphertext
	}
	defer clear(shared)
	key, err := shieldedV3RecordKey(shared, dk.EncapsulationKey().Bytes(), envelope[:shieldedV3HeaderSize], kem)
	if err != nil {
		return nil, err
	}
	plaintext, err := OpenShieldedV3RecordKey(key, envelope, context)
	clear(plaintext)
	if err != nil {
		clear(key)
		return nil, err
	}
	return key, nil
}

// OpenShieldedV3RecordKey opens a selected record using its disclosure key.
func OpenShieldedV3RecordKey(key, envelope []byte, context ShieldedV3Context) ([]byte, error) {
	if err := ValidateShieldedV3CiphertextContext(envelope, context); err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, ErrInvalidShieldedV3Ciphertext
	}
	return openShieldedV3Payload(aead, envelope)
}
