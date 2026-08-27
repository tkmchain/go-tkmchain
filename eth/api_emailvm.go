package eth

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

const (
	emailVMLegacyActionVersion  = uint64(1)
	emailVMPricingActionVersion = uint64(2)
	emailVMActionVersion        = uint64(3)
	emailVMBuiltinDomain        = "tkm"
	emailVMMaxDomainUnits       = uint64(1_000_000)
	emailVMMaxApplicationBytes  = 12 * 1024
	emailVMMaxCiphertextBytes   = 8 * 1024
	emailVMMaxKeyBytes          = 4 * 1024
	// Keep 0.1 TKM of the uint64 public-release range available for shielded
	// gas sponsorship. The prover computes the exact reserve for each part.
	emailVMMaxWithdrawalPartWei = uint64(18_346_744_073_709_551_615)
)

var (
	emailVMActionMagic                 = []byte("TKMEMAILVM1")
	emailVMStateKey                    = []byte("tkm-emailvm-state-v2")
	emailVMMessagePrefix               = []byte("tkm-emailvm-message-v1/")
	emailVMLegacyDomainRegistrationFee = new(big.Int).Mul(big.NewInt(30_000), big.NewInt(params.Ether))
	emailVMLegacySubscriberUnitFee     = new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))
	emailVMDomainRegistrationFee       = new(big.Int).Mul(big.NewInt(2_500), big.NewInt(params.Ether))
	emailVMSubscriberUnitFee           = new(big.Int).SetUint64(params.Ether)
	emailVMDomainPattern               = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	emailVMUsernamePattern             = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
)

type EmailVMService struct {
	lock          sync.Mutex
	eth           *Ethereum
	superAddress  common.Address
	superTx       common.Hash
	superBlock    uint64
	db            ethdb.KeyValueStore
	dir           string
	initialized   bool
	indexed       uint64
	indexedHash   common.Hash
	domains       map[string]EmailDomain
	mailboxes     map[string]EmailMailbox
	registry      map[common.Hash]EmailNameRegistration
	keys          map[string]EmailMailboxKey
	messages      map[common.Hash]EmailMessage
	inboxIndex    map[string][]common.Hash
	outboxIndex   map[string][]common.Hash
	dirtyMessages map[common.Hash]struct{}
	pending       map[string]EmailPendingPayment
}

type emailVMSnapshot struct {
	Initialized  bool
	Indexed      uint64
	IndexedHash  common.Hash
	Domains      map[string]EmailDomain
	Mailboxes    map[string]EmailMailbox
	Registry     map[common.Hash]EmailNameRegistration
	Keys         map[string]EmailMailboxKey
	Messages     map[common.Hash]EmailMessage `json:",omitempty"`
	Pending      map[string]EmailPendingPayment
	SuperAddress common.Address
	SuperTx      common.Hash
	SuperBlock   uint64
}

type emailVMAction struct {
	Version      uint64 `json:"v"`
	Kind         string `json:"kind"`
	Domain       string `json:"domain,omitempty"`
	Username     string `json:"username,omitempty"`
	Units        uint64 `json:"units,omitempty"`
	Mailbox      string `json:"mailbox,omitempty"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	Key          string `json:"key,omitempty"`
	Ciphertext   string `json:"ciphertext,omitempty"`
	Nonce        string `json:"nonce,omitempty"`
	Payout       string `json:"payout,omitempty"`
	RegistryHash string `json:"registryHash,omitempty"`
}

type EmailDomain struct {
	Name             string         `json:"name"`
	RegistryHash     common.Hash    `json:"registryHash"`
	Operator         common.Address `json:"operator"`
	PayoutAddress    common.Address `json:"payoutAddress"`
	TotalUnits       hexutil.Uint64 `json:"totalUnits"`
	UsedUnits        hexutil.Uint64 `json:"usedUnits"`
	AvailableUnits   hexutil.Uint64 `json:"availableUnits"`
	RegistrationFee  *hexutil.Big   `json:"registrationFee"`
	CapacityFee      *hexutil.Big   `json:"capacityFee"`
	SubscriberPrice  *hexutil.Big   `json:"subscriberPrice"`
	RegistrationTx   common.Hash    `json:"registrationTx"`
	PaymentTxs       []common.Hash  `json:"paymentTxs"`
	LastPayoutTx     common.Hash    `json:"lastPayoutTx,omitempty"`
	RegisteredBlock  hexutil.Uint64 `json:"registeredBlock"`
	LastExpansionTx  common.Hash    `json:"lastExpansionTx,omitempty"`
	LastUpdatedBlock hexutil.Uint64 `json:"lastUpdatedBlock"`
}

type EmailMailbox struct {
	Address          string         `json:"address"`
	RegistryHash     common.Hash    `json:"registryHash"`
	Username         string         `json:"username"`
	Domain           string         `json:"domain"`
	Owner            common.Address `json:"owner"`
	Operator         common.Address `json:"operator"`
	PaymentRecipient common.Address `json:"paymentRecipient"`
	Price            *hexutil.Big   `json:"price"`
	PurchaseTx       common.Hash    `json:"purchaseTx"`
	PaymentTxs       []common.Hash  `json:"paymentTxs"`
	CreatedBlock     hexutil.Uint64 `json:"createdBlock"`
	EncryptionKey    hexutil.Bytes  `json:"encryptionKey,omitempty"`
	KeyTx            common.Hash    `json:"keyTx,omitempty"`
	KeyBlock         hexutil.Uint64 `json:"keyBlock,omitempty"`
}

// EmailNameRegistration is the permanent, canonical name-registry entry for
// a domain or full mailbox address. The readable name and its deterministic
// hash are both committed by the proof-bound action stored in the transaction.
type EmailNameRegistration struct {
	RegistryHash common.Hash    `json:"registryHash"`
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	Domain       string         `json:"domain"`
	Username     string         `json:"username,omitempty"`
	Owner        common.Address `json:"owner"`
	TxHash       common.Hash    `json:"txHash"`
	Block        hexutil.Uint64 `json:"block"`
}

type EmailMailboxKey struct {
	Mailbox   string         `json:"mailbox"`
	Owner     common.Address `json:"owner"`
	PublicKey hexutil.Bytes  `json:"publicKey"`
	TxHash    common.Hash    `json:"txHash"`
	Block     hexutil.Uint64 `json:"block"`
}

type EmailMessage struct {
	ID         common.Hash    `json:"id"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Ciphertext hexutil.Bytes  `json:"ciphertext"`
	Nonce      hexutil.Bytes  `json:"nonce"`
	BodyHash   common.Hash    `json:"bodyHash"`
	TxHash     common.Hash    `json:"txHash"`
	Block      hexutil.Uint64 `json:"block"`
	TxIndex    hexutil.Uint64 `json:"transactionIndex"`
	Timestamp  hexutil.Uint64 `json:"timestamp"`
}

// EmailMessagePage is a bounded, newest-first view over the durable message
// index. Offset is stable while the canonical head is unchanged.
type EmailMessagePage struct {
	Messages   []EmailMessage `json:"messages"`
	Offset     hexutil.Uint64 `json:"offset"`
	NextOffset hexutil.Uint64 `json:"nextOffset"`
	Total      hexutil.Uint64 `json:"total"`
	HasMore    bool           `json:"hasMore"`
}

