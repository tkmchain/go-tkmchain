package core

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"testing"

	monerocrypto "git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/holiman/uint256"
)

// TestShieldedV6DepositUsesCanonicalBlockProcessor protects the production
// execution hook: validating a Shield6 envelope through the helper alone is
// insufficient if normal block processing forgets to invoke it.
func TestShieldedV6DepositUsesCanonicalBlockProcessor(t *testing.T) {
	const chainID = uint64(8979)
	activation := uint64(0)
	config := *params.MainnetChainConfig
	config.AntarticalTime = &activation
	config.PrivacyCommitmentTime = &activation
	config.QuantumResistantTime = &activation
	config.ConfidentialLedgerTime = &activation
	config.Shield6Time = &activation
	config.CancunTime = nil // Keep this minimal execution fixture independent of later fork ordering.

	seed := bytes.Repeat([]byte{0x71}, pqcrypto.MLDSA87SeedSize)
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	accountAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(accountKey))
	if err != nil {
		t.Fatal(err)
	}
	wallet, _, err := shielded6.CreateWallet(bytes.NewReader(bytes.Repeat([]byte{0x41}, 32)), chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer wallet.Clear()
	st := newShield6RehearsalState(t)
	st.AddBalance(accountAddress, uint256.MustFromBig(new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil)), tracing.BalanceChangeUnspecified)
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", accountAddress.Bytes()), common.Hash{0x71})
	chain := shield6TestChain{config: &config, engine: randomx.NewFaker()}

	amount := big.NewInt(7)
	publicKey := pqcrypto.PublicKeyBytes(accountKey)
	unsigned := shield6RehearsalTx(chainID, 0, amount, nil, nil, publicKey)
	depositData, err := BuildShieldedV6DepositEnvelope(unsigned, []ShieldedV6WalletOutput{{Address: wallet.Address(), Amount: amount}}, bytes.NewReader(bytes.Repeat([]byte{0x42}, 4096)))
	if err != nil {
		t.Fatalf("build Shield6 deposit: %v", err)
	}
	badEnvelope, ok, err := DecodeShieldedV6Transaction(depositData)
	if err != nil || !ok || len(badEnvelope.RangeProof) == 0 {
		t.Fatalf("decode Shield6 deposit for tamper case: ok=%t err=%v", ok, err)
	}
	badEnvelope.RangeProof[len(badEnvelope.RangeProof)-1] ^= 1
	badData, err := EncodeShieldedV6Transaction(badEnvelope)
	if err != nil {
		t.Fatalf("encode tampered Shield6 deposit: %v", err)
	}
	badUnsigned := shield6RehearsalTx(chainID, 0, amount, badData, nil, publicKey)
	badTx, err := types.SignPQTkmTx(badUnsigned, types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
	if err != nil {
		t.Fatalf("sign tampered Shield6 deposit: %v", err)
	}
	badState := st.Copy()
	badBlock := types.NewBlockWithHeader(&types.Header{
		Number: big.NewInt(1), Time: 0, GasLimit: 30_000_000,
		Difficulty: big.NewInt(1), BaseFee: new(big.Int), Coinbase: common.Address{0xca, 0xfe},
	}).WithBody(types.Body{Transactions: types.Transactions{badTx}})
	badResult, badErr := NewStateProcessor(chain).Process(context.Background(), badBlock, badState, vm.Config{})
	if badErr == nil {
		t.Fatalf("canonical block processor accepted a tampered Shield6 range proof: result=%+v", badResult)
	}
	if ShieldedV6OutputCount(badState) != 0 || badState.GetNonce(accountAddress) != 0 {
		t.Fatal("rejected Shield6 proof changed consensus state")
	}

	unsigned = shield6RehearsalTx(chainID, 0, amount, depositData, nil, publicKey)
	tx, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
	if err != nil {
		t.Fatalf("sign Shield6 deposit: %v", err)
	}
	unstampedState := st.Copy()
	unstampedState.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", accountAddress.Bytes()), common.Hash{})
	unstampedBlock := types.NewBlockWithHeader(&types.Header{
		Number: big.NewInt(1), Time: 0, GasLimit: 30_000_000,
		Difficulty: big.NewInt(1), BaseFee: new(big.Int), Coinbase: common.Address{0xca, 0xfe},
	}).WithBody(types.Body{Transactions: types.Transactions{tx}})
	if _, err := NewStateProcessor(chain).Process(context.Background(), unstampedBlock, unstampedState, vm.Config{}); err == nil {
		t.Fatal("canonical block processor accepted a Shield6 deposit from an unstamped account")
	}
	if ShieldedV6OutputCount(unstampedState) != 0 || unstampedState.GetNonce(accountAddress) != 0 {
		t.Fatal("rejected Shield6 deposit from an unstamped account changed consensus state")
	}
	if deposited := ConfidentialLedgerState(unstampedState, shielded3.AssetTKM).Deposited; deposited.Sign() != 0 {
		t.Fatal("rejected Shield6 deposit from an unstamped account changed confidential backing")
	}
	header := &types.Header{
		Number: big.NewInt(1), Time: 0, GasLimit: 30_000_000,
		Difficulty: big.NewInt(1), BaseFee: new(big.Int), Coinbase: common.Address{0xca, 0xfe},
	}
	block := types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: types.Transactions{tx}})
	result, err := NewStateProcessor(chain).Process(context.Background(), block, st, vm.Config{})
	if err != nil {
		t.Fatalf("canonical block processor rejected Shield6 deposit: %v", err)
	}
	if len(result.Receipts) != 1 || result.Receipts[0].Status != types.ReceiptStatusSuccessful {
		t.Fatalf("canonical Shield6 receipt = %#v", result.Receipts)
	}
	if got := ShieldedV6OutputCount(st); got != 1 {
		t.Fatalf("canonical block processor appended %d Shield6 outputs, want 1", got)
	}
	ledger := ConfidentialLedgerState(st, shielded3.AssetTKM)
	if ledger.Deposited.Cmp(amount) != 0 || ledger.Backing.Cmp(amount) != 0 {
		t.Fatalf("canonical Shield6 deposit backing = deposited %s, backing %s; want %s each", ledger.Deposited, ledger.Backing, amount)
	}
	if got := st.GetBalance(params.ShieldedPoolAddress).ToBig(); got.Cmp(amount) != 0 {
		t.Fatalf("canonical Shield6 deposit pool balance = %s, want %s", got, amount)
	}
	if got := st.GetNonce(accountAddress); got != 1 {
		t.Fatalf("canonical block processor sender nonce = %d, want 1", got)
	}
}

