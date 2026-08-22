// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"
)

func TestTKMChainIDs(t *testing.T) {
	if got := RandomXChainConfig.ChainID.Int64(); got != TKMMainnetChainID {
		t.Fatalf("RandomX chain ID = %d, want %d", got, TKMMainnetChainID)
	}
	if got := MainnetChainConfig.ChainID.Int64(); got != TKMMainnetChainID {
		t.Fatalf("mainnet chain ID = %d, want %d", got, TKMMainnetChainID)
	}
}

func TestCheckCompatible(t *testing.T) {
	type test struct {
		stored, new   *ChainConfig
		headBlock     uint64
		headTimestamp uint64
		wantErr       *ConfigCompatError
	}
	tests := []test{
		{stored: AllRandomXProtocolChanges, new: AllRandomXProtocolChanges, headBlock: 0, headTimestamp: 0, wantErr: nil},
		{stored: AllRandomXProtocolChanges, new: AllRandomXProtocolChanges, headBlock: 0, headTimestamp: uint64(time.Now().Unix()), wantErr: nil},
		{stored: AllRandomXProtocolChanges, new: AllRandomXProtocolChanges, headBlock: 100, wantErr: nil},
		{
			stored:    &ChainConfig{EIP150Block: big.NewInt(10)},
			new:       &ChainConfig{EIP150Block: big.NewInt(20)},
			headBlock: 9,
			wantErr:   nil,
		},
		{
			stored:    AllRandomXProtocolChanges,
			new:       &ChainConfig{HomesteadBlock: nil},
			headBlock: 3,
			wantErr: &ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      nil,
				RewindToBlock: 0,
			},
		},
		{
			stored:    AllRandomXProtocolChanges,
			new:       &ChainConfig{HomesteadBlock: big.NewInt(1)},
			headBlock: 3,
			wantErr: &ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      big.NewInt(1),
				RewindToBlock: 0,
			},
		},
		{
			stored:    &ChainConfig{HomesteadBlock: big.NewInt(30), EIP150Block: big.NewInt(10)},
			new:       &ChainConfig{HomesteadBlock: big.NewInt(25), EIP150Block: big.NewInt(20)},
			headBlock: 25,
			wantErr: &ConfigCompatError{
				What:          "EIP150 fork block",
				StoredBlock:   big.NewInt(10),
				NewBlock:      big.NewInt(20),
				RewindToBlock: 9,
			},
		},
		{
			stored:    &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:       &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(30)},
			headBlock: 40,
			wantErr:   nil,
		},
		{
			stored:    &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:       &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(31)},
			headBlock: 40,
			wantErr: &ConfigCompatError{
				What:          "Petersburg fork block",
				StoredBlock:   nil,
				NewBlock:      big.NewInt(31),
				RewindToBlock: 30,
			},
		},
		{
			stored:        &ChainConfig{ShanghaiTime: newUint64(10)},
			new:           &ChainConfig{ShanghaiTime: newUint64(20)},
			headTimestamp: 9,
			wantErr:       nil,
		},
		{
			stored:        &ChainConfig{ShanghaiTime: newUint64(10)},
			new:           &ChainConfig{ShanghaiTime: newUint64(20)},
			headTimestamp: 25,
			wantErr: &ConfigCompatError{
				What:         "Shanghai fork timestamp",
				StoredTime:   newUint64(10),
				NewTime:      newUint64(20),
				RewindToTime: 9,
			},
		},
	}

	for _, test := range tests {
		err := test.stored.CheckCompatible(test.new, test.headBlock, test.headTimestamp)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nnew: %v\nheadBlock: %v\nheadTimestamp: %v\nerr: %v\nwant: %v", test.stored, test.new, test.headBlock, test.headTimestamp, err, test.wantErr)
		}
	}
}

func TestConfigRules(t *testing.T) {
	c := &ChainConfig{
		LondonBlock:  new(big.Int),
		ShanghaiTime: newUint64(500),
		PhoneTime:    newUint64(600),
	}
	var stamp uint64
	if r := c.Rules(big.NewInt(0), true, stamp); r.IsShanghai {
		t.Errorf("expected %v to not be shanghai", stamp)
	}
	stamp = 500
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsShanghai {
		t.Errorf("expected %v to be shanghai", stamp)
	}
	stamp = math.MaxInt64
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsShanghai {
		t.Errorf("expected %v to be shanghai", stamp)
	}
	stamp = 599
	if r := c.Rules(big.NewInt(0), true, stamp); r.IsPhone {
		t.Errorf("expected %v to not be phone", stamp)
	}
	stamp = 600
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsPhone {
		t.Errorf("expected %v to be phone", stamp)
	}
}

