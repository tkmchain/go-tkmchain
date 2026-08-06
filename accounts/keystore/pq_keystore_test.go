// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package keystore

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestPQAccountCreateExportImportAndSign(t *testing.T) {
	dir := t.TempDir()
	ks := NewKeyStore(dir, LightScryptN, LightScryptP)
	acc, err := ks.NewPQAccount("old-pass")
	if err != nil {
		t.Fatal(err)
	}
	if !ks.HasAddress(acc.Address) {
		t.Fatal("PQ account not indexed")
	}
	algorithm, err := ks.AccountAlgorithm(acc)
	if err != nil {
		t.Fatal(err)
	}
	if algorithm != pqcrypto.AlgorithmMLDSA87 {
		t.Fatalf("algorithm = %s, want %s", algorithm, pqcrypto.AlgorithmMLDSA87)
	}
	if algorithms := ks.AccountAlgorithms(); algorithms[acc.Address] != pqcrypto.AlgorithmMLDSA87 {
		t.Fatalf("algorithm map = %v", algorithms)
	}
	exported, err := ks.ExportPQ(acc, "old-pass", "new-pass")
	if err != nil {
		t.Fatal(err)
	}
	if !isPQKeyJSON(exported) {
		t.Fatal("exported key is not PQ key JSON")
	}
	ks2 := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	imported, err := ks2.ImportPQ(exported, "new-pass", "import-pass")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Address != acc.Address {
		t.Fatalf("import address mismatch: got %s want %s", imported.Address, acc.Address)
	}
	tx := testPQTkmTx(t, big.NewInt(8979))
	signed, err := ks2.SignTxWithPassphrase(imported, "import-pass", tx, big.NewInt(8979))
	if err != nil {
		t.Fatal(err)
	}
	from, err := types.Sender(types.NewQuantumSigner(big.NewInt(8979)), signed)
	if err != nil {
		t.Fatal(err)
	}
	if from != imported.Address {
		t.Fatalf("sender mismatch: got %s want %s", from, imported.Address)
	}
}

func TestPQAccountUnlockSignAndDelete(t *testing.T) {
	ks := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	seed := make([]byte, pqcrypto.MLDSA87SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	acc, err := ks.ImportPQSeed(seed, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.TimedUnlock(acc, "pass", 0); err != nil {
		t.Fatal(err)
	}
	tx := testPQTkmTx(t, big.NewInt(8979))
	signed, err := ks.SignTx(acc, tx, big.NewInt(8979))
	if err != nil {
		t.Fatal(err)
	}
	from, err := types.Sender(types.NewQuantumSigner(big.NewInt(8979)), signed)
	if err != nil {
		t.Fatal(err)
	}
	if from != acc.Address {
		t.Fatalf("sender mismatch: got %s want %s", from, acc.Address)
	}
	if _, err := ks.SignHash(acc, common.Hash{1}.Bytes()); err != ErrNoMatch {
		t.Fatalf("PQ legacy hash sign error = %v, want %v", err, ErrNoMatch)
	}
	if err := ks.Delete(accounts.Account{Address: acc.Address}, "pass"); err != nil {
		t.Fatal(err)
	}
	if ks.HasAddress(acc.Address) {
		t.Fatal("PQ account still indexed after delete")
	}
}

func TestPreparePQMigration(t *testing.T) {
	ks := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	legacy, err := ks.NewAccount("legacy-pass")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := ks.PreparePQMigration(legacy, "legacy-pass", "pq-pass")
	if err != nil {
		t.Fatal(err)
	}
	if migration.LegacyAccount.Address != legacy.Address {
		t.Fatalf("legacy mismatch: got %s want %s", migration.LegacyAccount.Address, legacy.Address)
	}
	if migration.PQAccount.Address == legacy.Address {
		t.Fatal("PQ migration account reused legacy address")
	}
	if migration.PQAlgorithm != pqcrypto.AlgorithmMLDSA87 {
		t.Fatalf("algorithm = %s, want %s", migration.PQAlgorithm, pqcrypto.AlgorithmMLDSA87)
	}
	parsed, err := types.ParsePQMigrationData(migration.MigrationData)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Address != migration.PQAccount.Address {
		t.Fatalf("migration address = %s, want %s", parsed.Address, migration.PQAccount.Address)
	}
	if algorithm, err := ks.AccountAlgorithm(migration.PQAccount); err != nil {
		t.Fatal(err)
	} else if algorithm != pqcrypto.AlgorithmMLDSA87 {
		t.Fatalf("stored algorithm = %s, want %s", algorithm, pqcrypto.AlgorithmMLDSA87)
	}
	if _, err := ks.ExportPQ(migration.PQAccount, "pq-pass", "new-pq-pass"); err != nil {
		t.Fatal(err)
	}
}

func TestPreparePQMigrationRejectsPQSource(t *testing.T) {
	ks := NewKeyStore(t.TempDir(), LightScryptN, LightScryptP)
	pq, err := ks.NewPQAccount("pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ks.PreparePQMigration(pq, "pass", "pass"); err == nil {
		t.Fatal("expected PQ source migration rejection")
	}
}

func testPQTkmTx(t *testing.T, chainID *big.Int) *types.Transaction {
	t.Helper()
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	return types.NewTx(&types.PQTkmTx{
		ChainID:   chainID,
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(10),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(1),
	})
}