type EmailVMStatus struct {
	Ready         bool           `json:"ready"`
	IndexedBlock  hexutil.Uint64 `json:"indexedBlock"`
	IndexedHash   common.Hash    `json:"indexedHash"`
	HeadBlock     hexutil.Uint64 `json:"headBlock"`
	HeadHash      common.Hash    `json:"headHash"`
	Domains       hexutil.Uint64 `json:"domains"`
	Mailboxes     hexutil.Uint64 `json:"mailboxes"`
	Registrations hexutil.Uint64 `json:"registrations"`
	Messages      hexutil.Uint64 `json:"messages"`
	Pending       hexutil.Uint64 `json:"pendingPayments"`
	Protocol      string         `json:"protocol"`
	MessageStore  string         `json:"messageStore"`
	PageLimit     hexutil.Uint64 `json:"messagePageLimit"`
	SuperAddress  common.Address `json:"superAddress"`
	SuperClaimed  bool           `json:"superClaimed"`
	SuperTx       common.Hash    `json:"superTx,omitempty"`
	SuperBlock    hexutil.Uint64 `json:"superBlock,omitempty"`
}

type EmailVMActionPlan struct {
	Kind            string         `json:"kind"`
	OrderID         common.Hash    `json:"orderId"`
	RegistryHash    common.Hash    `json:"registryHash,omitempty"`
	Mailbox         string         `json:"mailbox,omitempty"`
	Domain          string         `json:"domain,omitempty"`
	Units           hexutil.Uint64 `json:"units,omitempty"`
	Recipient       common.Address `json:"withdrawalRecipient,omitempty"`
	AmountWei       *hexutil.Big   `json:"totalWithdrawalAmountWei,omitempty"`
	AmountTKM       string         `json:"withdrawalAmountTKM,omitempty"`
	MaximumPartWei  *hexutil.Big   `json:"maximumPartWei,omitempty"`
	PartCount       hexutil.Uint64 `json:"partCount,omitempty"`
	ApplicationData hexutil.Bytes  `json:"applicationData"`
	Instructions    string         `json:"instructions"`
}

type EmailPendingPayment struct {
	Key          string         `json:"key"`
	Kind         string         `json:"kind"`
	Domain       string         `json:"domain"`
	Username     string         `json:"username,omitempty"`
	Payer        common.Address `json:"payer"`
	Recipient    common.Address `json:"recipient"`
	Required     *hexutil.Big   `json:"required"`
	Paid         *hexutil.Big   `json:"paid"`
	PaymentTxs   []common.Hash  `json:"paymentTxs"`
	StartedBlock hexutil.Uint64 `json:"startedBlock"`
	UpdatedBlock hexutil.Uint64 `json:"updatedBlock"`
}

type TkmDomainAPI struct{ service *EmailVMService }
type EmailVMAPI struct{ service *EmailVMService }

func NewTkmDomainAPI(e *Ethereum) *TkmDomainAPI { return &TkmDomainAPI{service: e.emailVMService()} }
func NewEmailVMAPI(e *Ethereum) *EmailVMAPI     { return &EmailVMAPI{service: e.emailVMService()} }

func (s *Ethereum) emailVMService() *EmailVMService {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.emailService == nil {
		s.emailService = newEmailVMService(s, s.chainDb, s.emailDir)
	}
	return s.emailService
}

func newEmailVMService(e *Ethereum, db ethdb.KeyValueStore, dir string) *EmailVMService {
	svc := &EmailVMService{eth: e, db: db, dir: dir}
	svc.resetLocked()
	if err := svc.loadLocked(); err != nil {
		// The canonical chain remains the source of truth, so a corrupt optional
		// index is safely discarded and rebuilt on the next query.
		svc.resetLocked()
	} else if len(svc.dirtyMessages) > 0 {
		// Migrate legacy snapshots immediately instead of waiting for the next
		// block to arrive before creating individual message records.
		if err := svc.saveLocked(); err != nil {
			svc.resetLocked()
		}
	}
	return svc
}

func (svc *EmailVMService) resetLocked() {
	svc.initialized = false
	svc.indexed = 0
	svc.indexedHash = common.Hash{}
	svc.superAddress = common.Address{}
	svc.superTx = common.Hash{}
	svc.superBlock = 0
	svc.domains = make(map[string]EmailDomain)
	svc.mailboxes = make(map[string]EmailMailbox)
	svc.registry = make(map[common.Hash]EmailNameRegistration)
	svc.keys = make(map[string]EmailMailboxKey)
	svc.messages = make(map[common.Hash]EmailMessage)
	svc.inboxIndex = make(map[string][]common.Hash)
	svc.outboxIndex = make(map[string][]common.Hash)
	svc.dirtyMessages = make(map[common.Hash]struct{})
	svc.pending = make(map[string]EmailPendingPayment)
}

func (api *TkmDomainAPI) RegistrationFee() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(emailVMDomainRegistrationFee))
}

func (api *TkmDomainAPI) SubscriberUnitPrice() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).Set(emailVMSubscriberUnitFee))
}

// SuperAddress is the first canonical PQ transaction signer to claim @tkm.
// It receives all operator registration/capacity payments and @tkm mailbox
// purchases. A zero result means @tkm has not been claimed yet.
func (api *TkmDomainAPI) SuperAddress() (common.Address, error) {
	if err := api.service.sync(); err != nil {
		return common.Address{}, err
	}
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	return api.service.superAddress, nil
}

// ClaimSuper prepares the action which creates the reserved @tkm namespace.
// The first valid action in canonical chain order wins permanently.
func (api *TkmDomainAPI) ClaimSuper() (EmailVMActionPlan, error) {
	if err := api.service.sync(); err != nil {
		return EmailVMActionPlan{}, err
	}
	api.service.lock.Lock()
	claimed := api.service.superAddress != (common.Address{})
	api.service.lock.Unlock()
	if claimed {
		return EmailVMActionPlan{}, errors.New("@tkm is already claimed")
	}
	registryHash := emailVMRegistryHash("domain", emailVMBuiltinDomain)
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "super", Domain: emailVMBuiltinDomain, RegistryHash: registryHash.Hex()})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	plan := metadataPlan("super", "@"+emailVMBuiltinDomain, data)
	plan.RegistryHash = registryHash
	return plan, nil
}

func (api *TkmDomainAPI) Quote(totalUnits hexutil.Uint64) (*hexutil.Big, error) {
	amount, err := emailVMDomainQuote(uint64(totalUnits))
	if err != nil {
		return nil, err
	}
	return (*hexutil.Big)(amount), nil
}

// Operator returns the shielded-withdrawal plan for registering a custom
// operator domain. amountTKM is an explicit human confirmation and must equal
// 2500 + totalUnits.
func (api *TkmDomainAPI) Operator(totalUnits hexutil.Uint64, amountTKM string, domain string) (EmailVMActionPlan, error) {
	return api.operator(totalUnits, amountTKM, domain, common.Address{})
}

// OperatorWithPayout prepares an operator registration whose subscriber
// revenue is routed to payout. The canonical transaction signer remains the
// domain owner and can update the payout address later.
func (api *TkmDomainAPI) OperatorWithPayout(totalUnits hexutil.Uint64, amountTKM string, domain string, payout common.Address) (EmailVMActionPlan, error) {
	if payout == (common.Address{}) {
		return EmailVMActionPlan{}, errors.New("operator payout address must not be zero")
	}
	return api.operator(totalUnits, amountTKM, domain, payout)
}

func (api *TkmDomainAPI) operator(totalUnits hexutil.Uint64, amountTKM string, domain string, payout common.Address) (EmailVMActionPlan, error) {
	domain, err := normalizeEmailDomain(domain, false)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	if api.service.eth != nil {
		if err := api.service.sync(); err != nil {
			return EmailVMActionPlan{}, err
		}
	}
	registryHash := emailVMRegistryHash("domain", domain)
	api.service.lock.Lock()
	_, exists := api.service.domains[domain]
	_, hashExists := api.service.registry[registryHash]
	superAddress := api.service.superAddress
	api.service.lock.Unlock()
	if exists || hashExists {
		return EmailVMActionPlan{}, errors.New("domain is already registered")
	}
	if superAddress == (common.Address{}) {
		return EmailVMActionPlan{}, errors.New("@tkm must be claimed before operator registration")
	}
	quote, err := emailVMDomainQuote(uint64(totalUnits))
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	confirmed, err := parseWholeTKM(amountTKM)
	if err != nil || confirmed.Cmp(quote) != 0 {
		return EmailVMActionPlan{}, fmt.Errorf("amount must equal domain quote %s TKM", weiToWholeTKM(quote))
	}
	action := emailVMAction{Version: emailVMActionVersion, Kind: "operator", Domain: domain, Units: uint64(totalUnits), RegistryHash: registryHash.Hex()}
	if payout != (common.Address{}) {
		action.Payout = payout.Hex()
	}
	data, err := encodeEmailVMAction(action)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	plan := paymentPlan("operator", "", domain, uint64(totalUnits), superAddress, quote, data)
	plan.RegistryHash = registryHash
	return plan, nil
}

