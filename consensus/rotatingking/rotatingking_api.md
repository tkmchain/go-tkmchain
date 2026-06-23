# Rotating King API Documentation

## Overview

The Rotating King API provides RPC interfaces for interacting with the rotating king system in the go-tkmchain blockchain. This system manages the dynamic rotation of king addresses that receive a portion of block rewards, enabling decentralized governance and reward distribution.

## Core Concepts

### King Types

| Type | Description | Reward Share |
|------|-------------|--------------|
| **Main King** | Fixed address set at genesis | 10% of block rewards |
| **Rotating King** | Dynamic address that rotates periodically | 40% of block rewards |

### King Rotation

Rotating kings change every `rotationInterval` blocks, allowing for:
- Decentralized governance participation
- Fair reward distribution among multiple parties
- Community engagement through regular rotations

## API Reference

### GetCurrentKing

Returns the currently active rotating king.

```go
func (api *API) GetCurrentKing() common.Address
```

**Example Request** (JSON-RPC):

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_getCurrentKing",
    "params": [],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": "0x1234567890123456789012345678901234567890",
    "id": 1
}
```

---

### GetMainKing

Returns the fixed main king address.

```go
func (api *API) GetMainKing() common.Address
```

**Example Request**:

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_getMainKing",
    "params": [],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": "0xabcdef1234567890abcdef1234567890abcdef12",
    "id": 1
}
```

---

### GetNextKing

Returns the next king in the rotation sequence.

```go
func (api *API) GetNextKing() common.Address
```

**Example Request**:

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_getNextKing",
    "params": [],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": "0x9876543210987654321098765432109876543210",
    "id": 1
}
```

---

### GetRotationInfo

Returns comprehensive rotation information for a given block height.

```go
func (api *API) GetRotationInfo(height uint64) map[string]interface{}
```

**Parameters**:
- `height`: Block height to calculate rotation info for

**Returns**:
- `currentKing`: Current rotating king address
- `nextKing`: Next king in rotation
- `currentIndex`: Current king index
- `nextIndex`: Next king index
- `currentStartBlock`: Block when current king started
- `nextStartBlock`: Block when next king starts
- `blocksRemaining`: Blocks until next rotation

**Example Request**:

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_getRotationInfo",
    "params": [123456],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": {
        "currentKing": "0x1234567890123456789012345678901234567890",
        "nextKing": "0x9876543210987654321098765432109876543210",
        "currentIndex": 2,
        "nextIndex": 3,
        "currentStartBlock": 100000,
        "nextStartBlock": 110000,
        "blocksRemaining": 6544,
        "rotationInterval": 10000
    },
    "id": 1
}
```

---

### GetKingAddresses

Returns all rotating king addresses in the rotation list.

```go
func (api *API) GetKingAddresses() []common.Address
```

**Example Request**:

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_getKingAddresses",
    "params": [],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": [
        "0x1234567890123456789012345678901234567890",
        "0x2345678901234567890123456789012345678901",
        "0x3456789012345678901234567890123456789012",
        "0x4567890123456789012345678901234567890123"
    ],
    "id": 1
}
```

---

### IsKing

Checks if a given address is a rotating king.

```go
func (api *API) IsKing(address common.Address) bool
```

**Parameters**:
- `address`: Ethereum address to check

**Returns**:
- `true`: Address is in the rotating king list
- `false`: Address is not in the rotating king list

**Example Request**:

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_isKing",
    "params": ["0x1234567890123456789012345678901234567890"],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": true,
    "id": 1
}
```

---

### GetMonitoringResponsibilities

Returns the list of monitoring categories for rotating kings.

```go
func (api *API) GetMonitoringResponsibilities() []MonitoringCategory
```

**Returns**: List of monitoring categories with their descriptions

**Example Request**:

```json
{
    "jsonrpc": "2.0",
    "method": "rotatingking_getMonitoringResponsibilities",
    "params": [],
    "id": 1
}
```

**Example Response**:

```json
{
    "jsonrpc": "2.0",
    "result": [
        {
            "id": "network_health",
            "name": "Network Health",
            "description": "Monitor network health and performance"
        },
        {
            "id": "governance_proposals",
            "name": "Governance Proposals",
            "description": "Review and vote on governance proposals"
        },
        {
            "id": "consensus_performance",
            "name": "Consensus Performance",
            "description": "Monitor consensus algorithm performance"
        },
        {
            "id": "security_audits",
            "name": "Security Audits",
            "description": "Review security audit reports"
        }
    ],
    "id": 1
}
```

## Usage Examples

### Command Line (curl)

```bash
# Get current king
curl -X POST -H "Content-Type: application/json" --data '{
    "jsonrpc":"2.0",
    "method":"rotatingking_getCurrentKing",
    "params":[],
    "id":1
}' http://localhost:8545

# Get rotation info
curl -X POST -H "Content-Type: application/json" --data '{
    "jsonrpc":"2.0",
    "method":"rotatingking_getRotationInfo",
    "params":[123456],
    "id":1
}' http://localhost:8545

# Check if address is a king
curl -X POST -H "Content-Type: application/json" --data '{
    "jsonrpc":"2.0",
    "method":"rotatingking_isKing",
    "params":["0x1234567890123456789012345678901234567890"],
    "id":1
}' http://localhost:8545
```

