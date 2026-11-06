package params

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"testing"
)

func TestShield6ProductionActivation(t *testing.T) {
	for name, c := range map[string]*ChainConfig{"mainnet": MainnetChainConfig, "randomx": RandomXChainConfig} {
		t.Run(name, func(t *testing.T) {
			if c.Shield6Time == nil || c.Shield6RecipientTime == nil || c.ConfidentialLedgerTime == nil ||
				*c.Shield6Time != MainnetShield6Time || *c.Shield6RecipientTime != MainnetShield6Time || *c.ConfidentialLedgerTime != MainnetShield6Time {
				t.Fatal("production forks must activate together")
			}
			if err := c.CheckConfigForkOrder(); err != nil {
				t.Fatal(err)
			}
			for _, at := range []uint64{MainnetShield6Time - 1, MainnetShield6Time, MainnetShield6Time + 1} {
				active := at >= MainnetShield6Time
				if c.IsShield6(big.NewInt(1), at) != active || c.IsShield6Recipients(big.NewInt(1), at) != active || c.IsConfidentialLedger(big.NewInt(1), at) != active {
					t.Fatal("production activation boundary mismatch")
				}
			}
		})
	}
	data, err := os.ReadFile("shield6_production.json")
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		RelationVersion int    `json:"relationVersion"`
		ChainID         int64  `json:"chainId"`
		ActivationUnix  uint64 `json:"activationUnix"`
		Files           map[string]struct {
			SHA256 string `json:"sha256"`
			Size   int    `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.RelationVersion != 2 || metadata.ChainID != TKMMainnetChainID || metadata.ActivationUnix != MainnetShield6Time || len(metadata.Files) != 10 {
		t.Fatal("release metadata differs from production configuration")
	}
	c := MainnetChainConfig
	for name, hash := range map[string][32]byte{
		"recipient.vk": c.Shield6RecipientKeyHash, "registration.vk": c.Shield6RegistrationKeyHash,
		"recipient.pk": c.Shield6RecipientProvingKeyHash, "registration.pk": c.Shield6RegistrationProvingKeyHash,
		"recipient.r1cs": MainnetShield6RecipientCircuitHash, "registration.r1cs": MainnetShield6RegistrationCircuitHash,
	} {
		if hash == ([32]byte{}) || metadata.Files[name].SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatalf("production pin mismatch: %s", name)
		}
	}
	if sha256.Sum256(c.Shield6RecipientKey) != [32]byte(c.Shield6RecipientKeyHash) || sha256.Sum256(c.Shield6RegistrationKey) != [32]byte(c.Shield6RegistrationKeyHash) {
		t.Fatal("production verification-key authentication failed")
	}
}
