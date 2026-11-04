package shield3wallet

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

type shieldedV6ScanRPC struct {
	chainID          *big.Int
	head             common.Hash
	status           shieldedV6RPCStatus
	outputs          []shieldedV6RPCOutput
	rings            map[uint64]shieldedV6RingRPCOutput
	spentKeyImages   []hexutil.Bytes
	pendingKeyImages []hexutil.Bytes
	ringBatchCalls   int
	stamp            core.AntarticalStampStatus
	balance          *big.Int
	flipHead         bool
	headCall         int
}

func (f *shieldedV6ScanRPC) CallContext(_ context.Context, result any, method string, args ...any) error {
	var value any
	switch method {
	case "eth_chainId":
		value = hexutil.Big(*f.chainID)
	case "eth_getBlockByNumber":
		f.headCall++
		hash := f.head
		if f.flipHead && f.headCall > 1 {
			hash[0] ^= 1
		}
		value = map[string]any{"hash": hash, "number": hexutil.Uint64(5)}
	case "tkmprivacy_shieldedV6Status":
		value = f.status
	case "tkmprivacy_shieldedV6WalletOutputs":
		value = f.outputs
	case "tkmprivacy_shieldedV6SpentKeyImages":
		from := uint64(args[0].(hexutil.Uint64))
		limit := uint64(args[1].(hexutil.Uint64))
		if from > uint64(len(f.spentKeyImages)) {
			return errors.New("spent key-image page starts beyond test data")
		}
		end := from + limit
		if end > uint64(len(f.spentKeyImages)) {
			end = uint64(len(f.spentKeyImages))
		}
		value = f.spentKeyImages[from:end]
	case "tkmprivacy_shieldedV6PendingKeyImages":
		value = f.pendingKeyImages
	case "tkmprivacy_shieldedV6Outputs":
		index := uint64(args[0].(hexutil.Uint64))
		candidate, ok := f.rings[index]
		if !ok {
			return errors.New("missing test ring candidate")
		}
		value = []shieldedV6RingRPCOutput{candidate}
	case "tkmprivacy_shieldedV6OutputsByIndex":
		f.ringBatchCalls++
		indices, ok := args[0].([]hexutil.Uint64)
		if !ok {
			return errors.New("unexpected Shield6 ring-candidate batch type")
		}
		candidates := make([]shieldedV6RingRPCOutput, len(indices))
		for i, index := range indices {
			candidate, exists := f.rings[uint64(index)]
			if !exists {
				return errors.New("missing test ring candidate")
			}
			candidates[i] = candidate
		}
		value = candidates
	case "tkmprivacy_antarticalStamp":
		value = f.stamp
	case "eth_getBalance":
		value = hexutil.EncodeBig(f.balance)
	default:
		return errors.New("unexpected RPC method: " + method)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, result)
}

