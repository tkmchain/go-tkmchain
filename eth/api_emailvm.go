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
	emailVMActionVersion       = uint64(1)
	emailVMBuiltinDomain       = "tkm"
	emailVMMaxDomainUnits      = uint64(1_000_000)
	emailVMMaxApplicationBytes = 12 * 1024
	emailVMMaxCiphertextBytes  = 8 * 1024
	emailVMMaxKeyBytes         = 4 * 1024
	// Keep 0.1 TKM of the uint64 public-release range available for shielded
	// gas sponsorship. The prover computes the exact reserve for each part.
	emailVMMaxWithdrawalPartWei = uint64(18_346_744_073_709_551_615)
)

var (
	emailVMActionMagic           = []byte("TKMEMAILVM1")
	emailVMStateKey              = []byte("tkm-emailvm-state-v2")
	emailVMDomainRegistrationFee = new(big.Int).Mul(big.NewInt(30_000), big.NewInt(params.Ether))
	emailVMSubscriberUnitFee     = new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))
	emailVMDomainPattern         = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	emailVMUsernamePattern       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
)

type EmailVMService struct {
	lock         sync.Mutex
	eth          *Ethereum
	superAddress common.Address
	superTx      common.Hash
	superBlock   uint64
	db           ethdb.KeyValueStore
	dir          string
	initialized  bool
	indexed      uint64
	indexedHash  common.Hash
	domains      map[string]EmailDomain
	mailboxes    map[string]EmailMailbox
	keys         map[string]EmailMailboxKey
	messages     map[common.Hash]EmailMessage
	pending      map[string]EmailPendingPayment
}

type emailVMSnapshot struct {
	Initialized  bool
	Indexed      uint64
	IndexedHash  common.Hash
	Domains      map[string]EmailDomain
	Mailboxes    map[string]EmailMailbox
	Keys         map[string]EmailMailboxKey
	Messages     map[common.Hash]EmailMessage
	Pending      map[string]EmailPendingPayment
	SuperAddress common.Address
	SuperTx      common.Hash
	SuperBlock   uint64
}

type emailVMAction struct {
	Version    uint64 `json:"v"`
	Kind       string `json:"kind"`
	Domain     string `json:"domain,omitempty"`
	Username   string `json:"username,omitempty"`
	Units      uint64 `json:"units,omitempty"`
	Mailbox    string `json:"mailbox,omitempty"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Key        string `json:"key,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
	Nonce      string `json:"nonce,omitempty"`
	Payout     string `json:"payout,omitempty"`
}

type EmailDomain struct {
	Name             string         `json:"name"`
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
	Timestamp  hexutil.Uint64 `json:"timestamp"`
}

type EmailVMStatus struct {
	Ready        bool           `json:"ready"`
	IndexedBlock hexutil.Uint64 `json:"indexedBlock"`
	IndexedHash  common.Hash    `json:"indexedHash"`
	HeadBlock    hexutil.Uint64 `json:"headBlock"`
	HeadHash     common.Hash    `json:"headHash"`
	Domains      hexutil.Uint64 `json:"domains"`
	Mailboxes    hexutil.Uint64 `json:"mailboxes"`
	Messages     hexutil.Uint64 `json:"messages"`
	Pending      hexutil.Uint64 `json:"pendingPayments"`
	Protocol     string         `json:"protocol"`
	SuperAddress common.Address `json:"superAddress"`
	SuperClaimed bool           `json:"superClaimed"`
	SuperTx      common.Hash    `json:"superTx,omitempty"`
	SuperBlock   hexutil.Uint64 `json:"superBlock,omitempty"`
}

