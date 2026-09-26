package antartical

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
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
