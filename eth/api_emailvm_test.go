package eth

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
)

func TestEmailVMDomainEconomicsAndPlan(t *testing.T) {
	quote, err := emailVMDomainQuote(1000)
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Mul(big.NewInt(3_500), big.NewInt(params.Ether))
	if quote.Cmp(want) != 0 {
		t.Fatalf("quote = %s, want %s", quote, want)
	}
	mainKing := common.HexToAddress("0x1000000000000000000000000000000000000001")
	service := &EmailVMService{}
	service.resetLocked()
	service.superAddress = mainKing
	api := &TkmDomainAPI{service: service}
	plan, err := api.Operator(1000, "3500", "John")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Domain != "john" || plan.Recipient != mainKing || plan.AmountWei.ToInt().Cmp(want) != 0 || uint64(plan.PartCount) <= 1 {
		t.Fatalf("unexpected operator plan: %+v", plan)
	}
	action, ok := decodeEmailVMAction(plan.ApplicationData)
	domainHash := emailVMRegistryHash("domain", "john")
	if !ok || action.Version != emailVMActionVersion || action.Kind != "operator" || action.Domain != "john" || action.Units != 1000 || action.RegistryHash != domainHash.Hex() || plan.RegistryHash != domainHash {
		t.Fatalf("unexpected action: %+v, ok=%v", action, ok)
	}
	if _, err := api.Operator(1000, "3499", "john"); err == nil {
		t.Fatal("incorrect explicit amount was accepted")
	}
	payout := common.HexToAddress("0x4000000000000000000000000000000000000004")
	plan, err = api.OperatorWithPayout(1, "2501", "michael", payout)
	if err != nil {
		t.Fatal(err)
	}
	action, ok = decodeEmailVMAction(plan.ApplicationData)
	if !ok || action.Payout != payout.Hex() {
		t.Fatalf("operator payout was not bound into the plan: %+v", action)
	}
}

