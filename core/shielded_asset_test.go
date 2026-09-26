package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestShieldedAssetEnvelopeBindsPTKM(t *testing.T) {
	commitment := shielded3.Digest{1, 2, 3, 4, 5}
	e := &ShieldedV3Transaction{
		Version:         3,
		AssetID:         shielded3.AssetPTKM,
		Deposit:         true,
		WithdrawalValue: new(big.Int),
		GasSponsorValue: new(big.Int),
	}
	for i := range e.Outputs {
		e.Outputs[i].Commitment = commitment
		e.Outputs[i].OneTimeKey = commitment.Bytes()
	}
	tx := types.NewTx(&types.PQTkmTx{
		ChainID:   big.NewInt(8980),
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       7_000_000,
		To:        func() *common.Address { a := params.ShieldedPoolAddress; return &a }(),
		Value:     big.NewInt(1),
		Algorithm: pqcrypto.AlgorithmMLDSA87,
	})
	statement, err := ShieldedV3Statement(tx, e)
	if err != nil {
		t.Fatal(err)
	}
	if statement.AssetID != shielded3.AssetPTKM {
		t.Fatalf("statement asset = %d, want %d", statement.AssetID, shielded3.AssetPTKM)
	}
	raw, err := EncodeShieldedV3Transaction(e)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok, err := DecodeShieldedV3Transaction(raw)
	if err != nil || !ok {
		t.Fatalf("decode pTKM envelope: ok=%v err=%v", ok, err)
	}
	if decoded.AssetID != shielded3.AssetPTKM {
		t.Fatalf("decoded asset = %d, want %d", decoded.AssetID, shielded3.AssetPTKM)
	}
}

func TestShieldedAssetStateNamespaces(t *testing.T) {
	key := []byte("same-nullifier")
	if ShieldedV3StateSlotForAsset(shielded3.AssetTKM, "nullifier", key) == ShieldedV3StateSlotForAsset(shielded3.AssetPTKM, "nullifier", key) {
		t.Fatal("native and wrapped nullifier namespaces collide")
	}
	if ShieldedV3StateSlotForAsset(shielded3.AssetTKM, "supply", nil) == ShieldedV3StateSlotForAsset(shielded3.AssetPTKM, "supply", nil) {
		t.Fatal("native and wrapped supply namespaces collide")
	}
}
