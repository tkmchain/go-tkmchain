package core

import (
	"context"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

// ShieldedV6WalletRecipient is private construction data. It is never encoded
// into an envelope or passed to RPC. The receiver supplies the code privately;
// the proving artifacts are installed and authenticated locally.
type ShieldedV6WalletRecipient struct {
	Code       recipient.PaymentCode
	Prover     *recipient.PreparedProver
	Context    context.Context
	ValidUntil uint64
}

func createShieldedV6RecipientOutput(output ShieldedV6WalletOutput, position uint64, amount *big.Int, opening shielded6.WideWitness, random io.Reader, secret *[32]byte) (shielded6.OutputRecord, error) {
	if output.Recipient == nil {
		return shielded6.CreateOutput(output.Address, position, shielded6.AssetTKM, amount, opening, random)
	}
	record, r, err := recipient.CreateOutput(output.Address, position, amount, opening, random)
	*secret = r
	clear(r[:])
	return record, err
}

func prepareShieldedV6Recipients(tx *types.Transaction, e *ShieldedV6Transaction, outputs []ShieldedV6WalletOutput, force bool) error {
	bound := force
	for _, output := range outputs {
		bound = bound || output.Recipient != nil
	}
	if !bound {
		return nil
	}
	e.Version = ShieldedV6BoundVersion
	if len(outputs) > 0 && outputs[0].Recipient != nil {
		e.ValidUntil = outputs[0].Recipient.ValidUntil
	}
	if e.ValidUntil == 0 {
		return ErrInvalidShieldedTx
	}
	e.Recipients = &ShieldedV6RecipientProofs{Roots: make([]ShieldedV6RecipientRoots, len(outputs)), Proofs: make([][]byte, len(outputs))}
	var pin [32]byte
	for i, output := range outputs {
		p := output.Recipient
		if p == nil || p.Prover == nil || p.Context == nil || p.ValidUntil != e.ValidUntil || p.Code.Address != output.Address || p.Code.Validate(tx.ChainId().Uint64()) != nil {
			return ErrInvalidShieldedTx
		}
		if err := p.Context.Err(); err != nil {
			return err
		}
		key := p.Prover.Pins().VerifyingKey
		if i == 0 {
			pin = key
		} else if key != pin {
			return ErrInvalidShieldedTx
		}
		e.Recipients.Roots[i] = ShieldedV6RecipientRoots{StampRoot: p.Code.StampRoot, BindingRoot: p.Code.BindingRoot, Anchor: ShieldedV6RecipientAnchor(tx.ChainId().Uint64(), p.Code.StampRoot, p.Code.BindingRoot)}
	}
	return nil
}

func proveShieldedV6Recipients(tx *types.Transaction, e *ShieldedV6Transaction, outputs []ShieldedV6WalletOutput, secrets [][32]byte) error {
	if e.Recipients == nil {
		return nil
	}
	for i, output := range outputs {
		if output.Recipient == nil {
			return ErrInvalidShieldedTx
		}
		s, err := ShieldedV6RecipientStatement(tx, e, i)
		if err != nil {
			return err
		}
		w := output.Recipient.Code.Witness()
		w.EphemeralSecret = secrets[i]
		proof, err := output.Recipient.Prover.Prove(output.Recipient.Context, s, w)
		w.Clear()
		clear(secrets[i][:])
		if err != nil {
			return err
		}
		e.Recipients.Proofs[i] = proof
	}
	return nil
}
