package gui

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/shield3wallet"
	"github.com/ethereum/go-ethereum/zk/shielded3"
)

type relayReservationRPC struct {
	shield3wallet.RPC
	reserved map[shielded3.Digest]common.Hash
}

func (r relayReservationRPC) CallContext(ctx context.Context, dest any, method string, args ...any) error {
	if method != "tkmprivacy_shieldedV3NullifierStatus" {
		return r.RPC.CallContext(ctx, dest, method, args...)
	}
	var status struct {
		TransactionHash common.Hash `json:"transactionHash"`
		Pending         bool        `json:"pending"`
	}
	if err := r.RPC.CallContext(ctx, &status, method, args...); err != nil {
		return err
	}
	if len(args) == 1 && !status.Pending && status.TransactionHash == (common.Hash{}) {
		if n, ok := args[0].(shielded3.Digest); ok {
			if hash := r.reserved[n]; hash != (common.Hash{}) {
				status.TransactionHash = hash
				status.Pending = true
			}
		}
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// Called with shield3Mu held. Unsigned authorizations remain reserved until their
// consensus expiry, including across a wallet restart. Cancelling locally cannot
// revoke a packet already delivered to an operator.
func (g *GUI) shield3ReservedRPC(ctx context.Context, account common.Address) (shield3wallet.RPC, error) {
	r := relayReservationRPC{RPC: g.client, reserved: make(map[shielded3.Digest]common.Hash)}
	var head struct {
		Timestamp hexutil.Uint64 `json:"timestamp"`
	}
	if err := g.client.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return nil, err
	}
	records := make([]*shield3RequestRecord, 0, len(g.shield3Requests))
	for _, record := range g.shield3Requests {
		records = append(records, record)
	}
	if g.opts.WalletStateDir != "" {
		entries, err := os.ReadDir(g.opts.WalletStateDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || len(name) != 133 || !strings.HasSuffix(name, ".json") {
				continue
			}
			if _, err := hex.DecodeString(name[:128]); err != nil {
				continue
			}
			record, err := readShield3Submission(filepath.Join(g.opts.WalletStateDir, name))
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}
	for _, record := range records {
		if !record.Unsigned || record.DraftAccount != account {
			continue
		}
		var tx types.Transaction
		if err := tx.UnmarshalBinary(record.Raw); err != nil {
			return nil, err
		}
		e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
		if err != nil {
			return nil, err
		}
		if !ok || !e.Relayed {
			return nil, errors.New("invalid saved relay authorization")
		}
		if e.ValidUntil <= uint64(head.Timestamp) {
			continue
		}
		ns, err := core.ShieldedV3Nullifiers(e)
		if err != nil {
			return nil, err
		}
		for _, n := range ns {
			r.reserved[n] = record.Hash
		}
	}
	return r, nil
}

func (g *GUI) shield3RelayStatus(ctx context.Context, record *shield3RequestRecord) (map[string]any, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(record.Raw); err != nil {
		return nil, err
	}
	e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("invalid relay draft")
	}
	ns, err := core.ShieldedV3Nullifiers(e)
	if err != nil {
		return nil, err
	}
	var actual common.Hash
	pending := false
	found := 0
	for _, n := range ns {
		var state struct {
			TransactionHash common.Hash `json:"transactionHash"`
			Pending         bool        `json:"pending"`
		}
		if err := g.client.CallContext(ctx, &state, "tkmprivacy_shieldedV3NullifierStatus", n); err != nil {
			return nil, err
		}
		if state.TransactionHash != (common.Hash{}) {
			if actual != (common.Hash{}) && actual != state.TransactionHash {
				return map[string]any{"status": "conflicting inputs"}, nil
			}
			actual = state.TransactionHash
			pending = pending || state.Pending
			found++
		}
	}
	if found > 0 && found != len(ns) {
		return map[string]any{"status": "conflicting inputs", "transactionHash": actual}, nil
	}
	if found == len(ns) {
		var raw hexutil.Bytes
		if err := g.client.CallContext(ctx, &raw, "eth_getRawTransactionByHash", actual); err != nil {
			return nil, err
		}
		var submitted types.Transaction
		if err := submitted.UnmarshalBinary(raw); err != nil {
			return nil, err
		}
		_, pub, _, _ := submitted.PQTkmFields()
		_, draftPub, _, _ := tx.PQTkmFields()
		if submitted.Hash() != actual || !bytes.Equal(submitted.Data(), tx.Data()) || !bytes.Equal(pub, draftPub) || submitted.Nonce() != tx.Nonce() || submitted.Gas() != tx.Gas() || submitted.GasFeeCap().Cmp(tx.GasFeeCap()) != 0 || submitted.GasTipCap().Cmp(tx.GasTipCap()) != 0 || submitted.ChainId().Cmp(tx.ChainId()) != 0 || submitted.Value().Cmp(tx.Value()) != 0 || submitted.To() == nil || tx.To() == nil || *submitted.To() != *tx.To() {
			return map[string]any{"status": "conflicting inputs", "transactionHash": actual}, nil
		}
		status := "confirmed"
		if pending {
			status = "unconfirmed"
		}
		return map[string]any{"status": status, "transactionHash": actual}, nil
	}
	var head struct {
		Timestamp hexutil.Uint64 `json:"timestamp"`
	}
	if err := g.client.CallContext(ctx, &head, "eth_getBlockByNumber", "latest", false); err != nil {
		return nil, err
	}
	status := "prepared · notes reserved"
	if uint64(head.Timestamp) >= e.ValidUntil {
		status = "expired"
	}
	return map[string]any{"status": status, "draftHash": record.Hash, "validUntil": e.ValidUntil}, nil
}

func shield3DraftExpired(record *shield3RequestRecord, time uint64) bool {
	if !record.Unsigned {
		return false
	}
	var tx types.Transaction
	if tx.UnmarshalBinary(record.Raw) != nil {
		return false
	}
	e, ok, err := core.DecodeShieldedV3Transaction(tx.Data())
	return err == nil && ok && e.Relayed && e.ValidUntil <= time
}
