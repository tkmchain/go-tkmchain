# RandomX Consensus Engine for go-tkmchain

## Overview

This package implements a RandomX Proof-of-Work (PoW) consensus engine for the go-tkmchain blockchain. RandomX is a proof-of-work algorithm that is ASIC-resistant and optimized for general-purpose CPUs. It uses a heavy virtual machine and large memory requirements to ensure mining remains accessible to regular users.

## Features

- **ASIC-resistant mining**: Uses RandomX algorithm with large memory requirements (2GB+)
- **Dynamic difficulty adjustment**: Automatically adjusts difficulty with 2x cap to maintain ~2 minute block times
- **King reward system**: 
  - Main King: Receives 10% of block rewards
  - Rotating King: Receives 40% of block rewards (rotates every N blocks)
  - Miner: Receives 50% of block rewards
- **Epoch-based caching**: Cache and dataset are regenerated every 2048 blocks
- **Persistent difficulty storage**: Stores difficulty in database for chain continuity
- **Full and light mode**: Can operate with full dataset for faster mining or light mode for verification
- **JIT compilation**: Uses JIT compilation for improved performance
- **AES hardware acceleration**: Leverages AES-NI instructions when available

## Configuration

### Config Structure

```go
type Config struct {
    Enabled        bool   // Enable/disable RandomX
    EpochLength    uint64 // Blocks per epoch (default: 2048)
    CacheSize      uint64 // Size of RandomX cache
    DatasetSize    uint64 // Size of RandomX dataset
    MinMemory      uint64 // Minimum memory requirement
    PersistDataset bool   // Persist dataset to disk
}
```

### Default Configuration

```go
config := &Config{
    Enabled:     true,
    EpochLength: 2048,
    CacheSize:   256,  // MB
    DatasetSize: 2,    // GB
    MinMemory:   4,    // GB
}
```

## Usage

### Creating a New RandomX Engine

```go
import (
    "github.com/ethereum/go-ethereum/consensus/randomx"
    "github.com/ethereum/go-ethereum/common"
)

// Create configuration
config := randomx.DefaultConfig()

// Define king addresses
mainKing := common.HexToAddress("0x1234...")
rotatingKings := []common.Address{
    common.HexToAddress("0x5678..."),
    common.HexToAddress("0x9abc..."),
}

// Initialize RandomX
rx, err := randomx.New(config, 4, mainKing, rotatingKings, db)
if err != nil {
    log.Fatal("Failed to initialize RandomX", "error", err)
}
defer rx.Close()
```

### Mining Work

```go
// Get mining work
work, err := rx.GetWork()
if err != nil {
    log.Error("Failed to get work", "error", err)
}

// Work contains:
// - HeaderHash: Hash of the block header to mine
// - SeedHash: Seed hash for the current epoch
// - Target: Target value for valid proof
// - Difficulty: Current mining difficulty
// - BlockNumber: Block number being mined
```

### Submitting Work

```go
// Submit a valid nonce
valid, err := rx.SubmitWork(nonceHex, headerHashHex, mixDigestHex)
if err != nil {
    log.Error("Failed to submit work", "error", err)
}
if valid {
    log.Info("Block mined successfully!")
}
```

### API Endpoints

The RandomX engine exposes the following RPC APIs:

#### RandomX API (`randomx` namespace)

- `randomx_getSeedHash(blockNumber?)` - Get seed hash for a block
- `randomx_getCurrentEpoch(blockNumber)` - Get epoch for a block
- `randomx_getHashrate()` - Get current hashrate
- `randomx_getStats()` - Get mining statistics

#### Miner API (`miner` namespace)

- `miner_getWork()` - Get mining work
- `miner_submitWork(nonce, headerHash, mixDigest)` - Submit mined work
- `miner_getHashrate()` - Get current hashrate

## Reward Distribution

Block rewards are distributed as follows:

| Recipient | Percentage | Description |
|-----------|------------|-------------|
| Main King | 10% | Fixed address set at genesis |
| Rotating King | 40% | Rotates through a list of addresses |
| Miner | 50% | Block miner (coinbase) |

### Rotation Mechanism

Rotating kings rotate every `rotationInterval` blocks (default: 100). The index calculation is:

```go
index := (blockNumber / rotationInterval) % len(rotatingKings)
```

## Difficulty Adjustment

Difficulty adjusts based on actual block time compared to the target block time (120 seconds):

- **Target**: 2-minute block time
- **Adjustment**: Multiplicative with 2x cap
- **Minimum**: 2440
- **Maximum**: 10^30

The adjustment formula:

```
ratio = (targetTime * 100) / actualTime
newDifficulty = currentDifficulty * ratio / 100
```

Ratio is capped between 50 (0.5x) and 200 (2x).

## Memory Requirements

RandomX requires significant memory:

- **Cache**: ~256 MB
- **Dataset**: ~2 GB (full mode)
- **Light mode**: ~256 MB (cache only)

## Performance Considerations

1. **Dataset initialization**: Takes several minutes on first run
2. **Memory usage**: Full dataset requires 2+ GB of RAM
3. **JIT compilation**: Recommended for best performance
4. **AES-NI**: Hardware acceleration significantly improves performance
5. **Multi-threading**: Dataset generation and verification benefit from multiple threads

## Error Handling

Common errors:

- `errNoCache`: RandomX cache not initialized
- `errEngineClosed`: Engine has been closed
- `errInvalidWork`: Invalid work submitted
- `ErrInvalidNumber`: Invalid block number

## Testing

Use the `NewFaker()` function for testing environments:

```go
rx := randomx.NewFaker()
// All operations will succeed without actual work
```

## Dependencies

- **CGO**: Required for RandomX C library integration
- **RandomX C library**: Must be built with `build/_workspace/randomx/`
- **libstdc++**: C++ standard library
- **libm**: Math library

## Building

```bash
# Build RandomX library
cd build/_workspace/randomx
mkdir build && cd build
cmake ..
make

# Build go-tkmchain with RandomX
go build -tags randomx
```

## Logging

The engine provides extensive logging for:

- Initialization and shutdown
- Cache/dataset operations
- Difficulty adjustments
- Reward distribution
- Mining operations
- Validation results

## Security Considerations

1. **Memory protection**: Cache and dataset are protected by mutexes
2. **Resource cleanup**: All resources are properly released on shutdown
3. **Input validation**: All inputs are validated before processing
4. **DoS protection**: Rate limiting and validation prevent abuse

## License

This code is part of the go-tkmchain project and is licensed under the GNU Lesser General Public License v3.0.

## Contributing

Contributions are welcome! Please ensure:

1. Code follows the project's style guidelines
2. All tests pass
3. Documentation is updated
4. Changes are properly tested