// SetPayout returns proof-bound metadata for changing where a custom domain's
// future subscriber purchases are paid. Only the canonical domain owner may
// activate the change; that authorization is checked from the PQ transaction
// signer while indexing the block.
func (api *TkmDomainAPI) SetPayout(domain string, payout common.Address) (EmailVMActionPlan, error) {
	domain, err := normalizeEmailDomain(domain, false)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	if payout == (common.Address{}) {
		return EmailVMActionPlan{}, errors.New("operator payout address must not be zero")
	}
	if err := api.service.sync(); err != nil {
		return EmailVMActionPlan{}, err
	}
	api.service.lock.Lock()
	_, exists := api.service.domains[domain]
	api.service.lock.Unlock()
	if !exists {
		return EmailVMActionPlan{}, errors.New("operator domain is not registered")
	}
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "payout", Domain: domain, Payout: payout.Hex()})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return metadataPlan("payout", "", data), nil
}

func (api *TkmDomainAPI) Buy(username string, domain string) (EmailVMActionPlan, error) {
	if err := api.service.sync(); err != nil {
		return EmailVMActionPlan{}, err
	}
	username, err := normalizeEmailUsername(username)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	domain, err = normalizeEmailDomain(domain, true)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	mailbox := username + "@" + domain
	registryHash := emailVMRegistryHash("mailbox", mailbox)
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	if _, exists := api.service.mailboxes[mailbox]; exists {
		return EmailVMActionPlan{}, errors.New("mailbox is already registered")
	}
	if _, exists := api.service.registry[registryHash]; exists {
		return EmailVMActionPlan{}, errors.New("mailbox hash is already registered")
	}
	recipient := api.service.superAddress
	if recipient == (common.Address{}) {
		return EmailVMActionPlan{}, errors.New("@tkm must be claimed before mailbox purchases")
	}
	if domain != emailVMBuiltinDomain {
		record, ok := api.service.domains[domain]
		if !ok {
			return EmailVMActionPlan{}, errors.New("operator domain is not registered")
		}
		if uint64(record.UsedUnits) >= uint64(record.TotalUnits) {
			return EmailVMActionPlan{}, errors.New("operator domain has no subscriber units available")
		}
		recipient = record.PayoutAddress
		if recipient == (common.Address{}) { // backward-compatible cached records
			recipient = record.Operator
		}
	}
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "buy", Domain: domain, Username: username, RegistryHash: registryHash.Hex()})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	plan := paymentPlan("buy", mailbox, domain, 1, recipient, emailVMSubscriberUnitFee, data)
	plan.RegistryHash = registryHash
	return plan, nil
}

func (api *TkmDomainAPI) Expand(domain string, additionalUnits hexutil.Uint64, amountTKM string) (EmailVMActionPlan, error) {
	domain, err := normalizeEmailDomain(domain, false)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	if api.service.eth != nil {
		if err := api.service.sync(); err != nil {
			return EmailVMActionPlan{}, err
		}
	}
	api.service.lock.Lock()
	_, exists := api.service.domains[domain]
	superAddress := api.service.superAddress
	api.service.lock.Unlock()
	if !exists {
		return EmailVMActionPlan{}, errors.New("operator domain is not registered")
	}
	if superAddress == (common.Address{}) {
		return EmailVMActionPlan{}, errors.New("@tkm must be claimed before capacity purchases")
	}
	units := uint64(additionalUnits)
	if units == 0 || units > emailVMMaxDomainUnits {
		return EmailVMActionPlan{}, fmt.Errorf("additional units must be between 1 and %d", emailVMMaxDomainUnits)
	}
	quote := new(big.Int).Mul(new(big.Int).SetUint64(units), emailVMSubscriberUnitFee)
	confirmed, err := parseWholeTKM(amountTKM)
	if err != nil || confirmed.Cmp(quote) != 0 {
		return EmailVMActionPlan{}, fmt.Errorf("amount must equal capacity quote %s TKM", weiToWholeTKM(quote))
	}
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "expand", Domain: domain, Units: units})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return paymentPlan("expand", "", domain, units, superAddress, quote, data), nil
}

func (api *TkmDomainAPI) Domain(name string) (EmailDomain, error) {
	if err := api.service.sync(); err != nil {
		return EmailDomain{}, err
	}
	name, err := normalizeEmailDomain(name, true)
	if err != nil {
		return EmailDomain{}, err
	}
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	if name == emailVMBuiltinDomain {
		if api.service.superAddress == (common.Address{}) {
			return EmailDomain{}, errors.New("@tkm has not been claimed")
		}
		return builtinEmailDomain(api.service.superAddress, api.service.superTx, api.service.superBlock), nil
	}
	record, ok := api.service.domains[name]
	if !ok {
		return EmailDomain{}, errors.New("domain not found")
	}
	return cloneEmailDomain(record), nil
}

func (api *TkmDomainAPI) Domains() ([]EmailDomain, error) { return api.service.domainList() }

// DomainHash returns the deterministic registry hash committed alongside a
// readable canonical domain name in every new registration transaction.
func (api *TkmDomainAPI) DomainHash(name string) (common.Hash, error) {
	name, err := normalizeEmailDomain(name, true)
	if err != nil {
		return common.Hash{}, err
	}
	return emailVMRegistryHash("domain", name), nil
}

// MailboxHash returns the deterministic registry hash for username@domain.
func (api *TkmDomainAPI) MailboxHash(username string, domain string) (common.Hash, error) {
	username, err := normalizeEmailUsername(username)
	if err != nil {
		return common.Hash{}, err
	}
	domain, err = normalizeEmailDomain(domain, true)
	if err != nil {
		return common.Hash{}, err
	}
	return emailVMRegistryHash("mailbox", username+"@"+domain), nil
}

// Registration resolves a permanent name-registry entry by hash.
func (api *TkmDomainAPI) Registration(registryHash common.Hash) (EmailNameRegistration, error) {
	if err := api.service.sync(); err != nil {
		return EmailNameRegistration{}, err
	}
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	record, ok := api.service.registry[registryHash]
	if !ok {
		return EmailNameRegistration{}, errors.New("name registration not found")
	}
	return record, nil
}

func (api *TkmDomainAPI) Mailbox(address string) (EmailMailbox, error) {
	return api.service.mailbox(address)
}
func (api *TkmDomainAPI) Mailboxes(domain string) ([]EmailMailbox, error) {
	return api.service.mailboxList(domain)
}
func (api *TkmDomainAPI) Pending() ([]EmailPendingPayment, error) {
	return api.service.pendingList()
}
func (api *TkmDomainAPI) Status() (EmailVMStatus, error) { return api.service.status() }
func (api *TkmDomainAPI) Sync() (EmailVMStatus, error)   { return api.service.status() }

func (api *EmailVMAPI) Status() (EmailVMStatus, error) { return api.service.status() }

