package antartical

import "errors"

var ErrGasDimensionExceeded = errors.New("Antartical gas dimension exceeded")

// GasVector separates execution, state-read, state-write, and blob resources.
// Addition and comparison are integer-only, making the result independent of
// machine word size or execution order.
type GasVector struct {
	Execution  uint64
	StateRead  uint64
	StateWrite uint64
	Blob       uint64
}

func (g GasVector) Add(other GasVector) GasVector {
	return GasVector{g.Execution + other.Execution, g.StateRead + other.StateRead, g.StateWrite + other.StateWrite, g.Blob + other.Blob}
}

func (g GasVector) Fits(limit GasVector) bool {
	return g.Execution <= limit.Execution && g.StateRead <= limit.StateRead && g.StateWrite <= limit.StateWrite && g.Blob <= limit.Blob
}

func (g GasVector) Charge(limit *GasVector, amount GasVector) error {
	if limit == nil {
		return ErrGasDimensionExceeded
	}
	next := g.Add(amount)
	if !next.Fits(*limit) {
		return ErrGasDimensionExceeded
	}
	return nil
}
