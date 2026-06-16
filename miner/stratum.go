// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package miner

import (
	"bufio"
	"encoding/json"
	"errors"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

// StratumServer exposes the miner work API through a minimal stratum+tcp bridge.
type StratumServer struct {
	miner *Miner
	addr  string
	ln    net.Listener
	quit  chan struct{}
	wg    sync.WaitGroup
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

type stratumRequest struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

type stratumResponse struct {
	ID     json.RawMessage `json:"id"`
	Result interface{}     `json:"result,omitempty"`
	Error  interface{}     `json:"error,omitempty"`
}

// StartStratum starts a local stratum+tcp server for external RandomX miners.
func (miner *Miner) StartStratum(addr string) error {
	if miner.stratum != nil {
		return nil
	}
	server, err := NewStratumServer(miner, addr)
	if err != nil {
		return err
	}
	miner.stratum = server
	return nil
}

// NewStratumServer starts a stratum server bound to addr.
func NewStratumServer(miner *Miner, addr string) (*StratumServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &StratumServer{miner: miner, addr: addr, ln: ln, quit: make(chan struct{}), conns: make(map[net.Conn]struct{})}
	server.wg.Add(1)
	go server.accept()
	log.Info("RandomX stratum server listening", "addr", ln.Addr())
	return server, nil
}

// Stop closes the stratum listener and active handlers.
func (s *StratumServer) Stop() {
	select {
	case <-s.quit:
		return
	default:
		close(s.quit)
		_ = s.ln.Close()
		s.mu.Lock()
		for conn := range s.conns {
			_ = conn.Close()
		}
		s.mu.Unlock()
		s.wg.Wait()
	}
}

func (s *StratumServer) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Warn("RandomX stratum accept failed", "err", err)
				continue
			}
		}
		s.wg.Add(1)
		go s.handle(conn)
	}
}

func (s *StratumServer) handle(conn net.Conn) {
	defer s.wg.Done()
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	enc := json.NewEncoder(conn)
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req stratumRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		result, err := s.handleRequest(req, enc)
		resp := stratumResponse{ID: req.ID, Result: result}
		if err != nil {
			resp.Result = nil
			resp.Error = []interface{}{20, err.Error(), nil}
		}
		_ = enc.Encode(resp)
	}
}

func (s *StratumServer) handleRequest(req stratumRequest, enc *json.Encoder) (interface{}, error) {
	switch req.Method {
	case "mining.subscribe":
		return []interface{}{[]interface{}{[]string{"mining.set_difficulty", "1"}, []string{"mining.notify", "1"}}, randomHex(8), 4}, nil
	case "mining.authorize":
		return true, s.notify(enc)
	case "mining.get_job", "mining.get_work":
		return s.job()
	case "mining.submit":
		return s.submit(req.Params)
	default:
		return nil, errors.New("unsupported stratum method: " + req.Method)
	}
}

func (s *StratumServer) notify(enc *json.Encoder) error {
	job, err := s.job()
	if err != nil {
		return err
	}
	return enc.Encode(map[string]interface{}{"id": nil, "method": "mining.notify", "params": job})
}

func (s *StratumServer) job() ([]string, error) {
	work, err := s.miner.GetWork()
	if err != nil {
		return nil, err
	}
	return []string{strings.TrimPrefix(work[0], "0x"), strings.TrimPrefix(work[1], "0x"), strings.TrimPrefix(work[2], "0x"), strings.TrimPrefix(work[3], "0x")}, nil
}

func (s *StratumServer) submit(params []json.RawMessage) (bool, error) {
	if len(params) < 4 {
		return false, errors.New("not enough submit parameters")
	}
	var jobID, nonceHex, digestHex string
	if err := json.Unmarshal(params[1], &jobID); err != nil {
		return false, err
	}
	if err := json.Unmarshal(params[2], &nonceHex); err != nil {
		return false, err
	}
	if err := json.Unmarshal(params[3], &digestHex); err != nil {
		return false, err
	}
	var nonce types.BlockNonce
	nonceBytes := common.FromHex(nonceHex)
	if len(nonceBytes) != len(nonce) {
		return false, errors.New("invalid nonce length")
	}
	copy(nonce[:], nonceBytes)
	ok := s.miner.SubmitWork(nonce, common.HexToHash(jobID), common.HexToHash(digestHex))
	return ok, nil
}

func randomHex(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(strconv.FormatInt(int64(rand.Intn(16)), 16))
	}
	return b.String()
}
