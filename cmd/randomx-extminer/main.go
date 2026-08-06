package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/consensus/randomx"
	"github.com/ethereum/go-ethereum/rpc"
)

type work struct {
	sealHash []byte
	seedHash []byte
	target   *big.Int
	height   uint64
}

type miner struct {
	client *rpc.Client
	vmMu   sync.Mutex
	cache  *randomx.Cache
	vm     *randomx.VM
	seed   []byte

	hashes   uint64
	accepted uint64
	rejected uint64
}

func newMiner(rpcURL string) (*miner, error) {
	client, err := rpc.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	return &miner{client: client}, nil
}

func (m *miner) close() {
	m.vmMu.Lock()
	defer m.vmMu.Unlock()
	if m.vm != nil {
		m.vm.Close()
	}
	if m.cache != nil {
		m.cache.Close()
	}
}

func (m *miner) getWork() (*work, error) {
	var payload [4]string
	if err := m.client.Call(&payload, "randomx_getWork"); err != nil {
		return nil, err
	}
	sealHash, err := decodeHex(payload[0])
	if err != nil {
		return nil, fmt.Errorf("decode seal hash: %w", err)
	}
	seedHash, err := decodeHex(payload[1])
	if err != nil {
		return nil, fmt.Errorf("decode seed hash: %w", err)
	}
	targetBytes, err := decodeHex(payload[2])
	if err != nil {
		return nil, fmt.Errorf("decode target: %w", err)
	}
	height := uint64(0)
	if _, err := fmt.Sscanf(payload[3], "0x%x", &height); err != nil {
		return nil, fmt.Errorf("decode height: %w", err)
	}
	return &work{
		sealHash: sealHash,
		seedHash: seedHash,
		target:   new(big.Int).SetBytes(targetBytes),
		height:   height,
	}, nil
}

func (m *miner) ensureVM(seed []byte) error {
	m.vmMu.Lock()
	defer m.vmMu.Unlock()
	if bytes.Equal(seed, m.seed) && m.vm != nil {
		return nil
	}
	if m.vm != nil {
		m.vm.Close()
		m.vm = nil
	}
	if m.cache != nil {
		m.cache.Close()
		m.cache = nil
	}
	cache := randomx.NewCache(randomx.RANDOMX_FLAG_HARD_AES)
	if cache == nil {
		return fmt.Errorf("allocate RandomX cache")
	}
	cache.Init(seed)
	vm := randomx.NewVM(randomx.RANDOMX_FLAG_HARD_AES, cache, nil)
	if vm == nil {
		cache.Close()
		return fmt.Errorf("create RandomX VM")
	}
	m.cache = cache
	m.vm = vm
	m.seed = append(m.seed[:0], seed...)
	return nil
}

func (m *miner) hash(seed, sealHash []byte, nonce uint64) ([]byte, error) {
	if err := m.ensureVM(seed); err != nil {
		return nil, err
	}
	input := make([]byte, 40)
	copy(input[:32], sealHash)
	binary.BigEndian.PutUint64(input[32:], nonce)
	output := make([]byte, 32)

	m.vmMu.Lock()
	m.vm.CalculateHash(input, output)
	m.vmMu.Unlock()

	return output, nil
}

func (m *miner) submit(nonce uint64, sealHash, digest []byte) bool {
	nonceHex := fmt.Sprintf("0x%016x", nonce)
	sealHex := "0x" + hex.EncodeToString(sealHash)
	digestHex := "0x" + hex.EncodeToString(digest)
	var ok bool
	if err := m.client.Call(&ok, "randomx_submitWorkRaw", nonceHex, sealHex, digestHex); err != nil {
		return false
	}
	return ok
}

func (m *miner) mineThread(id int, initial *work) {
	current := initial
	nonce := uint64(id) << 56
	for {
		if atomic.LoadUint64(&m.hashes)%5000 == 0 {
			if next, err := m.getWork(); err == nil && !bytes.Equal(next.sealHash, current.sealHash) {
				current = next
				fmt.Printf("thread %d new work height=%d target=%s\n", id, current.height, shortBig(current.target))
			}
		}
		digest, err := m.hash(current.seedHash, current.sealHash, nonce)
		if err != nil {
			fmt.Printf("thread %d hash error: %v\n", id, err)
			time.Sleep(time.Second)
			continue
		}
		atomic.AddUint64(&m.hashes, 1)
		if new(big.Int).SetBytes(digest).Cmp(current.target) <= 0 {
			if m.submit(nonce, current.sealHash, digest) {
				atomic.AddUint64(&m.accepted, 1)
				fmt.Printf("accepted block height=%d nonce=%d digest=0x%s\n", current.height, nonce, hex.EncodeToString(digest))
				if next, err := m.getWork(); err == nil {
					current = next
				}
			} else {
				atomic.AddUint64(&m.rejected, 1)
				fmt.Printf("rejected share height=%d nonce=%d\n", current.height, nonce)
			}
		}
		nonce++
	}
}

func (m *miner) stats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var last uint64
	lastTime := time.Now()
	for range ticker.C {
		now := atomic.LoadUint64(&m.hashes)
		elapsed := time.Since(lastTime).Seconds()
		fmt.Printf("hashrate=%.2f H/s accepted=%d rejected=%d total=%d\n",
			float64(now-last)/elapsed,
			atomic.LoadUint64(&m.accepted),
			atomic.LoadUint64(&m.rejected),
			now,
		)
		last = now
		lastTime = time.Now()
	}
}

func decodeHex(s string) ([]byte, error) {
	trimmed := strings.TrimPrefix(s, "0x")
	if len(trimmed)%2 == 1 {
		trimmed = "0" + trimmed
	}
	return hex.DecodeString(trimmed)
}

func shortBig(n *big.Int) string {
	s := n.String()
	if len(s) > 18 {
		return s[:18] + "..."
	}
	return s
}

func main() {
	rpcURL := flag.String("rpc", "http://127.0.0.1:8545", "RandomX RPC endpoint")
	threads := flag.Int("threads", runtime.NumCPU(), "mining threads")
	flag.Parse()
	if *threads < 1 {
		*threads = 1
	}

	m, err := newMiner(*rpcURL)
	if err != nil {
		fmt.Printf("create miner: %v\n", err)
		return
	}
	defer m.close()

	initial, err := m.getWork()
	if err != nil {
		fmt.Printf("get work: %v\n", err)
		return
	}
	fmt.Printf("RandomX external miner rpc=%s threads=%d height=%d target=%s\n", *rpcURL, *threads, initial.height, shortBig(initial.target))
	for i := 0; i < *threads; i++ {
		go m.mineThread(i, initial)
	}
	m.stats()
}
