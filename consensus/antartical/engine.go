package antartical

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
)

var ErrExecutionEngineMismatch = errors.New("Antartical execution engines produced different results")
var ErrDuplicateExecutionEngine = errors.New("duplicate Antartical execution engine")

// ExecutionInput is the engine-neutral block execution request. All EVM
// implementations receive the same bytes and roots, which makes differential
// execution reproducible across Go, Rust/Revm, and evmone adapters.
type ExecutionInput struct {
	ParentStateRoot common.Hash
	BlockHash       common.Hash
	TransactionsRLP []byte
	Witness         []byte
}

type ExecutionOutput struct {
	StateRoot    common.Hash
	ReceiptsRoot common.Hash
	ProofDigest  common.Hash
}

type ExecutionEngine interface {
	Name() string
	Execute(ExecutionInput) (ExecutionOutput, error)
}

type EngineRegistry struct {
	engines   map[string]ExecutionEngine
	canonical string
}

func NewEngineRegistry(canonical ExecutionEngine) (*EngineRegistry, error) {
	if canonical == nil || canonical.Name() == "" {
		return nil, ErrExecutionEngineMismatch
	}
	r := &EngineRegistry{engines: make(map[string]ExecutionEngine), canonical: canonical.Name()}
	r.engines[canonical.Name()] = canonical
	return r, nil
}

func (r *EngineRegistry) Register(engine ExecutionEngine) error {
	if r == nil || engine == nil || engine.Name() == "" {
		return ErrExecutionEngineMismatch
	}
	if r.engines == nil {
		r.engines = make(map[string]ExecutionEngine)
	}
	if _, exists := r.engines[engine.Name()]; exists {
		return ErrDuplicateExecutionEngine
	}
	r.engines[engine.Name()] = engine
	return nil
}

func (r *EngineRegistry) Get(name string) (ExecutionEngine, bool) {
	if r == nil {
		return nil, false
	}
	engine, ok := r.engines[name]
	return engine, ok
}

// CompareEngines runs two implementations against the same input and checks
// every consensus-visible output. It is used by the formal verification and
// alternative-engine test harnesses before an adapter is selected.
func CompareEngines(primary, secondary ExecutionEngine, input ExecutionInput) error {
	if primary == nil || secondary == nil {
		return ErrExecutionEngineMismatch
	}
	a, err := primary.Execute(input)
	if err != nil {
		return err
	}
	b, err := secondary.Execute(input)
	if err != nil {
		return err
	}
	if a != b {
		return ErrExecutionEngineMismatch
	}
	return nil
}
