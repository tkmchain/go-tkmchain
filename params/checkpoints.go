
package params

import (
	"github.com/ethereum/go-ethereum/common"
)

// Checkpoint defines a hardcoded (blockNumber, blockHash) pair.
type Checkpoint struct {
	Number uint64
	Hash   common.Hash
}

// Checkpoints holds all hardcoded checkpoints for a given network.
type Checkpoints struct {
	// Map from block number to block hash
	Points map[uint64]common.Hash
}

// initRandomXCheckpoints initialises the checkpoints for the RandomX mainnet.
func initRandomXCheckpoints() *Checkpoints {
	cp := &Checkpoints{
		Points: make(map[uint64]common.Hash),
	}
	// Example checkpoint: block 0 (genesis) must match the actual genesis hash.
	cp.Points[0] = common.HexToHash("0x6bdca03e891cd028a92355065c211ead725d3e3be9f4de1047c3c5faa464a55e")

	// Add more checkpoints at strategic heights
	// cp.Points[1000] = common.HexToHash("0x...")
	// cp.Points[2000] = common.HexToHash("0x...")
	// cp.Points[10000] = common.HexToHash("0x...")

	return cp
}

// GetCheckpoint returns the hardcoded block hash for a given height, if any.
func (c *Checkpoints) GetCheckpoint(number uint64) (common.Hash, bool) {
	hash, ok := c.Points[number]
	return hash, ok
}
