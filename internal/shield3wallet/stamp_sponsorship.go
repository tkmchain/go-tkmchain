package shield3wallet

import (
	"bytes"
	"context"
	"encoding/base64"
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

// StampSponsorship is public exchange data. It never contains a spending seed,
// viewing key, stamp key, or plaintext labels. The fee payer confirms this exact
// transaction after the beneficiary adds its authorization and ownership proof.
type StampSponsorship struct {
	Transaction hexutil.Bytes  `json:"transaction"`
	Sponsor     common.Address `json:"sponsor"`
	Beneficiary common.Address `json:"beneficiary"`
	MaxFeeWei   string         `json:"maxFeeWei"`
	ValidUntil  hexutil.Uint64 `json:"validUntil"`
}

func stampSponsorshipPacket(tx *types.Transaction) (StampSponsorship, error) {
	e, err := core.DecodeAntarticalStamp(tx.Data())
	if err != nil {
		return StampSponsorship{}, err
	}
	algorithm, pub, _, _ := tx.PQTkmFields()
	sponsor, err := pqcrypto.Address(algorithm, pub)
	if err != nil {
		return StampSponsorship{}, err
	}
	beneficiary, err := core.AntarticalStampBeneficiary(sponsor, e)
	if err != nil {
		return StampSponsorship{}, err
	}
	raw, err := tx.MarshalBinary()
	return StampSponsorship{raw, sponsor, beneficiary, new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap()).String(), hexutil.Uint64(e.ValidUntil)}, err
}

func stampSponsorshipTransaction(data []byte, chainID uint64) (*types.Transaction, *core.AntarticalStampRegistration, common.Address, error) {
	fail := func() (*types.Transaction, *core.AntarticalStampRegistration, common.Address, error) {
		return nil, nil, common.Address{}, errors.New("invalid stamp sponsorship transaction")
	}
	if uint64(len(data)) > core.ShieldedV3MaxTxSize {
		return fail()
	}
	var tx types.Transaction
	if tx.UnmarshalBinary(data) != nil || tx.Type() != types.PQTkmTxType || tx.ChainId().Cmp(new(big.Int).SetUint64(chainID)) != 0 || tx.To() == nil || *tx.To() != params.ShieldedPoolAddress || tx.Value().Sign() != 0 || tx.Gas() != StampWalletGas || tx.GasFeeCap().Sign() <= 0 || tx.GasTipCap().Sign() < 0 || tx.GasTipCap().Cmp(tx.GasFeeCap()) > 0 || len(tx.AccessList()) != 0 {
		return fail()
	}
	algorithm, pub, sig, ok := tx.PQTkmFields()
	if algorithm != pqcrypto.AlgorithmMLDSA87 || len(sig) != 0 || !ok {
		return fail()
	}
	sponsor, err := pqcrypto.Address(algorithm, pub)
	if err != nil {
		return fail()
	}
	e, err := core.DecodeAntarticalStamp(tx.Data())
	if err != nil || e.Version != 1 || e.Stamp.ChainID != chainID || len(e.BeneficiaryPublicKey) != pqcrypto.MLDSA87PublicKeySize || !pqcrypto.VerifyShieldedV3Stamp(e.BeneficiaryPublicKey, &e.Stamp) {
		return fail()
	}
	beneficiary, err := core.AntarticalStampBeneficiary(sponsor, e)
	if err != nil || beneficiary == sponsor {
		return fail()
	}
	return &tx, e, sponsor, nil
}

func sponsorshipState(ctx context.Context, rpc RPC, tx *types.Transaction, e *core.AntarticalStampRegistration, sponsor common.Address) error {
	var active status
	if err := rpc.CallContext(ctx, &active, "tkmprivacy_shieldedV3Status"); err != nil {
		return err
	}
	if !active.Active || !active.NativeVerifier {
		return errors.New("stamp sponsorship activates at Antartical")
	}
	var head header
	if err := rpc.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return err
	}
	if e.ValidUntil <= uint64(head.Timestamp) || e.ValidUntil-uint64(head.Timestamp) > core.AntarticalStampSponsorshipLifetime {
		return errors.New("stamp sponsorship must expire within one hour of the current chain head")
	}
	var stamp core.AntarticalStampStatus
	if err := rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStamp", sponsor); err != nil {
		return err
	}
	if !stamp.Registered {
		return errors.New("stamp sponsor must have a confirmed stamp")
	}
	beneficiary, err := core.AntarticalStampBeneficiary(sponsor, e)
	if err != nil {
		return err
	}
	if err := rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStamp", beneficiary); err != nil {
		return err
	}
	if stamp.Registered {
		return errors.New("beneficiary stamp is already confirmed")
	}
	var nonce hexutil.Uint64
	if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", sponsor, "pending"); err != nil {
		return err
	}
	if tx.Nonce() != uint64(nonce) {
		return errors.New("sponsor nonce changed; create a new fee offer")
	}
	var balance hexutil.Big
	if err := rpc.CallContext(ctx, &balance, "eth_getBalance", sponsor, "latest"); err != nil {
		return err
	}
	if (*big.Int)(&balance).Cmp(new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas()), tx.GasFeeCap())) < 0 {
		return errors.New("sponsor public balance cannot cover the authorized maximum fee")
	}
	return nil
}