func TestBuildAndSignShieldedV6Payment(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires the production Shield3 stamp backend")
	}
	const chainID = uint64(8979)
	accountSeed := bytes.Repeat([]byte{0x42}, pqcrypto.MLDSA87SeedSize)
	stamp, err := pqcrypto.CreateShieldedV3Stamp(accountSeed, chainID, "Shield6 Test", "New Zealand")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := NewIdentity(accountSeed, chainID, stamp)
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Clear()
	keys, _, err := shielded6.CreateWallet(nil, chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()

	amount := big.NewInt(42)
	_, _, witnesses, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	record, err := shielded6.CreateOutput(keys.Address(), 0, shielded6.AssetTKM, amount, witnesses[0], cryptorand.Reader)
	clear(witnesses[0].Mask[:])
	clear(witnesses[0].LimbMasks[0][:])
	clear(witnesses[0].LimbMasks[1][:])
	if err != nil {
		t.Fatal(err)
	}
	note, owned, err := shielded6.ScanOutput(keys, 0, 0, record)
	if err != nil || !owned {
		t.Fatalf("scan the payment's input note: owned=%t err=%v", owned, err)
	}
	defer note.Clear()

	root := common.HexToHash("0x76543210")
	fake := &shieldedV6ScanRPC{
		chainID: new(big.Int).SetUint64(chainID), head: common.HexToHash("0x1234"), balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
		status: shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(new(big.Int).SetUint64(chainID)), OutputCount: shielded6.RingSize, Accumulator: root},
		stamp:  core.AntarticalStampStatus{Registered: true, Owner: identity.Owner, Commitment: stamp.Commitment},
		rings:  make(map[uint64]shieldedV6RingRPCOutput, shielded6.RingSize),
	}
	for i := uint64(0); i < shielded6.RingSize; i++ {
		var public [shielded6.PointSize]byte
		var commitment [shielded6.PointSize]byte
		if i == 0 {
			public, commitment = record.OneTimeKey, record.Commitment
		} else {
			seed := bytes.Repeat([]byte{byte(i + 1)}, 32)
			decoyKeys, _, createErr := shielded6.CreateWallet(bytes.NewReader(seed), chainID)
			if createErr != nil {
				t.Fatal(createErr)
			}
			public = decoyKeys.SpendPublic
			decoyKeys.Clear()
			commitment, err = shielded6.Commitment128(new(big.Int).SetUint64(i+100), [shielded6.ScalarSize]byte{})
			if err != nil {
				t.Fatal(err)
			}
		}
		fake.rings[i] = shieldedV6RingRPCOutput{Index: hexutil.Uint64(i), OneTimeKey: public[:], Commitment: commitment[:]}
	}
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pqcrypto.PublicKeyBytes(accountKey)
	skeleton := types.NewTx(&types.PQTkmTx{
		ChainID: new(big.Int).SetUint64(chainID), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 20_000_000, To: &params.ShieldedPoolAddress, Value: new(big.Int),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: publicKey,
	})
	built, err := BuildAndSignShieldedV6Payment(context.Background(), fake, accountSeed, identity, keys, skeleton, []*shielded6.Note{note}, []core.ShieldedV6WalletOutput{{Address: keys.Address(), Amount: amount}}, cryptorand.Reader)
	if err != nil {
		t.Fatalf("build and sign Shield6 payment: %v", err)
	}
	if fake.ringBatchCalls != 1 {
		t.Fatalf("wallet made %d ring-member batch calls for one input, want 1", fake.ringBatchCalls)
	}
	if !core.HasShieldedV6Prefix(built.Data()) {
		t.Fatal("wallet builder returned a transaction without a Shield6 envelope")
	}
	envelope, recognized, err := core.DecodeShieldedV6Transaction(built.Data())
	if err != nil || !recognized || len(envelope.Inputs) != 1 || len(envelope.Outputs) != 1 || len(envelope.RangeProof) == 0 {
		t.Fatalf("wallet builder returned an invalid Shield6 envelope: recognized=%t err=%v", recognized, err)
	}
	signer := types.NewQuantumSigner(new(big.Int).SetUint64(chainID))
	from, err := types.Sender(signer, built)
	if err != nil || from != identity.Address {
		t.Fatalf("wallet builder signed with the wrong account: from=%s err=%v", from, err)
	}
}

func TestBuildAndSignShieldedV6Deposit(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires the production Shield3 stamp backend")
	}
	const chainID = uint64(8979)
	accountSeed := bytes.Repeat([]byte{0x24}, pqcrypto.MLDSA87SeedSize)
	stamp, err := pqcrypto.CreateShieldedV3Stamp(accountSeed, chainID, "Shield6 Deposit", "Japan")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := NewIdentity(accountSeed, chainID, stamp)
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Clear()
	shieldKeys, _, err := shielded6.CreateWallet(nil, chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer shieldKeys.Clear()
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		t.Fatal(err)
	}
	amount := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fake := &shieldedV6ScanRPC{
		chainID: new(big.Int).SetUint64(chainID),
		balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(22), nil),
		status:  shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(new(big.Int).SetUint64(chainID))},
		stamp:   core.AntarticalStampStatus{Registered: true, Owner: identity.Owner, Commitment: stamp.Commitment},
	}
	to := params.ShieldedPoolAddress
	skeleton := types.NewTx(&types.PQTkmTx{
		ChainID: new(big.Int).SetUint64(chainID), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 8_000_000, To: &to, Value: amount,
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(accountKey),
	})
	signed, err := BuildAndSignShieldedV6Deposit(context.Background(), fake, accountSeed, identity, shieldKeys, skeleton, amount, cryptorand.Reader)
	if err != nil {
		t.Fatalf("build and sign Shield6 deposit: %v", err)
	}
	envelope, recognized, err := core.DecodeShieldedV6Transaction(signed.Data())
	if err != nil || !recognized || envelope == nil || !envelope.Deposit || len(envelope.Outputs) != 1 || len(envelope.RangeProof) == 0 {
		t.Fatalf("wallet builder returned an invalid Shield6 deposit: recognized=%t err=%v", recognized, err)
	}
	from, err := types.Sender(types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), signed)
	if err != nil || from != identity.Address {
		t.Fatalf("wallet deposit signed from wrong account: from=%s err=%v", from, err)
	}
}

