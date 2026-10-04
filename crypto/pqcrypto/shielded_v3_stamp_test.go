package pqcrypto

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestShield3PrivateStamp(t *testing.T) {
	seed := make([]byte, 32)
	record, err := CreateShieldedV3Stamp(seed, 8979, "Private Person", "  united states  ")
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
	if bytes.Contains(encoded, []byte("Private Person")) || bytes.Contains(encoded, []byte("United States")) {
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
			if err != nil || text.Name != "Private Person" || text.Country != "United States" {
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

func TestShield3StampRejectsUnknownCountry(t *testing.T) {
	seed := make([]byte, 32)
	if _, err := CreateShieldedV3Stamp(seed, 8979, "Private Person", "Middle Earth"); err == nil {
		t.Fatal("unknown country accepted")
	}
	for _, country := range []string{"Japan", "Côte d'Ivoire", "Antarctica"} {
		if _, ok := CanonicalStampCountry(country); !ok {
			t.Errorf("supported country %q rejected", country)
		}
	}
	countries := StampCountries()
	if got := len(countries); got != 249 {
		t.Fatalf("ISO 3166 country/territory count = %d, want 249", got)
	}
	seen := make(map[string]bool, len(countries))
	for _, country := range countries {
		canonical, ok := CanonicalStampCountry(country)
		if !ok || canonical != country || seen[country] {
			t.Errorf("country list contains invalid or duplicate entry %q", country)
		}
		seen[country] = true
	}
}
