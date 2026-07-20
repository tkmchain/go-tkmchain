package eth

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
)

func newTestTkmPhoneService(t *testing.T) (*TkmPhoneService, common.Address, common.Address, common.Address, *big.Int, *ecdsa.PrivateKey) {
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
	return svc, mainKing, operator, owner, mainKingKey.D, operatorKey
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

func generateTestTkmPhoneBuckets(t *testing.T, svc *TkmPhoneService, mainKingD *big.Int, seedLabel string) []PhoneNumberBucket {
	t.Helper()
	key, err := crypto.ToECDSA(mainKingD.FillBytes(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	seed := crypto.Keccak256Hash([]byte(seedLabel))
	round := svc.nextBucket/tkmPhoneBucketBatchSize + 1
	sig, err := crypto.Sign(svc.bucketGenerationHash(round, seed).Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	buckets, err := svc.GenerateBuckets(seed, sig)
	if err != nil {
		t.Fatal(err)
	}
	return buckets
}

func registerTestTkmPhoneOperator(t *testing.T, svc *TkmPhoneService, mainKingD *big.Int, operator common.Address) PhoneOperatorKey {
	t.Helper()
	hasOpenBucket := false
	for _, bucket := range svc.Buckets() {
		if uint64(bucket.AssignedAt) == 0 {
			hasOpenBucket = true
			break
		}
	}
	if !hasOpenBucket {
		generateTestTkmPhoneBuckets(t, svc, mainKingD, fmt.Sprintf("buckets-%d", len(svc.Buckets())+1))
	}
	keyHash := crypto.Keccak256Hash([]byte("operator-key"))
	paymentTx := crypto.Keccak256Hash([]byte("25000-tkm-bucket-payment"))
	expiresAt := uint64(time.Now().Add(24 * time.Hour).Unix())
	sig := signTkmPhoneGrant(t, svc, mainKingD, operator, keyHash, expiresAt, paymentTx)
	key, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, sig)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func openTestTkmPhoneBucket(t *testing.T, svc *TkmPhoneService, operator common.Address, operatorKey *ecdsa.PrivateKey) []PhoneNumber {
	t.Helper()
	key, ok := svc.operators[operator]
	if !ok {
		t.Fatal("operator key missing")
	}
	payload := svc.randomXServiceHash("open-bucket-payload", operator.Bytes(), tkmPhoneUint64Bytes(uint64(key.BucketID)))
	sig := signTkmPhoneDigest(t, operatorKey, payload)
	inventory, err := svc.OpenBucket(operator, uint64(key.BucketID), sig)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) == 0 {
		t.Fatal("operator inventory is empty")
	}
	return inventory
}

func sellTestTkmPhoneNumber(t *testing.T, svc *TkmPhoneService, operator common.Address, operatorKey *ecdsa.PrivateKey, buyer common.Address, label string) PhoneNumber {
	t.Helper()
	inventory := openTestTkmPhoneBucket(t, svc, operator, operatorKey)
	number, err := svc.SellNumber(operator, inventory[0].Number, buyer, tkmPhoneDefaultNumberSalePrice, crypto.Keccak256Hash([]byte(label)))
	if err != nil {
		t.Fatal(err)
	}
	return number
}

func TestTkmPhoneOperatorKeyRequiresMainKingSignatureAndPrice(t *testing.T) {
	svc, _, operator, _, mainKingD, operatorKey := newTestTkmPhoneService(t)
	keyHash := crypto.Keccak256Hash([]byte("operator-key"))
	paymentTx := crypto.Keccak256Hash([]byte("payment"))
	expiresAt := uint64(time.Now().Add(time.Hour).Unix())
	generateTestTkmPhoneBuckets(t, svc, mainKingD, "operator-key-test-buckets")

	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, big.NewInt(1), nil); err == nil || !strings.Contains(err.Error(), "25000 TKM") {
		t.Fatalf("wrong price error = %v, want 25000 TKM rejection", err)
	}
	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, make([]byte, 65)); err == nil {
		t.Fatal("accepted unsigned operator key")
	}

	sig := signTkmPhoneGrant(t, svc, mainKingD, operator, keyHash, expiresAt, paymentTx)
	key, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, sig)
	if err != nil {
		t.Fatalf("signed operator key rejected: %v", err)
	}
	if !key.Active || key.Operator != operator || key.Paid.ToInt().Cmp(tkmPhoneOperatorKeyPrice) != 0 || uint64(key.Numbers) != tkmPhoneBucketSize {
		t.Fatalf("bad operator key record: %#v", key)
	}
	inventory := openTestTkmPhoneBucket(t, svc, operator, operatorKey)
	if len(inventory) != int(tkmPhoneBucketSize) {
		t.Fatalf("operator inventory = %d, want %d", len(inventory), tkmPhoneBucketSize)
	}
	wrongKey, _ := crypto.GenerateKey()
	wrongSig := signTkmPhoneDigest(t, wrongKey, svc.randomXServiceHash("open-bucket-payload", operator.Bytes(), tkmPhoneUint64Bytes(uint64(key.BucketID))))
	if _, err := svc.OpenBucket(operator, uint64(key.BucketID), wrongSig); err == nil {
		t.Fatal("non-operator opened bucket")
	}
	buyer := common.HexToAddress("0x1000000000000000000000000000000000000001")
	saleTx := crypto.Keccak256Hash([]byte("10000-tkm-sale-payment"))
	sold, err := svc.SellNumber(operator, inventory[0].Number, buyer, tkmPhoneDefaultNumberSalePrice, saleTx)
	if err != nil {
		t.Fatal(err)
	}
	if sold.Owner != buyer || sold.Operator != operator || sold.SalePrice.ToInt().Cmp(tkmPhoneDefaultNumberSalePrice) != 0 || sold.SalePaymentTx != saleTx || uint64(sold.SoldAt) == 0 {
		t.Fatalf("bad sold number: %#v", sold)
	}
	if _, err := svc.SellNumber(operator, sold.Number, common.HexToAddress("0x1000000000000000000000000000000000000002"), tkmPhoneDefaultNumberSalePrice, crypto.Keccak256Hash([]byte("second-sale"))); err == nil {
		t.Fatal("resold an already sold number")
	}
	inventory = openTestTkmPhoneBucket(t, svc, operator, operatorKey)
	if len(inventory) != int(tkmPhoneBucketSize)-1 {
		t.Fatalf("operator inventory after sale = %d, want %d", len(inventory), int(tkmPhoneBucketSize)-1)
	}
}