// TestShieldedV6PrivateSpendUsesCanonicalBlockProcessor exercises the complete
// wallet-built CLSAG spend through normal block execution, replays it after a
// state rollback, and confirms a second canonical inclusion is rejected.
func TestShieldedV6PrivateSpendUsesCanonicalBlockProcessor(t *testing.T) {
	const chainID = uint64(8979)
	activation := uint64(0)
	config := *params.MainnetChainConfig
	config.AntarticalTime = &activation
	config.PrivacyCommitmentTime = &activation
	config.QuantumResistantTime = &activation
	config.ConfidentialLedgerTime = &activation
	config.Shield6Time = &activation
	config.CancunTime = nil
	chain := shield6TestChain{config: &config, engine: randomx.NewFaker()}

	accountSeed := bytes.Repeat([]byte{0x51}, pqcrypto.MLDSA87SeedSize)
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		t.Fatal(err)
	}
	accountAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(accountKey))
	if err != nil {
		t.Fatal(err)
	}
	wallet, _, err := shielded6.CreateWallet(bytes.NewReader(bytes.Repeat([]byte{0x52}, 32)), chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer wallet.Clear()
	decoyWallet, _, err := shielded6.CreateWallet(bytes.NewReader(bytes.Repeat([]byte{0x53}, 32)), chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer decoyWallet.Clear()
	st := newShield6RehearsalState(t)
	st.AddBalance(accountAddress, uint256.MustFromBig(new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil)), tracing.BalanceChangeUnspecified)
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", accountAddress.Bytes()), common.Hash{0x51})
	publicKey := pqcrypto.PublicKeyBytes(accountKey)
	rng := monerocrypto.NewDeterministicTestGenerator()

	// Fund a 16-member canonical ring with one wallet-owned output and 15
	// independently encrypted decoys, all represented in the real accumulator.
	depositOutputs := make([]ShieldedV6WalletOutput, shielded6.RingSize)
	depositOutputs[0] = ShieldedV6WalletOutput{Address: wallet.Address(), Amount: big.NewInt(5)}
	depositTotal := big.NewInt(5)
	for i := 1; i < len(depositOutputs); i++ {
		depositOutputs[i] = ShieldedV6WalletOutput{Address: decoyWallet.Address(), Amount: big.NewInt(1)}
		depositTotal.Add(depositTotal, big.NewInt(1))
	}
	unsignedDeposit := shield6RehearsalTx(chainID, 0, depositTotal, nil, nil, publicKey)
	depositData, err := BuildShieldedV6DepositEnvelope(unsignedDeposit, depositOutputs, rng)
	if err != nil {
		t.Fatalf("build Shield6 ring-funding deposit: %v", err)
	}
	unsignedDeposit = shield6RehearsalTx(chainID, 0, depositTotal, depositData, nil, publicKey)
	depositTx, err := types.SignPQTkmTx(unsignedDeposit, types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
	if err != nil {
		t.Fatalf("sign Shield6 ring-funding deposit: %v", err)
	}
	processBlockTx := func(st *state.StateDB, number uint64, parent common.Hash, tx *types.Transaction) (common.Hash, error) {
		header := &types.Header{
			ParentHash: parent, Number: new(big.Int).SetUint64(number), Time: 0,
			GasLimit: 30_000_000, Difficulty: big.NewInt(1), BaseFee: new(big.Int),
			Coinbase: common.Address{0xca, 0xfe},
		}
		block := types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: types.Transactions{tx}})
		result, err := NewStateProcessor(chain).Process(context.Background(), block, st, vm.Config{})
		if err == nil && (len(result.Receipts) != 1 || result.Receipts[0].Status != types.ReceiptStatusSuccessful) {
			err = fmt.Errorf("unexpected canonical receipt: %#v", result.Receipts)
		}
		return block.Hash(), err
	}
	block1Hash, err := processBlockTx(st, 1, common.Hash{}, depositTx)
	if err != nil {
		t.Fatalf("canonical processor rejected ring-funding deposit: %v", err)
	}
	if ShieldedV6OutputCount(st) != shielded6.RingSize {
		t.Fatalf("ring-funding deposit appended %d outputs, want %d", ShieldedV6OutputCount(st), shielded6.RingSize)
	}

	depositEnvelope, recognized, err := DecodeShieldedV6Transaction(depositTx.Data())
	if err != nil || !recognized || depositEnvelope == nil || len(depositEnvelope.Outputs) != shielded6.RingSize {
		t.Fatalf("decode ring-funding deposit: recognized=%t err=%v", recognized, err)
	}
	note, found, err := shielded6.ScanOutput(wallet, 0, 0, ShieldedV6WalletRecord(depositEnvelope.Outputs[0]))
	if err != nil || !found || note == nil || note.Amount.Cmp(big.NewInt(5)) != 0 {
		t.Fatalf("scan Shield6 funding note: found=%t note=%v err=%v", found, note, err)
	}
	defer note.Clear()
	var ring [shielded6.RingSize]shielded6.RingMember
	var input ShieldedV6WalletInput
	input.Note = note
	input.SignerIndex = 0
	for i := range ring {
		member, err := ShieldedV6OutputAt(st, uint64(i))
		if err != nil {
			t.Fatalf("read canonical Shield6 ring member %d: %v", i, err)
		}
		ring[i] = member
		input.RingIndices[i] = uint64(i)
	}
	input.Ring = ring
	privateSkeleton := shield6RehearsalTx(chainID, 1, new(big.Int), nil, nil, publicKey)
	privateData, err := BuildShieldedV6Envelope(privateSkeleton, []ShieldedV6WalletInput{input}, []ShieldedV6WalletOutput{
		{Address: decoyWallet.Address(), Amount: big.NewInt(4)},
		{Address: wallet.Address(), Amount: big.NewInt(1)},
	}, rng)
	if err != nil {
		t.Fatalf("build wallet Shield6 CLSAG payment: %v", err)
	}
	privateTx, err := types.SignPQTkmTx(shield6RehearsalTx(chainID, 1, new(big.Int), privateData, nil, publicKey), types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
	if err != nil {
		t.Fatalf("sign wallet Shield6 CLSAG payment: %v", err)
	}
	preSpend := st.Copy()
	block2Hash, err := processBlockTx(st, 2, block1Hash, privateTx)
	if err != nil {
		t.Fatalf("canonical processor rejected wallet-built Shield6 spend: %v", err)
	}
	if got := ShieldedV6KeyImageCount(st); got != 1 {
		t.Fatalf("canonical Shield6 spend recorded %d key images, want 1", got)
	}
	postSpendRoot := ShieldedV6OutputRoot(st)
	if postSpendRoot == ShieldedV6OutputRoot(preSpend) {
		t.Fatal("canonical Shield6 spend did not append its output accumulator leaves")
	}

	// Reorg to the state before block 2 and replay the exact block. Consensus
	// state must return to the same root, then reject a duplicate inclusion.
	st = preSpend
	replayedBlockHash, err := processBlockTx(st, 2, block1Hash, privateTx)
	if err != nil {
		t.Fatalf("Shield6 spend failed after rollback and canonical replay: %v", err)
	}
	if replayedBlockHash != block2Hash || ShieldedV6OutputRoot(st) != postSpendRoot {
		t.Fatal("Shield6 state after canonical reorg replay is not deterministic")
	}
	if _, err := processBlockTx(st, 3, block2Hash, privateTx); err == nil {
		t.Fatal("canonical processor accepted a second inclusion of an already-spent Shield6 key image")
	}
	if got := ShieldedV6KeyImageCount(st); got != 1 {
		t.Fatalf("rejected Shield6 key-image replay changed count to %d", got)
	}

	// Exercise the canonical public-exit path end to end. The exit is a
	// privacy boundary: its recipient and value are public, and consensus
	// requires that recipient to have an immutable on-chain stamp.
	privateEnvelope, recognized, err := DecodeShieldedV6Transaction(privateTx.Data())
	if err != nil || !recognized || len(privateEnvelope.Outputs) != 2 {
		t.Fatalf("decode private spend outputs for withdrawal: recognized=%t err=%v", recognized, err)
	}
	exitNote, owned, err := shielded6.ScanOutput(decoyWallet, 0, shielded6.RingSize, ShieldedV6WalletRecord(privateEnvelope.Outputs[0]))
	if err != nil || !owned || exitNote == nil || exitNote.Amount.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("scan Shield6 withdrawal note: owned=%t note=%v err=%v", owned, exitNote, err)
	}
	defer exitNote.Clear()
	var exitInput ShieldedV6WalletInput
	exitInput.Note = exitNote
	exitInput.SignerIndex = shielded6.RingSize - 1
	for i := range shielded6.RingSize {
		index := uint64(i + 1) // indices 1..16 include the real output at index 16
		member, err := ShieldedV6OutputAt(st, index)
		if err != nil {
			t.Fatalf("read canonical withdrawal-ring member %d: %v", index, err)
		}
		exitInput.Ring[i] = member
		exitInput.RingIndices[i] = index
	}
	withdrawalAmount := big.NewInt(4)
	unstampedRecipient := common.Address{0xee}
	unstampedData, err := BuildShieldedV6WithdrawalEnvelope(
		shield6RehearsalTx(chainID, 2, new(big.Int), nil, nil, publicKey),
		[]ShieldedV6WalletInput{exitInput}, nil, unstampedRecipient, withdrawalAmount, rng,
	)
	if err != nil {
		t.Fatalf("build withdrawal to test unstamped recipient rejection: %v", err)
	}
	unstampedTx, err := types.SignPQTkmTx(
		shield6RehearsalTx(chainID, 2, new(big.Int), unstampedData, nil, publicKey),
		types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey,
	)
	if err != nil {
		t.Fatalf("sign withdrawal to unstamped recipient: %v", err)
	}
	unstampedState := st.Copy()
	if _, err := processBlockTx(unstampedState, 3, block2Hash, unstampedTx); err == nil {
		t.Fatal("canonical processor accepted a Shield6 withdrawal to an unstamped address")
	}
	if ShieldedV6KeyImageCount(unstampedState) != ShieldedV6KeyImageCount(st) {
		t.Fatal("rejected withdrawal to an unstamped recipient changed key-image state")
	}

	withdrawalData, err := BuildShieldedV6WithdrawalEnvelope(
		shield6RehearsalTx(chainID, 2, new(big.Int), nil, nil, publicKey),
		[]ShieldedV6WalletInput{exitInput}, nil, common.Address{0x44}, withdrawalAmount, rng,
	)
	if err != nil {
		t.Fatalf("build Shield6 withdrawal to stamped account: %v", err)
	}
	withdrawalTx, err := types.SignPQTkmTx(
		shield6RehearsalTx(chainID, 2, new(big.Int), withdrawalData, nil, publicKey),
		types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey,
	)
	if err != nil {
		t.Fatalf("sign Shield6 withdrawal: %v", err)
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", common.Address{0x44}.Bytes()), common.Hash{0x44})
	recipientBalanceBefore := st.GetBalance(common.Address{0x44}).ToBig()
	poolBalanceBefore := st.GetBalance(params.ShieldedPoolAddress).ToBig()
	if _, err := processBlockTx(st, 3, block2Hash, withdrawalTx); err != nil {
		t.Fatalf("canonical processor rejected withdrawal to a stamped account: %v", err)
	}
	if got := new(big.Int).Sub(st.GetBalance(common.Address{0x44}).ToBig(), recipientBalanceBefore); got.Cmp(withdrawalAmount) != 0 {
		t.Fatalf("public withdrawal credited recipient %s wei, want %s", got, withdrawalAmount)
	}
	if got := new(big.Int).Sub(poolBalanceBefore, st.GetBalance(params.ShieldedPoolAddress).ToBig()); got.Cmp(withdrawalAmount) != 0 {
		t.Fatalf("public withdrawal debited pool %s wei, want %s", got, withdrawalAmount)
	}
	ledger := ConfidentialLedgerState(st, shielded3.AssetTKM)
	expectedBacking := new(big.Int).Add(ledger.OpeningBacking, ledger.Deposited)
	expectedBacking.Sub(expectedBacking, ledger.Withdrawn)
	expectedBacking.Sub(expectedBacking, ledger.FeesBurned)
	if ledger.Withdrawn.Cmp(withdrawalAmount) != 0 || ledger.Backing.Cmp(expectedBacking) != 0 {
		t.Fatalf("withdrawal did not preserve confidential backing: %+v", ledger)
	}
}

