package core

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestPrivateTVMEnvelopeRoundTripAndProofStripping(t *testing.T) {
	e := &PrivateTVMTransaction{
		Version:    1,
		CodeHash:   shielded3.Digest{1, 2, 3, 4, 5},
		OldRoot:    shielded3.Digest{6, 7, 8, 9, 10},
		NewRoot:    shielded3.Digest{11, 12, 13, 14, 15},
		Operation:  1,
		Ciphertext: []byte{0xaa, 0xbb},
		Proof:      bytes.Repeat([]byte{0x01}, 16),
	}
	data, err := EncodePrivateTVMTransaction(e)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok, err := DecodePrivateTVMTransaction(data)
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if decoded.Version != e.Version || !bytes.Equal(decoded.Ciphertext, e.Ciphertext) || !bytes.Equal(decoded.Proof, e.Proof) {
		t.Fatalf("round trip changed envelope: %#v", decoded)
	}

	to := params.ShieldedPoolAddress
	tx := types.NewTx(&types.PQTkmTx{
		ChainID: params.MainnetChainConfig.ChainID, Nonce: 1, Gas: 4_000_000,
		To: &to, Value: new(big.Int), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1),
		Algorithm: "ML-DSA-87", PublicKey: []byte{1}, Signature: []byte{2}, Data: data,
	})
	if _, err := PrivateTVMIntent(tx, e); err != nil {
		t.Fatalf("intent: %v", err)
	}
	clean, gas, err := PrivateTVMGasData(data)
	if err != nil {
		t.Fatalf("gas data: %v", err)
	}
	if gas != PrivateTVMVerifyGas+uint64(len(e.Proof)) {
		t.Fatalf("gas=%d want %d", gas, PrivateTVMVerifyGas+uint64(len(e.Proof)))
	}
	cleanEnvelope, ok, err := DecodePrivateTVMTransaction(clean)
	if err != nil || !ok || len(cleanEnvelope.Proof) != 0 {
		t.Fatalf("proof was not stripped: ok=%v err=%v", ok, err)
	}
	if cleanEnvelope.CodeHash == (shielded3.Digest{}) || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress {
		t.Fatal("unexpected cleaned envelope")
	}
}
