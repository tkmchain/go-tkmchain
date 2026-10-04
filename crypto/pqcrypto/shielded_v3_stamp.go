package pqcrypto

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type ShieldedV3StampText struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}
type ShieldedV3StampRecord struct {
	ChainID    uint64   `json:"chainId"`
	Commitment [64]byte `json:"commitment"`
	Ciphertext []byte   `json:"ciphertext"`
	Signature  []byte   `json:"signature"`
}

func stampMessage(r *ShieldedV3StampRecord) []byte {
	ctx := ShieldedV3Context{ChainID: r.ChainID, Purpose: ShieldedV3Stamp, Commitment: r.Commitment}
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_ADDRESS_STAMP_V1"))
	var chain [8]byte
	for i := range chain {
		chain[7-i] = byte(ctx.ChainID >> (8 * i))
	}
	h.Write(chain[:])
	h.Write(ctx.Commitment[:])
	h.Write(r.Ciphertext)
	return h.Sum(nil)
}

// CreateShieldedV3Stamp seals identity labels before account import. The
// random hiding commitment and encrypted labels disclose no name or country.
func CreateShieldedV3Stamp(seed []byte, chainID uint64, name, country string) (*ShieldedV3StampRecord, error) {
	text := ShieldedV3StampText{strings.TrimSpace(name), strings.TrimSpace(country)}
	if chainID == 0 {
		return nil, errors.New("stamp requires a valid chain ID")
	}
	if text.Name == "" || len(text.Name) > 120 || !utf8.ValidString(text.Name) {
		return nil, errors.New("stamp requires a name (up to 120 bytes) and country (up to 80 bytes)")
	}
	if len(text.Country) > 80 || !utf8.ValidString(text.Country) {
		return nil, errors.New("stamp country must be a valid country name of up to 80 bytes")
	}
	canonicalCountry, ok := CanonicalStampCountry(text.Country)
	if !ok {
		return nil, errors.New("stamp country is not recognized; choose one of the supported ISO 3166 countries or territories")
	}
	text.Country = canonicalCountry
	plain, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	var random [64]byte
	if _, err = rand.Read(random[:]); err != nil {
		return nil, err
	}
	defer clear(random[:])
	h := sha512.New()
	h.Write([]byte("TKM_SHIELD3_STAMP_HIDING_V1"))
	h.Write(random[:])
	h.Write(plain)
	record := &ShieldedV3StampRecord{ChainID: chainID}
	copy(record.Commitment[:], h.Sum(nil))
	viewSeed, err := DeriveShieldedV3ViewKey(seed, chainID, ShieldedV3Stamp)
	if err != nil {
		return nil, err
	}
	defer clear(viewSeed)
	pub, err := ShieldedV3ViewPublicKey(viewSeed)
	if err != nil {
		return nil, err
	}
	record.Ciphertext, err = SealShieldedV3(pub, plain, ShieldedV3Context{ChainID: chainID, Purpose: ShieldedV3Stamp, Commitment: record.Commitment})
	if err != nil {
		return nil, err
	}
	key, err := NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	record.Signature, err = SignMLDSA87(key, stampMessage(record))
	return record, err
}
func VerifyShieldedV3Stamp(publicKey []byte, record *ShieldedV3StampRecord) bool {
	return record != nil && record.ChainID != 0 && record.Commitment != ([64]byte{}) && ValidateShieldedV3CiphertextContext(record.Ciphertext, ShieldedV3Context{ChainID: record.ChainID, Purpose: ShieldedV3Stamp, Commitment: record.Commitment}) == nil && VerifyMLDSA87(publicKey, stampMessage(record), record.Signature)
}
func OpenShieldedV3Stamp(stampKey []byte, record *ShieldedV3StampRecord) (ShieldedV3StampText, error) {
	var text ShieldedV3StampText
	if record == nil {
		return text, ErrInvalidShieldedV3Context
	}
	plain, err := OpenShieldedV3(stampKey, record.Ciphertext, ShieldedV3Context{ChainID: record.ChainID, Purpose: ShieldedV3Stamp, Commitment: record.Commitment})
	if err != nil {
		return text, err
	}
	defer clear(plain)
	err = json.Unmarshal(plain, &text)
	if err == nil {
		text.Name = strings.TrimSpace(text.Name)
		country, ok := CanonicalStampCountry(text.Country)
		if text.Name == "" || len(text.Name) > 120 || !utf8.ValidString(text.Name) || !ok {
			err = ErrInvalidShieldedV3Ciphertext
		} else {
			text.Country = country
		}
	}
	return text, err
}
