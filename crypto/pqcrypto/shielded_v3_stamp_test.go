package pqcrypto

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestShield3PrivateStamp(t *testing.T) {
	seed := make([]byte, 32)
	record, err := CreateShieldedV3Stamp(seed, 8979, "Private Person", "Private Country")
	if err != nil {
		t.Fatal(err)
	}
	key, err := NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyShieldedV3Stamp(PublicKeyBytes(key), record) {
		t.Fatal("stamp signature invalid")
	}
	encoded, _ := json.Marshal(record)
	if bytes.Contains(encoded, []byte("Private Person")) || bytes.Contains(encoded, []byte("Private Country")) {
		t.Fatal("stamp labels exposed")
	}
	for _, role := range []ShieldedV3Purpose{ShieldedV3Incoming, ShieldedV3Outgoing, ShieldedV3Stamp} {
		private, err := DeriveShieldedV3ViewKey(seed, 8979, role)
		if err != nil {
			t.Fatal(err)
		}
		text, err := OpenShieldedV3Stamp(private, record)
		clear(private)
		if role == ShieldedV3Stamp {
			if err != nil || text.Name != "Private Person" || text.Country != "Private Country" {
				t.Fatal("stamp key failed")
			}
		} else if err == nil {
			t.Fatal("viewing key decrypted stamp")
		}
	}
	record.Ciphertext[len(record.Ciphertext)-1] ^= 1
	if VerifyShieldedV3Stamp(PublicKeyBytes(key), record) {
		t.Fatal("accepted tampered stamp")
	}
}
