package shield3wallet

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

// PaymentDisclosure authorizes inspection of one payment output, never a wallet.
// Its note opening can also link that output's eventual nullifier.
type PaymentDisclosure struct {
	Version         uint64        `json:"version"`
	ChainID         uint64        `json:"chainId"`
	TransactionHash common.Hash   `json:"transactionHash"`
	OutputIndex     uint64        `json:"outputIndex"`
	RecordKey       hexutil.Bytes `json:"recordKey"`
}
type DisclosureCapsule struct {
	Version    uint64        `json:"version"`
	ChainID    uint64        `json:"chainId"`
	Commitment [64]byte      `json:"commitment"`
	Ciphertext hexutil.Bytes `json:"ciphertext"`
}
type DisclosedPayment struct {
	TransactionHash common.Hash    `json:"transactionHash"`
	BlockHash       common.Hash    `json:"blockHash"`
	BlockNumber     hexutil.Uint64 `json:"blockNumber"`
	OutputIndex     uint64         `json:"outputIndex"`
	Recipient       common.Address `json:"recipient"`
	AmountWei       string         `json:"amountWei"`
}

func disclosureTransaction(ctx context.Context, rpc RPC, d PaymentDisclosure) (*types.Transaction, *core.ShieldedV3Transaction, error) {
	if d.Version != 1 || d.ChainID == 0 || d.TransactionHash == (common.Hash{}) || d.OutputIndex >= 3 {
		return nil, nil, errors.New("invalid payment disclosure; change and decoys cannot be disclosed")
	}
	var raw hexutil.Bytes
	if err := rpc.CallContext(ctx, &raw, "eth_getRawTransactionByHash", d.TransactionHash); err != nil {
		return nil, nil, err
	}
	if uint64(len(raw)) > core.ShieldedV3MaxTxSize {
		return nil, nil, errors.New("disclosure transaction exceeds consensus size")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, nil, err
	}
	if tx.Hash() != d.TransactionHash || tx.ChainId().Cmp(new(big.Int).SetUint64(d.ChainID)) != 0 {
		return nil, nil, errors.New("disclosure transaction or chain mismatch")
	}
	e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, errors.New("not a Shield3 transaction")
	}
	return &tx, e, nil
}
func VerifyPaymentDisclosure(ctx context.Context, rpc RPC, d PaymentDisclosure) (DisclosedPayment, error) {
	var result DisclosedPayment
	_, e, err := disclosureTransaction(ctx, rpc, d)
	if err != nil {
		return result, err
	}
	var receipt *struct {
		TransactionHash common.Hash    `json:"transactionHash"`
		BlockHash       common.Hash    `json:"blockHash"`
		BlockNumber     hexutil.Uint64 `json:"blockNumber"`
		Status          hexutil.Uint64 `json:"status"`
	}
	if err = rpc.CallContext(ctx, &receipt, "eth_getTransactionReceipt", d.TransactionHash); err != nil {
		return result, err
	}
	if receipt == nil || receipt.Status != 1 || receipt.TransactionHash != d.TransactionHash || receipt.BlockHash == (common.Hash{}) {
		return result, errors.New("payment is not successfully confirmed")
	}
	var canonical header
	if err = rpc.CallContext(ctx, &canonical, "eth_getBlockByNumber", hexutil.EncodeUint64(uint64(receipt.BlockNumber)), false); err != nil {
		return result, err
	}
	if canonical.Hash != receipt.BlockHash {
		return result, errors.New("payment receipt is outside the canonical chain")
	}
	out := e.Outputs[d.OutputIndex]
	plain, err := pqcrypto.OpenShieldedV3RecordKey(d.RecordKey, out.Outgoing, core.ShieldedV3OutputContext(d.ChainID, pqcrypto.ShieldedV3Outgoing, out.Commitment))
	if err != nil {
		return result, err
	}
	defer clear(plain)
	var note Note
	if err = json.Unmarshal(plain, &note); err != nil {
		return result, errors.New("invalid disclosed note")
	}
	amount, err := parseAmount(note.ValueWei)
	if err != nil || amount.Sign() <= 0 {
		return result, errors.New("decoy output is not a payment")
	}
	commitment, err := NoteCommitment(d.ChainID, note)
	if err != nil {
		return result, err
	}
	if commitment != out.Commitment {
		return result, errors.New("disclosed note does not match the on-chain commitment")
	}
	var stamp core.AntarticalStampStatus
	if err = rpc.CallContext(ctx, &stamp, "tkmprivacy_antarticalStamp", note.Recipient); err != nil {
		return result, err
	}
	if !stamp.Registered || stamp.Owner != note.Owner {
		return result, errors.New("disclosed recipient does not own the registered note identity")
	}
	if err = rpc.CallContext(ctx, &canonical, "eth_getBlockByNumber", hexutil.EncodeUint64(uint64(receipt.BlockNumber)), false); err != nil {
		return result, err
	}
	if canonical.Hash != receipt.BlockHash {
		return result, errors.New("chain reorganized during payment disclosure")
	}
	return DisclosedPayment{d.TransactionHash, receipt.BlockHash, receipt.BlockNumber, d.OutputIndex, note.Recipient, note.ValueWei}, nil
}
func ExportPaymentDisclosure(ctx context.Context, rpc RPC, identity *Identity, hash common.Hash, slot uint64) (PaymentDisclosure, error) {
	if identity == nil {
		return PaymentDisclosure{}, errors.New("missing wallet identity")
	}
	d := PaymentDisclosure{Version: 1, ChainID: identity.ChainID, TransactionHash: hash, OutputIndex: slot}
	_, e, err := disclosureTransaction(ctx, rpc, d)
	if err != nil {
		return d, err
	}
	out := e.Outputs[slot]
	d.RecordKey, err = pqcrypto.ShieldedV3RecordKey(identity.OutgoingSeed, out.Outgoing, core.ShieldedV3OutputContext(d.ChainID, pqcrypto.ShieldedV3Outgoing, out.Commitment))
	if err != nil {
		return d, err
	}
	if _, err = VerifyPaymentDisclosure(ctx, rpc, d); err != nil {
		clear(d.RecordKey)
		return PaymentDisclosure{}, err
	}
	return d, nil
}
func SealPaymentDisclosure(d PaymentDisclosure, publicKey []byte) (DisclosureCapsule, error) {
	if d.Version != 1 || d.ChainID == 0 || d.OutputIndex >= 3 || len(d.RecordKey) != 32 || d.TransactionHash == (common.Hash{}) {
		return DisclosureCapsule{}, errors.New("invalid disclosure")
	}
	c := DisclosureCapsule{Version: 1, ChainID: d.ChainID}
	if _, err := rand.Read(c.Commitment[:]); err != nil {
		return c, err
	}
	plain, err := json.Marshal(d)
	if err != nil {
		return c, err
	}
	defer clear(plain)
	c.Ciphertext, err = pqcrypto.SealShieldedV3(publicKey, plain, pqcrypto.ShieldedV3Context{ChainID: c.ChainID, Purpose: pqcrypto.ShieldedV3Disclosure, Commitment: c.Commitment})
	return c, err
}
func OpenPaymentDisclosure(c DisclosureCapsule, seed []byte) (PaymentDisclosure, error) {
	var d PaymentDisclosure
	if c.Version != 1 {
		return d, errors.New("unsupported disclosure capsule")
	}
	plain, err := pqcrypto.OpenShieldedV3(seed, c.Ciphertext, pqcrypto.ShieldedV3Context{ChainID: c.ChainID, Purpose: pqcrypto.ShieldedV3Disclosure, Commitment: c.Commitment})
	if err != nil {
		return d, err
	}
	defer clear(plain)
	if err = json.Unmarshal(plain, &d); err != nil {
		return d, err
	}
	if d.Version != 1 || d.ChainID != c.ChainID || d.OutputIndex >= 3 || len(d.RecordKey) != 32 || d.TransactionHash == (common.Hash{}) {
		clear(d.RecordKey)
		return PaymentDisclosure{}, errors.New("invalid disclosure capsule payload")
	}
	return d, nil
}
