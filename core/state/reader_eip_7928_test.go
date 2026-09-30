// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

package state

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
)

type accessListReaderTestReader struct {
	account *types.StateAccount
	code    []byte
	storage map[common.Hash]common.Hash
}

func (r *accessListReaderTestReader) Account(common.Address) (*types.StateAccount, error) {
	if r.account == nil {
		return nil, nil
	}
	return r.account.Copy(), nil
}

func (r *accessListReaderTestReader) Storage(_ common.Address, slot common.Hash) (common.Hash, error) {
	return r.storage[slot], nil
}

func (r *accessListReaderTestReader) Has(_ common.Address, codeHash common.Hash) bool {
	return common.BytesToHash(crypto.Keccak256(r.code)) == codeHash
}

func (r *accessListReaderTestReader) Code(_ common.Address, _ common.Hash) []byte {
	return common.CopyBytes(r.code)
}

func (r *accessListReaderTestReader) CodeSize(_ common.Address, _ common.Hash) int {
	return len(r.code)
}

func TestReaderWithBlockLevelAccessListOverlaysPriorTransactions(t *testing.T) {
	address := common.HexToAddress("0x1234")
	slot := common.HexToHash("0x01")
	oldCode := []byte{0x60, 0x00}
	newCode := []byte{0x60, 0x01, 0x00}
	base := &accessListReaderTestReader{
		account: &types.StateAccount{Nonce: 4, Balance: uint256.NewInt(10), Root: types.EmptyRootHash, CodeHash: crypto.Keccak256(oldCode)},
		code:    oldCode,
		storage: map[common.Hash]common.Hash{slot: common.HexToHash("0x10")},
	}
	access := bal.NewConstructionBlockAccessList()
	access.BalanceChange(1, address, uint256.NewInt(20))
	access.BalanceChange(3, address, uint256.NewInt(40))
	access.NonceChange(address, 1, 5)
	access.NonceChange(address, 3, 7)
	access.StorageWrite(1, address, slot, common.HexToHash("0x20"))
	access.StorageWrite(3, address, slot, common.HexToHash("0x30"))
	access.CodeChange(address, 1, newCode)

	reader := NewReaderWithBlockLevelAccessList(base, &access, 3)
	account, err := reader.Account(address)
	if err != nil {
		t.Fatal(err)
	}
	if account.Balance.Uint64() != 20 || account.Nonce != 5 {
		t.Fatalf("account overlay = balance %d nonce %d, want balance 20 nonce 5", account.Balance.Uint64(), account.Nonce)
	}
	if got, err := reader.Storage(address, slot); err != nil || got != common.HexToHash("0x20") {
		t.Fatalf("storage overlay = %s, %v", got, err)
	}
	codeHash := common.BytesToHash(crypto.Keccak256(newCode))
	if !reader.Has(address, codeHash) {
		t.Fatal("code overlay was not visible")
	}
	if got := reader.Code(address, codeHash); string(got) != string(newCode) {
		t.Fatalf("code overlay = %x", got)
	}
	if size := reader.CodeSize(address, codeHash); size != len(newCode) {
		t.Fatalf("code size overlay = %d", size)
	}
}