func (api *EmailVMAPI) PublishKey(mailbox string, publicKey hexutil.Bytes) (EmailVMActionPlan, error) {
	mailbox, _, _, err := normalizeEmailAddress(mailbox)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	if len(publicKey) != 32 {
		return EmailVMActionPlan{}, errors.New("X25519 encryption public key must be exactly 32 bytes")
	}
	if _, err := ecdh.X25519().NewPublicKey(publicKey); err != nil {
		return EmailVMActionPlan{}, errors.New("invalid X25519 encryption public key")
	}
	if err := api.service.sync(); err != nil {
		return EmailVMActionPlan{}, err
	}
	api.service.lock.Lock()
	_, exists := api.service.mailboxes[mailbox]
	api.service.lock.Unlock()
	if !exists {
		return EmailVMActionPlan{}, errors.New("mailbox must be canonically registered before publishing an encryption key")
	}
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "key", Mailbox: mailbox, Key: hex.EncodeToString(publicKey)})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return metadataPlan("key", mailbox, data), nil
}

// Key returns the latest canonical X25519 public-key publication for mailbox.
// The matching private key never leaves the client that generated or imported
// its encrypted portable mail-key file.
func (api *EmailVMAPI) Key(mailbox string) (EmailMailboxKey, error) {
	mailbox, _, _, err := normalizeEmailAddress(mailbox)
	if err != nil {
		return EmailMailboxKey{}, err
	}
	if err := api.service.sync(); err != nil {
		return EmailMailboxKey{}, err
	}
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	record, exists := api.service.keys[mailbox]
	if !exists {
		return EmailMailboxKey{}, errors.New("mailbox encryption key is not published")
	}
	record.PublicKey = append([]byte(nil), record.PublicKey...)
	return record, nil
}

func (api *EmailVMAPI) Send(from string, to string, ciphertext hexutil.Bytes, nonce hexutil.Bytes) (EmailVMActionPlan, error) {
	from, _, _, err := normalizeEmailAddress(from)
	if err != nil {
		return EmailVMActionPlan{}, fmt.Errorf("from: %w", err)
	}
	to, _, _, err = normalizeEmailAddress(to)
	if err != nil {
		return EmailVMActionPlan{}, fmt.Errorf("to: %w", err)
	}
	if len(ciphertext) == 0 || len(ciphertext) > emailVMMaxCiphertextBytes {
		return EmailVMActionPlan{}, fmt.Errorf("ciphertext must be between 1 and %d bytes", emailVMMaxCiphertextBytes)
	}
	if len(nonce) < 12 || len(nonce) > 32 {
		return EmailVMActionPlan{}, errors.New("nonce must be between 12 and 32 bytes")
	}
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "message", From: from, To: to, Ciphertext: hex.EncodeToString(ciphertext), Nonce: hex.EncodeToString(nonce)})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return metadataPlan("message", from, data), nil
}

func (api *EmailVMAPI) Inbox(mailbox string) ([]EmailMessage, error) {
	return api.service.messageList(mailbox, true)
}
func (api *EmailVMAPI) Outbox(mailbox string) ([]EmailMessage, error) {
	return api.service.messageList(mailbox, false)
}
func (api *EmailVMAPI) InboxPage(mailbox string, offset hexutil.Uint64, limit hexutil.Uint64) (EmailMessagePage, error) {
	return api.service.messagePage(mailbox, true, uint64(offset), uint64(limit))
}
func (api *EmailVMAPI) OutboxPage(mailbox string, offset hexutil.Uint64, limit hexutil.Uint64) (EmailMessagePage, error) {
	return api.service.messagePage(mailbox, false, uint64(offset), uint64(limit))
}
func (api *EmailVMAPI) Message(id common.Hash) (EmailMessage, error) {
	if err := api.service.sync(); err != nil {
		return EmailMessage{}, err
	}
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	message, ok := api.service.messages[id]
	if !ok {
		return EmailMessage{}, errors.New("email message not found")
	}
	return cloneEmailMessage(message), nil
}

func paymentPlan(kind, mailbox, domain string, units uint64, recipient common.Address, amount *big.Int, data []byte) EmailVMActionPlan {
	maximumPart := new(big.Int).SetUint64(emailVMMaxWithdrawalPartWei)
	partCount := new(big.Int).Add(new(big.Int).Set(amount), new(big.Int).Sub(maximumPart, big.NewInt(1)))
	partCount.Div(partCount, maximumPart)
	return EmailVMActionPlan{
		Kind: kind, OrderID: crypto.Keccak256Hash(data), Mailbox: mailbox, Domain: domain, Units: hexutil.Uint64(units), Recipient: recipient,
		AmountWei: (*hexutil.Big)(new(big.Int).Set(amount)), AmountTKM: weiToWholeTKM(amount), MaximumPartWei: (*hexutil.Big)(maximumPart), PartCount: hexutil.Uint64(partCount.Uint64()), ApplicationData: append([]byte(nil), data...),
		Instructions: "Split totalWithdrawalAmountWei into partCount shielded withdrawals no larger than maximumPartWei. Attach the same applicationData to every part, sign locally with the PQ wallet, and broadcast all parts; registration activates only after the exact total is canonical.",
	}
}

func metadataPlan(kind, mailbox string, data []byte) EmailVMActionPlan {
	return EmailVMActionPlan{
		Kind: kind, OrderID: crypto.Keccak256Hash(data), Mailbox: mailbox, ApplicationData: append([]byte(nil), data...),
		Instructions: "Attach applicationData to the next shielded spend (a self-transfer is sufficient), sign locally with the PQ wallet, and submit the raw transaction.",
	}
}

func (svc *EmailVMService) sync() error {
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if svc.eth == nil || svc.eth.blockchain == nil {
		return errors.New("canonical chain is unavailable")
	}
	head := svc.eth.blockchain.CurrentBlock()
	if head == nil {
		return errors.New("canonical head is unavailable")
	}
	if svc.initialized {
		canonical := svc.eth.blockchain.GetBlockByNumber(svc.indexed)
		if canonical == nil || canonical.Hash() != svc.indexedHash || svc.indexed > head.Number.Uint64() {
			if err := svc.purgeMessageRecordsLocked(); err != nil {
				return err
			}
			svc.resetLocked()
		}
	}
	start := uint64(0)
	if svc.initialized {
		if svc.indexed == head.Number.Uint64() {
			return nil
		}
		start = svc.indexed + 1
	}
	for number := start; number <= head.Number.Uint64(); number++ {
		block := svc.eth.blockchain.GetBlockByNumber(number)
		if block == nil {
			return fmt.Errorf("canonical block %d is unavailable", number)
		}
		for txIndex, tx := range block.Transactions() {
			svc.applyTransactionLocked(block, tx, uint64(txIndex))
		}
		svc.initialized = true
		svc.indexed = number
		svc.indexedHash = block.Hash()
		if number == ^uint64(0) {
			break
		}
	}
	return svc.saveLocked()
}

func (svc *EmailVMService) applyTransactionLocked(block *types.Block, tx *types.Transaction, txIndex uint64) {
	envelope, ok, err := core.DecodeShieldedTransaction(tx.Data())
	if err != nil || !ok || envelope == nil || len(envelope.Spends) == 0 {
		return
	}
	action, ok := decodeEmailVMAction(envelope.Spends[0].EncryptedSpendData)
	if !ok {
		return
	}
	sender, err := types.Sender(types.LatestSigner(svc.eth.blockchain.Config()), tx)
	if err != nil {
		return
	}
	blockNumber := block.NumberU64()
	switch action.Kind {
	case "super":
		svc.applySuperLocked(action, sender, tx.Hash(), blockNumber)
	case "operator":
		svc.applyOperatorLocked(action, envelope, sender, tx.Hash(), blockNumber)
	case "expand":
		svc.applyExpansionLocked(action, envelope, sender, tx.Hash(), blockNumber)
	case "buy":
		svc.applyMailboxPurchaseLocked(action, envelope, sender, tx.Hash(), blockNumber)
	case "payout":
		svc.applyPayoutLocked(action, sender, tx.Hash(), blockNumber)
	case "key":
		svc.applyMailboxKeyLocked(action, sender, tx.Hash(), blockNumber)
	case "message":
		svc.applyMessageLocked(action, sender, tx.Hash(), blockNumber, txIndex, block.Time())
	}
}

