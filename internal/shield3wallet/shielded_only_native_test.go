package shield3wallet

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

type forkWalletRPC struct {
	*walletRPC
	timestamp uint64
}

func (r *forkWalletRPC) CallContext(ctx context.Context, dst any, method string, args ...any) error {
	if method == "tkmprivacy_shieldedV3Status" || method == "tkmprivacy_shieldedV4Status" {
		activation := hexutil.Uint64(params.MainnetShieldedOnlyTime)
		antartical := hexutil.Uint64(params.MainnetAntarticalTime)
		data, _ := json.Marshal(status{Active: true, NativeVerifier: true, ActivationTime: &antartical, ShieldedOnly: r.timestamp >= uint64(activation), ShieldedOnlyTime: &activation})
		return json.Unmarshal(data, dst)
	}
	if err := r.walletRPC.CallContext(ctx, dst, method, args...); err != nil {
		return err
	}
	if method == "eth_getBlockByNumber" {
		if h, ok := dst.(*header); ok {
			h.Timestamp = hexutil.Uint64(r.timestamp)
		}
	}
	return nil
}

func applyForkWalletTx(t *testing.T, rpc *forkWalletRPC, tx *types.Transaction) {
	applyConfiguredWalletTx(t, rpc, tx, nativeShieldedOnlyConfig())
}

func applyConfiguredWalletTx(t *testing.T, rpc *forkWalletRPC, tx *types.Transaction, cfg *params.ChainConfig) *types.Receipt {
	t.Helper()
	h := &types.Header{Number: big.NewInt(1), Time: rpc.timestamp, GasLimit: 30_000_000, BaseFee: new(big.Int), Difficulty: big.NewInt(1)}
	if err := txpool.ValidateTransaction(tx, h, types.NewQuantumSigner(big.NewInt(8979)), &txpool.ValidationOptions{Config: cfg, Accept: 0xff, MaxSize: core.ShieldedV3MaxTxSize, MinTip: new(big.Int)}); err != nil {
		t.Fatalf("admission: %v", err)
	}
	if err := core.ProcessShieldedTransaction(cfg, h.Number, h.Time, rpc.state, tx, nil); err != nil {
		t.Fatalf("native consensus: %v", err)
	}
	block := vm.BlockContext{CanTransfer: core.CanTransfer, Transfer: core.Transfer, GetHash: func(uint64) common.Hash { return common.Hash{} }, Coinbase: common.HexToAddress("0xcafe"), BlockNumber: h.Number, Time: h.Time, GasLimit: h.GasLimit, BaseFee: h.BaseFee, Difficulty: h.Difficulty}
	evm := vm.NewEVM(block, rpc.state, cfg, vm.Config{})
	defer evm.Release()
	rpc.state.SetTxContext(tx.Hash(), 0)
	receipt, err := core.ApplyTransaction(evm, core.NewGasPool(h.GasLimit), rpc.state, h, tx)
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("execution: %v", err)
	}
	if len(receipt.Logs) != 0 {
		t.Fatal("private payment emitted public application logs")
	}
	rpc.transactions[tx.Hash()] = tx
	if core.HasShieldedV3Prefix(tx.Data()) {
		e, _, _ := core.DecodeShieldedV3Transaction(tx.Data())
		for _, out := range e.Outputs {
			rpc.outputs = append(rpc.outputs, scanOutput{Commitment: out.Commitment, Incoming: out.Incoming, Outgoing: out.Outgoing, TransactionHash: tx.Hash()})
		}
	} else if core.HasShieldedV4Prefix(tx.Data()) {
		e, _, _ := core.DecodeShieldedV4Transaction(tx.Data())
		for _, out := range e.Outputs {
			rpc.outputs = append(rpc.outputs, scanOutput{Commitment: out.Commitment, Incoming: out.Incoming, Outgoing: out.Outgoing, TransactionHash: tx.Hash()})
		}
	}
	return receipt
}