// BuildStampSponsorshipOffer creates an unsigned, expiring fee offer for an
// authenticated receiving code. It neither authorizes nor broadcasts payment.
func BuildStampSponsorshipOffer(ctx context.Context, rpc RPC, seed []byte, identity *Identity, recipientCode string) (StampSponsorship, error) {
	if identity == nil {
		return StampSponsorship{}, errors.New("missing sponsor identity")
	}
	recipient, err := DecodePaymentCode(recipientCode, identity.ChainID)
	if err != nil {
		return StampSponsorship{}, err
	}
	// DecodePaymentCode already checked the public code's canonical signature.
	raw, err := base64.RawURLEncoding.Strict().DecodeString(recipientCode[len("tkmshield3."):])
	if err != nil {
		return StampSponsorship{}, err
	}
	var code PaymentCode
	if err := rlp.DecodeBytes(raw, &code); err != nil {
		return StampSponsorship{}, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return StampSponsorship{}, err
	}
	pub := pqcrypto.PublicKeyBytes(key)
	address, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pub)
	if err != nil || address != identity.Address {
		return StampSponsorship{}, errors.New("sponsor seed does not match account")
	}
	var nonce hexutil.Uint64
	var price hexutil.Big
	var head header
	if err := rpc.CallContext(ctx, &nonce, "eth_getTransactionCount", identity.Address, "pending"); err != nil {
		return StampSponsorship{}, err
	}
	if err := rpc.CallContext(ctx, &price, "eth_gasPrice"); err != nil {
		return StampSponsorship{}, err
	}
	if err := rpc.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return StampSponsorship{}, err
	}
	if uint64(head.Timestamp) > ^uint64(0)-core.AntarticalStampSponsorshipLifetime {
		return StampSponsorship{}, errors.New("invalid chain timestamp")
	}
	e := &core.AntarticalStampRegistration{Version: 1, Owner: recipient.Owner, Stamp: recipient.Stamp, BeneficiaryPublicKey: code.PublicKey, ValidUntil: uint64(head.Timestamp) + core.AntarticalStampSponsorshipLifetime}
	data, err := core.EncodeAntarticalStamp(e)
	if err != nil {
		return StampSponsorship{}, err
	}
	tx := types.NewTx(&types.PQTkmTx{ChainID: new(big.Int).SetUint64(identity.ChainID), Nonce: uint64(nonce), GasFeeCap: (*big.Int)(&price), GasTipCap: (*big.Int)(&price), Gas: StampWalletGas, To: &params.ShieldedPoolAddress, Value: new(big.Int), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pub, Data: data})
	packet, err := stampSponsorshipPacket(tx)
	if err != nil {
		return StampSponsorship{}, err
	}
	checked, envelope, sponsor, err := stampSponsorshipTransaction(packet.Transaction, identity.ChainID)
	if err != nil {
		return StampSponsorship{}, err
	}
	if err := sponsorshipState(ctx, rpc, checked, envelope, sponsor); err != nil {
		return StampSponsorship{}, err
	}
	return packet, nil
}

