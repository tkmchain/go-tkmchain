package shield3wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

type walletRPC struct {
	state   *state.StateDB
	outputs []scanOutput
	pending map[shielded3.Digest]common.Hash
	reorg   bool
}

func (r *walletRPC) CallContext(_ context.Context, dest any, method string, args ...any) error {
	var result any
	switch method {
	case "eth_chainId":
		result = hexutil.EncodeUint64(8979)
	case "eth_getBlockByNumber":
		hash := common.HexToHash("0x1234")
		if r.reorg && args[0] != "latest" {
			hash = common.HexToHash("0xabcd")
		}
		result = header{Number: 0, Timestamp: hexutil.Uint64(params.MainnetAntarticalTime), Hash: hash}
	case "tkmprivacy_shieldedV3Status":
		fork := hexutil.Uint64(params.MainnetAntarticalTime)
		result = status{Active: true, NativeVerifier: true, ActivationTime: &fork}
	case "eth_getTransactionCount":
		result = hexutil.EncodeUint64(r.state.GetNonce(args[0].(common.Address)))
	case "eth_gasPrice":
		result = "0x1"
	case "eth_getBalance":
		result = hexutil.EncodeBig(r.state.GetBalance(args[0].(common.Address)).ToBig())
	case "tkmprivacy_shieldedV3Outputs":
		result = r.outputs
	case "tkmprivacy_shieldedV3Path":
		path, err := core.ShieldedV3CommitmentPath(r.state, args[0].(shielded3.Digest))
		if err != nil {
			return err
		}
		result = path
	case "tkmprivacy_shieldedV3NullifierStatus":
		n := args[0].(shielded3.Digest)
		hash := core.ShieldedV3NullifierTransaction(r.state, n)
		pending := r.pending[n] != (common.Hash{})
		if pending {
			hash = r.pending[n]
		}
		result = map[string]any{"transactionHash": hash, "pending": pending}
	default:
		return fmt.Errorf("unexpected RPC method %s", method)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dest)
}
func testIdentity(t *testing.T, seed []byte) *Identity {
	t.Helper()
	stamp, err := pqcrypto.CreateShieldedV3Stamp(seed, 8979, "Private Name", "Private Country")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := NewIdentity(seed, 8979, stamp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(identity.Clear)
	return identity
}
func TestShield3WalletConsensus(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires -tags shield3 and the native static library")
	}
	ctx := context.Background()
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	rpc := &walletRPC{state: st, pending: make(map[shielded3.Digest]common.Hash)}
	seedA, seedB := make([]byte, 32), make([]byte, 32)
	seedB[0] = 1
	a, b := testIdentity(t, seedA), testIdentity(t, seedB)
	pa, err := DecodePaymentCode(a.Code, 8979)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := DecodePaymentCode(b.Code, 8979)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePaymentCode(b.Code, 8980); err == nil {
		t.Fatal("accepted cross-chain address")
	}
	st.AddBalance(a.Address, uint256.MustFromBig(new(big.Int).Exp(big.NewInt(10), big.NewInt(21), nil)), tracing.BalanceChangeUnspecified)
	sign := func(tx *types.Transaction, seed []byte) *types.Transaction {
		t.Helper()
		key, err := pqcrypto.NewMLDSA87FromSeed(seed)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := types.SignPQTkmTx(tx, types.NewQuantumSigner(big.NewInt(8979)), key)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	process := func(tx *types.Transaction) {
		t.Helper()
		if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, tx, make(map[common.Hash]struct{})); err != nil {
			t.Fatal(err)
		}
		h := &types.Header{Number: big.NewInt(1), Time: params.MainnetAntarticalTime, GasLimit: 30_000_000, BaseFee: big.NewInt(0), Difficulty: big.NewInt(1)}
		block := vm.BlockContext{CanTransfer: core.CanTransfer, Transfer: core.Transfer, GetHash: func(uint64) common.Hash { return common.Hash{} }, Coinbase: common.HexToAddress("0xcafe"), BlockNumber: h.Number, Time: h.Time, GasLimit: h.GasLimit, BaseFee: h.BaseFee, Difficulty: h.Difficulty}
		evm := vm.NewEVM(block, st, params.MainnetChainConfig, vm.Config{})
		defer evm.Release()
		st.SetTxContext(tx.Hash(), 0)
		receipt, err := core.ApplyTransaction(evm, core.NewGasPool(h.GasLimit), st, h, tx)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			t.Fatal("wallet transaction reverted")
		}
		e, _, err := core.DecodeShieldedV3Transaction(tx.Data())
		if err != nil {
			t.Fatal(err)
		}
		for _, out := range e.Outputs {
			rpc.outputs = append(rpc.outputs, scanOutput{Commitment: out.Commitment, Incoming: out.Incoming, Outgoing: out.Outgoing, TransactionHash: tx.Hash()})
		}
	}
	amount := new(big.Int).Mul(big.NewInt(11), big.NewInt(1_000_000_000_000_000_000))
	unsigned, err := Build(ctx, rpc, seedA, a, pa, amount, true)
	if err != nil {
		t.Fatal(err)
	}
	deposit := sign(unsigned, seedA)
	if err := core.ValidateShieldedTransactionBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime-1, deposit); err == nil {
		t.Fatal("accepted Shield3 before fork")
	}
	st.SetCode(params.ShieldedPoolAddress, []byte{0x60, 0x00, 0x60, 0x00, 0xfd}, tracing.CodeChangeUnspecified)
	if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, deposit, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("accepted funding into a pool that can revert")
	}
	if core.ShieldedV3NextIndex(st) != 0 {
		t.Fatal("reverting pool created notes")
	}
	st.SetCode(params.ShieldedPoolAddress, nil, tracing.CodeChangeUnspecified)
	process(deposit)
	if st.GetBalance(params.ShieldedPoolAddress).ToBig().Cmp(amount) != 0 {
		t.Fatal("deposit reserve not transferred by execution")
	}
	viewA := a.ViewKey()
	defer clear(viewA.Incoming)
	defer clear(viewA.Outgoing)
	scan, err := Scan(ctx, rpc, viewA)
	if err != nil || scan.BalanceWei != amount.String() || len(scan.Notes) != 1 {
		t.Fatalf("view-key-only balance: %+v %v", scan, err)
	}
	input := scan.Notes[0]
	rpc.pending[input.Nullifier] = common.HexToHash("0xff")
	pending, err := Scan(ctx, rpc, viewA)
	if err != nil || pending.BalanceWei != "0" {
		t.Fatalf("pending note remained spendable: %v", err)
	}
	delete(rpc.pending, input.Nullifier)
	rpc.reorg = true
	if _, err := Scan(ctx, rpc, viewA); err == nil {
		t.Fatal("accepted reorged scan")
	}
	rpc.reorg = false
	payment := new(big.Int).Mul(big.NewInt(5), big.NewInt(1_000_000_000_000_000_000))
	unsigned, err = Build(ctx, rpc, seedA, a, pb, payment, false)
	if err != nil {
		t.Fatal(err)
	}
	spend := sign(unsigned, seedA)
	if err := core.ValidateShieldedV3Proof(spend); err != nil {
		t.Fatal(err)
	}
	envelope, _, _ := core.DecodeShieldedV3Transaction(unsigned.Data())
	envelope.Outputs[0].Outgoing[len(envelope.Outputs[0].Outgoing)-1] ^= 1
	changedData, err := core.EncodeShieldedV3Transaction(envelope)
	if err != nil {
		t.Fatal(err)
	}
	algorithm, pub, _, _ := unsigned.PQTkmFields()
	changed := sign(types.NewTx(&types.PQTkmTx{ChainID: unsigned.ChainId(), Nonce: unsigned.Nonce(), Gas: unsigned.Gas(), GasFeeCap: unsigned.GasFeeCap(), GasTipCap: unsigned.GasTipCap(), To: unsigned.To(), Value: unsigned.Value(), Data: changedData, Algorithm: algorithm, PublicKey: pub}), seedA)
	rootBefore, _ := core.ShieldedV3Root(st)
	if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, changed, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("accepted ciphertext changed after proof generation")
	}
	rootAfter, _ := core.ShieldedV3Root(st)
	if rootAfter != rootBefore || core.ShieldedV3NextIndex(st) != 4 {
		t.Fatal("failed proof mutated state")
	}
	process(spend)
	if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, spend, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("accepted duplicate nullifier")
	}
	scan, err = Scan(ctx, rpc, viewA)
	if err != nil || scan.BalanceWei != new(big.Int).Sub(amount, payment).String() {
		t.Fatalf("sender change: %+v %v", scan, err)
	}
	outgoing := false
	for _, entry := range scan.History {
		if entry.Direction == "outgoing" && entry.TransactionHash == spend.Hash() && entry.ValueWei == payment.String() {
			outgoing = true
		}
	}
	if !outgoing {
		t.Fatal("outgoing view key did not recover history")
	}
	viewB := b.ViewKey()
	defer clear(viewB.Incoming)
	defer clear(viewB.Outgoing)
	received, err := Scan(ctx, rpc, viewB)
	if err != nil || received.BalanceWei != payment.String() {
		t.Fatalf("recipient balance: %+v %v", received, err)
	}
	// Receiving private funds must be sufficient to send again even when the
	// recipient has no public gas balance. The proof funds bounded sponsorship.
	if !st.GetBalance(b.Address).IsZero() {
		t.Fatal("recipient unexpectedly has public funds")
	}
	backAmount := big.NewInt(1_000_000_000_000_000_000)
	backUnsigned, err := Build(ctx, rpc, seedB, b, pa, backAmount, false)
	if err != nil {
		t.Fatal(err)
	}
	back := sign(backUnsigned, seedB)
	if core.ShieldedTransactionPreBalanceCost(back).Sign() != 0 {
		t.Fatal("private sponsorship still requires public gas")
	}
	backEnvelope, _, _ := core.DecodeShieldedV3Transaction(back.Data())
	if backEnvelope.GasSponsorValue.Sign() <= 0 {
		t.Fatal("wallet did not fund gas from the private note")
	}
	process(back)
	bAfter, err := Scan(ctx, rpc, viewB)
	expectedB := new(big.Int).Sub(new(big.Int).Sub(payment, backAmount), backEnvelope.GasSponsorValue)
	if err != nil || bAfter.BalanceWei != expectedB.String() {
		t.Fatalf("sponsored change: %+v %v", bAfter, err)
	}
	if core.ShieldedV3NullifierTransaction(st, input.Nullifier) != spend.Hash() {
		t.Fatal("missing canonical spending hash")
	}
	if _, err := Build(ctx, rpc, seedA, a, pb, new(big.Int).Add(shielded3.MaxSendWei(), big.NewInt(1)), false); err == nil {
		t.Fatal("accepted send above cap")
	}
	// An unrelated public KEM key cannot read either direction.
	wrongView := viewB
	wrongView.Incoming = viewA.Incoming
	wrongView.Outgoing = nil
	other, err := Scan(ctx, rpc, wrongView)
	if err != nil || other.BalanceWei != "0" {
		t.Fatal("credited notes to a different owner")
	}
}
