// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
// or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser General Public License
// for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package external

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

type testAccountService struct {
	tx   *types.Transaction
	args apitypes.SendTxArgs
}

func (s *testAccountService) SignTransaction(ctx context.Context, args apitypes.SendTxArgs) (*signTransactionResult, error) {
	s.args = args
	return &signTransactionResult{Tx: s.tx}, nil
}

func TestSignTxSupportsRandomXTx(t *testing.T) {
	to := common.HexToAddress("0x000000000000000000000000000000000000c0de")
	tx := types.NewTx(&types.RandomXTx{
		ChainID:   big.NewInt(1),
		Nonce:     7,
		GasTipCap: big.NewInt(2),
		GasFeeCap: big.NewInt(20),
		Gas:       21000,
		To:        &to,
		Value:     big.NewInt(3),
	})

	service := &testAccountService{tx: tx}
	server := rpc.NewServer()
	if err := server.RegisterName("account", service); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	client := rpc.DialInProc(server)
	defer client.Close()
	signer := &ExternalSigner{client: client}

	account := accounts.Account{Address: common.HexToAddress("0x0000000000000000000000000000000000000123")}
	signed, err := signer.SignTx(account, tx, big.NewInt(1))
	if err != nil {
		t.Fatalf("SignTx returned error: %v", err)
	}
	if signed.Type() != types.RandomXTxType {
		t.Fatalf("signed tx type = %d, want %d", signed.Type(), types.RandomXTxType)
	}
	if service.args.Type == nil || uint64(*service.args.Type) != types.RandomXTxType {
		t.Fatalf("signer request type = %v, want %d", service.args.Type, types.RandomXTxType)
	}
	if have, want := (*big.Int)(service.args.MaxFeePerGas), tx.GasFeeCap(); have.Cmp(want) != 0 {
		t.Fatalf("maxFeePerGas = %v, want %v", have, want)
	}
	if have, want := (*big.Int)(service.args.MaxPriorityFeePerGas), tx.GasTipCap(); have.Cmp(want) != 0 {
		t.Fatalf("maxPriorityFeePerGas = %v, want %v", have, want)
	}
}

func TestSignTxSupportsPQTkmContractCreation(t *testing.T) {
	key, err := pqcrypto.GenerateMLDSA87()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := types.SignNewPQTkmTx(key, types.NewQuantumSigner(big.NewInt(8979)), &types.PQTkmTx{
		ChainID:   big.NewInt(8979),
		Nonce:     8,
		GasTipCap: big.NewInt(3),
		GasFeeCap: big.NewInt(30),
		Gas:       500000,
		Value:     new(big.Int),
		Data:      []byte{0x60, 0x00, 0x60, 0x00, 0xf3},
	})
	if err != nil {
		t.Fatal(err)
	}

	service := &testAccountService{tx: tx}
	server := rpc.NewServer()
	if err := server.RegisterName("account", service); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	client := rpc.DialInProc(server)
	defer client.Close()

	signer := &ExternalSigner{client: client}
	account := accounts.Account{Address: common.HexToAddress("0x0000000000000000000000000000000000000123")}
	signed, err := signer.SignTx(account, tx, big.NewInt(8979))
	if err != nil {
		t.Fatal(err)
	}
	if signed.Type() != types.PQTkmTxType {
		t.Fatalf("signed tx type = %d, want %d", signed.Type(), types.PQTkmTxType)
	}
	if service.args.Type == nil || uint64(*service.args.Type) != types.PQTkmTxType {
		t.Fatalf("signer request type = %v, want %d", service.args.Type, types.PQTkmTxType)
	}
	if service.args.To != nil {
		t.Fatalf("PQ contract creation recipient = %v, want nil", service.args.To)
	}
	if service.args.Input == nil || string(*service.args.Input) != string(tx.Data()) {
		t.Fatalf("PQ contract creation data = %x, want %x", service.args.Input, tx.Data())
	}
}

func TestSendTxArgsToTransactionSupportsRandomXTx(t *testing.T) {
	txType := hexutil.Uint64(types.RandomXTxType)
	args := apitypes.SendTxArgs{
		Type:                 &txType,
		ChainID:              (*hexutil.Big)(big.NewInt(1)),
		Gas:                  21000,
		MaxFeePerGas:         (*hexutil.Big)(big.NewInt(20)),
		MaxPriorityFeePerGas: (*hexutil.Big)(big.NewInt(2)),
	}
	tx, err := args.ToTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if tx.Type() != types.RandomXTxType {
		t.Fatalf("tx type = %d, want %d", tx.Type(), types.RandomXTxType)
	}
}

func TestSendTxArgsToTransactionSupportsPQTkmTx(t *testing.T) {
	txType := hexutil.Uint64(types.PQTkmTxType)
	input := hexutil.Bytes{0x60, 0x00, 0x60, 0x00, 0xf3}
	args := apitypes.SendTxArgs{
		Type:                 &txType,
		ChainID:              (*hexutil.Big)(big.NewInt(8979)),
		Gas:                  500000,
		MaxFeePerGas:         (*hexutil.Big)(big.NewInt(20)),
		MaxPriorityFeePerGas: (*hexutil.Big)(big.NewInt(2)),
		Input:                &input,
	}
	tx, err := args.ToTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if tx.Type() != types.PQTkmTxType {
		t.Fatalf("tx type = %d, want %d", tx.Type(), types.PQTkmTxType)
	}
	if tx.To() != nil {
		t.Fatalf("contract creation recipient = %v, want nil", tx.To())
	}
	if string(tx.Data()) != string(input) {
		t.Fatalf("contract creation data = %x, want %x", tx.Data(), input)
	}
}

func TestSendTxArgsToTransactionRejectsIncompletePQTkmTx(t *testing.T) {
	txType := hexutil.Uint64(types.PQTkmTxType)
	args := apitypes.SendTxArgs{Type: &txType}
	if _, err := args.ToTransaction(); err == nil {
		t.Fatal("incomplete PQ transaction was accepted")
	}
}