func (svc *EmailVMService) applySuperLocked(action emailVMAction, sender common.Address, txHash common.Hash, block uint64) {
	domain, err := normalizeEmailDomain(action.Domain, true)
	if err != nil || domain != emailVMBuiltinDomain || sender == (common.Address{}) || svc.superAddress != (common.Address{}) {
		return
	}
	registryHash, ok := emailVMActionRegistryHash(action, "domain", domain)
	if !ok {
		return
	}
	if _, exists := svc.registry[registryHash]; exists {
		return
	}
	svc.superAddress = sender
	svc.superTx = txHash
	svc.superBlock = block
	svc.registry[registryHash] = EmailNameRegistration{RegistryHash: registryHash, Kind: "domain", Name: domain, Domain: domain, Owner: sender, TxHash: txHash, Block: hexutil.Uint64(block)}
}

func (svc *EmailVMService) applyOperatorLocked(action emailVMAction, envelope *core.ShieldedTransaction, sender common.Address, txHash common.Hash, block uint64) {
	domain, err := normalizeEmailDomain(action.Domain, false)
	if err != nil || action.Units == 0 || action.Units > emailVMMaxDomainUnits || sender == (common.Address{}) {
		return
	}
	registryHash, hashOK := emailVMActionRegistryHash(action, "domain", domain)
	if !hashOK {
		return
	}
	if _, exists := svc.domains[domain]; exists {
		return
	}
	if _, exists := svc.registry[registryHash]; exists {
		return
	}
	quote, err := emailVMDomainQuoteForVersion(action.Units, action.Version)
	if err != nil {
		return
	}
	payout, ok := emailVMActionPayout(action.Payout, sender)
	if !ok {
		return
	}
	if svc.superAddress == (common.Address{}) {
		return
	}
	registrationFee, subscriberFee := emailVMFees(action.Version)
	key := emailVMPendingKey(action.Version, fmt.Sprintf("operator:%s:%s:%d:%s", sender.Hex(), domain, action.Units, payout.Hex()))
	complete, paymentTxs := svc.applyInstallmentLocked(key, "operator", domain, "", sender, svc.superAddress, quote, envelope, txHash, block)
	if !complete {
		return
	}
	capacity := new(big.Int).Mul(new(big.Int).SetUint64(action.Units), subscriberFee)
	svc.domains[domain] = EmailDomain{
		Name: domain, RegistryHash: registryHash, Operator: sender, PayoutAddress: payout, TotalUnits: hexutil.Uint64(action.Units), AvailableUnits: hexutil.Uint64(action.Units),
		RegistrationFee: (*hexutil.Big)(registrationFee), CapacityFee: (*hexutil.Big)(capacity),
		SubscriberPrice: (*hexutil.Big)(subscriberFee), RegistrationTx: txHash, PaymentTxs: paymentTxs,
		RegisteredBlock: hexutil.Uint64(block), LastUpdatedBlock: hexutil.Uint64(block),
	}
	svc.registry[registryHash] = EmailNameRegistration{RegistryHash: registryHash, Kind: "domain", Name: domain, Domain: domain, Owner: sender, TxHash: txHash, Block: hexutil.Uint64(block)}
}

func (svc *EmailVMService) applyPayoutLocked(action emailVMAction, sender common.Address, txHash common.Hash, block uint64) {
	domain, err := normalizeEmailDomain(action.Domain, false)
	if err != nil {
		return
	}
	payout, ok := emailVMActionPayout(action.Payout, common.Address{})
	if !ok {
		return
	}
	record, exists := svc.domains[domain]
	if !exists || record.Operator != sender {
		return
	}
	record.PayoutAddress = payout
	record.LastUpdatedBlock = hexutil.Uint64(block)
	record.LastPayoutTx = txHash
	svc.domains[domain] = record
}

func (svc *EmailVMService) applyExpansionLocked(action emailVMAction, envelope *core.ShieldedTransaction, sender common.Address, txHash common.Hash, block uint64) {
	domain, err := normalizeEmailDomain(action.Domain, false)
	if err != nil || action.Units == 0 || action.Units > emailVMMaxDomainUnits {
		return
	}
	record, ok := svc.domains[domain]
	if !ok || record.Operator != sender || uint64(record.TotalUnits) > emailVMMaxDomainUnits-action.Units {
		return
	}
	_, subscriberFee := emailVMFees(action.Version)
	quote := new(big.Int).Mul(new(big.Int).SetUint64(action.Units), subscriberFee)
	if svc.superAddress == (common.Address{}) {
		return
	}
	key := emailVMPendingKey(action.Version, fmt.Sprintf("expand:%s:%s:%d", sender.Hex(), domain, action.Units))
	complete, _ := svc.applyInstallmentLocked(key, "expand", domain, "", sender, svc.superAddress, quote, envelope, txHash, block)
	if !complete {
		return
	}
	record.TotalUnits = hexutil.Uint64(uint64(record.TotalUnits) + action.Units)
	record.AvailableUnits = hexutil.Uint64(uint64(record.TotalUnits) - uint64(record.UsedUnits))
	record.CapacityFee = (*hexutil.Big)(new(big.Int).Add(record.CapacityFee.ToInt(), quote))
	record.LastExpansionTx = txHash
	record.LastUpdatedBlock = hexutil.Uint64(block)
	svc.domains[domain] = record
}

func (svc *EmailVMService) applyMailboxPurchaseLocked(action emailVMAction, envelope *core.ShieldedTransaction, sender common.Address, txHash common.Hash, block uint64) {
	username, err := normalizeEmailUsername(action.Username)
	if err != nil {
		return
	}
	domain, err := normalizeEmailDomain(action.Domain, true)
	if err != nil {
		return
	}
	address := username + "@" + domain
	registryHash, hashOK := emailVMActionRegistryHash(action, "mailbox", address)
	if !hashOK {
		return
	}
	if _, exists := svc.mailboxes[address]; exists {
		return
	}
	if _, exists := svc.registry[registryHash]; exists {
		return
	}
	recipient := svc.superAddress
	operator := svc.superAddress
	if recipient == (common.Address{}) {
		return
	}
	if domain != emailVMBuiltinDomain {
		record, ok := svc.domains[domain]
		if !ok || uint64(record.UsedUnits) >= uint64(record.TotalUnits) {
			return
		}
		recipient, operator = record.PayoutAddress, record.Operator
		if recipient == (common.Address{}) {
			recipient = record.Operator
		}
	}
	_, subscriberFee := emailVMFees(action.Version)
	key := emailVMPendingKey(action.Version, fmt.Sprintf("buy:%s:%s", sender.Hex(), address))
	complete, paymentTxs := svc.applyInstallmentLocked(key, "buy", domain, username, sender, recipient, subscriberFee, envelope, txHash, block)
	if !complete {
		return
	}
	svc.mailboxes[address] = EmailMailbox{
		Address: address, RegistryHash: registryHash, Username: username, Domain: domain, Owner: sender, Operator: operator, PaymentRecipient: recipient,
		Price: (*hexutil.Big)(subscriberFee), PurchaseTx: txHash, PaymentTxs: paymentTxs, CreatedBlock: hexutil.Uint64(block),
	}
	svc.registry[registryHash] = EmailNameRegistration{RegistryHash: registryHash, Kind: "mailbox", Name: address, Domain: domain, Username: username, Owner: sender, TxHash: txHash, Block: hexutil.Uint64(block)}
	if domain != emailVMBuiltinDomain {
		record := svc.domains[domain]
		record.UsedUnits++
		record.AvailableUnits = hexutil.Uint64(uint64(record.TotalUnits) - uint64(record.UsedUnits))
		record.LastUpdatedBlock = hexutil.Uint64(block)
		svc.domains[domain] = record
	}
}

