package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func TestShield3BlockFitsPeerTransport(t *testing.T) {
	// Oversized activated bodies must fail before any chain/state lookup.
	tx := types.NewTx(&types.LegacyTx{Data: make([]byte, params.MaxBlockSize)})
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1), Time: params.MainnetAntarticalTime}).WithBody(types.Body{Transactions: types.Transactions{tx}})
	validator := &BlockValidator{config: params.MainnetChainConfig}
	if err := validator.ValidateBody(block); !errors.Is(err, ErrBlockOversized) {
		t.Fatalf("oversized Antartical block: %v", err)
	}
	if ShieldedV3MaxTxSize >= params.MaxBlockSize {
		t.Fatal("transaction budget leaves no space for the block header")
	}
}
