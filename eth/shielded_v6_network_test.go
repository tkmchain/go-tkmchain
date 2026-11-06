package eth

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/txpool/legacypool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

type shield6NetworkNode struct {
	db      ethdb.Database
	chain   *core.BlockChain
	pool    *txpool.TxPool
	handler *handler
	server  *p2p.Server
}

func newShield6NetworkNode(t *testing.T, genesis *core.Genesis) *shield6NetworkNode {
	t.Helper()
	n := &shield6NetworkNode{db: rawdb.NewMemoryDatabase()}
	cfg := core.DefaultConfig()
	cfg.SnapshotLimit = 0
	cfg.TrieCleanLimit, cfg.TrieDirtyLimit = 16, 16
	var err error
	n.chain, err = core.NewBlockChain(n.db, genesis, randomx.NewFaker(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopEthTestChain(n.chain); n.db.Close() })
	poolCfg := legacypool.DefaultConfig
	poolCfg.Journal = ""
	n.pool, err = txpool.New(1, n.chain, []txpool.SubPool{legacypool.New(poolCfg, n.chain)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.pool.Close() })
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	n.handler, err = newHandler(&handlerConfig{Database: n.db, Chain: n.chain, TxPool: n.pool, Network: 18979, Sync: ethconfig.FullSync, BloomCache: 1})
	if err != nil {
		t.Fatal(err)
	}
	n.handler.Start(4)
	n.handler.synced.Store(true)
	t.Cleanup(n.handler.Stop)
	n.server = &p2p.Server{Config: p2p.Config{PrivateKey: key, MaxPeers: 4, NoDiscovery: true, ListenAddr: "127.0.0.1:0", Name: "gtkm/v1.21.82/shield6-rehearsal", Protocols: ethproto.MakeProtocols((*ethHandler)(n.handler), 18979, nil)}}
	if err := n.server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.server.Stop)
	return n
}

func waitShield6Network(t *testing.T, label string, check func() bool) {
	t.Helper()
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if check() {
			return
		}
		select {
		case <-timeout.C:
			t.Fatalf("timed out waiting for %s", label)
		case <-tick.C:
		}
	}
}

