package legacypool

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
)

func shieldedV6PoolTestTx(t *testing.T, image [32]byte, nonce uint64) *types.Transaction {
	t.Helper()
	data, err := core.EncodeShieldedV6Transaction(&core.ShieldedV6Transaction{
		Version: 6,
		Inputs:  []core.ShieldedV6Input{{KeyImage: image}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return types.NewTx(&types.PQTkmTx{Nonce: nonce, Data: data})
}

func TestShieldedV6PendingKeyImageReservationLifecycle(t *testing.T) {
	var image [32]byte
	image[0] = 0x42
	txA := shieldedV6PoolTestTx(t, image, 0)
	txB := shieldedV6PoolTestTx(t, image, 1)
	images, err := core.ShieldedV6KeyImages(txA)
	if err != nil || len(images) != 1 || images[0] != image {
		t.Fatalf("extract key images: images=%x err=%v", images, err)
	}

	pool := &LegacyPool{shieldedV6KeyImages: make(map[[32]byte]common.Hash)}
	if err := pool.checkShieldedV6KeyImages(images, common.Hash{}); err != nil {
		t.Fatalf("unreserved key image rejected: %v", err)
	}
	pool.reserveShieldedV6KeyImages(images, txA.Hash())
	if err := pool.checkShieldedV6KeyImages(images, common.Hash{}); !errors.Is(err, ErrShieldedKeyImagePending) {
		t.Fatalf("conflicting pending spend error = %v, want %v", err, ErrShieldedKeyImagePending)
	}
	if err := pool.checkShieldedV6KeyImages(images, txA.Hash()); err != nil {
		t.Fatalf("same-nonce replacement reservation rejected: %v", err)
	}

	pool.reserveShieldedV6KeyImages(images, txB.Hash())
	pool.releaseShieldedV6KeyImages(txA)
	if owner := pool.shieldedV6KeyImages[image]; owner != txB.Hash() {
		t.Fatalf("releasing replaced transaction erased newer reservation: owner=%s want=%s", owner, txB.Hash())
	}
	pool.releaseShieldedV6KeyImages(txB)
	if _, exists := pool.shieldedV6KeyImages[image]; exists {
		t.Fatal("removing pending spend did not release its key image")
	}
	if err := pool.checkShieldedV6KeyImages(images, common.Hash{}); err != nil {
		t.Fatalf("released key image remains reserved: %v", err)
	}
}

func TestShieldedV6KeyImageExtractionIgnoresOtherTransactions(t *testing.T) {
	images, err := core.ShieldedV6KeyImages(types.NewTransaction(0, common.Address{}, nil, 21000, nil, nil))
	if err != nil || len(images) != 0 {
		t.Fatalf("non-Shield6 transaction extraction = %x, %v", images, err)
	}
}
