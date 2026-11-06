package core

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

func TestShield6RecipientRegistryAuthorizationAndRollback(t *testing.T) {
	if !shielded3.NativeAvailable() {
		t.Skip("native Tip5")
	}
	st := newShield6RehearsalState(t)
	owner := shielded3.Digest{1, 2, 3, 4, 5}
	otherOwner := shielded3.Digest{6, 7, 8, 9, 10}
	seed := bytes.Repeat([]byte{0x72}, pqcrypto.MLDSA87SeedSize)
	accountKey, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(accountKey))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := shielded6.DeriveWalletKeys(seed, 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	commitment, err := recipient.BindingCommitment(8979, owner, keys.Address())
	if err != nil {
		t.Fatal(err)
	}
	e := &ShieldedV6Transaction{Version: ShieldedV6BindingVersion, ValidUntil: 3600, AssetID: 1, Binding: &ShieldedV6BindingRegistration{Owner: owner, Commitment: commitment, Proof: []byte{1}}}
	data, err := EncodeShieldedV6Transaction(e)
	if err != nil {
		t.Fatal(err)
	}
	skeleton := shield6RehearsalTx(8979, 0, new(big.Int), data, nil, pqcrypto.PublicKeyBytes(accountKey))
	tx, err := types.SignPQTkmTx(skeleton, types.NewQuantumSigner(big.NewInt(8979)), accountKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateShieldedV6BindingBasics(tx, e); err != nil {
		t.Fatal(err)
	}
	if err := processShieldedV6Binding(st, tx, address, e); err == nil {
		t.Fatal("unstamped registration accepted")
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", address.Bytes()), common.Hash{1})
	v3WriteDigest(st, "stamp/address-owner", address.Bytes(), owner)
	if err := appendAntarticalStamp(st, 8979, owner, common.Hash{1}); err != nil {
		t.Fatal(err)
	}
	snapshot := st.Snapshot()
	if err := processShieldedV6Binding(st, tx, address, e); err != nil {
		t.Fatal(err)
	}
	path, err := ShieldedV6BindingPath(st, owner)
	if err != nil || !path.Found || path.Commitment != commitment {
		t.Fatalf("binding path: %+v %v", path, err)
	}
	stamp, err := AntarticalStampPath(st, owner)
	if err != nil {
		t.Fatal(err)
	}
	code := recipient.PaymentCode{Version: recipient.RelationVersion, Address: keys.Address(), Owner: owner, StampRoot: stamp.Root, StampIndex: uint32(stamp.Index), StampPath: stamp.Path, BindingRoot: path.Root, BindingIndex: uint32(path.Index), BindingPath: path.Path}
	if err := code.Validate(8979); err != nil {
		t.Fatal(err)
	}
	if err := processShieldedV6Binding(st, tx, address, e); err == nil {
		t.Fatal("immutable binding overwritten")
	}
	forged, err := recipient.AuthorizedBindingLeaf(8979, otherOwner, commitment)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := recipient.BindingLeaf(8979, owner, keys.Address())
	if err != nil || forged == actual {
		t.Fatal("registration did not distinguish authenticated owners")
	}
	roots := ShieldedV6RecipientRoots{StampRoot: stamp.Root, BindingRoot: path.Root, Anchor: ShieldedV6RecipientAnchor(8979, stamp.Root, path.Root)}
	bound := &ShieldedV6Transaction{Outputs: []ShieldedV6Output{{}}, Recipients: &ShieldedV6RecipientProofs{Roots: []ShieldedV6RecipientRoots{roots}}}
	if err := validateShieldedV6RecipientRoots(st, tx, bound); err != nil {
		t.Fatal(err)
	}
	st.RevertToSnapshot(snapshot)
	if ShieldedV6BindingCount(st) != 0 {
		t.Fatal("binding count survived rollback")
	}
	if err := validateShieldedV6RecipientRoots(st, tx, bound); err == nil {
		t.Fatal("reorganized registry root accepted")
	}
	path, err = ShieldedV6BindingPath(st, owner)
	if err != nil || path.Found {
		t.Fatal("binding lookup survived rollback")
	}
	if err := processShieldedV6Binding(st, tx, address, e); err != nil {
		t.Fatal(err)
	}
	if root, _ := ShieldedV6BindingRoot(st); root != roots.BindingRoot {
		t.Fatal("replayed registry produced a different root")
	}
}

func TestShield6RecipientIntentAndForkGate(t *testing.T) {
	pool := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), Gas: 10_000_000, GasFeeCap: big.NewInt(2), GasTipCap: big.NewInt(1), To: &pool, Value: new(big.Int)})
	e := &ShieldedV6Transaction{Version: ShieldedV6BoundVersion, AssetID: 1, Inputs: []ShieldedV6Input{{}}, Outputs: []ShieldedV6Output{{EncryptedNote: []byte{1}}}, RangeProof: []byte{1}, Recipients: &ShieldedV6RecipientProofs{Roots: []ShieldedV6RecipientRoots{{StampRoot: shielded3.Digest{1}, BindingRoot: shielded3.Digest{2}}}, Proofs: [][]byte{{1}}}}
	before, err := ShieldedV6Intent(tx, e)
	if err != nil {
		t.Fatal(err)
	}
	e.Recipients.Proofs[0] = []byte{9, 8, 7}
	after, err := ShieldedV6Intent(tx, e)
	if err != nil || before != after {
		t.Fatal("recipient proof created circular signing intent")
	}
	e.Recipients.Roots[0].BindingRoot[0]++
	after, err = ShieldedV6Intent(tx, e)
	if err != nil || before == after {
		t.Fatal("intent omitted recipient roots")
	}
	if err := ValidateShieldedV6RecipientProof(nil, tx, e); err == nil {
		t.Fatal("missing verification key accepted")
	}
	e.Recipients.Proofs = nil
	if err := ValidateShieldedV6RecipientProof(nil, tx, e); err == nil {
		t.Fatal("missing per-output proof accepted")
	}
	registration := &ShieldedV6Transaction{Version: ShieldedV6BindingVersion, ValidUntil: 3600, AssetID: 1, Binding: &ShieldedV6BindingRegistration{Owner: shielded3.Digest{1}, Commitment: shielded3.Digest{1}, Proof: []byte{1}}}
	data, err := EncodeShieldedV6Transaction(registration)
	if err != nil {
		t.Fatal(err)
	}
	_, gas, err := ShieldedV6GasData(data)
	if err != nil || gas != ShieldedV6BaseVerifyGas+106*ShieldedV6StateWriteGas+ShieldedV6RecipientVerifyGas+100 {
		t.Fatal("registration storage is not fully priced")
	}
	if _, err := shieldedOnlyFields(types.PQTkmTxType, &pool, new(big.Int), data); err != nil {
		t.Fatal(err)
	}
	registration.Outputs = []ShieldedV6Output{{}}
	if validShieldedV6BindingShape(registration) {
		t.Fatal("value-bearing registration accepted")
	}
}

func TestShield6RecipientExpiry(t *testing.T) {
	for _, version := range []uint64{ShieldedV6BoundVersion, ShieldedV6BindingVersion} {
		e := &ShieldedV6Transaction{Version: version, ValidUntil: 3700}
		if ValidateShieldedV6Time(e, 100) != nil || ValidateShieldedV6Time(e, 3699) != nil {
			t.Fatal("valid deadline rejected")
		}
		if ValidateShieldedV6Time(e, 3700) == nil || ValidateShieldedV6Time(e, 99) == nil {
			t.Fatal("expired or excessive lifetime accepted")
		}
		e.ValidUntil = 0
		if ValidateShieldedV6Time(e, 100) == nil {
			t.Fatal("missing deadline accepted")
		}
	}
	if ValidateShieldedV6Time(&ShieldedV6Transaction{Version: 6}, 100) != nil {
		t.Fatal("changed historical V6 deadline semantics")
	}
}