func TestBuildAndSignShieldedV6Withdrawal(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("requires the production Shield3 stamp backend")
	}
	const chainID = uint64(8979)
	accountSeed := bytes.Repeat([]byte{0x33}, pqcrypto.MLDSA87SeedSize)
	stamp, err := pqcrypto.CreateShieldedV3Stamp(accountSeed, chainID, "Shield6 Withdrawal", "Japan")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := NewIdentity(accountSeed, chainID, stamp)
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Clear()
	keys, _, err := shielded6.CreateWallet(nil, chainID)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	amount := big.NewInt(100)
	_, _, witnesses, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	record, err := shielded6.CreateOutput(keys.Address(), 0, shielded6.AssetTKM, amount, witnesses[0], cryptorand.Reader)
	clear(witnesses[0].Mask[:])
	clear(witnesses[0].LimbMasks[0][:])
	clear(witnesses[0].LimbMasks[1][:])
	if err != nil {
		t.Fatal(err)
	}
	note, owned, err := shielded6.ScanOutput(keys, 0, 0, record)
	if err != nil || !owned {
		t.Fatalf("scan owned note: owned=%t err=%v", owned, err)
	}
	defer note.Clear()
	fake := &shieldedV6ScanRPC{
		chainID: new(big.Int).SetUint64(chainID), balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(22), nil),
		status: shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(new(big.Int).SetUint64(chainID)), OutputCount: shielded6.RingSize, Accumulator: common.HexToHash("0x76543210")},
		stamp:  core.AntarticalStampStatus{Registered: true, Owner: identity.Owner, Commitment: stamp.Commitment},
		rings:  make(map[uint64]shieldedV6RingRPCOutput, shielded6.RingSize),
	}
	for i := uint64(0); i < shielded6.RingSize; i++ {
		pub, commitment := record.OneTimeKey, record.Commitment
		if i > 0 {
			seed := bytes.Repeat([]byte{byte(i + 3)}, 32)
			decoy, _, createErr := shielded6.CreateWallet(bytes.NewReader(seed), chainID)
			if createErr != nil {
				t.Fatal(createErr)
			}
			pub = decoy.SpendPublic
			decoy.Clear()
			commitment, err = shielded6.Commitment128(new(big.Int).SetUint64(i+200), [shielded6.ScalarSize]byte{})
			if err != nil {
				t.Fatal(err)
			}
		}
		fake.rings[i] = shieldedV6RingRPCOutput{Index: hexutil.Uint64(i), OneTimeKey: pub[:], Commitment: commitment[:]}
	}
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(accountSeed)
	if err != nil {
		t.Fatal(err)
	}
	skeleton := types.NewTx(&types.PQTkmTx{
		ChainID: new(big.Int).SetUint64(chainID), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 20_000_000, To: &params.ShieldedPoolAddress, Value: new(big.Int),
		Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqcrypto.PublicKeyBytes(accountKey),
	})
	to := common.HexToAddress("0x1234567890123456789012345678901234567890")
	signed, err := BuildAndSignShieldedV6Withdrawal(context.Background(), fake, accountSeed, identity, keys, skeleton, []*shielded6.Note{note}, to, big.NewInt(40), cryptorand.Reader)
	if err != nil {
		t.Fatalf("build and sign Shield6 withdrawal: %v", err)
	}
	envelope, recognized, err := core.DecodeShieldedV6Transaction(signed.Data())
	if err != nil || !recognized || envelope == nil || envelope.WithdrawalRecipient != to || envelope.WithdrawalValue.Cmp(big.NewInt(40)) != 0 || len(envelope.Outputs) != 1 {
		t.Fatalf("invalid Shield6 withdrawal envelope: recognized=%t err=%v", recognized, err)
	}
	if _, err := types.Sender(types.NewQuantumSigner(new(big.Int).SetUint64(chainID)), signed); err != nil {
		t.Fatalf("withdrawal sender signature: %v", err)
	}
	fullExit, err := BuildAndSignShieldedV6Withdrawal(context.Background(), fake, accountSeed, identity, keys, skeleton, []*shielded6.Note{note}, to, amount, cryptorand.Reader)
	if err != nil {
		t.Fatalf("build full Shield6 exit without change: %v", err)
	}
	fullEnvelope, recognized, err := core.DecodeShieldedV6Transaction(fullExit.Data())
	if err != nil || !recognized || fullEnvelope == nil || len(fullEnvelope.Outputs) != 0 || fullEnvelope.WithdrawalValue.Cmp(amount) != 0 {
		t.Fatalf("invalid full Shield6 exit envelope: recognized=%t err=%v", recognized, err)
	}
}

