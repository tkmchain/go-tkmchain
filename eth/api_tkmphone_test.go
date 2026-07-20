package eth

import (
	"bytes"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
)

func newTestTkmPhoneService(t *testing.T) (*TkmPhoneService, common.Address, common.Address, common.Address, *big.Int) {
	t.Helper()

	mainKingKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	operatorKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	ownerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mainKing := crypto.PubkeyToAddress(mainKingKey.PublicKey)
	operator := crypto.PubkeyToAddress(operatorKey.PublicKey)
	owner := crypto.PubkeyToAddress(ownerKey.PublicKey)
	svc := NewTkmPhoneService(nil, mainKing, big.NewInt(8979))
	return svc, mainKing, operator, owner, mainKingKey.D
}

func signTkmPhoneGrant(t *testing.T, svc *TkmPhoneService, mainKingD *big.Int, operator common.Address, keyHash common.Hash, expiresAt uint64, paymentTx common.Hash) []byte {
	t.Helper()
	key, err := crypto.ToECDSA(mainKingD.FillBytes(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(svc.operatorGrantHash(operator, keyHash, expiresAt, paymentTx).Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func registerTestTkmPhoneOperator(t *testing.T, svc *TkmPhoneService, mainKingD *big.Int, operator common.Address) PhoneOperatorKey {
	t.Helper()
	keyHash := crypto.Keccak256Hash([]byte("operator-key"))
	paymentTx := crypto.Keccak256Hash([]byte("5000-tkm-payment"))
	expiresAt := uint64(time.Now().Add(24 * time.Hour).Unix())
	sig := signTkmPhoneGrant(t, svc, mainKingD, operator, keyHash, expiresAt, paymentTx)
	key, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, sig)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestTkmPhoneOperatorKeyRequiresMainKingSignatureAndPrice(t *testing.T) {
	svc, _, operator, _, mainKingD := newTestTkmPhoneService(t)
	keyHash := crypto.Keccak256Hash([]byte("operator-key"))
	paymentTx := crypto.Keccak256Hash([]byte("payment"))
	expiresAt := uint64(time.Now().Add(time.Hour).Unix())

	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, big.NewInt(1), nil); err == nil || !strings.Contains(err.Error(), "5000 TKM") {
		t.Fatalf("wrong price error = %v, want 5000 TKM rejection", err)
	}
	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, make([]byte, 65)); err == nil {
		t.Fatal("accepted unsigned operator key")
	}

	sig := signTkmPhoneGrant(t, svc, mainKingD, operator, keyHash, expiresAt, paymentTx)
	key, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, sig)
	if err != nil {
		t.Fatalf("signed operator key rejected: %v", err)
	}
	if !key.Active || key.Operator != operator || key.Paid.ToInt().Cmp(tkmPhoneOperatorKeyPrice) != 0 {
		t.Fatalf("bad operator key record: %#v", key)
	}
}

func TestTkmPhoneMessageAndCallWork(t *testing.T) {
	svc, _, operator, alice, mainKingD := newTestTkmPhoneService(t)
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	bob := common.HexToAddress("0x2000000000000000000000000000000000000002")

	aliceNumber, err := svc.GenerateNumber(operator, alice, "alice")
	if err != nil {
		t.Fatal(err)
	}
	bobNumber, err := svc.GenerateNumber(operator, bob, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if aliceNumber.Number == bobNumber.Number {
		t.Fatal("generated duplicate phone numbers")
	}

	nonce := []byte("msg-nonce-01")
	plain := []byte("hello over tkmphone")
	cipher, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, nonce, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(cipher.Ciphertext, plain) {
		t.Fatal("ciphertext matches plaintext")
	}
	decrypted, err := svc.DecryptPayload(aliceNumber.Number, bobNumber.Number, nonce, cipher.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decrypted message = %q, want %q", decrypted, plain)
	}
	msg, err := svc.SendEncryptedMessage(aliceNumber.Number, bobNumber.Number, cipher.Ciphertext, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != 1 || msg.RandomXHash == (common.Hash{}) {
		t.Fatalf("bad message record: %#v", msg)
	}

	offer, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("offer-nonce1"), []byte("voice-offer"))
	if err != nil {
		t.Fatal(err)
	}
	call, err := svc.StartCall(aliceNumber.Number, bobNumber.Number, offer.Ciphertext, offer.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if call.State != PhoneCallRinging {
		t.Fatalf("call state = %s, want ringing", call.State)
	}
	answer, err := svc.EncryptPayload(bobNumber.Number, aliceNumber.Number, []byte("answer-nonce"), []byte("voice-answer"))
	if err != nil {
		t.Fatal(err)
	}
	call, err = svc.AcceptCall(uint64(call.ID), answer.Ciphertext, answer.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if call.State != PhoneCallActive || call.AnswerRandomXHash == (common.Hash{}) {
		t.Fatalf("bad accepted call: %#v", call)
	}
	call, err = svc.EndCall(uint64(call.ID))
	if err != nil {
		t.Fatal(err)
	}
	if call.State != PhoneCallEnded {
		t.Fatalf("call state = %s, want ended", call.State)
	}
}

func TestTkmPhoneSendsHelloToPersonBInDifferentLocation(t *testing.T) {
	svc, _, operator, personA, mainKingD := newTestTkmPhoneService(t)
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	personB := common.HexToAddress("0x5000000000000000000000000000000000000005")

	personANumber, err := svc.GenerateNumber(operator, personA, "person-a-location-1")
	if err != nil {
		t.Fatal(err)
	}
	personBNumber, err := svc.GenerateNumber(operator, personB, "person-b-location-2")
	if err != nil {
		t.Fatal(err)
	}
	if personANumber.Number == personBNumber.Number {
		t.Fatal("different-location users received the same number")
	}
	if personANumber.RandomX == personBNumber.RandomX {
		t.Fatal("different-location numbers received the same RandomX hash")
	}

	cipher, err := svc.EncryptPayload(personANumber.Number, personBNumber.Number, []byte("hello-nonce1"), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	message, err := svc.SendEncryptedMessage(personANumber.Number, personBNumber.Number, cipher.Ciphertext, cipher.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if message.From != personANumber.Number || message.To != personBNumber.Number {
		t.Fatalf("message route = %s -> %s, want %s -> %s", message.From, message.To, personANumber.Number, personBNumber.Number)
	}
	if bytes.Equal(message.Ciphertext, []byte("hello")) {
		t.Fatal("stored message is not encrypted")
	}
	opened, err := svc.DecryptPayload(personANumber.Number, personBNumber.Number, message.Nonce, message.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(opened) != "hello" {
		t.Fatalf("decrypted message = %q, want hello", opened)
	}
}

func TestTkmPhoneStatePersistsAcrossServiceRestart(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	svc, mainKing, operator, alice, mainKingD := newTestTkmPhoneService(t)
	svc.db = db
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	bob := common.HexToAddress("0x3000000000000000000000000000000000000003")

	aliceNumber, err := svc.GenerateNumber(operator, alice, "alice")
	if err != nil {
		t.Fatal(err)
	}
	bobNumber, err := svc.GenerateNumber(operator, bob, "bob")
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("persist-msg1"), []byte("persistent hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendEncryptedMessage(aliceNumber.Number, bobNumber.Number, cipher.Ciphertext, cipher.Nonce); err != nil {
		t.Fatal(err)
	}
	offer, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("persist-call"), []byte("persistent call"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartCall(aliceNumber.Number, bobNumber.Number, offer.Ciphertext, offer.Nonce); err != nil {
		t.Fatal(err)
	}

	reloaded := NewTkmPhoneServiceWithDB(nil, mainKing, big.NewInt(8979), db)
	got, err := reloaded.Number(aliceNumber.Number)
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != alice || got.Operator != operator {
		t.Fatalf("reloaded number mismatch: %#v", got)
	}
	if len(reloaded.operators) != 1 || len(reloaded.messages) != 1 || len(reloaded.calls) != 1 {
		t.Fatalf("reloaded state sizes: operators=%d messages=%d calls=%d", len(reloaded.operators), len(reloaded.messages), len(reloaded.calls))
	}
	next, err := reloaded.GenerateNumber(operator, common.HexToAddress("0x4000000000000000000000000000000000000004"), "charlie")
	if err != nil {
		t.Fatal(err)
	}
	if next.Number == aliceNumber.Number || next.Number == bobNumber.Number {
		t.Fatal("reloaded service reused an existing number")
	}
}
