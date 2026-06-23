# RandomX Reward System

## Overview

The RandomX reward system implements a comprehensive reward distribution mechanism for the go-tkmchain blockchain. It features a multi-party reward distribution model with halving schedules, king rewards, and transaction fee distribution.

## Key Features

- **Multi-party reward distribution**: Splits rewards among Main King (10%), Rotating King (40%), and Miner (50%)
- **Halving schedule**: Block rewards halve approximately every 4 years (based on target 2-minute block time)
- **Transaction fee integration**: Fees are included in the total reward pool
- **King rotation mechanism**: Rotating King changes every N blocks
- **Eligibility thresholds**: Minimum balance requirements for king participation
- **Uncle rewards**: Support for uncle block rewards

## Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `EligibilityThreshold` | 100,000 ANTD | Minimum balance for king eligibility |
| `InitialBlockReward` | 200 ANTD | Starting block reward |
| `GenesisPremine` | 60,000,000 ANTD | Initial supply at genesis |
| `TargetBlockTimeSeconds` | 120 | Target block time in seconds |
| `BlocksPerHalving` | ~1,051,200 | ~4 years (based on 2-minute blocks) |
| `MaxHalvings` | 64 | Maximum number of halving events |

## Reward Distribution

### Distribution Percentages

| Recipient | Percentage | Description |
|-----------|------------|-------------|
| Main King | 10% | Fixed address set at genesis |
| Rotating King | 40% | Rotates through a list of addresses |
| Miner | 50% | Block miner (coinbase) |

### Reward Calculation

Block rewards are calculated using the halving formula:

```
reward = InitialBlockReward / (2 ^ halvingPeriod)
```

Where `halvingPeriod = blockNumber / BlocksPerHalving`

The total reward includes both block rewards and transaction fees:

```
totalReward = blockReward + transactionFees
```

### Distribution Flow

```
                    ┌─────────────────┐
                    │  Block Mined    │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Calculate      │
                    │  Block Reward   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Add Transaction│
                    │  Fees           │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Split Reward   │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
┌───────▼───────┐   ┌───────▼───────┐   ┌───────▼───────┐
│  Main King    │   │ Rotating King │   │    Miner      │
│    (10%)      │   │    (40%)      │   │    (50%)      │
└───────────────┘   └───────────────┘   └───────────────┘
```

## Halving Schedule

| Halving | Block Range | Reward (ANTD) |
|---------|-------------|---------------|
| 0 | 0 - 1,051,199 | 200.0 |
| 1 | 1,051,200 - 2,102,399 | 100.0 |
| 2 | 2,102,400 - 3,153,599 | 50.0 |
| 3 | 3,153,600 - 4,204,799 | 25.0 |
| 4 | 4,204,800 - 5,255,999 | 12.5 |
| ... | ... | ... |
| 64 | ... | < 0.000000000000000001 |

## Usage

### Basic Reward Calculation

```go
import "github.com/ethereum/go-ethereum/consensus/randomx"

// Calculate block reward
blockNumber := uint64(100000)
blockReward := randomx.CalculateBlockReward(blockNumber)

// Calculate total reward with fees
transactionFees := big.NewInt(1000000000000000000) // 1 ANTD
totalReward := randomx.CalculateTotalReward(blockReward, transactionFees)
```

### Distributing Rewards

```go
// Setup addresses
mainKing := common.HexToAddress("0x1234...")
rotatingKing := common.HexToAddress("0x5678...")
miner := common.HexToAddress("0x9abc...")

// Distribute rewards
totalDistributed := randomx.DistributeRewards(
    stateDB,
    mainKing,
    rotatingKing,
    miner,
    totalReward,
    blockNumber,
)
```

### Getting Reward Information

```go
// Get comprehensive reward info
rewardInfo := randomx.GetRewardInfo(blockNumber, blockReward, transactionFees)

// Access reward details
fmt.Printf("Total Reward: %s\n", rewardInfo["totalRewardFormatted"])
fmt.Printf("Main King: %s\n", rewardInfo["rewardsFormatted"].(map[string]string)["mainKing"])
fmt.Printf("Rotating King: %s\n", rewardInfo["rewardsFormatted"].(map[string]string)["rotatingKing"])
fmt.Printf("Miner: %s\n", rewardInfo["rewardsFormatted"].(map[string]string)["miner"])
```

### Halving Information

```go
// Get next halving information
halvingInfo := randomx.GetNextHalvingInfo(currentBlockNumber)

fmt.Printf("Next halving at block: %d\n", halvingInfo["nextHalvingBlock"])
fmt.Printf("Blocks until halving: %d\n", halvingInfo["blocksUntil"])
fmt.Printf("Current reward: %s\n", halvingInfo["currentRewardFormatted"])
fmt.Printf("Next reward: %s\n", halvingInfo["nextRewardFormatted"])
```

### Supply Calculations

```go
// Calculate total supply cap
maxSupply := randomx.CalculateTotalSupplyCap()
fmt.Printf("Maximum supply: %s ANTD\n", randomx.FormatANTD(maxSupply))

// Calculate circulating supply at a specific block
circulatingSupply := randomx.CalculateCirculatingSupply(blockNumber)
fmt.Printf("Circulating supply: %s ANTD\n", randomx.FormatANTD(circulatingSupply))
```

