package core

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

// Preflight list lengths without allocating decoded slices. A byte-size cap
// alone does not bound [][]byte allocations: a list of empty strings can
// otherwise expand into millions of slice headers before semantic validation.
func shieldedV6List(raw []byte, max int) ([]rlp.RawValue, error) {
	content, rest, err := rlp.SplitList(raw)
	if err != nil || len(rest) != 0 {
		return nil, ErrInvalidShieldedTx
	}
	parts := make([]rlp.RawValue, 0, max)
	for len(content) > 0 {
		if len(parts) == max {
			return nil, ErrInvalidShieldedTx
		}
		_, _, rest, err := rlp.Split(content)
		if err != nil {
			return nil, err
		}
		parts = append(parts, content[:len(content)-len(rest)])
		content = rest
	}
	return parts, nil
}

func shieldedV6String(raw []byte, max int) error {
	content, rest, err := rlp.SplitString(raw)
	if err != nil || len(rest) != 0 || len(content) > max {
		return ErrInvalidShieldedTx
	}
	return nil
}

func shieldedV6Preflight(raw []byte) error {
	fields, err := shieldedV6List(raw, 12)
	if err != nil || len(fields) < 9 {
		return ErrInvalidShieldedTx
	}
	if _, err := shieldedV6List(fields[6], ShieldedV6MaxInputs); err != nil {
		return err
	}
	outputs, err := shieldedV6List(fields[7], ShieldedV6MaxOutputs)
	if err != nil {
		return err
	}
	for _, output := range outputs {
		parts, err := shieldedV6List(output, 5)
		if err != nil || len(parts) != 5 {
			return ErrInvalidShieldedTx
		}
		if err := shieldedV6String(parts[4], ShieldedV6MaxCiphertext); err != nil {
			return err
		}
	}
	if err := shieldedV6String(fields[8], shielded6.MaxRangeProofBundleSize); err != nil {
		return err
	}
	if len(fields) > 9 {
		bundle, err := shieldedV6List(fields[9], 2)
		if err != nil {
			return err
		}
		if len(bundle) != 0 {
			if len(bundle) != 2 {
				return ErrInvalidShieldedTx
			}
			if _, err := shieldedV6List(bundle[0], ShieldedV6MaxOutputs); err != nil {
				return err
			}
			proofs, err := shieldedV6List(bundle[1], ShieldedV6MaxOutputs)
			if err != nil {
				return err
			}
			for _, proof := range proofs {
				if err := shieldedV6String(proof, ShieldedV6MaxRecipientProof); err != nil {
					return err
				}
			}
		}
	}
	if len(fields) > 10 {
		binding, err := shieldedV6List(fields[10], 3)
		if err != nil {
			return err
		}
		if len(binding) != 0 {
			if len(binding) != 3 {
				return ErrInvalidShieldedTx
			}
			if err := shieldedV6String(binding[2], ShieldedV6MaxRecipientProof); err != nil {
				return err
			}
		}
	}
	return nil
}
