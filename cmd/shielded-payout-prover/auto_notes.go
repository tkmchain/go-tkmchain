package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// This journal is distinct from payout records: a payment to ourselves must
// never be reported as a completed payment to a miner. Persist before broadcast.
type NotePreparationRecord struct {
	PrivacyBoundary   string         `json:"privacyBoundary,omitempty"`
	ChainID           uint64         `json:"chainId"`
	Signer            common.Address `json:"signer"`
	TxHash            string         `json:"txHash"`
	SignedTransaction string         `json:"signedTransaction"`
}

type notePreparationPending struct{ hash, boundary string }

func (e *notePreparationPending) Error() string {
	return "automatic note preparation awaiting confirmation: " + e.hash
}

var errInsufficientPrivateFunds = errors.New("insufficient confirmed private funds; explicitly authorize public shielding to create notes from public balances")

func validateAutomaticFundingConfig(cfg Config) error {
	if cfg.AutoPublicFunding {
		limit, ok := new(big.Int).SetString(cfg.AutoPublicFundingLimitWei, 10)
		if !ok || limit.Sign() <= 0 || limit.Cmp(shielded3.MaxSendWei()) > 0 {
			return errors.New("autoPublicFunding requires autoPublicFundingLimitWei between 1 wei and 5000000 TKM")
		}
	}
	if cfg.PrepaidFeeLimitWei != "" {
		limit, ok := new(big.Int).SetString(cfg.PrepaidFeeLimitWei, 10)
		if !ok || limit.Sign() <= 0 || limit.BitLen() > 256 {
			return errors.New("invalid prepaidFeeLimitWei")
		}
	}
	return nil
}

// privateNotePreparation selects an exact self-payment that combines up to
// four inputs. A retry after confirmation repeats only if more merging is
// needed. No transparent balance is treated as an existing private note.
func privateNotePreparation(notes []shield3wallet.OwnedNote, target *big.Int) (*big.Int, error) {
	if target == nil || target.Sign() <= 0 || target.Cmp(shielded3.MaxSendWei()) > 0 {
		return nil, errors.New("invalid private note target")
	}
	capacity, err := nativeNoteCapacity(notes, new(big.Int))
	if err != nil {
		return nil, err
	}
	if capacity.Cmp(target) >= 0 {
		return nil, nil
	}
	total := new(big.Int)
	for _, note := range notes {
		value, ok := new(big.Int).SetString(note.ValueWei, 10)
		if !ok {
			return nil, errors.New("invalid private note value")
		}
		total.Add(total, value)
	}
	if total.Cmp(target) < 0 {
		return nil, errInsufficientPrivateFunds
	}
	return capacity, nil
}

// Called while ProcessPayout holds p.mu. All operations use only the configured
// signing account and send back to that same account. Public funding requires
// explicit operator configuration and a per-deposit limit.
func (p *Prover) preparePrivateNotes(ctx context.Context, seed []byte, id *shield3wallet.Identity, target *big.Int, version uint64, db *RequestDB) error {
	if saved := db.NotePreparation; saved != nil {
		if id == nil || saved.ChainID != id.ChainID || saved.Signer != id.Address {
			return errors.New("saved note preparation belongs to a different chain or signing account")
		}
		hash := common.HexToHash(saved.TxHash)
		receipt, err := p.client.TransactionReceipt(ctx, hash)
		if errors.Is(err, ethereum.NotFound) {
			_, err = p.submitNativeRecord(ctx, RequestRecord{TxHash: saved.TxHash, SignedTransaction: saved.SignedTransaction})
			if err != nil {
				return fmt.Errorf("rebroadcast private note preparation: %w", err)
			}
			return &notePreparationPending{saved.TxHash, saved.PrivacyBoundary}
		}
		if err != nil {
			return err
		}
		head, err := p.client.HeaderByNumber(ctx, receipt.BlockNumber)
		if err != nil {
			return err
		}
		if head.Hash() != receipt.BlockHash {
			return &notePreparationPending{saved.TxHash, saved.PrivacyBoundary}
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			db.NotePreparation = nil
			if err := writeRequestDB(p.cfg.RequestsPath, *db); err != nil {
				return err
			}
			return errors.New("private note consolidation reverted; balances will be rescanned on retry")
		}
		// A successful receipt is not enough: do not clear the durable journal
		// until the output is visible as a canonical, spendable note. Keeping the
		// journal blocks retries from creating another public deposit when the
		// output is malformed, encrypted to the wrong view key, or not yet indexed.
		view := id.ViewKey()
		scan, err := shield3wallet.Scan(ctx, p.client.Client(), view)
		view.Clear()
		if err != nil {
			return fmt.Errorf("confirmed note preparation %s; rescan failed: %w", saved.TxHash, err)
		}
		if err := requirePreparedNoteCapacity(scan.Notes, target); err != nil {
			return fmt.Errorf("confirmed note preparation %s is not spendable; refusing duplicate funding: %w", saved.TxHash, err)
		}
		db.NotePreparation = nil
		if err := writeRequestDB(p.cfg.RequestsPath, *db); err != nil {
			return err
		}
	}
	view := id.ViewKey()
	defer view.Clear()
	scan, err := shield3wallet.Scan(ctx, p.client.Client(), view)
	if err != nil {
		return err
	}
	amount, err := privateNotePreparation(scan.Notes, target)
	deposit := false
	if errors.Is(err, errInsufficientPrivateFunds) && p.cfg.AutoPublicFunding {
		amount, err = publicFundingAmount(scan.Notes, target, p.cfg.AutoPublicFundingLimitWei)
		deposit = err == nil
	}
	if err != nil || amount == nil {
		return err
	}
	// Keep fees separate from principal, including before the proposed cutoff.
	price, err := p.client.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}
	balance, err := p.client.BalanceAt(ctx, id.Address, nil)
	if err != nil {
		return err
	}
	fee := new(big.Int).Mul(price, new(big.Int).SetUint64(shield3wallet.WalletGas))
	requiredPublic := new(big.Int).Mul(fee, big.NewInt(2))
	if deposit {
		requiredPublic.Add(requiredPublic, amount)
	}
	if balance.Cmp(requiredPublic) < 0 {
		return errors.New("public balance must cover authorized shielding plus gas for preparation and payout")
	}
	confirmed, err := p.client.NonceAt(ctx, id.Address, nil)
	if err != nil {
		return err
	}
	pending, err := p.client.PendingNonceAt(ctx, id.Address)
	if err != nil {
		return err
	}
	if confirmed != pending {
		return errors.New("waiting for earlier signer transactions before automatic note consolidation")
	}
	self, err := shield3wallet.DecodePaymentCode(id.Code, id.ChainID)
	if err != nil {
		return err
	}
	var tx *types.Transaction
	if version == 4 {
		tx, err = shield3wallet.BuildV4(ctx, p.client.Client(), seed, id, self, amount, deposit)
	} else if version == 3 {
		tx, err = shield3wallet.Build(ctx, p.client.Client(), seed, id, self, amount, deposit)
	} else {
		return errors.New("unsupported private note version")
	}
	if err != nil {
		return err
	}
	// Recheck the constructed public fields, not just the requested operation.
	if !deposit {
		if err := validatePrivatePreparation(tx); err != nil {
			return err
		}
	} else {
		boundary, mode, err := core.ShieldedPaymentMetadata(tx)
		if err != nil || boundary != core.PublicShielding || mode != core.ShieldedFeePublic || tx.Value().Cmp(amount) != 0 {
			return errors.New("unexpected public funding transaction")
		}
	}
	signed, err := p.ks.SignTxWithPassphrase(accounts.Account{Address: id.Address}, p.passphrase, tx, new(big.Int).SetUint64(id.ChainID))
	if err != nil {
		return err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return err
	}
	saved := &NotePreparationRecord{ChainID: id.ChainID, Signer: id.Address, TxHash: signed.Hash().Hex(), SignedTransaction: hexutil.Encode(raw)}
	if deposit {
		saved.PrivacyBoundary = string(core.PublicShielding)
	} else {
		saved.PrivacyBoundary = string(core.PrivatePayment)
	}
	db.NotePreparation = saved
	if err := writeRequestDB(p.cfg.RequestsPath, *db); err != nil {
		return err
	}
	if _, err := p.submitNativeRecord(ctx, RequestRecord{TxHash: saved.TxHash, SignedTransaction: saved.SignedTransaction}); err != nil {
		return err
	}
	return &notePreparationPending{saved.TxHash, saved.PrivacyBoundary}
}

