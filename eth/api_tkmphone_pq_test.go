package eth

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestPhonePQSignature(t *testing.T) {
	key, err := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	pub := pqcrypto.PublicKeyBytes(key)
	owner, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	digest := common.HexToHash("0x1234")
	sig, err := pqcrypto.SignMLDSA87(key, phonePQMessage(digest))
	if err != nil {
		t.Fatal(err)
	}
	envelope := append(append(append([]byte(nil), phonePQPrefix...), pub...), sig...)
	if err := verifyPhoneAddressSignature(owner, digest, envelope); err != nil {
		t.Fatal(err)
	}
	if got, err := recoverPhoneActionSigner(digest, envelope); err != nil || got != owner {
		t.Fatalf("signer %s: %v", got, err)
	}
	if err := verifyPhoneAddressSignature(common.Address{1}, digest, envelope); err == nil {
		t.Fatal("accepted wrong owner")
	}
	if err := verifyPhoneAddressSignature(owner, common.Hash{1}, envelope); err == nil {
		t.Fatal("accepted wrong action")
	}
	if err := verifyPhoneAddressSignature(owner, digest, envelope[:len(envelope)-1]); err == nil {
		t.Fatal("accepted truncated signature")
	}
	envelope[len(envelope)-1] ^= 1
	if err := verifyPhoneAddressSignature(owner, digest, envelope); err == nil {
		t.Fatal("accepted tampered signature")
	}
	raw, _ := pqcrypto.SignMLDSA87(key, digest.Bytes())
	envelope = append(append(append([]byte(nil), phonePQPrefix...), pub...), raw...)
	if err := verifyPhoneAddressSignature(owner, digest, envelope); err == nil {
		t.Fatal("accepted signature without phone domain")
	}
}

