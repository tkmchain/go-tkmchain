// +build cgo,randomx

package randomx

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/_workspace/randomx/src
#cgo LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build -lrandomx -lstdc++ -lm
#include <stdlib.h>
#include <string.h>
#include "randomx.h"
*/
import "C"
import (
    "fmt"
    "log"
    "sync"
    "time"
    "unsafe"
)

const (
    RandomXEpochLength    = 2048
    RandomXCacheSize      = 256 * 1024 * 1024 // 256MB
    RandomXDatasetSize    = 2 * 1024 * 1024 * 1024 // 2GB
    MaxConcurrentVerifications = 32
)

// RandomX flags matching Monero's implementation
const (
    RANDOMX_FLAG_DEFAULT      C.randomx_flags = 0
    RANDOMX_FLAG_FULL_MEM     C.randomx_flags = 1
    RANDOMX_FLAG_JIT          C.randomx_flags = 2
    RANDOMX_FLAG_HARD_AES     C.randomx_flags = 4
    RANDOMX_FLAG_LARGE_PAGES  C.randomx_flags = 8
    RANDOMX_FLAG_SECURE       C.randomx_flags = 16
)

type RandomXCache struct {
    cache     *C.randomx_cache
    dataset   *C.randomx_dataset
    vmFull    *C.randomx_vm
    vmLight   *C.randomx_vm
    seedHash  []byte
    epoch     uint64
    createdAt time.Time
    lastUsed  time.Time
    mu        sync.RWMutex
}

type RandomXManager struct {
    mainCache       *RandomXCache
    secondaryCache  *RandomXCache
    mainSeedHash    []byte
    secondarySeedHash []byte
    mu              sync.RWMutex
    semaphore       chan struct{}
    maxThreads      int
}

func NewRandomXManager() *RandomXManager {
    return &RandomXManager{
        semaphore: make(chan struct{}, MaxConcurrentVerifications),
        maxThreads: 4,
    }
}

func (m *RandomXManager) SetMaxThreads(threads int) {
    m.maxThreads = threads
}

// GetCache implements Monero-style dual-cache system
func (m *RandomXManager) GetCache(epoch uint64, seedHash []byte) (*RandomXCache, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Check if this seed hash matches main cache
    if m.mainCache != nil && len(m.mainSeedHash) > 0 && bytesEqual(m.mainSeedHash, seedHash) {
        m.mainCache.updateLastUsed()
        return m.mainCache, nil
    }

    // Check if this seed hash matches secondary cache
    if m.secondaryCache != nil && len(m.secondarySeedHash) > 0 && bytesEqual(m.secondarySeedHash, seedHash) {
        m.secondaryCache.updateLastUsed()
        return m.secondaryCache, nil
    }

    // Need to create new cache (slow path)
    log.Printf("Creating new RandomX cache for seed: %x", seedHash[:8])
    startTime := time.Now()

    cache, err := m.createCache(seedHash)
    if err != nil {
        return nil, err
    }

    // Promote secondary to main if needed
    if m.secondaryCache != nil {
        // Move secondary to main, create new secondary
        m.mainCache = m.secondaryCache
        m.mainSeedHash = m.secondarySeedHash
    }

    m.secondaryCache = cache
    m.secondarySeedHash = seedHash

    log.Printf("RandomX cache created for seed %x in %v", seedHash[:8], time.Since(startTime))

    return cache, nil
}

func (m *RandomXManager) createCache(seedHash []byte) (*RandomXCache, error) {
    // Get flags (disable FULL_MEM if not enough memory)
    flags := m.getFlags()

    // Allocate cache
    cCache := C.randomx_alloc_cache(flags)
    if cCache == nil {
        // Try without large pages
        flags &^= RANDOMX_FLAG_LARGE_PAGES
        cCache = C.randomx_alloc_cache(flags)
        if cCache == nil {
            return nil, fmt.Errorf("failed to allocate RandomX cache")
        }
    }

    // Initialize cache with seed hash
    seedPtr := unsafe.Pointer(&seedHash[0])
    C.randomx_init_cache(cCache, seedPtr, C.size_t(len(seedHash)))

    // Try to allocate dataset for full mode (better performance)
    var dataset *C.randomx_dataset
    if m.shouldUseFullMem() {
        dataset = C.randomx_alloc_dataset(flags)
        if dataset != nil {
            log.Printf("RandomX dataset allocated for full mode")
            // Initialize dataset using multiple threads
            m.initDataset(dataset, cCache)
        }
    }

    cache := &RandomXCache{
        cache:     cCache,
        dataset:   dataset,
        seedHash:  seedHash,
        createdAt: time.Now(),
        lastUsed:  time.Now(),
    }

    return cache, nil
}