func TestTkmPhoneMainKingBucketsGateNumberIssuance(t *testing.T) {
	mainKingKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mainKing := crypto.PubkeyToAddress(mainKingKey.PublicKey)
	svc := NewTkmPhoneService(nil, mainKing, big.NewInt(8979))
	operatorKey, _ := crypto.GenerateKey()
	operator := crypto.PubkeyToAddress(operatorKey.PublicKey)
	keyHash := crypto.Keccak256Hash([]byte("bucket-gated-operator"))
	paymentTx := crypto.Keccak256Hash([]byte("bucket-gated-payment"))
	expiresAt := uint64(time.Now().Add(time.Hour).Unix())
	grantSig := signTkmPhoneDigest(t, mainKingKey, svc.operatorGrantHash(operator, keyHash, expiresAt, paymentTx))
	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, grantSig); err == nil || !strings.Contains(err.Error(), "no unsold main king") {
		t.Fatalf("operator registered without bucket: %v", err)
	}
	if _, err := svc.GenerateNumber(operator, operator, "fake"); err == nil {
		t.Fatal("direct forged number generation was allowed")
	}

	buckets := generateTestTkmPhoneBuckets(t, svc, mainKingKey.D, "limited-buckets")
	if len(buckets) != int(tkmPhoneBucketBatchSize) {
		t.Fatalf("bucket batch = %d, want %d", len(buckets), tkmPhoneBucketBatchSize)
	}
	if _, err := svc.GenerateBuckets(crypto.Keccak256Hash([]byte("too-soon")), signTkmPhoneDigest(t, mainKingKey, svc.bucketGenerationHash(1, crypto.Keccak256Hash([]byte("too-soon"))))); err == nil || !strings.Contains(err.Error(), "not completely bought") {
		t.Fatalf("generated new buckets before sellout: %v", err)
	}

	for i := uint64(0); i < tkmPhoneBucketBatchSize; i++ {
		opKey, _ := crypto.GenerateKey()
		op := crypto.PubkeyToAddress(opKey.PublicKey)
		kh := crypto.Keccak256Hash([]byte(fmt.Sprintf("operator-%d", i)))
		ptx := crypto.Keccak256Hash([]byte(fmt.Sprintf("operator-payment-%d", i)))
		sig := signTkmPhoneDigest(t, mainKingKey, svc.operatorGrantHash(op, kh, expiresAt, ptx))
		key, err := svc.RegisterOperatorKey(op, kh, expiresAt, ptx, tkmPhoneOperatorKeyPrice, sig)
		if err != nil {
			t.Fatalf("register operator %d: %v", i, err)
		}
		if key.BucketID == 0 {
			t.Fatalf("operator %d missing bucket id", i)
		}
	}
	seed := crypto.Keccak256Hash([]byte("second-batch"))
	sig := signTkmPhoneDigest(t, mainKingKey, svc.bucketGenerationHash(2, seed))
	if next, err := svc.GenerateBuckets(seed, sig); err != nil || len(next) != int(tkmPhoneBucketBatchSize) {
		t.Fatalf("second bucket batch = %#v err=%v", next, err)
	}
}

