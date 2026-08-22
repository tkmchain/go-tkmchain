package eth

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

func newTestPrivacyAPI() (*PrivacyAPI, *Ethereum) {
	eth := &Ethereum{
		chainDb:            rawdb.NewMemoryDatabase(),
		privacyDb:          rawdb.NewMemoryDatabase(),
		mainKingAddress:    params.EgyptChainConfig.MainKingAddress,
		privacyActivations: make(map[common.Address]privacyActivationInfo),
		privacyCommitments: make(map[common.Hash]privacyCommitmentInfo),
	}
	return NewPrivacyAPI(eth), eth
}

func TestPrivacyStatusActivatesAfterFiveConfirmations(t *testing.T) {
	address := common.HexToAddress("0x0000000000000000000000000000000000000001")
	info := privacyActivationInfo{
		PaymentHash:    common.HexToHash("0x1234"),
		PaidHeight:     1,
		ActivateHeight: 6,
		Amount:         privacyActivationFee,
	}

	status := privacyStatusAtHead(address, info, 6)
	if !status.Active {
		t.Fatalf("privacy active = false at height 6")
	}
	if status.Confirmations != 6 {
		t.Fatalf("confirmations = %d, want 6", status.Confirmations)
	}
}

func TestPrivacyStatusPendingBeforeFiveConfirmations(t *testing.T) {
	address := common.HexToAddress("0x0000000000000000000000000000000000000001")
	info := privacyActivationInfo{
		PaymentHash:    common.HexToHash("0x1234"),
		PaidHeight:     1,
		ActivateHeight: 6,
		Amount:         privacyActivationFee,
	}

	status := privacyStatusAtHead(address, info, 4)
	if status.Active {
		t.Fatalf("privacy active = true before activation height")
	}
	if status.Confirmations != 4 {
		t.Fatalf("confirmations = %d, want 4", status.Confirmations)
	}
}

