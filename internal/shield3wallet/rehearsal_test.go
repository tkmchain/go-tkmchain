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
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded4"
	"github.com/holiman/uint256"
)

func rehearsalNode(t *testing.T) *walletRPC {
	t.Helper()
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	return &walletRPC{state: st, pending: make(map[shielded3.Digest]common.Hash), transactions: make(map[common.Hash]*types.Transaction)}
}

func rehearsalSign(t *testing.T, tx *types.Transaction, seed []byte) *types.Transaction {
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

func rehearsalApply(t *testing.T, rpc *walletRPC, tx *types.Transaction) {
	t.Helper()
	if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, rpc.state, tx, make(map[common.Hash]struct{})); err != nil {
		t.Fatal(err)
	}
	h := &types.Header{Number: big.NewInt(1), Time: params.MainnetAntarticalTime, GasLimit: 30_000_000, BaseFee: big.NewInt(0), Difficulty: big.NewInt(1)}
	block := vm.BlockContext{CanTransfer: core.CanTransfer, Transfer: core.Transfer, GetHash: func(uint64) common.Hash { return common.Hash{} }, Coinbase: common.HexToAddress("0xcafe"), BlockNumber: h.Number, Time: h.Time, GasLimit: h.GasLimit, BaseFee: h.BaseFee, Difficulty: h.Difficulty}
	evm := vm.NewEVM(block, rpc.state, params.MainnetChainConfig, vm.Config{})
	defer evm.Release()
	rpc.state.SetTxContext(tx.Hash(), 0)
	receipt, err := core.ApplyTransaction(evm, core.NewGasPool(h.GasLimit), rpc.state, h, tx)
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("transaction execution failed: %v", err)
	}
	rpc.transactions[tx.Hash()] = tx
	if core.HasShieldedV4Prefix(tx.Data()) {
		e, _, err := core.DecodeShieldedV4Transaction(tx.Data())
		if err != nil {
			t.Fatal(err)
		}
		for _, out := range e.Outputs {
			rpc.outputs = append(rpc.outputs, scanOutput{Commitment: out.Commitment, Incoming: out.Incoming, Outgoing: out.Outgoing, TransactionHash: tx.Hash()})
		}
	}
}

func rehearsalReplicate(t *testing.T, nodes ...*walletRPC) func(*types.Transaction) {
	return func(tx *types.Transaction) {
		t.Helper()
		for _, node := range nodes {
			rehearsalApply(t, node, tx)
		}
		rootA, _ := core.ShieldedV3Root(nodes[0].state)
		for _, node := range nodes[1:] {
			root, _ := core.ShieldedV3Root(node.state)
			if root != rootA {
				t.Fatalf("node state diverged after %s", tx.Hash())
			}
		}
	}
}