type EmailVMActionPlan struct {
	Kind            string         `json:"kind"`
	OrderID         common.Hash    `json:"orderId"`
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
	svc.keys = make(map[string]EmailMailboxKey)
	svc.messages = make(map[common.Hash]EmailMessage)
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
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "super", Domain: emailVMBuiltinDomain})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return metadataPlan("super", "@"+emailVMBuiltinDomain, data), nil
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
// 30000 + totalUnits*100.
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
	api.service.lock.Lock()
	_, exists := api.service.domains[domain]
	superAddress := api.service.superAddress
	api.service.lock.Unlock()
	if exists {
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
	action := emailVMAction{Version: emailVMActionVersion, Kind: "operator", Domain: domain, Units: uint64(totalUnits)}
	if payout != (common.Address{}) {
		action.Payout = payout.Hex()
	}
	data, err := encodeEmailVMAction(action)
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return paymentPlan("operator", "", domain, uint64(totalUnits), superAddress, quote, data), nil
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
	api.service.lock.Lock()
	defer api.service.lock.Unlock()
	if _, exists := api.service.mailboxes[mailbox]; exists {
		return EmailVMActionPlan{}, errors.New("mailbox is already registered")
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
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "buy", Domain: domain, Username: username})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return paymentPlan("buy", mailbox, domain, 1, recipient, emailVMSubscriberUnitFee, data), nil
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
		return builtinEmailDomain(api.service.superAddress), nil
	}
	record, ok := api.service.domains[name]
	if !ok {
		return EmailDomain{}, errors.New("domain not found")
	}
	return cloneEmailDomain(record), nil
}

func (api *TkmDomainAPI) Domains() ([]EmailDomain, error) { return api.service.domainList() }
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
	data, err := encodeEmailVMAction(emailVMAction{Version: emailVMActionVersion, Kind: "key", Mailbox: mailbox, Key: hex.EncodeToString(publicKey)})
	if err != nil {
		return EmailVMActionPlan{}, err
	}
	return metadataPlan("key", mailbox, data), nil
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
		for _, tx := range block.Transactions() {
			svc.applyTransactionLocked(block, tx)
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

func (svc *EmailVMService) applyTransactionLocked(block *types.Block, tx *types.Transaction) {
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
		svc.applyMessageLocked(action, sender, tx.Hash(), blockNumber, block.Time())
	}
}

func (svc *EmailVMService) applySuperLocked(action emailVMAction, sender common.Address, txHash common.Hash, block uint64) {
	domain, err := normalizeEmailDomain(action.Domain, true)
	if err != nil || domain != emailVMBuiltinDomain || sender == (common.Address{}) || svc.superAddress != (common.Address{}) {
		return
	}
	svc.superAddress = sender
	svc.superTx = txHash
	svc.superBlock = block
}

