package core

import (
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// The runtime is compiled from contracts/TKMBlockHashAnchors.sol. Keeping the
// exact runtime in the node makes the predeployment reproducible across every
// client and prevents an operator from substituting a different contract.
//
//go:embed tkm_block_hash_anchor_runtime.hex
var tkmBlockHashAnchorRuntimeHex string

var (
	tkmBlockHashAnchorRuntimeOnce sync.Once
	tkmBlockHashAnchorRuntime     []byte
	tkmBlockHashAnchorRuntimeErr  error
)

var errTKMBlockHashAnchorCode = errors.New("TKM block-hash anchor predeployment code is unavailable")

func loadTKMBlockHashAnchorRuntime() ([]byte, error) {
	tkmBlockHashAnchorRuntimeOnce.Do(func() {
		tkmBlockHashAnchorRuntime, tkmBlockHashAnchorRuntimeErr = hex.DecodeString(tkmBlockHashAnchorRuntimeHex)
		if tkmBlockHashAnchorRuntimeErr != nil || len(tkmBlockHashAnchorRuntime) == 0 {
			tkmBlockHashAnchorRuntimeErr = errTKMBlockHashAnchorCode
		}
	})
	return tkmBlockHashAnchorRuntime, tkmBlockHashAnchorRuntimeErr
}

func tkmAnchorSlot(index uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(index))
}

func tkmAnchorMappingSlot(height uint64) common.Hash {
	var key [64]byte
	binary.BigEndian.PutUint64(key[24:32], height)
	binary.BigEndian.PutUint64(key[56:64], 6) // _anchors mapping is storage slot 6.
	return crypto.Keccak256Hash(key[:])
}

func tkmAnchorContractRolling(previous common.Hash, height uint64, blockHash common.Hash) common.Hash {
	var heightWord [32]byte
	binary.BigEndian.PutUint64(heightWord[24:], height)
	domain := crypto.Keccak256Hash([]byte("TKMCHAIN_BLOCK_HASH_ANCHOR_V1"))
	return crypto.Keccak256Hash(domain[:], previous[:], heightWord[:], blockHash[:])
}

// EnsureTKMBlockHashAnchor installs and advances the consensus-owned Solidity
// mirror. It runs as part of block state processing, rather than as a user
// transaction, because Antartical intentionally rejects transparent EVM
// transactions. Reorgs naturally rewind this state with the rest of the trie.
func EnsureTKMBlockHashAnchor(statedb *state.StateDB, config *params.ChainConfig, blockNumber *big.Int, blockTime uint64, parentHash common.Hash) error {
	if statedb == nil || config == nil || config.RandomX == nil || blockNumber == nil || !config.IsAntartical(blockNumber, blockTime) || blockNumber.Sign() == 0 {
		return nil
	}
	runtime, err := loadTKMBlockHashAnchorRuntime()
	if err != nil {
		return err
	}
	address := params.TKMBlockHashAnchorAddress
	if code := statedb.GetCode(address); len(code) != 0 && crypto.Keccak256Hash(code) != crypto.Keccak256Hash(runtime) {
		return errors.New("TKM block-hash anchor predeployment code mismatch")
	}
	if statedb.GetCodeSize(address) == 0 {
		statedb.SetCode(address, runtime, tracing.CodeChangeGenesis)
		owner := config.MainKingAddress
		statedb.SetState(address, tkmAnchorSlot(0), common.BytesToHash(common.LeftPadBytes(owner.Bytes(), common.HashLength)))
	}

	height := blockNumber.Uint64() - 1
	anchorSlot := tkmAnchorMappingSlot(height)
	if existing := statedb.GetState(address, anchorSlot); existing != (common.Hash{}) {
		if existing != parentHash {
			return errors.New("TKM block-hash anchor state conflicts with parent hash")
		}
		return nil
	}
	initialized := statedb.GetState(address, tkmAnchorSlot(1))
	if initialized == (common.Hash{}) {
		statedb.SetState(address, tkmAnchorSlot(1), common.BigToHash(big.NewInt(1)))
		statedb.SetState(address, tkmAnchorSlot(2), common.BigToHash(new(big.Int).SetUint64(height)))
	}
	statedb.SetState(address, anchorSlot, parentHash)
	statedb.SetState(address, tkmAnchorSlot(3), common.BigToHash(new(big.Int).SetUint64(height)))
	count := new(big.Int).SetBytes(statedb.GetState(address, tkmAnchorSlot(4)).Bytes())
	count.Add(count, big.NewInt(1))
	statedb.SetState(address, tkmAnchorSlot(4), common.BigToHash(count))
	previous := statedb.GetState(address, tkmAnchorSlot(5))
	statedb.SetState(address, tkmAnchorSlot(5), tkmAnchorContractRolling(previous, height, parentHash))
	return nil
}