func TestPhonePQBrowserCompatibility(t *testing.T) {
	data, err := os.ReadFile("../internal/gui/wallet-engine/testdata/phone-pq-signature.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Digest    common.Hash   `json:"digest"`
		Signature hexutil.Bytes `json:"signature"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	key, _ := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{7}, 32))
	owner, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqcrypto.PublicKeyBytes(key))
	if err := verifyPhoneAddressSignature(owner, fixture.Digest, fixture.Signature); err != nil {
		t.Fatal(err)
	}
}

func TestPhonePQRegistrationMigrationAndPropagation(t *testing.T) {
	svc := NewTkmPhoneService(nil, common.Address{1}, big.NewInt(8979))
	key, _ := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{7}, 32))
	pub := pqcrypto.PublicKeyBytes(key)
	owner, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	sign := func(hash common.Hash) []byte {
		sig, err := pqcrypto.SignMLDSA87(key, phonePQMessage(hash))
		if err != nil {
			t.Fatal(err)
		}
		return append(append(append([]byte(nil), phonePQPrefix...), pub...), sig...)
	}
	const number = "123456"
	svc.numbers[number] = PhoneNumber{Number: number, Owner: owner, Active: true}
	signature := sign(svc.deviceKeySigningHash(number, "Phone", pub))
	record, err := svc.RegisterDeviceKey(number, "Phone", pub, signature)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Active || len(record.OwnerSignature) == 0 {
		t.Fatal("registration omitted signature")
	}
	if _, err := svc.RegisterDeviceKey(number, "Phone", pub, signature); err != nil {
		t.Fatal(err)
	}
	if len(svc.devices[number]) != 1 {
		t.Fatal("duplicate registration")
	}
	peer := NewTkmPhoneService(nil, common.Address{1}, big.NewInt(8979))
	peer.numbers[number] = svc.numbers[number]
	data, _ := json.Marshal(record)
	prop := PhonePropagation{ID: 1, Kind: "device-key", Payload: data, Hash: svc.randomXServiceHash("device-key", []byte(number), []byte("Phone"), pub)}
	if err := peer.ImportPropagation(prop); err != nil {
		t.Fatal(err)
	}
	record.Device = "forged"
	data, _ = json.Marshal(record)
	prop.ID = 2
	prop.Payload = data
	if err := peer.ImportPropagation(prop); err == nil {
		t.Fatal("accepted forged propagated registration")
	}
	svc.recovery[number] = owner
	newOwner := common.Address{9}
	migrated, err := svc.TransferNumber(number, newOwner, sign(svc.transferNumberSigningHash(number, newOwner)))
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.devices[number]) != 0 || svc.recovery[number] != (common.Address{}) {
		t.Fatal("old authority survived migration")
	}
	data, _ = json.Marshal(migrated)
	prop = PhonePropagation{ID: 3, Kind: "number-transferred", Payload: data, Hash: migrated.TransferHash}
	if err := peer.ImportPropagation(prop); err != nil {
		t.Fatal(err)
	}
	if len(peer.devices[number]) != 0 || peer.numbers[number].Owner != newOwner {
		t.Fatal("peer did not migrate authority")
	}
	if _, err := svc.RegisterDeviceKey(number, "Old", pub, signature); err == nil {
		t.Fatal("old owner retained registration authority")
	}
}

func TestPhoneLegacyOwnerMigratesToPQ(t *testing.T) {
	svc := NewTkmPhoneService(nil, common.Address{1}, big.NewInt(8979))
	legacy, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	owner := crypto.PubkeyToAddress(legacy.PublicKey)
	pq, _ := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{8}, 32))
	pub := pqcrypto.PublicKeyBytes(pq)
	target, _ := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	svc.numbers["123"] = PhoneNumber{Number: "123", Owner: owner, Active: true}
	digest := svc.transferNumberSigningHash("123", target)
	signature, err := crypto.Sign(accounts.TextHash(digest.Bytes()), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TransferNumber("123", target, signature); err != nil {
		t.Fatal(err)
	}
	sig, _ := pqcrypto.SignMLDSA87(pq, phonePQMessage(svc.deviceKeySigningHash("123", "PQ device", pub)))
	envelope := append(append(append([]byte(nil), phonePQPrefix...), pub...), sig...)
	if _, err := svc.RegisterDeviceKey("123", "PQ device", pub, envelope); err != nil {
		t.Fatal(err)
	}
}

func TestPhonePQDeviceSignature(t *testing.T) {
	svc := NewTkmPhoneService(nil, common.Address{1}, big.NewInt(8979))
	device, _ := pqcrypto.NewMLDSA87FromSeed(bytes.Repeat([]byte{8}, 32))
	pub := pqcrypto.PublicKeyBytes(device)
	svc.numbers["123"] = PhoneNumber{Number: "123", Owner: common.Address{9}, Active: true}
	svc.devices["123"] = []PhoneDeviceKey{{Number: "123", Device: "PQ device", PublicKey: pub, Active: true}}
	payload := common.Hash{3}
	digest := svc.ownerActionHash("123", "send-message", payload)
	sig, _ := pqcrypto.SignMLDSA87(device, phonePQMessage(digest))
	envelope := append(append(append([]byte(nil), phonePQPrefix...), pub...), sig...)
	if err := svc.verifyNumberDeviceOrOwnerSignature("123", "send-message", payload, envelope); err != nil {
		t.Fatal(err)
	}
	svc.devices["123"][0].Active = false
	if err := svc.verifyNumberDeviceOrOwnerSignature("123", "send-message", payload, envelope); err == nil {
		t.Fatal("inactive device retained signing authority")
	}
}

func TestPhonePropagationRejectsForgedRecoveryAndPublicImport(t *testing.T) {
	svc := NewTkmPhoneService(nil, common.Address{1}, big.NewInt(8979))
	ownerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	attackerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	owner := crypto.PubkeyToAddress(ownerKey.PublicKey)
	recovery := crypto.PubkeyToAddress(attackerKey.PublicKey)
	const number = "123456"
	svc.numbers[number] = PhoneNumber{Number: number, Owner: owner, Active: true}
	peer := NewTkmPhoneService(nil, common.Address{1}, big.NewInt(8979))
	peer.numbers[number] = svc.numbers[number]
	peer.recovery[number] = recovery

	newOwnerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	newOwner := crypto.PubkeyToAddress(newOwnerKey.PublicKey)
	payload := peer.randomXServiceHash("recover-number-payload", []byte(number), newOwner.Bytes())
	forgedSignature := signTkmPhoneDigest(t, ownerKey, payload)
	record := phoneRecoveryRecord{Number: number, Recovery: recovery, NewOwner: newOwner, AuthHash: payload, Signature: forgedSignature}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	prop := PhonePropagation{
		ID:      1,
		Kind:    "number-recovered",
		Hash:    peer.stablePhoneHash("number-recovered", []byte(number), recovery.Bytes(), newOwner.Bytes(), payload.Bytes(), forgedSignature),
		Payload: data,
	}
	if err := peer.ImportPropagation(prop); err == nil {
		t.Fatal("accepted number-recovered propagation signed by the wrong key")
	}
	if peer.numbers[number].Owner != owner {
		t.Fatal("forged propagation changed the local phone owner")
	}

	api := &TkmPhoneAPI{service: peer}
	if ok, err := api.ImportPropagation(prop); err == nil || ok {
		t.Fatalf("public propagation import = (%v, %v), want a hard failure", ok, err)
	}
}
