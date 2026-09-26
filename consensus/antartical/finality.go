package antartical

import (
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrInvalidFinalityCertificate = errors.New("invalid Antartical finality certificate")

type FinalityCertificate struct {
	Slot          uint64
	BlockHash     common.Hash
	CommitteeSize uint64
	Signers       []common.Address
	PublicKeys    [][]byte
	Signatures    [][]byte
}

func (c FinalityCertificate) Verify(quorumNumerator, quorumDenominator uint64) error {
	committeeSize := c.CommitteeSize
	if committeeSize == 0 {
		committeeSize = uint64(len(c.Signers))
	}
	if c.BlockHash == (common.Hash{}) || len(c.Signers) == 0 || c.CommitteeSize > 0 && committeeSize < uint64(len(c.Signers)) || len(c.Signers) != len(c.PublicKeys) || len(c.Signers) != len(c.Signatures) || quorumDenominator == 0 {
		return ErrInvalidFinalityCertificate
	}
	seen := make(map[common.Address]bool)
	valid := uint64(0)
	for i, signer := range c.Signers {
		if signer == (common.Address{}) || seen[signer] || len(c.PublicKeys[i]) == 0 || len(c.Signatures[i]) != crypto.SignatureLength {
			return ErrInvalidFinalityCertificate
		}
		seen[signer] = true
		pub, err := crypto.UnmarshalPubkey(c.PublicKeys[i])
		if err != nil || crypto.PubkeyToAddress(*pub) != signer || !crypto.VerifySignature(c.PublicKeys[i], c.BlockHash.Bytes(), c.Signatures[i][:64]) {
			return ErrInvalidFinalityCertificate
		}
		valid++
	}
	if valid*quorumDenominator < committeeSize*quorumNumerator {
		return ErrInvalidFinalityCertificate
	}
	return nil
}

func SortedSigners(signers []common.Address) []common.Address {
	out := append([]common.Address(nil), signers...)
	sort.Slice(out, func(i, j int) bool { return out[i].Hex() < out[j].Hex() })
	return out
}
