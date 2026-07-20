package eth

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/params"
)

var (
	tkmPhoneOperatorKeyPrice = new(big.Int).Mul(big.NewInt(5000), big.NewInt(params.Ether))
	tkmPhoneDefaultChainID   = big.NewInt(8979)
	tkmPhoneStateKey         = []byte("tkmphone-state-v1")
	tkmPhoneMaxPayloadSize   = 64 * 1024
	tkmPhoneMessageRateLimit = 20
	tkmPhoneCallRateLimit    = 10
)

type TkmPhoneAPI struct {
	service *TkmPhoneService
}

type TkmPhoneService struct {
	lock      sync.RWMutex
	eth       *Ethereum
	mainKing  common.Address
	chainID   *big.Int
	db        ethdb.KeyValueStore
	nextID    uint64
	nextMsg   uint64
	nextCall  uint64
	nextNotif uint64
	operators map[common.Address]PhoneOperatorKey
	numbers   map[string]PhoneNumber
	messages  map[uint64]PhoneMessage
	calls     map[uint64]PhoneCall
	devices   map[string][]PhoneDeviceKey
	notifs    map[uint64]PhoneNotification
	rate      map[string][]uint64
}

type tkmPhoneSnapshot struct {
	NextID    uint64
	NextMsg   uint64
	NextCall  uint64
	NextNotif uint64
	Operators map[common.Address]PhoneOperatorKey
	Numbers   map[string]PhoneNumber
	Messages  map[uint64]PhoneMessage
	Calls     map[uint64]PhoneCall
	Devices   map[string][]PhoneDeviceKey
	Notifs    map[uint64]PhoneNotification
	Rate      map[string][]uint64
}

type PhoneOperatorKey struct {
	Operator  common.Address `json:"operator"`
	KeyHash   common.Hash    `json:"keyHash"`
	PaymentTx common.Hash    `json:"paymentTx"`
	Paid      *hexutil.Big   `json:"paid"`
	ExpiresAt hexutil.Uint64 `json:"expiresAt"`
	Active    bool           `json:"active"`
}

type PhoneNumber struct {
	Number    string         `json:"number"`
	Owner     common.Address `json:"owner"`
	Operator  common.Address `json:"operator"`
	RandomX   common.Hash    `json:"randomxHash"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
	Active    bool           `json:"active"`
}

type PhoneCipher struct {
	Ciphertext  hexutil.Bytes `json:"ciphertext"`
	Nonce       hexutil.Bytes `json:"nonce"`
	RandomXHash common.Hash   `json:"randomxHash"`
}

type PhoneMessage struct {
	ID          hexutil.Uint64     `json:"id"`
	From        string             `json:"from"`
	To          string             `json:"to"`
	Ciphertext  hexutil.Bytes      `json:"ciphertext"`
	Nonce       hexutil.Bytes      `json:"nonce"`
	RandomXHash common.Hash        `json:"randomxHash"`
	CreatedAt   hexutil.Uint64     `json:"createdAt"`
	Status      PhoneMessageStatus `json:"status"`
}

type PhoneMessageStatus string

const (
	PhoneMessageSent      PhoneMessageStatus = "sent"
	PhoneMessageDelivered PhoneMessageStatus = "delivered"
	PhoneMessageRead      PhoneMessageStatus = "read"
)

type PhoneDeviceKey struct {
	Number    string         `json:"number"`
	Device    string         `json:"device"`
	PublicKey hexutil.Bytes  `json:"publicKey"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
	Active    bool           `json:"active"`
}

