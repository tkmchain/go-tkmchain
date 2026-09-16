package shield3wallet

import (
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

// RelayOffer contains no payer identifier or private recipient information.
// A shared stamped operator signs its fee/nonce quote; the payer authorizes the
// exact completed transaction with the native hidden-owner spending proof.
type RelayOffer struct {
	Version    uint64        `json:"version"`
	ChainID    uint64        `json:"chainId"`
	PublicKey  hexutil.Bytes `json:"publicKey"`
	Nonce      uint64        `json:"nonce"`
	Gas        uint64        `json:"gas"`
	GasFeeCap  *hexutil.Big  `json:"gasFeeCap"`
	GasTipCap  *hexutil.Big  `json:"gasTipCap"`
	ValidUntil uint64        `json:"validUntil"`
	Signature  hexutil.Bytes `json:"signature"`
}
type RelayPacket struct {
	Transaction   hexutil.Bytes  `json:"transaction"`
	DraftHash     common.Hash    `json:"draftHash"`
	Relay         common.Address `json:"relay"`
	GasReserveWei string         `json:"gasReserveWei"`
	ValidUntil    uint64         `json:"validUntil"`
	InputCount    uint64         `json:"inputCount"`
}

func relayOfferMessage(o *RelayOffer) ([64]byte, error) {
	if o == nil || o.GasFeeCap == nil || o.GasTipCap == nil {
		return [64]byte{}, errors.New("missing relay fee quote")
	}
	fields := struct {
		Domain                                   string
		Version, ChainID, Nonce, Gas, ValidUntil uint64
		PublicKey                                []byte
		Fee, Tip                                 *big.Int
	}{"TKM_SHIELD3_RELAY_OFFER_V1", o.Version, o.ChainID, o.Nonce, o.Gas, o.ValidUntil, o.PublicKey, (*big.Int)(o.GasFeeCap), (*big.Int)(o.GasTipCap)}
	data, err := rlp.EncodeToBytes(fields)
	if err != nil {
		return [64]byte{}, err
	}
	return sha512.Sum512(data), nil
}
func validateRelayOffer(ctx context.Context, rpc RPC, o *RelayOffer, chainID uint64) error {
	if o == nil || o.Version != 1 || o.ChainID != chainID || o.Gas != WalletGas || o.GasFeeCap == nil || o.GasTipCap == nil || (*big.Int)(o.GasFeeCap).Sign() <= 0 || (*big.Int)(o.GasFeeCap).BitLen() > 256 || (*big.Int)(o.GasTipCap).Sign() < 0 || (*big.Int)(o.GasTipCap).Cmp((*big.Int)(o.GasFeeCap)) > 0 {
		return errors.New("invalid relay offer")
	}
	message, err := relayOfferMessage(o)
	if err != nil || !pqcrypto.VerifyMLDSA87(o.PublicKey, message[:], o.Signature) {
		return errors.New("invalid relay fee quote signature")
	}
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, o.PublicKey)
	if err != nil {
		return err
	}
	return relayState(ctx, rpc, address, o.Nonce, o.ValidUntil)
}
func relayState(ctx context.Context, rpc RPC, address common.Address, nonce, expiry uint64) error {
	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return err
	}
	if !active.Active || !active.NativeVerifier {
		return errors.New("shared relays activate with Shield3 at Antartical")
	}
	var head header
	if err := rpc.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return err
	}
	if expiry <= uint64(head.Timestamp) || expiry-uint64(head.Timestamp) > core.AntarticalStampSponsorshipLifetime {
		return errors.New("relay authorization expired or exceeds one hour")
	}
	var stamp core.AntarticalStampStatus
	if err := rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStamp", address); err != nil {
		return err
	}
	if !stamp.Registered {
		return errors.New("relay must have a confirmed consensus stamp")
	}
	var next hexutil.Uint64
	if err := rpc.CallContext(ctx, &next, "eth_getTransactionCount", address, "pending"); err != nil {
		return err
	}
	if uint64(next) != nonce {
		return errors.New("relay nonce changed; obtain a fresh signed fee offer")
	}
	return nil
}
func BuildRelayOffer(ctx context.Context, rpc RPC, seed []byte, identity *Identity) (RelayOffer, error) {
	if identity == nil {
		return RelayOffer{}, errors.New("missing relay identity")
	}
	if err := RequireRegisteredStamp(ctx, rpc, PaymentPayload{Address: identity.Address, Owner: identity.Owner, Stamp: *identity.Stamp}); err != nil {
		return RelayOffer{}, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return RelayOffer{}, err
	}
	pub := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	if err != nil || address != identity.Address {
		return RelayOffer{}, errors.New("relay seed/account mismatch")
	}
	var nonce hexutil.Uint64
	var price hexutil.Big
	var head header
	if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", address, "pending"); err != nil {
		return RelayOffer{}, err
	}
	if err := rpc.CallContext(ctx, &price, "eth_gasPrice"); err != nil {
		return RelayOffer{}, err
	}
	if err := rpc.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return RelayOffer{}, err
	}
	if uint64(head.Timestamp) > ^uint64(0)-core.AntarticalStampSponsorshipLifetime {
		return RelayOffer{}, errors.New("invalid chain timestamp")
	}
	offer := RelayOffer{Version: 1, ChainID: identity.ChainID, PublicKey: pub, Nonce: uint64(nonce), Gas: WalletGas, GasFeeCap: &price, GasTipCap: &price, ValidUntil: uint64(head.Timestamp) + core.AntarticalStampSponsorshipLifetime}
	message, err := relayOfferMessage(&offer)
	if err != nil {
		return RelayOffer{}, err
	}
	offer.Signature, err = pqcrypto.SignMLDSA87(key, message[:])
	if err != nil {
		return RelayOffer{}, err
	}
	if err := validateRelayOffer(ctx, rpc, &offer, identity.ChainID); err != nil {
		return RelayOffer{}, err
	}
	return offer, nil
}
func relayPacket(tx *types.Transaction) (RelayPacket, error) {
	e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
	if err != nil {
		return RelayPacket{}, err
	}
	if !ok || !e.Relayed {
		return RelayPacket{}, errors.New("not a private relay spend")
	}
	ns, err := core.ShieldedV3Nullifiers(e)
	if err != nil {
		return RelayPacket{}, err
	}
	algorithm, pub, _, _ := tx.PQTkmFields()
	address, err := pqcrypto.Address(algorithm, pub)
	if err != nil {
		return RelayPacket{}, err
	}
	raw, err := tx.MarshalBinary()
	return RelayPacket{raw, tx.Hash(), address, e.GasSponsorValue.String(), e.ValidUntil, uint64(len(ns))}, err
}

