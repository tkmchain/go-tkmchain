package keystore

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestShield3StampedAccountBackup(t *testing.T) {
	seed := make([]byte, 32)
	ks := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	account, err := ks.ImportStampedPQSeed(seed, "pass", 8979, "Hidden Name", "Hidden Country")
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.StampPQAccount(account, "pass", 8979, "Changed Name", "Changed Country"); err == nil {
		t.Fatal("replaced original stamp")
	}
	backup, err := ks.ExportPQ(account, "pass", "backup-pass")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backup, []byte("Hidden Name")) || bytes.Contains(backup, []byte("Hidden Country")) {
		t.Fatal("backup exposes stamp")
	}
	key, err := DecryptPQKey(backup, "backup-pass")
	if err != nil {
		t.Fatal(err)
	}
	defer zeroPQKey(key)
	private, err := pqcrypto.DeriveShieldedV3ViewKey(key.Seed, 8979, pqcrypto.ShieldedV3Stamp)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(private)
	stamp, err := pqcrypto.OpenShieldedV3Stamp(private, key.Shield3Stamp)
	if err != nil || stamp.Name != "Hidden Name" || stamp.Country != "Hidden Country" {
		t.Fatal("backup lost stamp")
	}
	ks2 := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	restored, err := ks2.ImportStampedPQBackup(backup, "backup-pass", "restored-pass", 8979)
	if err != nil || restored.Address != account.Address {
		t.Fatalf("restore: %v", err)
	}
	wrongChain := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	if _, err := wrongChain.ImportStampedPQBackup(backup, "backup-pass", "restored-pass", 8980); err == nil || len(wrongChain.Accounts()) != 0 {
		t.Fatal("cross-chain stamped backup created an account")
	}
	var encrypted encryptedPQKeyJSONV4
	if err := json.Unmarshal(backup, &encrypted); err != nil {
		t.Fatal(err)
	}
	encrypted.Shield3Stamp.Ciphertext[len(encrypted.Shield3Stamp.Ciphertext)-1] ^= 1
	tampered, _ := json.Marshal(encrypted)
	if _, err := DecryptPQKey(tampered, "backup-pass"); err == nil {
		t.Fatal("accepted modified stamp in keyfile")
	}
}