type PhoneNotification struct {
	ID        hexutil.Uint64 `json:"id"`
	Number    string         `json:"number"`
	Kind      string         `json:"kind"`
	RefID     hexutil.Uint64 `json:"refId"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
}

type PhoneCallState string

const (
	PhoneCallRinging PhoneCallState = "ringing"
	PhoneCallActive  PhoneCallState = "active"
	PhoneCallEnded   PhoneCallState = "ended"
)

type PhoneCall struct {
	ID                hexutil.Uint64 `json:"id"`
	From              string         `json:"from"`
	To                string         `json:"to"`
	OfferCiphertext   hexutil.Bytes  `json:"offerCiphertext"`
	OfferNonce        hexutil.Bytes  `json:"offerNonce"`
	OfferRandomXHash  common.Hash    `json:"offerRandomXHash"`
	AnswerCiphertext  hexutil.Bytes  `json:"answerCiphertext"`
	AnswerNonce       hexutil.Bytes  `json:"answerNonce"`
	AnswerRandomXHash common.Hash    `json:"answerRandomXHash"`
	State             PhoneCallState `json:"state"`
	StartedAt         hexutil.Uint64 `json:"startedAt"`
	AnsweredAt        hexutil.Uint64 `json:"answeredAt"`
	EndedAt           hexutil.Uint64 `json:"endedAt"`
}

func NewTkmPhoneAPI(e *Ethereum) *TkmPhoneAPI {
	return &TkmPhoneAPI{service: e.tkmPhoneService()}
}

func NewTkmPhoneService(e *Ethereum, mainKing common.Address, chainID *big.Int) *TkmPhoneService {
	return NewTkmPhoneServiceWithDB(e, mainKing, chainID, nil)
}

func NewTkmPhoneServiceWithDB(e *Ethereum, mainKing common.Address, chainID *big.Int, db ethdb.KeyValueStore) *TkmPhoneService {
	if chainID == nil {
		chainID = tkmPhoneDefaultChainID
	}
	svc := &TkmPhoneService{
		eth:       e,
		mainKing:  mainKing,
		chainID:   new(big.Int).Set(chainID),
		db:        db,
		operators: make(map[common.Address]PhoneOperatorKey),
		numbers:   make(map[string]PhoneNumber),
		messages:  make(map[uint64]PhoneMessage),
		calls:     make(map[uint64]PhoneCall),
		devices:   make(map[string][]PhoneDeviceKey),
		notifs:    make(map[uint64]PhoneNotification),
		rate:      make(map[string][]uint64),
	}
	if err := svc.load(); err != nil {
		log.Warn("Failed to load TKM phone service state", "err", err)
	}
	return svc
}

func (s *Ethereum) tkmPhoneService() *TkmPhoneService {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.phoneService == nil {
		chainID := tkmPhoneDefaultChainID
		if s.blockchain != nil && s.blockchain.Config() != nil && s.blockchain.Config().ChainID != nil {
			chainID = s.blockchain.Config().ChainID
		}
		s.phoneService = NewTkmPhoneServiceWithDB(s, s.GetMainKingAddress(), chainID, s.chainDb)
	}
	return s.phoneService
}

func (api *TkmPhoneAPI) OperatorKeyPrice() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(tkmPhoneOperatorKeyPrice))
}

func (api *TkmPhoneAPI) OperatorGrantHash(operator common.Address, keyHash common.Hash, expiresAt hexutil.Uint64, paymentTx common.Hash) common.Hash {
	return api.service.operatorGrantHash(operator, keyHash, uint64(expiresAt), paymentTx)
}

func (api *TkmPhoneAPI) RegisterOperatorKey(operator common.Address, keyHash common.Hash, expiresAt hexutil.Uint64, paymentTx common.Hash, paid hexutil.Big, signature hexutil.Bytes) (PhoneOperatorKey, error) {
	return api.service.RegisterOperatorKey(operator, keyHash, uint64(expiresAt), paymentTx, (*big.Int)(&paid), []byte(signature))
}

func (api *TkmPhoneAPI) GenerateNumber(operator common.Address, owner common.Address, label string) (PhoneNumber, error) {
	return api.service.GenerateNumber(operator, owner, label)
}

func (api *TkmPhoneAPI) Number(number string) (PhoneNumber, error) {
	return api.service.Number(number)
}

func (api *TkmPhoneAPI) EncryptPayload(from string, to string, nonce hexutil.Bytes, plaintext hexutil.Bytes) (PhoneCipher, error) {
	return api.service.EncryptPayload(from, to, []byte(nonce), []byte(plaintext))
}

func (api *TkmPhoneAPI) DecryptPayload(from string, to string, nonce hexutil.Bytes, ciphertext hexutil.Bytes) (hexutil.Bytes, error) {
	return api.service.DecryptPayload(from, to, []byte(nonce), []byte(ciphertext))
}

func (api *TkmPhoneAPI) SendEncryptedMessage(from string, to string, ciphertext hexutil.Bytes, nonce hexutil.Bytes, signature hexutil.Bytes) (PhoneMessage, error) {
	return api.service.SendEncryptedMessageSigned(from, to, []byte(ciphertext), []byte(nonce), []byte(signature))
}

func (api *TkmPhoneAPI) StartCall(from string, to string, offerCiphertext hexutil.Bytes, offerNonce hexutil.Bytes, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.StartCallSigned(from, to, []byte(offerCiphertext), []byte(offerNonce), []byte(signature))
}

func (api *TkmPhoneAPI) AcceptCall(id hexutil.Uint64, answerCiphertext hexutil.Bytes, answerNonce hexutil.Bytes, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.AcceptCallSigned(uint64(id), []byte(answerCiphertext), []byte(answerNonce), []byte(signature))
}

func (api *TkmPhoneAPI) EndCall(id hexutil.Uint64, number string, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.EndCallSigned(uint64(id), number, []byte(signature))
}

func (api *TkmPhoneAPI) OwnerActionHash(number string, action string, payload common.Hash) common.Hash {
	return api.service.ownerActionHash(number, action, payload)
}

func (api *TkmPhoneAPI) MessagesForNumber(number string) ([]PhoneMessage, error) {
	return api.service.MessagesForNumber(number)
}

func (api *TkmPhoneAPI) CallsForNumber(number string) ([]PhoneCall, error) {
	return api.service.CallsForNumber(number)
}

func (api *TkmPhoneAPI) RegisterDeviceKey(number string, device string, publicKey hexutil.Bytes, signature hexutil.Bytes) (PhoneDeviceKey, error) {
	return api.service.RegisterDeviceKey(number, device, []byte(publicKey), []byte(signature))
}

func (api *TkmPhoneAPI) TransferNumber(number string, newOwner common.Address, signature hexutil.Bytes) (PhoneNumber, error) {
	return api.service.TransferNumber(number, newOwner, []byte(signature))
}

func (api *TkmPhoneAPI) RevokeNumber(number string, signature hexutil.Bytes) (PhoneNumber, error) {
	return api.service.RevokeNumber(number, []byte(signature))
}

func (api *TkmPhoneAPI) AckMessage(id hexutil.Uint64, status string, signature hexutil.Bytes) (PhoneMessage, error) {
	return api.service.AckMessage(uint64(id), PhoneMessageStatus(status), []byte(signature))
}

func (api *TkmPhoneAPI) Notifications(number string) ([]PhoneNotification, error) {
	return api.service.Notifications(number)
}

func (api *TkmPhoneAPI) Prune(retentionSeconds hexutil.Uint64, maxMessages hexutil.Uint64, maxCalls hexutil.Uint64) (bool, error) {
	return true, api.service.Prune(uint64(retentionSeconds), int(maxMessages), int(maxCalls))
}

func (svc *TkmPhoneService) RegisterOperatorKey(operator common.Address, keyHash common.Hash, expiresAt uint64, paymentTx common.Hash, paid *big.Int, signature []byte) (PhoneOperatorKey, error) {
	if operator == (common.Address{}) {
		return PhoneOperatorKey{}, errors.New("operator address is required")
	}
	if keyHash == (common.Hash{}) {
		return PhoneOperatorKey{}, errors.New("operator key hash is required")
	}
	if paymentTx == (common.Hash{}) {
		return PhoneOperatorKey{}, errors.New("operator key payment transaction is required")
	}
	if paid == nil || paid.Cmp(tkmPhoneOperatorKeyPrice) != 0 {
		return PhoneOperatorKey{}, errors.New("operator key requires exactly 5000 TKM")
	}
	if expiresAt <= uint64(time.Now().Unix()) {
		return PhoneOperatorKey{}, errors.New("operator key is expired")
	}
	if err := svc.validateOperatorPayment(operator, paymentTx); err != nil {
		return PhoneOperatorKey{}, err
	}
	if err := svc.verifyMainKingSignature(svc.operatorGrantHash(operator, keyHash, expiresAt, paymentTx), signature); err != nil {
		return PhoneOperatorKey{}, err
	}

	key := PhoneOperatorKey{
		Operator:  operator,
		KeyHash:   keyHash,
		PaymentTx: paymentTx,
		Paid:      (*hexutil.Big)(new(big.Int).Set(paid)),
		ExpiresAt: hexutil.Uint64(expiresAt),
		Active:    true,
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.operators[operator] = key
	if err := svc.saveLocked(); err != nil {
		return PhoneOperatorKey{}, err
	}
	return key, nil
}

func (svc *TkmPhoneService) GenerateNumber(operator common.Address, owner common.Address, label string) (PhoneNumber, error) {
	if owner == (common.Address{}) {
		return PhoneNumber{}, errors.New("number owner is required")
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	key, ok := svc.operators[operator]
	if !ok || !key.Active || uint64(key.ExpiresAt) <= now {
		return PhoneNumber{}, errors.New("operator key is not active")
	}

	for {
		svc.nextID++
		rxh := svc.randomXServiceHash("number", operator.Bytes(), owner.Bytes(), []byte(label), tkmPhoneUint64Bytes(svc.nextID))
		number := fmt.Sprintf("+8979%011d", new(big.Int).SetBytes(rxh.Bytes()).Uint64()%100000000000)
		if _, exists := svc.numbers[number]; exists {
			continue
		}
		record := PhoneNumber{Number: number, Owner: owner, Operator: operator, RandomX: rxh, CreatedAt: hexutil.Uint64(now), Active: true}
		svc.numbers[number] = record
		if err := svc.saveLocked(); err != nil {
			return PhoneNumber{}, err
		}
		return record, nil
	}
}

func (svc *TkmPhoneService) Number(number string) (PhoneNumber, error) {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneNumber{}, errors.New("number not found")
	}
	return record, nil
}

func (svc *TkmPhoneService) EncryptPayload(from string, to string, nonce []byte, plaintext []byte) (PhoneCipher, error) {
	if err := svc.requireNumbers(from, to); err != nil {
		return PhoneCipher{}, err
	}
	if len(nonce) == 0 {
		return PhoneCipher{}, errors.New("nonce is required")
	}
	key := svc.messageKey(from, to, nonce)
	ciphertext, err := tkmPhoneSeal(key, nonce, plaintext, []byte(from+"->"+to))
	if err != nil {
		return PhoneCipher{}, err
	}
	return PhoneCipher{Ciphertext: ciphertext, Nonce: append([]byte(nil), nonce...), RandomXHash: key}, nil
}

func (svc *TkmPhoneService) DecryptPayload(from string, to string, nonce []byte, ciphertext []byte) (hexutil.Bytes, error) {
	if err := svc.requireNumbers(from, to); err != nil {
		return nil, err
	}
	if len(nonce) == 0 {
		return nil, errors.New("nonce is required")
	}
	key := svc.messageKey(from, to, nonce)
	return tkmPhoneOpen(key, nonce, ciphertext, []byte(from+"->"+to))
}

func (svc *TkmPhoneService) SendEncryptedMessage(from string, to string, ciphertext []byte, nonce []byte) (PhoneMessage, error) {
	if len(ciphertext) == 0 {
		return PhoneMessage{}, errors.New("ciphertext is required")
	}
	if len(ciphertext) > tkmPhoneMaxPayloadSize {
		return PhoneMessage{}, errors.New("ciphertext exceeds maximum payload size")
	}
	if len(nonce) == 0 {
		return PhoneMessage{}, errors.New("nonce is required")
	}
	if err := svc.requireNumbers(from, to); err != nil {
		return PhoneMessage{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if err := svc.checkRateLocked("msg:"+from, now, tkmPhoneMessageRateLimit); err != nil {
		return PhoneMessage{}, err
	}
	svc.nextMsg++
	msg := PhoneMessage{ID: hexutil.Uint64(svc.nextMsg), From: from, To: to, Ciphertext: append([]byte(nil), ciphertext...), Nonce: append([]byte(nil), nonce...), RandomXHash: svc.messageKey(from, to, nonce), CreatedAt: hexutil.Uint64(now), Status: PhoneMessageSent}
	svc.messages[svc.nextMsg] = msg
	svc.addNotificationLocked(to, "message", svc.nextMsg, now)
	if err := svc.saveLocked(); err != nil {
		return PhoneMessage{}, err
	}
	return msg, nil
}

func (svc *TkmPhoneService) StartCall(from string, to string, offerCiphertext []byte, offerNonce []byte) (PhoneCall, error) {
	if len(offerCiphertext) == 0 || len(offerNonce) == 0 {
		return PhoneCall{}, errors.New("encrypted call offer and nonce are required")
	}
	if len(offerCiphertext) > tkmPhoneMaxPayloadSize {
		return PhoneCall{}, errors.New("call offer exceeds maximum payload size")
	}
	if err := svc.requireNumbers(from, to); err != nil {
		return PhoneCall{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if err := svc.checkRateLocked("call:"+from, now, tkmPhoneCallRateLimit); err != nil {
		return PhoneCall{}, err
	}
	svc.nextCall++
	call := PhoneCall{ID: hexutil.Uint64(svc.nextCall), From: from, To: to, OfferCiphertext: append([]byte(nil), offerCiphertext...), OfferNonce: append([]byte(nil), offerNonce...), OfferRandomXHash: svc.messageKey(from, to, offerNonce), State: PhoneCallRinging, StartedAt: hexutil.Uint64(now)}
	svc.calls[svc.nextCall] = call
	svc.addNotificationLocked(to, "call", svc.nextCall, now)
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) AcceptCall(id uint64, answerCiphertext []byte, answerNonce []byte) (PhoneCall, error) {
	if len(answerCiphertext) == 0 || len(answerNonce) == 0 {
		return PhoneCall{}, errors.New("encrypted call answer and nonce are required")
	}
	if len(answerCiphertext) > tkmPhoneMaxPayloadSize {
		return PhoneCall{}, errors.New("call answer exceeds maximum payload size")
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	call, ok := svc.calls[id]
	if !ok {
		return PhoneCall{}, errors.New("call not found")
	}
	if call.State != PhoneCallRinging {
		return PhoneCall{}, errors.New("call is not ringing")
	}
	call.AnswerCiphertext = append([]byte(nil), answerCiphertext...)
	call.AnswerNonce = append([]byte(nil), answerNonce...)
	call.AnswerRandomXHash = svc.messageKey(call.To, call.From, answerNonce)
	call.State = PhoneCallActive
	call.AnsweredAt = hexutil.Uint64(time.Now().Unix())
	svc.calls[id] = call
	svc.addNotificationLocked(call.From, "call-accepted", id, uint64(call.AnsweredAt))
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) EndCall(id uint64) (PhoneCall, error) {
	svc.lock.Lock()
	defer svc.lock.Unlock()
	call, ok := svc.calls[id]
	if !ok {
		return PhoneCall{}, errors.New("call not found")
	}
	if call.State == PhoneCallEnded {
		return PhoneCall{}, errors.New("call already ended")
	}
	call.State = PhoneCallEnded
	call.EndedAt = hexutil.Uint64(time.Now().Unix())
	svc.calls[id] = call
	svc.addNotificationLocked(call.From, "call-ended", id, uint64(call.EndedAt))
	svc.addNotificationLocked(call.To, "call-ended", id, uint64(call.EndedAt))
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) SendEncryptedMessageSigned(from string, to string, ciphertext []byte, nonce []byte, signature []byte) (PhoneMessage, error) {
	payload := svc.randomXServiceHash("send-message-payload", []byte(from), []byte(to), nonce, ciphertext)
	if err := svc.verifyNumberOwnerSignature(from, "send-message", payload, signature); err != nil {
		return PhoneMessage{}, err
	}
	return svc.SendEncryptedMessage(from, to, ciphertext, nonce)
}

func (svc *TkmPhoneService) StartCallSigned(from string, to string, offerCiphertext []byte, offerNonce []byte, signature []byte) (PhoneCall, error) {
	payload := svc.randomXServiceHash("start-call-payload", []byte(from), []byte(to), offerNonce, offerCiphertext)
	if err := svc.verifyNumberOwnerSignature(from, "start-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	return svc.StartCall(from, to, offerCiphertext, offerNonce)
}

func (svc *TkmPhoneService) AcceptCallSigned(id uint64, answerCiphertext []byte, answerNonce []byte, signature []byte) (PhoneCall, error) {
	svc.lock.RLock()
	call, ok := svc.calls[id]
	svc.lock.RUnlock()
	if !ok {
		return PhoneCall{}, errors.New("call not found")
	}
	payload := svc.randomXServiceHash("accept-call-payload", tkmPhoneUint64Bytes(id), answerNonce, answerCiphertext)
	if err := svc.verifyNumberOwnerSignature(call.To, "accept-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	return svc.AcceptCall(id, answerCiphertext, answerNonce)
}

func (svc *TkmPhoneService) EndCallSigned(id uint64, number string, signature []byte) (PhoneCall, error) {
	svc.lock.RLock()
	call, ok := svc.calls[id]
	svc.lock.RUnlock()
	if !ok {
		return PhoneCall{}, errors.New("call not found")
	}
	if number != call.From && number != call.To {
		return PhoneCall{}, errors.New("number is not in call")
	}
	payload := svc.randomXServiceHash("end-call-payload", tkmPhoneUint64Bytes(id))
	if err := svc.verifyNumberOwnerSignature(number, "end-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	return svc.EndCall(id)
}

func (svc *TkmPhoneService) MessagesForNumber(number string) ([]PhoneMessage, error) {
	if _, err := svc.Number(number); err != nil {
		return nil, err
	}
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	out := make([]PhoneMessage, 0)
	for _, msg := range svc.messages {
		if msg.From == number || msg.To == number {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (svc *TkmPhoneService) CallsForNumber(number string) ([]PhoneCall, error) {
	if _, err := svc.Number(number); err != nil {
		return nil, err
	}
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	out := make([]PhoneCall, 0)
	for _, call := range svc.calls {
		if call.From == number || call.To == number {
			out = append(out, call)
		}
	}
	return out, nil
}

func (svc *TkmPhoneService) RegisterDeviceKey(number string, device string, publicKey []byte, signature []byte) (PhoneDeviceKey, error) {
	if device == "" {
		return PhoneDeviceKey{}, errors.New("device is required")
	}
	if len(publicKey) == 0 {
		return PhoneDeviceKey{}, errors.New("public key is required")
	}
	payload := svc.randomXServiceHash("device-key-payload", []byte(number), []byte(device), publicKey)
	if err := svc.verifyNumberOwnerSignature(number, "register-device", payload, signature); err != nil {
		return PhoneDeviceKey{}, err
	}
	key := PhoneDeviceKey{Number: number, Device: device, PublicKey: append([]byte(nil), publicKey...), CreatedAt: hexutil.Uint64(time.Now().Unix()), Active: true}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.devices[number] = append(svc.devices[number], key)
	if err := svc.saveLocked(); err != nil {
		return PhoneDeviceKey{}, err
	}
	return key, nil
}

func (svc *TkmPhoneService) TransferNumber(number string, newOwner common.Address, signature []byte) (PhoneNumber, error) {
	if newOwner == (common.Address{}) {
		return PhoneNumber{}, errors.New("new owner is required")
	}
	payload := svc.randomXServiceHash("transfer-number-payload", []byte(number), newOwner.Bytes())
	if err := svc.verifyNumberOwnerSignature(number, "transfer-number", payload, signature); err != nil {
		return PhoneNumber{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneNumber{}, errors.New("number not found")
	}
	record.Owner = newOwner
	svc.numbers[number] = record
	if err := svc.saveLocked(); err != nil {
		return PhoneNumber{}, err
	}
	return record, nil
}

func (svc *TkmPhoneService) RevokeNumber(number string, signature []byte) (PhoneNumber, error) {
	payload := svc.randomXServiceHash("revoke-number-payload", []byte(number))
	if err := svc.verifyNumberOwnerSignature(number, "revoke-number", payload, signature); err != nil {
		return PhoneNumber{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneNumber{}, errors.New("number not found")
	}
	record.Active = false
	svc.numbers[number] = record
	if err := svc.saveLocked(); err != nil {
		return PhoneNumber{}, err
	}
	return record, nil
}

func (svc *TkmPhoneService) AckMessage(id uint64, status PhoneMessageStatus, signature []byte) (PhoneMessage, error) {
	if status != PhoneMessageDelivered && status != PhoneMessageRead {
		return PhoneMessage{}, errors.New("invalid message status")
	}
	svc.lock.RLock()
	msg, ok := svc.messages[id]
	svc.lock.RUnlock()
	if !ok {
		return PhoneMessage{}, errors.New("message not found")
	}
	payload := svc.randomXServiceHash("ack-message-payload", tkmPhoneUint64Bytes(id), []byte(status))
	if err := svc.verifyNumberOwnerSignature(msg.To, "ack-message", payload, signature); err != nil {
		return PhoneMessage{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	msg = svc.messages[id]
	msg.Status = status
	svc.messages[id] = msg
	svc.addNotificationLocked(msg.From, "message-"+string(status), id, uint64(time.Now().Unix()))
	if err := svc.saveLocked(); err != nil {
		return PhoneMessage{}, err
	}
	return msg, nil
}

func (svc *TkmPhoneService) Notifications(number string) ([]PhoneNotification, error) {
	if _, err := svc.Number(number); err != nil {
		return nil, err
	}
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	out := make([]PhoneNotification, 0)
	for _, notif := range svc.notifs {
		if notif.Number == number {
			out = append(out, notif)
		}
	}
	return out, nil
}

func (svc *TkmPhoneService) Prune(retentionSeconds uint64, maxMessages int, maxCalls int) error {
	svc.lock.Lock()
	defer svc.lock.Unlock()
	cutoff := uint64(0)
	if retentionSeconds > 0 {
		now := uint64(time.Now().Unix())
		if now > retentionSeconds {
			cutoff = now - retentionSeconds
		}
	}
	for id, msg := range svc.messages {
		if cutoff > 0 && uint64(msg.CreatedAt) < cutoff {
			delete(svc.messages, id)
		}
	}
	for id, call := range svc.calls {
		if cutoff > 0 && uint64(call.StartedAt) < cutoff {
			delete(svc.calls, id)
		}
	}
	for maxMessages > 0 && len(svc.messages) > maxMessages {
		delete(svc.messages, oldestMessageID(svc.messages))
	}
	for maxCalls > 0 && len(svc.calls) > maxCalls {
		delete(svc.calls, oldestCallID(svc.calls))
	}
	return svc.saveLocked()
}

func (svc *TkmPhoneService) load() error {
	if svc.db == nil {
		return nil
	}
	data, err := svc.db.Get(tkmPhoneStateKey)
	if err != nil || len(data) == 0 {
		return nil
	}
	var snap tkmPhoneSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.nextID = snap.NextID
	svc.nextMsg = snap.NextMsg
	svc.nextCall = snap.NextCall
	svc.nextNotif = snap.NextNotif
	if snap.Operators != nil {
		svc.operators = snap.Operators
	}
	if snap.Numbers != nil {
		svc.numbers = snap.Numbers
	}
	if snap.Messages != nil {
		svc.messages = snap.Messages
	}
	if snap.Calls != nil {
		svc.calls = snap.Calls
	}
	if snap.Devices != nil {
		svc.devices = snap.Devices
	}
	if snap.Notifs != nil {
		svc.notifs = snap.Notifs
	}
	if snap.Rate != nil {
		svc.rate = snap.Rate
	}
	return nil
}

func (svc *TkmPhoneService) saveLocked() error {
	if svc.db == nil {
		return nil
	}
	snap := tkmPhoneSnapshot{
		NextID:    svc.nextID,
		NextMsg:   svc.nextMsg,
		NextCall:  svc.nextCall,
		NextNotif: svc.nextNotif,
		Operators: svc.operators,
		Numbers:   svc.numbers,
		Messages:  svc.messages,
		Calls:     svc.calls,
		Devices:   svc.devices,
		Notifs:    svc.notifs,
		Rate:      svc.rate,
	}
	data, err := json.Marshal(&snap)
	if err != nil {
		return err
	}
	if err := svc.db.Put(tkmPhoneStateKey, data); err != nil {
		return err
	}
	return svc.db.SyncKeyValue()
}

func (svc *TkmPhoneService) requireNumbers(from string, to string) error {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	fromNumber, ok := svc.numbers[from]
	if !ok || !fromNumber.Active {
		return errors.New("from number not found")
	}
	toNumber, ok := svc.numbers[to]
	if !ok || !toNumber.Active {
		return errors.New("to number not found")
	}
	return nil
}

func (svc *TkmPhoneService) validateOperatorPayment(operator common.Address, paymentTx common.Hash) error {
	if svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	_, tx := svc.eth.blockchain.GetCanonicalTransaction(paymentTx)
	if tx == nil {
		return errors.New("operator payment transaction is not canonical or indexed")
	}
	to := tx.To()
	if to == nil || *to != svc.mainKing {
		return errors.New("operator payment must be sent to main king")
	}
	if tx.Value().Cmp(tkmPhoneOperatorKeyPrice) != 0 {
		return errors.New("operator payment transaction must be exactly 5000 TKM")
	}
	config := svc.eth.blockchain.Config()
	signer, err := types.Sender(types.LatestSigner(config), tx)
	if err != nil {
		return err
	}
	if signer != operator {
		return fmt.Errorf("operator payment sent by %s, want %s", signer.Hex(), operator.Hex())
	}
	return nil
}

func (svc *TkmPhoneService) verifyNumberOwnerSignature(number string, action string, payload common.Hash, signature []byte) error {
	svc.lock.RLock()
	record, ok := svc.numbers[number]
	svc.lock.RUnlock()
	if !ok || !record.Active {
		return errors.New("number not found")
	}
	return verifyPhoneAddressSignature(record.Owner, svc.ownerActionHash(number, action, payload), signature)
}

func (svc *TkmPhoneService) ownerActionHash(number string, action string, payload common.Hash) common.Hash {
	return svc.randomXServiceHash("owner-action", []byte(number), []byte(action), payload.Bytes())
}

func verifyPhoneAddressSignature(want common.Address, digest common.Hash, signature []byte) error {
	if want == (common.Address{}) {
		return errors.New("owner address is not configured")
	}
	if len(signature) != crypto.SignatureLength {
		return fmt.Errorf("signature must be %d bytes", crypto.SignatureLength)
	}
	sig := append([]byte(nil), signature...)
	if sig[crypto.RecoveryIDOffset] >= 27 {
		sig[crypto.RecoveryIDOffset] -= 27
	}
	if sig[crypto.RecoveryIDOffset] > 1 {
		return fmt.Errorf("invalid signature recovery id %d", sig[crypto.RecoveryIDOffset])
	}
	if signer, err := tkmPhoneRecoverSigner(digest, sig); err == nil && signer == want {
		return nil
	}
	prefixed := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), digest.Bytes())
	signer, err := tkmPhoneRecoverSigner(prefixed, sig)
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}
	if signer != want {
		return fmt.Errorf("signature from %s, want %s", signer.Hex(), want.Hex())
	}
	return nil
}

func (svc *TkmPhoneService) checkRateLocked(key string, now uint64, limit int) error {
	windowStart := now - 60
	events := svc.rate[key][:0]
	for _, ts := range svc.rate[key] {
		if ts >= windowStart {
			events = append(events, ts)
		}
	}
	if len(events) >= limit {
		svc.rate[key] = events
		return errors.New("rate limit exceeded")
	}
	events = append(events, now)
	svc.rate[key] = events
	return nil
}

func (svc *TkmPhoneService) addNotificationLocked(number string, kind string, refID uint64, now uint64) {
	svc.nextNotif++
	svc.notifs[svc.nextNotif] = PhoneNotification{ID: hexutil.Uint64(svc.nextNotif), Number: number, Kind: kind, RefID: hexutil.Uint64(refID), CreatedAt: hexutil.Uint64(now)}
}

func oldestMessageID(messages map[uint64]PhoneMessage) uint64 {
	var oldest uint64
	var oldestTime uint64
	for id, msg := range messages {
		if oldest == 0 || uint64(msg.CreatedAt) < oldestTime {
			oldest = id
			oldestTime = uint64(msg.CreatedAt)
		}
	}
	return oldest
}

func oldestCallID(calls map[uint64]PhoneCall) uint64 {
	var oldest uint64
	var oldestTime uint64
	for id, call := range calls {
		if oldest == 0 || uint64(call.StartedAt) < oldestTime {
			oldest = id
			oldestTime = uint64(call.StartedAt)
		}
	}
	return oldest
}

func (svc *TkmPhoneService) operatorGrantHash(operator common.Address, keyHash common.Hash, expiresAt uint64, paymentTx common.Hash) common.Hash {
	return svc.randomXServiceHash("operator-grant", operator.Bytes(), keyHash.Bytes(), tkmPhoneUint64Bytes(expiresAt), paymentTx.Bytes(), tkmPhoneOperatorKeyPrice.Bytes())
}

func (svc *TkmPhoneService) messageKey(from string, to string, nonce []byte) common.Hash {
	return svc.randomXServiceHash("message-key", []byte(from), []byte(to), nonce)
}

func (svc *TkmPhoneService) randomXServiceHash(label string, parts ...[]byte) common.Hash {
	payload := []byte("TKMPHONE_RANDOMX_HASH_V1")
	payload = append(payload, []byte(label)...)
	payload = append(payload, svc.chainID.Bytes()...)
	if svc.eth != nil && svc.eth.blockchain != nil {
		head := svc.eth.blockchain.CurrentBlock()
		if head != nil {
			seed := miner.RandomXSeedHash(svc.eth.blockchain.Config(), head.Number.Uint64()+1)
			payload = append(payload, seed.Bytes()...)
		}
	}
	for _, part := range parts {
		payload = append(payload, tkmPhoneUint64Bytes(uint64(len(part)))...)
		payload = append(payload, part...)
	}
	return crypto.Keccak256Hash(payload)
}

func (svc *TkmPhoneService) verifyMainKingSignature(digest common.Hash, signature []byte) error {
	if svc.mainKing == (common.Address{}) {
		return errors.New("main king address is not configured")
	}
	if len(signature) != crypto.SignatureLength {
		return fmt.Errorf("signature must be %d bytes", crypto.SignatureLength)
	}
	sig := append([]byte(nil), signature...)
	if sig[crypto.RecoveryIDOffset] >= 27 {
		sig[crypto.RecoveryIDOffset] -= 27
	}
	if sig[crypto.RecoveryIDOffset] > 1 {
		return fmt.Errorf("invalid signature recovery id %d", sig[crypto.RecoveryIDOffset])
	}
	if signer, err := tkmPhoneRecoverSigner(digest, sig); err == nil && signer == svc.mainKing {
		return nil
	}
	prefixed := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), digest.Bytes())
	signer, err := tkmPhoneRecoverSigner(prefixed, sig)
	if err != nil {
		return fmt.Errorf("invalid main king signature: %w", err)
	}
	if signer != svc.mainKing {
		return fmt.Errorf("operator key signed by %s, want main king %s", signer.Hex(), svc.mainKing.Hex())
	}
	return nil
}

func tkmPhoneRecoverSigner(digest common.Hash, signature []byte) (common.Address, error) {
	pub, err := crypto.SigToPub(digest.Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}

func tkmPhoneUint64Bytes(v uint64) []byte {
	var out [8]byte
	binary.BigEndian.PutUint64(out[:], v)
	return out[:]
}

func tkmPhoneSeal(key common.Hash, nonce []byte, plaintext []byte, aad []byte) ([]byte, error) {
	gcm, err := tkmPhoneGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce must be %d bytes", gcm.NonceSize())
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nil
}

func tkmPhoneOpen(key common.Hash, nonce []byte, ciphertext []byte, aad []byte) ([]byte, error) {
	gcm, err := tkmPhoneGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce must be %d bytes", gcm.NonceSize())
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func tkmPhoneGCM(key common.Hash) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key.Bytes())
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
