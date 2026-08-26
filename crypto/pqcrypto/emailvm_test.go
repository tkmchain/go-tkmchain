package pqcrypto

import (
	"encoding/hex"
	"testing"
)

func TestEmailVMKeyMatchesWebWallet(t *testing.T) {
	seed := make([]byte, MLDSA87SeedSize)
	for i := range seed {
		seed[i] = 1
	}
	privateKey, err := EmailVMPrivateKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(privateKey)
	if got := hex.EncodeToString(privateKey); got != "2b596e832b12d80e665ff8eb6070715532cf6859307bbb4dc8e423fb33a41e3e" {
		t.Fatalf("private key = %s", got)
	}
	publicKey, err := EmailVMPublicKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(publicKey); got != "b8446ba6841552661ec38ccb0b199b136a1492e101cb1567e000d6e847503f35" {
		t.Fatalf("public key = %s", got)
	}
}
