# Rotating King Manager Documentation

## Overview

The Rotating King Manager is a core component of the go-tkmchain blockchain that manages the dynamic rotation of king addresses receiving block rewards. It implements a sophisticated system for tracking, validating, and rotating king addresses while ensuring eligibility and fair distribution.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    RotatingKingManager                      │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐    ┌──────────────────────────────┐   │
│  │     Config      │    │          State               │   │
│  ├─────────────────┤    ├──────────────────────────────┤   │
│  │ RotationInterval│    │ CurrentKingIndex             │   │
│  │ KingAddresses   │    │ RotationHeight               │   │
│  │ ActivationHeights│   │ NextRotationAt               │   │
│  │ MinStakeRequired│    │ KingsHistory                 │   │
│  │ ActivationDelay │    │ TotalRewardsDistributed      │   │
│  └─────────────────┘    └──────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Main King (Fixed)                      │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │            Rotating Kings (Dynamic)                 │    │
│  ├───────┬───────┬───────┬───────┬───────┬───────────┤    │
│  │ King 1│ King 2│ King 3│ King 4│ King 5│    ...    │    │
│  └───────┴───────┴───────┴───────┴───────┴───────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Core Components

### RotatingKingConfig

Configuration parameters for the rotating king system:

```go
type RotatingKingConfig struct {
    RotationInterval  uint64                    // Blocks between rotations
    RotationOffset    uint64                    // Offset for first rotation
    KingAddresses     []common.Address          // List of rotating king addresses
    ActivationHeights map[common.Address]uint64 // Block height when each king becomes active
    ActivationDelay   uint64                    // Delay before activation
    MinStakeRequired  *big.Int                  // Minimum stake required for eligibility
}
```

### RotatingKingState

Runtime state tracking:

```go
type RotatingKingState struct {
    CurrentKingIndex        int                    // Index of current king
    RotationHeight          uint64                 // Block height of last rotation
    NextRotationAt          uint64                 // Block height of next rotation
    LastUpdated             time.Time              // Last update timestamp
    RotationCount           uint64                 // Total rotations executed
    KingsHistory            []KingRotation         // History of rotations
    TotalRewardsDistributed *big.Int              // Total rewards distributed
    KingRewards             map[common.Address]*big.Int // Rewards per king
}
```

### KingRotation

Record of a rotation event:

```go
type KingRotation struct {
    BlockHeight  uint64        // Block when rotation occurred
    PreviousKing common.Address // Previous king address
    NewKing      common.Address // New king address
    Timestamp    time.Time     // Rotation timestamp
    Reward       *big.Int      // Reward amount
    WasEligible  bool          // Whether king was eligible
}
```

## Key Features

### 1. King Eligibility

Kings must maintain a minimum balance to remain eligible:

```go
const EligibilityThreshold = 100,000 ANTD
```

Eligibility checking:
- Balance verification on rotation
- Automatic skipping of ineligible kings
- Fallback to next eligible king

### 2. Rotation Logic

Rotation occurs at fixed intervals:

```
Rotation Interval: N blocks
Rotation: CurrentKing -> NextKing (circular)
Eligibility: Skip ineligible kings
Activation: New kings become active after activationHeight
```

### 3. Activation Management

New kings can be added with delayed activation:

```go
// Add king at specific activation height
manager.AddKingAddressAt(address, activationHeight)

// Add king immediately
manager.AddKingAddress(address)
```

## Usage Examples

### Creating a Manager

```go
import "github.com/ethereum/go-ethereum/consensus/rotatingking"

// Define main king and rotating kings
mainKing := common.HexToAddress("0x1234567890123456789012345678901234567890")
rotatingKings := []common.Address{
    common.HexToAddress("0x2345678901234567890123456789012345678901"),
    common.HexToAddress("0x3456789012345678901234567890123456789012"),
    common.HexToAddress("0x4567890123456789012345678901234567890123"),
}

// Create manager with 10000 block rotation interval
manager := rotatingking.NewRotatingKingManager(mainKing, rotatingKings, 10000)
```

### Getting Current King

```go
// Get current rotating king
currentKing := manager.GetCurrentKing()
fmt.Printf("Current King: %s\n", currentKing.Hex())

// Get main king
mainKing := manager.GetMainKing()
fmt.Printf("Main King: %s\n", mainKing.Hex())

// Get next king
nextKing := manager.GetNextKing()
fmt.Printf("Next King: %s\n", nextKing.Hex())
```

### Checking Eligibility

```go
// State provider implementation
type StateProvider struct {
    db ethdb.Database
}

func (sp *StateProvider) GetBalance(address common.Address) *big.Int {
    // Return balance from state
}

func (sp *StateProvider) GetBlockNumber() uint64 {
    // Return current block number
}

// Check and rotate if needed
if manager.ShouldRotate(currentBlockHeight) {
    stateProvider := &StateProvider{db: db}
    err := manager.RotateToNextKing(currentBlockHeight, blockHash, stateProvider)
    if err != nil {
        log.Error("Failed to rotate king", "error", err)
    }
}
```

