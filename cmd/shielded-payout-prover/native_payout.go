package main

import (
	"context"
	"errors"
	"math"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type nativePayoutCapacity struct {
	Count                  int
	MaxPayoutWei           string
	ImmediateMaxPayoutWei  string
	AutomaticPreparation   bool
	PublicFundingAvailable bool
	GasReserveWei          string
	PendingFunding         bool
}

// Read-only health uses the same canonical scan as BuildV4, never the legacy
// notes.json inventory. In particular a V2 note cannot fund a V4 payment.
func (p *Prover) nativeCapacity(ctx context.Context) (*nativePayoutCapacity, error) {
	chain, err := p.client.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if p.ks == nil {
		return nil, errors.New("signing keystore unavailable")
	}
	a, err := p.ks.Find(accounts.Account{Address: common.HexToAddress(p.cfg.SignerAddress)})
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(a.URL.Path)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptPQKey(b, p.passphrase)
	if err != nil {
		return nil, err
	}
	defer clear(key.Seed)
	if key.Shield3Stamp == nil {
		return nil, errors.New("pool signer needs its original Shield3 stamp")
	}
	id, err := shield3wallet.NewIdentity(key.Seed, chain.Uint64(), key.Shield3Stamp)
	if err != nil {
		return nil, err
	}
	defer id.Clear()
	view := id.ViewKey()
	defer view.Clear()
	scan, err := shield3wallet.Scan(ctx, p.client.Client(), view)
	if err != nil {
		return nil, err
	}
	price, err := p.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	balance, err := p.client.BalanceAt(ctx, id.Address, nil)
	if err != nil {
		return nil, err
	}
	reserve := new(big.Int).Mul(price, new(big.Int).SetUint64(shield3wallet.WalletGas))
	var active struct {
		ConfidentialLedger bool `json:"confidentialLedger"`
	}
	if err := p.client.Client().CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return nil, err
	}
	privateFee := new(big.Int)
	if p.cfg.PrepaidFeeLimitWei != "" {
		limit, ok := new(big.Int).SetString(p.cfg.PrepaidFeeLimitWei, 10)
		if !ok || !active.ConfidentialLedger || reserve.Cmp(limit) > 0 {
			return nil, errors.New("prepaid fee budget unavailable under current chain rules or gas price")
		}
		privateFee.Set(reserve)
	} else if active.ConfidentialLedger && balance.Cmp(reserve) < 0 {
		return nil, errors.New("public gas balance required unless prepaid note fees are explicitly authorized")
	} else if balance.Cmp(reserve) < 0 {
		privateFee.Set(reserve)
	}
	max, err := nativeNoteCapacity(scan.Notes, privateFee)
	if err != nil {
		return nil, err
	}
	out := &nativePayoutCapacity{Count: len(scan.Notes), MaxPayoutWei: max.String(), ImmediateMaxPayoutWei: max.String(), GasReserveWei: reserve.String()}
	// Advertise eventual capacity when the daemon can privately consolidate.
	// Otherwise a pool would request a public deposit before ever calling the
	// payout endpoint that performs automatic preparation.
	if len(scan.Notes) > shielded3.InputSlots {
		steps := (len(scan.Notes) - shielded3.InputSlots + shielded3.InputSlots - 2) / (shielded3.InputSlots - 1)
		fees := new(big.Int).Mul(reserve, big.NewInt(int64(steps+1)))
		if balance.Cmp(fees) >= 0 {
			total := new(big.Int)
			for _, note := range scan.Notes {
				value, _ := new(big.Int).SetString(note.ValueWei, 10) // nativeNoteCapacity validated these values.
				total.Add(total, value)
			}
			total.Sub(total, privateFee)
			if total.Cmp(shielded3.MaxSendWei()) > 0 {
				total.Set(shielded3.MaxSendWei())
			}
			if total.Cmp(max) > 0 {
				out.MaxPayoutWei = total.String()
				out.AutomaticPreparation = true
			}
		}
	}
	if p.cfg.AutoPublicFunding {
		limit, ok := new(big.Int).SetString(p.cfg.AutoPublicFundingLimitWei, 10)
		if !ok || limit.Sign() <= 0 || limit.Cmp(shielded3.MaxSendWei()) > 0 {
			return nil, errors.New("invalid automatic public funding limit")
		}
		// Include gas for funding, every possible consolidation, and payout.
		steps := 0
		if len(scan.Notes)+1 > shielded3.InputSlots {
			steps = (len(scan.Notes) + 1 - shielded3.InputSlots + shielded3.InputSlots - 2) / (shielded3.InputSlots - 1)
		}
		available := new(big.Int).Sub(balance, new(big.Int).Mul(reserve, big.NewInt(int64(steps+2))))
		if available.Sign() > 0 {
			if available.Cmp(limit) > 0 {
				available.Set(limit)
			}
			for _, note := range scan.Notes {
				value, _ := new(big.Int).SetString(note.ValueWei, 10)
				available.Add(available, value)
			}
			// preparePrivateNotes caps its total target (principal plus fee).
			if available.Cmp(shielded3.MaxSendWei()) > 0 {
				available.Set(shielded3.MaxSendWei())
			}
			available.Sub(available, privateFee)
			current, _ := new(big.Int).SetString(out.MaxPayoutWei, 10)
			if available.Cmp(current) > 0 {
				out.MaxPayoutWei = available.String()
				out.AutomaticPreparation = true
				out.PublicFundingAvailable = true
			}
		}
	}
	confirmedNonce, err := p.client.NonceAt(ctx, id.Address, nil)
	if err != nil {
		return nil, err
	}
	pendingNonce, err := p.client.PendingNonceAt(ctx, id.Address)
	if err != nil {
		return nil, err
	}
	// Historical databases may contain thousands of stale pending entries.
	// Wait for any earlier signer transaction before creating more funding.
	out.PendingFunding = pendingNonce > confirmedNonce
	return out, nil
}

