package main

import (
        "encoding/binary"
        "encoding/hex"
        "fmt"
        "math/big"
        "runtime"
        "sync/atomic"
        "time"

        "github.com/ethereum/go-ethereum/consensus/randomx"
        "github.com/ethereum/go-ethereum/rpc"
        "github.com/ethereum/go-ethereum/common"
)

type Work struct {
        SealHash []byte
        SeedHash []byte
        Target   *big.Int
        Height   uint64
}

type Miner struct {
        client   *rpc.Client
        threads  int
        stop     int32
        hashes   uint64
        accepted uint64
        rejected uint64
        engine   *randomx.RandomX
}

func NewMiner(rpcURL string, threads int) (*Miner, error) {
        client, err := rpc.Dial(rpcURL)
        if err != nil {
                return nil, err
        }

        // Create RandomX engine directly
        config := randomx.DefaultConfig()
        engine, err := randomx.New(config, threads, common.Address{}, nil)
        if err != nil {
                return nil, err
        }

        return &Miner{
                client:  client,
                threads: threads,
                engine:  engine,
        }, nil
}

func (m *Miner) GetWork() (*Work, error) {
        var work [4]string
        err := m.client.Call(&work, "randomx_getWork")
        if err != nil {
                return nil, err
        }

        sealHash, _ := hex.DecodeString(work[0][2:])
        seedHash, _ := hex.DecodeString(work[1][2:])
        target, _ := hex.DecodeString(work[2][2:])

        targetBig := new(big.Int).SetBytes(target)
        height := uint64(0)
        fmt.Sscanf(work[3], "0x%x", &height)

        return &Work{
                SealHash: sealHash,
                SeedHash: seedHash,
                Target:   targetBig,
                Height:   height,
        }, nil
}

func (m *Miner) SubmitWork(nonce uint64, sealHash []byte, mixDigest []byte) bool {
        nonceHex := fmt.Sprintf("0x%016x", nonce)
        sealHex := "0x" + hex.EncodeToString(sealHash)
        mixHex := "0x" + hex.EncodeToString(mixDigest)
        
        var result bool
        err := m.client.Call(&result, "randomx_submitWorkRaw", nonceHex, sealHex, mixHex)
        return err == nil && result
}

func (m *Miner) Mine() {
        fmt.Printf("�� RandomX Go Miner Started\n")
        fmt.Printf("   Threads: %d\n", m.threads)
        
        // Get initial work
        work, err := m.GetWork()
        if err != nil {
                fmt.Printf("Failed to get work: %v\n", err)
                return
        }
        
        fmt.Printf("RandomX ready, mining...\n")
        fmt.Printf("   Height: %d\n", work.Height)
        targetStr := work.Target.String()
        if len(targetStr) > 16 {
                targetStr = targetStr[:16]
        }
        fmt.Printf("   Target: %s...\n", targetStr)
        
        for i := 0; i < m.threads; i++ {
                go m.mineThread(i, work)
        }
        
        go m.statsReporter()
        select {}
}

func (m *Miner) mineThread(id int, initialWork *Work) {
        input := make([]byte, 40)
        output := make([]byte, 32)
        copy(input[:32], initialWork.SealHash)
        
        nonce := uint64(id) << 56
        localHashes := uint64(0)
        currentWork := initialWork
        currentSeedHash := currentWork.SeedHash
        
        fmt.Printf("�� Thread %d started\n", id)
        
        for atomic.LoadInt32(&m.stop) == 0 {
                // Refresh work periodically
                if localHashes%10000 == 0 {
                        newWork, err := m.GetWork()
                        if err == nil {
                                // Check if seed changed
                                if string(newWork.SeedHash) != string(currentSeedHash) {
                                        currentSeedHash = newWork.SeedHash
                                        fmt.Printf("�� Thread %d: Seed updated for height %d\n", id, newWork.Height)
                                }
                                if string(newWork.SealHash) != string(currentWork.SealHash) {
                                        currentWork = newWork
                                        copy(input[:32], currentWork.SealHash)
                                        targetStr := currentWork.Target.String()
                                        if len(targetStr) > 16 {
                                                targetStr = targetStr[:16]
                                        }
                                        fmt.Printf("�� Thread %d: New work - Height: %d, Target: %s...\n", 
                                                id, currentWork.Height, targetStr)
                                }
                        }
                }
                
                // Update nonce (little-endian)
                binary.LittleEndian.PutUint64(input[32:], nonce)
                
                // Need to get VM from engine to calculate hash
                // For now, we'll skip hashing and just increment
                // TODO: Add method to get VM from RandomX engine
                
                // Placeholder - actual hashing would go here
                for i := 0; i < 32; i++ {
                        output[i] = byte(nonce >> (i % 8))
                }
                
                localHashes++
                atomic.AddUint64(&m.hashes, 1)
                
                // For testing, submit every 1000 hashes
                if localHashes%1000 == 0 {
                        fmt.Printf("\n�� Thread %d: TEST SHARE\n", id)
                        if m.SubmitWork(nonce, currentWork.SealHash, output) {
                                atomic.AddUint64(&m.accepted, 1)
                                fmt.Printf("   ✅ ACCEPTED!\n")
                        } else {
                                atomic.AddUint64(&m.rejected, 1)
                                fmt.Printf("   ❌ REJECTED!\n")
                        }
                }
                
                nonce++
                if nonce == 0 {
                        nonce = 1
                }
        }
}

func (m *Miner) statsReporter() {
        ticker := time.NewTicker(10 * time.Second)
        var lastHashes uint64
        lastTime := time.Now()
        
        for range ticker.C {
                if atomic.LoadInt32(&m.stop) == 1 {
                        return
                }
                
                current := atomic.LoadUint64(&m.hashes)
                elapsed := time.Since(lastTime).Seconds()
                hashrate := float64(current-lastHashes) / elapsed
                
                fmt.Printf("\n�� STATS | Hashrate: %.2f H/s | Accepted: %d | Rejected: %d | Total: %d\n",
                        hashrate, atomic.LoadUint64(&m.accepted), atomic.LoadUint64(&m.rejected),
                        atomic.LoadUint64(&m.accepted)+atomic.LoadUint64(&m.rejected))
                
                lastHashes = current
                lastTime = time.Now()
        }
}

func main() {
        threads := runtime.NumCPU()
        if threads > 4 {
                threads = 4
        }
        
        fmt.Println("╔═══════════════════════════════════════════════════╗")
        fmt.Println("║     RandomX Go External Miner v1.0               ║")
        fmt.Println("╚═══════════════════════════════════════════════════╝")
        
        miner, err := NewMiner("http://localhost:8545", threads)
        if err != nil {
                fmt.Printf("Failed to create miner: %v\n", err)
                return
        }
        
        miner.Mine()
}