func TestPhoneTimestampCompatError(t *testing.T) {
	stored := &ChainConfig{LondonBlock: new(big.Int), PhoneTime: newUint64(600)}
	updated := &ChainConfig{LondonBlock: new(big.Int), PhoneTime: newUint64(700)}
	if err := stored.CheckCompatible(updated, 0, 599); err != nil {
		t.Fatalf("pre-phone fork config should be compatible: %v", err)
	}
	err := stored.CheckCompatible(updated, 0, 650)
	if err == nil || err.What != "Phone fork timestamp" {
		t.Fatalf("expected phone timestamp compatibility error, got %v", err)
	}
}

func TestMainnetHistoricalForkTimestamps(t *testing.T) {
	configs := map[string]*ChainConfig{
		"randomx": RandomXChainConfig,
		"mainnet": MainnetChainConfig,
	}
	for name, config := range configs {
		t.Run(name, func(t *testing.T) {
			tkmForkTimes := map[string]*uint64{
				"EDATime":                 config.EDATime,
				"KyotoTime":               config.KyotoTime,
				"PhoneTime":               config.PhoneTime,
				"PrivacyCommitmentTime":   config.PrivacyCommitmentTime,
				"QuantumResistantTime":    config.QuantumResistantTime,
				"PQMigrationRecoveryTime": config.PQMigrationRecoveryTime,
			}
			want := map[string]uint64{
				"EDATime":                 0,
				"KyotoTime":               MainnetKyotoTime,
				"PhoneTime":               MainnetPhoneTime,
				"PrivacyCommitmentTime":   MainnetPrivacyQuantumTime,
				"QuantumResistantTime":    MainnetPrivacyQuantumTime,
				"PQMigrationRecoveryTime": MainnetPQMigrationRecoveryTime,
			}
			for forkName, wantTime := range want {
				forkTime := tkmForkTimes[forkName]
				if forkTime == nil || *forkTime != wantTime {
					t.Fatalf("%s = %v, want %d", forkName, forkTime, wantTime)
				}
			}
		})
	}
}

func TestRandomXMoneroForkSchedule(t *testing.T) {
	for name, config := range map[string]*ChainConfig{
		"randomx": RandomXChainConfig,
		"mainnet": MainnetChainConfig,
	} {
		t.Run(name, func(t *testing.T) {
			if config.IsRandomXMonero(new(big.Int).SetUint64(MainnetRandomXMoneroBlock - 1)) {
				t.Fatal("RandomX Monero proof rules active before the fork")
			}
			if !config.IsRandomXMonero(new(big.Int).SetUint64(MainnetRandomXMoneroBlock)) {
				t.Fatal("RandomX Monero proof rules inactive at the fork")
			}
		})
	}
	if !EgyptChainConfig.IsRandomXMonero(big.NewInt(0)) {
		t.Fatal("Egypt RandomX Monero proof rules are not active at genesis")
	}
}

func TestPrivacyCommitmentForkSchedule(t *testing.T) {
	if !EgyptChainConfig.IsPrivacyCommitments(big.NewInt(0), 0) {
		t.Fatal("Egypt privacy commitments are not active at genesis")
	}
	if MainnetChainConfig.PrivacyCommitmentTime == nil || *MainnetChainConfig.PrivacyCommitmentTime != MainnetPrivacyQuantumTime {
		t.Fatalf("mainnet privacy commitment time = %v, want %d", MainnetChainConfig.PrivacyCommitmentTime, MainnetPrivacyQuantumTime)
	}
	if MainnetChainConfig.IsPrivacyCommitments(big.NewInt(0), MainnetPrivacyQuantumTime-1) {
		t.Fatal("mainnet privacy commitments active before scheduled timestamp")
	}
	if !MainnetChainConfig.IsPrivacyCommitments(big.NewInt(0), MainnetPrivacyQuantumTime) {
		t.Fatal("mainnet privacy commitments inactive at scheduled timestamp")
	}
}

func TestShieldedGasSponsorForkSchedule(t *testing.T) {
	if IsMainnetShieldedGasSponsor(MainnetChainConfig, MainnetShieldedGasSponsorTime-1) {
		t.Fatal("shielded gas sponsorship active before scheduled timestamp")
	}
	if !IsMainnetShieldedGasSponsor(MainnetChainConfig, MainnetShieldedGasSponsorTime) {
		t.Fatal("shielded gas sponsorship inactive at scheduled timestamp")
	}
	if IsMainnetShieldedGasSponsor(EgyptChainConfig, MainnetShieldedGasSponsorTime) {
		t.Fatal("mainnet shielded gas sponsorship enabled for a different chain")
	}
}

