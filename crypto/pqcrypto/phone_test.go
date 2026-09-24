package pqcrypto

import (
	"bytes"
	"testing"
)

func TestPhoneV2EnvelopeRoundTripAndContextBinding(t *testing.T) {
	seed, publicKey, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	nonce := []byte("phone-nonce-1234567890123456")
	from, to := "+8979000000001", "+8979000000002"
	context := PhoneV2Context(8979, from, to, nonce)
	ciphertext, err := SealShieldedV3(publicKey, []byte("hello from Antartical"), context)
	if err != nil {
		t.Fatal(err)
	}
	if len(ciphertext) != ShieldedV3CiphertextSize {
		t.Fatalf("ciphertext size = %d, want %d", len(ciphertext), ShieldedV3CiphertextSize)
	}
	if err := ValidatePhoneV2Envelope(ciphertext, 8979, from, to, nonce); err != nil {
		t.Fatal(err)
	}
	plaintext, err := OpenShieldedV3(seed, ciphertext, context)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plaintext, []byte("hello from Antartical")) {
		t.Fatalf("plaintext mismatch: %q", plaintext)
	}
	if err := ValidatePhoneV2Envelope(ciphertext, 8979, to, from, nonce); err == nil {
		t.Fatal("reversed participants accepted")
	}
	if err := ValidatePhoneV2Envelope(ciphertext, 8979, from, to, []byte("different nonce")); err == nil {
		t.Fatal("wrong nonce accepted")
	}
}

func TestPhoneV2RejectsVariablePlaintextEnvelope(t *testing.T) {
	_, publicKey, err := GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	context := PhoneV2Context(8979, "from", "to", []byte("nonce"))
	ciphertext, err := SealShieldedV3(publicKey, []byte("payload"), context)
	if err != nil {
		t.Fatal(err)
	}
	truncated := ciphertext[:len(ciphertext)-1]
	if err := ValidatePhoneV2Envelope(truncated, 8979, "from", "to", []byte("nonce")); err == nil {
		t.Fatal("truncated envelope accepted")
	}
}
