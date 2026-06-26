// Copyright 2026 The go-ethereum Authors

package randomx

import (
    "math/big"
    
//    "github.com/ethereum/go-ethereum/ethdb"
    "github.com/ethereum/go-ethereum/log"
)

var (
    // Key for storing the current difficulty in the database
    difficultyStoreKey = []byte("randomx-current-difficulty")
    // Key for storing the last block number where difficulty was stored
    blockNumberStoreKey = []byte("randomx-last-block-number")
)

// StoreDifficulty saves the current difficulty and block number to the database
func (rx *RandomX) StoreDifficulty(blockNumber uint64, difficulty *big.Int) error {
    if rx.db == nil {
        log.Warn("No database available, difficulty not persisted",
            "block", blockNumber,
            "difficulty", difficulty)
        return nil
    }

    // Store difficulty
    if err := rx.db.Put(difficultyStoreKey, difficulty.Bytes()); err != nil {
        log.Error("Failed to store difficulty", "error", err)
        return err
    }

    // Store block number
    blockNumBytes := make([]byte, 8)
    for i := 0; i < 8; i++ {
        blockNumBytes[7-i] = byte(blockNumber >> (8 * uint(i)))
    }
    if err := rx.db.Put(blockNumberStoreKey, blockNumBytes); err != nil {
        log.Error("Failed to store block number", "error", err)
        return err
    }

    log.Debug("Stored difficulty in database",
        "block", blockNumber,
        "difficulty", difficulty)
    
    return nil
}

// LoadStoredDifficulty retrieves the last stored difficulty from the database
func (rx *RandomX) LoadStoredDifficulty() (*big.Int, uint64) {
    if rx.db == nil {
        log.Warn("No database available, cannot load stored difficulty")
        return nil, 0
    }

    // Load difficulty
    diffData, err := rx.db.Get(difficultyStoreKey)
    if err != nil {
        log.Debug("No stored difficulty found in database")
        return nil, 0
    }

    if len(diffData) == 0 {
        return nil, 0
    }

    difficulty := new(big.Int).SetBytes(diffData)

    // Load block number
    blockNumData, err := rx.db.Get(blockNumberStoreKey)
    if err != nil {
        log.Warn("Stored difficulty found but block number missing",
            "difficulty", difficulty)
        return difficulty, 0
    }

    blockNumber := uint64(0)
    for i := 0; i < 8 && i < len(blockNumData); i++ {
        blockNumber |= uint64(blockNumData[i]) << (8 * uint(7-i))
    }

    log.Debug("Loaded stored difficulty",
        "block", blockNumber,
        "difficulty", difficulty)
    
    return difficulty, blockNumber
}