// BuildRelayed creates an unsigned relay transaction. The public packet contains
// the relay key, never the payer's PQ key/address or spending/viewing secrets.
func BuildRelayed(ctx context.Context, rpc RPC, seed []byte, identity *Identity, to PaymentPayload, amount *big.Int, offer *RelayOffer) (*types.Transaction, error) {
	return build(ctx, rpc, seed, identity, to, amount, false, offer)
}
func RelayPacketForTransaction(tx *types.Transaction) (RelayPacket, error) { return relayPacket(tx) }

func BuildRelaySubmission(ctx context.Context, rpc RPC, seed []byte, identity *Identity, raw []byte) (*types.Transaction, error) {
	if identity == nil || uint64(len(raw)) > core.ShieldedV3MaxTxSize {
		return nil, errors.New("invalid relay transaction size")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, err
	}
	algorithm, pub, sig, ok := tx.PQTkmFields()
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	if !ok || algorithm != pqcrypto.AlgorithmMLDSA87 || len(sig) != 0 || !bytes.Equal(pub, pqcrypto.PublicKeyBytes(key)) || tx.ChainId().Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.Gas() != WalletGas || len(tx.AccessList()) != 0 || tx.GasFeeCap().Sign() <= 0 || tx.GasFeeCap().BitLen() > 256 || tx.GasTipCap().Sign() < 0 || tx.GasTipCap().Cmp(tx.GasFeeCap()) > 0 {
		return nil, errors.New("relay packet does not match this operator and bounded fee policy")
	}
	address, err := pqcrypto.Address(algorithm, pub)
	if err != nil || address != identity.Address {
		return nil, errors.New("wrong relay operator")
	}
	e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if !ok || e.Version != 3 || !e.Relayed || e.Deposit || e.WithdrawalValue == nil || e.WithdrawalValue.Sign() != 0 || e.WithdrawalRecipient != (common.Address{}) || e.GasSponsorValue == nil || e.GasSponsorValue.Cmp(new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())) != 0 {
		return nil, errors.New("relay must not release value except its authorized gas reserve")
	}
	seenOutputs := make(map[shielded3.Digest]bool)
	commitments := make([]shielded3.Digest, 0, len(e.Outputs))
	for _, out := range e.Outputs {
		if out.Commitment == (shielded3.Digest{}) || seenOutputs[out.Commitment] {
			return nil, errors.New("relay has zero or duplicate output commitment")
		}
		seenOutputs[out.Commitment] = true
		commitments = append(commitments, out.Commitment)
		for _, record := range []struct {
			data []byte
			role pqcrypto.ShieldedV3Purpose
		}{{out.Incoming, pqcrypto.ShieldedV3Incoming}, {out.Outgoing, pqcrypto.ShieldedV3Outgoing}, {out.Stamp, pqcrypto.ShieldedV3Stamp}} {
			if err := pqcrypto.ValidateShieldedV3CiphertextContext(record.data, core.ShieldedV3OutputContext(identity.ChainID, record.role, out.Commitment)); err != nil {
				return nil, err
			}
		}
	}
	var outputs []core.ShieldedV3Path
	if err := rpc.CallContext(ctx, &outputs, "tkmprivacy_shieldedV3Paths", commitments); err != nil {
		return nil, err
	}
	if len(outputs) != len(commitments) {
		return nil, errors.New("incomplete relay output reuse check")
	}
	for _, path := range outputs {
		if path.Found {
			return nil, errors.New("relay output commitment already exists")
		}
	}
	if err := relayState(ctx, rpc, address, tx.Nonce(), e.ValidUntil); err != nil {
		return nil, err
	}
	ns, err := core.ShieldedV3Nullifiers(e)
	if err != nil {
		return nil, err
	}
	for _, n := range ns {
		var state struct {
			TransactionHash common.Hash `json:"transactionHash"`
			Pending         bool        `json:"pending"`
		}
		if err := rpc.CallContext(ctx, &state, "tkmprivacy_shieldedV3NullifierStatus", n); err != nil {
			return nil, err
		}
		if state.Pending || state.TransactionHash != (common.Hash{}) {
			return nil, errors.New("relay input is already pending or spent")
		}
	}
	var known bool
	if err := rpc.CallContext(ctx, &known, "tkmprivacy_shieldedV3RootsKnown", e.Anchor, e.StampRoot); err != nil {
		return nil, err
	}
	if !known {
		return nil, errors.New("relay packet uses roots outside the canonical chain")
	}
	if err := core.ValidateShieldedV3Proof(&tx); err != nil {
		return nil, err
	}
	gas, err := core.IntrinsicGasWithShield3(tx.Data(), nil, nil, false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > tx.Gas() {
		return nil, errors.New("relay proof exceeds gas limit")
	}
	return &tx, nil
}
func ReviewRelaySubmission(ctx context.Context, rpc RPC, seed []byte, identity *Identity, raw []byte) (RelayPacket, error) {
	tx, err := BuildRelaySubmission(ctx, rpc, seed, identity, raw)
	if err != nil {
		return RelayPacket{}, err
	}
	return relayPacket(tx)
}

func ReviewRelayOffer(ctx context.Context, rpc RPC, offer *RelayOffer, chainID uint64) (RelayPacket, error) {
	if err := validateRelayOffer(ctx, rpc, offer, chainID); err != nil {
		return RelayPacket{}, err
	}
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, offer.PublicKey)
	if err != nil {
		return RelayPacket{}, err
	}
	return RelayPacket{Relay: address, GasReserveWei: new(big.Int).Mul(new(big.Int).SetUint64(offer.Gas), (*big.Int)(offer.GasFeeCap)).String(), ValidUntil: offer.ValidUntil}, nil
}
