package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	shieldedNotePayloadFormat   = "TKM_SHIELDED_NOTE_PAYLOAD_V3"
	shieldedNotePayloadFormatV2 = "TKM_SHIELDED_NOTE_PAYLOAD_V4"
	shieldedNoteKDFInfo         = "TKM_SHIELDED_NOTE_X25519_XCHACHA20POLY1305_V1"
)

type shieldedNoteOpening struct {
	Format         string `json:"format"`
	Version        uint64 `json:"version,omitempty"`
	Recipient      string `json:"recipient"`
	OwnerSecret    string `json:"ownerSecret,omitempty"`
	AssetID        string `json:"assetId"`
	NoteValueWei   string `json:"noteValueWei"`
	NoteRandomness string `json:"noteRandomness"`
	Commitment     string `json:"commitment"`
	Nullifier      string `json:"nullifier"`
}

func noteOpeningV2(recipient common.Address, asset, value, randomness *big.Int, commitment, nullifier common.Hash) shieldedNoteOpening {
	return shieldedNoteOpening{
		Format:         shieldedNotePayloadFormatV2,
		Version:        core.ShieldedTxVersionV2,
		Recipient:      recipient.Hex(),
		AssetID:        asset.String(),
		NoteValueWei:   value.String(),
		NoteRandomness: randomness.String(),
		Commitment:     commitment.Hex(),
		Nullifier:      nullifier.Hex(),
	}
}

func parseViewPublicKey(raw string) ([]byte, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "0x")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return nil, errorsViewKey(raw)
	}
	return decoded, nil
}

func errorsViewKey(raw string) error {
	return fmt.Errorf("recipient viewing public key must be 32-byte hexadecimal data (got %d hex characters)", len(raw))
}

func encryptShieldedNote(commitment common.Hash, opening shieldedNoteOpening, recipientViewKey []byte) (core.ShieldedOutput, error) {
	curve := ecdh.X25519()
	recipient, err := curve.NewPublicKey(recipientViewKey)
	if err != nil {
		return core.ShieldedOutput{}, fmt.Errorf("invalid recipient viewing public key: %w", err)
	}
	ephemeral, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return core.ShieldedOutput{}, err
	}
	shared, err := ephemeral.ECDH(recipient)
	if err != nil {
		return core.ShieldedOutput{}, fmt.Errorf("derive shielded note key: %w", err)
	}
	keyMaterial := make([]byte, chacha20poly1305.KeySize+1)
	reader := hkdf.New(sha256.New, shared, commitment[:], []byte(shieldedNoteKDFInfo))
	if _, err := io.ReadFull(reader, keyMaterial); err != nil {
		return core.ShieldedOutput{}, err
	}
	aead, err := chacha20poly1305.NewX(keyMaterial[:chacha20poly1305.KeySize])
	if err != nil {
		return core.ShieldedOutput{}, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return core.ShieldedOutput{}, err
	}
	if opening.Format == "" {
		opening.Format = shieldedNotePayloadFormat
	}
	opening.Commitment = commitment.Hex()
	plain, err := json.Marshal(opening)
	if err != nil {
		return core.ShieldedOutput{}, err
	}
	aad := append([]byte(shieldedNoteKDFInfo), commitment[:]...)
	ciphertext := aead.Seal(nil, nonce, plain, aad)
	return core.ShieldedOutput{
		Commitment:       commitment,
		PayloadHash:      crypto.Keccak256Hash(ciphertext),
		EphemeralPubKey:  ephemeral.PublicKey().Bytes(),
		ViewTag:          []byte{keyMaterial[chacha20poly1305.KeySize]},
		EncryptedPayload: ciphertext,
		Nonce:            nonce,
	}, nil
}

func decoyShieldedOutput(commitment common.Hash) (core.ShieldedOutput, error) {
	payload := randomBytes(outputEncryptedBytesSize)
	return core.ShieldedOutput{
		Commitment:       commitment,
		PayloadHash:      crypto.Keccak256Hash(payload),
		EphemeralPubKey:  randomBytes(32),
		ViewTag:          randomBytes(1),
		EncryptedPayload: payload,
		Nonce:            randomBytes(chacha20poly1305.NonceSizeX),
	}, nil
}

// legacyMetadataOutput preserves the original pool signer API for callers that
// have not adopted shielded payment codes. Wallet builders always require a
// viewing key and never use this compatibility payload.
func legacyMetadataOutput(commitment common.Hash, requestID string, index int, source string) core.ShieldedOutput {
	payload, _ := json.Marshal(map[string]string{
		"format":    "TKM_SHIELDED_NOTE_PAYLOAD_V2",
		"requestId": requestID,
		"output":    fmt.Sprintf("%d", index),
		"source":    source,
	})
	if len(payload) < outputEncryptedBytesSize {
		payload = append(payload, randomBytes(outputEncryptedBytesSize-len(payload))...)
	}
	return core.ShieldedOutput{
		Commitment:       commitment,
		PayloadHash:      crypto.Keccak256Hash(payload),
		EphemeralPubKey:  randomBytes(32),
		ViewTag:          randomBytes(1),
		EncryptedPayload: payload,
		Nonce:            randomBytes(chacha20poly1305.NonceSizeX),
	}
}

func noteOpening(recipient common.Address, owner, asset, value, randomness *big.Int, commitment, nullifier common.Hash) shieldedNoteOpening {
	return shieldedNoteOpening{
		Recipient:      recipient.Hex(),
		OwnerSecret:    owner.String(),
		AssetID:        asset.String(),
		NoteValueWei:   value.String(),
		NoteRandomness: randomness.String(),
		Commitment:     commitment.Hex(),
		Nullifier:      nullifier.Hex(),
	}
}