func TestShieldedOnlyNativePayments(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires -tags shield3 and native library")
	}
	ctx := context.Background()
	nodes := []*forkWalletRPC{{rehearsalNode(t), params.MainnetShieldedOnlyTime - 10}, {rehearsalNode(t), params.MainnetShieldedOnlyTime - 10}}
	seedA, seedB := make([]byte, 32), make([]byte, 32)
	seedB[0] = 42
	a, b := testIdentity(t, seedA), testIdentity(t, seedB)
	pa, err := DecodePaymentCode(a.Code, 8979)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := DecodePaymentCode(b.Code, 8979)
	if err != nil {
		t.Fatal(err)
	}
	tkm := func(amount int64) *big.Int { return new(big.Int).Mul(big.NewInt(amount), big.NewInt(params.Ether)) }
	for _, node := range nodes {
		for _, id := range []*Identity{a, b} {
			node.state.AddBalance(id.Address, uint256.MustFromBig(tkm(10000)), tracing.BalanceChangeUnspecified)
		}
	}
	replicate := func(tx *types.Transaction) {
		for _, node := range nodes {
			applyForkWalletTx(t, node, tx)
		}
		if nodes[0].state.IntermediateRoot(true) != nodes[1].state.IntermediateRoot(true) {
			t.Fatal("independent state replicas diverged")
		}
	}
	for _, entry := range []struct {
		seed []byte
		id   *Identity
	}{{seedA, a}, {seedB, b}} {
		unsigned, err := BuildStamp(ctx, nodes[0], entry.seed, entry.id)
		if err != nil {
			t.Fatal(err)
		}
		replicate(rehearsalSign(t, unsigned, entry.seed))
	}
	t.Log("registered both owners with real native stamp proofs")
	deposit, err := BuildV4(ctx, nodes[0], seedA, a, pa, tkm(2000), true)
	if err != nil {
		t.Fatal(err)
	}
	replicate(rehearsalSign(t, deposit, seedA))
	backing := nodes[0].state.GetBalance(params.ShieldedPoolAddress).ToBig()
	if backing.Cmp(tkm(2000)) != 0 {
		t.Fatal("incorrect pre-fork backing")
	}
	for _, node := range nodes {
		node.timestamp = params.MainnetShieldedOnlyTime
	}
	if _, err := BuildV4(ctx, nodes[0], seedA, a, pa, tkm(1), true); !errors.Is(err, core.ErrPublicPaymentDisabled) {
		t.Fatalf("deposit still buildable: %v", err)
	}
	if err := core.ProcessShieldedTransaction(nativeShieldedOnlyConfig(), big.NewInt(1), nodes[0].timestamp, nodes[0].state, deposit, nil); !errors.Is(err, core.ErrPublicPaymentDisabled) {
		t.Fatalf("old deposit accepted after cutoff: %v", err)
	}
	t.Log("activated fork; public funding rejected")
	spend, err := BuildV4(ctx, nodes[0], seedA, a, pb, tkm(1000), false)
	if err != nil {
		t.Fatal(err)
	}
	e4, _, _ := core.DecodeShieldedV4Transaction(spend.Data())
	s4, err := core.ShieldedV4Statement(spend, e4)
	if err != nil {
		t.Fatal(err)
	}
	if spend.Value().Sign() != 0 || s4.PublicValue.Big().Sign() != 0 || e4.GasSponsorValue.Sign() != 0 {
		t.Fatal("Shield4 exposed public value")
	}
	signed := rehearsalSign(t, spend, seedA)
	before := nodes[1].state.Copy()
	replicate(signed)
	if err := core.ProcessShieldedTransaction(nativeShieldedOnlyConfig(), big.NewInt(1), nodes[0].timestamp, nodes[0].state, signed, nil); err == nil {
		t.Fatal("replayed spend accepted")
	}
	// Re-execute from the earlier state, as block rollback/re-import does.
	nodes[1].state = before
	nodes[1].outputs = nodes[1].outputs[:len(nodes[1].outputs)-len(e4.Outputs)]
	delete(nodes[1].transactions, signed.Hash())
	applyForkWalletTx(t, nodes[1], signed)
	if nodes[0].state.IntermediateRoot(true) != nodes[1].state.IntermediateRoot(true) {
		t.Fatal("re-execution diverged")
	}
	view := b.ViewKey()
	defer view.Clear()
	scan, err := Scan(ctx, nodes[1], view)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Notes) != 1 || scan.Notes[0].ValueWei != tkm(1000).String() {
		t.Fatal("recipient did not decrypt 1000 TKM")
	}
	t.Log("1000 TKM Shield4 spend verified on both replicas; replay rejected and rollback/re-import matched")
	spend3, err := Build(ctx, nodes[1], seedB, b, pa, tkm(500), false)
	if err != nil {
		t.Fatal(err)
	}
	e3, _, _ := core.DecodeShieldedV3Transaction(spend3.Data())
	s3, err := core.ShieldedV3Statement(spend3, e3)
	if err != nil || s3.PublicValue.Big().Sign() != 0 || spend3.Value().Sign() != 0 {
		t.Fatalf("Shield3 exposed value: %v", err)
	}
	replicate(rehearsalSign(t, spend3, seedB))
	t.Log("500 TKM Shield3 return payment verified")
	offer, err := BuildFeeSponsoredRelayOffer(ctx, nodes[0], seedA, a)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := BuildV4Relayed(ctx, nodes[1], seedB, b, pa, tkm(100), &offer)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := relay.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildRelaySubmission(ctx, nodes[0], seedA, a, raw); err == nil {
		t.Fatal("relay paid fees without opt-in")
	}
	if _, err := BuildFeeSponsoredRelaySubmission(ctx, nodes[0], seedA, a, raw, big.NewInt(1)); err == nil {
		t.Fatal("relay exceeded operator budget")
	}
	reviewed, err := BuildFeeSponsoredRelaySubmission(ctx, nodes[0], seedA, a, raw, new(big.Int).Mul(new(big.Int).SetUint64(WalletGas), relay.GasFeeCap()))
	if err != nil {
		t.Fatal(err)
	}
	replicate(rehearsalSign(t, reviewed, seedA))
	// The daemon's automatic preparation uses this same self-payment builder.
	// Combine A's 1000 TKM change, 500 TKM return and 100 TKM receipt.
	consolidated, err := BuildV4(ctx, nodes[0], seedA, a, pa, tkm(1600), false)
	if err != nil {
		t.Fatal(err)
	}
	merged, _, err := core.DecodeShieldedV4Transaction(consolidated.Data())
	if err != nil || merged.InputCount != 3 || consolidated.Value().Sign() != 0 || merged.GasSponsorValue.Sign() != 0 {
		t.Fatalf("private consolidation shape: %v", err)
	}
	replicate(rehearsalSign(t, consolidated, seedA))
	viewA := a.ViewKey()
	defer viewA.Clear()
	mergedScan, err := Scan(ctx, nodes[1], viewA)
	if err != nil || len(mergedScan.Notes) != 1 || mergedScan.Notes[0].ValueWei != tkm(1600).String() {
		t.Fatalf("self-owned consolidated note missing: %v", err)
	}
	for _, node := range nodes {
		if node.state.GetBalance(params.ShieldedPoolAddress).ToBig().Cmp(backing) != 0 {
			t.Fatal("private payments changed public pool backing")
		}
	}
	t.Log("100 TKM relayed payment verified with explicit fee sponsorship; public pool backing unchanged throughout")
	t.Log("three-input 1600 TKM private self-consolidation verified and recovered by the wallet")
}

func nativeShieldedOnlyConfig() *params.ChainConfig {
	cfg := *params.MainnetChainConfig
	at := params.MainnetShieldedOnlyTime
	cfg.ShieldedOnlyTime = &at
	return &cfg
}
