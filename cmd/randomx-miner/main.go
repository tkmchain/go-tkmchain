package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/consensus/randomx"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/log"
    "github.com/ethereum/go-ethereum/params"
    "github.com/gorilla/mux"
)

type MiningServer struct {
    engine      *randomx.RandomX
    coinbase    common.Address
    threads     int
    soloMining  bool
    httpServer  *http.Server
}

func NewMiningServer(coinbase common.Address, threads int, soloMining bool) (*MiningServer, error) {
    config := randomx.DefaultConfig()
    engine, err := randomx.New(config, threads, coinbase, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create RandomX engine: %w", err)
    }

    server := &MiningServer{
        engine:     engine,
        coinbase:   coinbase,
        threads:    threads,
        soloMining: soloMining,
    }

    if !soloMining {
        server.setupHTTPServer()
    }

    return server, nil
}

func (s *MiningServer) setupHTTPServer() {
    router := mux.NewRouter()
    router.HandleFunc("/getwork", s.handleGetWork).Methods("POST", "GET")
    router.HandleFunc("/submitwork", s.handleSubmitWork).Methods("POST")
    router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    })

    port := os.Getenv("RPC_PORT")
    if port == "" {
        port = "8545"
    }

    s.httpServer = &http.Server{
        Addr:    ":" + port,
        Handler: router,
    }
}

func (s *MiningServer) handleGetWork(w http.ResponseWriter, r *http.Request) {
    work, err := s.engine.GetWork()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    response := map[string]interface{}{
        "result": work,
        "error":  nil,
        "id":     1,
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (s *MiningServer) handleSubmitWork(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Params []string `json:"params"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if len(req.Params) < 3 {
        http.Error(w, "Invalid params", http.StatusBadRequest)
        return
    }

    valid, err := s.engine.SubmitWork(req.Params[0], req.Params[1], req.Params[2])
    if err != nil {
        json.NewEncoder(w).Encode(map[string]interface{}{
            "result": false,
            "error":  err.Error(),
            "id":     1,
        })
        return
    }

    json.NewEncoder(w).Encode(map[string]interface{}{
        "result": valid,
        "error":  nil,
        "id":     1,
    })
}

func (s *MiningServer) Start() error {
    if !s.soloMining {
        go func() {
            log.Info("Starting RPC server", "addr", s.httpServer.Addr)
            if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                log.Error("HTTP server error", "error", err)
            }
        }()
        log.Info("External miner mode - waiting for getwork requests")
        return nil
    }

    // Solo mining mode
    log.Info("Starting solo mining", "threads", s.threads, "coinbase", s.coinbase.Hex())
    
    results := make(chan *types.Block, 10)
    stop := make(chan struct{})
    
    // Start mining workers
    for i := 0; i < s.threads; i++ {
        go s.mineWorker(i, results, stop)
    }
    
    // Handle results
    go func() {
        for block := range results {
            log.Info("�� BLOCK MINED! ��",
                "number", block.NumberU64(),
                "hash", block.Hash().Hex(),
                "txs", len(block.Transactions()))
        }
    }()
    
    // Wait for interrupt
    sigc := make(chan os.Signal, 1)
    signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
    <-sigc
    
    close(stop)
    close(results)
    return nil
}

func (s *MiningServer) mineWorker(workerID int, results chan<- *types.Block, stop <-chan struct{}) {
    log.Info("Worker started", "worker", workerID)
    
    header := &types.Header{
        Number:     common.Big1,
        Difficulty: randomx.GenesisDifficulty,
        Time:       uint64(time.Now().Unix()),
        Coinbase:   s.coinbase,
        Extra:      []byte(fmt.Sprintf("RandomX Miner %d", workerID)),
    }
    
    block := types.NewBlock(header, nil, nil, nil, nil)
    workerResults := make(chan *types.Block, 1)
    
    for {
        select {
        case <-stop:
            return
        default:
            if err := s.engine.Seal(nil, block, workerResults, stop); err == nil {
                select {
                case minedBlock := <-workerResults:
                    results <- minedBlock
                case <-stop:
                    return
                }
            }
            time.Sleep(time.Second)
        }
    }
}

func (s *MiningServer) Stop() error {
    if s.httpServer != nil {
        s.httpServer.Close()
    }
    s.engine.Close()
    return nil
}

func main() {
    log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

    coinbaseHex := os.Getenv("COINBASE")
    if coinbaseHex == "" {
        coinbaseHex = "0x79eb43064b826570FFa9c329c5685208E5257703"
    }
    
    soloMining := os.Getenv("SOLO_MINE") == "true"
    threads := 2
    if t := os.Getenv("THREADS"); t != "" {
        fmt.Sscanf(t, "%d", &threads)
    }

    server, err := NewMiningServer(common.HexToAddress(coinbaseHex), threads, soloMining)
    if err != nil {
        log.Error("Failed to create mining server", "error", err)
        os.Exit(1)
    }
    defer server.Stop()

    if err := server.Start(); err != nil {
        log.Error("Failed to start server", "error", err)
        os.Exit(1)
    }

    select {}
}
