package core

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/shield6test"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/holiman/uint256"
)

func newShield6RecipientNetworkState(t *testing.T) *state.StateDB {
	t.Helper()
	disk := rawdb.NewMemoryDatabase()
	cfg := *triedb.HashDefaults
	cfg.Preimages = true // The exported genesis must retain exact slot keys.
	trie := triedb.NewDatabase(disk, &cfg)
	t.Cleanup(func() { trie.Close(); disk.Close() })
	st, err := state.New(types.EmptyRootHash, state.NewDatabase(trie, state.NewCodeDB(disk)))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func shield6RecipientNetworkAlloc(t *testing.T, original *state.StateDB) types.GenesisAlloc {
	t.Helper()
	copyState := original.Copy()
	root, err := copyState.Commit(0, false, false)
	if err != nil {
		t.Fatal(err)
	}
	db := original.Database()
	if err := db.TrieDB().Commit(root, false); err != nil {
		t.Fatal(err)
	}
	committed, err := state.New(root, db)
	if err != nil {
		t.Fatal(err)
	}
	dump := &state.Dump{Accounts: make(map[string]state.DumpAccount)}
	if _, err := committed.DumpToCollector(dump, nil); err != nil {
		t.Fatal(err)
	}
	alloc := make(types.GenesisAlloc, len(dump.Accounts))
	for address, account := range dump.Accounts {
		balance, ok := new(big.Int).SetString(account.Balance, 10)
		if !common.IsHexAddress(address) || !ok {
			t.Fatal("network snapshot is missing an account preimage")
		}
		storage := make(map[common.Hash]common.Hash, len(account.Storage))
		for key, value := range account.Storage {
			storage[key] = common.HexToHash(value)
		}
		alloc[common.HexToAddress(address)] = types.Account{Balance: balance, Nonce: account.Nonce, Code: account.Code, Storage: storage}
	}
	// Dumping skips storage keys without preimages. Reconstructing the exact
	// root catches any such omission before a fixture can reach networking.
	got, err := hashAlloc(&alloc, false)
	if err != nil || got != root {
		t.Fatalf("network snapshot lost state: have %s want %s (%v)", got, root, err)
	}
	return alloc
}

func TestShield6RecipientNetworkSnapshot(t *testing.T) {
	st := newShield6RecipientNetworkState(t)
	address := common.Address{1}
	st.AddBalance(address, uint256.NewInt(123), tracing.BalanceChangeUnspecified)
	st.SetNonce(address, 7, tracing.NonceChangeUnspecified)
	key, value := common.Hash{2}, common.Hash{3}
	st.SetState(params.ShieldedPoolAddress, key, value)
	alloc := shield6RecipientNetworkAlloc(t, st)
	if alloc[address].Balance.Cmp(big.NewInt(123)) != 0 || alloc[address].Nonce != 7 || alloc[params.ShieldedPoolAddress].Storage[key] != value {
		t.Fatal("network snapshot lost account or protocol state")
	}
	// Exercise the JSON boundary too: GenesisAlloc uses custom address and
	// account decoding, while the TCP process runs in a separate test binary.
	path := filepath.Join(t.TempDir(), "network.json")
	config := *params.TestChainConfig
	config.ChainID = big.NewInt(shield6test.ChainID)
	fixture := shield6test.Fixture{Config: &config, Alloc: alloc, Payment: []byte{1}, Attacks: make([]hexutil.Bytes, 4)}
	if err := shield6test.Write(path, fixture); err != nil {
		t.Fatal(err)
	}
	decoded, err := shield6test.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := hashAlloc(&alloc, false)
	if err != nil {
		t.Fatal(err)
	}
	after, err := hashAlloc(&decoded.Alloc, false)
	if err != nil || before != after {
		t.Fatal("network fixture JSON changed the canonical snapshot root", err)
	}
	if err := shield6test.Write(path, fixture); err == nil {
		t.Fatal("network fixture overwrote an earlier rehearsal result")
	}
}
