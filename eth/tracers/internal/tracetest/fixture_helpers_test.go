package tracetest

import "github.com/ethereum/go-ethereum/core"

// stopTracerFixture shuts down a test chain without leaving the snapshot
// journal goroutine behind. The production shutdown path intentionally keeps
// its normal persistence behavior; this helper is only for short-lived
// in-memory tracer fixtures.
func stopTracerFixture(chain *core.BlockChain) {
	if chain == nil {
		return
	}
	if snapshots := chain.Snapshots(); snapshots != nil {
		snapshots.Disable()
	}
	chain.Stop()
}
