package eth

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	ethproto "github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	tkmPhoneMainKingNumberPrice           = new(big.Int).Mul(big.NewInt(5000), big.NewInt(params.Ether))
	tkmPhoneOperatorKeyPrice              = new(big.Int).Mul(big.NewInt(25000), big.NewInt(params.Ether))
	tkmPhoneDefaultNumberSalePrice        = new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))
	tkmPhoneBucketSize             uint64 = 5
	tkmPhoneBucketBatchSize        uint64 = 5
	tkmPhoneDefaultChainID                = big.NewInt(8979)
	tkmPhoneStateKey                      = []byte("tkmphone-state-v1")
	tkmPhoneMaxPayloadSize                = 64 * 1024
	tkmPhoneBucketPaymentScanLimit uint64 = 20000
	tkmPhoneMessageRateLimit              = 20
	tkmPhoneCallRateLimit                 = 10
	tkmPhoneCallCandidateRateLimit        = 120
)

var tkmPhoneBucketPaymentDataPrefix = []byte("TKMPHONE_BUCKET_V1")

type TkmPhoneAPI struct {
	service *TkmPhoneService
}

type PhoneForkStatus struct {
	Active              bool           `json:"active"`
	ActivationTimestamp hexutil.Uint64 `json:"activationTimestamp"`
	HeadNumber          hexutil.Uint64 `json:"headNumber"`
	HeadTimestamp       hexutil.Uint64 `json:"headTimestamp"`
	CurrentTimestamp    hexutil.Uint64 `json:"currentTimestamp"`
	UsingChainHead      bool           `json:"usingChainHead"`
}

type TkmPhoneService struct {
	lock           sync.RWMutex
	eth            *Ethereum
	mainKing       common.Address
	chainID        *big.Int
	db             ethdb.KeyValueStore
	phoneDir       string
	nextID         uint64
	nextMsg        uint64
	nextCall       uint64
	nextNotif      uint64
	nextBucket     uint64
	operators      map[common.Address]PhoneOperatorKey
	buckets        map[uint64]PhoneNumberBucket
	numbers        map[string]PhoneNumber
	messages       map[uint64]PhoneMessage
	calls          map[uint64]PhoneCall
	callSignals    map[uint64][]PhoneCallSignal
	devices        map[string][]PhoneDeviceKey
	notifs         map[uint64]PhoneNotification
	rate           map[string][]uint64
	contacts       map[string][]PhoneContact
	blocked        map[string]map[string]bool
	recovery       map[string]common.Address
	reports        map[uint64]PhoneFraudReport
	prop           map[uint64]PhonePropagation
	nextReport     uint64
	nextProp       uint64
	msgFeed        event.Feed
	callFeed       event.Feed
	callSignalFeed event.Feed
	notifFeed      event.Feed
}

type tkmPhoneSnapshot struct {
	NextID      uint64
	NextMsg     uint64
	NextCall    uint64
	NextNotif   uint64
	NextBucket  uint64
	Operators   map[common.Address]PhoneOperatorKey
	Buckets     map[uint64]PhoneNumberBucket
	Numbers     map[string]PhoneNumber
	Messages    map[uint64]PhoneMessage
	Calls       map[uint64]PhoneCall
	CallSignals map[uint64][]PhoneCallSignal
	Devices     map[string][]PhoneDeviceKey
	Notifs      map[uint64]PhoneNotification
	Rate        map[string][]uint64
	Contacts    map[string][]PhoneContact
	Blocked     map[string]map[string]bool
	Recovery    map[string]common.Address
	Reports     map[uint64]PhoneFraudReport
	Prop        map[uint64]PhonePropagation
	NextReport  uint64
	NextProp    uint64
}

type PhoneOperatorKey struct {
	Operator  common.Address `json:"operator"`
	KeyHash   common.Hash    `json:"keyHash"`
	PaymentTx common.Hash    `json:"paymentTx"`
	Paid      *hexutil.Big   `json:"paid"`
	ExpiresAt hexutil.Uint64 `json:"expiresAt"`
	Active    bool           `json:"active"`
	Numbers   hexutil.Uint64 `json:"numbers"`
	BucketID  hexutil.Uint64 `json:"bucketId"`
}

type PhonePendingOperatorApproval struct {
	Operator  common.Address `json:"operator"`
	KeyHash   common.Hash    `json:"keyHash"`
	ExpiresAt hexutil.Uint64 `json:"expiresAt"`
	PaymentTx common.Hash    `json:"paymentTx"`
	Paid      *hexutil.Big   `json:"paid"`
	GrantHash common.Hash    `json:"grantHash"`
	Block     hexutil.Uint64 `json:"block"`
	TxIndex   hexutil.Uint64 `json:"txIndex"`
	Approved  bool           `json:"approved"`
	Reason    string         `json:"reason,omitempty"`
}

type PhoneNumber struct {
	Number         string         `json:"number"`
	Owner          common.Address `json:"owner"`
	Operator       common.Address `json:"operator"`
	RandomX        common.Hash    `json:"randomxHash"`
	CreatedAt      hexutil.Uint64 `json:"createdAt"`
	Active         bool           `json:"active"`
	SalePrice      *hexutil.Big   `json:"salePrice"`
	SalePaymentTx  common.Hash    `json:"salePaymentTx"`
	SoldAt         hexutil.Uint64 `json:"soldAt"`
	BucketID       hexutil.Uint64 `json:"bucketId"`
	BucketHash     common.Hash    `json:"bucketHash"`
	IssuanceHash   common.Hash    `json:"issuanceHash"`
	TransferHash   common.Hash    `json:"transferHash"`
	OwnerHash      common.Hash    `json:"ownerHash"`
	MainKingIssued bool           `json:"mainKingIssued"`
	InUse          bool           `json:"inUse"`
	InUseAt        hexutil.Uint64 `json:"inUseAt"`
	UseHash        common.Hash    `json:"useHash"`
}

type PhoneNumberBucket struct {
	ID         hexutil.Uint64 `json:"id"`
	Round      hexutil.Uint64 `json:"round"`
	Index      hexutil.Uint64 `json:"index"`
	Hash       common.Hash    `json:"hash"`
	Seed       common.Hash    `json:"seed"`
	MainKing   common.Address `json:"mainKing"`
	Operator   common.Address `json:"operator"`
	PaymentTx  common.Hash    `json:"paymentTx"`
	CreationTx common.Hash    `json:"creationTx"`
	CreatedAt  hexutil.Uint64 `json:"createdAt"`
	AssignedAt hexutil.Uint64 `json:"assignedAt"`
	Signature  hexutil.Bytes  `json:"signature"`
	IssueHash  common.Hash    `json:"issueHash"`
	OwnerHash  common.Hash    `json:"ownerHash"`
	AssignHash common.Hash    `json:"assignHash"`
}

type PhoneOwnershipStep struct {
	Kind      string         `json:"kind"`
	From      common.Address `json:"from"`
	To        common.Address `json:"to"`
	PaymentTx common.Hash    `json:"paymentTx,omitempty"`
	BucketID  hexutil.Uint64 `json:"bucketId,omitempty"`
	Number    string         `json:"number,omitempty"`
	Hash      common.Hash    `json:"hash"`
	At        hexutil.Uint64 `json:"at,omitempty"`
}

