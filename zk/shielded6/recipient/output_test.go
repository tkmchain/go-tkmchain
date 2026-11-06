package recipient

import (
	"bytes"
	"errors"
	"io"
	"math/big"
	"testing"

	monerocrypto "git.gammaspectra.live/P2Pool/consensus/v5/monero/crypto"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func TestRecipientOutputEntropyAndScalarFailures(t *testing.T) {
	keys, err := shielded6.DeriveWalletKeys(bytes.Repeat([]byte{0x72}, 32), 18979)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.Clear()
	rng := monerocrypto.NewDeterministicTestGenerator()
	amount := big.NewInt(1)
	_, _, w, err := shielded6.ProveRangeProof128WithWitness([]*big.Int{amount}, rng)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { clear(w[0].Mask[:]); clear(w[0].LimbMasks[0][:]); clear(w[0].LimbMasks[1][:]) }()
	if _, secret, err := CreateOutput(keys.Address(), 0, amount, w[0], bytes.NewReader(nil)); !errors.Is(err, io.EOF) || secret != ([32]byte{}) {
		t.Fatalf("entropy failure returned key material: %v", err)
	}
	if _, secret, err := CreateOutput(keys.Address(), 0, amount, w[0], bytes.NewReader(make([]byte, maxEphemeralAttempts*64))); !errors.Is(err, ErrNoBoundedTweak) || secret != ([32]byte{}) {
		t.Fatalf("zero entropy scalar was accepted: %v", err)
	}
	for _, scalar := range [][32]byte{{}, {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}} {
		if _, err := shielded6.CreateOutputWithEphemeral(keys.Address(), 0, shielded6.AssetTKM, amount, w[0], scalar, rng); err == nil {
			t.Fatal("accepted invalid explicit ephemeral scalar")
		}
	}
}
