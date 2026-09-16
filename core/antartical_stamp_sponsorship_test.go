package core

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

func TestAntarticalStampSponsorshipAuthorization(t *testing.T) {
	sponsorSeed, beneficiarySeed := make([]byte, 32), make([]byte, 32)
	beneficiarySeed[0] = 1
	sponsorKey, err := pqcrypto.NewMLDSA87FromSeed(sponsorSeed)
	if err != nil {
		t.Fatal(err)
	}
	beneficiaryKey, err := pqcrypto.NewMLDSA87FromSeed(beneficiarySeed)
	if err != nil {
		t.Fatal(err)
	}
	sponsorPub, beneficiaryPub := pqcrypto.PublicKeyBytes(sponsorKey), pqcrypto.PublicKeyBytes(beneficiaryKey)
	sponsor, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, sponsorPub)
	if err != nil {
		t.Fatal(err)
	}
	beneficiary, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, beneficiaryPub)
	if err != nil {
		t.Fatal(err)
	}
	stamp, err := pqcrypto.CreateShieldedV3Stamp(beneficiarySeed, 8979, "Private Name", "Private Country")
	if err != nil {
		t.Fatal(err)
	}
	// Canonical field-word encoding is sufficient for stateless tests. This is
	// deliberately not a valid STARK and must fail native consensus processing.
	e := &AntarticalStampRegistration{Version: 1, Owner: shielded3.Digest{1}, Stamp: *stamp, Proof: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, BeneficiaryPublicKey: beneficiaryPub, ValidUntil: params.MainnetAntarticalTime + 3600}
	makeTx := func(envelope *AntarticalStampRegistration, nonce uint64, pub []byte) *types.Transaction {
		t.Helper()
		data, err := EncodeAntarticalStamp(envelope)
		if err != nil {
			t.Fatal(err)
		}
		return types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), Nonce: nonce, Gas: 4_000_000, GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1), To: &params.ShieldedPoolAddress, Value: new(big.Int), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pub, Data: data})
	}
	intent, err := AntarticalStampIntent(makeTx(e, 3, sponsorPub), e)
	if err != nil {
		t.Fatal(err)
	}
	e.Authorization, err = pqcrypto.SignMLDSA87(beneficiaryKey, intent[:])
	if err != nil {
		t.Fatal(err)
	}
	tx := makeTx(e, 3, sponsorPub)
	if err := ValidateAntarticalStampBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		time  uint64
		nonce uint64
		pub   []byte
	}{
		{"prefork", params.MainnetAntarticalTime - 1, 3, sponsorPub},
		{"expired", e.ValidUntil + 1, 3, sponsorPub},
		{"expiry-boundary", e.ValidUntil, 3, sponsorPub},
		{"nonce-replay", params.MainnetAntarticalTime, 4, sponsorPub},
		{"sponsor-substitution", params.MainnetAntarticalTime, 3, beneficiaryPub},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateAntarticalStampBasics(params.MainnetChainConfig, big.NewInt(1), test.time, makeTx(e, test.nonce, test.pub)); err == nil {
				t.Fatal("invalid sponsorship accepted")
			}
		})
	}

	for _, mutation := range []string{"missing-authorization", "expiry-extension", "missing-beneficiary"} {
		t.Run(mutation, func(t *testing.T) {
			data, err := EncodeAntarticalStamp(e)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := DecodeAntarticalStamp(data)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "missing-authorization":
				changed.Authorization = nil
			case "expiry-extension":
				changed.ValidUntil++
			case "missing-beneficiary":
				changed.BeneficiaryPublicKey = nil
			}
			if err := ValidateAntarticalStampBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, makeTx(changed, 3, sponsorPub)); err == nil {
				t.Fatal("incomplete or extended sponsorship accepted")
			}
		})
	}
	st, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAntarticalStampRegistrationState(st, sponsor, tx.Data(), params.MainnetAntarticalTime); err == nil {
		t.Fatal("unstamped sponsor accepted")
	}
	sponsorHash := common.HexToHash("0x1234")
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", sponsor.Bytes()), sponsorHash)
	if err := ValidateAntarticalStampRegistrationState(st, sponsor, tx.Data(), params.MainnetAntarticalTime); err != nil {
		t.Fatal("existing sponsor stamp blocked beneficiary registration", err)
	}
	signed, err := types.SignPQTkmTx(tx, types.NewQuantumSigner(big.NewInt(8979)), sponsorKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProcessAntarticalStamp(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, st, signed); err == nil {
		t.Fatal("fake owner proof registered beneficiary")
	}
	if IsAntarticalStamped(st, beneficiary) || st.GetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", sponsor.Bytes())) != sponsorHash {
		t.Fatal("failed sponsorship modified stamps")
	}
	st.SetState(params.ShieldedPoolAddress, ShieldedV3StateSlot("stamp/address", beneficiary.Bytes()), common.HexToHash("0x55"))
	if err := ValidateAntarticalStampRegistrationState(st, sponsor, tx.Data(), params.MainnetAntarticalTime); err == nil {
		t.Fatal("already stamped beneficiary accepted")
	}
}

func TestAntarticalStampSelfFundedEncodingUnchanged(t *testing.T) {
	e := &AntarticalStampRegistration{Version: 1, Owner: shielded3.Digest{1}, Proof: []byte{1}}
	old, err := rlp.EncodeToBytes(struct {
		Version uint64
		Owner   shielded3.Digest
		Stamp   pqcrypto.ShieldedV3StampRecord
		Proof   []byte
	}{e.Version, e.Owner, e.Stamp, e.Proof})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeAntarticalStamp(e)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, append([]byte(AntarticalStampMagic), old...)) {
		t.Fatal("self-funded registration encoding changed")
	}
	e.ValidUntil = 1
	data, err := EncodeAntarticalStamp(e)
	if err != nil {
		t.Fatal(err)
	}
	tx := types.NewTx(&types.PQTkmTx{ChainID: big.NewInt(8979), To: &params.ShieldedPoolAddress, Value: new(big.Int), Data: data})
	if err := ValidateAntarticalStampBasics(params.MainnetChainConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err == nil {
		t.Fatal("partial sponsorship treated as self funded")
	}
}
