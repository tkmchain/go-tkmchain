// Package shielded4 exposes the production Shield4 relation. Shield4 uses a
// separate native Triton claim program while retaining the canonical Tip5
// digest and witness types, allowing the full-chain tree to span the V3 to V4
// transition without converting or revealing existing notes.
package shielded4

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/zk/shielded3"
)

const (
	MerkleDepth  = shielded3.MerkleDepth
	OutputSlots  = shielded3.OutputSlots
	InputSlots   = shielded3.InputSlots
	AssetTKM     = shielded3.AssetTKM
	MaxSendTKM   = shielded3.MaxSendTKM
	MaxProofSize = shielded3.MaxProofSize
)

type Digest = shielded3.Digest
type Amount = shielded3.Amount
type Statement = shielded3.StatementV4
type OutputOpening = shielded3.OutputOpening
type InputOpening = shielded3.InputOpening
type SpendWitness = shielded3.SpendWitness
type DerivedSpend = shielded3.DerivedSpend

var (
	ErrInvalidStatement   = shielded3.ErrInvalidStatement
	ErrInvalidWitness     = shielded3.ErrInvalidWitness
	ErrInvalidProof       = shielded3.ErrInvalidProof
	ErrBackendUnavailable = shielded3.ErrBackendUnavailable
)

func MaxSendWei() *big.Int                     { return shielded3.MaxSendWei() }
func AmountFromBig(v *big.Int) (Amount, error) { return shielded3.AmountFromBig(v) }
func GenerateSecret() (Digest, error)          { return shielded3.GenerateSecret() }
func DigestFromBytes(b []byte) (Digest, error) { return shielded3.DigestFromBytes(b) }
func ValidProofEncoding(proof []byte) bool     { return shielded3.ValidProofEncoding(proof) }

// NativeBackend is deliberately a distinct type so callers cannot silently
// fall back to the Shield3 verifier when constructing or checking Shield4.
type NativeBackend struct{}

func NativeAvailable() bool { return shielded3.NativeAvailable() }
func (NativeBackend) Verify(ctx context.Context, s Statement, proof []byte) error {
	return (shielded3.NativeBackendV4{}).Verify(ctx, s, proof)
}
func (NativeBackend) Prove(ctx context.Context, s Statement, w SpendWitness) ([]byte, error) {
	return (shielded3.NativeBackendV4{}).Prove(ctx, s, w)
}
func (NativeBackend) Describe(ctx context.Context, s Statement, w SpendWitness) (DerivedSpend, error) {
	return (shielded3.NativeBackendV4{}).Describe(ctx, s, w)
}
