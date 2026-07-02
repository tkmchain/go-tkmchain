
# Go Tkmchain

Golang execution layer implementation of the Tkmchain protocol with **RandomX PoW** and **Rotating Kings (RK)** governance.

[![API Reference](
https://pkg.go.dev/badge/github.com/tkmchain/go-tkmchain
)](https://pkg.go.dev/github.com/ethereum/go-ethereum?tab=doc)
[![Go Report Card](https://goreportcard.com/badge/github.com/ethereum/go-ethereum)](https://goreportcard.com/report/github.com/ethereum/go-ethereum)
[![Travis](https://app.travis-ci.com/ethereum/go-ethereum.svg?branch=master)](https://app.travis-ci.com/github/ethereum/go-ethereum)
[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://discord.gg/vbJF7PrXF8)
[![Twitter](https://img.shields.io/twitter/follow/go_tkmchain)](https://x.com/go_tkmchain)

Automated builds are available for stable releases and the unstable master branch. Binary archives are published at https://gtkm.tkmchain.site/downloads/.

---

## Rotating Kings (RK) Governance

Tkmchain features a unique **Rotating Kings** governance system with a 10/40/50 reward split:

| Role | Share | Description |
|------|-------|-------------|
|    **Main King** | 10% | Permanent king, network leadership |
|    **Rotating King** | 40% | Rotates every 100 blocks, decentralized governance |
| ⛏️ **Miner** | 50% | Secures the network via RandomX mining |

### Reward Distribution

```
Block Reward = 200 ANTD (halving every ~4 years)
├── Main King:     20 ANTD (10%)
├── Rotating King: 80 ANTD (40%)
└── Miner:        100 ANTD (50%)
```

### King Registration

To become a Rotating King:
1. Hold at least **50,001 ANTD**: 50,000 ANTD is locked as the Rotating King stake and 1 ANTD is reserved as the registration fee.
2. Register your address with the `rk_add` RPC method.
3. Remain funded while the address is active. Registered kings are removed when the stake lock expires or the address no longer satisfies the funding requirement.
4. Kings rotate every 100 blocks by default. Each king serves for one rotation period before the next registered address receives the Rotating King slot.

### Rotating King RPC API

The Rotating King service is exposed through the `king`, `mainking`, `rk`, and `rotatingking` RPC namespaces. The short `rk` namespace is intended for operational scripts, while `mainking` is used for checkpoint submission. Enable the namespaces on HTTP or WebSocket explicitly when serving remote RPC:

```shell
gtkm --http --http.addr 127.0.0.1 --http.api eth,net,web3,rk,mainking,randomx,miner
```

Common calls:

| Method | Parameters | Description |
|--------|------------|-------------|
| `rk_add` | `address` | Registers a funded address as a Rotating King candidate and returns its status. |
| `rk_list` | none | Lists registered and locked Rotating King addresses with status metadata. |
| `rk_status` | `address` | Returns status for one address, including lock, rotation, and reward fields. |
| `rk_getKingStats` | optional ignored value | Returns current king, next king, total registered kings, rotation height, and per-king statuses. |
| `rotatingking_getInfo` | none | Returns the current schedule, main king, current king, next king, and rotation interval. |
| `rotatingking_getCurrentKing` | none | Returns the address assigned to the current block's Rotating King slot. |
| `rotatingking_getRotationHistory` | optional `limit` | Returns recent rotation boundaries derived from chain height. |
| `mainking_addCheckpoint` | `number`, `hash` | Adds and broadcasts a checkpoint after the Main King node verifies the local block hash. |

Example JSON-RPC requests:

```shell
# Register a funded Rotating King address
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"rk_add","params":["0xYourKingAddress"]}'

# Inspect the current schedule and all registered kings
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"rk_getKingStats","params":[null]}'

# Add a checkpoint from the Main King node
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"mainking_addCheckpoint","params":[12345,"0xBlockHash"]}'
```

---

## ⛏️ RandomX (RX) Mining

Tkmchain uses **RandomX PoW** - an ASIC-resistant mining algorithm optimized for CPUs:

### Mining Features

- **Algorithm**: Monero's RandomX (CPU-friendly)
- **Block Time**: 120 seconds (2 minutes)
- **Difficulty Adjustment**: 2x cap per block (smooth, self-correcting)
- **Epoch Length**: 2,048 blocks (~2.8 days)

### Mining Commands

Build `gtkm` with RandomX support before mining:

```shell
make gtkm
```

Start from a fully synced node, set the reward address with `--miner.etherbase`, and select the number of CPU mining threads with `--miner.threads`:

```shell
# Start CPU mining
gtkm --mine --miner.threads=2 --miner.etherbase=0xYourAddress

# Configure mining threads
gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress

# Solo mining mode on a full node
gtkm --mine --miner.etherbase=0xYourAddress --syncmode=full

# Mining with boost (JIT + AES)
gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress --randomx.boost
```

For external RandomX miners, run a node that exposes mining work. `gtkm` provides the standard `miner_*` calls, a `randomx_*` namespace, and a local stratum bridge on `127.0.0.1:3333` when external mining is started by the miner service. Keep stratum bound to localhost unless you place it behind trusted network controls.

```shell
# Node with HTTP work APIs for a local external miner
gtkm --syncmode=full --http --http.addr 127.0.0.1 --http.api eth,net,web3,miner,randomx \
  --miner.etherbase=0xYourAddress
```

### RandomX Mining RPC API

The mining work tuple is `[sealHash, seedHash, target, blockHeight]`. External miners hash the seal hash with a nonce using the RandomX cache selected by `seedHash`, then submit the nonce and digest back to the node.

| Method | Parameters | Description |
|--------|------------|-------------|
| `miner_getWork` | none | Returns `[sealHash, seedHash, target, blockHeight]` for external miners. |
| `miner_submitWork` | `nonce`, `sealHash`, `digest` | Submits a proof-of-work solution. Returns `true` when accepted. |
| `miner_getSeedHash` | none | Returns the RandomX seed hash for the next block. |
| `randomx_getSeedHash` | optional `blockNumber` | Returns the seed hash for the next block or for the supplied block number. |
| `randomx_getSeedHashForBlock` | `blockNumber` | Returns the seed hash for a specific block number. |
| `randomx_getWork` | none | Returns the same external-mining work tuple as `miner_getWork`. |
| `randomx_submitWork` | `nonce`, `sealHash`, `digest` | Submits typed nonce/hash/digest values. |
| `randomx_submitWorkRaw` | `nonceHex`, `sealHashHex`, `digestHex` | Submits hex strings directly from mining adapters. |
| `randomx_getCurrentHeight` | none | Returns the current canonical block height. |
| `randomx_getHashrate` | none | Returns the node miner hashrate counter. |

Example work loop calls:

```shell
# Fetch work
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"randomx_getWork","params":[]}'

# Fetch the seed hash for block 2048
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"randomx_getSeedHashForBlock","params":[2048]}'

# Submit a solution returned by an external miner
curl -s http://127.0.0.1:8545 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"randomx_submitWorkRaw","params":["0xNonce8Bytes","0xSealHash","0xDigest"]}'
```

### Difficulty Adjustment Formula

```go
ratio = (targetTime * 100) / actualTime
if ratio > 200 { ratio = 200 }  // Max 2x increase
if ratio < 50 { ratio = 50 }    // Min 0.5x decrease
newDiff = currentDiff * ratio / 100
```

---

## Building the Source

For prerequisites and detailed build instructions please read the [Installation Instructions](https://gtkm.tkmchain.site/docs/getting-started/installing-gtkm).

Building `gtkm` requires both a Go (version 1.23 or later) and a C compiler. You can install them using your favourite package manager. Once the dependencies are installed, run:

```shell
make gtkm
```

or, to build the full suite of utilities:

```shell
make all
```

### Cross-Platform Builds

Install mingw for Windows cross-compilation:

```shell
sudo apt-get install gcc-mingw-w64-x86-64 gcc-mingw-w64-i686

# Build Windows 64-bit only
make cross-windows

# Build all Windows architectures
make cross-windows-all

# Build all platforms (Linux, Windows, macOS)
make cross-all-all
```

### Cross-Platform Output

```
build/dist/
├── windows/
│   ├── gtkm-windows-amd64.exe
│   └── gtkm-windows-386.exe
├── darwin/
│   ├── gtkm-darwin-amd64
│   └── gtkm-darwin-arm64
└── linux/
    ├── gtkm-linux-amd64
    ├── gtkm-linux-386
    └── gtkm-linux-arm64
```

---

## Executables

The go-tkmchain project comes with several wrappers/executables found in the `cmd` directory.

| Command | Description |
| :-----: | ----------- |
| **`gtkm`** | Our main Tkmchain CLI client. It is the entry point into the Tkmchain network (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It can be used by other processes as a gateway into the Tkmchain network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `gtkm --help` and the [CLI page](https://gtkm.tkmchain.site/docs/fundamentals/command-line-options) for command line options. |
| `clef` | Stand-alone signing tool, which can be used as a backend signer for `gtkm`. |
| `devp2p` | Utilities to interact with nodes on the networking layer, without running a full blockchain. |
| `abigen` | Source code generator to convert Tkmchain contract definitions into easy-to-use, compile-time type-safe Go packages. It operates on plain [Tkmchain contract ABIs](https://docs.soliditylang.org/en/develop/abi-spec.html) with expanded functionality if the contract bytecode is also available. However, it also accepts Solidity source files, making development much more streamlined. Please see our [Native DApps](https://gtkm.tkmchain.site/docs/developers/dapp-developer/native-bindings) page for details. |
| `evm` | Developer utility version of the EVM (Tkmchain Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug run`). |
| `rlpdump` | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp)) dumps (data encoding used by the Tkmchain protocol both network as well as consensus wise) to user-friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`). |

---

## Running `gtkm`

Going through all the possible command line flags is out of scope here (please consult our [CLI Wiki page](https://gtkm.tkmchain.site/docs/fundamentals/command-line-options)), but we've enumerated a few common parameter combos to get you up to speed quickly on how you can run your own `gtkm` instance.

### Hardware Requirements

**Minimum:**
- CPU with 4+ cores (AES-NI support recommended for RandomX)
- 8GB RAM
- 1TB free storage space to sync the Mainnet
- 8 MBit/sec download Internet service

**Recommended:**
- Fast CPU with 8+ cores (AES-NI, AVX2 support)
- 16GB+ RAM
- High-performance SSD with at least 1TB of free space
- 25+ MBit/sec download Internet service

### Full Node on the Main Tkmchain Network

By far the most common scenario is people wanting to simply interact with the Tkmchain network: create accounts; transfer funds; deploy and interact with contracts. For this particular use case, the user doesn't care about years-old historical data, so we can sync quickly to the current state of the network. To do so:

```shell
$ gtkm console
```

This command will:
- Start `gtkm` in snap sync mode (default, can be changed with the `--syncmode` flag), causing it to download more data in exchange for avoiding processing the entire history of the Tkmchain network, which is very CPU intensive.
- Start the built-in interactive [JavaScript console](https://gtkm.tkmchain.site/docs/interacting-with-gtkm/javascript-console), (via the trailing `console` subcommand) through which you can interact using [`web3` methods](https://github.com/ChainSafe/web3.js/blob/0.20.7/DOCUMENTATION.md) (note: the `web3` version bundled within `gtkm` is very old, and not up to date with official docs), as well as `gtkm`'s own [management APIs](https://gtkm.tkmchain.site/docs/interacting-with-gtkm/rpc). This tool is optional and if you leave it out you can always attach it to an already running `gtkm` instance with `gtkm attach`.

### Mining with RandomX

```shell
# Start mining with 2 threads
$ gtkm --mine --miner.threads=2 --miner.etherbase=0xYourAddress

# Start mining with boost mode (JIT + AES)
$ gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress --randomx.boost

# Configure RandomX cache size
$ gtkm --mine --randomx.cache-size=256 --randomx.dataset-size=2
```

### A Full Node on the Holesky Test Network

Transitioning towards developers, if you'd like to play around with creating Tkmchain contracts, you almost certainly would like to do that without any real money involved until you get the hang of the entire system. In other words, instead of attaching to the main network, you want to join the **test** network with your node, which is fully equivalent to the main network, but with play-Ether only.

```shell
$ gtkm --holesky console
```

The `console` subcommand has the same meaning as above and is equally useful on the testnet too.

Specifying the `--holesky` flag, however, will reconfigure your `gtkm` instance a bit:
- Instead of connecting to the main Tkmchain network, the client will connect to the Holesky test network, which uses different P2P bootnodes, different network IDs and genesis states.
- Instead of using the default data directory (`~/.tkmchain` on Linux for example), `gtkm` will nest itself one level deeper into a `holesky` subfolder (`~/.tkmchain/holesky` on Linux). Note, on OSX and Linux this also means that attaching to a running testnet node requires the use of a custom endpoint since `gtkm attach` will try to attach to a production node endpoint by default, e.g., `gtkm attach <datadir>/holesky/gtkm.ipc`. Windows users are not affected by this.

*Note: Although some internal protective measures prevent transactions from crossing over between the main network and test network, you should always use separate accounts for play and real money. Unless you manually move accounts, `gtkm` will by default correctly separate the two networks and will not make any accounts available between them.*

### Configuration

As an alternative to passing the numerous flags to the `gtkm` binary, you can also pass a configuration file via:

```shell
$ gtkm --config /path/to/your_config.toml
```

To get an idea of how the file should look like you can use the `dumpconfig` subcommand to export your existing configuration:

```shell
$ gtkm --your-favourite-flags dumpconfig
```

#### Docker Quick Start

One of the quickest ways to get Tkmchain up and running on your machine is by using Docker:

```shell
docker run -d --name tkmchain-node -v /Users/alice/tkmchain:/root \
           -p 8545:8545 -p 3000:3000 \
           tkmchain/client-go
```

This will start `gtkm` in snap-sync mode with a DB memory allowance of 1GB, as the above command does. It will also create a persistent volume in your home directory for saving your blockchain as well as map the default ports. There is also an `alpine` tag available for a slim version of the image.

Do not forget `--http.addr 0.0.0.0`, if you want to access RPC from other containers and/or hosts. By default, `gtkm` binds to the local interface and RPC endpoints are not accessible from the outside.

### Programmatically Interfacing `gtkm` Nodes

As a developer, sooner rather than later you'll want to start interacting with `gtkm` and the Tkmchain network via your own programs and not manually through the console. To aid this, `gtkm` has built-in support for a JSON-RPC based APIs ([standard APIs](https://ethereum.org/en/developers/docs/apis/json-rpc/) and [`gtkm` specific APIs](https://gtkm.tkmchain.site/docs/interacting-with-gtkm/rpc)). These can be exposed via HTTP, WebSockets and IPC (UNIX sockets on UNIX based platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by `gtkm`, whereas the HTTP and WS interfaces need to manually be enabled and only expose a subset of APIs due to security reasons. These can be turned on/off and configured as you'd expect.

**HTTP based JSON-RPC API options:**
- `--http` Enable the HTTP-RPC server
- `--http.addr` HTTP-RPC server listening interface (default: `localhost`)
- `--http.port` HTTP-RPC server listening port (default: `8545`)
- `--http.api` API's offered over the HTTP-RPC interface (default: `tkm,net,web3,miner,randomx`)
- `--http.corsdomain` Comma separated list of domains from which to accept cross-origin requests (browser enforced)
- `--ws` Enable the WS-RPC server
- `--ws.addr` WS-RPC server listening interface (default: `localhost`)
- `--ws.port` WS-RPC server listening port (default: `8546`)
- `--ws.api` API's offered over the WS-RPC interface (default: `tkm,net,web3,miner,randomx`)
- `--ws.origins` Origins from which to accept WebSocket requests
- `--ipcdisable` Disable the IPC-RPC server
- `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

#### Rotating Kings API Methods

```javascript
// Get current Rotating King
web3.eth.getRotatingKing().then(console.log)

// Get Main King
web3.tkm.getMainKing().then(console.log)

// Get reward distribution
web3.tkm.getRewardDistribution().then(console.log)

// Register as King
web3.tkm.registerAsKing().then(console.log)
```

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to connect via HTTP, WS or IPC to a `gtkm` node configured with the above flags and you'll need to speak [JSON-RPC](https://www.jsonrpc.org/specification) on all transports. You can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based transport before doing so! Hackers on the internet are actively trying to subvert Tkmchain nodes with exposed APIs! Further, all browser tabs can access locally running web servers, so malicious web pages could try to subvert locally available APIs!**

### Operating a Private Network

Maintaining your own private network is more involved as a lot of configurations taken for granted in the official networks need to be manually set up.

Unfortunately since [the Merge](https://ethereum.org/en/roadmap/merge/) it is no longer possible to easily set up a network of gtkm nodes without also setting up a corresponding beacon chain.

There are three different solutions depending on your use case:
- If you are looking for a simple way to test smart contracts from go in your CI, you can use the [Simulated Backend](https://gtkm.tkmchain.site/docs/developers/dapp-developer/native-bindings#blockchain-simulator).
- If you want a convenient single node environment for testing, you can use our [Dev Mode](https://gtkm.tkmchain.site/docs/developers/dapp-developer/dev-mode).
- If you are looking for a multiple node test network, you can set one up quite easily with [Kurtosis](https://gtkm.tkmchain.site/docs/fundamentals/kurtosis).

---

## Configuration Reference

### RandomX Mining Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `--mine` | Enable RandomX CPU mining | `false` |
| `--miner.threads` | Number of mining threads | `0` (auto) |
| `--miner.etherbase` | Address to receive mining rewards | First account |
| `--randomx.cache-size` | Cache size in MB | `256` |
| `--randomx.dataset-size` | Dataset size in GB | `2` |
| `--randomx.epoch-length` | Blocks per epoch | `2048` |
| `--randomx.min-memory` | Minimum memory in GB | `4` |
| `--randomx.boost` | Enable JIT + AES acceleration | `false` |

### Rotating Kings Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `--king.main` | Main king address (10% rewards) | `0x...` |
| `--king.rotating` | Rotating king addresses (40% rewards) | `0x...` |
| `--king.rotation-interval` | Blocks between rotations | `100` |

---

## Contribution

Thank you for considering helping out with the source code! We welcome contributions from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to go-tkmchain, please fork, fix, commit and send a pull request for the maintainers to review and merge into the main code base. If you wish to submit more complex changes though, please check up with the core devs first on [our Discord Server](https://discord.gg/invite/nthXNEv) to ensure those changes are in line with the general philosophy of the project and/or get some early feedback which can make both your efforts much lighter as well as our review and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:
- Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/doc/cmd/gofmt/)).
- Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary) guidelines.
- Pull requests need to be based on and opened against the `master` branch.
- Commit messages should be prefixed with the package(s) they modify.
  - E.g. "tkm, rpc: make trace configs optional"

Please see the [Developers' Guide](https://gtkm.tkmchain.site/docs/developers/gtkm-developer/dev-guide) for more details on configuring your environment, managing project dependencies, and testing procedures.

### Contributing to gtkm.tkmchain.site

For contributions to the [go-tkmchain website](https://gtkm.tkmchain.site), please checkout and raise pull requests against the `website` branch. For more detailed instructions please see the `website` branch [README](https://github.com/tkmchain/go-tkmchain/tree/website#readme) or the [contributing](https://gtkm.tkmchain.site/docs/developers/gtkm-developer/contributing) page of the website.

---

## License

The go-tkmchain library (i.e. all code outside of the `cmd` directory) is licensed under the [GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also included in our repository in the `COPYING.LESSER` file.

The go-tkmchain binaries (i.e. all code inside of the `cmd` directory) are licensed under the [GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included in our repository in the `COPYING` file.

---

## Quick Reference

```shell
# Build
make gtkm                    # Build only gtkm
make all                     # Build all tools
make cross-windows           # Build Windows 64-bit
make cross-linux             # Build Linux
make cross-all-all           # Build all platforms

# Run with mining
gtkm --mine --miner.threads=2 --miner.etherbase=0xYourAddress

# Run with Rotating Kings
gtkm --king.main=0x... --king.rotating=0x... --mine

# Check Rotating Kings
gtkm attach --exec "tkm.getRotatingKing()"

# RandomX mining with boost
gtkm --mine --miner.threads=4 --miner.etherbase=0xYourAddress --randomx.boost

# Full node with RPC enabled
gtkm --http --http.api "tkm,net,web3,miner,randomx" --mine --miner.threads=2

# Export configuration
gtkm dumpconfig > config.toml

# Run with config file
gtkm --config config.toml
```