func TestSelectShieldedV6RingUsesCanonicalCandidates(t *testing.T) {
	const count = shielded6.RingSize + 4
	root := common.HexToHash("0x9876")
	fake := &shieldedV6ScanRPC{
		chainID: big.NewInt(8979), head: common.HexToHash("0x1234"),
		status: shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(big.NewInt(8979)), OutputCount: count, Accumulator: root},
		rings:  make(map[uint64]shieldedV6RingRPCOutput, count),
	}
	zeroMask := [shielded6.ScalarSize]byte{}
	for i := uint64(0); i < count; i++ {
		seed := make([]byte, 32)
		seed[0], seed[31] = byte(i+1), byte(i+11)
		keys, _, err := shielded6.CreateWallet(bytes.NewReader(seed), 8979)
		if err != nil {
			t.Fatal(err)
		}
		commitment, err := shielded6.Commitment128(new(big.Int).SetUint64(i+1), zeroMask)
		keys.Clear()
		if err != nil {
			t.Fatal(err)
		}
		fake.rings[i] = shieldedV6RingRPCOutput{Index: hexutil.Uint64(i), OneTimeKey: hexutil.Bytes(keys.SpendPublic[:]), Commitment: hexutil.Bytes(commitment[:])}
	}
	ring, err := SelectShieldedV6Ring(context.Background(), fake, 8979, 5, nil)
	if err != nil {
		t.Fatalf("select canonical Shield6 ring: %v", err)
	}
	if fake.ringBatchCalls != 1 {
		t.Fatalf("ring selector made %d RPC requests for one ring, want 1 bounded batch", fake.ringBatchCalls)
	}
	if ring.Root != root || ring.Indices[ring.SignerIndex] != 5 {
		t.Fatalf("selected ring has wrong root or real member: %+v", ring)
	}
	seen := make(map[uint64]struct{}, shielded6.RingSize)
	for _, index := range ring.Indices {
		if _, exists := seen[index]; exists {
			t.Fatalf("ring repeats accumulator index %d", index)
		}
		seen[index] = struct{}{}
	}
}

func TestScanShieldedV6FindsSpendableNoteAndDetectsReorg(t *testing.T) {
	keys, _, err := shielded6.CreateWallet(nil, 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	amount := big.NewInt(42)
	commitments, _, witnesses, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(witnesses[0].Mask[:])
	defer clear(witnesses[0].LimbMasks[0][:])
	defer clear(witnesses[0].LimbMasks[1][:])
	record, err := shielded6.CreateOutput(keys.Address(), 3, shielded6.AssetTKM, amount, witnesses[0], cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if record.Commitment != commitments[0].Commitment || record.AmountLimbs != commitments[0].Limbs {
		t.Fatal("fixture output does not match its range-proof commitments")
	}
	fake := &shieldedV6ScanRPC{
		chainID: big.NewInt(8979), head: common.HexToHash("0x1234"),
		status: shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(big.NewInt(8979)), OutputCount: 1, Accumulator: common.HexToHash("0x5678")},
		outputs: []shieldedV6RPCOutput{{
			AccumulatorIndex: 0, OutputPosition: 3, BlockNumber: 2,
			BlockHash: common.HexToHash("0xabcd"), TransactionHash: common.HexToHash("0xbeef"),
			OneTimeKey: record.OneTimeKey[:], EphemeralKey: record.EphemeralKey[:],
			Commitment: record.Commitment[:], AmountLimbs: [2]hexutil.Bytes{record.AmountLimbs[0][:], record.AmountLimbs[1][:]},
			EncryptedNote: record.EncryptedNote,
		}},
	}
	result, err := ScanShieldedV6(context.Background(), fake, keys)
	if err != nil {
		t.Fatalf("scan Shield6 outputs: %v", err)
	}
	if result.BalanceWei.Cmp(amount) != 0 || len(result.Notes) != 1 || result.Notes[0].Note.Index != 0 || result.Notes[0].BlockNumber != 2 {
		t.Fatalf("unexpected scan result: balance=%v notes=%+v", result.BalanceWei, result.Notes)
	}
	result.Notes[0].Note.Clear()

	fake = &shieldedV6ScanRPC{
		chainID: big.NewInt(8979), head: common.HexToHash("0x1234"), flipHead: true,
		status: shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(big.NewInt(8979)), OutputCount: 1, Accumulator: common.HexToHash("0x5678")},
		outputs: []shieldedV6RPCOutput{{
			AccumulatorIndex: 0, OutputPosition: 3, BlockNumber: 2,
			BlockHash: common.HexToHash("0xabcd"), TransactionHash: common.HexToHash("0xbeef"),
			OneTimeKey: record.OneTimeKey[:], EphemeralKey: record.EphemeralKey[:],
			Commitment: record.Commitment[:], AmountLimbs: [2]hexutil.Bytes{record.AmountLimbs[0][:], record.AmountLimbs[1][:]},
			EncryptedNote: record.EncryptedNote,
		}},
	}
	if _, err := ScanShieldedV6(context.Background(), fake, keys); err == nil {
		t.Fatal("scan accepted a reorg during page retrieval")
	}
}

