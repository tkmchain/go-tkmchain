package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestAccountAbstractionEnvelopeRoundTrip(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	op := &antartical.UserOperation{
		Sender: crypto.PubkeyToAddress(key.PublicKey), Nonce: new(big.Int),
		CallData:     append(common.HexToAddress("0x100").Bytes(), []byte{0x01, 0x02}...),
		CallGasLimit: big.NewInt(100000), VerificationGasLimit: big.NewInt(50000),
		PreVerificationGas: big.NewInt(1000), MaxFeePerGas: big.NewInt(10), MaxPriorityFeePerGas: big.NewInt(1),
	}
	digest, err := op.Hash(big.NewInt(8979), common.HexToAddress("0x0000000000000000000000000000000000004337"))
	if err != nil {
		t.Fatal(err)
	}
	op.Signature, err = crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeAccountAbstraction(op)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeAccountAbstraction(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := decoded.VerifySignature(big.NewInt(8979), common.HexToAddress("0x0000000000000000000000000000000000004337")); err != nil {
		t.Fatal(err)
	}
}
