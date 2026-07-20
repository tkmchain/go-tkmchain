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
	operators map[common.Address]PhoneOperatorKey
	numbers   map[string]PhoneNumber
	messages  map[uint64]PhoneMessage
	calls     map[uint64]PhoneCall
}

type tkmPhoneSnapshot struct {
	NextID    uint64
	NextMsg   uint64
	NextCall  uint64
	Operators map[common.Address]PhoneOperatorKey
	Numbers   map[string]PhoneNumber
	Messages  map[uint64]PhoneMessage
	Calls     map[uint64]PhoneCall
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
	ID          hexutil.Uint64 `json:"id"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	Ciphertext  hexutil.Bytes  `json:"ciphertext"`
	Nonce       hexutil.Bytes  `json:"nonce"`
	RandomXHash common.Hash    `json:"randomxHash"`
	CreatedAt   hexutil.Uint64 `json:"createdAt"`
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

func (api *TkmPhoneAPI) SendEncryptedMessage(from string, to string, ciphertext hexutil.Bytes, nonce hexutil.Bytes) (PhoneMessage, error) {
	return api.service.SendEncryptedMessage(from, to, []byte(ciphertext), []byte(nonce))
}

func (api *TkmPhoneAPI) StartCall(from string, to string, offerCiphertext hexutil.Bytes, offerNonce hexutil.Bytes) (PhoneCall, error) {
	return api.service.StartCall(from, to, []byte(offerCiphertext), []byte(offerNonce))
}

func (api *TkmPhoneAPI) AcceptCall(id hexutil.Uint64, answerCiphertext hexutil.Bytes, answerNonce hexutil.Bytes) (PhoneCall, error) {
	return api.service.AcceptCall(uint64(id), []byte(answerCiphertext), []byte(answerNonce))
}

func (api *TkmPhoneAPI) EndCall(id hexutil.Uint64) (PhoneCall, error) {
	return api.service.EndCall(uint64(id))
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
	if len(nonce) == 0 {
		return PhoneMessage{}, errors.New("nonce is required")
	}
	if err := svc.requireNumbers(from, to); err != nil {
		return PhoneMessage{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.nextMsg++
	msg := PhoneMessage{ID: hexutil.Uint64(svc.nextMsg), From: from, To: to, Ciphertext: append([]byte(nil), ciphertext...), Nonce: append([]byte(nil), nonce...), RandomXHash: svc.messageKey(from, to, nonce), CreatedAt: hexutil.Uint64(now)}
	svc.messages[svc.nextMsg] = msg
	if err := svc.saveLocked(); err != nil {
		return PhoneMessage{}, err
	}
	return msg, nil
}

func (svc *TkmPhoneService) StartCall(from string, to string, offerCiphertext []byte, offerNonce []byte) (PhoneCall, error) {
	if len(offerCiphertext) == 0 || len(offerNonce) == 0 {
		return PhoneCall{}, errors.New("encrypted call offer and nonce are required")
	}
	if err := svc.requireNumbers(from, to); err != nil {
		return PhoneCall{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.nextCall++
	call := PhoneCall{ID: hexutil.Uint64(svc.nextCall), From: from, To: to, OfferCiphertext: append([]byte(nil), offerCiphertext...), OfferNonce: append([]byte(nil), offerNonce...), OfferRandomXHash: svc.messageKey(from, to, offerNonce), State: PhoneCallRinging, StartedAt: hexutil.Uint64(now)}
	svc.calls[svc.nextCall] = call
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) AcceptCall(id uint64, answerCiphertext []byte, answerNonce []byte) (PhoneCall, error) {
	if len(answerCiphertext) == 0 || len(answerNonce) == 0 {
		return PhoneCall{}, errors.New("encrypted call answer and nonce are required")
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
	return call, nil
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
		Operators: svc.operators,
		Numbers:   svc.numbers,
		Messages:  svc.messages,
		Calls:     svc.calls,
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
