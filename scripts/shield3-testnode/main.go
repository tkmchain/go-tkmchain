// shield3-testnode mines and spends test funds on a fresh, isolated chain.
// Build with -tags randomx,cgo,shield3 after scripts/shield3-build.sh.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/internal/shield3relay"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	tlog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type countingRPC struct {
	shield3wallet.RPC
	broadcasts atomic.Int64
}

func (r *countingRPC) CallContext(ctx context.Context, dest any, method string, args ...any) error {
	if method == "eth_sendRawTransaction" {
		r.broadcasts.Add(1)
	}
	return r.RPC.CallContext(ctx, dest, method, args...)
}

type actor struct {
	Seed     []byte                          `json:"seed"`
	Stamp    *pqcrypto.ShieldedV3StampRecord `json:"stamp"`
	Identity *shield3wallet.Identity         `json:"-"`
}
type txResult struct {
	Label  string      `json:"label"`
	Hash   common.Hash `json:"hash"`
	Block  uint64      `json:"block"`
	Gas    uint64      `json:"gasUsed"`
	Status uint64      `json:"status"`
}
type report struct {
	StartedUTC       string            `json:"startedUTC"`
	FinishedUTC      string            `json:"finishedUTC"`
	FinalHeight      uint64            `json:"finalHeight"`
	RelayBroadcasts  int64             `json:"relayBroadcasts"`
	ChainID          uint64            `json:"chainId"`
	Endpoint         string            `json:"rpcEndpoint"`
	MinedBlocks      uint64            `json:"initialFundingBlocks"`
	InitialPublicWei string            `json:"minedPublicBalanceWei"`
	Transactions     []txResult        `json:"transactions"`
	Balances         map[string]string `json:"shieldedBalancesWei"`
	Checks           []string          `json:"checks"`
	Error            string            `json:"error,omitempty"`
	Success          bool              `json:"success"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func save(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	must(os.WriteFile(path, b, 0600))
}
func amount(n int64) *big.Int { return new(big.Int).Mul(big.NewInt(n), big.NewInt(1e18)) }
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "TEST FAILED:", err)
		os.Exit(1)
	}
}
func run() (err error) {
	output := flag.String("output", "", "new directory for private test-chain data and public evidence; must not exist")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if !shielded3.NativeAvailable() {
		return fmt.Errorf("build with -tags randomx,cgo,shield3 and the current native libraries")
	}
	dir := *output
	var e error
	if dir == "" {
		dir, e = os.MkdirTemp("", "tkm-shield3-live-")
	} else {
		e = os.Mkdir(dir, 0700)
	}
	if e != nil {
		return fmt.Errorf("fresh test directory: %w", e)
	}
	fmt.Println("TEST evidence directory", dir)
	rep := report{StartedUTC: time.Now().UTC().Format(time.RFC3339), ChainID: 8980, Balances: map[string]string{}}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
		if err != nil {
			rep.Error = err.Error()
		}
		rep.FinishedUTC = time.Now().UTC().Format(time.RFC3339)
		save(filepath.Join(dir, "report.json"), rep)
	}()
	lf, e := os.OpenFile(filepath.Join(dir, "node.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	must(e)
	defer lf.Close()
	tlog.SetDefault(tlog.NewLogger(tlog.NewTerminalHandlerWithLevel(lf, slog.LevelInfo, false)))
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	actors := make([]actor, 4)
	for i := range actors {
		actors[i].Seed = make([]byte, 32)
		_, e = rand.Read(actors[i].Seed)
		must(e)
		actors[i].Stamp, e = pqcrypto.CreateShieldedV3Stamp(actors[i].Seed, rep.ChainID, fmt.Sprintf("Test Wallet %d", i), "Test Country")
		must(e)
		actors[i].Identity, e = shield3wallet.NewIdentity(actors[i].Seed, rep.ChainID, actors[i].Stamp)
		must(e)
		defer actors[i].Identity.Clear()
		defer clear(actors[i].Seed)
	}
	a, b, c := actors[0].Identity, actors[1].Identity, actors[2].Identity
	fmt.Printf("TEST wallets sender=%s recipient=%s operator=%s\n", a.Address, b.Address, c.Address)
	chain := *params.MainnetChainConfig
	chain.ChainID = big.NewInt(int64(rep.ChainID))
	zero := new(uint64)
	chain.AntarticalTime = zero
	chain.QuantumResistantTime = zero
	chain.PrivacyCommitmentTime = zero
	chain.PQMigrationRecoveryTime = zero
	chain.ShanghaiTime = zero
	chain.CancunTime = zero
	chain.EDATime = zero
	chain.KyotoTime = zero
	chain.PhoneTime = zero
	chain.RandomXTxBlock = big.NewInt(0)
	chain.RandomXMoneroBlock = big.NewInt(0)
	chain.MainKingAddress = common.Address{}
	chain.PostQuantumMainKingAddress = common.Address{}
	chain.RotatingKingAddresses = nil
	rx := *params.DefaultRandomXConfig()
	rx.PersistDataset = false
	chain.RandomX = &rx
	genesis := &core.Genesis{Config: &chain, Timestamp: uint64(time.Now().Unix() - 10), GasLimit: 30_000_000, Difficulty: big.NewInt(1), BaseFee: big.NewInt(1), Alloc: types.GenesisAlloc{}}
	save(filepath.Join(dir, "genesis.json"), genesis)
	nc := node.DefaultConfig
	nc.Name = "shield3-live-test"
	nc.DataDir = filepath.Join(dir, "node")
	nc.IPCPath = ""
	nc.HTTPHost = "127.0.0.1"
	nc.HTTPPort = 0
	nc.HTTPVirtualHosts = []string{"127.0.0.1", "localhost"}
	nc.HTTPModules = []string{"eth", "net", "web3", "miner", "tkmprivacy"}
	nc.WSHost = ""
	nc.AuthPort = 0
	nc.P2P = p2p.Config{MaxPeers: 0, NoDiscovery: true, ListenAddr: ""}
	stack, e := node.New(&nc)
	must(e)
	closed := false
	defer func() {
		if !closed {
			stack.Close()
		}
	}()
	ec := ethconfig.Defaults
	ec.Genesis = genesis
	ec.NetworkId = rep.ChainID
	ec.SyncMode = ethconfig.FullSync
	ec.StateScheme = rawdb.HashScheme
	ec.NoPruning = true
	ec.DatabaseCache = 64
	ec.TrieCleanCache = 32
	ec.TrieDirtyCache = 0
	ec.SnapshotCache = 0
	ec.Miner.Enabled = true
	ec.Miner.Etherbase = a.Address
	ec.Miner.GasPrice = big.NewInt(1)
	ec.Miner.GasCeil = 30_000_000
	ec.Miner.Recommit = time.Second
	ec.RandomXMinerThreads = 1
	ec.TxPool.Journal = ""
	ec.BlobPool.Datadir = "blobpool"
	ec.GPO.IgnorePrice = big.NewInt(2)
	backend, e := eth.New(stack, &ec)
	must(e)
	fs := filters.NewFilterSystem(backend.APIBackend, filters.Config{})
	stack.RegisterAPIs([]rpc.API{{Namespace: "eth", Service: filters.NewFilterAPI(fs)}})
	must(stack.Start())
	rep.Endpoint = stack.HTTPEndpoint()
	fmt.Println("TEST RPC", rep.Endpoint)
	client, e := rpc.DialContext(ctx, rep.Endpoint)
	must(e)
	defer client.Close()
	var status eth.ShieldedV3Status
	must(client.CallContext(ctx, &status, "tkmprivacy_shieldedV3Status"))
	if !status.Active || !status.NativeVerifier {
		panic("Shield3 unavailable")
	}
	rep.Checks = append(rep.Checks, "Antartical active with native verifier")
	for _, actor := range actors {
		var starting hexutil.Big
		must(client.CallContext(ctx, &starting, "eth_getBalance", actor.Identity.Address, "latest"))
		if (*big.Int)(&starting).Sign() != 0 {
			panic("test wallet was prefunded")
		}
	}
	must(backend.StartMining())
	defer backend.StopMining()
	deadline := time.Now().Add(90 * time.Second)
	for backend.BlockChain().CurrentBlock().Number.Uint64() < 3 {
		if time.Now().After(deadline) {
			panic("RandomX did not mine three blocks within 90 seconds")
		}
		time.Sleep(200 * time.Millisecond)
	}
	must(backend.StopMining())
	rep.MinedBlocks = backend.BlockChain().CurrentBlock().Number.Uint64()
	for n := uint64(1); n <= rep.MinedBlocks; n++ {
		h := backend.BlockChain().GetHeaderByNumber(n)
		if h.MixDigest == (common.Hash{}) {
			panic("mined zero mix digest")
		}
		must(backend.Engine().VerifyHeader(backend.BlockChain(), h))
	}
	var public hexutil.Big
	must(client.CallContext(ctx, &public, "eth_getBalance", a.Address, "latest"))
	rep.InitialPublicWei = (*big.Int)(&public).String()
	if (*big.Int)(&public).Cmp(amount(10)) < 0 {
		panic("mining failed to fund sender")
	}
	fmt.Printf("TEST mined %d genuine RandomX blocks; public rewards=%s wei\n", rep.MinedBlocks, rep.InitialPublicWei)
	rep.Checks = append(rep.Checks, "real RandomX blocks have nonzero verified mix digests; zero genesis allocation")
	sign := func(tx *types.Transaction, index int) *types.Transaction {
		key, e := pqcrypto.NewMLDSA87FromSeed(actors[index].Seed)
		must(e)
		signed, e := types.SignPQTkmTx(tx, types.NewQuantumSigner(chain.ChainID), key)
		must(e)
		return signed
	}
	confirm := func(label string, tx *types.Transaction, broadcast bool) {
		raw, e := tx.MarshalBinary()
		must(e)
		must(os.WriteFile(filepath.Join(dir, label+".bin"), raw, 0600))
		hash := tx.Hash()
		if broadcast {
			if sendErr := client.CallContext(ctx, &hash, "eth_sendRawTransaction", hexutil.Bytes(raw)); sendErr != nil {
				var known hexutil.Bytes
				if lookupErr := client.CallContext(ctx, &known, "eth_getRawTransactionByHash", tx.Hash()); lookupErr != nil || string(known) != string(raw) {
					panic(sendErr)
				}
				hash = tx.Hash()
			}
		}
		if hash != tx.Hash() {
			panic("broadcast hash mismatch")
		}
		fmt.Printf("TEST submitted %s %s (%d bytes)\n", label, hash, len(raw))
		must(backend.StartMining())
		defer backend.StopMining()
		var receipt *types.Receipt
		deadline := time.Now().Add(120 * time.Second)
		for {
			must(client.CallContext(ctx, &receipt, "eth_getTransactionReceipt", hash))
			if receipt != nil {
				break
			}
			if time.Now().After(deadline) {
				panic(label + " not included within 120 seconds")
			}
			time.Sleep(250 * time.Millisecond)
		}
		must(backend.StopMining())
		if receipt.Status != types.ReceiptStatusSuccessful {
			panic(label + " reverted")
		}
		rep.Transactions = append(rep.Transactions, txResult{label, hash, receipt.BlockNumber.Uint64(), receipt.GasUsed, receipt.Status})
		fmt.Printf("TEST confirmed %s block=%d gas=%d status=1\n", label, receipt.BlockNumber.Uint64(), receipt.GasUsed)
	}
	fmt.Println("TEST proving sender stamp")
	stamp, e := shield3wallet.BuildStamp(ctx, client, actors[0].Seed, a)
	must(e)
	confirm("sender-stamp", sign(stamp, 0), true)
	for _, idx := range []int{1, 2} {
		fmt.Println("TEST sponsored stamp", idx)
		offer, e := shield3wallet.BuildStampSponsorshipOffer(ctx, client, actors[0].Seed, a, actors[idx].Identity.Code)
		must(e)
		auth, e := shield3wallet.AuthorizeStampSponsorship(ctx, client, actors[idx].Seed, actors[idx].Identity, offer.Transaction)
		must(e)
		tx, e := shield3wallet.BuildSponsoredStamp(ctx, client, actors[0].Seed, a, auth.Transaction)
		must(e)
		confirm(fmt.Sprintf("sponsored-stamp-%d", idx), sign(tx, 0), true)
	}
	pa, e := shield3wallet.DecodePaymentCode(a.Code, rep.ChainID)
	must(e)
	pb, e := shield3wallet.DecodePaymentCode(b.Code, rep.ChainID)
	must(e)
	pc, e := shield3wallet.DecodePaymentCode(c.Code, rep.ChainID)
	must(e)
	if _, e := shield3wallet.BuildBatch(ctx, client, actors[0].Seed, a, []shield3wallet.Payment{{Recipient: pb, Amount: amount(5_000_001)}}); e == nil {
		panic("over-limit send accepted")
	}
	rep.Checks = append(rep.Checks, "aggregate 5 million TKM limit rejects excessive send")
	fmt.Println("TEST proving 10 TKM deposit")
	deposit, e := shield3wallet.Build(ctx, client, actors[0].Seed, a, pa, amount(10), true)
	must(e)
	confirm("shield-deposit-10", sign(deposit, 0), true)
	scan := func(id *shield3wallet.Identity) shield3wallet.ScanResult {
		view := id.ViewKey()
		defer view.Clear()
		s, e := shield3wallet.Scan(ctx, client, view)
		must(e)
		return s
	}
	if s := scan(a); s.BalanceWei != amount(10).String() {
		panic("deposit balance mismatch: " + s.BalanceWei)
	}
	fmt.Println("TEST proving direct 2 TKM send")
	direct, e := shield3wallet.Build(ctx, client, actors[0].Seed, a, pb, amount(2), false)
	must(e)
	confirm("direct-send-2", sign(direct, 0), true)
	if scan(a).BalanceWei != amount(8).String() || scan(b).BalanceWei != amount(2).String() {
		panic("direct balance mismatch")
	}
	rep.Checks = append(rep.Checks, "direct 2 TKM transfer confirmed with 8/2 TKM shielded balances")
	fmt.Println("TEST splitting remaining 8 TKM into four 2 TKM inputs")
	split, e := shield3wallet.BuildBatch(ctx, client, actors[0].Seed, a, []shield3wallet.Payment{{Recipient: pa, Amount: amount(2)}, {Recipient: pa, Amount: amount(2)}, {Recipient: pa, Amount: amount(2)}})
	must(e)
	confirm("split-four-inputs", sign(split, 0), true)
	if splitScan := scan(a); len(splitScan.Notes) != 4 || splitScan.BalanceWei != amount(8).String() {
		panic("input split did not produce four spendable notes")
	}
	unregistered, e := shield3wallet.DecodePaymentCode(actors[3].Identity.Code, rep.ChainID)
	must(e)
	if _, e := shield3wallet.Build(ctx, client, actors[0].Seed, a, unregistered, amount(1), true); e == nil {
		panic("unstamped receiving address accepted")
	}
	rep.Checks = append(rep.Checks, "wallet refuses Shield3 deposits to unregistered stamps")
	var publicOperator hexutil.Big
	must(client.CallContext(ctx, &publicOperator, "eth_getBalance", c.Address, "latest"))
	if (*big.Int)(&publicOperator).Sign() != 0 {
		panic("operator must start with no public funds")
	}
	var payerNonce hexutil.Uint64
	must(client.CallContext(ctx, &payerNonce, "eth_getTransactionCount", a.Address, "latest"))
	observed := &countingRPC{RPC: client}
	service, e := shield3relay.New(observed, actors[2].Seed, c, filepath.Join(dir, "relay-state"))
	must(e)
	srv := httptest.NewServer(service)
	defer func() { srv.Close(); service.Close() }()
	requestID := "isolated-shield3-live-batch-20260916"
	fmt.Println("TEST automatic relay quote", srv.URL)
	offer, e := shield3wallet.FetchRelayOffer(ctx, srv.URL, requestID)
	must(e)
	must(func() error { _, e := shield3wallet.ReviewRelayOffer(ctx, client, &offer, rep.ChainID); return e }())
	fmt.Println("TEST proving relayed batch: 1 TKM recipient, 2 TKM operator, 3 TKM self")
	batch, e := shield3wallet.BuildRelayedBatch(ctx, client, actors[0].Seed, a, []shield3wallet.Payment{{Recipient: pb, Amount: amount(1)}, {Recipient: pc, Amount: amount(2)}, {Recipient: pa, Amount: amount(3)}}, &offer)
	must(e)
	draft, e := batch.MarshalBinary()
	must(e)
	reply, e := shield3wallet.SubmitRelayPacket(ctx, srv.URL, requestID, draft)
	must(e)
	var signed types.Transaction
	must(signed.UnmarshalBinary(reply.Transaction))
	confirm("relayed-batch-6", &signed, false)
	if observed.broadcasts.Load() != 1 {
		panic("relay first submission was not broadcast exactly once")
	}
	srv.Close()
	must(service.Close())
	service, e = shield3relay.New(observed, actors[2].Seed, c, filepath.Join(dir, "relay-state"))
	must(e)
	srv = httptest.NewServer(service)
	again, e := shield3wallet.SubmitRelayPacket(ctx, srv.URL, requestID, draft)
	must(e)
	if again.SubmissionUncertain || again.TransactionHash != reply.TransactionHash || !bytes.Equal(again.Transaction, reply.Transaction) || observed.broadcasts.Load() != 1 {
		panic("relay exact retry changed transaction")
	}
	rep.Checks = append(rep.Checks, "after actual relay restart, confirmed retry returns identical hash/bytes with no new broadcast or uncertainty")
	rep.RelayBroadcasts = observed.broadcasts.Load()
	var afterNonce hexutil.Uint64
	must(client.CallContext(ctx, &afterNonce, "eth_getTransactionCount", a.Address, "latest"))
	if afterNonce != payerNonce {
		panic("relay changed payer public nonce")
	}
	outer, e := types.Sender(types.NewQuantumSigner(chain.ChainID), &signed)
	must(e)
	if outer != c.Address || outer == a.Address {
		panic("relay exposed payer as outer signer")
	}
	payerKey, e := pqcrypto.NewMLDSA87FromSeed(actors[0].Seed)
	must(e)
	if bytes.Contains(draft, pqcrypto.PublicKeyBytes(payerKey)) {
		panic("payer public key appears in relay draft")
	}
	rep.Checks = append(rep.Checks, "zero-public-balance operator funds gas privately; payer public key omitted and payer nonce unchanged")
	sa, sb, sc := scan(a), scan(b), scan(c)
	fee := new(big.Int).Mul(new(big.Int).SetUint64(offer.Gas), (*big.Int)(offer.GasFeeCap))
	wantA := new(big.Int).Sub(amount(5), fee)
	if sa.BalanceWei != wantA.String() || sb.BalanceWei != amount(3).String() || sc.BalanceWei != amount(2).String() {
		panic(fmt.Sprintf("batch balances wrong sender=%s recipient=%s operator=%s fee=%s", sa.BalanceWei, sb.BalanceWei, sc.BalanceWei, fee))
	}
	rep.Balances["sender"] = sa.BalanceWei
	rep.Balances["recipient"] = sb.BalanceWei
	rep.Balances["operator"] = sc.BalanceWei
	env, found, e := core.DecodeShieldedV3Transaction(batch.Data())
	must(e)
	if !found {
		panic("missing envelope")
	}
	if env.InputCount != 4 {
		panic("relay batch did not consume four inputs")
	}
	nullifiers, e := core.ShieldedV3Nullifiers(env)
	must(e)
	for _, n := range nullifiers {
		var spent common.Hash
		must(client.CallContext(ctx, &spent, "tkmprivacy_shieldedV3Nullifier", n))
		if spent != reply.TransactionHash {
			panic("nullifier not linked to actual signed transaction")
		}
	}
	for slot, expected := range []struct {
		recipient common.Address
		amount    *big.Int
	}{{b.Address, amount(1)}, {c.Address, amount(2)}, {a.Address, amount(3)}} {
		disclosure, e := shield3wallet.ExportPaymentDisclosure(ctx, client, a, reply.TransactionHash, uint64(slot))
		must(e)
		payment, e := shield3wallet.VerifyPaymentDisclosure(ctx, client, disclosure)
		clear(disclosure.RecordKey)
		must(e)
		if payment.Recipient != expected.recipient || payment.AmountWei != expected.amount.String() {
			panic("selected payment disclosure mismatch")
		}
	}
	stampDisclosure, e := shield3wallet.ExportStampDisclosure(actors[1].Seed, b)
	must(e)
	labels, e := shield3wallet.VerifyStampDisclosure(stampDisclosure)
	clear(stampDisclosure.RecordKey)
	must(e)
	if labels.Name != "Test Wallet 1" || labels.Country != "Test Country" || labels.Address != b.Address {
		panic("stamp disclosure mismatch")
	}
	rep.Checks = append(rep.Checks, "selected payment disclosures verify all three mined outputs; separate stamp-only disclosure opens labels")
	incoming, e := b.ScopedViewKey("incoming")
	must(e)
	defer incoming.Clear()
	incomingScan, e := shield3wallet.Scan(ctx, client, incoming)
	must(e)
	if incomingScan.SpendStatusKnown || incomingScan.BalanceWei != "" || incomingScan.ReceivedWei != amount(3).String() {
		panic("receive-only scan permissions mismatch")
	}
	rep.Checks = append(rep.Checks, "receive-only scan reveals receipts but no spendable balance")
	rep.Checks = append(rep.Checks, "four-input, three-recipient batch confirmed; all nullifiers reference actual signed hash")
	rep.FinalHeight = backend.BlockChain().CurrentBlock().Number.Uint64()
	for n := uint64(1); n <= rep.FinalHeight; n++ {
		h := backend.BlockChain().GetHeaderByNumber(n)
		if h.MixDigest == (common.Hash{}) {
			panic("zero mix in test chain")
		}
		must(backend.Engine().VerifyHeader(backend.BlockChain(), h))
	}
	bad := types.CopyHeader(backend.BlockChain().CurrentBlock())
	bad.MixDigest = common.Hash{}
	if backend.Engine().VerifyHeader(backend.BlockChain(), bad) == nil {
		panic("consensus accepted mutated zero mix digest")
	}
	rep.Checks = append(rep.Checks, "every final-chain block has a valid nonzero RandomX mix; mutated zero mix is rejected")
	must(backend.StopMining())
	must(stack.Close())
	closed = true
	rep.Success = true
	fmt.Printf("TEST PASSED final shielded wei sender=%s recipient=%s operator=%s; relay fee reserve=%s\n", sa.BalanceWei, sb.BalanceWei, sc.BalanceWei, fee)
	fmt.Println("TEST report", filepath.Join(dir, "report.json"))
	return nil
}
