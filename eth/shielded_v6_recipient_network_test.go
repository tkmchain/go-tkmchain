package eth

import (
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/shield6test"
)

// This test consumes only a fixture written after the real canonical proof
// rehearsal passes. It reuses its signed payment, never a mock proof or a
// production setup. Genesis seeds the validated post-registration/deposit
// state; native stamp enrollment and proof of work are separate release gates.
func TestShield6RecipientTCPSpendAndReorg(t *testing.T) {
	path := os.Getenv(shield6test.FixtureEnvironment)
	if testing.Short() || path == "" {
		t.Skip("run the real canonical proof rehearsal with SHIELD6_RECIPIENT_NETWORK_FIXTURE first")
	}
	fixture, err := shield6test.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	var payment types.Transaction
	if err := payment.UnmarshalBinary(fixture.Payment); err != nil {
		t.Fatal(err)
	}
	envelope, recognized, err := core.DecodeShieldedV6Transaction(payment.Data())
	if err != nil || !recognized || envelope.Version != core.ShieldedV6BoundVersion || envelope.Recipients == nil || len(envelope.Outputs) != 2 || len(envelope.Recipients.Proofs) != 2 {
		t.Fatal("network fixture is not a proof-bearing payment and change")
	}
	sender, err := types.Sender(types.NewQuantumSigner(fixture.Config.ChainID), &payment)
	if err != nil || sender != fixture.Sender || payment.Nonce() != fixture.SenderNonce {
		t.Fatal("network fixture sender mismatch", err)
	}
	genesis := &core.Genesis{Config: fixture.Config, Timestamp: fixture.Timestamp, GasLimit: 60_000_000, BaseFee: new(big.Int), Alloc: fixture.Alloc}
	source, sink := newShield6NetworkNode(t, genesis), newShield6NetworkNode(t, genesis)
	if len(source.handler.requiredBlocks) != 0 || len(sink.handler.requiredBlocks) != 0 {
		t.Fatal("isolated proof fixture unexpectedly activated production checkpoint challenges")
	}
	source.server.AddPeer(sink.server.Self())
	waitShield6Network(t, "proof-bearing eth handshake", func() bool { return source.handler.peers.len() == 1 && sink.handler.peers.len() == 1 })
	stateAtHead := func(node *shield6NetworkNode) *state.StateDB {
		t.Helper()
		st, err := node.chain.StateAt(node.chain.CurrentBlock())
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	checkState := func(node *shield6NetworkNode, spent bool) {
		t.Helper()
		st := stateAtHead(node)
		outputs, images, nonce := fixture.OutputCount, uint64(0), fixture.SenderNonce
		if spent {
			outputs += uint64(len(envelope.Outputs))
			images, nonce = 1, nonce+1
		}
		if core.ShieldedV6OutputCount(st) != outputs || core.ShieldedV6KeyImageCount(st) != images || st.GetNonce(sender) != nonce || core.ShieldedV6BindingCount(st) != 2 {
			t.Fatal("proof-bearing network state disagrees with expected outputs, key images, nonce or bindings")
		}
		if err := core.ValidateShieldedV6State(st, &payment); !spent && err != nil {
			t.Fatal("reorganized payment did not regain valid canonical roots and inputs", err)
		}
	}
	for _, node := range []*shield6NetworkNode{source, sink} {
		checkState(node, false)
	}
	for _, encoded := range fixture.Attacks {
		var attack types.Transaction
		if err := attack.UnmarshalBinary(encoded); err != nil {
			t.Fatal(err)
		}
		if errs := source.pool.Add([]*types.Transaction{&attack}, true); errs[0] == nil {
			t.Fatal("real pool accepted a signed recipient proof substitution")
		}
	}
	propagationStart := time.Now()
	if errs := source.pool.Add([]*types.Transaction{&payment}, true); errs[0] != nil {
		t.Fatal("real pool rejected the proof-bearing payment", errs[0])
	}
	admission := time.Since(propagationStart)
	waitShield6Network(t, "recipient payment proof propagation", func() bool { return sink.pool.Has(payment.Hash()) })
	t.Logf("genuine proof-bearing payment: source pool validation %s; second-pool TCP propagation including source validation %s", admission, time.Since(propagationStart))
	importStart := time.Now()
	spent, _ := core.GenerateChain(fixture.Config, source.chain.Genesis(), randomx.NewFaker(), source.db, 1, func(_ int, block *core.BlockGen) { block.AddTx(&payment) })
	if _, err := source.chain.InsertChain(spent); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "proof-bearing payment block sync", func() bool { return sink.chain.CurrentBlock().Hash() == spent[0].Hash() })
	t.Logf("proof-bearing block construction, canonical import and second-peer sync: %s", time.Since(importStart))
	for _, node := range []*shield6NetworkNode{source, sink} {
		checkState(node, true)
	}
	reorgStart := time.Now()
	fork, _ := core.GenerateChain(fixture.Config, source.chain.Genesis(), randomx.NewFaker(), source.db, 2, func(_ int, block *core.BlockGen) { block.SetExtra([]byte("recipient proof alternate branch")) })
	if _, err := source.chain.InsertChain(fork); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "proof-bearing payment rollback sync", func() bool { return sink.chain.CurrentBlock().Hash() == fork[1].Hash() })
	t.Logf("longer-branch construction, canonical reorg and second-peer sync: %s", time.Since(reorgStart))
	for _, node := range []*shield6NetworkNode{source, sink} {
		checkState(node, false)
	}
	replay, _ := core.GenerateChain(fixture.Config, fork[1], randomx.NewFaker(), source.db, 1, func(_ int, block *core.BlockGen) { block.AddTx(&payment) })
	if _, err := source.chain.InsertChain(replay); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "proof-bearing exact transaction re-inclusion", func() bool { return sink.chain.CurrentBlock().Hash() == replay[0].Hash() })
	for _, node := range []*shield6NetworkNode{source, sink} {
		checkState(node, true)
		if stateAtHead(node).IntermediateRoot(true) != replay[0].Root() {
			t.Fatal("proof-bearing peers disagree on final state root")
		}
		if err := node.pool.Sync(); err != nil {
			t.Fatal(err)
		}
		if errs := node.pool.Add([]*types.Transaction{&payment}, true); errs[0] == nil {
			t.Fatal("real pool accepted a canonically spent recipient payment")
		}
	}
	t.Log("real recipient proofs passed signed-substitution rejection, TCP propagation, canonical import, rollback and exact replay on two peers")
}
