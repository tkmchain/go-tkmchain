package verkle

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestWitnessPacketRoundTrip(t *testing.T) {
	witness := &stateless.ExtWitness{Headers: []*types.Header{{Number: big.NewInt(10), Root: common.HexToHash("0x1234")}}, Codes: nil, State: nil, Keys: nil}
	raw, err := rlp.EncodeToBytes(witness)
	if err != nil {
		t.Fatal(err)
	}
	want := &WitnessPacket{ID: 7, BlockHash: common.HexToHash("0xab"), StateRoot: common.HexToHash("0x1234"), Witness: raw}
	encoded, err := rlp.EncodeToBytes(want)
	if err != nil {
		t.Fatal(err)
	}
	var got WitnessPacket
	if err := rlp.DecodeBytes(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.BlockHash != want.BlockHash || got.StateRoot != want.StateRoot || string(got.Witness) != string(want.Witness) {
		t.Fatalf("witness packet mismatch: got=%+v want=%+v", got, *want)
	}
	var decoded stateless.Witness
	if err := rlp.DecodeBytes(got.Witness, &decoded); err != nil || len(decoded.Headers) != 1 || decoded.Headers[0].Root != got.StateRoot {
		t.Fatalf("invalid embedded witness: %v", err)
	}
}