func TestTkmPhoneMessageAndCallWork(t *testing.T) {
	svc, _, operator, alice, mainKingD, operatorKey := newTestTkmPhoneService(t)
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	bob := common.HexToAddress("0x2000000000000000000000000000000000000002")

	aliceNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, alice, "alice-number-sale")
	bobNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, bob, "bob-number-sale")
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
	aliceCandidate, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("alice-ice001"), []byte("candidate:alice udp 10.0.0.1:40000"))
	if err != nil {
		t.Fatal(err)
	}
	aliceSignal, err := svc.AddCallCandidate(uint64(call.ID), aliceNumber.Number, aliceCandidate.Ciphertext, aliceCandidate.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if aliceSignal.From != aliceNumber.Number || aliceSignal.To != bobNumber.Number || aliceSignal.Kind != "ice" {
		t.Fatalf("bad caller ICE signal: %#v", aliceSignal)
	}
	bobCandidate, err := svc.EncryptPayload(bobNumber.Number, aliceNumber.Number, []byte("bob-ice00001"), []byte("candidate:bob udp 10.0.0.2:40000"))
	if err != nil {
		t.Fatal(err)
	}
	bobSignal, err := svc.AddCallCandidate(uint64(call.ID), bobNumber.Number, bobCandidate.Ciphertext, bobCandidate.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	signalsForAlice, err := svc.CallCandidates(uint64(call.ID), aliceNumber.Number)
	if err != nil {
		t.Fatal(err)
	}
	if len(signalsForAlice) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(signalsForAlice))
	}
	openedBobCandidate, err := svc.DecryptPayload(bobSignal.From, bobSignal.To, bobSignal.Nonce, bobSignal.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(openedBobCandidate) != "candidate:bob udp 10.0.0.2:40000" {
		t.Fatalf("decrypted bob candidate = %q", openedBobCandidate)
	}
	config := svc.WebRTCConfig()
	if !config.AudioOnly || !config.RequiredEncryption || config.MaxSignalBytes == 0 || len(config.ICEServers) == 0 {
		t.Fatalf("bad WebRTC config: %#v", config)
	}
	call, err = svc.EndCall(uint64(call.ID))
	if err != nil {
		t.Fatal(err)
	}
	if call.State != PhoneCallEnded {
		t.Fatalf("call state = %s, want ended", call.State)
	}
}

func TestTkmPhoneVoiceCallRejectAndMissedWork(t *testing.T) {
	svc, _, operator, alice, mainKingD, operatorKey := newTestTkmPhoneService(t)
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	bob := common.HexToAddress("0x2000000000000000000000000000000000000002")
	aliceNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, alice, "reject-alice-number-sale")
	bobNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, bob, "reject-bob-number-sale")

	offer, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("reject-offer"), []byte("voice-offer"))
	if err != nil {
		t.Fatal(err)
	}
	call, err := svc.StartCall(aliceNumber.Number, bobNumber.Number, offer.Ciphertext, offer.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := svc.RejectCall(uint64(call.ID), bobNumber.Number, "busy")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.State != PhoneCallRejected || rejected.EndReason != "busy" {
		t.Fatalf("rejected call = %#v", rejected)
	}

	missedOffer, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("missed-offer"), []byte("voice-offer"))
	if err != nil {
		t.Fatal(err)
	}
	missedCall, err := svc.StartCall(aliceNumber.Number, bobNumber.Number, missedOffer.Ciphertext, missedOffer.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	svc.lock.Lock()
	missedCall.StartedAt = hexutil.Uint64(uint64(time.Now().Unix()) - 120)
	svc.calls[uint64(missedCall.ID)] = missedCall
	svc.lock.Unlock()
	expired, err := svc.ExpireRingingCalls(60)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].State != PhoneCallMissed || expired[0].EndReason != "timeout" {
		t.Fatalf("expired calls = %#v", expired)
	}
}

