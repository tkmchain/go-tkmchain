package shield3wallet

import (
	"bytes"
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

// BuildAndSignShieldedV6Binding authorizes this wallet's private key commitment
// using the already-stamped PQ account. It publishes no Shield6 static key.
func BuildAndSignShieldedV6Binding(ctx context.Context, rpc RPC, seed []byte, identity *Identity, keys *shielded6.WalletKeys, skeleton *types.Transaction, registrationProvers ...*recipient.PreparedProver) (*types.Transaction, error) {
	if ctx == nil || rpc == nil || identity == nil || keys == nil || skeleton == nil || identity.ChainID == 0 || identity.ChainID != keys.ChainID || skeleton.Type() != types.PQTkmTxType || skeleton.ChainId().Cmp(new(big.Int).SetUint64(identity.ChainID)) != 0 || skeleton.To() == nil || *skeleton.To() != params.ShieldedPoolAddress || skeleton.Value().Sign() != 0 || len(skeleton.Data()) != 0 || skeleton.Gas() == 0 {
		return nil, errors.New("invalid Shield6 recipient registration request")
	}
	algorithm, publicKey, signature, ok := skeleton.PQTkmFields()
	if !ok || algorithm != pqcrypto.AlgorithmMLDSA87 || len(signature) != 0 {
		return nil, errors.New("registration requires an unsigned ML-DSA account skeleton")
	}
	pqKey, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		return nil, err
	}
	address, err := pqcrypto.Address(algorithm, pqcrypto.PublicKeyBytes(pqKey))
	if err != nil || address != identity.Address || !bytes.Equal(publicKey, pqcrypto.PublicKeyBytes(pqKey)) {
		return nil, errors.New("registration account seed mismatch")
	}
	derived, err := shielded6.DeriveWalletKeys(seed, identity.ChainID)
	if err != nil {
		return nil, err
	}
	defer derived.Clear()
	if derived.Address() != keys.Address() {
		return nil, errors.New("registration recipient keys mismatch")
	}
	verified, err := NewIdentity(seed, identity.ChainID, identity.Stamp)
	if err != nil {
		return nil, err
	}
	defer verified.Clear()
	if verified.Owner != identity.Owner {
		return nil, errors.New("registration stamp owner mismatch")
	}
	payload, err := verified.RegisteredStampPayload()
	if err != nil {
		return nil, err
	}
	if err := RequireRegisteredStamp(ctx, rpc, payload); err != nil {
		return nil, err
	}
	var status shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &status, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	if !status.Active || !status.RecipientProofsRequired || status.ChainID == nil || (*big.Int)(status.ChainID).Cmp(skeleton.ChainId()) != 0 {
		return nil, errors.New("Shield6 recipient registration is not active")
	}
	if uint64(status.HeadTime) > ^uint64(0)-core.ShieldedV6MaxLifetime {
		return nil, errors.New("invalid canonical head time")
	}
	commitment, err := recipient.BindingCommitment(identity.ChainID, identity.Owner, keys.Address())
	if err != nil {
		return nil, err
	}
	if len(registrationProvers) != 1 || registrationProvers[0] == nil || (status.RegistrationProvingKeyHash == (common.Hash{}) || common.Hash(registrationProvers[0].Pins().ProvingKey) != status.RegistrationProvingKeyHash || common.Hash(registrationProvers[0].Pins().VerifyingKey) != status.RegistrationKeyHash) {
		return nil, errors.New("authenticated registration proving parameters matching this chain are required")
	}
	var path core.ShieldedV3Path
	if err := rpc.CallContext(ctx, &path, "tkmprivacy_shieldedV6BindingPath", identity.Owner); err != nil {
		return nil, err
	}
	if path.Found {
		return nil, errors.New("Shield6 recipient keys are already registered")
	}
	envelope := &core.ShieldedV6Transaction{Version: core.ShieldedV6BindingVersion, ValidUntil: uint64(status.HeadTime) + core.ShieldedV6MaxLifetime, AssetID: shielded6.AssetTKM, Binding: &core.ShieldedV6BindingRegistration{Owner: identity.Owner, Commitment: commitment}}
	statement, err := core.ShieldedV6RegistrationStatement(skeleton, envelope)
	if err != nil {
		return nil, err
	}
	witness := recipient.RegistrationWitness{SpendSecret: keys.SpendSecret, ViewSecret: keys.ViewSecret}
	proof, err := registrationProvers[0].ProveRegistration(ctx, statement, witness)
	witness.Clear()
	if err != nil {
		return nil, err
	}
	envelope.Binding.Proof = proof
	data, err := core.EncodeShieldedV6Transaction(envelope)
	if err != nil {
		return nil, err
	}
	if err := RequireRegisteredStamp(ctx, rpc, payload); err != nil {
		return nil, err
	}

	var current shieldedV6RPCStatus
	if err := rpc.CallContext(ctx, &current, "tkmprivacy_shieldedV6Status"); err != nil {
		return nil, err
	}
	if !current.Active || !current.RecipientProofsRequired || current.RegistrationKeyHash != status.RegistrationKeyHash || uint64(current.HeadTime) >= envelope.ValidUntil {
		return nil, errors.New("registration context changed or expired during proving")
	}
	unsigned := types.NewTx(&types.PQTkmTx{ChainID: skeleton.ChainId(), Nonce: skeleton.Nonce(), GasTipCap: skeleton.GasTipCap(), GasFeeCap: skeleton.GasFeeCap(), Gas: skeleton.Gas(), To: skeleton.To(), Value: skeleton.Value(), Data: data, AccessList: skeleton.AccessList(), Algorithm: algorithm, PublicKey: publicKey})
	gas, err := core.IntrinsicGasWithShield3(data, unsigned.AccessList(), unsigned.SetCodeAuthorizations(), false, true, true, true, false, true)
	if err != nil {
		return nil, err
	}
	if gas.RegularGas > unsigned.Gas() {
		return nil, errors.New("recipient registration exceeds authorized gas limit")
	}
	return types.SignPQTkmTx(unsigned, types.NewQuantumSigner(skeleton.ChainId()), pqKey)
}
