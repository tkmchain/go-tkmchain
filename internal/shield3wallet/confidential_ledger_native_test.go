package shield3wallet

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

type ledgerWalletRPC struct{ *forkWalletRPC }

func (r *ledgerWalletRPC) CallContext(ctx context.Context, dst any, method string, args ...any) error {
	if method == "tkmprivacy_shieldedV3Status" || method == "tkmprivacy_shieldedV4Status" {
		activation := hexutil.Uint64(params.MainnetAntarticalTime)
		data, _ := json.Marshal(status{Active: true, NativeVerifier: true, ActivationTime: &activation, ConfidentialLedger: r.timestamp >= params.ProposedConfidentialLedgerTime})
		return json.Unmarshal(data, dst)
	}
	return r.forkWalletRPC.CallContext(ctx, dst, method, args...)
}

func TestConfidentialLedgerNativePayments(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires -tags shield3 and native library")
	}
	ctx := context.Background()
	cfg := *params.MainnetChainConfig
	at := params.ProposedConfidentialLedgerTime
	cfg.ConfidentialLedgerTime = &at
	nodes := []*ledgerWalletRPC{{&forkWalletRPC{rehearsalNode(t), at}}, {&forkWalletRPC{rehearsalNode(t), at}}}
	seed := make([]byte, 32)
	id := testIdentity(t, seed)
	self, err := DecodePaymentCode(id.Code, 8979)
	if err != nil {
		t.Fatal(err)
	}
	tkm := func(n int64) *big.Int { return new(big.Int).Mul(big.NewInt(n), big.NewInt(params.Ether)) }
	for _, n := range nodes {
		n.state.AddBalance(id.Address, uint256.MustFromBig(tkm(10000)), tracing.BalanceChangeUnspecified)
	}
	replicate := func(tx *types.Transaction) {
		for _, n := range nodes {
			rec := applyConfiguredWalletTx(t, n.forkWalletRPC, tx, &cfg)
			_, mode, err := core.ShieldedPaymentMetadata(tx)
			if err == nil && mode == core.ShieldedFeePrepaid && rec.GasUsed != core.ConfidentialPrepaidGas {
				t.Fatal("prepaid gas charge changed")
			}
		}
		if nodes[0].state.IntermediateRoot(true) != nodes[1].state.IntermediateRoot(true) {
			t.Fatal("replicas diverged")
		}
	}
	stamp, err := BuildStamp(ctx, nodes[0], seed, id)
	if err != nil {
		t.Fatal(err)
	}
	replicate(rehearsalSign(t, stamp, seed))
	deposit, err := BuildV4(ctx, nodes[0], seed, id, self, tkm(2000), true)
	if err != nil {
		t.Fatal(err)
	}
	replicate(rehearsalSign(t, deposit, seed))
	if boundary, _, _ := core.ShieldedPaymentMetadata(deposit); boundary != core.PublicShielding {
		t.Fatal("deposit not classified public")
	}
	t.Log("native stamp and 2000 TKM public deposit verified on both replicas")
	// Model an account that holds only notes, with no public balance for gas.
	for _, n := range nodes {
		n.state.SetBalance(id.Address, new(uint256.Int), tracing.BalanceChangeUnspecified)
	}
	for _, version := range []uint64{4, 3} {
		unsigned, err := BuildPrepaidPayment(ctx, nodes[0], seed, id, []Payment{{Recipient: self, Amount: tkm(500)}}, version, tkm(1))
		if err != nil {
			t.Fatal(err)
		}
		signed := rehearsalSign(t, unsigned, seed)
		before := nodes[1].state.Copy()
		oldOutputs := len(nodes[1].outputs)
		minerBefore := nodes[0].state.GetBalance(common.HexToAddress("0xcafe")).ToBig()
		replicate(signed)
		if nodes[0].state.GetBalance(id.Address).Sign() != 0 {
			t.Fatal("prepaid payment exposed a public refund")
		}
		if nodes[0].state.GetBalance(common.HexToAddress("0xcafe")).ToBig().Cmp(minerBefore) != 0 {
			t.Fatal("burned fee credited to miner")
		}
		if boundary, mode, _ := core.ShieldedPaymentMetadata(signed); boundary != core.PrivatePayment || mode != core.ShieldedFeePrepaid {
			t.Fatal("incorrect payment metadata")
		}
		if err := core.ProcessShieldedTransaction(&cfg, big.NewInt(1), at, nodes[0].state, signed, nil); err == nil {
			t.Fatal("replay accepted")
		}
		nodes[1].state = before
		nodes[1].outputs = nodes[1].outputs[:oldOutputs]
		applyConfiguredWalletTx(t, nodes[1].forkWalletRPC, signed, &cfg)
		if nodes[0].state.IntermediateRoot(true) != nodes[1].state.IntermediateRoot(true) {
			t.Fatal("reorg/re-import diverged")
		}
		t.Logf("Shield%d prepaid spend: no public refund, replay rejected, re-import matched", version)
	}
	withdraw, err := BuildPublicWithdrawal(ctx, nodes[0], seed, id, id.Address, tkm(100), 4, tkm(1))
	if err != nil {
		t.Fatal(err)
	}
	replicate(rehearsalSign(t, withdraw, seed))
	if nodes[0].state.GetBalance(id.Address).ToBig().Cmp(tkm(100)) != 0 {
		t.Fatal("explicit public exit amount incorrect")
	}
	if boundary, _, _ := core.ShieldedPaymentMetadata(withdraw); boundary != core.PublicUnshielding {
		t.Fatal("withdrawal not classified as public")
	}
	totals := core.ConfidentialLedgerState(nodes[0].state, shielded3.AssetTKM)
	if !totals.Initialized || totals.Deposited.Cmp(tkm(2000)) != 0 || totals.Withdrawn.Cmp(tkm(100)) != 0 || totals.FeesBurned.Sign() <= 0 {
		t.Fatalf("incorrect counters: %+v", totals)
	}
	expected := new(big.Int).Sub(tkm(1900), totals.FeesBurned)
	if totals.Backing.Cmp(expected) != 0 || nodes[0].state.GetBalance(params.ShieldedPoolAddress).ToBig().Cmp(expected) != 0 {
		t.Fatal("backing conservation failed")
	}
	view := id.ViewKey()
	defer view.Clear()
	scan, err := Scan(ctx, nodes[0], view)
	if err != nil {
		t.Fatal(err)
	}
	sum := new(big.Int)
	for _, note := range scan.Notes {
		n, ok := new(big.Int).SetString(note.ValueWei, 10)
		if !ok {
			t.Fatal("bad note")
		}
		sum.Add(sum, n)
	}
	if sum.Cmp(expected) != 0 {
		t.Fatal("private balances do not match backed issuance less withdrawals and fees")
	}
	t.Log("100 TKM explicit public withdrawal verified; private balances and backing conserve value")
}
