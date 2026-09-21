package shield3wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/holiman/uint256"
)

type walletRPC struct {
	state        *state.StateDB
	outputs      []scanOutput
	pending      map[shielded3.Digest]common.Hash
	transactions map[common.Hash]*types.Transaction
	reorg        bool
}

func (r *walletRPC) CallContext(_ context.Context, dest any, method string, args ...any) error {
	var result any
	switch method {
	case "eth_getRawTransactionByHash":
		tx := r.transactions[args[0].(common.Hash)]
		if tx == nil {
			return fmt.Errorf("unknown transaction")
		}
		raw, err := tx.MarshalBinary()
		if err != nil {
			return err
		}
		result = hexutil.Bytes(raw)
	case "eth_getTransactionReceipt":
		if r.transactions[args[0].(common.Hash)] == nil {
			result = nil
		} else {
			result = map[string]any{"transactionHash": args[0], "blockHash": common.HexToHash("0x1234"), "blockNumber": "0x0", "status": "0x1"}
		}
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
	case "tkmprivacy_antarticalStamp":
		stamp, err := core.AntarticalStampForAddress(r.state, args[0].(common.Address))
		if err != nil {
			return err
		}
		result = stamp
	case "tkmprivacy_antarticalStampPath":
		path, err := core.AntarticalStampPath(r.state, args[0].(shielded3.Digest))
		if err != nil {
			return err
		}
		result = path
	case "tkmprivacy_shieldedV3Paths":
		paths := make([]core.ShieldedV3Path, len(args[0].([]shielded3.Digest)))
		for i, c := range args[0].([]shielded3.Digest) {
			p, err := core.ShieldedV3CommitmentPath(r.state, c)
			if err != nil {
				return err
			}
			paths[i] = p
		}
		result = paths
	case "tkmprivacy_shieldedV3RootsKnown":
		result = r.state.GetState(params.ShieldedPoolAddress, core.ShieldedV3StateSlot("root", args[0].(shielded3.Digest).Bytes())) != (common.Hash{}) && r.state.GetState(params.ShieldedPoolAddress, core.ShieldedV3StateSlot("stamp/root", args[1].(shielded3.Digest).Bytes())) != (common.Hash{})
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
	rpc := &walletRPC{state: st, pending: make(map[shielded3.Digest]common.Hash), transactions: make(map[common.Hash]*types.Transaction)}
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
		t.Logf("processed canonical transaction %s", tx.Hash())
		rpc.transactions[tx.Hash()] = tx
		if core.HasAntarticalStampPrefix(tx.Data()) {
			return
		}
		e, _, err := core.DecodeShieldedV3Transaction(tx.Data())
		if err != nil {
			t.Fatal(err)
		}
		for _, out := range e.Outputs {
			rpc.outputs = append(rpc.outputs, scanOutput{Commitment: out.Commitment, Incoming: out.Incoming, Outgoing: out.Outgoing, TransactionHash: tx.Hash()})
		}
	}
	if _, err := Build(ctx, rpc, seedA, a, pb, big.NewInt(1), true); err == nil {
		t.Fatal("wallet accepted an unregistered stamp")
	}
	for _, entry := range []struct {
		seed     []byte
		identity *Identity
	}{{seedA, a}, {seedB, b}} {
		var registration *types.Transaction
		payerSeed := entry.seed
		if entry.identity == a {
			registration, err = BuildStamp(ctx, rpc, entry.seed, entry.identity)
		} else {
			if st.GetBalance(b.Address).Sign() != 0 || st.GetNonce(b.Address) != 0 {
				t.Fatal("beneficiary must start without funds or transactions")
			}
			if _, err := BuildStamp(ctx, rpc, seedB, b); err == nil {
				t.Fatal("unfunded self-registration accepted")
			}
			offer, offerErr := BuildStampSponsorshipOffer(ctx, rpc, seedA, a, b.Code)
			if offerErr != nil {
				t.Fatal(offerErr)
			}
			if _, err := AuthorizeStampSponsorship(ctx, rpc, seedA, a, offer.Transaction); err == nil {
				t.Fatal("another beneficiary accepted fee offer")
			}
			authorized, authErr := AuthorizeStampSponsorship(ctx, rpc, seedB, b, offer.Transaction)
			if authErr != nil {
				t.Fatal(authErr)
			}
			if _, err := BuildSponsoredStamp(ctx, rpc, seedB, b, authorized.Transaction); err == nil {
				t.Fatal("another fee payer accepted authorized packet")
			}
			registration, err = BuildSponsoredStamp(ctx, rpc, seedA, a, authorized.Transaction)
			payerSeed = seedA
		}
		if err != nil {
			t.Fatal(err)
		}
		signed := sign(registration, payerSeed)
		if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime-1, st, signed, nil); err == nil {
			t.Fatal("accepted pre-fork registration")
		}
		head := &types.Header{Number: big.NewInt(1), Time: params.MainnetAntarticalTime, GasLimit: 8000000, BaseFee: big.NewInt(1), Difficulty: big.NewInt(1)}
		if err := txpool.ValidateTransaction(signed, head, types.NewQuantumSigner(big.NewInt(8979)), &txpool.ValidationOptions{Config: params.MainnetChainConfig, Accept: 1 << types.PQTkmTxType, MaxSize: 128 * 1024, MinTip: big.NewInt(1)}); err != nil {
			t.Fatal("registration rejected by real txpool", err)
		}
		forged, err := core.DecodeAntarticalStamp(signed.Data())
		if err != nil {
			t.Fatal(err)
		}
		forged.Owner[0] ^= 1
		forgedData, err := core.EncodeAntarticalStamp(forged)
		if err != nil {
			t.Fatal(err)
		}
		bad := sign(types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), Nonce: signed.Nonce(), To: &params.ShieldedPoolAddress, Gas: signed.Gas(), GasFeeCap: signed.GasFeeCap(), GasTipCap: signed.GasTipCap(), Value: new(big.Int), Data: forgedData}), payerSeed)
		if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, head.Number, head.Time, st, bad, nil); err == nil {
			t.Fatal("forged owner registered")
		}
		if core.IsAntarticalStamped(st, entry.identity.Address) {
			t.Fatal("failed proof changed registry")
		}
		if entry.identity == b {
			e, err := core.DecodeAntarticalStamp(signed.Data())
			if err != nil {
				t.Fatal(err)
			}
			if err := core.ProcessAntarticalStamp(params.MainnetChainConfig, head.Number, e.ValidUntil+1, st, signed); err == nil {
				t.Fatal("expired sponsorship accepted")
			}
			for _, mutation := range []string{"nonce", "fee", "gas", "expiry", "beneficiary", "stamp"} {
				t.Run("sponsorship-binding-"+mutation, func(t *testing.T) {
					changed, err := core.DecodeAntarticalStamp(signed.Data())
					if err != nil {
						t.Fatal(err)
					}
					nonce, gas := signed.Nonce(), signed.Gas()
					fee := new(big.Int).Set(signed.GasFeeCap())
					switch mutation {
					case "nonce":
						nonce++
					case "fee":
						fee.Add(fee, big.NewInt(1))
					case "gas":
						gas++
					case "expiry":
						changed.ValidUntil++
					case "beneficiary":
						changed.BeneficiaryPublicKey[0] ^= 1
					case "stamp":
						changed.Stamp.Commitment[0] ^= 1
					}
					data, err := core.EncodeAntarticalStamp(changed)
					if err != nil {
						t.Fatal(err)
					}
					bad := sign(types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), Nonce: nonce, To: &params.ShieldedPoolAddress, Gas: gas, GasFeeCap: fee, GasTipCap: signed.GasTipCap(), Value: new(big.Int), Data: data}), seedA)
					if err := core.ProcessAntarticalStamp(params.MainnetChainConfig, head.Number, head.Time, st, bad); err == nil {
						t.Fatal("modified authorization accepted")
					}
					if core.IsAntarticalStamped(st, b.Address) {
						t.Fatal("invalid sponsorship wrote beneficiary stamp")
					}
				})
			}
			poolOpts := &txpool.ValidationOptionsWithState{Antartical: true, State: st, ExistingExpenditure: func(common.Address) *big.Int { return new(big.Int) }, ExistingCost: func(common.Address, uint64) *big.Int { return nil }}
			if err := txpool.ValidateTransactionWithState(signed, types.NewQuantumSigner(big.NewInt(8979)), poolOpts); err != nil {
				t.Fatal("stateful pool rejected stamped sponsor", err)
			}
			before := new(uint256.Int).Set(st.GetBalance(a.Address))
			sponsorNonce := st.GetNonce(a.Address)
			process(signed)
			if st.GetBalance(a.Address).Cmp(before) >= 0 || st.GetNonce(a.Address) != sponsorNonce+1 {
				t.Fatal("sponsor did not pay gas and advance its nonce")
			}
			if st.GetBalance(b.Address).Sign() != 0 || st.GetNonce(b.Address) != 0 {
				t.Fatal("sponsorship changed beneficiary balance or nonce")
			}
			if err := txpool.ValidateTransactionWithState(signed, types.NewQuantumSigner(big.NewInt(8979)), poolOpts); err == nil {
				t.Fatal("confirmed sponsorship re-entered pool")
			}
			if err := core.ProcessAntarticalStamp(params.MainnetChainConfig, head.Number, head.Time, st, signed); err != nil {
				t.Fatal("same processing stage is not idempotent", err)
			}
		} else {
			process(signed)
		}
		status, err := core.AntarticalStampForAddress(st, entry.identity.Address)
		if err != nil || !status.Registered || status.Owner != entry.identity.Owner || status.Commitment != entry.identity.Stamp.Commitment {
			t.Fatal("missing consensus stamp", err)
		}
		if _, err := BuildStamp(ctx, rpc, entry.seed, entry.identity); err == nil {
			t.Fatal("wallet allowed replacing a stamp")
		}
	}
	st.SubBalance(b.Address, st.GetBalance(b.Address), tracing.BalanceChangeUnspecified)
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
	defer viewA.Clear()
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
	defer viewB.Clear()
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
	other, err := Scan(ctx, rpc, wrongView)
	if err != nil || other.BalanceWei != "0" {
		t.Fatal("credited notes to a different owner")
	}
	// Four distinct notes require all four inputs for this amount plus relay gas.
	for n := 0; n < 3; n++ {
		unsigned, err := Build(ctx, rpc, seedA, a, pb, big.NewInt(1_000_000_000_000_000_000), false)
		if err != nil {
			t.Fatal(err)
		}
		process(sign(unsigned, seedA))
	}
	offer, err := BuildRelayOffer(ctx, rpc, seedA, a)
	if err != nil {
		t.Fatal(err)
	}
	badOffer := offer
	badOffer.ValidUntil--
	if _, err = ReviewRelayOffer(ctx, rpc, &badOffer, 8979); err == nil {
		t.Fatal("accepted modified signed quote")
	}
	relayAmount := big.NewInt(6_000_000_000_000_000_000)
	batchAmount := new(big.Int).Div(new(big.Int).Set(relayAmount), big.NewInt(3))
	relayUnsigned, err := BuildRelayedBatch(ctx, rpc, seedB, b, []Payment{{pa, batchAmount}, {pb, batchAmount}, {pa, batchAmount}}, &offer)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := RelayPacketForTransaction(relayUnsigned)
	if err != nil || packet.InputCount != 4 {
		t.Fatal("did not combine four notes", packet.InputCount, err)
	}
	if dir := os.Getenv("TKM_SHIELD3_RELAY_TESTDATA"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "draft.bin"), packet.Transaction, 0600); err != nil {
			t.Fatal(err)
		}
	}
	payerKey, _ := pqcrypto.NewMLDSA87FromSeed(seedB)
	if bytes.Contains(packet.Transaction, pqcrypto.PublicKeyBytes(payerKey)) {
		t.Fatal("relay packet exposes payer PQ key")
	}
	if _, err = BuildRelaySubmission(ctx, rpc, seedB, b, packet.Transaction); err == nil {
		t.Fatal("wrong operator accepted packet")
	}
	reviewed, err := BuildRelaySubmission(ctx, rpc, seedA, a, packet.Transaction)
	if err != nil {
		t.Fatal(err)
	}
	relayTx := sign(reviewed, seedA)
	sender, err := types.Sender(types.NewQuantumSigner(big.NewInt(8979)), relayTx)
	if err != nil || sender != a.Address || sender == b.Address {
		t.Fatal("outer payer identity not hidden", err)
	}
	e, _, _ := core.DecodeShieldedV3Transaction(relayTx.Data())
	ns, err := core.ShieldedV3Nullifiers(e)
	if err != nil || len(ns) != 4 {
		t.Fatal(err)
	}
	// A spent secondary input must reject without consuming any other input.
	snapshot := st.Snapshot()
	slot := core.ShieldedV3StateSlot("nullifier", ns[3].Bytes())
	st.SetState(params.ShieldedPoolAddress, slot, common.HexToHash("0xdead"))
	beforeRoot, _ := core.ShieldedV3Root(st)
	if err := core.ProcessShieldedTransaction(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, relayTx, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("accepted spent secondary input")
	}
	afterRoot, _ := core.ShieldedV3Root(st)
	if afterRoot != beforeRoot {
		t.Fatal("failed aggregate changed root")
	}
	for _, n := range ns[:3] {
		if core.ShieldedV3NullifierTransaction(st, n) != (common.Hash{}) {
			t.Fatal("failed aggregate consumed another input")
		}
	}
	st.RevertToSnapshot(snapshot)
	payerNonce := st.GetNonce(b.Address)
	process(relayTx)
	if st.GetNonce(b.Address) != payerNonce {
		t.Fatal("relay advanced payer's public nonce")
	}
	for _, n := range ns {
		if core.ShieldedV3NullifierTransaction(st, n) != relayTx.Hash() {
			t.Fatal("input did not record canonical relay hash")
		}
	}
	disclosure, err := ExportPaymentDisclosure(ctx, rpc, b, relayTx.Hash(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(disclosure.RecordKey)
	verified, err := VerifyPaymentDisclosure(ctx, rpc, disclosure)
	if err != nil || verified.Recipient != a.Address || verified.AmountWei != batchAmount.String() {
		t.Fatal("selective payment verification", err)
	}
	wrongDisclosure := disclosure
	wrongDisclosure.OutputIndex = 1
	if _, err = VerifyPaymentDisclosure(ctx, rpc, wrongDisclosure); err == nil {
		t.Fatal("single record key opened different output")
	}
	if _, err = ExportPaymentDisclosure(ctx, rpc, b, relayTx.Hash(), 3); err == nil {
		t.Fatal("disclosed change")
	}
	auditSeed, auditPub, err := pqcrypto.GenerateShieldedV3ViewKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(auditSeed)
	capsule, err := SealPaymentDisclosure(disclosure, auditPub)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenPaymentDisclosure(capsule, auditSeed)
	if err != nil || !bytes.Equal(opened.RecordKey, disclosure.RecordKey) {
		t.Fatal("PQ disclosure capsule", err)
	}
	clear(opened.RecordKey)
	if _, err = OpenPaymentDisclosure(capsule, a.IncomingSeed); err == nil {
		t.Fatal("unrelated viewing key opened disclosure")
	}
	incoming, err := a.ScopedViewKey("incoming")
	if err != nil {
		t.Fatal(err)
	}
	defer incoming.Clear()
	receiveRPC := &receiptOnlyRPC{RPC: rpc}
	receiveScan, err := Scan(ctx, receiveRPC, incoming)
	if err != nil || receiveScan.SpendStatusKnown || receiveScan.BalanceWei != "" || len(receiveScan.Notes) != 0 || receiveRPC.spendCalls != 0 {
		t.Fatal("receive-only scan exposed spend status", err)
	}
	fullView := a.ViewKey()
	defer fullView.Clear()
	fullScan, err := Scan(ctx, rpc, fullView)
	if err != nil || !fullScan.SpendStatusKnown {
		t.Fatal("full scan cannot track spends", err)
	}
	for _, received := range fullScan.Notes {
		guessed, err := NoteNullifier(8979, received.Note, b.NullifierKey)
		if err != nil || guessed == received.Nullifier {
			t.Fatal("sender could derive recipient nullifier", err)
		}
	}
	if _, err := ExportPaymentDisclosure(ctx, rpc, b, relayTx.Hash(), 1); err != nil {
		t.Fatal("could not disclose second batch recipient", err)
	}
	rpc.reorg = true
	if _, err = VerifyPaymentDisclosure(ctx, rpc, disclosure); err == nil {
		t.Fatal("accepted orphaned payment receipt")
	}
	rpc.reorg = false
}

func TestCarrotInspiredOutputKeyAndOutgoingViewScope(t *testing.T) {
	owner := shielded3.Digest{1}
	randomness := shielded3.Digest{2}
	commitment := shielded3.Digest{3}
	first := OneTimeOutputKey(owner, randomness, commitment)
	second := OneTimeOutputKey(owner, randomness, commitment)
	if len(first) != 32 || !bytes.Equal(first, second) {
		t.Fatalf("one-time output key is not deterministic and fixed-size")
	}
	if bytes.Equal(first, OneTimeOutputKey(owner, shielded3.Digest{4}, commitment)) {
		t.Fatalf("one-time output key reused across randomness")
	}
}