func (svc *EmailVMService) applyOperatorLocked(action emailVMAction, envelope *core.ShieldedTransaction, sender common.Address, txHash common.Hash, block uint64) {
	domain, err := normalizeEmailDomain(action.Domain, false)
	if err != nil || action.Units == 0 || action.Units > emailVMMaxDomainUnits || sender == (common.Address{}) {
		return
	}
	if _, exists := svc.domains[domain]; exists {
		return
	}
	quote, err := emailVMDomainQuote(action.Units)
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
	key := fmt.Sprintf("operator:%s:%s:%d:%s", sender.Hex(), domain, action.Units, payout.Hex())
	complete, paymentTxs := svc.applyInstallmentLocked(key, "operator", domain, "", sender, svc.superAddress, quote, envelope, txHash, block)
	if !complete {
		return
	}
	capacity := new(big.Int).Mul(new(big.Int).SetUint64(action.Units), emailVMSubscriberUnitFee)
	svc.domains[domain] = EmailDomain{
		Name: domain, Operator: sender, PayoutAddress: payout, TotalUnits: hexutil.Uint64(action.Units), AvailableUnits: hexutil.Uint64(action.Units),
		RegistrationFee: (*hexutil.Big)(new(big.Int).Set(emailVMDomainRegistrationFee)), CapacityFee: (*hexutil.Big)(capacity),
		SubscriberPrice: (*hexutil.Big)(new(big.Int).Set(emailVMSubscriberUnitFee)), RegistrationTx: txHash, PaymentTxs: paymentTxs,
		RegisteredBlock: hexutil.Uint64(block), LastUpdatedBlock: hexutil.Uint64(block),
	}
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
	quote := new(big.Int).Mul(new(big.Int).SetUint64(action.Units), emailVMSubscriberUnitFee)
	if svc.superAddress == (common.Address{}) {
		return
	}
	key := fmt.Sprintf("expand:%s:%s:%d", sender.Hex(), domain, action.Units)
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
	if _, exists := svc.mailboxes[address]; exists {
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
	key := fmt.Sprintf("buy:%s:%s", sender.Hex(), address)
	complete, paymentTxs := svc.applyInstallmentLocked(key, "buy", domain, username, sender, recipient, emailVMSubscriberUnitFee, envelope, txHash, block)
	if !complete {
		return
	}
	svc.mailboxes[address] = EmailMailbox{
		Address: address, Username: username, Domain: domain, Owner: sender, Operator: operator, PaymentRecipient: recipient,
		Price: (*hexutil.Big)(new(big.Int).Set(emailVMSubscriberUnitFee)), PurchaseTx: txHash, PaymentTxs: paymentTxs, CreatedBlock: hexutil.Uint64(block),
	}
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
	svc.mailboxes[mailbox] = record
	svc.keys[mailbox] = EmailMailboxKey{Mailbox: mailbox, Owner: sender, PublicKey: append([]byte(nil), key...), TxHash: txHash, Block: hexutil.Uint64(block)}
}

func (svc *EmailVMService) applyMessageLocked(action emailVMAction, sender common.Address, txHash common.Hash, block uint64, timestamp uint64) {
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
	svc.messages[id] = EmailMessage{
		ID: id, From: from, To: to, Ciphertext: append([]byte(nil), ciphertext...), Nonce: append([]byte(nil), nonce...),
		BodyHash: crypto.Keccak256Hash(ciphertext), TxHash: txHash, Block: hexutil.Uint64(block), Timestamp: hexutil.Uint64(timestamp),
	}
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
	status := EmailVMStatus{Ready: true, IndexedBlock: hexutil.Uint64(svc.indexed), IndexedHash: svc.indexedHash, Domains: hexutil.Uint64(domainCount), Mailboxes: hexutil.Uint64(len(svc.mailboxes)), Messages: hexutil.Uint64(len(svc.messages)), Pending: hexutil.Uint64(len(svc.pending)), Protocol: "shielded-application-data-v1", SuperAddress: svc.superAddress, SuperClaimed: svc.superAddress != (common.Address{}), SuperTx: svc.superTx, SuperBlock: hexutil.Uint64(svc.superBlock)}
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
		out = append(out, builtinEmailDomain(svc.superAddress))
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
	out := make([]EmailMessage, 0)
	for _, message := range svc.messages {
		if (inbox && message.To == mailbox) || (!inbox && message.From == mailbox) {
			out = append(out, cloneEmailMessage(message))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Block == out[j].Block {
			return bytes.Compare(out[i].ID[:], out[j].ID[:]) < 0
		}
		return out[i].Block < out[j].Block
	})
	return out, nil
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
		svc.messages = snapshot.Messages
	}
	if snapshot.Pending != nil {
		svc.pending = snapshot.Pending
	}
	svc.superAddress, svc.superTx, svc.superBlock = snapshot.SuperAddress, snapshot.SuperTx, snapshot.SuperBlock
	return nil
}

func (svc *EmailVMService) saveLocked() error {
	snapshot := emailVMSnapshot{Initialized: svc.initialized, Indexed: svc.indexed, IndexedHash: svc.indexedHash, Domains: svc.domains, Mailboxes: svc.mailboxes, Keys: svc.keys, Messages: svc.messages, Pending: svc.pending, SuperAddress: svc.superAddress, SuperTx: svc.superTx, SuperBlock: svc.superBlock}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if svc.db != nil {
		if err := svc.db.Put(emailVMStateKey, data); err != nil {
			return err
		}
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
	if json.Unmarshal(data[len(emailVMActionMagic):], &action) != nil || action.Version != emailVMActionVersion {
		return emailVMAction{}, false
	}
	return action, true
}

func emailVMDomainQuote(units uint64) (*big.Int, error) {
	if units == 0 || units > emailVMMaxDomainUnits {
		return nil, fmt.Errorf("total units must be between 1 and %d", emailVMMaxDomainUnits)
	}
	capacity := new(big.Int).Mul(new(big.Int).SetUint64(units), emailVMSubscriberUnitFee)
	return new(big.Int).Add(new(big.Int).Set(emailVMDomainRegistrationFee), capacity), nil
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

func builtinEmailDomain(superAddress common.Address) EmailDomain {
	return EmailDomain{Name: emailVMBuiltinDomain, Operator: superAddress, PayoutAddress: superAddress, SubscriberPrice: (*hexutil.Big)(new(big.Int).Set(emailVMSubscriberUnitFee))}
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
