// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package pqcrypto

import (
	"crypto/sha512"
	"encoding/binary"
)

// PhoneV2MaxPlaintextSize is the fixed plaintext limit for the Antartical
// phone envelope. The resulting ciphertext has a constant size.
const PhoneV2MaxPlaintextSize = ShieldedV3MaxPlaintextSize

// PhoneV2Context derives the public authenticated context for a phone
// envelope. Phone numbers and the nonce are associated data; they are not key
// material. The ML-KEM secret is required to recover the payload.
func PhoneV2Context(chainID uint64, from, to string, nonce []byte) ShieldedV3Context {
	h := sha512.New()
	h.Write([]byte("TKM_PHONE_ANTARTICAL_CONTEXT_V1"))
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], chainID)
	h.Write(number[:])
	writePhoneV2Field(h, []byte(from))
	writePhoneV2Field(h, []byte(to))
	writePhoneV2Field(h, nonce)
	var commitment [sha512.Size]byte
	copy(commitment[:], h.Sum(nil))
	return ShieldedV3Context{ChainID: chainID, Purpose: ShieldedV3Phone, Commitment: commitment}
}

// ValidatePhoneV2Envelope performs public framing checks for an Antartical
// phone ciphertext. It does not decrypt or reveal the payload.
func ValidatePhoneV2Envelope(data []byte, chainID uint64, from, to string, nonce []byte) error {
	return ValidateShieldedV3CiphertextContext(data, PhoneV2Context(chainID, from, to, nonce))
}

func writePhoneV2Field(h interface{ Write([]byte) (int, error) }, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	h.Write(length[:])
	h.Write(value)
}
