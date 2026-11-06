package params

import (
	"crypto/sha256"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestShield6RecipientForkPinsAndCompatibility(t *testing.T) {
	c := *MainnetChainConfig
	zero, activation := MainnetAntarticalTime-1, MainnetAntarticalTime+3600
	c.Shield6Time = &activation
	c.Shield6RecipientTime = &activation
	c.Shield6RecipientKey = nil
	if c.CheckConfigForkOrder() == nil {
		t.Fatal("recipient fork without key pin accepted")
	}
	c.Shield6RecipientKey = []byte{1, 2, 3} // Shape/cryptography is separately checked by the verifier.
	c.Shield6RecipientKeyHash = common.Hash(sha256.Sum256(c.Shield6RecipientKey))
	c.Shield6RecipientProvingKeyHash = common.Hash{1}
	c.Shield6RegistrationProvingKeyHash = common.Hash{2}
	c.Shield6RegistrationKey = []byte{7, 8, 9}
	c.Shield6RegistrationKeyHash = common.Hash(sha256.Sum256(c.Shield6RegistrationKey))
	if err := c.CheckConfigForkOrder(); err != nil {
		t.Fatal(err)
	}
	// Existing deployments can have confidential backing before recipient
	// authorization. The new fork must not invalidate that earlier schedule.
	staggered := c
	staggered.Shield6Time = new(uint64)
	*staggered.Shield6Time = MainnetAntarticalTime
	staggered.ConfidentialLedgerTime = staggered.Shield6Time
	if err := staggered.CheckConfigForkOrder(); err != nil {
		t.Fatalf("recipient fork rejected an already-active confidential ledger: %v", err)
	}
	if c.IsShield6Recipients(big.NewInt(1), activation-1) || !c.IsShield6Recipients(big.NewInt(1), activation) {
		t.Fatal("recipient fork boundary")
	}
	changed := c
	changed.Shield6RecipientKey = []byte{4, 5, 6}
	changed.Shield6RecipientKeyHash = common.Hash(sha256.Sum256(changed.Shield6RecipientKey))
	if c.CheckCompatible(&changed, 1, activation) == nil {
		t.Fatal("active recipient key changed without rewind")
	}
	changed = c
	changed.Shield6RecipientTime = &zero
	if changed.CheckConfigForkOrder() == nil {
		t.Fatal("recipient fork before Shield6 accepted")
	}
}