func TestTkmPhoneSendsHelloToPersonBInDifferentLocation(t *testing.T) {
	svc, _, operator, personA, mainKingD, operatorKey := newTestTkmPhoneService(t)
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	personB := common.HexToAddress("0x5000000000000000000000000000000000000005")

	personANumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, personA, "person-a-number-sale")
	personBNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, personB, "person-b-number-sale")
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
	svc, mainKing, operator, alice, mainKingD, operatorKey := newTestTkmPhoneService(t)
	svc.db = db
	registerTestTkmPhoneOperator(t, svc, mainKingD, operator)
	bob := common.HexToAddress("0x3000000000000000000000000000000000000003")

	aliceNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, alice, "alice-number-sale")
	bobNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, bob, "bob-number-sale")
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
	next := sellTestTkmPhoneNumber(t, reloaded, operator, operatorKey, common.HexToAddress("0x4000000000000000000000000000000000000004"), "charlie-number-sale")
	if next.Number == aliceNumber.Number || next.Number == bobNumber.Number {
		t.Fatal("reloaded service reused an existing number")
	}
}

func signTkmPhoneDigest(t *testing.T, key *ecdsa.PrivateKey, digest common.Hash) []byte {
	t.Helper()
	sig, err := crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func signTkmPhoneOwnerAction(t *testing.T, svc *TkmPhoneService, key *ecdsa.PrivateKey, number string, action string, payload common.Hash) []byte {
	t.Helper()
	return signTkmPhoneDigest(t, key, svc.ownerActionHash(number, action, payload))
}

func TestTkmPhoneSignedActionsInboxNotificationsDevicesTransferRevokeAndPrune(t *testing.T) {
	mainKingKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	operatorKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	aliceKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	bobKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mainKing := crypto.PubkeyToAddress(mainKingKey.PublicKey)
	operator := crypto.PubkeyToAddress(operatorKey.PublicKey)
	alice := crypto.PubkeyToAddress(aliceKey.PublicKey)
	bob := crypto.PubkeyToAddress(bobKey.PublicKey)
	svc := NewTkmPhoneService(nil, mainKing, big.NewInt(8979))
	generateTestTkmPhoneBuckets(t, svc, mainKingKey.D, "signed-actions-buckets")

	keyHash := crypto.Keccak256Hash([]byte("operator-key-signed-actions"))
	paymentTx := crypto.Keccak256Hash([]byte("operator-payment-signed-actions"))
	expiresAt := uint64(time.Now().Add(24 * time.Hour).Unix())
	grantSig := signTkmPhoneDigest(t, mainKingKey, svc.operatorGrantHash(operator, keyHash, expiresAt, paymentTx))
	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, grantSig); err != nil {
		t.Fatal(err)
	}

	aliceNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, alice, "alice-number-sale")
	bobNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, bob, "bob-number-sale")

	devicePayload := svc.randomXServiceHash("device-key-payload", []byte(aliceNumber.Number), []byte("alice-phone"), []byte("alice-device-public-key"))
	deviceSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "register-device", devicePayload)
	device, err := svc.RegisterDeviceKey(aliceNumber.Number, "alice-phone", []byte("alice-device-public-key"), deviceSig)
	if err != nil {
		t.Fatal(err)
	}
	if !device.Active || device.Number != aliceNumber.Number {
		t.Fatalf("bad device key: %#v", device)
	}

	cipher, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("signed-msg01"), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendEncryptedMessageSigned(aliceNumber.Number, bobNumber.Number, cipher.Ciphertext, cipher.Nonce, nil); err == nil {
		t.Fatal("accepted unsigned message")
	}
	msgPayload := svc.randomXServiceHash("send-message-payload", []byte(aliceNumber.Number), []byte(bobNumber.Number), cipher.Nonce, cipher.Ciphertext)
	msgSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "send-message", msgPayload)
	msg, err := svc.SendEncryptedMessageSigned(aliceNumber.Number, bobNumber.Number, cipher.Ciphertext, cipher.Nonce, msgSig)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Status != PhoneMessageSent {
		t.Fatalf("message status = %s, want sent", msg.Status)
	}
	bobMessages, err := svc.MessagesForNumber(bobNumber.Number)
	if err != nil {
		t.Fatal(err)
	}
	if len(bobMessages) != 1 || bobMessages[0].ID != msg.ID {
		t.Fatalf("bob inbox = %#v", bobMessages)
	}
	bobNotifications, err := svc.Notifications(bobNumber.Number)
	if err != nil {
		t.Fatal(err)
	}
	if len(bobNotifications) != 1 || bobNotifications[0].Kind != "message" {
		t.Fatalf("bob notifications = %#v", bobNotifications)
	}

	ackPayload := svc.randomXServiceHash("ack-message-payload", tkmPhoneUint64Bytes(uint64(msg.ID)), []byte(PhoneMessageDelivered))
	ackSig := signTkmPhoneOwnerAction(t, svc, bobKey, bobNumber.Number, "ack-message", ackPayload)
	acked, err := svc.AckMessage(uint64(msg.ID), PhoneMessageDelivered, ackSig)
	if err != nil {
		t.Fatal(err)
	}
	if acked.Status != PhoneMessageDelivered {
		t.Fatalf("acked status = %s", acked.Status)
	}

	offer, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("signed-call1"), []byte("call offer"))
	if err != nil {
		t.Fatal(err)
	}
	callPayload := svc.randomXServiceHash("start-call-payload", []byte(aliceNumber.Number), []byte(bobNumber.Number), offer.Nonce, offer.Ciphertext)
	callSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "start-call", callPayload)
	call, err := svc.StartCallSigned(aliceNumber.Number, bobNumber.Number, offer.Ciphertext, offer.Nonce, callSig)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := svc.EncryptPayload(bobNumber.Number, aliceNumber.Number, []byte("signed-answ1"), []byte("call answer"))
	if err != nil {
		t.Fatal(err)
	}
	acceptPayload := svc.randomXServiceHash("accept-call-payload", tkmPhoneUint64Bytes(uint64(call.ID)), answer.Nonce, answer.Ciphertext)
	acceptSig := signTkmPhoneOwnerAction(t, svc, bobKey, bobNumber.Number, "accept-call", acceptPayload)
	call, err = svc.AcceptCallSigned(uint64(call.ID), answer.Ciphertext, answer.Nonce, acceptSig)
	if err != nil {
		t.Fatal(err)
	}
	if call.State != PhoneCallActive {
		t.Fatalf("call state = %s", call.State)
	}
	aliceCalls, err := svc.CallsForNumber(aliceNumber.Number)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliceCalls) != 1 || aliceCalls[0].ID != call.ID {
		t.Fatalf("alice calls = %#v", aliceCalls)
	}
	candidate, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("signed-ice01"), []byte("candidate:signed-alice"))
	if err != nil {
		t.Fatal(err)
	}
	candidatePayload := svc.callCandidateHash(uint64(call.ID), aliceNumber.Number, candidate.Nonce, candidate.Ciphertext)
	candidateSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "add-call-candidate", candidatePayload)
	if _, err := svc.AddCallCandidateSigned(uint64(call.ID), aliceNumber.Number, candidate.Ciphertext, candidate.Nonce, candidateSig); err != nil {
		t.Fatal(err)
	}
	listPayload := svc.callCandidateListHash(uint64(call.ID), bobNumber.Number)
	listSig := signTkmPhoneOwnerAction(t, svc, bobKey, bobNumber.Number, "list-call-candidates", listPayload)
	signals, err := svc.CallCandidatesSigned(uint64(call.ID), bobNumber.Number, listSig)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].From != aliceNumber.Number || signals[0].To != bobNumber.Number {
		t.Fatalf("signed call signals = %#v", signals)
	}
	endPayload := svc.randomXServiceHash("end-call-payload", tkmPhoneUint64Bytes(uint64(call.ID)))
	endSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "end-call", endPayload)
	call, err = svc.EndCallSigned(uint64(call.ID), aliceNumber.Number, endSig)
	if err != nil {
		t.Fatal(err)
	}
	if call.State != PhoneCallEnded {
		t.Fatalf("call state = %s", call.State)
	}

	transferPayload := svc.randomXServiceHash("transfer-number-payload", []byte(aliceNumber.Number), bob.Bytes())
	transferSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "transfer-number", transferPayload)
	transferred, err := svc.TransferNumber(aliceNumber.Number, bob, transferSig)
	if err != nil {
		t.Fatal(err)
	}
	if transferred.Owner != bob {
		t.Fatalf("transferred owner = %s, want %s", transferred.Owner, bob)
	}
	revokePayload := svc.randomXServiceHash("revoke-number-payload", []byte(aliceNumber.Number))
	revokeSig := signTkmPhoneOwnerAction(t, svc, bobKey, aliceNumber.Number, "revoke-number", revokePayload)
	revoked, err := svc.RevokeNumber(aliceNumber.Number, revokeSig)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Active {
		t.Fatal("revoked number is still active")
	}

	if err := svc.Prune(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if len(svc.messages) != 1 || len(svc.calls) != 1 {
		t.Fatalf("unbounded prune removed records")
	}
	if err := svc.Prune(0, 0, 0); err != nil {
		t.Fatal(err)
	}
}

