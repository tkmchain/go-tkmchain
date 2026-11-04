package eth

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func TestShieldedV6IndexedOutputsPreserveRequestOrder(t *testing.T) {
	storage := make(map[common.Hash]common.Hash)
	storage[core.ShieldedV6StateSlot("output-count", nil)] = common.BigToHash(big.NewInt(3))
	wantKeys := make([][shielded6.PointSize]byte, 3)
	for index := range wantKeys {
		keys, _, err := shielded6.CreateWallet(nil, 8979)
		if err != nil {
			t.Fatal(err)
		}
		copy(wantKeys[index][:], keys.SpendPublic[:])
		keys.Clear()
		commitment, err := shielded6.Commitment128(big.NewInt(int64(index+1)), [shielded6.ScalarSize]byte{})
		if err != nil {
			t.Fatal(err)
		}
		var encodedIndex [8]byte
		binary.BigEndian.PutUint64(encodedIndex[:], uint64(index))
		storage[core.ShieldedV6StateSlot("output-key", encodedIndex[:])] = common.BytesToHash(wantKeys[index][:])
		storage[core.ShieldedV6StateSlot("output-commitment", encodedIndex[:])] = common.BytesToHash(commitment[:])
	}
	_, eth := newTestKingAPI(t, types.GenesisAlloc{params.ShieldedPoolAddress: {Storage: storage}})
	config := eth.blockchain.Config()
	config.AntarticalTime, config.PrivacyCommitmentTime, config.Shield6Time = new(uint64), new(uint64), new(uint64)
	api := NewPrivacyAPI(eth)
	indices := []hexutil.Uint64{2, 0, 1}
	outputs, err := api.ShieldedV6OutputsByIndex(indices)
	if err != nil {
		t.Fatalf("resolve indexed Shield6 candidates: %v", err)
	}
	if len(outputs) != len(indices) {
		t.Fatalf("indexed output count = %d, want %d", len(outputs), len(indices))
	}
	for position, index := range indices {
		if outputs[position].Index != index || string(outputs[position].OneTimeKey) != string(wantKeys[index][:]) {
			t.Fatalf("candidate at request position %d does not match requested index %d", position, index)
		}
	}
}

func TestShieldedV6ReadAPIsAreBoundedAndActivationGated(t *testing.T) {
	_, eth := newTestKingAPI(t, nil)
	cfg := eth.blockchain.Config()
	cfg.AntarticalTime = new(uint64)
	cfg.PrivacyCommitmentTime = new(uint64)
	cfg.Shield6Time = new(uint64)
	api := NewPrivacyAPI(eth)
	status, err := api.ShieldedV6Status()
	if err != nil {
		t.Fatalf("Shield6 status: %v", err)
	}
	if !status.Active || status.RingSize == 0 || status.OutputCount != 0 || status.Accumulator == ([32]byte{}) {
		t.Fatalf("unexpected empty Shield6 status: %+v", status)
	}
	candidates, err := api.ShieldedV6Outputs(0, 32)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("empty Shield6 ring-candidate page: count=%d err=%v", len(candidates), err)
	}
	walletOutputs, err := api.ShieldedV6WalletOutputs(0, 16)
	if err != nil || len(walletOutputs) != 0 {
		t.Fatalf("empty Shield6 wallet-output page: count=%d err=%v", len(walletOutputs), err)
	}
	if _, err := api.ShieldedV6Outputs(0, hexutil.Uint64(maxShieldedV6OutputPage+1)); err == nil {
		t.Fatal("oversized Shield6 ring-candidate page was accepted")
	}
	if _, err := api.ShieldedV6OutputsByIndex(nil); err == nil {
		t.Fatal("empty Shield6 indexed-output batch was accepted")
	}
	if _, err := api.ShieldedV6OutputsByIndex([]hexutil.Uint64{0}); err == nil {
		t.Fatal("out-of-range Shield6 indexed output was accepted")
	}
	if _, err := api.ShieldedV6OutputsByIndex([]hexutil.Uint64{0, 0}); err == nil {
		t.Fatal("duplicate Shield6 indexed output was accepted")
	}
	tooManyIndices := make([]hexutil.Uint64, maxShieldedV6OutputPage+1)
	for i := range tooManyIndices {
		tooManyIndices[i] = hexutil.Uint64(i)
	}
	if _, err := api.ShieldedV6OutputsByIndex(tooManyIndices); err == nil {
		t.Fatal("oversized Shield6 indexed-output batch was accepted")
	}
	if _, err := api.ShieldedV6WalletOutputs(0, hexutil.Uint64(maxShieldedV6WalletOutputPage+1)); err == nil {
		t.Fatal("oversized Shield6 wallet-output page was accepted")
	}
	spentImages, err := api.ShieldedV6SpentKeyImages(0, 16)
	if err != nil || len(spentImages) != 0 {
		t.Fatalf("empty Shield6 public key-image page: count=%d err=%v", len(spentImages), err)
	}
	if _, err := api.ShieldedV6SpentKeyImages(0, hexutil.Uint64(maxShieldedV6KeyImagePage+1)); err == nil {
		t.Fatal("oversized Shield6 public key-image page was accepted")
	}
	if pending, err := api.ShieldedV6PendingKeyImages(); err != nil || len(pending) != 0 {
		t.Fatalf("empty Shield6 pending key-image set: count=%d err=%v", len(pending), err)
	}
	cfg.Shield6Time = nil
	if _, err := api.ShieldedV6Outputs(0, 1); err == nil {
		t.Fatal("Shield6 accumulator outputs were served before activation")
	}
	if _, err := api.ShieldedV6OutputsByIndex([]hexutil.Uint64{0}); err == nil {
		t.Fatal("Shield6 indexed outputs were served before activation")
	}
}

func TestShieldedV6SpentKeyImagePageReturnsPublicIndexOrder(t *testing.T) {
	keys, _, err := shielded6.CreateWallet(nil, 8979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	image, err := shielded6.DeriveKeyImage(keys.SpendPublic[:], keys.SpendSecret[:])
	if err != nil {
		t.Fatal(err)
	}
	var index [8]byte
	storage := map[common.Hash]common.Hash{
		core.ShieldedV6StateSlot("key-image-count", nil):      common.BigToHash(big.NewInt(1)),
		core.ShieldedV6StateSlot("key-image-index", index[:]): common.BytesToHash(image[:]),
	}
	_, eth := newTestKingAPI(t, types.GenesisAlloc{params.ShieldedPoolAddress: {Storage: storage}})
	config := eth.blockchain.Config()
	config.AntarticalTime, config.PrivacyCommitmentTime, config.Shield6Time = new(uint64), new(uint64), new(uint64)
	page, err := NewPrivacyAPI(eth).ShieldedV6SpentKeyImages(0, 16)
	if err != nil || len(page) != 1 || string(page[0]) != string(image[:]) {
		t.Fatalf("public Shield6 key-image page = %x, err=%v; want %x", page, err, image)
	}
}
