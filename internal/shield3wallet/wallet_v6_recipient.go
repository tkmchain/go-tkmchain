package shield3wallet

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

type ShieldedV6RecipientClient struct {
	Prover *recipient.PreparedProver
	Codes  []recipient.PaymentCode
}

func ShieldedV6PaymentCode(ctx context.Context, rpc RPC, identity *Identity, keys *shielded6.WalletKeys) (recipient.PaymentCode, error) {
	var code recipient.PaymentCode
	if rpc == nil || identity == nil || keys == nil || identity.ChainID != keys.ChainID {
		return code, errors.New("invalid recipient payment-code request")
	}
	var stamp, binding core.ShieldedV3Path
	if err := rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStampPath", identity.Owner); err != nil {
		return code, err
	}
	if err := rpc.CallContext(ctx, &binding, "tkmprivacy_shieldedV6BindingPath", identity.Owner); err != nil {
		return code, err
	}
	if !stamp.Found || !binding.Found || stamp.Index >= uint64(1)<<32 || binding.Index >= uint64(1)<<32 {
		return code, errors.New("register Shield6 recipient keys before receiving private payments")
	}
	commitment, err := recipient.BindingCommitment(identity.ChainID, identity.Owner, keys.Address())
	if err != nil {
		return code, err
	}
	if binding.Commitment != commitment {
		return code, errors.New("registered Shield6 recipient commitment does not match this wallet")
	}
	code = recipient.PaymentCode{Version: recipient.RelationVersion, Address: keys.Address(), Owner: identity.Owner, StampRoot: stamp.Root, BindingRoot: binding.Root, StampIndex: uint32(stamp.Index), StampPath: stamp.Path, BindingIndex: uint32(binding.Index), BindingPath: binding.Path}
	if err := code.Validate(identity.ChainID); err != nil {
		return recipient.PaymentCode{}, err
	}
	return code, nil
}

func prepareShieldedV6ClientRecipients(ctx context.Context, rpc RPC, status shieldedV6RPCStatus, identity *Identity, keys *shielded6.WalletKeys, outputs []core.ShieldedV6WalletOutput, clients []*ShieldedV6RecipientClient) ([]core.ShieldedV6WalletOutput, error) {
	if len(clients) > 1 {
		return nil, errors.New("multiple recipient prover configurations")
	}
	if !status.RecipientProofsRequired {
		if len(clients) > 0 && clients[0] != nil {
			return nil, errors.New("recipient-proof fork is not active")
		}
		return outputs, nil
	}
	if uint64(status.HeadTime) > ^uint64(0)-core.ShieldedV6MaxLifetime {
		return nil, errors.New("invalid canonical head time")
	}
	if len(outputs) == 0 {
		return outputs, nil
	}
	if len(clients) != 1 || clients[0] == nil || clients[0].Prover == nil || (status.RecipientProvingKeyHash == (common.Hash{}) || common.Hash(clients[0].Prover.Pins().ProvingKey) != status.RecipientProvingKeyHash || common.Hash(clients[0].Prover.Pins().VerifyingKey) != status.RecipientKeyHash) {
		return nil, errors.New("authenticated local recipient proving parameters matching this chain are required")
	}
	client := clients[0]
	codes := client.Codes
	if len(codes) == 0 {
		code, err := ShieldedV6PaymentCode(ctx, rpc, identity, keys)
		if err != nil {
			return nil, err
		}
		codes = make([]recipient.PaymentCode, len(outputs))
		for i, output := range outputs {
			if output.Address != keys.Address() {
				return nil, errors.New("recipient must provide a stamped Shield6 payment code")
			}
			codes[i] = code
		}
	}
	if len(codes) != len(outputs) {
		return nil, errors.New("each confidential output requires a recipient payment code")
	}
	prepared := append([]core.ShieldedV6WalletOutput(nil), outputs...)
	roots := make([]core.ShieldedV6RecipientRoots, len(outputs))
	for i, code := range codes {
		if code.Address != outputs[i].Address || code.Validate(identity.ChainID) != nil {
			return nil, errors.New("recipient code does not match the output")
		}
		prepared[i].Recipient = &core.ShieldedV6WalletRecipient{Code: code, Prover: client.Prover, Context: ctx, ValidUntil: uint64(status.HeadTime) + core.ShieldedV6MaxLifetime}
		roots[i] = core.ShieldedV6RecipientRoots{StampRoot: code.StampRoot, BindingRoot: code.BindingRoot, Anchor: core.ShieldedV6RecipientAnchor(identity.ChainID, code.StampRoot, code.BindingRoot)}
	}
	if err := checkShieldedV6RecipientRoots(ctx, rpc, roots); err != nil {
		return nil, err
	}
	return prepared, nil
}

func checkShieldedV6RecipientRoots(ctx context.Context, rpc RPC, roots []core.ShieldedV6RecipientRoots) error {
	if len(roots) == 0 {
		return nil
	}
	var known bool
	if err := rpc.CallContext(ctx, &known, "tkmprivacy_shieldedV6RecipientRootsKnown", roots); err != nil {
		return err
	}
	if !known {
		return errors.New("recipient registry roots were reorganized; refresh the payment code")
	}
	return nil
}

func recheckShieldedV6RecipientBuild(ctx context.Context, rpc RPC, status shieldedV6RPCStatus, data []byte) error {
	if !status.RecipientProofsRequired {
		return nil
	}
	var current shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &current, "tkmprivacy_shieldedV6Status"); err != nil {
		return err
	}
	if !current.Active || !current.RecipientProofsRequired || current.RecipientKeyHash != status.RecipientKeyHash {
		return errors.New("recipient-proof chain configuration changed during construction")
	}
	e, _, err := core.DecodeShieldedV6Transaction(data)
	if err != nil || e == nil || e.Recipients == nil {
		return errors.New("missing recipient proofs")
	}
	if e.ValidUntil == 0 || e.ValidUntil <= uint64(current.HeadTime) {
		return errors.New("recipient proof expired during construction; retry with a fresh request")
	}
	return checkShieldedV6RecipientRoots(ctx, rpc, e.Recipients.Roots)
}