func (svc *EmailVMService) applyInstallmentLocked(key, kind, domain, username string, payer, recipient common.Address, required *big.Int, envelope *core.ShieldedTransaction, txHash common.Hash, block uint64) (bool, []common.Hash) {
	amount := emailVMWithdrawalAmount(envelope, recipient)
	if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 64 {
		return false, nil
	}
	pending, exists := svc.pending[key]
	if !exists {
		pending = EmailPendingPayment{Key: key, Kind: kind, Domain: domain, Username: username, Payer: payer, Recipient: recipient, Required: (*hexutil.Big)(new(big.Int).Set(required)), Paid: (*hexutil.Big)(new(big.Int)), StartedBlock: hexutil.Uint64(block)}
	}
	if pending.Payer != payer || pending.Recipient != recipient || pending.Required == nil || pending.Paid == nil || pending.Required.ToInt().Cmp(required) != 0 {
		return false, nil
	}
	for _, existing := range pending.PaymentTxs {
		if existing == txHash {
			return false, nil
		}
	}
	paid := new(big.Int).Add(pending.Paid.ToInt(), amount)
	if paid.Cmp(required) > 0 {
		return false, nil
	}
	pending.Paid = (*hexutil.Big)(paid)
	pending.PaymentTxs = append(pending.PaymentTxs, txHash)
	pending.UpdatedBlock = hexutil.Uint64(block)
	if paid.Cmp(required) != 0 {
		svc.pending[key] = pending
		return false, nil
	}
	delete(svc.pending, key)
	return true, append([]common.Hash(nil), pending.PaymentTxs...)
}

func (svc *EmailVMService) applyMailboxKeyLocked(action emailVMAction, sender common.Address, txHash common.Hash, block uint64) {
	mailbox, _, _, err := normalizeEmailAddress(action.Mailbox)
	if err != nil {
		return
	}
	key, err := hex.DecodeString(action.Key)
	if err != nil || len(key) != 32 {
		return
	}
	if _, err := ecdh.X25519().NewPublicKey(key); err != nil {
		return
	}
	record, ok := svc.mailboxes[mailbox]
	if !ok || record.Owner != sender {
		return
	}
	record.EncryptionKey = append([]byte(nil), key...)
	record.KeyTx = txHash
	record.KeyBlock = hexutil.Uint64(block)
	svc.mailboxes[mailbox] = record
	svc.keys[mailbox] = EmailMailboxKey{Mailbox: mailbox, Owner: sender, PublicKey: append([]byte(nil), key...), TxHash: txHash, Block: hexutil.Uint64(block)}
}

func (svc *EmailVMService) applyMessageLocked(action emailVMAction, sender common.Address, txHash common.Hash, block, txIndex, timestamp uint64) {
	from, _, _, err := normalizeEmailAddress(action.From)
	if err != nil {
		return
	}
	to, _, _, err := normalizeEmailAddress(action.To)
	if err != nil {
		return
	}
	fromRecord, fromOK := svc.mailboxes[from]
	_, toOK := svc.mailboxes[to]
	if !fromOK || !toOK || fromRecord.Owner != sender {
		return
	}
	ciphertext, err := hex.DecodeString(action.Ciphertext)
	if err != nil || len(ciphertext) == 0 || len(ciphertext) > emailVMMaxCiphertextBytes {
		return
	}
	nonce, err := hex.DecodeString(action.Nonce)
	if err != nil || len(nonce) < 12 || len(nonce) > 32 {
		return
	}
	id := crypto.Keccak256Hash([]byte("TKMEMAIL_MESSAGE_V1"), txHash.Bytes(), []byte(from), []byte(to), nonce, ciphertext)
	if _, exists := svc.messages[id]; exists {
		return
	}
	message := EmailMessage{
		ID: id, From: from, To: to, Ciphertext: append([]byte(nil), ciphertext...), Nonce: append([]byte(nil), nonce...),
		BodyHash: crypto.Keccak256Hash(ciphertext), TxHash: txHash, Block: hexutil.Uint64(block), TxIndex: hexutil.Uint64(txIndex), Timestamp: hexutil.Uint64(timestamp),
	}
	svc.messages[id] = message
	svc.inboxIndex[to] = append(svc.inboxIndex[to], id)
	svc.outboxIndex[from] = append(svc.outboxIndex[from], id)
	svc.dirtyMessages[id] = struct{}{}
}

