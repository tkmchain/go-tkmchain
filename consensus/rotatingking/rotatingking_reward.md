# Rotating King Rewards Documentation

## Overview

The Rotating King Rewards system handles the distribution of block rewards among the main king, rotating king, and miner. It implements a configurable reward distribution mechanism with percentage-based splits and proper error handling.

## Reward Distribution

### Distribution Percentages

| Recipient | Percentage | Description |
|-----------|------------|-------------|
| Main King | 10% | Fixed address set at genesis |
| Rotating King | 40% | Dynamically rotating address |
| Miner | 50% | Block producer (coinbase) |

### Distribution Flow

```
                    ┌─────────────────┐
                    │  Block Mined    │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Calculate      │
                    │  Total Reward   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Split Reward   │
                    │  by Percentages │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
┌───────▼───────┐   ┌───────▼───────┐   ┌───────▼───────┐
│  Main King    │   │ Rotating King │   │    Miner      │
│    (10%)      │   │    (40%)      │   │    (50%)      │
└───────────────┘   └───────────────┘   └───────────────┘
```

## Functions

### DistributeRewards

Distributes block rewards among all parties.

```go
func DistributeRewards(
    stateDB *state.StateDB,
    mainKing common.Address,
    rotatingKing common.Address,
    miner common.Address,
    totalReward *big.Int,
    blockNumber uint64,
) *big.Int
```

**Parameters:**
- `stateDB`: State database for balance updates
- `mainKing`: Fixed main king address
- `rotatingKing`: Current rotating king address
- `miner`: Block miner address
- `totalReward`: Total reward to distribute
- `blockNumber`: Current block number

**Returns:**
- Total reward distributed

**Example:**

```go
import "github.com/ethereum/go-ethereum/consensus/rotatingking"

// Calculate total reward
blockReward := big.NewInt(2000000000000000000) // 2 ANTD
txFees := big.NewInt(100000000000000000)      // 0.1 ANTD
totalReward := rotatingking.CalculateTotalReward(blockReward, txFees)

// Distribute rewards
distributed := rotatingking.DistributeRewards(
    stateDB,
    mainKingAddr,
    rotatingKingAddr,
    minerAddr,
    totalReward,
    blockNumber,
)
```

### CalculateTotalReward

Calculates the total reward including block reward and transaction fees.

```go
func CalculateTotalReward(blockReward *big.Int, transactionFees *big.Int) *big.Int
```

**Parameters:**
- `blockReward`: Base block reward
- `transactionFees`: Total transaction fees

**Returns:**
- Combined total reward

**Example:**

```go
blockReward := big.NewInt(2000000000000000000)
txFees := big.NewInt(50000000000000000)
total := rotatingking.CalculateTotalReward(blockReward, txFees)
// total = 2.05 ANTD
```

## Usage Examples

### Basic Reward Distribution

```go
package main

import (
    "math/big"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/state"
    "github.com/ethereum/go-ethereum/consensus/rotatingking"
)

func distributeBlockRewards(
    stateDB *state.StateDB,
    blockNumber uint64,
    blockReward *big.Int,
    txFees *big.Int,
    mainKing common.Address,
    rotatingKing common.Address,
    miner common.Address,
) {
    // Calculate total reward
    totalReward := rotatingking.CalculateTotalReward(blockReward, txFees)
    
    // Distribute rewards
    rotatingking.DistributeRewards(
        stateDB,
        mainKing,
        rotatingKing,
        miner,
        totalReward,
        blockNumber,
    )
}
```

### Integration with Block Finalization

```go
func (rx *RandomX) FinalizeAndAssemble(
    chain consensus.ChainHeaderReader,
    header *types.Header,
    state *state.StateDB,
    body *types.Body,
    receipts []*types.Receipt,
) (*types.Block, error) {
    // Calculate rewards
    blockReward := CalculateBlockReward(header.Number.Uint64())
    totalFees := GetTotalTransactionFees(header, receipts)
    totalReward := rotatingking.CalculateTotalReward(blockReward, totalFees)
    
    // Get kings
    mainKing := rx.GetMainKing()
    rotatingKing := rx.GetCurrentKing()
    miner := header.Coinbase
    
    // Distribute rewards
    if totalReward.Sign() > 0 {
        rotatingking.DistributeRewards(
            state,
            mainKing,
            rotatingKing,
            miner,
            totalReward,
            header.Number.Uint64(),
        )
    }
    
    return types.NewBlock(header, body, receipts, trie.NewStackTrie(nil)), nil
}
```

