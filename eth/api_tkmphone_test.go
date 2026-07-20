package eth

import (
	"bytes"
	"crypto/ecdsa"
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

	keyHash := crypto.Keccak256Hash([]byte("operator-key-signed-actions"))
	paymentTx := crypto.Keccak256Hash([]byte("operator-payment-signed-actions"))
	expiresAt := uint64(time.Now().Add(24 * time.Hour).Unix())
	grantSig := signTkmPhoneDigest(t, mainKingKey, svc.operatorGrantHash(operator, keyHash, expiresAt, paymentTx))
	if _, err := svc.RegisterOperatorKey(operator, keyHash, expiresAt, paymentTx, tkmPhoneOperatorKeyPrice, grantSig); err != nil {
		t.Fatal(err)
	}

	aliceNumber, err := svc.GenerateNumber(operator, alice, "alice-lagos")
	if err != nil {
		t.Fatal(err)
	}
	bobNumber, err := svc.GenerateNumber(operator, bob, "bob-abuja")
	if err != nil {
		t.Fatal(err)
	}

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
	aliceNumber, _ := svc.GenerateNumber(operator, alice, "a")
	bobNumber, _ := svc.GenerateNumber(operator, bob, "b")
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
}
