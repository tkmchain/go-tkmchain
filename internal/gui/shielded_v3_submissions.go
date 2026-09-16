package gui

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
)

func (g *GUI) shield3SubmissionPath(id string) string {
	digest := sha512.Sum512([]byte(id))
	return filepath.Join(g.opts.WalletStateDir, hex.EncodeToString(digest[:])+".json")
}
func (g *GUI) loadShield3Submission(id string) (*shield3RequestRecord, error) {
	file, err := os.Open(g.shield3SubmissionPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read saved wallet submission: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, int64(core.ShieldedV3MaxTxSize*2)+1))
	if err != nil || uint64(len(raw)) > core.ShieldedV3MaxTxSize*2 {
		return nil, errors.New("invalid saved wallet submission size")
	}
	var record shield3RequestRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, errors.New("invalid saved wallet submission")
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(record.Raw); err != nil || tx.Hash() != record.Hash || tx.Type() != types.PQTkmTxType || !core.HasShieldedV3Prefix(tx.Data()) {
		return nil, errors.New("invalid saved wallet transaction")
	}
	if _, err := types.Sender(types.NewQuantumSigner(tx.ChainId()), &tx); err != nil {
		return nil, errors.New("saved wallet transaction has invalid PQ authentication")
	}
	return &record, nil
}

// Persist only the signed public transaction and an opaque request digest.
// Write and sync before broadcast; never store seeds, witnesses or viewing keys.
func (g *GUI) saveShield3Submission(id string, record *shield3RequestRecord) error {
	if g.opts.WalletStateDir == "" {
		return nil
	}
	if err := os.MkdirAll(g.opts.WalletStateDir, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(g.opts.WalletStateDir, ".submission-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Windows does not replace existing destinations with os.Rename. Existing
	// records need no replacement: their exact bytes/hash remain retryable even
	// when the successful-broadcast flag has not been persisted.
	target := g.shield3SubmissionPath(id)
	if _, err = os.Stat(target); err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(file.Name(), target)
}