func nativeNoteCapacity(notes []shield3wallet.OwnedNote, fee *big.Int) (*big.Int, error) {
	values := make([]*big.Int, 0, len(notes))
	seen := make(map[shielded3.Digest]bool)
	for _, n := range notes {
		v, ok := new(big.Int).SetString(n.ValueWei, 10)
		if !ok || v.Sign() < 0 || v.BitLen() > 256 || seen[n.Commitment] {
			return nil, errors.New("invalid or duplicate confirmed note")
		}
		seen[n.Commitment] = true
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Cmp(values[j]) > 0 })
	total := new(big.Int)
	for i, v := range values {
		if i == shielded3.InputSlots {
			break
		}
		total.Add(total, v)
	}
	total.Sub(total, fee)
	if total.Sign() < 0 {
		total.SetInt64(0)
	}
	if total.Cmp(shielded3.MaxSendWei()) > 0 {
		total.Set(shielded3.MaxSendWei())
	}
	return total, nil
}

func nativeAmount(raw string) (*big.Int, error) {
	v, ok := parseBigFlexible(raw)
	if !ok || v.Sign() <= 0 || v.Cmp(shielded3.MaxSendWei()) > 0 {
		return nil, errors.New("Shield3/Shield4 amount must be positive and at most 5,000,000 TKM")
	}
	return v, nil
}

func nativeDepositAmount(req DepositRequest) (*big.Int, error) {
	if strings.TrimSpace(req.AmountWei) != "" {
		return nativeAmount(req.AmountWei)
	}
	if req.AmountAntd <= 0 || math.IsNaN(req.AmountAntd) || math.IsInf(req.AmountAntd, 0) {
		return nil, errors.New("deposit amount must be positive")
	}
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(req.AmountAntd, 'f', 8, 64))
	if !ok {
		return nil, errors.New("invalid decimal deposit amount")
	}
	r.Mul(r, new(big.Rat).SetInt(big.NewInt(1e18)))
	return nativeAmount(new(big.Int).Quo(r.Num(), r.Denom()).String())
}