## Formatting Functions

### FormatANTD

Converts a raw big.Int amount to a human-readable string with decimal places:

```go
amount := big.NewInt(2000000000000000000) // 2 ANTD
formatted := randomx.FormatANTD(amount)
// Returns: "2.000000"
```

### ParseANTD

Parses a formatted ANTD string back to a big.Int:

```go
amount, err := randomx.ParseANTD("2.000000")
// Returns: 2000000000000000000
```

## Reward Distribution Process

### 1. Block Reward Calculation
- Calculates base block reward based on current halving period
- Uses `CalculateBlockReward(blockNumber)`

### 2. Transaction Fee Collection
- Sums all transaction fees from receipts
- Uses `GetTotalTransactionFees(header, receipts)`

### 3. Total Reward Calculation
- Combines block reward and transaction fees
- Uses `CalculateTotalReward(blockReward, transactionFees)`

### 4. Reward Splitting
- Main King: 10% of total reward
- Rotating King: 40% of total reward
- Miner: 50% of total reward

### 5. Distribution Execution
- Updates state balances for all recipients
- Logs distribution details
- Handles rounding adjustments

### 6. Uncle Rewards (if applicable)
- Calculates uncle rewards using formula: `(uncleNumber + 8 - blockNumber) * blockReward / 8`
- Distributes to uncle miners

## RPC Integration

The reward system integrates with the RandomX RPC API:

```bash
# Get reward information for a block
curl -X POST -H "Content-Type: application/json" --data '{
    "jsonrpc":"2.0",
    "method":"randomx_getRewardInfo",
    "params":[123456],
    "id":1
}' http://localhost:8545
```

## Transaction Fees

Transaction fees are included in the total reward pool and distributed according to the same percentages:

```
Total Reward = Block Reward + Sum(Transaction Fees)
```

All fees are distributed to:
- Main King (10%)
- Rotating King (40%)  
- Miner (50%)

## Uncle Rewards

Uncle blocks receive a reduced reward:

```
uncleReward = (uncleNumber + 8 - blockNumber) * blockReward / 8
```

- Uncle blocks must be included in the main chain
- Rewards are distributed to the uncle miner
- Uncle rewards do not include transaction fees

## Supply Calculations

### Maximum Supply

The theoretical maximum supply includes:
- Genesis premine: 60,000,000 ANTD
- All halving periods until reward reaches 0

### Circulating Supply

Calculates approximate circulating supply at any block:
- Genesis premine
- All rewards up to the specified block
- Does not account for burned tokens

## Error Handling

The reward system includes comprehensive error handling:

```go
if totalReward == nil || totalReward.Sign() == 0 {
    log.Debug("No rewards to distribute", "block", blockNumber)
    return big.NewInt(0)
}

if mainKing == (common.Address{}) {
    log.Warn("Main King address is empty, skipping reward")
}
```

## Logging

The reward system provides extensive logging:

```
========================================
�� REWARD DISTRIBUTION START
========================================
�� Block Information block=100000 totalReward=200.000000
�� Reward Distribution Breakdown:
  Main King (10%): 20.000000
  Rotating King (40%): 80.000000
  Miner (50%): 100.000000
  Main King reward sent: 0x1234... (20.000000)
  Rotating King reward sent: 0x5678... (80.000000)
  Miner reward sent: 0x9abc... (100.000000)
========================================
  REWARD DISTRIBUTION COMPLETE
========================================
```

## Performance Considerations

- **State updates**: Each reward distribution requires state database updates
- **Transaction fees**: Fee calculation iterates through all receipts
- **Halving calculations**: Uses integer arithmetic for precision
- **Memory usage**: Reward calculations use minimal memory
- **Logging**: Extensive logging may impact performance in production

## Security Considerations

1. **Address validation**: All recipient addresses are validated
2. **Balance overflow protection**: Uses uint256 for state updates
3. **Rounding handling**: Adjusts rewards to ensure total matches
4. **Zero-value checks**: Skips distributions for zero rewards
5. **State consistency**: Uses atomic state updates

## Testing

### Unit Tests

```go
func TestCalculateBlockReward(t *testing.T) {
    // Test initial reward
    reward := CalculateBlockReward(0)
    assert.Equal(t, InitialBlockReward, reward)
    
    // Test halving
    reward = CalculateBlockReward(BlocksPerHalving)
    expected := new(big.Int).Div(InitialBlockReward, big.NewInt(2))
    assert.Equal(t, expected, reward)
}
```

### Integration Tests

The reward system should be tested with:
1. Full block generation and validation
2. Transaction fee inclusion
3. Uncle block rewards
4. King rotation
5. Halving transitions

## Dependencies

- `github.com/ethereum/go-ethereum/common`: Common types and utilities
- `github.com/ethereum/go-ethereum/core/state`: State database interface
- `github.com/ethereum/go-ethereum/core/types`: Block and transaction types
- `github.com/ethereum/go-ethereum/log`: Logging framework
- `github.com/holiman/uint256`: 256-bit integer arithmetic

## License

This code is part of the go-tkmchain project and is licensed under the GNU Lesser General Public License v3.0.

## Contributing

Contributions are welcome! Please ensure:
1. Code follows the project's style guidelines
2. All tests pass
3. Documentation is updated
4. Changes are properly tested with both unit and integration tests