func TestQuantumResistantForkSchedule(t *testing.T) {
	if !EgyptChainConfig.IsQuantumResistant(big.NewInt(0), 0) {
		t.Fatal("Egypt quantum-resistant transactions are not active at genesis")
	}
	if MainnetChainConfig.QuantumResistantTime == nil || *MainnetChainConfig.QuantumResistantTime != MainnetPrivacyQuantumTime {
		t.Fatalf("mainnet quantum-resistant time = %v, want %d", MainnetChainConfig.QuantumResistantTime, MainnetPrivacyQuantumTime)
	}
	if MainnetChainConfig.IsQuantumResistant(big.NewInt(0), MainnetPrivacyQuantumTime-1) {
		t.Fatal("mainnet quantum-resistant transactions active before scheduled timestamp")
	}
	if !MainnetChainConfig.IsQuantumResistant(big.NewInt(0), MainnetPrivacyQuantumTime) {
		t.Fatal("mainnet quantum-resistant transactions inactive at scheduled timestamp")
	}
}

func TestPQMigrationRecoveryForkSchedule(t *testing.T) {
	if !EgyptChainConfig.IsPQMigrationAllowed(big.NewInt(0), 0) {
		t.Fatal("Egypt PQ migration recovery is not active at genesis")
	}
	if MainnetChainConfig.PQMigrationRecoveryTime == nil || *MainnetChainConfig.PQMigrationRecoveryTime != MainnetPQMigrationRecoveryTime {
		t.Fatalf("mainnet PQ migration recovery time = %v, want %d", MainnetChainConfig.PQMigrationRecoveryTime, MainnetPQMigrationRecoveryTime)
	}
	if !MainnetChainConfig.IsPQMigrationAllowed(big.NewInt(0), MainnetPrivacyQuantumTime-1) {
		t.Fatal("mainnet PQ migration should be available before the PQ-only fork")
	}
	if MainnetChainConfig.IsPQMigrationAllowed(big.NewInt(0), MainnetPrivacyQuantumTime) {
		t.Fatal("mainnet PQ migration recovery active before its scheduled timestamp")
	}
	if !MainnetChainConfig.IsPQMigrationAllowed(big.NewInt(0), MainnetPQMigrationRecoveryTime) {
		t.Fatal("mainnet PQ migration recovery inactive at its scheduled timestamp")
	}
}

func TestEgyptMainKingPQAddress(t *testing.T) {
	legacy := common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2")
	pq := common.HexToAddress("0x095943648A687DA264c3c49993b8B4aa4fF5aC2b")
	if EgyptChainConfig.MainKingAddress != legacy {
		t.Fatalf("Egypt legacy main king address = %s, want %s", EgyptChainConfig.MainKingAddress, legacy)
	}
	if EgyptChainConfig.PostQuantumMainKingAddress != pq {
		t.Fatalf("Egypt PQ main king address = %s, want %s", EgyptChainConfig.PostQuantumMainKingAddress, pq)
	}
	if got := EgyptChainConfig.MainKingAddressAt(big.NewInt(0), 0); got != pq {
		t.Fatalf("Egypt active main king address = %s, want %s", got, pq)
	}
}

func TestMainnetMainKingPQAddress(t *testing.T) {
	legacy := common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2")
	pq := common.HexToAddress("0xb14bBd5BD6E2e7CD74E88931ef439D253Eb6B58f")
	if MainnetChainConfig.MainKingAddress != legacy {
		t.Fatalf("mainnet legacy main king address = %s, want %s", MainnetChainConfig.MainKingAddress, legacy)
	}
	if MainnetChainConfig.PostQuantumMainKingAddress != pq {
		t.Fatalf("mainnet PQ main king address = %s, want %s", MainnetChainConfig.PostQuantumMainKingAddress, pq)
	}
	if got := MainnetChainConfig.MainKingAddressAt(big.NewInt(0), MainnetPrivacyQuantumTime-1); got != legacy {
		t.Fatalf("mainnet pre-fork main king = %s, want %s", got, legacy)
	}
	if got := MainnetChainConfig.MainKingAddressAt(big.NewInt(0), MainnetPrivacyQuantumTime); got != pq {
		t.Fatalf("mainnet post-fork main king = %s, want %s", got, pq)
	}
}