func buildRehearsalWithdrawal(ctx context.Context, rpc RPC, seed []byte, identity *Identity, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	view := identity.ViewKey()
	defer view.Clear()
	scan, err := Scan(ctx, rpc, view)
	if err != nil {
		return nil, err
	}
	price := big.NewInt(1)
	sponsor := new(big.Int).Mul(new(big.Int).SetUint64(WalletGas), price)
	var note OwnedNote
	var value *big.Int
	for _, candidate := range scan.Notes {
		candidateValue, parseErr := parseAmount(candidate.ValueWei)
		if parseErr == nil && candidateValue.Cmp(new(big.Int).Add(amount, sponsor)) >= 0 {
			note, value = candidate, candidateValue
			break
		}
	}
	if value == nil {
		return nil, errors.New("no spendable Shield4 note covers withdrawal and gas")
	}
	var paths []core.ShieldedV3Path
	if err := rpc.CallContext(ctx, &paths, "tkmprivacy_shieldedV3Paths", []shielded3.Digest{note.Commitment}); err != nil {
		return nil, err
	}
	if len(paths) != 1 || !paths[0].Found {
		return nil, errors.New("withdrawal input path unavailable")
	}
	var stampPath core.ShieldedV3Path
	if err := rpc.CallContext(ctx, &stampPath, "tkmprivacy_antarticalStampPath", identity.Owner); err != nil {
		return nil, err
	}
	if !stampPath.Found {
		return nil, errors.New("withdrawal sender stamp path unavailable")
	}
	var nonce hexutil.Uint64
	if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", identity.Address, "pending"); err != nil {
		return nil, err
	}
	var gasPrice hexutil.Big
	if err := rpc.CallContext(ctx, &gasPrice, "eth_gasPrice"); err != nil {
		return nil, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	withdrawal := new(big.Int).Set(amount)
	change := new(big.Int).Sub(value, new(big.Int).Add(withdrawal, sponsor))
	if change.Sign() < 0 {
		return nil, errors.New("negative withdrawal change")
	}
	envelope := &core.ShieldedV4Transaction{Version: 4, Anchor: paths[0].Root, Nullifier: note.Nullifier, StampRoot: stampPath.Root, WithdrawalRecipient: recipient, WithdrawalValue: withdrawal, GasSponsorValue: sponsor, InputCount: 1}
	witness := shielded4.SpendWitness{SpendingSecret: identity.SpendingSecret, Randomness: note.Randomness, LeafIndex: uint32(paths[0].Index), MerklePath: paths[0].Path}
	witness.Value, err = shielded4.AmountFromBig(value)
	if err != nil {
		return nil, err
	}
	self := PaymentPayload{ChainID: identity.ChainID, Address: identity.Address, Owner: identity.Owner, IncomingPublicKey: identity.IncomingPublicKey, StampPublicKey: identity.StampPublicKey, Stamp: *identity.Stamp}
	for slot := range envelope.Outputs {
		outputValue := new(big.Int)
		if slot == len(envelope.Outputs)-1 {
			outputValue = change
		}
		var path core.ShieldedV3Path
		if err := rpc.CallContext(ctx, &path, "tkmprivacy_antarticalStampPath", self.Owner); err != nil {
			return nil, err
		}
		if !path.Found || path.Root != envelope.StampRoot {
			return nil, errors.New("withdrawal stamp root changed")
		}
		witness.StampIndices[slot], witness.StampPaths[slot] = uint32(path.Index), path.Path
		randomness, err := shielded4.GenerateSecret()
		if err != nil {
			return nil, err
		}
		limbs, err := shielded4.AmountFromBig(outputValue)
		if err != nil {
			return nil, err
		}
		witness.Outputs[slot] = shielded4.OutputOpening{Owner: self.Owner, Randomness: randomness, Value: limbs}
		commitment, err := NoteCommitment(identity.ChainID, Note{Owner: self.Owner, Randomness: randomness, ValueWei: outputValue.String(), Recipient: self.Address})
		if err != nil {
			return nil, err
		}
		oneTimeKey := OneTimeOutputKey(self.Owner, randomness, commitment)
		tag, err := newPaymentTag()
		if err != nil {
			return nil, err
		}
		note := Note{Owner: self.Owner, Randomness: randomness, ValueWei: outputValue.String(), Recipient: self.Address, OneTimeKey: oneTimeKey, PaymentTag: tag}
		envelope.Outputs[slot].Commitment, envelope.Outputs[slot].OneTimeKey = commitment, common.CopyBytes(oneTimeKey)
		plain, err := json.Marshal(note)
		if err != nil {
			return nil, err
		}
		envelope.Outputs[slot].Incoming, err = pqcrypto.SealShieldedV3(self.IncomingPublicKey, plain, core.ShieldedV3OutputContext(identity.ChainID, pqcrypto.ShieldedV3Incoming, commitment))
		if err != nil {
			clear(plain)
			return nil, err
		}
		envelope.Outputs[slot].Outgoing, err = pqcrypto.SealShieldedV3(identity.OutgoingPublicKey, plain, core.ShieldedV3OutputContext(identity.ChainID, pqcrypto.ShieldedV3Outgoing, commitment))
		clear(plain)
		if err != nil {
			return nil, err
		}
		envelope.Outputs[slot].Stamp, err = pqcrypto.SealShieldedV3(self.StampPublicKey, identity.Stamp.Commitment[:], core.ShieldedV3OutputContext(identity.ChainID, pqcrypto.ShieldedV3Stamp, commitment))
		if err != nil {
			return nil, err
		}
	}
	makeTx := func() (*types.Transaction, error) {
		data, err := core.EncodeShieldedV4Transaction(envelope)
		if err != nil {
			return nil, err
		}
		fee := new(big.Int).Set((*big.Int)(&gasPrice))
		return types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), Nonce: uint64(nonce), GasTipCap: new(big.Int).Set(fee), GasFeeCap: fee, Gas: WalletGas, To: &params.ShieldedPoolAddress, Data: data, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(key)}), nil
	}
	unsigned, err := makeTx()
	if err != nil {
		return nil, err
	}
	statement, err := core.ShieldedV4Statement(unsigned, envelope)
	if err != nil {
		return nil, err
	}
	derived, err := (shielded4.NativeBackend{}).Describe(ctx, statement, witness)
	if err != nil {
		return nil, err
	}
	envelope.LinkTag = derived.LinkTag
	statement, err = core.ShieldedV4Statement(unsigned, envelope)
	if err != nil {
		return nil, err
	}
	envelope.Proof, err = (shielded4.NativeBackend{}).Prove(ctx, statement, witness)
	if err != nil {
		return nil, err
	}
	if err := (shielded4.NativeBackend{}).Verify(ctx, statement, envelope.Proof); err != nil {
		return nil, err
	}
	return makeTx()
}