func (m *RandomXManager) initDataset(dataset *C.randomx_dataset, cache *C.randomx_cache) {
    itemCount := int(C.randomx_dataset_item_count())
    numThreads := m.maxThreads
    if numThreads < 1 {
        numThreads = 1
    }
    if numThreads > 4 {
        numThreads = 4
    }

    delta := itemCount / numThreads
    start := 0

    var wg sync.WaitGroup
    for i := 0; i < numThreads; i++ {
        wg.Add(1)
        go func(threadId int, startIdx, count int) {
            defer wg.Done()
            C.randomx_init_dataset(dataset, cache, C.uint32_t(startIdx), C.uint32_t(count))
        }(i, start, delta)
        start += delta
    }
    wg.Wait()

    log.Printf("RandomX dataset initialized with %d threads", numThreads)
}

func (m *RandomXManager) getFlags() C.randomx_flags {
    flags := RANDOMX_FLAG_HARD_AES | RANDOMX_FLAG_JIT
    
    // Check if we should use large pages
    if m.canUseLargePages() {
        flags |= RANDOMX_FLAG_LARGE_PAGES
    }
    
    return flags
}

func (m *RandomXManager) canUseLargePages() bool {
    // Check if large pages are available
    // On Linux, check /proc/meminfo for HugePages_Total
    return true // Implement based on your system
}

func (m *RandomXManager) shouldUseFullMem() bool {
    // Check if we have enough memory for full dataset (2GB)
    // Check MONERO_RANDOMX_FULL_MEM environment variable
    return true // Configure based on your needs
}

func (c *RandomXCache) ComputeHash(seedHash, nonce []byte) ([]byte, error) {
    if len(seedHash) != 32 {
        return nil, fmt.Errorf("invalid seed hash length: %d, expected 32", len(seedHash))
    }
    if len(nonce) != 8 {
        return nil, fmt.Errorf("invalid nonce length: %d, expected 8", len(nonce))
    }

    c.mu.Lock()
    defer c.mu.Unlock()

    // Prepare input: 32 bytes seedHash + 8 bytes nonce = 40 bytes
    input := make([]byte, 40)
    copy(input[:32], seedHash)
    copy(input[32:40], nonce[:8])

    output := make([]byte, 32)

    // Try to use full VM if available (faster)
    if c.dataset != nil {
        if c.vmFull == nil {
            flags := RANDOMX_FLAG_FULL_MEM | RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES
            c.vmFull = C.randomx_create_vm(flags, nil, c.dataset)
            if c.vmFull == nil {
                // Fall back to light mode
                c.vmFull = nil
            }
        }
        if c.vmFull != nil {
            C.randomx_calculate_hash(c.vmFull, unsafe.Pointer(&input[0]), C.size_t(len(input)), unsafe.Pointer(&output[0]))
            return output, nil
        }
    }

    // Light mode (slower but uses less memory)
    if c.vmLight == nil {
        flags := RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES
        c.vmLight = C.randomx_create_vm(flags, c.cache, nil)
        if c.vmLight == nil {
            return nil, fmt.Errorf("failed to create RandomX VM")
        }
    }

    C.randomx_calculate_hash(c.vmLight, unsafe.Pointer(&input[0]), C.size_t(len(input)), unsafe.Pointer(&output[0]))
    
    return output, nil
}

func (c *RandomXCache) updateLastUsed() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.lastUsed = time.Now()
}

func (c *RandomXCache) Close() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if c.vmFull != nil {
        C.randomx_destroy_vm(c.vmFull)
        c.vmFull = nil
    }
    if c.vmLight != nil {
        C.randomx_destroy_vm(c.vmLight)
        c.vmLight = nil
    }
    if c.dataset != nil {
        C.randomx_release_dataset(c.dataset)
        c.dataset = nil
    }
    if c.cache != nil {
        C.randomx_release_cache(c.cache)
        c.cache = nil
    }
}

func bytesEqual(a, b []byte) bool {
    if len(a) != len(b) {
        return false
    }
    for i := range a {
        if a[i] != b[i] {
            return false
        }
    }
    return true
}