type shield6TestChain struct {
	config *params.ChainConfig
	engine consensus.Engine
}

func (c shield6TestChain) Config() *params.ChainConfig                 { return c.config }
func (c shield6TestChain) Engine() consensus.Engine                    { return c.engine }
func (c shield6TestChain) CurrentHeader() *types.Header                { return nil }
func (c shield6TestChain) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (c shield6TestChain) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (c shield6TestChain) GetHeaderByHash(common.Hash) *types.Header   { return nil }

// TestShield6TwoNodeDepositSpendReplayAndReorgRehearsal runs the same signed
// public shielding deposits, a two-input private spend, and a public withdrawal
// against independent state databases. It checks output-accumulator and
// backing agreement, replay rejection, and fork rollback/re-import. This is a
// deterministic consensus rehearsal, not a substitute for a live P2P fork
// rehearsal.
func TestShield6TwoNodeDepositSpendReplayAndReorgRehearsal(t *testing.T) {
	const chainID = uint64(8979)
	const blockTime = uint64(0)
	activation := uint64(0)
	config := *params.MainnetChainConfig
	config.PrivacyCommitmentTime = &activation
	config.AntarticalTime = &activation
	config.QuantumResistantTime = &activation
	config.ConfidentialLedgerTime = &activation
	config.Shield6Time = &activation
	if !config.IsShield6(big.NewInt(1), blockTime) {
		t.Fatal("test configuration did not activate Shield6")
	}

	accountSeed := bytes.Repeat([]byte{0x61}, pqcrypto.MLDSA87SeedSize)
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		t.Fatal(err)
	}
	accountAddress, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(accountKey))
	if err != nil {
		t.Fatal(err)
	}
	shieldKeys, _, err := shielded6.CreateWallet(bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)), chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer shieldKeys.Clear()
	decoyKeys, _, err := shielded6.CreateWallet(bytes.NewReader(bytes.Repeat([]byte{0x32}, 32)), chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer decoyKeys.Clear()
	recipientKeys, _, err := shielded6.CreateWallet(bytes.NewReader(bytes.Repeat([]byte{0x33}, 32)), chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer recipientKeys.Clear()

	nodeA, nodeB := newShield6RehearsalState(t), newShield6RehearsalState(t)
	accountFunds := uint256.MustFromBig(new(big.Int).Exp(big.NewInt(10), big.NewInt(20), nil))
	stampTxHash := common.Hash{0x61}
	for _, st := range []*state.StateDB{nodeA, nodeB} {
		st.AddBalance(accountAddress, accountFunds, tracing.BalanceChangeUnspecified)
		st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", accountAddress.Bytes()), stampTxHash)
	}
	seenA, seenB := make(map[common.Hash]struct{}), make(map[common.Hash]struct{})
	rng := monerocrypto.NewDeterministicTestGenerator()
	var nonce uint64
	deposit := func(wallet *shielded6.WalletKeys, amounts []*big.Int) (*types.Transaction, *ShieldedV6Transaction) {
		t.Helper()
		total := new(big.Int)
		outputs := make([]ShieldedV6WalletOutput, len(amounts))
		for i, amount := range amounts {
			total.Add(total, amount)
			outputs[i] = ShieldedV6WalletOutput{Address: wallet.Address(), Amount: new(big.Int).Set(amount)}
		}
		publicKey := pqcrypto.PublicKeyBytes(accountKey)
		unsigned := shield6RehearsalTx(chainID, nonce, total, nil, nil, publicKey)
		data, err := BuildShieldedV6DepositEnvelope(unsigned, outputs, rng)
		if err != nil {
			t.Fatalf("build Shield6 deposit: %v", err)
		}
		unsigned = shield6RehearsalTx(chainID, nonce, total, data, nil, publicKey)
		tx, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
		if err != nil {
			t.Fatalf("sign Shield6 deposit: %v", err)
		}
		envelope, ok, err := DecodeShieldedV6Transaction(tx.Data())
		if err != nil || !ok || envelope == nil || !envelope.Deposit {
			t.Fatalf("decode Shield6 deposit: ok=%t err=%v", ok, err)
		}
		nonce++
		return tx, envelope
	}

	applyBoth := func(tx *types.Transaction) {
		t.Helper()
		applyShield6RehearsalTx(t, &config, nodeA, tx, seenA)
		applyShield6RehearsalTx(t, &config, nodeB, tx, seenB)
		assertShield6RehearsalNodesEqual(t, nodeA, nodeB)
	}
	depositTx, depositEnvelope := deposit(shieldKeys, []*big.Int{big.NewInt(5), big.NewInt(5)})
	applyBoth(depositTx)
	if err := ProcessShieldedTransaction(&config, big.NewInt(1), blockTime, nodeA, depositTx, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("replayed Shield6 deposit was accepted")
	}
	decoyAmounts := make([]*big.Int, shielded6.RingSize-2)
	for i := range decoyAmounts {
		decoyAmounts[i] = big.NewInt(1)
	}
	decoyDepositTx, _ := deposit(decoyKeys, decoyAmounts)
	applyBoth(decoyDepositTx)
	if got := ShieldedV6OutputCount(nodeA); got != shielded6.RingSize {
		t.Fatalf("accumulator output count = %d, want %d", got, shielded6.RingSize)
	}

	notes := make([]*shielded6.Note, 2)
	for i := range notes {
		note, found, err := shielded6.ScanOutput(shieldKeys, uint64(i), uint64(i), ShieldedV6WalletRecord(depositEnvelope.Outputs[i]))
		if err != nil || !found || note == nil || note.Amount.Cmp(big.NewInt(5)) != 0 {
			t.Fatalf("scan deposited note %d: found=%t note=%v err=%v", i, found, note, err)
		}
		notes[i] = note
		defer note.Clear()
	}
	ring := [shielded6.RingSize]shielded6.RingMember{}
	for i := range ring {
		member, err := ShieldedV6OutputAt(nodeA, uint64(i))
		if err != nil {
			t.Fatalf("read canonical rehearsal ring member %d: %v", i, err)
		}
		ring[i] = member
	}
	inputs := make([]ShieldedV6WalletInput, len(notes))
	for inputIndex, note := range notes {
		inputs[inputIndex].Note = note
		inputs[inputIndex].Ring = ring
		inputs[inputIndex].SignerIndex = uint64(inputIndex)
		for i := range ring {
			inputs[inputIndex].RingIndices[i] = uint64(i)
		}
	}
	pool := params.ShieldedPoolAddress
	publicKey := pqcrypto.PublicKeyBytes(accountKey)
	skeleton := shield6RehearsalTx(chainID, nonce, new(big.Int), nil, nil, publicKey)
	privateData, err := BuildShieldedV6Envelope(skeleton, inputs, []ShieldedV6WalletOutput{
		{Address: recipientKeys.Address(), Amount: big.NewInt(9)},
		{Address: shieldKeys.Address(), Amount: big.NewInt(1)},
	}, rng)
	if err != nil {
		t.Fatalf("build two-input Shield6 payment: %v", err)
	}
	unsigned := shield6RehearsalTx(chainID, nonce, new(big.Int), privateData, &pool, publicKey)
	privateTx, err := types.SignPQTkmTx(unsigned, types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
	if err != nil {
		t.Fatalf("sign two-input Shield6 payment: %v", err)
	}
	nodeAReorgPoint, nodeBReorgPoint := nodeA.Copy(), nodeB.Copy()
	applyBoth(privateTx)
	if err := ProcessShieldedTransaction(&config, big.NewInt(1), blockTime, nodeA, privateTx, make(map[common.Hash]struct{})); err == nil {
		t.Fatal("replayed two-input Shield6 spend was accepted")
	}
	rootAfterSpend := ShieldedV6OutputRoot(nodeA)
	nodeA, nodeB = nodeAReorgPoint, nodeBReorgPoint
	assertShield6RehearsalNodesEqual(t, nodeA, nodeB)
	if root := ShieldedV6OutputRoot(nodeA); root == rootAfterSpend {
		t.Fatal("reorg did not roll back Shield6 output accumulator changes")
	}
	seenA, seenB = make(map[common.Hash]struct{}), make(map[common.Hash]struct{})
	applyBoth(privateTx)

	// Exit one private output through the canonical public boundary on both
	// replicas. The private destination remains confidential until this exit;
	// the stamped public recipient and amount are explicit in the envelope.
	privateEnvelope, recognized, err := DecodeShieldedV6Transaction(privateTx.Data())
	if err != nil || !recognized || len(privateEnvelope.Outputs) != 2 {
		t.Fatalf("decode two-input spend outputs for withdrawal: recognized=%t err=%v", recognized, err)
	}
	exitNote, found, err := shielded6.ScanOutput(
		recipientKeys, 0, uint64(shielded6.RingSize), ShieldedV6WalletRecord(privateEnvelope.Outputs[0]),
	)
	if err != nil || !found || exitNote == nil || exitNote.Amount.Cmp(big.NewInt(9)) != 0 {
		t.Fatalf("scan Shield6 withdrawal note: found=%t note=%v err=%v", found, exitNote, err)
	}
	defer exitNote.Clear()
	var exitInput ShieldedV6WalletInput
	exitInput.Note = exitNote
	for i := range shielded6.RingSize {
		index := uint64(i + 2) // choose 16 distinct members and include real index 16
		member, err := ShieldedV6OutputAt(nodeA, index)
		if err != nil {
			t.Fatalf("read canonical withdrawal ring member %d: %v", index, err)
		}
		exitInput.Ring[i] = member
		exitInput.RingIndices[i] = index
		if index == exitNote.Index {
			exitInput.SignerIndex = uint64(i)
		}
	}
	withdrawalAmount := big.NewInt(9)
	withdraw := func(to common.Address) *types.Transaction {
		t.Helper()
		unsignedWithdrawal := shield6RehearsalTx(chainID, nonce+1, new(big.Int), nil, nil, publicKey)
		data, err := BuildShieldedV6WithdrawalEnvelope(unsignedWithdrawal, []ShieldedV6WalletInput{exitInput}, nil, to, withdrawalAmount, rng)
		if err != nil {
			t.Fatalf("build Shield6 withdrawal: %v", err)
		}
		unsignedWithdrawal = shield6RehearsalTx(chainID, nonce+1, new(big.Int), data, nil, publicKey)
		tx, err := types.SignPQTkmTx(unsignedWithdrawal, types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), accountKey)
		if err != nil {
			t.Fatalf("sign Shield6 withdrawal: %v", err)
		}
		return tx
	}
	unstampedWithdrawal := withdraw(common.Address{0xfa})
	for label, st := range map[string]*state.StateDB{"replica A": nodeA, "replica B": nodeB} {
		rejectedState := st.Copy()
		if err := ProcessShieldedTransaction(&config, big.NewInt(1), blockTime, rejectedState, unstampedWithdrawal, make(map[common.Hash]struct{})); err == nil {
			t.Fatalf("%s accepted Shield6 withdrawal to an unstamped recipient", label)
		}
		if ShieldedV6KeyImageCount(rejectedState) != ShieldedV6KeyImageCount(st) {
			t.Fatalf("%s changed state after rejecting an unstamped withdrawal", label)
		}
	}
	withdrawalTx := withdraw(accountAddress)
	applyBoth(withdrawalTx)
	if ConfidentialLedgerState(nodeA, shielded3.AssetTKM).Withdrawn.Cmp(withdrawalAmount) != 0 {
		t.Fatal("two replicas did not record the Shield6 public withdrawal in backing state")
	}
}

func newShield6RehearsalState(t *testing.T) *state.StateDB {
	t.Helper()
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func shield6RehearsalTx(chainID, nonce uint64, value *big.Int, data []byte, to *common.Address, publicKey []byte) *types.Transaction {
	if to == nil {
		pool := params.ShieldedPoolAddress
		to = &pool
	}
	return types.NewTx(&types.PQTkmTx{
		ChainID: new(big.Int).SetUint64(chainID), Nonce: nonce, To: to, Value: new(big.Int).Set(value),
		// The larger multi-output decoy deposit exercises accumulator writes;
		// reserve enough gas for Shield6's conservative worst-case state charge.
		Gas: 25_000_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: publicKey, Data: data,
	})
}

func applyShield6RehearsalTx(t *testing.T, config *params.ChainConfig, st *state.StateDB, tx *types.Transaction, seen map[common.Hash]struct{}) {
	t.Helper()
	const blockTime = uint64(0)
	blockNumber := big.NewInt(1)
	if err := ProcessShieldedTransaction(config, blockNumber, blockTime, st, tx, seen); err != nil {
		t.Fatalf("Shield6 consensus transition %s: %v", tx.Hash(), err)
	}
	header := &types.Header{Number: blockNumber, Time: blockTime, GasLimit: 30_000_000, BaseFee: new(big.Int), Difficulty: big.NewInt(1)}
	block := vm.BlockContext{
		CanTransfer: CanTransfer, Transfer: Transfer,
		GetHash: func(uint64) common.Hash { return common.Hash{} }, Coinbase: common.Address{0xca, 0xfe},
		BlockNumber: header.Number, Time: header.Time, GasLimit: header.GasLimit, BaseFee: header.BaseFee, Difficulty: header.Difficulty,
	}
	evm := vm.NewEVM(block, st, config, vm.Config{})
	defer evm.Release()
	st.SetTxContext(tx.Hash(), 0)
	receipt, err := ApplyTransaction(evm, NewGasPool(header.GasLimit), st, header, tx)
	if err != nil || receipt.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("Shield6 EVM transaction %s: receipt=%v err=%v", tx.Hash(), receipt, err)
	}
}

func assertShield6RehearsalNodesEqual(t *testing.T, a, b *state.StateDB) {
	t.Helper()
	if ShieldedV6OutputRoot(a) != ShieldedV6OutputRoot(b) || ShieldedV6OutputCount(a) != ShieldedV6OutputCount(b) {
		t.Fatal("Shield6 nodes diverged in output accumulator")
	}
	outputCount := ShieldedV6OutputCount(a)
	for index := uint64(0); index < outputCount; index++ {
		left, leftErr := shieldedV6ReadOutput(a, index)
		right, rightErr := shieldedV6ReadOutput(b, index)
		if leftErr != nil || rightErr != nil || left != right {
			t.Fatalf("Shield6 nodes diverged at accumulator output %d: %v / %v", index, leftErr, rightErr)
		}
		leftTx, leftErr := ShieldedV6OutputTransactionAt(a, index)
		rightTx, rightErr := ShieldedV6OutputTransactionAt(b, index)
		if leftErr != nil || rightErr != nil || leftTx != rightTx {
			t.Fatalf("Shield6 nodes diverged at output transaction index %d: %v / %v", index, leftErr, rightErr)
		}
	}
	imageCountA, imageCountB := ShieldedV6KeyImageCount(a), ShieldedV6KeyImageCount(b)
	if imageCountA != imageCountB {
		t.Fatalf("Shield6 nodes diverged in key-image count: %d != %d", imageCountA, imageCountB)
	}
	for index := uint64(0); index < imageCountA; index++ {
		left, leftErr := ShieldedV6KeyImageAt(a, index)
		right, rightErr := ShieldedV6KeyImageAt(b, index)
		if leftErr != nil || rightErr != nil || left != right {
			t.Fatalf("Shield6 nodes diverged at key-image index %d: %v / %v", index, leftErr, rightErr)
		}
	}
	ledgerA, ledgerB := ConfidentialLedgerState(a, 1), ConfidentialLedgerState(b, 1)
	if ledgerA.Backing.Cmp(ledgerB.Backing) != 0 || ledgerA.Deposited.Cmp(ledgerB.Deposited) != 0 || ledgerA.Withdrawn.Cmp(ledgerB.Withdrawn) != 0 {
		t.Fatal("Shield6 nodes diverged in confidential backing ledger")
	}
	if a.GetBalance(params.ShieldedPoolAddress).Cmp(b.GetBalance(params.ShieldedPoolAddress)) != 0 {
		t.Fatal("Shield6 nodes diverged in pool backing balance")
	}
}