### Managing King Addresses

```go
// Add a new king immediately
newKing := common.HexToAddress("0x5678901234567890123456789012345678901234")
manager.AddKingAddress(newKing)

// Add a king with activation delay
activationHeight := uint64(100000)
manager.AddKingAddressAt(newKing, activationHeight)

// Get all king addresses
allKings := manager.GetKingAddresses()

// Check if address is a king
isKing := manager.IsKing(address)
```

### Getting Rotation Information

```go
// Get detailed rotation info
info := manager.GetRotationInfo(currentBlockHeight)

fmt.Printf("Current King: %s\n", info["currentKing"])
fmt.Printf("Next King: %s\n", info["nextKing"])
fmt.Printf("Blocks until rotation: %d\n", info["blocksUntilRotation"])
fmt.Printf("Rotation Interval: %d\n", info["rotationInterval"])
fmt.Printf("Total Rotations: %d\n", info["rotationCount"])
```

### Monitoring Responsibilities

```go
// Get monitoring responsibilities
responsibilities := manager.GetMonitoringResponsibilities()

for _, category := range responsibilities {
    fmt.Printf("Category: %s\n", category.Name)
    fmt.Printf("Metrics: %s\n", strings.Join(category.Metrics, ", "))
}
```

## Advanced Features

### 1. State Provider Interface

The `BlockchainStateProvider` interface enables flexible state access:

```go
type BlockchainStateProvider interface {
    GetBalance(common.Address) *big.Int
    GetBlockNumber() uint64
}
```

### 2. Rotation History

Track all rotations for auditing and analysis:

```go
// Access rotation history
state := manager.state // Internal - use API for public access
// Implement custom history access through API
```

### 3. Reward Tracking

Track rewards distributed per king:

```go
// Manager tracks rewards internally
// Access through API methods
totalRewards := manager.state.TotalRewardsDistributed
```

## Error Handling

| Error | Description | Resolution |
|-------|-------------|------------|
| `no king addresses configured` | No addresses in rotation list | Add king addresses |
| `rotation not due yet` | Attempted rotation too early | Wait until next rotation block |
| `ineligible king` | King below threshold | Increase balance or skip |

## Performance Considerations

- **O(1)** operations for current king access
- **O(n)** operations for eligibility checks
- **O(n)** for address management operations
- Rotation history limited to last 100 entries
- Thread-safe with RWMutex

## Security Considerations

1. **Eligibility Verification**: All kings must maintain minimum balance
2. **Activation Delays**: New kings can have delayed activation
3. **Rotation Validation**: Ensures valid transitions
4. **History Tracking**: Complete audit trail of rotations
5. **Thread Safety**: All operations are thread-safe

## Integration with Reward System

```go
// Integrate with reward distribution
func distributeBlockRewards(
    manager *rotatingking.RotatingKingManager,
    stateDB *state.StateDB,
    blockNumber uint64,
    blockReward *big.Int,
) {
    // Get current kings
    mainKing := manager.GetMainKing()
    rotatingKing := manager.GetCurrentKing()
    miner := block.Coinbase()
    
    // Distribute rewards using the reward system
    totalReward := randomx.DistributeRewards(
        stateDB,
        mainKing,
        rotatingKing,
        miner,
        blockReward,
        blockNumber,
    )
}
```

## Logging

The manager provides comprehensive logging:

```
INFO [01-01|00:00:00.000] King rotation executed 
  previousKing=0x1234... newKing=0x5678... 
  blockHeight=100000 nextRotationAt=110000

INFO [01-01|00:00:00.000] Found eligible king 
  address=0x7890...
```

## Testing

### Unit Test Example

```go
func TestKingRotation(t *testing.T) {
    mainKing := common.HexToAddress("0x1234")
    kings := []common.Address{
        common.HexToAddress("0x5678"),
        common.HexToAddress("0x9abc"),
    }
    
    manager := NewRotatingKingManager(mainKing, kings, 100)
    
    // Test initial state
    assert.Equal(t, kings[0], manager.GetCurrentKing())
    
    // Test rotation
    err := manager.RotateToNextKing(100, common.Hash{}, nil)
    assert.NoError(t, err)
    assert.Equal(t, kings[1], manager.GetCurrentKing())
}
```

## Configuration Example

```yaml
rotatingking:
  mainKing: "0x1234567890123456789012345678901234567890"
  rotationInterval: 10000
  rotatingKings:
    - "0x2345678901234567890123456789012345678901"
    - "0x3456788901234567890123456789012345678902"
    - "0x4567899901234567890123456789012345678903"
  minStakeRequired: "100000000000000000000000" # 100k ANTD
  activationDelay: 2
```

## License

This code is part of the go-tkmchain project and is licensed under the GNU Lesser General Public License v3.0.

## Contributing

Contributions are welcome! Please ensure:
1. Code follows project style guidelines
2. All tests pass
3. Documentation is updated
4. Changes are properly tested
