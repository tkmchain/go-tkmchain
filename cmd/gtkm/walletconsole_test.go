package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

type walletReceiptConfirmationStub struct {
	receipt      *types.Receipt
	canonical    *types.Header
	head         *types.Header
	receiptErr   error
	canonicalErr error
	headErr      error
}

func (s walletReceiptConfirmationStub) TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error) {
	return s.receipt, s.receiptErr
}

func (s walletReceiptConfirmationStub) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	if number == nil {
		return s.head, s.headErr
	}
	return s.canonical, s.canonicalErr
}

func TestWaitWalletCanonicalConfirmations(t *testing.T) {
	block := &types.Header{Number: big.NewInt(100), Extra: []byte("canonical")}
	receipt := &types.Receipt{TxHash: common.HexToHash("0x01"), BlockNumber: big.NewInt(100), BlockHash: block.Hash()}
	head := &types.Header{Number: big.NewInt(111)} // 12 confirmations, including block 100.
	client := walletReceiptConfirmationStub{receipt: receipt, canonical: block, head: head}
	if err := waitWalletCanonicalConfirmations(context.Background(), client, receipt, 12); err != nil {
		t.Fatalf("canonical 12-confirmation withdrawal rejected: %v", err)
	}
	client.head.Number = big.NewInt(110)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitWalletCanonicalConfirmations(canceled, client, receipt, 12); !errors.Is(err, context.Canceled) {
		t.Fatalf("11-confirmation withdrawal error = %v, want wait/cancellation", err)
	}
	client.head.Number = big.NewInt(111)

	orphan := &types.Header{Number: big.NewInt(100), Extra: []byte("replacement")}
	client.canonical = orphan
	if err := waitWalletCanonicalConfirmations(context.Background(), client, receipt, 12); err == nil || !strings.Contains(err.Error(), "reorganized") {
		t.Fatalf("reorganized withdrawal error = %v, want migration stop", err)
	}

	client.canonical = block
	client.receiptErr = ethereum.NotFound
	if err := waitWalletCanonicalConfirmations(context.Background(), client, receipt, 12); err == nil || !errors.Is(err, ethereum.NotFound) {
		t.Fatalf("missing receipt error = %v, want not-found migration stop", err)
	}
	if err := waitWalletCanonicalConfirmations(context.Background(), client, receipt, 0); err == nil {
		t.Fatal("zero-confirmation migration request was accepted")
	}
}

func TestShield6MigrationFeeReserve(t *testing.T) {
	withdrawal := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(8979), Nonce: 4, GasTipCap: big.NewInt(2), GasFeeCap: big.NewInt(5),
		Gas: 100_000, Value: big.NewInt(7),
	})
	reserve, err := shield6MigrationFeeReserve(withdrawal, 200_000, big.NewInt(3))
	if err != nil {
		t.Fatalf("compute migration fee reserve: %v", err)
	}
	if want := big.NewInt(1_100_007); reserve.Cmp(want) != 0 {
		t.Fatalf("migration reserve = %s, want %s", reserve, want)
	}
	if _, err := shield6MigrationFeeReserve(nil, 200_000, big.NewInt(3)); err == nil {
		t.Fatal("nil withdrawal accepted")
	}
	if _, err := shield6MigrationFeeReserve(withdrawal, 0, big.NewInt(3)); err == nil {
		t.Fatal("zero deposit gas accepted")
	}
	if _, err := shield6MigrationFeeReserve(withdrawal, 200_000, new(big.Int)); err == nil {
		t.Fatal("zero deposit fee cap accepted")
	}
}

func TestShield6MigrationDepositBalanceRequired(t *testing.T) {
	amount := big.NewInt(50)
	feeCap := big.NewInt(3)
	required, err := shield6MigrationDepositBalanceRequired(amount, 200_000, feeCap)
	if err != nil {
		t.Fatalf("compute deposit balance requirement: %v", err)
	}
	if want := big.NewInt(600_050); required.Cmp(want) != 0 {
		t.Fatalf("required balance = %s, want %s", required, want)
	}
	amount.SetInt64(0)
	if _, err := shield6MigrationDepositBalanceRequired(amount, 200_000, feeCap); err == nil {
		t.Fatal("zero deposit amount accepted")
	}
}

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