func TestTkmPhoneMarketplaceContactsBlockingRecoveryExpiryAndPropagation(t *testing.T) {
	mainKingKey, _ := crypto.GenerateKey()
	operatorKey, _ := crypto.GenerateKey()
	aliceKey, _ := crypto.GenerateKey()
	bobKey, _ := crypto.GenerateKey()
	recoveryKey, _ := crypto.GenerateKey()
	mainKing := crypto.PubkeyToAddress(mainKingKey.PublicKey)
	operator := crypto.PubkeyToAddress(operatorKey.PublicKey)
	alice := crypto.PubkeyToAddress(aliceKey.PublicKey)
	bob := crypto.PubkeyToAddress(bobKey.PublicKey)
	recovery := crypto.PubkeyToAddress(recoveryKey.PublicKey)
	svc := NewTkmPhoneService(nil, mainKing, big.NewInt(8979))
	generateTestTkmPhoneBuckets(t, svc, mainKingKey.D, "market-buckets")
	keyHash := crypto.Keccak256Hash([]byte("market-key"))
	paymentTx := crypto.Keccak256Hash([]byte("market-payment"))
	expiresAt := uint64(time.Now().Add(time.Hour).Unix())
	grantSig := signTkmPhoneDigest(t, mainKingKey, svc.operatorGrantHash(operator, keyHash, expiresAt, paymentTx))
	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, grantSig); err != nil {
		t.Fatal(err)
	}
	if ops := svc.ListOperators(); len(ops) != 1 || ops[0].Operator != operator {
		t.Fatalf("operators = %#v", ops)
	}
	aliceNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, alice, "market-alice-number-sale")
	bobNumber := sellTestTkmPhoneNumber(t, svc, operator, operatorKey, bob, "market-bob-number-sale")
	devicePayload := svc.randomXServiceHash("device-key-payload", []byte(bobNumber.Number), []byte("bob-phone"), []byte("bob-pub"))
	deviceSig := signTkmPhoneOwnerAction(t, svc, bobKey, bobNumber.Number, "register-device", devicePayload)
	if _, err := svc.RegisterDeviceKey(bobNumber.Number, "bob-phone", []byte("bob-pub"), deviceSig); err != nil {
		t.Fatal(err)
	}
	env, err := svc.EncryptPayloadForDevices(aliceNumber.Number, bobNumber.Number, []byte("device-nonce"), []byte("hello device"))
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 1 || env[0].Device != "bob-phone" {
		t.Fatalf("device envelopes = %#v", env)
	}
	contactCipher, _ := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("contact-nonc"), []byte("Bob"))
	contactPayload := svc.randomXServiceHash("add-contact-payload", []byte(aliceNumber.Number), []byte(bobNumber.Number), contactCipher.Nonce, contactCipher.Ciphertext)
	contactSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "add-contact", contactPayload)
	if _, err := svc.AddContact(aliceNumber.Number, bobNumber.Number, contactCipher.Ciphertext, contactCipher.Nonce, contactSig); err != nil {
		t.Fatal(err)
	}
	if contacts, _ := svc.Contacts(aliceNumber.Number); len(contacts) != 1 {
		t.Fatalf("contacts = %#v", contacts)
	}
	blockPayload := svc.randomXServiceHash("block-number-payload", []byte(bobNumber.Number), []byte(aliceNumber.Number))
	blockSig := signTkmPhoneOwnerAction(t, svc, bobKey, bobNumber.Number, "block-number", blockPayload)
	if err := svc.BlockNumber(bobNumber.Number, aliceNumber.Number, blockSig); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("blocked-nonc"), []byte("blocked")); err == nil {
		t.Fatal("blocked sender was allowed")
	}
	unblockPayload := svc.randomXServiceHash("unblock-number-payload", []byte(bobNumber.Number), []byte(aliceNumber.Number))
	unblockSig := signTkmPhoneOwnerAction(t, svc, bobKey, bobNumber.Number, "unblock-number", unblockPayload)
	if err := svc.UnblockNumber(bobNumber.Number, aliceNumber.Number, unblockSig); err != nil {
		t.Fatal(err)
	}
	msgCipher, _ := svc.EncryptPayload(aliceNumber.Number, bobNumber.Number, []byte("expiry-nonce"), []byte("expires"))
	msgPayload := svc.randomXServiceHash("send-message-payload", []byte(aliceNumber.Number), []byte(bobNumber.Number), msgCipher.Nonce, msgCipher.Ciphertext)
	msgSig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "send-message", msgPayload)
	if _, err := svc.SendEncryptedMessageWithExpiry(aliceNumber.Number, bobNumber.Number, msgCipher.Ciphertext, msgCipher.Nonce, uint64(time.Now().Add(-time.Second).Unix()), msgSig); err != nil {
		t.Fatal(err)
	}
	if len(svc.PropagationQueue()) == 0 {
		t.Fatal("missing propagation envelope")
	}
	if err := svc.Prune(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if len(svc.messages) != 0 {
		t.Fatal("expired message was not pruned")
	}
	recoveryPayload := svc.randomXServiceHash("register-recovery-payload", []byte(aliceNumber.Number), recovery.Bytes())
	recoverySig := signTkmPhoneOwnerAction(t, svc, aliceKey, aliceNumber.Number, "register-recovery", recoveryPayload)
	if err := svc.RegisterRecovery(aliceNumber.Number, recovery, recoverySig); err != nil {
		t.Fatal(err)
	}
	recoverPayload := svc.randomXServiceHash("recover-number-payload", []byte(aliceNumber.Number), bob.Bytes())
	recoverSig := signTkmPhoneDigest(t, recoveryKey, recoverPayload)
	recovered, err := svc.RecoverNumber(aliceNumber.Number, bob, recoverSig)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Owner != bob {
		t.Fatalf("recovered owner = %s", recovered.Owner)
	}
	reportPayload := svc.randomXServiceHash("report-operator-payload", operator.Bytes(), []byte(aliceNumber.Number), []byte("duplicate"), common.Hash{}.Bytes())
	reportSig := signTkmPhoneOwnerAction(t, svc, bobKey, aliceNumber.Number, "report-operator", reportPayload)
	if _, err := svc.ReportOperator(operator, aliceNumber.Number, "duplicate", common.Hash{}, reportSig); err != nil {
		t.Fatal(err)
	}

	remote := NewTkmPhoneService(nil, mainKing, big.NewInt(8979))
	for _, prop := range svc.PropagationQueue() {
		if err := remote.ImportPropagation(prop); err != nil {
			t.Fatalf("import propagation %s/%d: %v", prop.Kind, uint64(prop.ID), err)
		}
	}
	if got, err := remote.Number(bobNumber.Number); err != nil || got.Owner != bob {
		t.Fatalf("remote bob number = %#v err=%v", got, err)
	}
	if got, err := remote.Number(aliceNumber.Number); err != nil || got.Owner != bob {
		t.Fatalf("remote recovered alice number = %#v err=%v", got, err)
	}
	if contacts, err := remote.Contacts(aliceNumber.Number); err != nil || len(contacts) != 1 {
		t.Fatalf("remote contacts = %#v err=%v", contacts, err)
	}
	if reports := remote.reports; len(reports) != 1 {
		t.Fatalf("remote reports = %#v", reports)
	}
}