func TestClearShieldedV6SpendCandidatesZeroizesPageNotes(t *testing.T) {
	note := &shielded6.Note{
		Amount:      big.NewInt(123),
		SpendSecret: [shielded6.ScalarSize]byte{1, 2, 3},
		Witness: shielded6.WideWitness{
			Mask:      [shielded6.ScalarSize]byte{4, 5, 6},
			LimbMasks: [2][shielded6.ScalarSize]byte{{7, 8, 9}, {10, 11, 12}},
		},
	}
	candidates := []shieldedV6SpendCandidate{{note: note}}
	clearShieldedV6SpendCandidates(candidates)
	if candidates[0].note != nil || note.Amount.Sign() != 0 {
		t.Fatal("candidate note reference or amount was not cleared")
	}
	if note.SpendSecret != ([shielded6.ScalarSize]byte{}) || note.Witness.Mask != ([shielded6.ScalarSize]byte{}) || note.Witness.LimbMasks != ([2][shielded6.ScalarSize]byte{}) {
		t.Fatal("candidate note secrets were not zeroized")
	}
}

func TestScanShieldedV6MatchesPublicSpentKeyImagesLocally(t *testing.T) {
	keys, _, err := shielded6.CreateWallet(nil, 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	amount := big.NewInt(7)
	commitments, _, witnesses, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, cryptorand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	record, err := shielded6.CreateOutput(keys.Address(), 0, shielded6.AssetTKM, amount, witnesses[0], cryptorand.Reader)
	clear(witnesses[0].Mask[:])
	clear(witnesses[0].LimbMasks[0][:])
	clear(witnesses[0].LimbMasks[1][:])
	if err != nil {
		t.Fatal(err)
	}
	note, owned, err := shielded6.ScanOutput(keys, 0, 0, record)
	if err != nil || !owned {
		t.Fatalf("scan test-owned output: owned=%t err=%v", owned, err)
	}
	image, err := shielded6.DeriveKeyImage(record.OneTimeKey[:], note.SpendSecret[:])
	note.Clear()
	if err != nil {
		t.Fatal(err)
	}
	fake := &shieldedV6ScanRPC{
		chainID: big.NewInt(8979), head: common.HexToHash("0x1234"),
		status: shieldedV6RPCStatus{Active: true, ChainID: (*hexutil.Big)(big.NewInt(8979)), OutputCount: 1, SpentKeyImageCount: 1, Accumulator: common.HexToHash("0x5678")},
		outputs: []shieldedV6RPCOutput{{
			AccumulatorIndex: 0, OutputPosition: 0, BlockNumber: 2,
			BlockHash: common.HexToHash("0xabcd"), TransactionHash: common.HexToHash("0xbeef"),
			OneTimeKey: record.OneTimeKey[:], EphemeralKey: record.EphemeralKey[:],
			Commitment: commitments[0].Commitment[:], AmountLimbs: [2]hexutil.Bytes{commitments[0].Limbs[0][:], commitments[0].Limbs[1][:]},
			EncryptedNote: record.EncryptedNote,
		}},
		spentKeyImages: []hexutil.Bytes{common.CopyBytes(image[:])},
	}
	result, err := ScanShieldedV6(context.Background(), fake, keys)
	if err != nil {
		t.Fatal(err)
	}
	if result.BalanceWei.Sign() != 0 || len(result.Notes) != 0 {
		t.Fatal("confirmed key-image spend was returned as spendable")
	}
	fake.status.SpentKeyImageCount = 0
	fake.spentKeyImages = nil
	fake.pendingKeyImages = []hexutil.Bytes{common.CopyBytes(image[:])}
	pending, err := ScanShieldedV6(context.Background(), fake, keys)
	if err != nil {
		t.Fatal(err)
	}
	if pending.BalanceWei.Sign() != 0 || len(pending.Notes) != 0 {
		t.Fatal("public pending key-image set did not exclude the locally owned note")
	}
	clear(image[:])
}
