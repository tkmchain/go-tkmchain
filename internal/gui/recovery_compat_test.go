package gui

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestBrowserRecoveryMatchesGoPQIdentity(t *testing.T) {
	var fixture struct{ Address, PublicKey, PaymentCode string }
	data, err := os.ReadFile("wallet-engine/testdata/pq-identity.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	seed := bytes.Repeat([]byte{7}, 32)
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	pub := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address("ML-DSA-87", pub)
	if err != nil {
		t.Fatal(err)
	}
	code, err := pqcrypto.ShieldedPaymentCode(seed, big.NewInt(8979), address)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(address.Hex(), fixture.Address) || hex.EncodeToString(pub) != fixture.PublicKey || code != fixture.PaymentCode {
		t.Fatal("browser recovery differs from native PQ identity")
	}
}