func TestPostQuantumMainKingAddressChangeErrors(t *testing.T) {
	stored := *MainnetChainConfig
	updated := stored
	updated.PostQuantumMainKingAddress = common.HexToAddress("0x0000000000000000000000000000000000000001")
	err := stored.CheckCompatible(&updated, 0, MainnetPrivacyQuantumTime-1)
	if err == nil {
		t.Fatal("post-quantum main king address change accepted")
	}
	if err.What != "post-quantum main king address" {
		t.Fatalf("compat error = %v, want post-quantum main king address", err)
	}
}

func TestMainnetShieldedPrivacyRequiresCeremonyKey(t *testing.T) {
	config := *MainnetChainConfig
	config.ShieldedGroth16VerifyingKey = nil
	err := config.CheckMainnetShieldedPrivacyReady()
	if err == nil {
		t.Fatal("mainnet privacy config accepted without shielded verifying key")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("audited ceremony output")) {
		t.Fatalf("unexpected missing key error: %v", err)
	}
	config.ShieldedGroth16VerifyingKey = []byte("bad")
	if err := config.CheckMainnetShieldedPrivacyReady(); err == nil {
		t.Fatal("mainnet privacy config accepted malformed shielded verifying key")
	}
}

func TestMainnetShieldedPrivacyAcceptsWellFormedCeremonyKeyEnvelope(t *testing.T) {
	config := *MainnetChainConfig
	config.ShieldedGroth16VerifyingKey = testShieldedGroth16VKEnvelope(t)
	if err := config.CheckMainnetShieldedPrivacyReady(); err != nil {
		t.Fatalf("well-formed shielded verifying key envelope rejected: %v", err)
	}
}

func TestMainnetShieldedPrivacyRejectsMalformedCeremonyPoint(t *testing.T) {
	config := *MainnetChainConfig
	config.ShieldedGroth16VerifyingKey = testShieldedGroth16VKEnvelopeWith(t, func(envelope *shieldedGroth16VerifyingKeyEnvelope) {
		envelope.AlphaG1 = []byte{1}
	})
	if err := config.CheckMainnetShieldedPrivacyReady(); err == nil {
		t.Fatal("mainnet privacy config accepted malformed shielded verifying key point")
	}
}

func TestEgyptShieldedPrivacyCanUseDevVerifierKey(t *testing.T) {
	config := *EgyptChainConfig
	config.ShieldedGroth16VerifyingKey = nil
	if err := config.CheckMainnetShieldedPrivacyReady(); err != nil {
		t.Fatalf("Egypt dev privacy config unexpectedly rejected: %v", err)
	}
}

func testShieldedGroth16VKEnvelope(t *testing.T) []byte {
	t.Helper()
	return testShieldedGroth16VKEnvelopeWith(t, nil)
}

func testShieldedGroth16VKEnvelopeWith(t *testing.T, mutate func(*shieldedGroth16VerifyingKeyEnvelope)) []byte {
	t.Helper()
	_, _, g1, g2 := bn254.Generators()
	g1Bytes := g1.Bytes()
	g2Bytes := g2.Bytes()
	ic := make([][]byte, shieldedGroth16PublicInputs+1)
	for i := range ic {
		ic[i] = g1Bytes[:]
	}
	envelope := shieldedGroth16VerifyingKeyEnvelope{
		AlphaG1: g1Bytes[:],
		BetaG2:  g2Bytes[:],
		GammaG2: g2Bytes[:],
		DeltaG2: g2Bytes[:],
		IC:      ic,
	}
	if mutate != nil {
		mutate(&envelope)
	}
	payload, err := rlp.EncodeToBytes(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(shieldedGroth16VKMagic), payload...)
}

func TestTimestampCompatError(t *testing.T) {
	require.Equal(t, new(ConfigCompatError).Error(), "")

	errWhat := "Shanghai fork timestamp"
	require.Equal(t, newTimestampCompatError(errWhat, nil, newUint64(1681338455)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp nil, want timestamp 1681338455, rewindto timestamp 1681338454)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(1681338455), nil).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 1681338455, want timestamp nil, rewindto timestamp 1681338454)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(1681338455), newUint64(600624000)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 1681338455, want timestamp 600624000, rewindto timestamp 600623999)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(0), newUint64(1681338455)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 0, want timestamp 1681338455, rewindto timestamp 0)")
}

func TestDefaultRotatingKingConfigsHaveNoUsableRotatingKing(t *testing.T) {
	configs := map[string]*ChainConfig{
		"randomx": RandomXChainConfig,
		"mainnet": MainnetChainConfig,
		"test":    TestChainConfig,
	}

	for name, config := range configs {
		t.Run(name, func(t *testing.T) {
			if len(config.RotatingKingAddresses) != 0 {
				t.Fatalf("default rotating king addresses = %v, want none", config.RotatingKingAddresses)
			}
		})
	}
}