// TestShield6TCPForkSpendAndReorg uses actual encrypted loopback TCP peers,
// production transaction pools, the eth downloader and blockchain importer.
// Only proof of work is faked; the sender stamp is seeded in isolated genesis.
// It validates the experimental envelope's networking. The proof-bearing
// recipient relation has a separate real-proof canonical integration gate;
// this test does not close its TCP network release gate.
func TestShield6TCPForkSpendAndReorg(t *testing.T) {
	const chainID = uint64(18979)
	config := *params.TestChainConfig
	config.ChainID = new(big.Int).SetUint64(chainID)
	config.QuantumResistantTime, config.AntarticalTime = new(uint64), new(uint64)
	config.PrivacyCommitmentTime = new(uint64)
	activation := uint64(20)
	config.Shield6Time = &activation
	config.ConfidentialLedgerTime = &activation
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{0x38}, pqcrypto.MLDSA87SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pqcrypto.PublicKeyBytes(accountKey)
	account, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	wallet, seed, err := shielded6.CreateWallet(rand.Reader, chainID)
	clear(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	defer wallet.Clear()
	genesis := &core.Genesis{Config: &config, Timestamp: 1, GasLimit: 60_000_000, BaseFee: big.NewInt(1), Alloc: types.GenesisAlloc{
		account:                    {Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)},
		params.ShieldedPoolAddress: {Storage: map[common.Hash]common.Hash{core.ShieldedV3StateSlot("stamp/address", account.Bytes()): common.HexToHash("0x1")}},
	}}
	source, sink := newShield6NetworkNode(t, genesis), newShield6NetworkNode(t, genesis)
	source.server.AddPeer(sink.server.Self())
	waitShield6Network(t, "eth handshake", func() bool { return source.handler.peers.len() == 1 && sink.handler.peers.len() == 1 })
	t.Log("loopback eth handshake completed")
	signer := types.NewQuantumSigner(config.ChainID)
	skeleton := func(nonce uint64, value *big.Int, data []byte) *types.Transaction {
		return types.NewTx(&types.PQTkmTx{ChainID: config.ChainID, Nonce: nonce, GasTipCap: big.NewInt(1_000_000_000), GasFeeCap: big.NewInt(2_000_000_000), Gas: 40_000_000, To: &params.ShieldedPoolAddress, Value: value, Data: data, Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: publicKey})
	}
	outputs := make([]core.ShieldedV6WalletOutput, shielded6.RingSize)
	for i := range outputs {
		outputs[i] = core.ShieldedV6WalletOutput{Address: wallet.Address(), Amount: big.NewInt(1)}
	}
	depositSkeleton := skeleton(0, big.NewInt(shielded6.RingSize), nil)
	depositData, err := core.BuildShieldedV6DepositEnvelope(depositSkeleton, outputs, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deposit, err := types.SignPQTkmTx(skeleton(0, depositSkeleton.Value(), depositData), signer, accountKey)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that the real pool rejects the envelope before fork activation.
	if errs := source.pool.Add([]*types.Transaction{deposit}, true); errs[0] == nil {
		t.Fatal("pre-fork pool accepted Shield6")
	}
	first, _ := core.GenerateChain(&config, source.chain.Genesis(), randomx.NewFaker(), source.db, 1, nil)
	if _, err := source.chain.InsertChain(first); err != nil {
		t.Fatal(err)
	}
	// Block 1 is pre-activation; block 2 crosses the configured fork time.
	funded, _ := core.GenerateChain(&config, first[0], randomx.NewFaker(), source.db, 1, func(_ int, b *core.BlockGen) { b.AddTx(deposit) })
	if _, err := source.chain.InsertChain(funded); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "fork and deposit sync", func() bool { return sink.chain.CurrentBlock().Hash() == funded[0].Hash() })
	stateAtHead := func(n *shield6NetworkNode) *state.StateDB {
		t.Helper()
		st, err := n.chain.StateAt(n.chain.CurrentBlock())
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	st := stateAtHead(source)
	if core.ShieldedV6OutputCount(st) != shielded6.RingSize {
		t.Fatal("deposit accumulator count")
	}
	envelope, _, err := core.DecodeShieldedV6Transaction(deposit.Data())
	if err != nil {
		t.Fatal(err)
	}
	note, owned, err := shielded6.ScanOutput(wallet, 0, 0, core.ShieldedV6WalletRecord(envelope.Outputs[0]))
	if err != nil || !owned {
		t.Fatalf("scan funded note: %v owned=%v", err, owned)
	}
	defer note.Clear()
	var ring [shielded6.RingSize]shielded6.RingMember
	var indices [shielded6.RingSize]uint64
	for i := range ring {
		ring[i], err = core.ShieldedV6OutputAt(st, uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		indices[i] = uint64(i)
	}
	spendSkeleton := skeleton(1, new(big.Int), nil)
	spendData, err := core.BuildShieldedV6Envelope(spendSkeleton, []core.ShieldedV6WalletInput{{Note: note, Ring: ring, RingIndices: indices, SignerIndex: 0}}, []core.ShieldedV6WalletOutput{{Address: wallet.Address(), Amount: big.NewInt(1)}}, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spend, err := types.SignPQTkmTx(skeleton(1, new(big.Int), spendData), signer, accountKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.pool.Sync(); err != nil {
		t.Fatal(err)
	}
	malformed, _, err := core.DecodeShieldedV6Transaction(spend.Data())
	if err != nil {
		t.Fatal(err)
	}
	malformed.RangeProof[len(malformed.RangeProof)-1] ^= 1
	malformedData, err := core.EncodeShieldedV6Transaction(malformed)
	if err != nil {
		t.Fatal(err)
	}
	malformedTx, err := types.SignPQTkmTx(skeleton(1, new(big.Int), malformedData), signer, accountKey)
	if err != nil {
		t.Fatal(err)
	}
	if errs := source.pool.Add([]*types.Transaction{malformedTx}, true); errs[0] == nil {
		t.Fatal("real pool accepted a corrupted Shield6 range proof")
	}
	if errs := source.pool.Add([]*types.Transaction{spend}, true); errs[0] != nil {
		t.Fatalf("real pool rejected spend: %v", errs[0])
	}
	waitShield6Network(t, "private spend propagation through real pools", func() bool { return sink.pool.Has(spend.Hash()) })
	spent, _ := core.GenerateChain(&config, funded[0], randomx.NewFaker(), source.db, 1, func(_ int, b *core.BlockGen) { b.AddTx(spend) })
	if _, err := source.chain.InsertChain(spent); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "spend block sync", func() bool { return sink.chain.CurrentBlock().Hash() == spent[0].Hash() })
	for _, n := range []*shield6NetworkNode{source, sink} {
		if core.ShieldedV6KeyImageCount(stateAtHead(n)) != 1 {
			t.Fatal("spend key image missing")
		}
	}
	// Fork at the deposit block. The longer empty branch must revert both
	// accumulator append and key-image index through the actual chain importer.
	fork, _ := core.GenerateChain(&config, funded[0], randomx.NewFaker(), source.db, 2, func(_ int, b *core.BlockGen) { b.SetExtra([]byte("shield6 alternate branch")) })
	if _, err := source.chain.InsertChain(fork); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "longer fork reorg sync", func() bool { return sink.chain.CurrentBlock().Hash() == fork[1].Hash() })
	for _, n := range []*shield6NetworkNode{source, sink} {
		reverted := stateAtHead(n)
		if core.ShieldedV6KeyImageCount(reverted) != 0 || core.ShieldedV6OutputCount(reverted) != shielded6.RingSize || reverted.GetNonce(account) != 1 {
			t.Fatal("reorg did not restore Shield6 state and nonce")
		}
	}
	// The exact original signed spend must become valid after the rollback.
	replay, _ := core.GenerateChain(&config, fork[1], randomx.NewFaker(), source.db, 1, func(_ int, b *core.BlockGen) { b.AddTx(spend) })
	if _, err := source.chain.InsertChain(replay); err != nil {
		t.Fatal(err)
	}
	waitShield6Network(t, "re-included spend sync", func() bool { return sink.chain.CurrentBlock().Hash() == replay[0].Hash() })
	for _, n := range []*shield6NetworkNode{source, sink} {
		final := stateAtHead(n)
		if final.IntermediateRoot(true) != replay[0].Root() || core.ShieldedV6KeyImageCount(final) != 1 || core.ShieldedV6OutputCount(final) != shielded6.RingSize+1 {
			t.Fatal("final nodes disagree on canonical Shield6 state")
		}
		if err := n.pool.Sync(); err != nil {
			t.Fatal(err)
		}
		if errs := n.pool.Add([]*types.Transaction{spend}, true); errs[0] == nil {
			t.Fatal("pool accepted canonically spent transaction replay")
		}
	}
	t.Log("two TCP peers crossed the fork, propagated a validated spend, synced blocks, reverted key images and outputs, and re-included the signed spend")
}
