package recipient

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded3"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

const PaymentCodePrefix = "tkmshield6r."
const MaxPaymentCodeBytes = 8192

// PaymentCode is shared privately with a payer. It contains public keys and
// membership witnesses, never spend/view secrets. Roots must additionally be
// authenticated against canonical chain state before use.
type PaymentCode struct {
	Version                uint64
	Address                shielded6.Address
	Owner                  shielded3.Digest
	StampRoot, BindingRoot shielded3.Digest
	StampIndex             uint32
	StampPath              [MerkleDepth]shielded3.Digest
	BindingIndex           uint32
	BindingPath            [MerkleDepth]shielded3.Digest
}

func (p PaymentCode) Witness() Witness {
	return Witness{Owner: p.Owner, Recipient: p.Address, StampIndex: p.StampIndex, StampPath: p.StampPath, BindingIndex: p.BindingIndex, BindingPath: p.BindingPath}
}

func (p PaymentCode) Validate(chainID uint64) error {
	invalid := errors.New("invalid recipient-stamped Shield6 payment code")
	if p.Version != RelationVersion || chainID == 0 || p.Address.ChainID != chainID || p.Address.Validate() != nil || !validDigest(p.Owner) || p.Owner == (shielded3.Digest{}) || !validDigest(p.StampRoot) || !validDigest(p.BindingRoot) {
		return invalid
	}
	stamp, err := shielded3.StampLeaf(chainID, p.Owner)
	if err != nil {
		return err
	}
	binding, err := BindingLeaf(chainID, p.Owner, p.Address)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		leaf, root shielded3.Digest
		index      uint32
		path       [MerkleDepth]shielded3.Digest
	}{{stamp, p.StampRoot, p.StampIndex, p.StampPath}, {binding, p.BindingRoot, p.BindingIndex, p.BindingPath}} {
		current := item.leaf
		for level, sibling := range item.path {
			if !validDigest(sibling) {
				return invalid
			}
			if item.index>>level&1 == 0 {
				current, err = shielded3.HashPair(current, sibling)
			} else {
				current, err = shielded3.HashPair(sibling, current)
			}
			if err != nil {
				return err
			}
		}
		if current != item.root {
			return invalid
		}
	}
	return nil
}

func EncodePaymentCode(p PaymentCode) (string, error) {
	if err := p.Validate(p.Address.ChainID); err != nil {
		return "", err
	}
	raw, err := rlp.EncodeToBytes(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(PaymentCodePrefix), raw...))
	encoded := PaymentCodePrefix + base64.RawURLEncoding.EncodeToString(append(raw, sum[:8]...))
	if len(encoded) > MaxPaymentCodeBytes {
		return "", errors.New("recipient payment code exceeds limit")
	}
	return encoded, nil
}

func DecodePaymentCode(code string, chainID uint64) (PaymentCode, error) {
	var p PaymentCode
	invalid := errors.New("invalid recipient-stamped Shield6 payment code")
	if len(code) > MaxPaymentCodeBytes || !strings.HasPrefix(code, PaymentCodePrefix) {
		return p, invalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(code, PaymentCodePrefix))
	if err != nil || len(raw) < 8 {
		return p, invalid
	}
	payload, check := raw[:len(raw)-8], raw[len(raw)-8:]
	sum := sha256.Sum256(append([]byte(PaymentCodePrefix), payload...))
	if !bytes.Equal(check, sum[:8]) || rlp.DecodeBytes(payload, &p) != nil {
		return PaymentCode{}, invalid
	}
	if err := p.Validate(chainID); err != nil {
		return PaymentCode{}, err
	}
	return p, nil
}