// AuthorizeStampSponsorship binds the beneficiary's original encrypted stamp
// and secret owner to the exact sponsor, nonce, fee limits, chain, and expiry.
func AuthorizeStampSponsorship(ctx context.Context, rpc RPC, seed []byte, identity *Identity, data []byte) (StampSponsorship, error) {
	if identity == nil || identity.Stamp == nil {
		return StampSponsorship{}, errors.New("create the private stamp first")
	}
	tx, e, sponsor, err := stampSponsorshipTransaction(data, identity.ChainID)
	if err != nil {
		return StampSponsorship{}, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return StampSponsorship{}, err
	}
	ownStamp, err := rlp.EncodeToBytes(identity.Stamp)
	if err != nil {
		return StampSponsorship{}, err
	}
	offerStamp, err := rlp.EncodeToBytes(&e.Stamp)
	if err != nil {
		return StampSponsorship{}, err
	}
	if !bytes.Equal(pqcrypto.PublicKeyBytes(key), e.BeneficiaryPublicKey) || e.Owner != identity.Owner || !bytes.Equal(ownStamp, offerStamp) {
		return StampSponsorship{}, errors.New("fee offer does not register this wallet's original stamp")
	}
	if len(e.Proof) != 0 || len(e.Authorization) != 0 {
		return StampSponsorship{}, errors.New("fee offer already contains authorization; use the saved authorized packet")
	}
	if err := sponsorshipState(ctx, rpc, tx, e, sponsor); err != nil {
		return StampSponsorship{}, err
	}
	select {
	case buildSlot <- struct{}{}:
		defer func() { <-buildSlot }()
	case <-ctx.Done():
		return StampSponsorship{}, ctx.Err()
	}
	intent, err := core.AntarticalStampIntent(tx, e)
	if err != nil {
		return StampSponsorship{}, err
	}
	e.Proof, err = (shielded3.NativeBackend{}).ProveOwner(ctx, identity.ChainID, identity.Owner, intent, identity.SpendingSecret)
	if err != nil {
		return StampSponsorship{}, err
	}
	e.Authorization, err = pqcrypto.SignMLDSA87(key, intent[:])
	if err != nil {
		return StampSponsorship{}, err
	}
	tx, err = replaceStampSponsorshipEnvelope(tx, e)
	if err != nil {
		return StampSponsorship{}, err
	}
	if err := validateCompletedSponsorship(ctx, rpc, tx, e, sponsor); err != nil {
		return StampSponsorship{}, err
	}
	return stampSponsorshipPacket(tx)
}

func replaceStampSponsorshipEnvelope(tx *types.Transaction, e *core.AntarticalStampRegistration) (*types.Transaction, error) {
	data, err := core.EncodeAntarticalStamp(e)
	if err != nil {
		return nil, err
	}
	algorithm, pub, _, _ := tx.PQTkmFields()
	return types.NewTx(&types.PQTkmTx{ChainID: tx.ChainId(), Nonce: tx.Nonce(), GasFeeCap: tx.GasFeeCap(), GasTipCap: tx.GasTipCap(), Gas: tx.Gas(), To: tx.To(), Value: tx.Value(), Algorithm: algorithm, PublicKey: pub, Data: data}), nil
}

func validateCompletedSponsorship(ctx context.Context, rpc RPC, tx *types.Transaction, e *core.AntarticalStampRegistration, sponsor common.Address) error {
	if err := sponsorshipState(ctx, rpc, tx, e, sponsor); err != nil {
		return err
	}
	intent, err := core.AntarticalStampIntent(tx, e)
	if err != nil || !pqcrypto.VerifyMLDSA87(e.BeneficiaryPublicKey, intent[:], e.Authorization) {
		return errors.New("invalid beneficiary stamp sponsorship authorization")
	}
	if err := core.ValidateAntarticalStampProof(tx); err != nil {
		return err
	}
	gas, err := core.IntrinsicGasWithShield3(tx.Data(), nil, nil, false, true, true, true, false, true)
	if err != nil {
		return err
	}
	if gas.RegularGas > tx.Gas() {
		return errors.New("stamp proof exceeds the sponsor's authorized gas limit")
	}
	return nil
}

// BuildSponsoredStamp verifies an authorized packet for the selected sponsor.
// Callers must explicitly approve the shown maximum fee before signing it.
func BuildSponsoredStamp(ctx context.Context, rpc RPC, seed []byte, identity *Identity, data []byte) (*types.Transaction, error) {
	if identity == nil {
		return nil, errors.New("missing sponsor identity")
	}
	tx, e, sponsor, err := stampSponsorshipTransaction(data, identity.ChainID)
	if err != nil {
		return nil, err
	}
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	_, pub, _, _ := tx.PQTkmFields()
	if sponsor != identity.Address || !bytes.Equal(pub, pqcrypto.PublicKeyBytes(key)) {
		return nil, errors.New("authorized stamp packet belongs to another sponsor")
	}
	if err := validateCompletedSponsorship(ctx, rpc, tx, e, sponsor); err != nil {
		return nil, err
	}
	return tx, nil
}

// ReviewStampSponsorship returns metadata derived from the verified transaction,
// never from untrusted JSON display fields supplied by the other party.
func ReviewStampSponsorship(ctx context.Context, rpc RPC, seed []byte, identity *Identity, data []byte) (StampSponsorship, error) {
	tx, err := BuildSponsoredStamp(ctx, rpc, seed, identity, data)
	if err != nil {
		return StampSponsorship{}, err
	}
	return stampSponsorshipPacket(tx)
}