type PhoneNumberOwnershipProof struct {
	Number           string               `json:"number"`
	BucketID         hexutil.Uint64       `json:"bucketId"`
	BucketHash       common.Hash          `json:"bucketHash"`
	CreationTx       common.Hash          `json:"creationTx"`
	SalePaymentTx    common.Hash          `json:"salePaymentTx"`
	MainKing         common.Address       `json:"mainKing"`
	Operator         common.Address       `json:"operator"`
	CurrentOwner     common.Address       `json:"currentOwner"`
	IssuanceHash     common.Hash          `json:"issuanceHash"`
	BucketOwnerHash  common.Hash          `json:"bucketOwnerHash"`
	BucketAssignHash common.Hash          `json:"bucketAssignHash"`
	NumberOwnerHash  common.Hash          `json:"numberOwnerHash"`
	TransferHash     common.Hash          `json:"transferHash"`
	InUse            bool                 `json:"inUse"`
	InUseAt          hexutil.Uint64       `json:"inUseAt"`
	UseHash          common.Hash          `json:"useHash"`
	ProofHash        common.Hash          `json:"proofHash"`
	Steps            []PhoneOwnershipStep `json:"steps"`
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
	ExpiresAt   hexutil.Uint64     `json:"expiresAt"`
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

type RegisteredPhoneNumber struct {
	Number      PhoneNumber      `json:"number"`
	Registered  bool             `json:"registered"`
	DeviceCount hexutil.Uint64   `json:"deviceCount"`
	Devices     []PhoneDeviceKey `json:"devices"`
}

type PhoneNotification struct {
	ID        hexutil.Uint64 `json:"id"`
	Number    string         `json:"number"`
	Kind      string         `json:"kind"`
	RefID     hexutil.Uint64 `json:"refId"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
}

type PhoneDeviceEnvelope struct {
	Device     string        `json:"device"`
	Ciphertext hexutil.Bytes `json:"ciphertext"`
	Nonce      hexutil.Bytes `json:"nonce"`
}

type PhoneContact struct {
	OwnerNumber string         `json:"ownerNumber"`
	PeerNumber  string         `json:"peerNumber"`
	Ciphertext  hexutil.Bytes  `json:"ciphertext"`
	Nonce       hexutil.Bytes  `json:"nonce"`
	CreatedAt   hexutil.Uint64 `json:"createdAt"`
}

type PhoneFraudReport struct {
	ID        hexutil.Uint64 `json:"id"`
	Operator  common.Address `json:"operator"`
	Reporter  string         `json:"reporter"`
	Reason    string         `json:"reason"`
	Evidence  common.Hash    `json:"evidence"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
}

type PhonePropagation struct {
	ID        hexutil.Uint64 `json:"id"`
	Kind      string         `json:"kind"`
	RefID     hexutil.Uint64 `json:"refId"`
	Hash      common.Hash    `json:"hash"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
	Payload   hexutil.Bytes  `json:"payload"`
}

type phoneBlockRecord struct {
	OwnerNumber   string         `json:"ownerNumber"`
	BlockedNumber string         `json:"blockedNumber"`
	CreatedAt     hexutil.Uint64 `json:"createdAt"`
}

type phoneRecoveryRecord struct {
	Number    string         `json:"number"`
	Recovery  common.Address `json:"recovery"`
	NewOwner  common.Address `json:"newOwner"`
	CreatedAt hexutil.Uint64 `json:"createdAt"`
}

type PhoneCallState string

const (
	PhoneCallRinging  PhoneCallState = "ringing"
	PhoneCallActive   PhoneCallState = "active"
	PhoneCallEnded    PhoneCallState = "ended"
	PhoneCallRejected PhoneCallState = "rejected"
	PhoneCallMissed   PhoneCallState = "missed"
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
	EndReason         string         `json:"endReason"`
	StartedAt         hexutil.Uint64 `json:"startedAt"`
	AnsweredAt        hexutil.Uint64 `json:"answeredAt"`
	EndedAt           hexutil.Uint64 `json:"endedAt"`
	ExpiresAt         hexutil.Uint64 `json:"expiresAt"`
}

type PhoneICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username"`
	Credential string   `json:"credential"`
}

type PhoneWebRTCConfig struct {
	AudioOnly              bool             `json:"audioOnly"`
	RequiredEncryption     bool             `json:"requiredEncryption"`
	OfferAction            string           `json:"offerAction"`
	AnswerAction           string           `json:"answerAction"`
	CandidateAction        string           `json:"candidateAction"`
	CandidateListAction    string           `json:"candidateListAction"`
	MaxSignalBytes         hexutil.Uint64   `json:"maxSignalBytes"`
	CandidateRateLimit     hexutil.Uint64   `json:"candidateRateLimitPerMinute"`
	RecommendedRingSeconds hexutil.Uint64   `json:"recommendedRingSeconds"`
	ICEServers             []PhoneICEServer `json:"iceServers"`
}

type PhoneCallSignal struct {
	CallID      hexutil.Uint64 `json:"callId"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	Kind        string         `json:"kind"`
	Ciphertext  hexutil.Bytes  `json:"ciphertext"`
	Nonce       hexutil.Bytes  `json:"nonce"`
	RandomXHash common.Hash    `json:"randomxHash"`
	CreatedAt   hexutil.Uint64 `json:"createdAt"`
}

func NewTkmPhoneAPI(e *Ethereum) *TkmPhoneAPI {
	return &TkmPhoneAPI{service: e.tkmPhoneService()}
}

func NewTkmPhoneService(e *Ethereum, mainKing common.Address, chainID *big.Int) *TkmPhoneService {
	return NewTkmPhoneServiceWithDB(e, mainKing, chainID, nil)
}

func NewTkmPhoneServiceWithDB(e *Ethereum, mainKing common.Address, chainID *big.Int, db ethdb.KeyValueStore) *TkmPhoneService {
	return NewTkmPhoneServiceWithDBAndDir(e, mainKing, chainID, db, "")
}

func NewTkmPhoneServiceWithDBAndDir(e *Ethereum, mainKing common.Address, chainID *big.Int, db ethdb.KeyValueStore, phoneDir string) *TkmPhoneService {
	if chainID == nil {
		chainID = tkmPhoneDefaultChainID
	}
	svc := &TkmPhoneService{
		eth:         e,
		mainKing:    mainKing,
		chainID:     new(big.Int).Set(chainID),
		db:          db,
		phoneDir:    phoneDir,
		operators:   make(map[common.Address]PhoneOperatorKey),
		buckets:     make(map[uint64]PhoneNumberBucket),
		numbers:     make(map[string]PhoneNumber),
		messages:    make(map[uint64]PhoneMessage),
		calls:       make(map[uint64]PhoneCall),
		callSignals: make(map[uint64][]PhoneCallSignal),
		devices:     make(map[string][]PhoneDeviceKey),
		notifs:      make(map[uint64]PhoneNotification),
		rate:        make(map[string][]uint64),
		contacts:    make(map[string][]PhoneContact),
		blocked:     make(map[string]map[string]bool),
		recovery:    make(map[string]common.Address),
		reports:     make(map[uint64]PhoneFraudReport),
		prop:        make(map[uint64]PhonePropagation),
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
		s.phoneService = NewTkmPhoneServiceWithDBAndDir(s, s.GetMainKingAddress(), chainID, s.chainDb, s.phoneDir)
	}
	return s.phoneService
}

func (api *TkmPhoneAPI) OperatorKeyPrice() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(tkmPhoneOperatorKeyPrice))
}

func (api *TkmPhoneAPI) BucketPrice() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(tkmPhoneOperatorKeyPrice))
}

func (api *TkmPhoneAPI) MainKingNumberPrice() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(tkmPhoneMainKingNumberPrice))
}

func (api *TkmPhoneAPI) NumberSalePrice() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(tkmPhoneDefaultNumberSalePrice))
}

func (api *TkmPhoneAPI) Status() PhoneForkStatus {
	return api.service.Status()
}

func (api *TkmPhoneAPI) BucketGenerationHash(round hexutil.Uint64, seed common.Hash, creationTx common.Hash) common.Hash {
	return api.service.bucketGenerationHash(uint64(round), seed, creationTx)
}

func (api *TkmPhoneAPI) NextBucketRound() hexutil.Uint64 {
	return hexutil.Uint64(api.service.NextBucketRound())
}

func (api *TkmPhoneAPI) GenerateBuckets(seed common.Hash, creationTx common.Hash, signature hexutil.Bytes) ([]PhoneNumberBucket, error) {
	return api.service.GenerateBuckets(seed, creationTx, []byte(signature))
}

func (api *TkmPhoneAPI) Buckets() []PhoneNumberBucket { return api.service.Buckets() }

func (api *TkmPhoneAPI) OperatorGrantHash(operator common.Address, keyHash common.Hash, expiresAt hexutil.Uint64, paymentTx common.Hash) common.Hash {
	return api.service.operatorGrantHash(operator, keyHash, uint64(expiresAt), paymentTx)
}

func (api *TkmPhoneAPI) RegisterOperatorKey(operator common.Address, keyHash common.Hash, expiresAt hexutil.Uint64, paymentTx common.Hash, paid hexutil.Big, signature hexutil.Bytes) (PhoneOperatorKey, error) {
	return api.service.RegisterOperatorKey(operator, keyHash, uint64(expiresAt), paymentTx, (*big.Int)(&paid), []byte(signature))
}

func (api *TkmPhoneAPI) GenerateNumber(operator common.Address, owner common.Address, label string) (PhoneNumber, error) {
	return api.service.GenerateNumber(operator, owner, label)
}

func (api *TkmPhoneAPI) OpenBucket(operator common.Address, bucketID hexutil.Uint64, signature hexutil.Bytes) ([]PhoneNumber, error) {
	return api.service.OpenBucket(operator, uint64(bucketID), []byte(signature))
}

func (api *TkmPhoneAPI) OpenBucketHash(operator common.Address, bucketID hexutil.Uint64) common.Hash {
	return api.service.openBucketHash(operator, uint64(bucketID))
}

func (api *TkmPhoneAPI) OperatorInventory(operator common.Address, bucketID hexutil.Uint64, signature hexutil.Bytes) ([]PhoneNumber, error) {
	return api.service.OpenBucket(operator, uint64(bucketID), []byte(signature))
}

func (api *TkmPhoneAPI) SellNumber(operator common.Address, number string, buyer common.Address, price hexutil.Big, paymentTx common.Hash) (PhoneNumber, error) {
	return api.service.SellNumber(operator, number, buyer, (*big.Int)(&price), paymentTx)
}

func (api *TkmPhoneAPI) UseNumber(number string, signature hexutil.Bytes) (PhoneNumber, error) {
	return api.service.UseNumber(number, []byte(signature))
}

func (api *TkmPhoneAPI) UseNumberSigningHash(number string) common.Hash {
	return api.service.useNumberSigningHash(number)
}

func (api *TkmPhoneAPI) Number(number string) (PhoneNumber, error) {
	return api.service.Number(number)
}

func (api *TkmPhoneAPI) NumberOwnershipProof(number string) (PhoneNumberOwnershipProof, error) {
	return api.service.NumberOwnershipProof(number)
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

func (api *TkmPhoneAPI) SendMessageSigningHash(from string, to string, nonce hexutil.Bytes, ciphertext hexutil.Bytes) common.Hash {
	return api.service.sendMessageSigningHash(from, to, []byte(nonce), []byte(ciphertext))
}

func (api *TkmPhoneAPI) StartCallSigningHash(from string, to string, offerNonce hexutil.Bytes, offerCiphertext hexutil.Bytes) common.Hash {
	return api.service.startCallSigningHash(from, to, []byte(offerNonce), []byte(offerCiphertext))
}

func (api *TkmPhoneAPI) StartCall(from string, to string, offerCiphertext hexutil.Bytes, offerNonce hexutil.Bytes, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.StartCallSigned(from, to, []byte(offerCiphertext), []byte(offerNonce), []byte(signature))
}

func (api *TkmPhoneAPI) AcceptCall(id hexutil.Uint64, answerCiphertext hexutil.Bytes, answerNonce hexutil.Bytes, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.AcceptCallSigned(uint64(id), []byte(answerCiphertext), []byte(answerNonce), []byte(signature))
}

func (api *TkmPhoneAPI) AcceptCallSigningHash(id hexutil.Uint64, answerNonce hexutil.Bytes, answerCiphertext hexutil.Bytes) (common.Hash, error) {
	return api.service.acceptCallSigningHash(uint64(id), []byte(answerNonce), []byte(answerCiphertext))
}

func (api *TkmPhoneAPI) RejectCall(id hexutil.Uint64, number string, reason string, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.RejectCallSigned(uint64(id), number, reason, []byte(signature))
}

func (api *TkmPhoneAPI) RejectCallSigningHash(id hexutil.Uint64, number string, reason string) common.Hash {
	return api.service.rejectCallSigningHash(uint64(id), number, reason)
}

func (api *TkmPhoneAPI) ExpireRingingCalls(timeoutSeconds hexutil.Uint64) ([]PhoneCall, error) {
	return api.service.ExpireRingingCalls(uint64(timeoutSeconds))
}

func (api *TkmPhoneAPI) WebRTCConfig() PhoneWebRTCConfig {
	return api.service.WebRTCConfig()
}

func (api *TkmPhoneAPI) EndCall(id hexutil.Uint64, number string, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.EndCallSigned(uint64(id), number, []byte(signature))
}

func (api *TkmPhoneAPI) EndCallSigningHash(id hexutil.Uint64, number string) common.Hash {
	return api.service.endCallSigningHash(uint64(id), number)
}

func (api *TkmPhoneAPI) CallCandidateHash(id hexutil.Uint64, number string, nonce hexutil.Bytes, ciphertext hexutil.Bytes) common.Hash {
	return api.service.callCandidateHash(uint64(id), number, []byte(nonce), []byte(ciphertext))
}

func (api *TkmPhoneAPI) CallCandidateSigningHash(id hexutil.Uint64, number string, nonce hexutil.Bytes, ciphertext hexutil.Bytes) common.Hash {
	return api.service.callCandidateSigningHash(uint64(id), number, []byte(nonce), []byte(ciphertext))
}

func (api *TkmPhoneAPI) CallCandidateListHash(id hexutil.Uint64, number string) common.Hash {
	return api.service.callCandidateListHash(uint64(id), number)
}

func (api *TkmPhoneAPI) CallCandidateListSigningHash(id hexutil.Uint64, number string) common.Hash {
	return api.service.callCandidateListSigningHash(uint64(id), number)
}

func (api *TkmPhoneAPI) AddCallCandidate(id hexutil.Uint64, number string, ciphertext hexutil.Bytes, nonce hexutil.Bytes, signature hexutil.Bytes) (PhoneCallSignal, error) {
	return api.service.AddCallCandidateSigned(uint64(id), number, []byte(ciphertext), []byte(nonce), []byte(signature))
}

func (api *TkmPhoneAPI) CallCandidates(id hexutil.Uint64, number string, signature hexutil.Bytes) ([]PhoneCallSignal, error) {
	return api.service.CallCandidatesSigned(uint64(id), number, []byte(signature))
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

func (api *TkmPhoneAPI) DeviceKeys(number string) ([]PhoneDeviceKey, error) {
	return api.service.DeviceKeys(number)
}

func (api *TkmPhoneAPI) RegisteredNumber(number string) (RegisteredPhoneNumber, error) {
	return api.service.RegisteredNumber(number)
}

func (api *TkmPhoneAPI) RegisteredNumbers() []RegisteredPhoneNumber {
	return api.service.RegisteredNumbers()
}

func (api *TkmPhoneAPI) DeviceKeySigningHash(number string, device string, publicKey hexutil.Bytes) common.Hash {
	return api.service.deviceKeySigningHash(number, device, []byte(publicKey))
}

func (api *TkmPhoneAPI) TransferNumber(number string, newOwner common.Address, signature hexutil.Bytes) (PhoneNumber, error) {
	return api.service.TransferNumber(number, newOwner, []byte(signature))
}

func (api *TkmPhoneAPI) TransferNumberSigningHash(number string, newOwner common.Address) common.Hash {
	return api.service.transferNumberSigningHash(number, newOwner)
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

func (api *TkmPhoneAPI) SendEncryptedMessageWithExpiry(from string, to string, ciphertext hexutil.Bytes, nonce hexutil.Bytes, expiresAt hexutil.Uint64, signature hexutil.Bytes) (PhoneMessage, error) {
	return api.service.SendEncryptedMessageWithExpiry(from, to, []byte(ciphertext), []byte(nonce), uint64(expiresAt), []byte(signature))
}

func (api *TkmPhoneAPI) StartCallWithExpiry(from string, to string, offerCiphertext hexutil.Bytes, offerNonce hexutil.Bytes, expiresAt hexutil.Uint64, signature hexutil.Bytes) (PhoneCall, error) {
	return api.service.StartCallWithExpiry(from, to, []byte(offerCiphertext), []byte(offerNonce), uint64(expiresAt), []byte(signature))
}

func (api *TkmPhoneAPI) EncryptPayloadForDevices(from string, to string, nonce hexutil.Bytes, plaintext hexutil.Bytes) ([]PhoneDeviceEnvelope, error) {
	return api.service.EncryptPayloadForDevices(from, to, []byte(nonce), []byte(plaintext))
}

func (api *TkmPhoneAPI) ListOperators() []PhoneOperatorKey { return api.service.ListOperators() }
func (api *TkmPhoneAPI) PendingOperatorApprovals(scanBlocks hexutil.Uint64) []PhonePendingOperatorApproval {
	return api.service.PendingOperatorApprovals(uint64(scanBlocks))
}
func (api *TkmPhoneAPI) ApproveOperatorPayment(paymentTx common.Hash, signature hexutil.Bytes) (PhoneOperatorKey, error) {
	return api.service.ApproveOperatorPayment(paymentTx, []byte(signature))
}
func (api *TkmPhoneAPI) ReportOperator(operator common.Address, reporter string, reason string, evidence common.Hash, signature hexutil.Bytes) (PhoneFraudReport, error) {
	return api.service.ReportOperator(operator, reporter, reason, evidence, []byte(signature))
}
func (api *TkmPhoneAPI) AddContact(ownerNumber string, peerNumber string, ciphertext hexutil.Bytes, nonce hexutil.Bytes, signature hexutil.Bytes) (PhoneContact, error) {
	return api.service.AddContact(ownerNumber, peerNumber, []byte(ciphertext), []byte(nonce), []byte(signature))
}
func (api *TkmPhoneAPI) Contacts(number string) ([]PhoneContact, error) {
	return api.service.Contacts(number)
}
func (api *TkmPhoneAPI) BlockNumber(ownerNumber string, blockedNumber string, signature hexutil.Bytes) (bool, error) {
	return true, api.service.BlockNumber(ownerNumber, blockedNumber, []byte(signature))
}
func (api *TkmPhoneAPI) UnblockNumber(ownerNumber string, blockedNumber string, signature hexutil.Bytes) (bool, error) {
	return true, api.service.UnblockNumber(ownerNumber, blockedNumber, []byte(signature))
}
func (api *TkmPhoneAPI) RegisterRecovery(number string, recovery common.Address, signature hexutil.Bytes) (bool, error) {
	return true, api.service.RegisterRecovery(number, recovery, []byte(signature))
}
func (api *TkmPhoneAPI) RecoverNumber(number string, newOwner common.Address, signature hexutil.Bytes) (PhoneNumber, error) {
	return api.service.RecoverNumber(number, newOwner, []byte(signature))
}
func (api *TkmPhoneAPI) PropagationQueue() []PhonePropagation { return api.service.PropagationQueue() }
func (api *TkmPhoneAPI) ImportPropagation(prop PhonePropagation) (bool, error) {
	return true, api.service.ImportPropagation(prop)
}
func (api *TkmPhoneAPI) NewMessages(ctx context.Context) (*rpc.Subscription, error) {
	return api.service.subscribe(ctx, "message")
}
func (api *TkmPhoneAPI) CallUpdates(ctx context.Context) (*rpc.Subscription, error) {
	return api.service.subscribe(ctx, "call")
}
func (api *TkmPhoneAPI) CallSignals(ctx context.Context) (*rpc.Subscription, error) {
	return api.service.subscribe(ctx, "call-signal")
}
func (api *TkmPhoneAPI) NewNotifications(ctx context.Context) (*rpc.Subscription, error) {
	return api.service.subscribe(ctx, "notification")
}

func (s *Ethereum) broadcastTkmPhonePropagation(prop PhonePropagation) {
	s.broadcastTkmPhonePropagationExcept(prop, "")
}

func (s *Ethereum) broadcastTkmPhonePropagationExcept(prop PhonePropagation, skip string) {
	if s == nil || s.handler == nil {
		return
	}
	if len(prop.Payload) > tkmPhoneMaxPayloadSize {
		return
	}
	peers := s.handler.peers.all()
	if len(peers) == 0 {
		return
	}
	packet := phonePropagationPacket(prop)
	for _, peer := range peers {
		if skip != "" && peer.ID() == skip {
			continue
		}
		if err := peer.SendTkmPhonePropagation(packet); err != nil {
			log.Debug("Failed to announce TKM Phone propagation", "peer", peer.ID(), "kind", prop.Kind, "id", uint64(prop.ID), "err", err)
		}
	}
}

func (svc *TkmPhoneService) Status() PhoneForkStatus {
	status := PhoneForkStatus{CurrentTimestamp: hexutil.Uint64(time.Now().Unix())}
	if svc == nil || svc.eth == nil || svc.eth.blockchain == nil || svc.eth.blockchain.Config() == nil {
		status.Active = true
		return status
	}
	cfg := svc.eth.blockchain.Config()
	if cfg.PhoneTime != nil {
		status.ActivationTimestamp = hexutil.Uint64(*cfg.PhoneTime)
	}
	if head := svc.eth.blockchain.CurrentHeader(); head != nil {
		status.HeadNumber = hexutil.Uint64(head.Number.Uint64())
		status.HeadTimestamp = hexutil.Uint64(head.Time)
		status.UsingChainHead = true
		status.Active = cfg.IsPhone(head.Number, head.Time)
		return status
	}
	status.Active = cfg.IsPhone(big.NewInt(0), uint64(status.CurrentTimestamp))
	return status
}

func (svc *TkmPhoneService) requirePhoneForkActive() error {
	if svc == nil {
		return errors.New("tkm phone service is not available")
	}
	status := svc.Status()
	if status.Active {
		return nil
	}
	if status.ActivationTimestamp == 0 {
		return errors.New("tkm phone hardfork is not configured")
	}
	if status.UsingChainHead {
		return fmt.Errorf("tkm phone hardfork is not active yet: head timestamp %d, activation timestamp %d", status.HeadTimestamp, status.ActivationTimestamp)
	}
	return fmt.Errorf("tkm phone hardfork is not active yet: current timestamp %d, activation timestamp %d", status.CurrentTimestamp, status.ActivationTimestamp)
}

func (s *Ethereum) noteTkmPhonePropagationFromPeer(packet ethproto.TkmPhonePropagationPacket, peerID string) {
	if s == nil {
		return
	}
	svc := s.tkmPhoneService()
	if svc == nil {
		return
	}
	prop := phonePropagationFromPacket(packet)
	if svc.hasPropagation(prop.ID, prop.Hash) {
		return
	}
	if err := svc.ImportPropagation(prop); err != nil {
		log.Debug("Rejected TKM Phone propagation", "peer", peerID, "kind", prop.Kind, "id", uint64(prop.ID), "err", err)
		return
	}
	s.broadcastTkmPhonePropagationExcept(prop, peerID)
}

func (svc *TkmPhoneService) RegisterOperatorKey(operator common.Address, keyHash common.Hash, expiresAt uint64, paymentTx common.Hash, paid *big.Int, signature []byte) (PhoneOperatorKey, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneOperatorKey{}, err
	}
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
		return PhoneOperatorKey{}, errors.New("operator bucket purchase requires exactly 25000 TKM")
	}
	now := uint64(time.Now().Unix())
	if expiresAt <= now {
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
		Numbers:   hexutil.Uint64(tkmPhoneBucketSize),
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if existing, ok := svc.operators[operator]; ok && existing.Active && uint64(existing.ExpiresAt) > now {
		return PhoneOperatorKey{}, errors.New("operator key is already active")
	}
	bucket, err := svc.assignNextBucketLocked(operator, paymentTx, now)
	if err != nil {
		return PhoneOperatorKey{}, err
	}
	key.BucketID = bucket.ID
	svc.operators[operator] = key
	svc.addPropagationLocked("operator-key", 0, keyHash, now, key)
	if err := svc.saveLocked(); err != nil {
		return PhoneOperatorKey{}, err
	}
	return key, nil
}

func (svc *TkmPhoneService) GenerateNumber(operator common.Address, owner common.Address, label string) (PhoneNumber, error) {
	return PhoneNumber{}, errors.New("phone numbers can only be generated by main king bucket issuance")
}

func (svc *TkmPhoneService) assignNextBucketLocked(operator common.Address, paymentTx common.Hash, now uint64) (PhoneNumberBucket, error) {
	ids := make([]uint64, 0, len(svc.buckets))
	for id := range svc.buckets {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		bucket := svc.buckets[id]
		if uint64(bucket.AssignedAt) != 0 {
			continue
		}
		bucket.Operator = operator
		bucket.PaymentTx = paymentTx
		bucket.AssignedAt = hexutil.Uint64(now)
		svc.refreshBucketOwnershipHashesLocked(&bucket)
		svc.buckets[id] = bucket
		for number, record := range svc.numbers {
			if record.BucketID == bucket.ID && record.Owner == svc.mainKing && record.MainKingIssued {
				record.Owner = operator
				record.Operator = operator
				svc.numbers[number] = record
			}
		}
		svc.addPropagationLocked("bucket-assigned", id, bucket.Hash, now, bucket)
		return bucket, nil
	}
	return PhoneNumberBucket{}, errors.New("no unsold main king phone-number bucket is available")
}

func (svc *TkmPhoneService) GenerateBuckets(seed common.Hash, creationTx common.Hash, signature []byte) ([]PhoneNumberBucket, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return nil, err
	}
	if seed == (common.Hash{}) {
		return nil, errors.New("bucket seed is required")
	}
	if creationTx == (common.Hash{}) {
		return nil, errors.New("bucket creation transaction hash is required")
	}
	if err := svc.validateBucketCreationTx(creationTx); err != nil {
		return nil, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	for _, bucket := range svc.buckets {
		if uint64(bucket.AssignedAt) == 0 {
			return nil, errors.New("existing bucket batch is not completely bought")
		}
	}
	round := svc.nextBucket/tkmPhoneBucketBatchSize + 1
	if err := svc.verifyMainKingSignature(svc.bucketGenerationHash(round, seed, creationTx), signature); err != nil {
		return nil, err
	}
	now := uint64(time.Now().Unix())
	out := make([]PhoneNumberBucket, 0, tkmPhoneBucketBatchSize)
	for i := uint64(0); i < tkmPhoneBucketBatchSize; i++ {
		svc.nextBucket++
		bucketHash := svc.randomXServiceHash("phone-bucket", svc.mainKing.Bytes(), seed.Bytes(), creationTx.Bytes(), tkmPhoneUint64Bytes(round), tkmPhoneUint64Bytes(i+1), tkmPhoneUint64Bytes(svc.nextBucket))
		bucket := PhoneNumberBucket{ID: hexutil.Uint64(svc.nextBucket), Round: hexutil.Uint64(round), Index: hexutil.Uint64(i + 1), Hash: bucketHash, Seed: seed, MainKing: svc.mainKing, CreationTx: creationTx, CreatedAt: hexutil.Uint64(now), Signature: append([]byte(nil), signature...)}
		svc.refreshBucketOwnershipHashesLocked(&bucket)
		svc.buckets[svc.nextBucket] = bucket
		svc.addPropagationLocked("bucket", svc.nextBucket, bucketHash, now, bucket)
		for n := uint64(0); n < tkmPhoneBucketSize; n++ {
			if _, err := svc.generateNumberLocked(bucket, n+1, now); err != nil {
				return nil, err
			}
		}
		out = append(out, bucket)
	}
	if err := svc.saveLocked(); err != nil {
		return nil, err
	}
	return out, nil
}

func (svc *TkmPhoneService) Buckets() []PhoneNumberBucket {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	out := make([]PhoneNumberBucket, 0, len(svc.buckets))
	for _, bucket := range svc.buckets {
		svc.refreshBucketOwnershipHashesLocked(&bucket)
		out = append(out, bucket)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (svc *TkmPhoneService) NextBucketRound() uint64 {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	return svc.nextBucket/tkmPhoneBucketBatchSize + 1
}

func (svc *TkmPhoneService) OpenBucket(operator common.Address, bucketID uint64, signature []byte) ([]PhoneNumber, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return nil, err
	}
	if operator == (common.Address{}) {
		return nil, errors.New("operator address is required")
	}
	if bucketID == 0 {
		return nil, errors.New("bucket id is required")
	}
	payload := svc.randomXServiceHash("open-bucket-payload", operator.Bytes(), tkmPhoneUint64Bytes(bucketID))
	if err := verifyPhoneAddressSignature(operator, payload, signature); err != nil {
		return nil, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	key, ok := svc.operators[operator]
	if !ok || !key.Active || uint64(key.ExpiresAt) <= now {
		return nil, errors.New("operator key is not active")
	}
	if uint64(key.BucketID) != bucketID {
		return nil, errors.New("bucket is not assigned to operator")
	}
	bucket, ok := svc.buckets[bucketID]
	if !ok || bucket.Operator != operator || uint64(bucket.AssignedAt) == 0 {
		return nil, errors.New("bucket is not assigned to operator")
	}
	out := make([]PhoneNumber, 0, tkmPhoneBucketSize)
	for _, number := range svc.numbers {
		if uint64(number.BucketID) == bucketID && number.Operator == operator && number.Owner == operator && number.Active && uint64(number.SoldAt) == 0 && !number.InUse {
			svc.refreshNumberOwnershipHashesLocked(&number)
			out = append(out, number)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (svc *TkmPhoneService) operatorInventoryForTest(operator common.Address) ([]PhoneNumber, error) {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	key, ok := svc.operators[operator]
	if !ok || !key.Active {
		return nil, errors.New("operator key is not active")
	}
	out := make([]PhoneNumber, 0, tkmPhoneBucketSize)
	for _, number := range svc.numbers {
		if uint64(number.BucketID) == uint64(key.BucketID) && number.Operator == operator && number.Owner == operator && number.Active && uint64(number.SoldAt) == 0 && !number.InUse {
			svc.refreshNumberOwnershipHashesLocked(&number)
			out = append(out, number)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (svc *TkmPhoneService) SellNumber(operator common.Address, number string, buyer common.Address, price *big.Int, paymentTx common.Hash) (PhoneNumber, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneNumber{}, err
	}
	if buyer == (common.Address{}) {
		return PhoneNumber{}, errors.New("buyer address is required")
	}
	if price == nil || price.Cmp(tkmPhoneDefaultNumberSalePrice) != 0 {
		return PhoneNumber{}, errors.New("phone number sale requires exactly 10000 TKM")
	}
	if paymentTx == (common.Hash{}) {
		return PhoneNumber{}, errors.New("phone number sale payment transaction is required")
	}
	if err := svc.validateNumberSalePayment(operator, buyer, price, paymentTx); err != nil {
		return PhoneNumber{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	key, ok := svc.operators[operator]
	if !ok || !key.Active || uint64(key.ExpiresAt) <= now {
		return PhoneNumber{}, errors.New("operator key is not active")
	}
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneNumber{}, errors.New("number not found")
	}
	if !record.MainKingIssued || uint64(record.BucketID) == 0 || record.BucketHash == (common.Hash{}) {
		return PhoneNumber{}, errors.New("number was not issued by main king bucket")
	}
	bucket, ok := svc.buckets[uint64(record.BucketID)]
	if !ok || bucket.Hash != record.BucketHash || bucket.Operator != operator || uint64(bucket.AssignedAt) == 0 {
		return PhoneNumber{}, errors.New("number bucket is not assigned to operator")
	}
	if record.Operator != operator {
		return PhoneNumber{}, errors.New("number is not assigned to operator")
	}
	if record.Owner != operator || uint64(record.SoldAt) != 0 {
		return PhoneNumber{}, errors.New("number is already sold")
	}
	if record.InUse {
		return PhoneNumber{}, errors.New("number is permanently in use and cannot be sold")
	}
	record.Owner = buyer
	record.SalePrice = (*hexutil.Big)(new(big.Int).Set(price))
	record.SalePaymentTx = paymentTx
	record.SoldAt = hexutil.Uint64(now)
	svc.refreshNumberOwnershipHashesLocked(&record)
	svc.numbers[number] = record
	svc.addPropagationLocked("number-sold", 0, svc.randomXServiceHash("number-sold", []byte(number), buyer.Bytes(), price.Bytes(), paymentTx.Bytes()), now, record)
	if err := svc.saveLocked(); err != nil {
		return PhoneNumber{}, err
	}
	return record, nil
}

func (svc *TkmPhoneService) generateNumberLocked(bucket PhoneNumberBucket, index uint64, now uint64) (PhoneNumber, error) {
	for {
		svc.nextID++
		rxh := svc.randomXServiceHash("number", bucket.Hash.Bytes(), tkmPhoneUint64Bytes(index), tkmPhoneUint64Bytes(svc.nextID))
		number := fmt.Sprintf("+8979%011d", new(big.Int).SetBytes(rxh.Bytes()).Uint64()%100000000000)
		if _, exists := svc.numbers[number]; exists {
			continue
		}
		record := PhoneNumber{Number: number, Owner: svc.mainKing, RandomX: rxh, CreatedAt: hexutil.Uint64(now), Active: true, BucketID: bucket.ID, BucketHash: bucket.Hash, MainKingIssued: true}
		svc.numbers[number] = record
		svc.addPropagationLocked("number", svc.nextID, rxh, now, record)
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
	svc.refreshNumberOwnershipHashesLocked(&record)
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
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneMessage{}, err
	}
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
	svc.addPropagationLocked("message", svc.nextMsg, msg.RandomXHash, now, msg)
	svc.msgFeed.Send(msg)
	if err := svc.saveLocked(); err != nil {
		return PhoneMessage{}, err
	}
	return msg, nil
}

func (svc *TkmPhoneService) StartCall(from string, to string, offerCiphertext []byte, offerNonce []byte) (PhoneCall, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneCall{}, err
	}
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
	svc.addPropagationLocked("call", svc.nextCall, call.OfferRandomXHash, now, call)
	svc.callFeed.Send(call)
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) AcceptCall(id uint64, answerCiphertext []byte, answerNonce []byte) (PhoneCall, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneCall{}, err
	}
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
	svc.addPropagationLocked("call-accepted", id, call.AnswerRandomXHash, uint64(call.AnsweredAt), call)
	svc.callFeed.Send(call)
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) RejectCall(id uint64, number string, reason string) (PhoneCall, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneCall{}, err
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
	if number != call.To && number != call.From {
		return PhoneCall{}, errors.New("number is not in call")
	}
	now := uint64(time.Now().Unix())
	call.State = PhoneCallRejected
	call.EndReason = reason
	call.EndedAt = hexutil.Uint64(now)
	svc.calls[id] = call
	svc.addNotificationLocked(call.From, "call-rejected", id, now)
	svc.addNotificationLocked(call.To, "call-rejected", id, now)
	svc.addPropagationLocked("call-rejected", id, svc.randomXServiceHash("call-rejected", tkmPhoneUint64Bytes(id), []byte(reason)), now, call)
	svc.callFeed.Send(call)
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) ExpireRingingCalls(timeoutSeconds uint64) ([]PhoneCall, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return nil, err
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 60
	}
	now := uint64(time.Now().Unix())
	cutoff := uint64(0)
	if now > timeoutSeconds {
		cutoff = now - timeoutSeconds
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	expired := make([]PhoneCall, 0)
	for id, call := range svc.calls {
		if call.State != PhoneCallRinging || uint64(call.StartedAt) > cutoff {
			continue
		}
		call.State = PhoneCallMissed
		call.EndReason = "timeout"
		call.EndedAt = hexutil.Uint64(now)
		svc.calls[id] = call
		expired = append(expired, call)
		svc.addNotificationLocked(call.From, "call-missed", id, now)
		svc.addNotificationLocked(call.To, "call-missed", id, now)
		svc.addPropagationLocked("call-missed", id, svc.randomXServiceHash("call-missed", tkmPhoneUint64Bytes(id)), now, call)
		svc.callFeed.Send(call)
	}
	if len(expired) == 0 {
		return expired, nil
	}
	return expired, svc.saveLocked()
}

func (svc *TkmPhoneService) WebRTCConfig() PhoneWebRTCConfig {
	return PhoneWebRTCConfig{
		AudioOnly:              true,
		RequiredEncryption:     true,
		OfferAction:            "start-call",
		AnswerAction:           "accept-call",
		CandidateAction:        "add-call-candidate",
		CandidateListAction:    "list-call-candidates",
		MaxSignalBytes:         hexutil.Uint64(tkmPhoneMaxPayloadSize),
		CandidateRateLimit:     hexutil.Uint64(tkmPhoneCallCandidateRateLimit),
		RecommendedRingSeconds: hexutil.Uint64(60),
		ICEServers: []PhoneICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}
}

func (svc *TkmPhoneService) EndCall(id uint64) (PhoneCall, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneCall{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	call, ok := svc.calls[id]
	if !ok {
		return PhoneCall{}, errors.New("call not found")
	}
	if call.State == PhoneCallEnded || call.State == PhoneCallRejected || call.State == PhoneCallMissed {
		return PhoneCall{}, errors.New("call already ended")
	}
	call.State = PhoneCallEnded
	call.EndedAt = hexutil.Uint64(time.Now().Unix())
	svc.calls[id] = call
	svc.addNotificationLocked(call.From, "call-ended", id, uint64(call.EndedAt))
	svc.addNotificationLocked(call.To, "call-ended", id, uint64(call.EndedAt))
	svc.addPropagationLocked("call-ended", id, svc.randomXServiceHash("call-ended", tkmPhoneUint64Bytes(id)), uint64(call.EndedAt), call)
	svc.callFeed.Send(call)
	if err := svc.saveLocked(); err != nil {
		return PhoneCall{}, err
	}
	return call, nil
}

func (svc *TkmPhoneService) AddCallCandidate(id uint64, number string, ciphertext []byte, nonce []byte) (PhoneCallSignal, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneCallSignal{}, err
	}
	if len(ciphertext) == 0 || len(nonce) == 0 {
		return PhoneCallSignal{}, errors.New("encrypted call candidate and nonce are required")
	}
	if len(ciphertext) > tkmPhoneMaxPayloadSize {
		return PhoneCallSignal{}, errors.New("call candidate exceeds maximum payload size")
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	call, ok := svc.calls[id]
	if !ok {
		return PhoneCallSignal{}, errors.New("call not found")
	}
	if call.State == PhoneCallEnded {
		return PhoneCallSignal{}, errors.New("call already ended")
	}
	peer := ""
	switch number {
	case call.From:
		peer = call.To
	case call.To:
		peer = call.From
	default:
		return PhoneCallSignal{}, errors.New("number is not a call participant")
	}
	if err := svc.checkRateLocked("call-candidate:"+number, now, tkmPhoneCallCandidateRateLimit); err != nil {
		return PhoneCallSignal{}, err
	}
	signal := PhoneCallSignal{CallID: hexutil.Uint64(id), From: number, To: peer, Kind: "ice", Ciphertext: append([]byte(nil), ciphertext...), Nonce: append([]byte(nil), nonce...), RandomXHash: svc.messageKey(number, peer, nonce), CreatedAt: hexutil.Uint64(now)}
	svc.callSignals[id] = append(svc.callSignals[id], signal)
	svc.addNotificationLocked(peer, "call-candidate", id, now)
	svc.addPropagationLocked("call-candidate", id, signal.RandomXHash, now, signal)
	svc.callSignalFeed.Send(signal)
	if err := svc.saveLocked(); err != nil {
		return PhoneCallSignal{}, err
	}
	return signal, nil
}

func (svc *TkmPhoneService) CallCandidates(id uint64, number string) ([]PhoneCallSignal, error) {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	call, ok := svc.calls[id]
	if !ok {
		return nil, errors.New("call not found")
	}
	if number != call.From && number != call.To {
		return nil, errors.New("number is not a call participant")
	}
	signals := svc.callSignals[id]
	out := make([]PhoneCallSignal, len(signals))
	copy(out, signals)
	return out, nil
}

func (svc *TkmPhoneService) SendEncryptedMessageSigned(from string, to string, ciphertext []byte, nonce []byte, signature []byte) (PhoneMessage, error) {
	payload := svc.randomXServiceHash("send-message-payload", []byte(from), []byte(to), nonce, ciphertext)
	if err := svc.verifyNumberDeviceOrOwnerSignature(from, "send-message", payload, signature); err != nil {
		return PhoneMessage{}, err
	}
	if err := svc.requireActiveDeviceKey(from); err != nil {
		return PhoneMessage{}, err
	}
	return svc.SendEncryptedMessage(from, to, ciphertext, nonce)
}

func (svc *TkmPhoneService) StartCallSigned(from string, to string, offerCiphertext []byte, offerNonce []byte, signature []byte) (PhoneCall, error) {
	payload := svc.randomXServiceHash("start-call-payload", []byte(from), []byte(to), offerNonce, offerCiphertext)
	if err := svc.verifyNumberDeviceOrOwnerSignature(from, "start-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	if err := svc.requireActiveDeviceKey(from); err != nil {
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
	if err := svc.verifyNumberDeviceOrOwnerSignature(call.To, "accept-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	if err := svc.requireActiveDeviceKey(call.To); err != nil {
		return PhoneCall{}, err
	}
	return svc.AcceptCall(id, answerCiphertext, answerNonce)
}

func (svc *TkmPhoneService) RejectCallSigned(id uint64, number string, reason string, signature []byte) (PhoneCall, error) {
	svc.lock.RLock()
	call, ok := svc.calls[id]
	svc.lock.RUnlock()
	if !ok {
		return PhoneCall{}, errors.New("call not found")
	}
	if number != call.From && number != call.To {
		return PhoneCall{}, errors.New("number is not in call")
	}
	payload := svc.randomXServiceHash("reject-call-payload", tkmPhoneUint64Bytes(id), []byte(reason))
	if err := svc.verifyNumberDeviceOrOwnerSignature(number, "reject-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	if err := svc.requireActiveDeviceKey(number); err != nil {
		return PhoneCall{}, err
	}
	return svc.RejectCall(id, number, reason)
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
	if err := svc.verifyNumberDeviceOrOwnerSignature(number, "end-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	if err := svc.requireActiveDeviceKey(number); err != nil {
		return PhoneCall{}, err
	}
	return svc.EndCall(id)
}

func (svc *TkmPhoneService) AddCallCandidateSigned(id uint64, number string, ciphertext []byte, nonce []byte, signature []byte) (PhoneCallSignal, error) {
	payload := svc.callCandidateHash(id, number, nonce, ciphertext)
	if err := svc.verifyNumberDeviceOrOwnerSignature(number, "add-call-candidate", payload, signature); err != nil {
		return PhoneCallSignal{}, err
	}
	if err := svc.requireActiveDeviceKey(number); err != nil {
		return PhoneCallSignal{}, err
	}
	return svc.AddCallCandidate(id, number, ciphertext, nonce)
}

func (svc *TkmPhoneService) CallCandidatesSigned(id uint64, number string, signature []byte) ([]PhoneCallSignal, error) {
	payload := svc.callCandidateListHash(id, number)
	if err := svc.verifyNumberDeviceOrOwnerSignature(number, "list-call-candidates", payload, signature); err != nil {
		return nil, err
	}
	if err := svc.requireActiveDeviceKey(number); err != nil {
		return nil, err
	}
	return svc.CallCandidates(id, number)
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

func (svc *TkmPhoneService) DeviceKeys(number string) ([]PhoneDeviceKey, error) {
	if _, err := svc.Number(number); err != nil {
		return nil, err
	}
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	keys := append([]PhoneDeviceKey(nil), svc.devices[number]...)
	return keys, nil
}

func (svc *TkmPhoneService) RegisteredNumber(number string) (RegisteredPhoneNumber, error) {
	num, err := svc.Number(number)
	if err != nil {
		return RegisteredPhoneNumber{}, err
	}
	keys, err := svc.DeviceKeys(number)
	if err != nil {
		return RegisteredPhoneNumber{}, err
	}
	active := make([]PhoneDeviceKey, 0, len(keys))
	for _, key := range keys {
		if key.Active {
			active = append(active, key)
		}
	}
	return RegisteredPhoneNumber{Number: num, Registered: len(active) > 0, DeviceCount: hexutil.Uint64(len(active)), Devices: active}, nil
}

func (svc *TkmPhoneService) RegisteredNumbers() []RegisteredPhoneNumber {
	svc.lock.RLock()
	numbers := make([]string, 0, len(svc.numbers))
	for number := range svc.numbers {
		numbers = append(numbers, number)
	}
	svc.lock.RUnlock()
	out := make([]RegisteredPhoneNumber, 0, len(numbers))
	for _, number := range numbers {
		registered, err := svc.RegisteredNumber(number)
		if err == nil && registered.Registered {
			out = append(out, registered)
		}
	}
	return out
}

func (svc *TkmPhoneService) UseNumber(number string, signature []byte) (PhoneNumber, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneNumber{}, err
	}
	payload := svc.randomXServiceHash("use-number-payload", []byte(number))
	if err := svc.verifyNumberOwnerSignature(number, "use-number", payload, signature); err != nil {
		return PhoneNumber{}, err
	}
	now := uint64(time.Now().Unix())
	svc.lock.Lock()
	defer svc.lock.Unlock()
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneNumber{}, errors.New("number not found")
	}
	if !record.InUse {
		record.InUse = true
		record.InUseAt = hexutil.Uint64(now)
		record.UseHash = svc.stablePhoneHash("number-use", []byte(record.Number), record.Owner.Bytes(), record.Operator.Bytes(), record.BucketHash.Bytes(), tkmPhoneUint64Bytes(now))
	}
	svc.refreshNumberOwnershipHashesLocked(&record)
	svc.numbers[number] = record
	svc.addPropagationLocked("number-used", 0, record.UseHash, now, record)
	if err := svc.saveLocked(); err != nil {
		return PhoneNumber{}, err
	}
	return record, nil
}

func (svc *TkmPhoneService) RegisterDeviceKey(number string, device string, publicKey []byte, signature []byte) (PhoneDeviceKey, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneDeviceKey{}, err
	}
	if device == "" {
		return PhoneDeviceKey{}, errors.New("device is required")
	}
	if len(publicKey) == 0 {
		return PhoneDeviceKey{}, errors.New("public key is required")
	}
	payload := svc.deviceKeyPayloadHash(number, device, publicKey)
	if err := svc.verifyNumberOwnerSignature(number, "register-device", payload, signature); err != nil {
		return PhoneDeviceKey{}, err
	}
	key := PhoneDeviceKey{Number: number, Device: device, PublicKey: append([]byte(nil), publicKey...), CreatedAt: hexutil.Uint64(time.Now().Unix()), Active: true}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneDeviceKey{}, errors.New("number not found")
	}
	if !record.InUse {
		record.InUse = true
		record.InUseAt = key.CreatedAt
		record.UseHash = svc.stablePhoneHash("number-use", []byte(record.Number), record.Owner.Bytes(), record.Operator.Bytes(), record.BucketHash.Bytes(), tkmPhoneUint64Bytes(uint64(key.CreatedAt)))
		svc.refreshNumberOwnershipHashesLocked(&record)
		svc.numbers[number] = record
		svc.addPropagationLocked("number-used", 0, record.UseHash, uint64(key.CreatedAt), record)
	}
	svc.devices[number] = append(svc.devices[number], key)
	svc.addPropagationLocked("device-key", uint64(len(svc.devices[number])), svc.randomXServiceHash("device-key", []byte(number), []byte(device), publicKey), uint64(key.CreatedAt), key)
	if err := svc.saveLocked(); err != nil {
		return PhoneDeviceKey{}, err
	}
	return key, nil
}

func (svc *TkmPhoneService) TransferNumber(number string, newOwner common.Address, signature []byte) (PhoneNumber, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneNumber{}, err
	}
	if newOwner == (common.Address{}) {
		return PhoneNumber{}, errors.New("new owner is required")
	}
	payload := svc.transferNumberPayloadHash(number, newOwner)
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
	svc.refreshNumberOwnershipHashesLocked(&record)
	svc.numbers[number] = record
	svc.addPropagationLocked("number-transferred", 0, record.TransferHash, uint64(time.Now().Unix()), record)
	if err := svc.saveLocked(); err != nil {
		return PhoneNumber{}, err
	}
	return record, nil
}

func (svc *TkmPhoneService) RevokeNumber(number string, signature []byte) (PhoneNumber, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneNumber{}, err
	}
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
	svc.addPropagationLocked("number-revoked", 0, svc.randomXServiceHash("number-revoked", []byte(number)), uint64(time.Now().Unix()), record)
	if err := svc.saveLocked(); err != nil {
		return PhoneNumber{}, err
	}
	return record, nil
}

func (svc *TkmPhoneService) AckMessage(id uint64, status PhoneMessageStatus, signature []byte) (PhoneMessage, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneMessage{}, err
	}
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
	svc.msgFeed.Send(msg)
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
	if err := svc.requirePhoneForkActive(); err != nil {
		return err
	}
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
		if (cutoff > 0 && uint64(msg.CreatedAt) < cutoff) || (msg.ExpiresAt != 0 && uint64(msg.ExpiresAt) <= uint64(time.Now().Unix())) {
			delete(svc.messages, id)
		}
	}
	for id, call := range svc.calls {
		if (cutoff > 0 && uint64(call.StartedAt) < cutoff) || (call.ExpiresAt != 0 && uint64(call.ExpiresAt) <= uint64(time.Now().Unix())) {
			delete(svc.calls, id)
			delete(svc.callSignals, id)
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

func (svc *TkmPhoneService) SendEncryptedMessageWithExpiry(from string, to string, ciphertext []byte, nonce []byte, expiresAt uint64, signature []byte) (PhoneMessage, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneMessage{}, err
	}
	payload := svc.randomXServiceHash("send-message-payload", []byte(from), []byte(to), nonce, ciphertext)
	if err := svc.verifyNumberDeviceOrOwnerSignature(from, "send-message", payload, signature); err != nil {
		return PhoneMessage{}, err
	}
	if err := svc.requireActiveDeviceKey(from); err != nil {
		return PhoneMessage{}, err
	}
	msg, err := svc.SendEncryptedMessage(from, to, ciphertext, nonce)
	if err != nil {
		return PhoneMessage{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	msg.ExpiresAt = hexutil.Uint64(expiresAt)
	svc.messages[uint64(msg.ID)] = msg
	return msg, svc.saveLocked()
}
func (svc *TkmPhoneService) StartCallWithExpiry(from string, to string, offerCiphertext []byte, offerNonce []byte, expiresAt uint64, signature []byte) (PhoneCall, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneCall{}, err
	}
	payload := svc.randomXServiceHash("start-call-payload", []byte(from), []byte(to), offerNonce, offerCiphertext)
	if err := svc.verifyNumberDeviceOrOwnerSignature(from, "start-call", payload, signature); err != nil {
		return PhoneCall{}, err
	}
	if err := svc.requireActiveDeviceKey(from); err != nil {
		return PhoneCall{}, err
	}
	call, err := svc.StartCall(from, to, offerCiphertext, offerNonce)
	if err != nil {
		return PhoneCall{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	call.ExpiresAt = hexutil.Uint64(expiresAt)
	svc.calls[uint64(call.ID)] = call
	return call, svc.saveLocked()
}
func (svc *TkmPhoneService) EncryptPayloadForDevices(from string, to string, nonce []byte, plaintext []byte) ([]PhoneDeviceEnvelope, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return nil, err
	}
	if err := svc.requireNumbers(from, to); err != nil {
		return nil, err
	}
	svc.lock.RLock()
	devices := append([]PhoneDeviceKey(nil), svc.devices[to]...)
	svc.lock.RUnlock()
	out := make([]PhoneDeviceEnvelope, 0, len(devices))
	for _, dev := range devices {
		if !dev.Active {
			continue
		}
		key := svc.randomXServiceHash("device-message-key", []byte(from), []byte(to), []byte(dev.Device), dev.PublicKey, nonce)
		ct, err := tkmPhoneSeal(key, nonce, plaintext, []byte(from+"->"+to+":"+dev.Device))
		if err != nil {
			return nil, err
		}
		out = append(out, PhoneDeviceEnvelope{Device: dev.Device, Ciphertext: ct, Nonce: append([]byte(nil), nonce...)})
	}
	return out, nil
}
func (svc *TkmPhoneService) PendingOperatorApprovals(scanBlocks uint64) []PhonePendingOperatorApproval {
	if scanBlocks == 0 || scanBlocks > tkmPhoneBucketPaymentScanLimit {
		scanBlocks = tkmPhoneBucketPaymentScanLimit
	}
	if svc == nil || svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	head := svc.eth.blockchain.CurrentBlock()
	if head == nil {
		return nil
	}
	end := head.Number.Uint64()
	start := uint64(0)
	if end > scanBlocks {
		start = end - scanBlocks
	}
	out := make([]PhonePendingOperatorApproval, 0)
	seen := make(map[common.Hash]bool)
	for n := end; ; n-- {
		block := svc.eth.blockchain.GetBlockByNumber(n)
		if block != nil {
			for idx, tx := range block.Transactions() {
				pending, ok := svc.pendingApprovalFromTx(tx, n, uint64(idx))
				if !ok || seen[pending.PaymentTx] {
					continue
				}
				seen[pending.PaymentTx] = true
				out = append(out, pending)
			}
		}
		if n == start || n == 0 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return uint64(out[i].Block) > uint64(out[j].Block) })
	return out
}

func (svc *TkmPhoneService) ApproveOperatorPayment(paymentTx common.Hash, signature []byte) (PhoneOperatorKey, error) {
	if paymentTx == (common.Hash{}) {
		return PhoneOperatorKey{}, errors.New("operator payment transaction is required")
	}
	if svc == nil || svc.eth == nil || svc.eth.blockchain == nil {
		return PhoneOperatorKey{}, errors.New("canonical chain is unavailable")
	}
	_, tx := svc.eth.blockchain.GetCanonicalTransaction(paymentTx)
	if tx == nil {
		return PhoneOperatorKey{}, errors.New("operator payment transaction is not canonical or indexed")
	}
	pending, ok := svc.pendingApprovalFromTx(tx, 0, 0)
	if !ok {
		return PhoneOperatorKey{}, errors.New("payment transaction is not a tkmphone bucket approval payment")
	}
	return svc.RegisterOperatorKey(pending.Operator, pending.KeyHash, uint64(pending.ExpiresAt), paymentTx, (*big.Int)(pending.Paid), signature)
}

func (svc *TkmPhoneService) pendingApprovalFromTx(tx *types.Transaction, blockNumber uint64, txIndex uint64) (PhonePendingOperatorApproval, bool) {
	if tx == nil || tx.To() == nil || *tx.To() != svc.mainKing || tx.Value().Cmp(tkmPhoneOperatorKeyPrice) != 0 {
		return PhonePendingOperatorApproval{}, false
	}
	keyHash, expiresAt, ok := tkmPhoneDecodeBucketPaymentData(tx.Data())
	if !ok {
		return PhonePendingOperatorApproval{}, false
	}
	signer, err := types.Sender(types.LatestSigner(svc.eth.blockchain.Config()), tx)
	if err != nil {
		return PhonePendingOperatorApproval{}, false
	}
	paymentTx := tx.Hash()
	pending := PhonePendingOperatorApproval{
		Operator:  signer,
		KeyHash:   keyHash,
		ExpiresAt: hexutil.Uint64(expiresAt),
		PaymentTx: paymentTx,
		Paid:      (*hexutil.Big)(new(big.Int).Set(tkmPhoneOperatorKeyPrice)),
		GrantHash: svc.operatorGrantHash(signer, keyHash, expiresAt, paymentTx),
		Block:     hexutil.Uint64(blockNumber),
		TxIndex:   hexutil.Uint64(txIndex),
	}
	now := uint64(time.Now().Unix())
	if expiresAt <= now {
		pending.Reason = "expired"
	}
	svc.lock.RLock()
	for _, op := range svc.operators {
		if op.PaymentTx == paymentTx || (op.Operator == signer && op.KeyHash == keyHash && op.Active) {
			pending.Approved = true
			break
		}
	}
	svc.lock.RUnlock()
	return pending, true
}

func tkmPhoneDecodeBucketPaymentData(data []byte) (common.Hash, uint64, bool) {
	want := len(tkmPhoneBucketPaymentDataPrefix) + common.HashLength + 8
	if len(data) != want || !bytes.Equal(data[:len(tkmPhoneBucketPaymentDataPrefix)], tkmPhoneBucketPaymentDataPrefix) {
		return common.Hash{}, 0, false
	}
	keyHash := common.BytesToHash(data[len(tkmPhoneBucketPaymentDataPrefix) : len(tkmPhoneBucketPaymentDataPrefix)+common.HashLength])
	expiresAt := binary.BigEndian.Uint64(data[len(tkmPhoneBucketPaymentDataPrefix)+common.HashLength:])
	if keyHash == (common.Hash{}) || expiresAt == 0 {
		return common.Hash{}, 0, false
	}
	return keyHash, expiresAt, true
}

func (svc *TkmPhoneService) ListOperators() []PhoneOperatorKey {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	out := make([]PhoneOperatorKey, 0, len(svc.operators))
	for _, op := range svc.operators {
		out = append(out, op)
	}
	return out
}
func (svc *TkmPhoneService) ReportOperator(operator common.Address, reporter string, reason string, evidence common.Hash, signature []byte) (PhoneFraudReport, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneFraudReport{}, err
	}
	payload := svc.randomXServiceHash("report-operator-payload", operator.Bytes(), []byte(reporter), []byte(reason), evidence.Bytes())
	if err := svc.verifyNumberOwnerSignature(reporter, "report-operator", payload, signature); err != nil {
		return PhoneFraudReport{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.nextReport++
	r := PhoneFraudReport{ID: hexutil.Uint64(svc.nextReport), Operator: operator, Reporter: reporter, Reason: reason, Evidence: evidence, CreatedAt: hexutil.Uint64(time.Now().Unix())}
	svc.reports[svc.nextReport] = r
	svc.addPropagationLocked("operator-report", svc.nextReport, svc.randomXServiceHash("operator-report", operator.Bytes(), []byte(reporter), []byte(reason), evidence.Bytes()), uint64(r.CreatedAt), r)
	return r, svc.saveLocked()
}
func (svc *TkmPhoneService) AddContact(ownerNumber string, peerNumber string, ciphertext []byte, nonce []byte, signature []byte) (PhoneContact, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneContact{}, err
	}
	payload := svc.randomXServiceHash("add-contact-payload", []byte(ownerNumber), []byte(peerNumber), nonce, ciphertext)
	if err := svc.verifyNumberOwnerSignature(ownerNumber, "add-contact", payload, signature); err != nil {
		return PhoneContact{}, err
	}
	c := PhoneContact{OwnerNumber: ownerNumber, PeerNumber: peerNumber, Ciphertext: append([]byte(nil), ciphertext...), Nonce: append([]byte(nil), nonce...), CreatedAt: hexutil.Uint64(time.Now().Unix())}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.contacts[ownerNumber] = append(svc.contacts[ownerNumber], c)
	svc.addPropagationLocked("contact", uint64(len(svc.contacts[ownerNumber])), svc.randomXServiceHash("contact", []byte(ownerNumber), []byte(peerNumber), nonce, ciphertext), uint64(c.CreatedAt), c)
	return c, svc.saveLocked()
}
func (svc *TkmPhoneService) Contacts(number string) ([]PhoneContact, error) {
	if _, err := svc.Number(number); err != nil {
		return nil, err
	}
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	return append([]PhoneContact(nil), svc.contacts[number]...), nil
}
func (svc *TkmPhoneService) BlockNumber(ownerNumber string, blockedNumber string, signature []byte) error {
	if err := svc.requirePhoneForkActive(); err != nil {
		return err
	}
	payload := svc.randomXServiceHash("block-number-payload", []byte(ownerNumber), []byte(blockedNumber))
	if err := svc.verifyNumberOwnerSignature(ownerNumber, "block-number", payload, signature); err != nil {
		return err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if svc.blocked[ownerNumber] == nil {
		svc.blocked[ownerNumber] = make(map[string]bool)
	}
	svc.blocked[ownerNumber][blockedNumber] = true
	now := uint64(time.Now().Unix())
	record := phoneBlockRecord{OwnerNumber: ownerNumber, BlockedNumber: blockedNumber, CreatedAt: hexutil.Uint64(now)}
	svc.addPropagationLocked("blocked", 0, svc.randomXServiceHash("blocked", []byte(ownerNumber), []byte(blockedNumber)), now, record)
	return svc.saveLocked()
}
func (svc *TkmPhoneService) UnblockNumber(ownerNumber string, blockedNumber string, signature []byte) error {
	if err := svc.requirePhoneForkActive(); err != nil {
		return err
	}
	payload := svc.randomXServiceHash("unblock-number-payload", []byte(ownerNumber), []byte(blockedNumber))
	if err := svc.verifyNumberOwnerSignature(ownerNumber, "unblock-number", payload, signature); err != nil {
		return err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if svc.blocked[ownerNumber] != nil {
		delete(svc.blocked[ownerNumber], blockedNumber)
	}
	return svc.saveLocked()
}
func (svc *TkmPhoneService) RegisterRecovery(number string, recovery common.Address, signature []byte) error {
	if err := svc.requirePhoneForkActive(); err != nil {
		return err
	}
	payload := svc.randomXServiceHash("register-recovery-payload", []byte(number), recovery.Bytes())
	if err := svc.verifyNumberOwnerSignature(number, "register-recovery", payload, signature); err != nil {
		return err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	svc.recovery[number] = recovery
	now := uint64(time.Now().Unix())
	record := phoneRecoveryRecord{Number: number, Recovery: recovery, CreatedAt: hexutil.Uint64(now)}
	svc.addPropagationLocked("recovery", 0, svc.randomXServiceHash("recovery", []byte(number), recovery.Bytes()), now, record)
	return svc.saveLocked()
}
func (svc *TkmPhoneService) RecoverNumber(number string, newOwner common.Address, signature []byte) (PhoneNumber, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneNumber{}, err
	}
	svc.lock.RLock()
	recovery := svc.recovery[number]
	svc.lock.RUnlock()
	payload := svc.randomXServiceHash("recover-number-payload", []byte(number), newOwner.Bytes())
	if err := verifyPhoneAddressSignature(recovery, payload, signature); err != nil {
		return PhoneNumber{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	rec, ok := svc.numbers[number]
	if !ok {
		return PhoneNumber{}, errors.New("number not found")
	}
	rec.Owner = newOwner
	rec.Active = true
	svc.refreshNumberOwnershipHashesLocked(&rec)
	svc.numbers[number] = rec
	now := uint64(time.Now().Unix())
	record := phoneRecoveryRecord{Number: number, Recovery: recovery, NewOwner: newOwner, CreatedAt: hexutil.Uint64(now)}
	svc.addPropagationLocked("number-recovered", 0, svc.randomXServiceHash("number-recovered", []byte(number), newOwner.Bytes()), now, record)
	return rec, svc.saveLocked()
}
func (svc *TkmPhoneService) PropagationQueue() []PhonePropagation {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	out := make([]PhonePropagation, 0, len(svc.prop))
	for _, p := range svc.prop {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (svc *TkmPhoneService) hasPropagation(id hexutil.Uint64, hash common.Hash) bool {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	existing, ok := svc.prop[uint64(id)]
	return ok && existing.Hash == hash
}

func (svc *TkmPhoneService) ImportPropagation(prop PhonePropagation) error {
	if err := svc.requirePhoneForkActive(); err != nil {
		return err
	}
	if uint64(prop.ID) == 0 {
		return errors.New("propagation id is required")
	}
	if prop.Kind == "" {
		return errors.New("propagation kind is required")
	}
	if len(prop.Payload) > tkmPhoneMaxPayloadSize {
		return errors.New("propagation payload exceeds maximum size")
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if existing, ok := svc.prop[uint64(prop.ID)]; ok && existing.Hash != prop.Hash {
		return errors.New("conflicting propagation record")
	}
	if err := svc.importPropagationLocked(prop); err != nil {
		return err
	}
	svc.prop[uint64(prop.ID)] = prop
	if uint64(prop.ID) > svc.nextProp {
		svc.nextProp = uint64(prop.ID)
	}
	return svc.saveLocked()
}
func (svc *TkmPhoneService) subscribe(ctx context.Context, kind string) (*rpc.Subscription, error) {
	notifier, ok := rpc.NotifierFromContext(ctx)
	if !ok {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()
	ch := make(chan interface{}, 16)
	var evsub event.Subscription
	switch kind {
	case "message":
		evsub = svc.msgFeed.Subscribe(ch)
	case "call":
		evsub = svc.callFeed.Subscribe(ch)
	case "call-signal":
		evsub = svc.callSignalFeed.Subscribe(ch)
	default:
		evsub = svc.notifFeed.Subscribe(ch)
	}
	go func() {
		defer evsub.Unsubscribe()
		for {
			select {
			case ev := <-ch:
				notifier.Notify(sub.ID, ev)
			case <-sub.Err():
				return
			}
		}
	}()
	return sub, nil
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
	svc.nextBucket = snap.NextBucket
	if snap.Operators != nil {
		svc.operators = snap.Operators
	}
	if snap.Buckets != nil {
		svc.buckets = snap.Buckets
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
	if snap.CallSignals != nil {
		svc.callSignals = snap.CallSignals
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
	if snap.Contacts != nil {
		svc.contacts = snap.Contacts
	}
	if snap.Blocked != nil {
		svc.blocked = snap.Blocked
	}
	if snap.Recovery != nil {
		svc.recovery = snap.Recovery
	}
	if snap.Reports != nil {
		svc.reports = snap.Reports
	}
	if snap.Prop != nil {
		svc.prop = snap.Prop
	}
	svc.nextReport = snap.NextReport
	svc.nextProp = snap.NextProp
	svc.refreshOwnershipHashesLocked()
	return nil
}

func (svc *TkmPhoneService) saveLocked() error {
	if svc.db == nil {
		return nil
	}
	snap := tkmPhoneSnapshot{
		NextID:      svc.nextID,
		NextMsg:     svc.nextMsg,
		NextCall:    svc.nextCall,
		NextNotif:   svc.nextNotif,
		NextBucket:  svc.nextBucket,
		Operators:   svc.operators,
		Buckets:     svc.buckets,
		Numbers:     svc.numbers,
		Messages:    svc.messages,
		Calls:       svc.calls,
		CallSignals: svc.callSignals,
		Devices:     svc.devices,
		Notifs:      svc.notifs,
		Rate:        svc.rate,
		Contacts:    svc.contacts,
		Blocked:     svc.blocked,
		Recovery:    svc.recovery,
		Reports:     svc.reports,
		Prop:        svc.prop,
		NextReport:  svc.nextReport,
		NextProp:    svc.nextProp,
	}
	data, err := json.Marshal(&snap)
	if err != nil {
		return err
	}
	if err := svc.db.Put(tkmPhoneStateKey, data); err != nil {
		return err
	}
	if err := svc.db.SyncKeyValue(); err != nil {
		return err
	}
	return svc.writeFileSnapshotLocked(&snap)
}

func (svc *TkmPhoneService) writeFileSnapshotLocked(snap *tkmPhoneSnapshot) error {
	if svc.phoneDir == "" {
		return nil
	}
	if err := os.MkdirAll(svc.phoneDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(svc.phoneDir, "state.json.tmp")
	path := filepath.Join(svc.phoneDir, "state.json")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (svc *TkmPhoneService) requireActiveDeviceKey(number string) error {
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	for _, key := range svc.devices[number] {
		if key.Active {
			return nil
		}
	}
	return errors.New("registered device key required for phone number")
}

func (svc *TkmPhoneService) NumberOwnershipProof(number string) (PhoneNumberOwnershipProof, error) {
	if err := svc.requirePhoneForkActive(); err != nil {
		return PhoneNumberOwnershipProof{}, err
	}
	svc.lock.RLock()
	defer svc.lock.RUnlock()
	record, ok := svc.numbers[number]
	if !ok || !record.Active {
		return PhoneNumberOwnershipProof{}, errors.New("number not found")
	}
	bucket, ok := svc.buckets[uint64(record.BucketID)]
	if !ok || bucket.Hash != record.BucketHash {
		return PhoneNumberOwnershipProof{}, errors.New("number bucket proof not found")
	}
	svc.refreshBucketOwnershipHashesLocked(&bucket)
	svc.refreshNumberOwnershipHashesLocked(&record)
	steps := []PhoneOwnershipStep{
		{
			Kind:     "mainking-issue-number",
			From:     svc.mainKing,
			To:       svc.mainKing,
			BucketID: record.BucketID,
			Number:   record.Number,
			Hash:     record.IssuanceHash,
			At:       record.CreatedAt,
		},
	}
	if bucket.Operator != (common.Address{}) && uint64(bucket.AssignedAt) != 0 {
		steps = append(steps, PhoneOwnershipStep{
			Kind:      "mainking-transfer-bucket-to-operator",
			From:      svc.mainKing,
			To:        bucket.Operator,
			PaymentTx: bucket.PaymentTx,
			BucketID:  bucket.ID,
			Hash:      bucket.AssignHash,
			At:        bucket.AssignedAt,
		})
	}
	if record.SalePaymentTx != (common.Hash{}) && uint64(record.SoldAt) != 0 {
		steps = append(steps, PhoneOwnershipStep{
			Kind:      "operator-transfer-number-to-user",
			From:      record.Operator,
			To:        record.Owner,
			PaymentTx: record.SalePaymentTx,
			BucketID:  record.BucketID,
			Number:    record.Number,
			Hash:      record.TransferHash,
			At:        record.SoldAt,
		})
	}
	if record.InUse {
		steps = append(steps, PhoneOwnershipStep{
			Kind:     "number-selected-for-use",
			From:     record.Owner,
			To:       record.Owner,
			BucketID: record.BucketID,
			Number:   record.Number,
			Hash:     record.UseHash,
			At:       record.InUseAt,
		})
	}
	proofHash := svc.stablePhoneHash("number-ownership-proof", []byte(record.Number), bucket.Hash.Bytes(), svc.mainKing.Bytes(), record.Operator.Bytes(), record.Owner.Bytes(), record.IssuanceHash.Bytes(), bucket.OwnerHash.Bytes(), bucket.AssignHash.Bytes(), record.OwnerHash.Bytes(), record.TransferHash.Bytes(), record.UseHash.Bytes())
	return PhoneNumberOwnershipProof{
		Number:           record.Number,
		BucketID:         record.BucketID,
		BucketHash:       record.BucketHash,
		CreationTx:       bucket.CreationTx,
		SalePaymentTx:    record.SalePaymentTx,
		MainKing:         svc.mainKing,
		Operator:         record.Operator,
		CurrentOwner:     record.Owner,
		IssuanceHash:     record.IssuanceHash,
		BucketOwnerHash:  bucket.OwnerHash,
		BucketAssignHash: bucket.AssignHash,
		NumberOwnerHash:  record.OwnerHash,
		TransferHash:     record.TransferHash,
		InUse:            record.InUse,
		InUseAt:          record.InUseAt,
		UseHash:          record.UseHash,
		ProofHash:        proofHash,
		Steps:            steps,
	}, nil
}

func (svc *TkmPhoneService) refreshOwnershipHashesLocked() {
	for id, bucket := range svc.buckets {
		svc.refreshBucketOwnershipHashesLocked(&bucket)
		svc.buckets[id] = bucket
	}
	for number, record := range svc.numbers {
		svc.refreshNumberOwnershipHashesLocked(&record)
		svc.numbers[number] = record
	}
}

func (svc *TkmPhoneService) refreshBucketOwnershipHashesLocked(bucket *PhoneNumberBucket) {
	if bucket == nil {
		return
	}
	bucket.IssueHash = svc.stablePhoneHash("bucket-issued", svc.mainKing.Bytes(), bucket.Hash.Bytes(), bucket.Seed.Bytes(), bucket.CreationTx.Bytes(), tkmPhoneUint64Bytes(uint64(bucket.ID)), tkmPhoneUint64Bytes(uint64(bucket.Round)), tkmPhoneUint64Bytes(uint64(bucket.Index)), tkmPhoneUint64Bytes(uint64(bucket.CreatedAt)))
	owner := svc.mainKing
	if bucket.Operator != (common.Address{}) {
		owner = bucket.Operator
	}
	bucket.OwnerHash = svc.stablePhoneHash("bucket-owner", bucket.Hash.Bytes(), svc.mainKing.Bytes(), owner.Bytes(), bucket.PaymentTx.Bytes(), tkmPhoneUint64Bytes(uint64(bucket.AssignedAt)))
	if bucket.Operator != (common.Address{}) && uint64(bucket.AssignedAt) != 0 {
		bucket.AssignHash = svc.stablePhoneHash("bucket-transfer", bucket.Hash.Bytes(), svc.mainKing.Bytes(), bucket.Operator.Bytes(), bucket.PaymentTx.Bytes(), tkmPhoneUint64Bytes(uint64(bucket.ID)), tkmPhoneUint64Bytes(uint64(bucket.AssignedAt)))
	}
}

func (svc *TkmPhoneService) refreshNumberOwnershipHashesLocked(record *PhoneNumber) {
	if record == nil {
		return
	}
	record.IssuanceHash = svc.stablePhoneHash("number-issued", []byte(record.Number), svc.mainKing.Bytes(), record.BucketHash.Bytes(), record.RandomX.Bytes(), tkmPhoneUint64Bytes(uint64(record.BucketID)), tkmPhoneUint64Bytes(uint64(record.CreatedAt)))
	if record.InUse && record.UseHash == (common.Hash{}) {
		record.UseHash = svc.stablePhoneHash("number-use", []byte(record.Number), record.Owner.Bytes(), record.Operator.Bytes(), record.BucketHash.Bytes(), tkmPhoneUint64Bytes(uint64(record.InUseAt)))
	}
	if record.Operator != (common.Address{}) && record.SalePaymentTx != (common.Hash{}) && uint64(record.SoldAt) != 0 {
		price := []byte(nil)
		if record.SalePrice != nil {
			price = record.SalePrice.ToInt().Bytes()
		}
		record.TransferHash = svc.stablePhoneHash("number-sale-transfer", []byte(record.Number), record.BucketHash.Bytes(), record.Operator.Bytes(), record.Owner.Bytes(), price, record.SalePaymentTx.Bytes(), tkmPhoneUint64Bytes(uint64(record.SoldAt)))
	} else if record.Owner != svc.mainKing && record.Operator != (common.Address{}) {
		record.TransferHash = svc.stablePhoneHash("number-bucket-owner", []byte(record.Number), record.BucketHash.Bytes(), svc.mainKing.Bytes(), record.Operator.Bytes(), tkmPhoneUint64Bytes(uint64(record.BucketID)))
	} else if record.TransferHash == (common.Hash{}) {
		record.TransferHash = record.IssuanceHash
	}
	record.OwnerHash = svc.stablePhoneHash("number-owner", []byte(record.Number), record.BucketHash.Bytes(), record.Owner.Bytes(), record.TransferHash.Bytes(), record.UseHash.Bytes())
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
	if blockedByRecipient := svc.blocked[to]; blockedByRecipient != nil && blockedByRecipient[from] {
		return errors.New("sender is blocked by recipient")
	}
	return nil
}

func (svc *TkmPhoneService) validateBucketCreationTx(creationTx common.Hash) error {
	if svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	_, tx := svc.eth.blockchain.GetCanonicalTransaction(creationTx)
	if tx == nil {
		return errors.New("bucket creation transaction is not canonical or indexed")
	}
	config := svc.eth.blockchain.Config()
	signer, err := types.Sender(types.LatestSigner(config), tx)
	if err != nil {
		return fmt.Errorf("bucket creation transaction sender unavailable: %w", err)
	}
	if signer != svc.mainKing {
		return fmt.Errorf("bucket creation transaction sent by %s, want main king %s", signer.Hex(), svc.mainKing.Hex())
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
		return errors.New("operator bucket payment must be sent to main king")
	}
	if tx.Value().Cmp(tkmPhoneOperatorKeyPrice) != 0 {
		return errors.New("operator bucket payment transaction must be exactly 25000 TKM")
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

func (svc *TkmPhoneService) validateNumberSalePayment(operator common.Address, buyer common.Address, price *big.Int, paymentTx common.Hash) error {
	if svc.eth == nil || svc.eth.blockchain == nil {
		return nil
	}
	_, tx := svc.eth.blockchain.GetCanonicalTransaction(paymentTx)
	if tx == nil {
		return errors.New("phone number sale payment transaction is not canonical or indexed")
	}
	to := tx.To()
	if to == nil || *to != operator {
		return errors.New("phone number sale payment must be sent to operator")
	}
	if tx.Value().Cmp(price) != 0 {
		return errors.New("phone number sale payment transaction must be exactly 10000 TKM")
	}
	config := svc.eth.blockchain.Config()
	signer, err := types.Sender(types.LatestSigner(config), tx)
	if err != nil {
		return fmt.Errorf("phone number sale payment sender unavailable: %w", err)
	}
	if signer != buyer {
		return fmt.Errorf("phone number sale payment sent by %s, want %s", signer.Hex(), buyer.Hex())
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

func (svc *TkmPhoneService) verifyNumberDeviceOrOwnerSignature(number string, action string, payload common.Hash, signature []byte) error {
	svc.lock.RLock()
	record, ok := svc.numbers[number]
	devices := append([]PhoneDeviceKey(nil), svc.devices[number]...)
	svc.lock.RUnlock()
	if !ok || !record.Active {
		return errors.New("number not found")
	}
	digest := svc.ownerActionHash(number, action, payload)
	if err := verifyPhoneAddressSignature(record.Owner, digest, signature); err == nil {
		return nil
	}
	signer, err := recoverPhoneActionSigner(digest, signature)
	if err != nil {
		return fmt.Errorf("invalid phone action signature: %w", err)
	}
	for _, device := range devices {
		if !device.Active {
			continue
		}
		if len(device.PublicKey) == common.AddressLength && bytes.Equal(device.PublicKey, signer.Bytes()) {
			return nil
		}
		if len(device.PublicKey) == 33 || len(device.PublicKey) == 65 {
			if pub, err := crypto.UnmarshalPubkey(device.PublicKey); err == nil && crypto.PubkeyToAddress(*pub) == signer {
				return nil
			}
		}
	}
	return fmt.Errorf("phone action signed by %s, want owner %s or an active registered device key", signer.Hex(), record.Owner.Hex())
}

func (svc *TkmPhoneService) ownerActionHash(number string, action string, payload common.Hash) common.Hash {
	return svc.randomXServiceHash("owner-action", []byte(number), []byte(action), payload.Bytes())
}

func (svc *TkmPhoneService) deviceKeyPayloadHash(number string, device string, publicKey []byte) common.Hash {
	return svc.randomXServiceHash("device-key-payload", []byte(number), []byte(device), publicKey)
}

func (svc *TkmPhoneService) deviceKeySigningHash(number string, device string, publicKey []byte) common.Hash {
	return svc.ownerActionHash(number, "register-device", svc.deviceKeyPayloadHash(number, device, publicKey))
}

func (svc *TkmPhoneService) useNumberSigningHash(number string) common.Hash {
	return svc.ownerActionHash(number, "use-number", svc.randomXServiceHash("use-number-payload", []byte(number)))
}

func (svc *TkmPhoneService) transferNumberPayloadHash(number string, newOwner common.Address) common.Hash {
	return svc.randomXServiceHash("transfer-number-payload", []byte(number), newOwner.Bytes())
}

func (svc *TkmPhoneService) transferNumberSigningHash(number string, newOwner common.Address) common.Hash {
	return svc.ownerActionHash(number, "transfer-number", svc.transferNumberPayloadHash(number, newOwner))
}

func recoverPhoneActionSigner(digest common.Hash, signature []byte) (common.Address, error) {
	if len(signature) != crypto.SignatureLength {
		return common.Address{}, fmt.Errorf("signature must be %d bytes", crypto.SignatureLength)
	}
	sig := append([]byte(nil), signature...)
	if sig[crypto.RecoveryIDOffset] >= 27 {
		sig[crypto.RecoveryIDOffset] -= 27
	}
	if sig[crypto.RecoveryIDOffset] > 1 {
		return common.Address{}, fmt.Errorf("invalid signature recovery id %d", sig[crypto.RecoveryIDOffset])
	}
	if signer, err := tkmPhoneRecoverSigner(digest, sig); err == nil {
		return signer, nil
	}
	prefixed := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), digest.Bytes())
	return tkmPhoneRecoverSigner(prefixed, sig)
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
	notif := PhoneNotification{ID: hexutil.Uint64(svc.nextNotif), Number: number, Kind: kind, RefID: hexutil.Uint64(refID), CreatedAt: hexutil.Uint64(now)}
	svc.notifs[svc.nextNotif] = notif
	svc.notifFeed.Send(notif)
}

func (svc *TkmPhoneService) addPropagationLocked(kind string, refID uint64, hash common.Hash, now uint64, payload any) {
	data, err := json.Marshal(payload)
	if err != nil || len(data) > tkmPhoneMaxPayloadSize {
		return
	}
	svc.nextProp++
	prop := PhonePropagation{ID: hexutil.Uint64(svc.nextProp), Kind: kind, RefID: hexutil.Uint64(refID), Hash: hash, CreatedAt: hexutil.Uint64(now), Payload: data}
	svc.prop[svc.nextProp] = prop
	if svc.eth != nil {
		go svc.eth.broadcastTkmPhonePropagation(prop)
	}
}

func (svc *TkmPhoneService) importPropagationLocked(prop PhonePropagation) error {
	switch prop.Kind {
	case "bucket", "bucket-assigned":
		var bucket PhoneNumberBucket
		if err := json.Unmarshal(prop.Payload, &bucket); err != nil {
			return err
		}
		id := uint64(bucket.ID)
		if id == 0 || bucket.Hash != prop.Hash || bucket.MainKing != svc.mainKing {
			return errors.New("invalid propagated bucket")
		}
		svc.buckets[id] = bucket
		if prop.Kind == "bucket-assigned" {
			for number, record := range svc.numbers {
				if record.BucketID == bucket.ID && record.Owner == svc.mainKing && record.MainKingIssued {
					record.Owner = bucket.Operator
					record.Operator = bucket.Operator
					svc.numbers[number] = record
				}
			}
		}
		if id > svc.nextBucket {
			svc.nextBucket = id
		}
	case "operator-key":
		var key PhoneOperatorKey
		if err := json.Unmarshal(prop.Payload, &key); err != nil {
			return err
		}
		if key.Operator == (common.Address{}) || key.KeyHash != prop.Hash {
			return errors.New("invalid propagated operator key")
		}
		svc.operators[key.Operator] = key
	case "number", "number-sold", "number-transferred", "number-revoked", "number-used":
		var number PhoneNumber
		if err := json.Unmarshal(prop.Payload, &number); err != nil {
			return err
		}
		if number.Number == "" {
			return errors.New("invalid propagated number")
		}
		svc.numbers[number.Number] = number
		if uint64(prop.RefID) > svc.nextID {
			svc.nextID = uint64(prop.RefID)
		}
	case "device-key":
		var key PhoneDeviceKey
		if err := json.Unmarshal(prop.Payload, &key); err != nil {
			return err
		}
		if key.Number == "" || key.Device == "" {
			return errors.New("invalid propagated device key")
		}
		keys := svc.devices[key.Number]
		for _, existing := range keys {
			if existing.Device == key.Device && bytes.Equal(existing.PublicKey, key.PublicKey) {
				return nil
			}
		}
		svc.devices[key.Number] = append(keys, key)
	case "message":
		var msg PhoneMessage
		if err := json.Unmarshal(prop.Payload, &msg); err != nil {
			return err
		}
		id := uint64(msg.ID)
		if id == 0 || msg.RandomXHash != prop.Hash {
			return errors.New("invalid propagated message")
		}
		svc.messages[id] = msg
		if id > svc.nextMsg {
			svc.nextMsg = id
		}
		svc.msgFeed.Send(msg)
	case "call", "call-accepted", "call-ended", "call-rejected", "call-missed":
		var call PhoneCall
		if err := json.Unmarshal(prop.Payload, &call); err != nil {
			return err
		}
		id := uint64(call.ID)
		if id == 0 {
			return errors.New("invalid propagated call")
		}
		svc.calls[id] = call
		if id > svc.nextCall {
			svc.nextCall = id
		}
		svc.callFeed.Send(call)
	case "call-candidate":
		var signal PhoneCallSignal
		if err := json.Unmarshal(prop.Payload, &signal); err != nil {
			return err
		}
		id := uint64(signal.CallID)
		if id == 0 || signal.RandomXHash != prop.Hash || signal.From == "" || signal.To == "" {
			return errors.New("invalid propagated call candidate")
		}
		existing := svc.callSignals[id]
		for _, have := range existing {
			if have.RandomXHash == signal.RandomXHash && bytes.Equal(have.Nonce, signal.Nonce) {
				return nil
			}
		}
		svc.callSignals[id] = append(existing, signal)
		svc.callSignalFeed.Send(signal)
	case "contact":
		var contact PhoneContact
		if err := json.Unmarshal(prop.Payload, &contact); err != nil {
			return err
		}
		contacts := svc.contacts[contact.OwnerNumber]
		for _, existing := range contacts {
			if existing.PeerNumber == contact.PeerNumber && bytes.Equal(existing.Ciphertext, contact.Ciphertext) {
				return nil
			}
		}
		svc.contacts[contact.OwnerNumber] = append(contacts, contact)
	case "blocked", "unblocked":
		var record phoneBlockRecord
		if err := json.Unmarshal(prop.Payload, &record); err != nil {
			return err
		}
		if svc.blocked[record.OwnerNumber] == nil {
			svc.blocked[record.OwnerNumber] = make(map[string]bool)
		}
		if prop.Kind == "blocked" {
			svc.blocked[record.OwnerNumber][record.BlockedNumber] = true
		} else {
			delete(svc.blocked[record.OwnerNumber], record.BlockedNumber)
		}
	case "recovery", "number-recovered":
		var record phoneRecoveryRecord
		if err := json.Unmarshal(prop.Payload, &record); err != nil {
			return err
		}
		if record.Recovery != (common.Address{}) {
			svc.recovery[record.Number] = record.Recovery
		}
		if record.NewOwner != (common.Address{}) {
			number := svc.numbers[record.Number]
			number.Owner = record.NewOwner
			number.Active = true
			svc.numbers[record.Number] = number
		}
	case "operator-report":
		var report PhoneFraudReport
		if err := json.Unmarshal(prop.Payload, &report); err != nil {
			return err
		}
		id := uint64(report.ID)
		if id == 0 {
			return errors.New("invalid propagated report")
		}
		svc.reports[id] = report
		if id > svc.nextReport {
			svc.nextReport = id
		}
	}
	return nil
}

func phonePropagationPacket(prop PhonePropagation) ethproto.TkmPhonePropagationPacket {
	return ethproto.TkmPhonePropagationPacket{ID: uint64(prop.ID), Type: prop.Kind, RefID: uint64(prop.RefID), Hash: prop.Hash, CreatedAt: uint64(prop.CreatedAt), Payload: append([]byte(nil), prop.Payload...)}
}

func phonePropagationFromPacket(packet ethproto.TkmPhonePropagationPacket) PhonePropagation {
	return PhonePropagation{ID: hexutil.Uint64(packet.ID), Kind: packet.Type, RefID: hexutil.Uint64(packet.RefID), Hash: packet.Hash, CreatedAt: hexutil.Uint64(packet.CreatedAt), Payload: append([]byte(nil), packet.Payload...)}
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

func (svc *TkmPhoneService) bucketGenerationHash(round uint64, seed common.Hash, creationTx common.Hash) common.Hash {
	return svc.randomXServiceHash("bucket-generation", svc.mainKing.Bytes(), tkmPhoneUint64Bytes(round), seed.Bytes(), creationTx.Bytes(), tkmPhoneUint64Bytes(tkmPhoneBucketBatchSize), tkmPhoneUint64Bytes(tkmPhoneBucketSize))
}

func (svc *TkmPhoneService) operatorGrantHash(operator common.Address, keyHash common.Hash, expiresAt uint64, paymentTx common.Hash) common.Hash {
	return svc.randomXServiceHash("operator-grant", operator.Bytes(), keyHash.Bytes(), tkmPhoneUint64Bytes(expiresAt), paymentTx.Bytes(), tkmPhoneOperatorKeyPrice.Bytes())
}

func (svc *TkmPhoneService) openBucketHash(operator common.Address, bucketID uint64) common.Hash {
	return svc.randomXServiceHash("open-bucket-payload", operator.Bytes(), tkmPhoneUint64Bytes(bucketID))
}

func (svc *TkmPhoneService) sendMessageSigningHash(from string, to string, nonce []byte, ciphertext []byte) common.Hash {
	payload := svc.randomXServiceHash("send-message-payload", []byte(from), []byte(to), nonce, ciphertext)
	return svc.ownerActionHash(from, "send-message", payload)
}

func (svc *TkmPhoneService) startCallSigningHash(from string, to string, offerNonce []byte, offerCiphertext []byte) common.Hash {
	payload := svc.randomXServiceHash("start-call-payload", []byte(from), []byte(to), offerNonce, offerCiphertext)
	return svc.ownerActionHash(from, "start-call", payload)
}

func (svc *TkmPhoneService) acceptCallSigningHash(id uint64, answerNonce []byte, answerCiphertext []byte) (common.Hash, error) {
	svc.lock.RLock()
	call, ok := svc.calls[id]
	svc.lock.RUnlock()
	if !ok {
		return common.Hash{}, errors.New("call not found")
	}
	payload := svc.randomXServiceHash("accept-call-payload", tkmPhoneUint64Bytes(id), answerNonce, answerCiphertext)
	return svc.ownerActionHash(call.To, "accept-call", payload), nil
}

func (svc *TkmPhoneService) rejectCallSigningHash(id uint64, number string, reason string) common.Hash {
	payload := svc.randomXServiceHash("reject-call-payload", tkmPhoneUint64Bytes(id), []byte(reason))
	return svc.ownerActionHash(number, "reject-call", payload)
}

func (svc *TkmPhoneService) endCallSigningHash(id uint64, number string) common.Hash {
	payload := svc.randomXServiceHash("end-call-payload", tkmPhoneUint64Bytes(id))
	return svc.ownerActionHash(number, "end-call", payload)
}

func (svc *TkmPhoneService) callCandidateHash(id uint64, number string, nonce []byte, ciphertext []byte) common.Hash {
	return svc.randomXServiceHash("call-candidate-payload", tkmPhoneUint64Bytes(id), []byte(number), nonce, ciphertext)
}

func (svc *TkmPhoneService) callCandidateSigningHash(id uint64, number string, nonce []byte, ciphertext []byte) common.Hash {
	return svc.ownerActionHash(number, "add-call-candidate", svc.callCandidateHash(id, number, nonce, ciphertext))
}

func (svc *TkmPhoneService) callCandidateListHash(id uint64, number string) common.Hash {
	return svc.randomXServiceHash("list-call-candidates-payload", tkmPhoneUint64Bytes(id), []byte(number))
}

func (svc *TkmPhoneService) callCandidateListSigningHash(id uint64, number string) common.Hash {
	return svc.ownerActionHash(number, "list-call-candidates", svc.callCandidateListHash(id, number))
}

func (svc *TkmPhoneService) messageKey(from string, to string, nonce []byte) common.Hash {
	return svc.randomXServiceHash("message-key", []byte(from), []byte(to), nonce)
}

func (svc *TkmPhoneService) stablePhoneHash(label string, parts ...[]byte) common.Hash {
	payload := []byte("TKMPHONE_STABLE_HASH_V1")
	payload = append(payload, []byte(label)...)
	payload = append(payload, svc.chainID.Bytes()...)
	for _, part := range parts {
		payload = append(payload, tkmPhoneUint64Bytes(uint64(len(part)))...)
		payload = append(payload, part...)
	}
	return crypto.Keccak256Hash(payload)
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
