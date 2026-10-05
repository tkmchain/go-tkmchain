package main

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestWalletKingStatusDecodesRPCBigInts(t *testing.T) {
	const response = `{"address":"0x0000000000000000000000000000000000000001","registered":true,"lockedAmount":500000000000000000000000,"registrationFee":100000000000000000000,"totalReceived":2500000000000000000}`
	var status walletKingStatusView
	if err := json.Unmarshal([]byte(response), &status); err != nil {
		t.Fatalf("decode rk status response: %v", err)
	}
	wantStake, _ := new(big.Int).SetString("500000000000000000000000", 10)
	if status.LockedAmount.Cmp(wantStake) != 0 {
		t.Fatalf("locked amount = %v", status.LockedAmount)
	}
	wantFee, _ := new(big.Int).SetString("100000000000000000000", 10)
	if status.RegistrationFee.Cmp(wantFee) != 0 {
		t.Fatalf("registration fee = %v", status.RegistrationFee)
	}
	if status.TotalReceived.Cmp(big.NewInt(2500000000000000000)) != 0 {
		t.Fatalf("total received = %v", status.TotalReceived)
	}
}

func TestParseWalletAmount(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1", "1000000000000000000"},
		{"0.000000000000000001", "1"},
		{"12.50", "12500000000000000000"},
	}
	for _, test := range tests {
		got, err := parseWalletAmount(test.input, 18)
		if err != nil || got.String() != test.want {
			t.Fatalf("parseWalletAmount(%q) = %v, %v; want %s", test.input, got, err, test.want)
		}
	}
	if _, err := parseWalletAmount("1.0000000000000000001", 18); err == nil {
		t.Fatal("expected excessive precision to be rejected")
	}
	if _, err := parseWalletAmount("-1", 18); err == nil {
		t.Fatal("expected negative amount to be rejected")
	}
}

func TestWalletTransferTransactionUsesPQEnvelope(t *testing.T) {
	chainID := big.NewInt(8979)
	to := common.HexToAddress("0x0000000000000000000000000000000000000001")
	tx := walletTransferTransaction(chainID, 7, to, big.NewInt(42), 21000, big.NewInt(3), pqcrypto.AlgorithmMLDSA87)
	if tx.Type() != types.PQTkmTxType || tx.Nonce() != 7 || tx.To() == nil || *tx.To() != to || tx.Value().Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("unexpected PQ transfer transaction: type=%d nonce=%d to=%v value=%v", tx.Type(), tx.Nonce(), tx.To(), tx.Value())
	}
}

func TestWalletMigrationTransactionCarriesMarker(t *testing.T) {
	chainID := big.NewInt(8979)
	to := common.HexToAddress("0x0000000000000000000000000000000000000002")
	data := []byte("TKMPQMIG1-marker")
	tx := walletMigrationTransaction(chainID, 8, to, big.NewInt(99), 50000, big.NewInt(4), data)
	if tx.Type() != types.DynamicFeeTxType || tx.Nonce() != 8 || tx.To() == nil || *tx.To() != to || tx.Value().Cmp(big.NewInt(99)) != 0 {
		t.Fatalf("unexpected migration transaction: type=%d nonce=%d to=%v value=%v", tx.Type(), tx.Nonce(), tx.To(), tx.Value())
	}
	if string(tx.Data()) != string(data) {
		t.Fatalf("migration data = %q, want %q", tx.Data(), data)
	}
}

func TestWalletPQSeedMnemonic(t *testing.T) {
	seed := make([]byte, pqcrypto.MLDSA87SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	phrase, err := walletPQSeedMnemonic(seed)
	if err != nil {
		t.Fatalf("walletPQSeedMnemonic: %v", err)
	}
	if words := strings.Fields(phrase); len(words) != 24 {
		t.Fatalf("phrase has %d words, want 24", len(words))
	}
	if _, err := walletPQSeedMnemonic(seed[:pqcrypto.MLDSA87SeedSize-1]); err == nil {
		t.Fatal("expected invalid seed length to be rejected")
	}
}