### Web3.js

```javascript
const Web3 = require('web3');
const web3 = new Web3('http://localhost:8545');

// Get current king
const currentKing = await web3.eth.call({
    method: 'rotatingking_getCurrentKing',
    params: []
});

// Get rotation info
const rotationInfo = await web3.eth.call({
    method: 'rotatingking_getRotationInfo',
    params: [123456]
});

console.log(`Current King: ${currentKing}`);
console.log(`Next King: ${rotationInfo.nextKing}`);
console.log(`Blocks until rotation: ${rotationInfo.blocksRemaining}`);
```

### Python (web3.py)

```python
from web3 import Web3

w3 = Web3(Web3.HTTPProvider('http://localhost:8545'))

# Get current king
current_king = w3.manager.request_blocking(
    'rotatingking_getCurrentKing',
    []
)

# Get rotation info
rotation_info = w3.manager.request_blocking(
    'rotatingking_getRotationInfo',
    [123456]
)

print(f"Current King: {current_king}")
print(f"Next King: {rotation_info['nextKing']}")
print(f"Blocks until rotation: {rotation_info['blocksRemaining']}")
```

## Integration with Reward Distribution

The rotating king API integrates with the reward distribution system:

```go
// Example: Getting the current rotating king for reward distribution
func distributeRewards(api *rotatingking.API, stateDB *state.StateDB, blockNumber uint64) {
    currentKing := api.GetCurrentKing()
    
    // Distribute rewards using the current king
    // ... reward distribution logic ...
}
```

## Monitoring Responsibilities

Rotating kings have specific monitoring responsibilities:

| Category | Description |
|----------|-------------|
| Network Health | Monitor network performance and uptime |
| Governance Proposals | Review and participate in governance |
| Consensus Performance | Monitor consensus algorithm health |
| Security Audits | Review security reports and updates |
| Community Engagement | Engage with community and address concerns |
| Development Progress | Track development milestones |
| Node Operations | Monitor node infrastructure |
| Documentation | Keep documentation up-to-date |

## Error Handling

| Error Code | Description |
|------------|-------------|
| `-32000` | Invalid block height |
| `-32001` | No rotating kings configured |
| `-32002` | Address not found in rotation list |
| `-32003` | Rotation interval not set |
| `-32004` | Internal error in rotation manager |

## Security Considerations

1. **Address Validation**: All addresses are validated before use
2. **Rotation Verification**: Rotation logic is verified on each request
3. **Access Control**: API is read-only for security
4. **Rate Limiting**: Requests should be rate-limited to prevent abuse

## Performance Considerations

- All API calls are O(1) or O(n) where n is the number of rotating kings
- Caching is implemented for frequently accessed data
- Rotation calculations are optimized for minimal overhead

## Configuration

The rotating king system is configured during initialization:

```go
config := &rotatingking.Config{
    MainKing:         common.HexToAddress("0x1234..."),
    RotatingKings:    []common.Address{...},
    RotationInterval: 10000, // Blocks
}

manager := rotatingking.NewRotatingKingManager(config)
api := rotatingking.NewAPI(manager)
```

## Common Use Cases

### 1. Monitoring Current King

```go
// Monitor the current king at regular intervals
func monitorKing(api *rotatingking.API) {
    currentKing := api.GetCurrentKing()
    log.Info("Current rotating king", "address", currentKing)
    
    // Check if king is active and participating
    if api.IsKing(currentKing) {
        log.Info("King is valid and active")
    }
}
```

### 2. Preparing for King Rotation

```go
// Prepare for upcoming king rotation
func prepareForRotation(api *rotatingking.API, currentHeight uint64) {
    info := api.GetRotationInfo(currentHeight)
    
    if info["blocksRemaining"].(uint64) < 100 {
        log.Warn("King rotation approaching",
            "currentKing", info["currentKing"],
            "nextKing", info["nextKing"],
            "blocksRemaining", info["blocksRemaining"])
        
        // Notify the next king
        notifyKing(info["nextKing"].(common.Address))
    }
}
```

### 3. Validating King Eligibility

```go
// Validate king eligibility for reward distribution
func validateKingEligibility(api *rotatingking.API, address common.Address) bool {
    // Check if address is a king
    if !api.IsKing(address) {
        return false
    }
    
    // Check if it's the current king
    if api.GetCurrentKing() == address {
        return true
    }
    
    // Check if it's the next king
    if api.GetNextKing() == address {
        return true
    }
    
    return false
}
```

## License

This code is part of the go-tkmchain project and is licensed under the GNU Lesser General Public License v3.0.

## Contributing

Contributions are welcome! Please ensure:
1. Code follows the project's style guidelines
2. All tests pass
3. Documentation is updated
4. Changes are properly tested with both unit and integration tests