// Native requests bypass the legacy one-note uint64 reservation. The native
// builder selects up to four real unspent notes; pending nullifiers are
// excluded by its canonical scan.
func (p *Prover) processNativePayout(ctx context.Context, req PayoutRequest, db *RequestDB, version uint64) (string, error) {
	if payoutReplacementRequested(req) {
		return "", errors.New("native payout retries must reuse the original signed transaction")
	}
	amount, err := nativeAmount(req.AmountWei)
	if err != nil {
		return "", err
	}
	chain, err := p.client.ChainID(ctx)
	if err != nil {
		return "", err
	}
	seed, id, err := p.signerIdentity(ctx, chain.Uint64())
	if err != nil {
		return "", err
	}
	defer clear(seed)
	defer id.Clear()
	recipient, err := shield3wallet.DecodePaymentCode(req.RecipientCode, chain.Uint64())
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(recipient.Address.Hex(), req.To) || !strings.EqualFold(id.Address.Hex(), req.PoolWallet) {
		return "", errors.New("payout account or recipient code does not match request")
	}
	target := new(big.Int).Set(amount)
	var prepaidLimit *big.Int
	if p.cfg.PrepaidFeeLimitWei != "" {
		var active struct {
			ConfidentialLedger bool `json:"confidentialLedger"`
		}
		if err := p.client.Client().CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
			return "", err
		}
		if !active.ConfidentialLedger {
			return "", errors.New("prepaid note fees are not active")
		}
		var ok bool
		prepaidLimit, ok = new(big.Int).SetString(p.cfg.PrepaidFeeLimitWei, 10)
		if !ok || prepaidLimit.Sign() <= 0 || prepaidLimit.BitLen() > 256 {
			return "", errors.New("invalid configured prepaid fee budget")
		}
		price, err := p.client.SuggestGasPrice(ctx)
		if err != nil {
			return "", err
		}
		fee := new(big.Int).Mul(price, new(big.Int).SetUint64(core.ConfidentialPrepaidGas))
		if fee.Sign() <= 0 || fee.Cmp(prepaidLimit) > 0 {
			return "", errors.New("prepaid payout fee exceeds authorized budget")
		}
		target.Add(target, fee)
	}
	if err := p.preparePrivateNotes(ctx, seed, id, target, version, db); err != nil {
		return "", err
	}
	var tx *types.Transaction
	if prepaidLimit != nil {
		tx, err = shield3wallet.BuildPrepaidPayment(ctx, p.client.Client(), seed, id, []shield3wallet.Payment{{Recipient: recipient, Amount: amount}}, version, prepaidLimit)
	} else if version == 4 {
		tx, err = shield3wallet.BuildV4(ctx, p.client.Client(), seed, id, recipient, amount, false)
	} else if version == 3 {
		tx, err = shield3wallet.Build(ctx, p.client.Client(), seed, id, recipient, amount, false)
	} else {
		return "", errors.New("unsupported native shield version")
	}
	if err != nil {
		return "", err
	}
	signed, err := p.ks.SignTxWithPassphrase(accounts.Account{Address: id.Address}, p.passphrase, tx, chain)
	if err != nil {
		return "", err
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		return "", err
	}
	rec := RequestRecord{Request: req, Status: "signed", TxHash: signed.Hash().Hex(), SignedTransaction: hexutil.Encode(raw), UpdatedAt: time.Now().UTC()}
	db.Requests[req.RequestID] = rec
	if err := writeRequestDB(p.cfg.RequestsPath, *db); err != nil {
		return "", err
	}
	return p.submitNativeRecord(ctx, rec)
}

func (p *Prover) submitNativeRecord(ctx context.Context, rec RequestRecord) (string, error) {
	raw, err := hexutil.Decode(rec.SignedTransaction)
	if err != nil {
		return "", err
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return "", err
	}
	if tx.Type() != types.PQTkmTxType || (!core.HasShieldedV3Prefix(tx.Data()) && !core.HasShieldedV4Prefix(tx.Data())) {
		return "", errors.New("refusing to rebroadcast a saved non-Shield3/Shield4 payout")
	}
	if tx.Hash().Hex() != rec.TxHash {
		return "", errors.New("saved native payout hash mismatch")
	}
	if err := p.client.SendTransaction(ctx, &tx); err != nil {
		// A lost RPC reply or already-mined transaction must not create a new payment.
		if known, _, lookupErr := p.client.TransactionByHash(ctx, tx.Hash()); lookupErr != nil || known == nil {
			return "", err
		}
	}
	return tx.Hash().Hex(), nil
}