func TestShield4MultiNodeRehearsal(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires -tags shield3 and the native static library")
	}
	ctx := context.Background()
	nodeA, nodeB := rehearsalNode(t), rehearsalNode(t)
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
	reserve := uint256.MustFromBig(new(big.Int).Mul(big.NewInt(1000), big.NewInt(1_000_000_000_000_000_000)))
	for _, node := range []*walletRPC{nodeA, nodeB} {
		node.state.AddBalance(a.Address, reserve, tracing.BalanceChangeUnspecified)
		node.state.AddBalance(b.Address, reserve, tracing.BalanceChangeUnspecified)
	}
	replicate := rehearsalReplicate(t, nodeA, nodeB)
	for _, entry := range []struct {
		seed []byte
		id   *Identity
		rpc  *walletRPC
	}{{seedA, a, nodeA}, {seedB, b, nodeB}} {
		unsigned, err := BuildStamp(ctx, entry.rpc, entry.seed, entry.id)
		if err != nil {
			t.Fatal("stamp build:", err)
		}
		replicate(rehearsalSign(t, unsigned, entry.seed))
	}
	for _, node := range []*walletRPC{nodeA, nodeB} {
		node.state.SubBalance(b.Address, node.state.GetBalance(b.Address), tracing.BalanceChangeUnspecified)
	}
	one := big.NewInt(1_000_000_000_000_000_000)
	for i := 0; i < 3; i++ {
		unsigned, err := BuildV4(ctx, nodeA, seedA, a, pb, one, true)
		if err != nil {
			t.Fatal("deposit build:", err)
		}
		replicate(rehearsalSign(t, unsigned, seedA))
	}
	bView := b.ViewKey()
	deposits, err := Scan(ctx, nodeB, bView)
	if err != nil || len(deposits.Notes) != 3 {
		t.Fatalf("deposit scan: notes=%d result=%+v err=%v", len(deposits.Notes), deposits, err)
	}
	batchUnsigned, err := BuildV4Batch(ctx, nodeB, seedB, b, []Payment{{pa, big.NewInt(1_500_000_000_000_000_000)}, {pb, one}})
	if err != nil {
		t.Fatal("multi-input build:", err)
	}
	batch, _, err := core.DecodeShieldedV4Transaction(batchUnsigned.Data())
	if err != nil || batch.InputCount < 2 {
		t.Fatalf("multi-input envelope: %+v %v", batch, err)
	}
	replicate(rehearsalSign(t, batchUnsigned, seedB))
	offer, err := BuildRelayOffer(ctx, nodeA, seedA, a)
	if err != nil {
		t.Fatal("relay offer:", err)
	}
	relayUnsigned, err := BuildV4Relayed(ctx, nodeB, seedB, b, pa, big.NewInt(200_000_000_000_000_000), &offer)
	if err != nil {
		t.Fatal("relay build:", err)
	}
	packet, err := RelayPacketForTransaction(relayUnsigned)
	if err != nil || packet.InputCount == 0 {
		t.Fatalf("relay packet: %+v %v", packet, err)
	}
	reviewed, err := BuildRelaySubmission(ctx, nodeA, seedA, a, packet.Transaction)
	if err != nil {
		t.Fatal("relay review:", err)
	}
	relay := rehearsalSign(t, reviewed, seedA)
	replicate(relay)
	for _, node := range []*walletRPC{nodeA, nodeB} {
		if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, node.state, relay, make(map[common.Hash]struct{})); err == nil {
			t.Fatal("replayed relay accepted")
		}
	}
	withdrawUnsigned, err := buildRehearsalWithdrawal(ctx, nodeB, seedB, b, a.Address, big.NewInt(200_000_000_000_000_000))
	if err != nil {
		t.Fatal("withdrawal build:", err)
	}
	withdraw, _, err := core.DecodeShieldedV4Transaction(withdrawUnsigned.Data())
	if err != nil || withdraw.WithdrawalValue.Sign() <= 0 || withdraw.WithdrawalRecipient != a.Address {
		t.Fatalf("withdrawal envelope: %+v %v", withdraw, err)
	}
	aBefore := nodeA.state.GetBalance(a.Address).ToBig()
	replicate(rehearsalSign(t, withdrawUnsigned, seedB))
	after := nodeA.state.GetBalance(a.Address).ToBig()
	if new(big.Int).Sub(after, aBefore).Cmp(withdraw.WithdrawalValue) != 0 {
		t.Fatalf("withdrawal release: before=%s after=%s amount=%s", aBefore, after, withdraw.WithdrawalValue)
	}
	for _, node := range []*walletRPC{nodeA, nodeB} {
		if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, node.state, withdrawUnsigned, make(map[common.Hash]struct{})); err == nil {
			t.Fatal("replayed withdrawal accepted")
		}
	}
	nodeB.reorg = true
	if _, err := Scan(ctx, nodeB, bView); err == nil {
		t.Fatal("scan accepted a reorged head")
	}
	nodeB.reorg = false
	bView.Clear()
}