func requirePreparedNoteCapacity(notes []shield3wallet.OwnedNote, target *big.Int) error {
	if target == nil || target.Sign() <= 0 {
		return errors.New("invalid note preparation target")
	}
	capacity, err := nativeNoteCapacity(notes, new(big.Int))
	if err != nil {
		return err
	}
	if capacity.Cmp(target) < 0 {
		return fmt.Errorf("spendable note total is below the requested payout target")
	}
	return nil
}

// Fund only the missing principal. A limit is required even when the operator
// enabled automatic public funding. The configured account receives the note.
func publicFundingAmount(notes []shield3wallet.OwnedNote, target *big.Int, limitText string) (*big.Int, error) {
	limit, ok := new(big.Int).SetString(limitText, 10)
	if !ok || limit.Sign() <= 0 || limit.Cmp(shielded3.MaxSendWei()) > 0 {
		return nil, errors.New("automatic public funding requires an explicit positive per-deposit limit up to 5000000 TKM")
	}
	if target == nil || target.Sign() <= 0 || target.Cmp(shielded3.MaxSendWei()) > 0 {
		return nil, errors.New("invalid funding target")
	}
	total := new(big.Int)
	for _, note := range notes {
		n, ok := new(big.Int).SetString(note.ValueWei, 10)
		if !ok || n.Sign() <= 0 || n.Cmp(shielded3.MaxSendWei()) > 0 {
			return nil, errors.New("invalid private note value")
		}
		total.Add(total, n)
	}
	deficit := new(big.Int).Sub(target, total)
	if deficit.Sign() <= 0 {
		return nil, errors.New("public funding is unnecessary")
	}
	if deficit.Cmp(limit) > 0 {
		return nil, errors.New("public funding exceeds the authorized per-deposit limit")
	}
	return deficit, nil
}

func validatePrivatePreparation(tx *types.Transaction) error {
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 {
		return errors.New("note preparation must not publish principal")
	}
	if core.HasShieldedV4Prefix(tx.Data()) {
		e, ok, err := core.DecodeShieldedV4Transaction(tx.Data())
		if err == nil && ok && e != nil && !e.Deposit && e.WithdrawalValue != nil && e.GasSponsorValue != nil && e.WithdrawalValue.Sign() == 0 && e.GasSponsorValue.Sign() == 0 && e.WithdrawalRecipient == (common.Address{}) {
			return nil
		}
	} else if core.HasShieldedV3Prefix(tx.Data()) {
		e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
		if err == nil && ok && e != nil && !e.Deposit && e.WithdrawalValue != nil && e.GasSponsorValue != nil && e.WithdrawalValue.Sign() == 0 && e.GasSponsorValue.Sign() == 0 && e.WithdrawalRecipient == (common.Address{}) {
			return nil
		}
	}
	return errors.New("note preparation cannot use public funding, withdrawals or private gas refunds")
}