### Validation and Error Handling

```go
func distributeWithValidation(
    stateDB *state.StateDB,
    mainKing common.Address,
    rotatingKing common.Address,
    miner common.Address,
    totalReward *big.Int,
    blockNumber uint64,
) error {
    // Validate addresses
    if mainKing == (common.Address{}) {
        return fmt.Errorf("main king address is empty")
    }
    if miner == (common.Address{}) {
        return fmt.Errorf("miner address is empty")
    }
    
    // Validate reward
    if totalReward.Sign() < 0 {
        return fmt.Errorf("invalid reward amount: %s", totalReward.String())
    }
    
    // Distribute rewards
    if totalReward.Sign() == 0 {
        log.Warn("Zero reward, skipping distribution", "block", blockNumber)
        return nil
    }
    
    rotatingking.DistributeRewards(
        stateDB,
        mainKing,
        rotatingKing,
        miner,
        totalReward,
        blockNumber,
    )
    
    return nil
}
```

## Reward Calculation Details

### Percentage Calculation

The reward split uses exact integer arithmetic through `big.Float` for precision:

```go
distribution := DefaultRewardDistribution()
totalBig := new(big.Float).SetInt(totalReward)

mainKingPercent := new(big.Float).SetFloat64(0.10)
rotatingKingPercent := new(big.Float).SetFloat64(0.40)
minerPercent := new(big.Float).SetFloat64(0.50)

// Calculate each share
mainKingReward := new(big.Int)
new(big.Float).Mul(totalBig, mainKingPercent).Int(mainKingReward)

rotatingKingReward := new(big.Int)
new(big.Float).Mul(totalBig, rotatingKingPercent).Int(rotatingKingReward)

minerReward := new(big.Int)
new(big.Float).Mul(totalBig, minerPercent).Int(minerReward)
```

### Example Calculation

For a total reward of 2.5 ANTD:

```
Total Reward: 2,500,000,000,000,000,000 (2.5 ANTD)

Main King (10%):  250,000,000,000,000,000 (0.25 ANTD)
Rotating King (40%): 1,000,000,000,000,000,000 (1.0 ANTD)
Miner (50%):       1,250,000,000,000,000,000 (1.25 ANTD)
```

## State Updates

### Balance Addition

```go
stateDB.AddBalance(
    address,
    uint256.MustFromBig(rewardAmount),
    tracing.BalanceIncreaseRewardMineBlock,
)
```

### Tracing

All reward distributions are traced for auditing:

```go
tracing.BalanceIncreaseRewardMineBlock   // Block mining reward
tracing.BalanceIncreaseRewardMineUncle   // Uncle block reward
```

## Logging

### Debug Logs

```go
log.Debug("Main king reward distributed",
    "address", mainKing.Hex(),
    "amount", mainKingReward.String())
```

### Info Logs

```go
log.Info("Block rewards distributed",
    "block", blockNumber,
    "total", totalReward.String(),
    "mainKing", mainKingReward.String(),
    "rotatingKing", rotatingKingReward.String(),
    "miner", minerReward.String())
```

## Integration Examples

### Complete Block Processing

```go
func processBlock(
    stateDB *state.StateDB,
    block *types.Block,
    receipts []*types.Receipt,
    mainKing common.Address,
    rotatingKing common.Address,
) error {
    header := block.Header()
    blockNumber := header.Number.Uint64()
    
    // Calculate rewards
    blockReward := CalculateBlockReward(blockNumber)
    txFees := GetTotalTransactionFees(header, receipts)
    totalReward := rotatingking.CalculateTotalReward(blockReward, txFees)
    
    // Distribute rewards
    if totalReward.Sign() > 0 {
        rotatingking.DistributeRewards(
            stateDB,
            mainKing,
            rotatingKing,
            header.Coinbase,
            totalReward,
            blockNumber,
        )
    }
    
    // Update state root
    header.Root = stateDB.IntermediateRoot(true)
    
    return nil
}
```

### RPC Integration