func TestPrivacyMainKingAuditRequiresSignature(t *testing.T) {
	api, eth := newTestPrivacyAPI()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	eth.mainKingAddress = crypto.PubkeyToAddress(key.PublicKey)
	address := common.HexToAddress("0x0000000000000000000000000000000000000001")
	eth.privacyActivations[address] = privacyActivationInfo{
		PaymentHash:    common.HexToHash("0x1234"),
		PaidHeight:     1,
		ActivateHeight: 6,
		Amount:         new(big.Int).Set(privacyActivationFee),
	}

	sig, err := crypto.Sign(api.AuditSigningHash().Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := api.MainKingAudit(sig)
	if err != nil {
		t.Fatalf("main king audit failed: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Address != address {
		t.Fatalf("audit statuses = %+v, want one record for %s", statuses, address.Hex())
	}
	if _, err := api.MainKingAudit(make([]byte, crypto.SignatureLength)); err == nil {
		t.Fatal("audit accepted invalid signature")
	}
}

func TestPrivacyActivationRecordsPersistSorted(t *testing.T) {
	_, eth := newTestPrivacyAPI()
	first := common.HexToAddress("0x0000000000000000000000000000000000000001")
	second := common.HexToAddress("0x0000000000000000000000000000000000000002")
	eth.privacyActivations[second] = privacyActivationInfo{PaymentHash: common.HexToHash("0x2"), PaidHeight: 20, ActivateHeight: 25, Amount: privacyActivationFee}
	eth.privacyActivations[first] = privacyActivationInfo{PaymentHash: common.HexToHash("0x1"), PaidHeight: 10, ActivateHeight: 15, Amount: privacyActivationFee}
	eth.persistPrivacyActivationsLocked()

	records := rawdb.ReadPrivacyActivations(eth.privacyDb)
	if len(records) != 2 {
		t.Fatalf("persisted privacy activation count = %d, want 2", len(records))
	}
	if records[0].Address != first || records[1].Address != second {
		t.Fatalf("persisted privacy activation order = %+v", records)
	}
}

func TestPrivacyCommitmentStatusActivatesAfterFiveConfirmations(t *testing.T) {
	commitment := common.HexToHash("0x1234")
	info := privacyCommitmentInfo{
		EncryptedPayload: []byte("ciphertext"),
		Nonce:            []byte("nonce"),
		PaidHeight:       1,
		ActivateHeight:   6,
		Amount:           privacyActivationFee,
		PayloadHash:      common.HexToHash("0x5678"),
		EphemeralPubKey:  []byte("ephemeral"),
		ViewTag:          []byte{0x7},
	}

	status := privacyCommitmentStatusAtHead(commitment, info, 6)
	if !status.Active {
		t.Fatalf("commitment active = false at height 6")
	}
	if status.Confirmations != 6 {
		t.Fatalf("confirmations = %d, want 6", status.Confirmations)
	}
	if string(status.EncryptedPayload) != "ciphertext" {
		t.Fatalf("encrypted payload = %q", status.EncryptedPayload)
	}
	if status.PayloadHash != info.PayloadHash || string(status.EphemeralPubKey) != "ephemeral" || len(status.ViewTag) != 1 {
		t.Fatalf("shielded note metadata = %+v", status)
	}
}

func TestPrivacyCommitmentRecordsPersistSorted(t *testing.T) {
	_, eth := newTestPrivacyAPI()
	first := common.HexToHash("0x1")
	second := common.HexToHash("0x2")
	eth.privacyCommitments[second] = privacyCommitmentInfo{EncryptedPayload: []byte("second"), Nonce: []byte("n2"), PaidHeight: 20, ActivateHeight: 25, Amount: privacyActivationFee}
	eth.privacyCommitments[first] = privacyCommitmentInfo{EncryptedPayload: []byte("first"), Nonce: []byte("n1"), PaidHeight: 10, ActivateHeight: 15, Amount: privacyActivationFee}
	eth.persistPrivacyCommitmentsLocked()

	records := rawdb.ReadPrivacyCommitments(eth.privacyDb)
	if len(records) != 2 {
		t.Fatalf("persisted privacy commitment count = %d, want 2", len(records))
	}
	if records[0].Commitment != first || records[1].Commitment != second {
		t.Fatalf("persisted privacy commitment order = %+v", records)
	}
	if string(records[0].EncryptedPayload) != "first" || string(records[0].Nonce) != "n1" {
		t.Fatalf("persisted privacy commitment payload = %+v", records[0])
	}
}

func TestPrivacyDefaultsPreferCommitments(t *testing.T) {
	api, _ := newTestPrivacyAPI()
	if got := uint64(api.ShieldedGasSponsorActivationTime()); got != params.MainnetShieldedGasSponsorTime {
		t.Fatalf("gas sponsor activation time = %d, want %d", got, params.MainnetShieldedGasSponsorTime)
	}
	if api.ShieldedGasSponsorActive() {
		t.Fatal("gas sponsorship active without a canonical blockchain")
	}
	defaults := api.Defaults()
	if !defaults.CommitmentMode || defaults.LegacyAddressRegistry {
		t.Fatalf("privacy defaults = %+v, want commitment mode without legacy registry", defaults)
	}
	if !defaults.Nullifiers {
		t.Fatalf("privacy defaults nullifiers = false")
	}
}

func TestPrivacyNullifierStatus(t *testing.T) {
	api, eth := newTestPrivacyAPI()
	nullifier := common.HexToHash("0x99")
	info := privacyNullifierInfo{
		ProofHash:          common.HexToHash("0x77"),
		EncryptedSpendData: []byte("spend"),
		SpentHeight:        20,
	}
	eth.privacyNullifiers = map[common.Hash]privacyNullifierInfo{nullifier: info}

	status, err := api.NullifierStatus(nullifier)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Spent || status.ProofHash != info.ProofHash || string(status.EncryptedSpendData) != "spend" {
		t.Fatalf("nullifier status = %+v", status)
	}
}
