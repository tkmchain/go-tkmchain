package core

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/internal/shield6test"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
	"github.com/holiman/uint256"
)

// TestShield6RecipientEndToEnd uses real parameters/proofs and the canonical
// block processor. Setup is expensive; release CI explicitly enables it on an
// isolated runner. The optional, separately named test-only setup cache avoids
// repeating setup after an unrelated failure. The TCP handoff contains only
// public state, verification keys and signed transactions.
func TestShield6RecipientEndToEnd(t *testing.T) {
	const chainID = shield6test.ChainID
	if testing.Short() || os.Getenv("SHIELD6_RECIPIENT_INTEGRATION") != "1" {
		t.Skip("set SHIELD6_RECIPIENT_INTEGRATION=1 for real recipient consensus integration")
	}
	if !shielded3.NativeAvailable() {
		t.Fatal("native Tip5 reference required")
	}
	started := time.Now()
	prepared, encodedKey, pins := shield6RecipientTestParameters(t, false)
	regPrepared, regEncoded, regPins := shield6RecipientTestParameters(t, true)
	zero, activation := uint64(0), uint64(10)
	cfg := *params.TestChainConfig
	cfg.ChainID = big.NewInt(chainID)
	cfg.AntarticalTime, cfg.PrivacyCommitmentTime, cfg.QuantumResistantTime = &zero, &zero, &zero
	cfg.Shield6Time, cfg.ConfidentialLedgerTime = &zero, &zero
	cfg.Shield6RecipientTime = &activation
	cfg.Shield6RecipientKey = encodedKey
	cfg.Shield6RecipientKeyHash = common.Hash(pins.VerifyingKey)
	cfg.Shield6RegistrationKey = regEncoded
	cfg.Shield6RegistrationKeyHash = common.Hash(regPins.VerifyingKey)
	cfg.Shield6RecipientProvingKeyHash = common.Hash(pins.ProvingKey)
	cfg.Shield6RegistrationProvingKeyHash = common.Hash(regPins.ProvingKey)
	cfg.CancunTime = nil
	if err := cfg.CheckConfigForkOrder(); err != nil {
		t.Fatal(err)
	}
	chain := shield6TestChain{config: &cfg, engine: randomx.NewFaker()}
	st := newShield6RecipientNetworkState(t)
	seeds := [][]byte{bytes.Repeat([]byte{0x76}, pqcrypto.MLDSA87SeedSize), bytes.Repeat([]byte{0x77}, pqcrypto.MLDSA87SeedSize)}
	owners := []shielded3.Digest{{1, 2, 3, 4, 5}, {6, 7, 8, 9, 10}}
	wallets := make([]*shielded6.WalletKeys, 2)
	publicKeys := make([][]byte, 2)
	addresses := make([]common.Address, 2)
	for i, seed := range seeds {
		key, err := pqcrypto.NewMLDSA87FromSeed(seed)
		if err != nil {
			t.Fatal(err)
		}
		publicKeys[i] = pqcrypto.PublicKeyBytes(key)
		addresses[i], err = pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKeys[i])
		if err != nil {
			t.Fatal(err)
		}
		wallets[i], err = shielded6.DeriveWalletKeys(seed, chainID)
		if err != nil {
			t.Fatal(err)
		}
		defer wallets[i].Clear()
		st.AddBalance(addresses[i], uint256.MustFromBig(new(big.Int).Exp(big.NewInt(10), big.NewInt(25), nil)), tracing.BalanceChangeUnspecified)
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", addresses[i].Bytes()), common.Hash{byte(i + 1)})
		v3WriteDigest(st, "stamp/address-owner", addresses[i].Bytes(), owners[i])
		if err := appendAntarticalStamp(st, chainID, owners[i], common.Hash{byte(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	sign := func(index int, nonce uint64, value *big.Int, data []byte) *types.Transaction {
		t.Helper()
		key, err := pqcrypto.NewMLDSA87FromSeed(seeds[index])
		if err != nil {
			t.Fatal(err)
		}
		tx, err := types.SignPQTkmTx(shield6RehearsalTx(chainID, nonce, value, data, nil, publicKeys[index]), types.NewQuantumSigner(big.NewInt(chainID)), key)
		if err != nil {
			t.Fatal(err)
		}
		return tx
	}
	apply := func(target *state.StateDB, tx *types.Transaction, number, stamp uint64) error {
		block := types.NewBlockWithHeader(&types.Header{Number: new(big.Int).SetUint64(number), Time: stamp, GasLimit: 40_000_000, Difficulty: big.NewInt(1), BaseFee: new(big.Int), Coinbase: common.Address{0xca, 0xfe}}).WithBody(types.Body{Transactions: types.Transactions{tx}})
		_, err := NewStateProcessor(chain).Process(context.Background(), block, target, vm.Config{})
		return err
	}
	// Establish eligible historical decoys before the recipient-proof fork.
	legacyOutputs := make([]ShieldedV6WalletOutput, 16)
	for i := range legacyOutputs {
		legacyOutputs[i] = ShieldedV6WalletOutput{Address: wallets[1].Address(), Amount: big.NewInt(1)}
	}
	legacySkeleton := shield6RehearsalTx(chainID, 0, big.NewInt(16), nil, nil, publicKeys[0])
	legacyData, err := BuildShieldedV6DepositEnvelope(legacySkeleton, legacyOutputs, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(st, sign(0, 0, big.NewInt(16), legacyData), 1, 0); err != nil {
		t.Fatal(err)
	}
	makeCode := func(index int) recipient.PaymentCode {
		t.Helper()
		a, err := AntarticalStampPath(st, owners[index])
		if err != nil {
			t.Fatal(err)
		}
		b, err := ShieldedV6BindingPath(st, owners[index])
		if err != nil {
			t.Fatal(err)
		}
		p := recipient.PaymentCode{Version: recipient.RelationVersion, Address: wallets[index].Address(), Owner: owners[index], StampRoot: a.Root, StampIndex: uint32(a.Index), StampPath: a.Path, BindingRoot: b.Root, BindingIndex: uint32(b.Index), BindingPath: b.Path}
		if err := p.Validate(chainID); err != nil {
			t.Fatal(err)
		}
		return p
	}
	codes := make([]recipient.PaymentCode, 2)
	for i := range wallets {
		commitment, err := recipient.BindingCommitment(chainID, owners[i], wallets[i].Address())
		if err != nil {
			t.Fatal(err)
		}
		envelope := &ShieldedV6Transaction{Version: ShieldedV6BindingVersion, ValidUntil: 3610, AssetID: 1, Binding: &ShieldedV6BindingRegistration{Owner: owners[i], Commitment: commitment}}
		nonce := uint64(0)
		if i == 0 {
			nonce = 1
		}
		skeleton := shield6RehearsalTx(chainID, nonce, new(big.Int), nil, nil, publicKeys[i])
		statement, err := ShieldedV6RegistrationStatement(skeleton, envelope)
		if err != nil {
			t.Fatal(err)
		}
		private := recipient.RegistrationWitness{SpendSecret: wallets[i].SpendSecret, ViewSecret: wallets[i].ViewSecret}
		envelope.Binding.Proof, err = regPrepared.ProveRegistration(context.Background(), statement, private)
		private.Clear()
		if err != nil {
			t.Fatal(err)
		}
		data, err := EncodeShieldedV6Transaction(envelope)
		if err != nil {
			t.Fatal(err)
		}

		if err := apply(st, sign(i, nonce, new(big.Int), data), uint64(i+2), 10); err != nil {
			t.Fatal(err)
		}
		codes[i] = makeCode(i)
	}
	bound := func(index int) *ShieldedV6WalletRecipient {
		return &ShieldedV6WalletRecipient{Code: codes[index], Prover: prepared, Context: context.Background(), ValidUntil: 3610}
	}
	depositSkeleton := shield6RehearsalTx(chainID, 2, big.NewInt(7), nil, nil, publicKeys[0])
	proofStart := time.Now()
	depositData, err := BuildShieldedV6DepositEnvelope(depositSkeleton, []ShieldedV6WalletOutput{{Address: wallets[0].Address(), Amount: big.NewInt(7), Recipient: bound(0)}}, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("proof-bearing deposit constructed in %s", time.Since(proofStart))
	deposit := sign(0, 2, big.NewInt(7), depositData)
	if err := apply(st, deposit, 4, 10); err != nil {
		t.Fatal(err)
	}
	e, _, err := DecodeShieldedV6Transaction(depositData)
	if err != nil {
		t.Fatal(err)
	}
	note, owned, err := shielded6.ScanOutput(wallets[0], 0, 16, ShieldedV6WalletRecord(e.Outputs[0]))
	if err != nil || !owned {
		t.Fatalf("receiver scan: %v", err)
	}
	defer note.Clear()
	input := ShieldedV6WalletInput{Note: note, SignerIndex: 15}
	for i := 0; i < 16; i++ {
		index := uint64(i)
		if i == 15 {
			index = 16
		}
		input.RingIndices[i] = index
		input.Ring[i], err = ShieldedV6OutputAt(st, index)
		if err != nil {
			t.Fatal(err)
		}
	}
	paymentSkeleton := shield6RehearsalTx(chainID, 3, new(big.Int), nil, nil, publicKeys[0])
	proofStart = time.Now()
	paymentData, err := BuildShieldedV6Envelope(paymentSkeleton, []ShieldedV6WalletInput{input}, []ShieldedV6WalletOutput{{Address: wallets[1].Address(), Amount: big.NewInt(4), Recipient: bound(1)}, {Address: wallets[0].Address(), Amount: big.NewInt(3), Recipient: bound(0)}}, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("payment plus change constructed in %s", time.Since(proofStart))
	payment := sign(0, 3, new(big.Int), paymentData)
	e, _, err = DecodeShieldedV6Transaction(paymentData)
	if err != nil {
		t.Fatal(err)
	}
	networkState := st.Copy()
	var networkAttacks []hexutil.Bytes
	for _, name := range []string{"missing", "ciphertext", "nonce", "root"} {
		bad, _, err := DecodeShieldedV6Transaction(paymentData)
		if err != nil {
			t.Fatal(err)
		}
		nonce := uint64(3)
		switch name {
		case "missing":
			bad.Recipients.Proofs = bad.Recipients.Proofs[:1]
		case "ciphertext":
			bad.Outputs[0].EncryptedNote[0] ^= 1
		case "nonce":
			nonce++
		case "root":
			bad.Recipients.Roots[0].BindingRoot[0]++
		}
		data, err := EncodeShieldedV6Transaction(bad)
		if err != nil {
			t.Fatal(err)
		}
		attack := sign(0, nonce, new(big.Int), data)
		raw, err := attack.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		networkAttacks = append(networkAttacks, raw)
		copyState := st.Copy()
		if err := apply(copyState, attack, 5, 10); err == nil {
			t.Fatalf("accepted %s attack", name)
		}
		if ShieldedV6OutputCount(copyState) != ShieldedV6OutputCount(st) || ShieldedV6KeyImageCount(copyState) != 0 {
			t.Fatal("invalid proof changed state")
		}
	}
	nodeB := st.Copy()
	rollback := st.Copy()
	for _, target := range []*state.StateDB{st, nodeB} {
		if err := apply(target, payment, 5, 10); err != nil {
			t.Fatal(err)
		}
	}
	if ShieldedV6OutputRoot(st) != ShieldedV6OutputRoot(nodeB) || ShieldedV6KeyImageCount(st) != 1 {
		t.Fatal("replicas disagreed")
	}
	if err := ValidateShieldedV6State(st, payment); err == nil {
		t.Fatal("key-image replay accepted")
	}
	if err := apply(rollback, payment, 5, 10); err != nil {
		t.Fatalf("canonical rollback/replay: %v", err)
	}
	for i, index := range []int{1, 0} {
		decoded, owned, err := shielded6.ScanOutput(wallets[index], uint64(i), uint64(17+i), ShieldedV6WalletRecord(e.Outputs[i]))
		if err != nil || !owned {
			t.Fatalf("payment receiver %d: %v", i, err)
		}
		want := int64(4)
		if i == 1 {
			want = 3
		}
		if decoded.Amount.Cmp(big.NewInt(want)) != 0 {
			t.Fatal("payment amount mismatch")
		}
		decoded.Clear()
	}
	if path := os.Getenv(shield6test.FixtureEnvironment); path != "" {
		raw, err := payment.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		fixture := shield6test.Fixture{Config: &cfg, Timestamp: 10, Alloc: shield6RecipientNetworkAlloc(t, networkState), Payment: raw, Attacks: networkAttacks, Sender: addresses[0], OutputCount: ShieldedV6OutputCount(networkState), SenderNonce: networkState.GetNonce(addresses[0])}
		if err := shield6test.Write(path, fixture); err != nil {
			t.Fatal(err)
		}
		t.Log("exported public, ephemeral fixture for the proof-bearing TCP rehearsal")
	}
	t.Logf("real proof registration, canonical deposit, payment, change, rejection and replay passed in %s", time.Since(started))
}