func TestEmailVMLegacyDomainEconomicsRemainReplayable(t *testing.T) {
	quote, err := emailVMDomainQuoteForVersion(1000, emailVMLegacyActionVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Mul(big.NewInt(130_000), big.NewInt(params.Ether))
	if quote.Cmp(want) != 0 {
		t.Fatalf("legacy quote = %s, want %s", quote, want)
	}
	action := emailVMAction{Version: emailVMLegacyActionVersion, Kind: "operator", Domain: "legacy", Units: 1000}
	encoded, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok := decodeEmailVMAction(append(append([]byte(nil), emailVMActionMagic...), encoded...))
	if !ok || decoded.Version != emailVMLegacyActionVersion {
		t.Fatalf("legacy action was not decoded: %+v, ok=%v", decoded, ok)
	}
	action.Version = emailVMPricingActionVersion
	encoded, err = json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	decoded, ok = decodeEmailVMAction(append(append([]byte(nil), emailVMActionMagic...), encoded...))
	if !ok || decoded.Version != emailVMPricingActionVersion {
		t.Fatalf("version 2 action was not decoded: %+v, ok=%v", decoded, ok)
	}
	if hash, accepted := emailVMActionRegistryHash(decoded, "domain", "legacy"); !accepted || hash != emailVMRegistryHash("domain", "legacy") {
		t.Fatal("version 2 canonical name was not migrated into the hash registry")
	}
}

func TestEmailVMCanonicalInstallmentsAndMessage(t *testing.T) {
	mainKing := common.HexToAddress("0x1000000000000000000000000000000000000001")
	operator := common.HexToAddress("0x2000000000000000000000000000000000000002")
	buyer := common.HexToAddress("0x3000000000000000000000000000000000000003")
	service := &EmailVMService{}
	service.resetLocked()
	service.applySuperLocked(emailVMAction{Domain: "tkm"}, mainKing, common.HexToHash("0x01"), 1)
	service.applySuperLocked(emailVMAction{Domain: "tkm"}, buyer, common.HexToHash("0x02"), 2)
	if service.superAddress != mainKing {
		t.Fatal("the first canonical @tkm claimant did not remain super address")
	}

	payout := common.HexToAddress("0x4000000000000000000000000000000000000004")
	domainHash := emailVMRegistryHash("domain", "john")
	action := emailVMAction{Version: emailVMActionVersion, Kind: "operator", Domain: "john", Units: 1, Payout: payout.Hex(), RegistryHash: domainHash.Hex()}
	remaining, err := emailVMDomainQuote(action.Units)
	if err != nil {
		t.Fatal(err)
	}
	part := new(big.Int).SetUint64(emailVMMaxWithdrawalPartWei)
	var index uint64
	for remaining.Sign() > 0 {
		value := new(big.Int).Set(part)
		if value.Cmp(remaining) > 0 {
			value.Set(remaining)
		}
		index++
		envelope := &core.ShieldedTransaction{WithdrawalRecipient: mainKing, WithdrawalValue: value}
		service.applyOperatorLocked(action, envelope, operator, common.BigToHash(new(big.Int).SetUint64(index)), index)
		remaining.Sub(remaining, value)
	}
	domain, ok := service.domains["john"]
	if !ok || domain.RegistryHash != domainHash || domain.Operator != operator || domain.PayoutAddress != payout || uint64(domain.TotalUnits) != 1 || uint64(domain.AvailableUnits) != 1 || len(domain.PaymentTxs) < 2 {
		t.Fatalf("domain was not activated after exact installments: %+v", domain)
	}
	if registration := service.registry[domainHash]; registration.Name != "john" || registration.Owner != operator {
		t.Fatalf("domain registry entry is incomplete: %+v", registration)
	}

	mailboxHash := emailVMRegistryHash("mailbox", "alice@john")
	buy := emailVMAction{Version: emailVMActionVersion, Kind: "buy", Domain: "john", Username: "alice", RegistryHash: mailboxHash.Hex()}
	remaining.Set(emailVMSubscriberUnitFee)
	for remaining.Sign() > 0 {
		value := new(big.Int).Set(part)
		if value.Cmp(remaining) > 0 {
			value.Set(remaining)
		}
		index++
		envelope := &core.ShieldedTransaction{WithdrawalRecipient: payout, WithdrawalValue: value}
		service.applyMailboxPurchaseLocked(buy, envelope, buyer, common.BigToHash(new(big.Int).SetUint64(index)), index)
		remaining.Sub(remaining, value)
	}
	mailbox, ok := service.mailboxes["alice@john"]
	if !ok || mailbox.RegistryHash != mailboxHash || mailbox.Owner != buyer || uint64(service.domains["john"].AvailableUnits) != 0 {
		t.Fatalf("mailbox was not activated: %+v", mailbox)
	}
	if registration := service.registry[mailboxHash]; registration.Name != "alice@john" || registration.Owner != buyer {
		t.Fatalf("mailbox registry entry is incomplete: %+v", registration)
	}
	duplicate := &core.ShieldedTransaction{WithdrawalRecipient: payout, WithdrawalValue: new(big.Int).Set(emailVMSubscriberUnitFee)}
	service.applyMailboxPurchaseLocked(buy, duplicate, operator, common.HexToHash("0xffff"), index+1)
	if service.mailboxes["alice@john"].Owner != buyer || len(service.registry) != 3 {
		t.Fatal("a duplicate canonical mailbox purchase replaced the first owner")
	}
	newPayout := common.HexToAddress("0x5000000000000000000000000000000000000005")
	index++
	service.applyPayoutLocked(emailVMAction{Domain: "john", Payout: newPayout.Hex()}, operator, common.BigToHash(new(big.Int).SetUint64(index)), index)
	if service.domains["john"].PayoutAddress != newPayout {
		t.Fatal("operator payout update was not applied")
	}
	service.applyPayoutLocked(emailVMAction{Domain: "john", Payout: buyer.Hex()}, buyer, common.BigToHash(new(big.Int).SetUint64(index+1)), index+1)
	if service.domains["john"].PayoutAddress != newPayout {
		t.Fatal("non-owner changed the operator payout address")
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	index++
	service.applyMailboxKeyLocked(emailVMAction{Mailbox: "alice@john", Key: hex.EncodeToString(key)}, buyer, common.BigToHash(new(big.Int).SetUint64(index)), index)
	if got := service.mailboxes["alice@john"].EncryptionKey; len(got) != len(key) {
		t.Fatalf("published key length = %d", len(got))
	}
	keyRecord, ok := service.keys["alice@john"]
	if !ok || keyRecord.Owner != buyer || len(keyRecord.PublicKey) != len(key) {
		t.Fatalf("canonical network key record is incomplete: %+v", keyRecord)
	}
	wrongKey := append([]byte(nil), key...)
	wrongKey[0] ^= 0xff
	service.applyMailboxKeyLocked(emailVMAction{Mailbox: "alice@john", Key: hex.EncodeToString(wrongKey)}, operator, common.HexToHash("0xeeee"), index+1)
	if got := service.mailboxes["alice@john"].EncryptionKey; !bytes.Equal(got, key) {
		t.Fatal("non-owner replaced the canonical mailbox encryption key")
	}
	index++
	service.applyMessageLocked(emailVMAction{From: "alice@john", To: "alice@john", Ciphertext: "010203", Nonce: "000102030405060708090a0b"}, buyer, common.BigToHash(new(big.Int).SetUint64(index)), index, 1234)
	if len(service.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(service.messages))
	}
}

func TestEmailVMNamesAreCanonical(t *testing.T) {
	if got, err := normalizeEmailDomain("@Michael", false); err != nil || got != "michael" {
		t.Fatalf("domain normalization = %q, %v", got, err)
	}
	if _, err := normalizeEmailDomain("tkm", false); err == nil {
		t.Fatal("reserved tkm operator domain was accepted")
	}
	if got, _, _, err := normalizeEmailAddress("Alice.Smith@John"); err != nil || got != "alice.smith@john" {
		t.Fatalf("mailbox normalization = %q, %v", got, err)
	}
	if got := emailVMRegistryHash("domain", "john"); got != common.HexToHash("0x1856997af25dc25a26ab6b7fd3bdc7aba219ee3c7e3091223d6bc20458ed6e04") {
		t.Fatalf("domain registry hash = %s", got)
	}
	if got := emailVMRegistryHash("mailbox", "alice@john"); got != common.HexToHash("0xc39314210816a49799cc7f12397ec4747fe9bdfad23c6b0801a7532b94d2b3c1") {
		t.Fatalf("mailbox registry hash = %s", got)
	}
	if _, ok := emailVMActionRegistryHash(emailVMAction{Version: emailVMActionVersion, RegistryHash: common.HexToHash("0xdead").Hex()}, "domain", "john"); ok {
		t.Fatal("version 3 action accepted an incorrect registry hash")
	}
}
