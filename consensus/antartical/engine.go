package antartical

import (
	"bytes"
	"errors"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var ErrExecutionEngineMismatch = errors.New("Antartical execution engines produced different results")
var ErrDuplicateExecutionEngine = errors.New("duplicate Antartical execution engine")
var ErrExecutionEngineNotConformant = errors.New("Antartical execution engine failed conformance vectors")

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

// RegisterConformant admits an alternate backend only after it agrees with
// the canonical interpreter on every supplied vector. Callers should use this
// method for Rust/Revm, evmone, or any other implementation that may execute
// consensus transactions.
func (r *EngineRegistry) RegisterConformant(engine ExecutionEngine, vectors []ExecutionInput) error {
	if r == nil || engine == nil {
		return ErrExecutionEngineNotConformant
	}
	canonical, ok := r.Get(r.canonical)
	if !ok {
		return ErrExecutionEngineNotConformant
	}
	for _, vector := range vectors {
		if err := CompareEngines(canonical, engine, vector); err != nil {
			return ErrExecutionEngineNotConformant
		}
	}
	return r.Register(engine)
}

func (r *EngineRegistry) Get(name string) (ExecutionEngine, bool) {
	if r == nil {
		return nil, false
	}
	engine, ok := r.engines[name]
	return engine, ok
}

func (r *EngineRegistry) Canonical() (ExecutionEngine, bool) {
	if r == nil {
		return nil, false
	}
	return r.Get(r.canonical)
}

// Commitment records the set of admitted execution engines. Alternate
// implementations are descriptive only until RegisterConformant has compared
// them against the canonical engine; callers should commit this value in the
// Antartical metadata rather than selecting an engine by local preference.
func (r *EngineRegistry) Commitment() (common.Hash, error) {
	if r == nil || r.canonical == "" {
		return common.Hash{}, ErrExecutionEngineMismatch
	}
	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([][]byte, 0, len(names))
	for _, name := range names {
		if _, ok := r.engines[name]; !ok {
			return common.Hash{}, ErrExecutionEngineMismatch
		}
		entries = append(entries, []byte(name))
	}
	blob, err := rlp.EncodeToBytes([]interface{}{[]byte("TKM_EXECUTION_ENGINES_V1"), r.canonical, entries})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func (r *EngineRegistry) ConformantNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return bytes.Compare([]byte(names[i]), []byte(names[j])) < 0 })
	return names
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
