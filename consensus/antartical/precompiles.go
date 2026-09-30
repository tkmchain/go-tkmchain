package antartical

import (
	"bytes"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrDuplicatePrecompile = errors.New("duplicate Antartical precompile")

type PrecompileModule struct {
	Address common.Address
	Name    string
	Gas     uint64
	Run     func(input []byte, value *big.Int) ([]byte, error)
}

type PrecompileRegistry struct {
	modules map[common.Address]PrecompileModule
}

func NewPrecompileRegistry() *PrecompileRegistry {
	return &PrecompileRegistry{modules: make(map[common.Address]PrecompileModule)}
}

func (r *PrecompileRegistry) Register(module PrecompileModule) error {
	if r == nil || module.Address == (common.Address{}) || module.Name == "" || module.Run == nil {
		return ErrDuplicatePrecompile
	}
	if r.modules == nil {
		r.modules = make(map[common.Address]PrecompileModule)
	}
	if _, exists := r.modules[module.Address]; exists {
		return ErrDuplicatePrecompile
	}
	r.modules[module.Address] = module
	return nil
}

func (r *PrecompileRegistry) Lookup(address common.Address) (PrecompileModule, bool) {
	if r == nil {
		return PrecompileModule{}, false
	}
	m, ok := r.modules[address]
	return m, ok
}

// Commitment returns the canonical registry root.  Registry order is part of
// the commitment, so two nodes cannot assign different code or gas to the same
// precompile address while advertising the same Antartical header.
func (r *PrecompileRegistry) Commitment() (common.Hash, error) {
	if r == nil {
		return common.Hash{}, ErrDuplicatePrecompile
	}
	addresses := make([]common.Address, 0, len(r.modules))
	for address := range r.modules {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return bytes.Compare(addresses[i][:], addresses[j][:]) < 0 })
	leaves := make([]common.Hash, 0, len(addresses))
	for _, address := range addresses {
		module := r.modules[address]
		if module.Address != address || module.Name == "" || module.Run == nil {
			return common.Hash{}, ErrDuplicatePrecompile
		}
		leaf, err := rlp.EncodeToBytes([]interface{}{[]byte("TKM_PRECOMPILE_V1"), address, module.Name, module.Gas})
		if err != nil {
			return common.Hash{}, err
		}
		leaves = append(leaves, crypto.Keccak256Hash(leaf))
	}
	if len(leaves) == 0 {
		return EmptyCommitment("precompile-registry"), nil
	}
	for len(leaves) > 1 {
		next := make([]common.Hash, 0, (len(leaves)+1)/2)
		for i := 0; i < len(leaves); i += 2 {
			right := leaves[i]
			if i+1 < len(leaves) {
				right = leaves[i+1]
			}
			next = append(next, crypto.Keccak256Hash([]byte("TKM_PRECOMPILE_NODE_V1"), leaves[i][:], right[:]))
		}
		leaves = next
	}
	return crypto.Keccak256Hash([]byte("TKM_PRECOMPILE_ROOT_V1"), leaves[0][:]), nil
}