```go
type RewardAPI struct {
    stateDB *state.StateDB
    manager *rotatingking.RotatingKingManager
}

func (api *RewardAPI) GetRewardDistribution(blockNumber uint64) map[string]interface{} {
    // Get current kings
    mainKing := api.manager.GetMainKing()
    rotatingKing := api.manager.GetCurrentKing()
    
    // Calculate rewards
    blockReward := CalculateBlockReward(blockNumber)
    txFees := big.NewInt(0) // Get from block
    
    totalReward := rotatingking.CalculateTotalReward(blockReward, txFees)
    
    // Calculate distribution
    distribution := DefaultRewardDistribution()
    mainKingReward := new(big.Int).Mul(totalReward, big.NewInt(int64(distribution.MainKingPercent)))
    mainKingReward.Div(mainKingReward, big.NewInt(100))
    
    return map[string]interface{}{
        "blockNumber": blockNumber,
        "totalReward": totalReward.String(),
        "distribution": map[string]interface{}{
            "mainKing": map[string]interface{}{
                "address": mainKing.Hex(),
                "percent": distribution.MainKingPercent,
                "amount":  mainKingReward.String(),
            },
            "rotatingKing": map[string]interface{}{
                "address": rotatingKing.Hex(),
                "percent": distribution.RotatingKingPercent,
                "amount":  "", // Calculate similarly
            },
            "miner": map[string]interface{}{
                "percent": distribution.MinerPercent,
                "amount":  "", // Calculate similarly
            },
        },
    }
}
```

## Performance Considerations

1. **Big.Int Operations**: Uses `big.Int` and `big.Float` for precision
2. **State Updates**: Each distribution requires 3 state updates
3. **Gas Costs**: Balance updates consume gas proportional to operation
4. **Logging**: Debug logs should be disabled in production for performance
5. **Memory Usage**: Minimal, only uses temporary big.Int allocations

## Error Handling

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| Zero reward | Block reward is 0 | Check halving schedule |
| Empty address | Address is zero | Validate addresses before distribution |
| Negative reward | Invalid calculation | Check reward calculation logic |
| State error | StateDB unavailable | Verify stateDB initialization |

### Error Prevention

```go
func safeDistributeRewards(
    stateDB *state.StateDB,
    mainKing common.Address,
    rotatingKing common.Address,
    miner common.Address,
    totalReward *big.Int,
    blockNumber uint64,
) error {
    // Validate inputs
    if stateDB == nil {
        return fmt.Errorf("stateDB is nil")
    }
    if totalReward.Sign() < 0 {
        return fmt.Errorf("negative reward: %s", totalReward.String())
    }
    
    // Check if addresses are valid
    if mainKing == (common.Address{}) {
        return fmt.Errorf("main king address is empty")
    }
    if miner == (common.Address{}) {
        return fmt.Errorf("miner address is empty")
    }
    
    // Distribute rewards
    rotatingking.DistributeRewards(
        stateDB,
        mainKing,
        rotatingKing,
        miner,
        totalReward,
        blockNumber,
    )
    
    return nil
}
```

## Testing

### Unit Test Example

```go
func TestRewardDistribution(t *testing.T) {
    // Setup
    stateDB := setupTestState()
    mainKing := common.HexToAddress("0x1234")
    rotatingKing := common.HexToAddress("0x5678")
    miner := common.HexToAddress("0x9abc")
    
    totalReward := big.NewInt(1000000000000000000) // 1 ANTD
    
    // Distribute
    distributed := rotatingking.DistributeRewards(
        stateDB,
        mainKing,
        rotatingKing,
        miner,
        totalReward,
        1,
    )
    
    // Verify
    assert.Equal(t, totalReward, distributed)
    
    // Check balances
    mainKingBalance := stateDB.GetBalance(mainKing)
    expectedMainKing := new(big.Int).Div(totalReward, big.NewInt(10))
    assert.Equal(t, expectedMainKing, mainKingBalance)
}
```

### Integration Test

```go
func TestFullRewardIntegration(t *testing.T) {
    // Create blockchain
    chain := setupBlockchain()
    
    // Mine block
    block := mineBlock(chain)
    
    // Process block
    err := processBlock(chain.StateDB(), block)
    assert.NoError(t, err)
    
    // Verify rewards
    verifyRewards(t, chain.StateDB(), block)
}
```

## License

This code is part of the go-tkmchain project and is licensed under the GNU Lesser General Public License v3.0.

## Contributing

Contributions are welcome! Please ensure:
1. Code follows project style guidelines
2. All tests pass
3. Documentation is updated
4. Changes are properly tested with both unit and integration tests
