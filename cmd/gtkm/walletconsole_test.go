package main

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

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