func (svc *EmailVMService) status() (EmailVMStatus, error) {
	if err := svc.sync(); err != nil {
		return EmailVMStatus{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	domainCount := len(svc.domains)
	if svc.superAddress != (common.Address{}) {
		domainCount++
	}
	status := EmailVMStatus{Ready: true, IndexedBlock: hexutil.Uint64(svc.indexed), IndexedHash: svc.indexedHash, Domains: hexutil.Uint64(domainCount), Mailboxes: hexutil.Uint64(len(svc.mailboxes)), Registrations: hexutil.Uint64(len(svc.registry)), Messages: hexutil.Uint64(len(svc.messages)), Pending: hexutil.Uint64(len(svc.pending)), Protocol: "shielded-emailvm-registry-v1", MessageStore: "keyvalue-v1", PageLimit: 100, SuperAddress: svc.superAddress, SuperClaimed: svc.superAddress != (common.Address{}), SuperTx: svc.superTx, SuperBlock: hexutil.Uint64(svc.superBlock)}
	if svc.eth != nil && svc.eth.blockchain != nil {
		if head := svc.eth.blockchain.CurrentBlock(); head != nil {
			status.HeadBlock, status.HeadHash = hexutil.Uint64(head.Number.Uint64()), head.Hash()
		}
	}
	return status, nil
}

func (svc *EmailVMService) pendingList() ([]EmailPendingPayment, error) {
	if err := svc.sync(); err != nil {
		return nil, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	out := make([]EmailPendingPayment, 0, len(svc.pending))
	for _, pending := range svc.pending {
		pending.Required = cloneHexBig(pending.Required)
		pending.Paid = cloneHexBig(pending.Paid)
		pending.PaymentTxs = append([]common.Hash(nil), pending.PaymentTxs...)
		out = append(out, pending)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (svc *EmailVMService) domainList() ([]EmailDomain, error) {
	if err := svc.sync(); err != nil {
		return nil, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	out := make([]EmailDomain, 0, len(svc.domains)+1)
	if svc.superAddress != (common.Address{}) {
		out = append(out, builtinEmailDomain(svc.superAddress, svc.superTx, svc.superBlock))
	}
	for _, domain := range svc.domains {
		out = append(out, cloneEmailDomain(domain))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (svc *EmailVMService) mailbox(address string) (EmailMailbox, error) {
	if err := svc.sync(); err != nil {
		return EmailMailbox{}, err
	}
	address, _, _, err := normalizeEmailAddress(address)
	if err != nil {
		return EmailMailbox{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	record, ok := svc.mailboxes[address]
	if !ok {
		return EmailMailbox{}, errors.New("mailbox not found")
	}
	return cloneEmailMailbox(record), nil
}

func (svc *EmailVMService) mailboxList(domain string) ([]EmailMailbox, error) {
	if err := svc.sync(); err != nil {
		return nil, err
	}
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain != "" {
		var err error
		domain, err = normalizeEmailDomain(domain, true)
		if err != nil {
			return nil, err
		}
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	out := make([]EmailMailbox, 0)
	for _, mailbox := range svc.mailboxes {
		if domain == "" || mailbox.Domain == domain {
			out = append(out, cloneEmailMailbox(mailbox))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

func (svc *EmailVMService) messageList(mailbox string, inbox bool) ([]EmailMessage, error) {
	if err := svc.sync(); err != nil {
		return nil, err
	}
	mailbox, _, _, err := normalizeEmailAddress(mailbox)
	if err != nil {
		return nil, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	if _, ok := svc.mailboxes[mailbox]; !ok {
		return nil, errors.New("mailbox not found")
	}
	ids := svc.outboxIndex[mailbox]
	if inbox {
		ids = svc.inboxIndex[mailbox]
	}
	out := make([]EmailMessage, 0, len(ids))
	for _, id := range ids {
		if message, ok := svc.messages[id]; ok {
			out = append(out, cloneEmailMessage(message))
		}
	}
	return out, nil
}

func (svc *EmailVMService) messagePage(mailbox string, inbox bool, offset, limit uint64) (EmailMessagePage, error) {
	if err := svc.sync(); err != nil {
		return EmailMessagePage{}, err
	}
	svc.lock.Lock()
	defer svc.lock.Unlock()
	return svc.messagePageLocked(mailbox, inbox, offset, limit)
}

func (svc *EmailVMService) messagePageLocked(mailbox string, inbox bool, offset, limit uint64) (EmailMessagePage, error) {
	mailbox, _, _, err := normalizeEmailAddress(mailbox)
	if err != nil {
		return EmailMessagePage{}, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit > 100 {
		return EmailMessagePage{}, errors.New("message page limit must not exceed 100")
	}
	if _, ok := svc.mailboxes[mailbox]; !ok {
		return EmailMessagePage{}, errors.New("mailbox not found")
	}
	ids := svc.outboxIndex[mailbox]
	if inbox {
		ids = svc.inboxIndex[mailbox]
	}
	total := uint64(len(ids))
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	messages := make([]EmailMessage, 0, end-offset)
	for index := offset; index < end; index++ {
		id := ids[total-1-index]
		if message, ok := svc.messages[id]; ok {
			messages = append(messages, cloneEmailMessage(message))
		}
	}
	return EmailMessagePage{
		Messages: messages, Offset: hexutil.Uint64(offset), NextOffset: hexutil.Uint64(end),
		Total: hexutil.Uint64(total), HasMore: end < total,
	}, nil
}

func (svc *EmailVMService) loadLocked() error {
	if svc.db == nil {
		return nil
	}
	data, err := svc.db.Get(emailVMStateKey)
	if err != nil || len(data) == 0 {
		return nil
	}
	var snapshot emailVMSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	svc.initialized, svc.indexed, svc.indexedHash = snapshot.Initialized, snapshot.Indexed, snapshot.IndexedHash
	if snapshot.Domains != nil {
		svc.domains = snapshot.Domains
	}
	if snapshot.Mailboxes != nil {
		svc.mailboxes = snapshot.Mailboxes
	}
	if snapshot.Keys != nil {
		svc.keys = snapshot.Keys
	}
	if snapshot.Messages != nil {
		for id, message := range snapshot.Messages {
			svc.messages[id] = message
			svc.dirtyMessages[id] = struct{}{}
		}
	}
	if snapshot.Pending != nil {
		svc.pending = snapshot.Pending
	}
	svc.superAddress, svc.superTx, svc.superBlock = snapshot.SuperAddress, snapshot.SuperTx, snapshot.SuperBlock
	iterator := svc.db.NewIterator(emailVMMessagePrefix, nil)
	defer iterator.Release()
	for iterator.Next() {
		var message EmailMessage
		if err := json.Unmarshal(iterator.Value(), &message); err != nil {
			return fmt.Errorf("decode EmailVM message record: %w", err)
		}
		if message.ID == (common.Hash{}) {
			return errors.New("EmailVM message database contains a record without an ID")
		}
		svc.messages[message.ID] = message
	}
	if err := iterator.Error(); err != nil {
		return err
	}
	svc.rebuildMessageIndexesLocked()
	svc.ensureRegistryLocked()
	return nil
}

func (svc *EmailVMService) saveLocked() error {
	var snapshotMessages map[common.Hash]EmailMessage
	if svc.db == nil {
		snapshotMessages = svc.messages
	}
	snapshot := emailVMSnapshot{Initialized: svc.initialized, Indexed: svc.indexed, IndexedHash: svc.indexedHash, Domains: svc.domains, Mailboxes: svc.mailboxes, Registry: svc.registry, Keys: svc.keys, Messages: snapshotMessages, Pending: svc.pending, SuperAddress: svc.superAddress, SuperTx: svc.superTx, SuperBlock: svc.superBlock}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if svc.db != nil {
		batch := svc.db.NewBatch()
		defer batch.Close()
		for id := range svc.dirtyMessages {
			message, ok := svc.messages[id]
			if !ok {
				continue
			}
			encoded, err := json.Marshal(message)
			if err != nil {
				return err
			}
			if err := batch.Put(emailVMMessageKey(id), encoded); err != nil {
				return err
			}
		}
		if err := batch.Put(emailVMStateKey, data); err != nil {
			return err
		}
		if err := batch.Write(); err != nil {
			return err
		}
		clear(svc.dirtyMessages)
	}
	if svc.dir == "" {
		return nil
	}
	if err := os.MkdirAll(svc.dir, 0700); err != nil {
		return err
	}
	tmp, path := filepath.Join(svc.dir, "state.json.tmp"), filepath.Join(svc.dir, "state.json")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func emailVMMessageKey(id common.Hash) []byte {
	key := make([]byte, 0, len(emailVMMessagePrefix)+len(id))
	key = append(key, emailVMMessagePrefix...)
	return append(key, id[:]...)
}

func (svc *EmailVMService) rebuildMessageIndexesLocked() {
	svc.inboxIndex = make(map[string][]common.Hash)
	svc.outboxIndex = make(map[string][]common.Hash)
	ordered := make([]EmailMessage, 0, len(svc.messages))
	for _, message := range svc.messages {
		ordered = append(ordered, message)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Block == ordered[j].Block {
			if ordered[i].TxIndex != ordered[j].TxIndex {
				return ordered[i].TxIndex < ordered[j].TxIndex
			}
			return bytes.Compare(ordered[i].ID[:], ordered[j].ID[:]) < 0
		}
		return ordered[i].Block < ordered[j].Block
	})
	for _, message := range ordered {
		svc.inboxIndex[message.To] = append(svc.inboxIndex[message.To], message.ID)
		svc.outboxIndex[message.From] = append(svc.outboxIndex[message.From], message.ID)
	}
}

func (svc *EmailVMService) purgeMessageRecordsLocked() error {
	if svc.db == nil {
		return nil
	}
	iterator := svc.db.NewIterator(emailVMMessagePrefix, nil)
	defer iterator.Release()
	batch := svc.db.NewBatch()
	defer batch.Close()
	for iterator.Next() {
		if err := batch.Delete(append([]byte(nil), iterator.Key()...)); err != nil {
			return err
		}
	}
	if err := iterator.Error(); err != nil {
		return err
	}
	return batch.Write()
}

func encodeEmailVMAction(action emailVMAction) ([]byte, error) {
	action.Version = emailVMActionVersion
	payload, err := json.Marshal(action)
	if err != nil {
		return nil, err
	}
	data := append(append(make([]byte, 0, len(emailVMActionMagic)+len(payload)), emailVMActionMagic...), payload...)
	if len(data) > emailVMMaxApplicationBytes {
		return nil, fmt.Errorf("email application data exceeds %d bytes", emailVMMaxApplicationBytes)
	}
	return data, nil
}

func decodeEmailVMAction(data []byte) (emailVMAction, bool) {
	if len(data) <= len(emailVMActionMagic) || len(data) > emailVMMaxApplicationBytes || !bytes.Equal(data[:len(emailVMActionMagic)], emailVMActionMagic) {
		return emailVMAction{}, false
	}
	var action emailVMAction
	if json.Unmarshal(data[len(emailVMActionMagic):], &action) != nil || action.Version < emailVMLegacyActionVersion || action.Version > emailVMActionVersion {
		return emailVMAction{}, false
	}
	return action, true
}

func emailVMDomainQuote(units uint64) (*big.Int, error) {
	return emailVMDomainQuoteForVersion(units, emailVMActionVersion)
}

func emailVMDomainQuoteForVersion(units uint64, version uint64) (*big.Int, error) {
	if units == 0 || units > emailVMMaxDomainUnits {
		return nil, fmt.Errorf("total units must be between 1 and %d", emailVMMaxDomainUnits)
	}
	registrationFee, subscriberFee := emailVMFees(version)
	capacity := new(big.Int).Mul(new(big.Int).SetUint64(units), subscriberFee)
	return new(big.Int).Add(registrationFee, capacity), nil
}

func emailVMFees(version uint64) (registrationFee, subscriberFee *big.Int) {
	if version == emailVMLegacyActionVersion {
		return new(big.Int).Set(emailVMLegacyDomainRegistrationFee), new(big.Int).Set(emailVMLegacySubscriberUnitFee)
	}
	return new(big.Int).Set(emailVMDomainRegistrationFee), new(big.Int).Set(emailVMSubscriberUnitFee)
}

func emailVMPendingKey(version uint64, key string) string {
	if version == emailVMLegacyActionVersion {
		return key
	}
	return fmt.Sprintf("v%d:%s", version, key)
}

func emailVMRegistryHash(kind, canonicalName string) common.Hash {
	return crypto.Keccak256Hash([]byte("TKM_EMAILVM_REGISTRY_V1\x00" + kind + "\x00" + canonicalName))
}

func emailVMActionRegistryHash(action emailVMAction, kind, canonicalName string) (common.Hash, bool) {
	expected := emailVMRegistryHash(kind, canonicalName)
	// Versions 1 and 2 predate explicit registry hashes. Their canonical names
	// are still migrated into the same permanent hash registry during replay.
	if action.Version < emailVMActionVersion {
		return expected, true
	}
	return expected, strings.EqualFold(strings.TrimSpace(action.RegistryHash), expected.Hex())
}

func (svc *EmailVMService) ensureRegistryLocked() {
	if svc.registry == nil {
		svc.registry = make(map[common.Hash]EmailNameRegistration)
	}
	if svc.superAddress != (common.Address{}) {
		hash := emailVMRegistryHash("domain", emailVMBuiltinDomain)
		svc.registry[hash] = EmailNameRegistration{RegistryHash: hash, Kind: "domain", Name: emailVMBuiltinDomain, Domain: emailVMBuiltinDomain, Owner: svc.superAddress, TxHash: svc.superTx, Block: hexutil.Uint64(svc.superBlock)}
	}
	for name, record := range svc.domains {
		hash := emailVMRegistryHash("domain", name)
		if record.RegistryHash == (common.Hash{}) {
			record.RegistryHash = hash
			svc.domains[name] = record
		}
		svc.registry[hash] = EmailNameRegistration{RegistryHash: hash, Kind: "domain", Name: name, Domain: name, Owner: record.Operator, TxHash: record.RegistrationTx, Block: record.RegisteredBlock}
	}
	for address, record := range svc.mailboxes {
		hash := emailVMRegistryHash("mailbox", address)
		if record.RegistryHash == (common.Hash{}) {
			record.RegistryHash = hash
			svc.mailboxes[address] = record
		}
		svc.registry[hash] = EmailNameRegistration{RegistryHash: hash, Kind: "mailbox", Name: address, Domain: record.Domain, Username: record.Username, Owner: record.Owner, TxHash: record.PurchaseTx, Block: record.CreatedBlock}
	}
}

func emailVMWithdrawalAmount(envelope *core.ShieldedTransaction, recipient common.Address) *big.Int {
	if envelope == nil || envelope.WithdrawalRecipient != recipient || envelope.WithdrawalValue == nil || envelope.WithdrawalValue.Sign() <= 0 {
		return nil
	}
	return new(big.Int).Set(envelope.WithdrawalValue)
}

func normalizeEmailDomain(value string, allowBuiltin bool) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "@")
	if value == emailVMBuiltinDomain {
		if allowBuiltin {
			return value, nil
		}
		return "", errors.New("tkm is the reserved network domain")
	}
	if !emailVMDomainPattern.MatchString(value) {
		return "", errors.New("domain must be 1-63 lowercase letters, digits, or interior hyphens")
	}
	return value, nil
}

func normalizeEmailUsername(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if !emailVMUsernamePattern.MatchString(value) {
		return "", errors.New("username must be 1-64 lowercase letters, digits, dots, underscores, or interior hyphens")
	}
	return value, nil
}

func normalizeEmailAddress(value string) (string, string, string, error) {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(value)), "@")
	if len(parts) != 2 {
		return "", "", "", errors.New("email address must contain one @ separator")
	}
	username, err := normalizeEmailUsername(parts[0])
	if err != nil {
		return "", "", "", err
	}
	domain, err := normalizeEmailDomain(parts[1], true)
	if err != nil {
		return "", "", "", err
	}
	return username + "@" + domain, username, domain, nil
}

func parseWholeTKM(value string) (*big.Int, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, ".eE+-") {
		return nil, errors.New("amount must be a positive whole TKM value")
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok || amount.Sign() <= 0 {
		return nil, errors.New("amount must be a positive whole TKM value")
	}
	return amount.Mul(amount, big.NewInt(params.Ether)), nil
}

func weiToWholeTKM(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return new(big.Int).Div(new(big.Int).Set(value), big.NewInt(params.Ether)).String()
}

func builtinEmailDomain(superAddress common.Address, registrationTx common.Hash, registeredBlock uint64) EmailDomain {
	return EmailDomain{Name: emailVMBuiltinDomain, RegistryHash: emailVMRegistryHash("domain", emailVMBuiltinDomain), Operator: superAddress, PayoutAddress: superAddress, SubscriberPrice: (*hexutil.Big)(new(big.Int).Set(emailVMSubscriberUnitFee)), RegistrationTx: registrationTx, RegisteredBlock: hexutil.Uint64(registeredBlock)}
}

func emailVMActionPayout(value string, fallback common.Address) (common.Address, bool) {
	if strings.TrimSpace(value) == "" {
		return fallback, fallback != (common.Address{})
	}
	if !common.IsHexAddress(value) {
		return common.Address{}, false
	}
	payout := common.HexToAddress(value)
	return payout, payout != (common.Address{})
}

func cloneEmailDomain(record EmailDomain) EmailDomain {
	record.RegistrationFee = cloneHexBig(record.RegistrationFee)
	record.CapacityFee = cloneHexBig(record.CapacityFee)
	record.SubscriberPrice = cloneHexBig(record.SubscriberPrice)
	record.PaymentTxs = append([]common.Hash(nil), record.PaymentTxs...)
	return record
}

func cloneEmailMailbox(record EmailMailbox) EmailMailbox {
	record.Price = cloneHexBig(record.Price)
	record.EncryptionKey = append([]byte(nil), record.EncryptionKey...)
	record.PaymentTxs = append([]common.Hash(nil), record.PaymentTxs...)
	return record
}

func cloneEmailMessage(record EmailMessage) EmailMessage {
	record.Ciphertext = append([]byte(nil), record.Ciphertext...)
	record.Nonce = append([]byte(nil), record.Nonce...)
	return record
}

func cloneHexBig(value *hexutil.Big) *hexutil.Big {
	if value == nil {
		return nil
	}
	return (*hexutil.Big)(new(big.Int).Set(value.ToInt()))
}
