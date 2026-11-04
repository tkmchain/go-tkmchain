package shield3wallet

import (
	"context"
	cryptorand "crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/holiman/uint256"
)

// TestShield6MigrationNativeRehearsal runs the real legacy V4 withdrawal and
// Shield6 deposit independently of the broader V3 proof stress test. Both
// state replicas execute every transaction and must converge after each step.
func TestShield6MigrationNativeRehearsal(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires -tags shield3 and native library")
	}
	ctx := context.Background()
	const chainID = uint64(8979)
	at := params.ProposedConfidentialLedgerTime
	cfg := *params.MainnetChainConfig
	cfg.ConfidentialLedgerTime = &at
	cfg.Shield6Time = &at
	nodes := []*ledgerWalletRPC{
		{&forkWalletRPC{rehearsalNode(t), at}},
		{&forkWalletRPC{rehearsalNode(t), at}},
	}
	seed := make([]byte, 32)
	id := testIdentity(t, seed)
	legacyAddress, err := DecodePaymentCode(id.Code, chainID)
	if err != nil {
		t.Fatal("decode legacy wallet address:", err)
	}
	tkm := func(amount int64) *big.Int {
		return new(big.Int).Mul(big.NewInt(amount), big.NewInt(params.Ether))
	}
	for _, node := range nodes {
		node.state.AddBalance(id.Address, uint256.MustFromBig(tkm(1000)), tracing.BalanceChangeUnspecified)
	}
	t.Log("initialized two isolated migration replicas")
	applyBoth := func(tx *types.Transaction) {
		t.Helper()
		for _, node := range nodes {
			applyConfiguredWalletTx(t, node.forkWalletRPC, tx, &cfg)
		}
		if nodes[0].state.IntermediateRoot(true) != nodes[1].state.IntermediateRoot(true) {
			t.Fatalf("migration replicas diverged after %s", tx.Hash())
		}
	}
	stamp, err := BuildStamp(ctx, nodes[0], seed, id)
	if err != nil {
		t.Fatal("build account stamp:", err)
	}
	applyBoth(rehearsalSign(t, stamp, seed))
	t.Log("registered the legacy account stamp on both replicas")

	legacyDepositStarted := time.Now()
	legacyDeposit, err := BuildV4(ctx, nodes[0], seed, id, legacyAddress, tkm(200), true)
	if err != nil {
		t.Fatal("build legacy Shield4 deposit:", err)
	}
	t.Logf("built legacy Shield4 deposit in %s", time.Since(legacyDepositStarted).Round(time.Millisecond))
	applyBoth(rehearsalSign(t, legacyDeposit, seed))
	t.Log("confirmed legacy Shield4 note on both replicas")

	withdrawAmount := tkm(50)
	withdrawalStarted := time.Now()
	withdrawal, err := BuildPublicWithdrawal(ctx, nodes[0], seed, id, id.Address, withdrawAmount, 4, tkm(1))
	if err != nil {
		t.Fatal("build legacy self-withdrawal:", err)
	}
	t.Logf("built legacy self-withdrawal in %s", time.Since(withdrawalStarted).Round(time.Millisecond))
	withdrawal = rehearsalSign(t, withdrawal, seed)
	withdrawalHash := withdrawal.Hash()
	applyBoth(withdrawal)
	if nodes[0].state.GetBalance(id.Address).ToBig().Cmp(withdrawAmount) < 0 {
		t.Fatal("legacy self-withdrawal did not release the public migration amount")
	}
	if boundary, _, err := core.ShieldedPaymentMetadata(withdrawal); err != nil || boundary != core.PublicUnshielding {
		t.Fatalf("legacy migration exit metadata = %v, %v", boundary, err)
	}

	keys, _, err := shielded6.CreateWallet(cryptorand.Reader, chainID)
	if err != nil {
		t.Fatal("create Shield6 wallet:", err)
	}
	defer keys.Clear()
	t.Log("created the destination Shield6 wallet")
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal("derive account key:", err)
	}
	var nonce hexutil.Uint64
	if err := nodes[0].CallContext(ctx, &nonce, "eth_getTransactionCount", id.Address, "pending"); err != nil {
		t.Fatal("read migration deposit nonce:", err)
	}
	skeleton := types.NewTx(&types.PQTkmTx{
		ChainID: new(big.Int).SetUint64(chainID), Nonce: uint64(nonce),
		GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 10_000_000,
		To: &params.ShieldedPoolAddress, Value: new(big.Int).Set(withdrawAmount),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(accountKey),
	})
	started := time.Now()
	deposit, err := BuildAndSignShieldedV6Deposit(ctx, nodes[0], seed, id, keys, skeleton, withdrawAmount, cryptorand.Reader)
	if err != nil {
		t.Fatal("build Shield6 migration deposit:", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Minute {
		t.Fatalf("Shield6 migration deposit build took %s; exceeds the two-minute target", elapsed.Round(time.Second))
	} else {
		t.Logf("Shield6 migration deposit build took %s", elapsed.Round(time.Millisecond))
	}
	envelope, recognized, err := core.DecodeShieldedV6Transaction(deposit.Data())
	if err != nil || !recognized || envelope == nil || !envelope.Deposit || deposit.Value().Cmp(withdrawAmount) != 0 {
		t.Fatalf("invalid Shield6 migration deposit envelope: recognized=%t err=%v", recognized, err)
	}
	t.Logf("built Shield6 migration deposit envelope %s", deposit.Hash())
	if boundary, _, err := core.ShieldedPaymentMetadata(deposit); err != nil || boundary != core.PublicShielding {
		t.Fatalf("Shield6 migration entry metadata = %v, %v", boundary, err)
	}
	applyBoth(deposit)
	t.Log("confirmed Shield6 migration deposit and accumulator append on both replicas")

	owned, found, err := shielded6.ScanOutput(keys, 0, 0, core.ShieldedV6WalletRecord(envelope.Outputs[0]))
	if err != nil || !found || owned.Amount.Cmp(withdrawAmount) != 0 {
		if owned != nil {
			owned.Clear()
		}
		t.Fatalf("scan migrated Shield6 note: found=%t err=%v", found, err)
	}
	owned.Clear()
	t.Log("scanned migrated Shield6 note with the destination wallet")
	if core.ShieldedV6OutputCount(nodes[0].state) != 1 || core.ShieldedV6OutputCount(nodes[1].state) != 1 || core.ShieldedV6OutputRoot(nodes[0].state) != core.ShieldedV6OutputRoot(nodes[1].state) {
		t.Fatal("Shield6 migration output accumulator diverged across replicas")
	}
	totals := core.ConfidentialLedgerState(nodes[0].state, shielded3.AssetTKM)
	if totals.Deposited.Cmp(tkm(250)) != 0 || totals.Withdrawn.Cmp(withdrawAmount) != 0 || totals.Backing.Cmp(nodes[0].state.GetBalance(params.ShieldedPoolAddress).ToBig()) != 0 {
		t.Fatalf("migration backing mismatch: deposited=%s withdrawn=%s backing=%s pool=%s", totals.Deposited, totals.Withdrawn, totals.Backing, nodes[0].state.GetBalance(params.ShieldedPoolAddress).ToBig())
	}
	t.Logf("legacy withdrawal %s and Shield6 deposit %s replayed on two replicas; migrated note scans at the expected amount", withdrawalHash, deposit.Hash())
}
