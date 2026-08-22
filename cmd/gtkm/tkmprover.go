package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

const (
	tkmProvingKeySHA256   = "7c3dc3b9f33e522e84665189fa02c08299d209daaa80f96d2dfa6ad43dc2be40"
	tkmProvingKeyURL      = "https://raw.githubusercontent.com/tkmchain/go-tkmchain/shielded-groth16-recovery-20260820/prover-keys/shielded-groth16-recovery-20260820/proving.key"
	tkmProvingKeyV2SHA256 = "248d2a299233c0d57e5a03d30cba62d4dde8f716594e67585842065b5eebd626"
	tkmProvingKeyV2URL    = "https://raw.githubusercontent.com/tkmchain/go-tkmchain/shielded-v2-recipient-binding-20260820/prover-keys/shielded-v2-recipient-binding-20260820/proving.key"
	tkmProvingKeyMaxSize  = 128 << 20
)

var (
	tkmProverFlag = &cli.BoolFlag{
		Name:  "tkmprover",
		Usage: "start the shielded payout prover with gtkm",
	}
	tkmProverConfigFlag = &cli.PathFlag{
		Name:  "tkmprover.config",
		Usage: "prover configuration file (used with --tkmprover)",
	}
	tkmProverBinaryFlag = &cli.PathFlag{
		Name:  "tkmprover.bin",
		Usage: "shielded-payout-prover executable (used with --tkmprover)",
	}
	tkmProverKeyURLFlag = &cli.StringFlag{
		Name:  "tkmprover.key-url",
		Value: tkmProvingKeyURL,
		Usage: "trusted URL used to download the shared proving key (used with --tkmprover)",
	}
	tkmProverV2KeyURLFlag = &cli.StringFlag{
		Name:  "tkmprover.v2-key-url",
		Value: tkmProvingKeyV2URL,
		Usage: "trusted URL used to download the recipient-bound V2 proving key (used with --tkmprover)",
	}
)

type managedTkmProver struct {
	cmd *exec.Cmd
}

func startTkmProver(ctx *cli.Context) (*managedTkmProver, error) {
	if !ctx.Bool(tkmProverFlag.Name) {
		return &managedTkmProver{}, nil
	}
	config := ctx.Path(tkmProverConfigFlag.Name)
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve tkmprover config directory: %w", err)
		}
		config = filepath.Join(home, ".tkmchain", "tkmprover", "config.json")
	}
	if err := ensureTkmProverConfig(ctx, config); err != nil {
		return nil, err
	}

	binary := ctx.Path(tkmProverBinaryFlag.Name)
	if binary == "" {
		if executable, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(executable), "shielded-payout-prover")
			if _, err := os.Stat(candidate); err == nil {
				binary = candidate
			}
		}
	}
	if binary == "" {
		var err error
		binary, err = exec.LookPath("shielded-payout-prover")
		if err != nil {
			return nil, fmt.Errorf("find shielded-payout-prover: %w (build it with make production or pass --tkmprover.bin)", err)
		}
	}

	cmd := exec.Command(binary, "--config", config)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start tkmprover: %w", err)
	}
	log.Info("TKM shielded prover started", "pid", cmd.Process.Pid, "config", config)
	return &managedTkmProver{cmd: cmd}, nil
}

type tkmProverConfig struct {
	Listen               string `json:"listen"`
	AllowedOrigin        string `json:"allowedOrigin"`
	BearerToken          string `json:"bearerToken"`
	NodeRPC              string `json:"nodeRPC"`
	KeystoreDir          string `json:"keystoreDir"`
	SignerAddress        string `json:"signerAddress"`
	SignerPassphraseFile string `json:"signerPassphraseFile"`
	SignMode             string `json:"signMode"`
	ProvingKeyPath       string `json:"provingKeyPath"`
	ProvingKeyV2Path     string `json:"provingKeyV2Path"`
	NotesPath            string `json:"notesPath"`
	RequestsPath         string `json:"requestsPath"`
	GasLimit             uint64 `json:"gasLimit"`
	SubmitSync           bool   `json:"submitSync"`
	ReceiptTimeoutMs     int64  `json:"receiptTimeoutMs"`
}

func ensureTkmProverConfig(ctx *cli.Context, configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return err
		}
		var cfg tkmProverConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode tkmprover config: %w", err)
		}
		if cfg.AllowedOrigin == "" {
			cfg.AllowedOrigin = "https://wallet.tkmchain.site"
		}
		cfg.SignMode = "proof-only"
		cfg.SubmitSync = false
		cfg.KeystoreDir = ""
		cfg.SignerAddress = ""
		cfg.SignerPassphraseFile = ""
		encoded, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(configPath, append(encoded, '\n'), 0600); err != nil {
			return err
		}
		if cfg.ProvingKeyPath == "" {
			cfg.ProvingKeyPath = filepath.Join(filepath.Dir(configPath), "proving.key")
			encoded, err = json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(configPath, append(encoded, '\n'), 0600); err != nil {
				return err
			}
		}
		if cfg.ProvingKeyV2Path == "" {
			cfg.ProvingKeyV2Path = filepath.Join(filepath.Dir(configPath), "proving-v2.key")
			encoded, err = json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(configPath, append(encoded, '\n'), 0600); err != nil {
				return err
			}
		}
		if err := ensureTkmProvingKey(ctx, cfg.ProvingKeyPath); err != nil {
			return err
		}
		return ensureTkmProvingKeyV2(ctx, cfg.ProvingKeyV2Path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect tkmprover config %q: %w", configPath, err)
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create tkmprover directory: %w", err)
	}
	port := ctx.Int(utils.HTTPPortFlag.Name)
	if port == 0 {
		port = 8545
	}
	nodeRPC := fmt.Sprintf("http://127.0.0.1:%d", port)
	pkPath := filepath.Join(dir, "proving.key")
	if err := ensureTkmProvingKey(ctx, pkPath); err != nil {
		return err
	}
	pkV2Path := filepath.Join(dir, "proving-v2.key")
	if err := ensureTkmProvingKeyV2(ctx, pkV2Path); err != nil {
		return err
	}
	bearer := make([]byte, 32)
	if _, err := rand.Read(bearer); err != nil {
		return fmt.Errorf("generate prover bearer token: %w", err)
	}
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	for _, item := range []struct{ path, contents string }{{filepath.Join(dataDir, "notes.json"), "{\"notes\":[]}\n"}, {filepath.Join(dataDir, "requests.json"), "{\"requests\":{},\"deposits\":{}}\n"}} {
		if _, err := os.Stat(item.path); os.IsNotExist(err) {
			if err := os.WriteFile(item.path, []byte(item.contents), 0600); err != nil {
				return err
			}
		}
	}
	cfg := tkmProverConfig{Listen: "127.0.0.1:8787", AllowedOrigin: "https://wallet.tkmchain.site", BearerToken: hex.EncodeToString(bearer), NodeRPC: nodeRPC, SignMode: "proof-only", ProvingKeyPath: pkPath, ProvingKeyV2Path: pkV2Path, NotesPath: filepath.Join(dataDir, "notes.json"), RequestsPath: filepath.Join(dataDir, "requests.json"), GasLimit: 3000000, SubmitSync: false, ReceiptTimeoutMs: 20000}
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(configPath, encoded, 0600); err != nil {
		return fmt.Errorf("write tkmprover config: %w", err)
	}
	log.Info("Generated proof-only TKM prover configuration", "config", configPath)
	return nil
}

func verifyTkmProvingKey(path string) error {
	return verifyTkmProvingKeyHash(path, tkmProvingKeySHA256)
}

func verifyTkmProvingKeyHash(path, expectedSHA256 string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != expectedSHA256 {
		return fmt.Errorf("shielded proving key %q has SHA-256 %s, want %s", path, got, expectedSHA256)
	}
	return nil
}

func ensureTkmProvingKeyV2(ctx *cli.Context, path string) error {
	if tkmProvingKeyV2SHA256 == "" {
		return errors.New("recipient-bound V2 proving-key hash is not embedded")
	}
	if err := verifyTkmProvingKeyHash(path, tkmProvingKeyV2SHA256); err == nil {
		return nil
	}
	keyURL := ctx.String(tkmProverV2KeyURLFlag.Name)
	if keyURL == "" {
		return fmt.Errorf("matching V2 shielded proving key is unavailable at %q and --%s is empty", path, tkmProverV2KeyURLFlag.Name)
	}
	return downloadTkmProvingKey(ctx.Context, path, keyURL, tkmProvingKeyV2SHA256, tkmProvingKeyMaxSize)
}

func ensureTkmProvingKey(ctx *cli.Context, path string) error {
	if err := verifyTkmProvingKey(path); err == nil {
		return nil
	}
	keyURL := ctx.String(tkmProverKeyURLFlag.Name)
	if keyURL == "" {
		return fmt.Errorf("matching shielded proving key is unavailable at %q and --%s is empty", path, tkmProverKeyURLFlag.Name)
	}
	return downloadTkmProvingKey(ctx.Context, path, keyURL, tkmProvingKeySHA256, tkmProvingKeyMaxSize)
}

func downloadTkmProvingKey(ctx context.Context, path, keyURL, expectedSHA256 string, maxSize int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create proving-key cache directory: %w", err)
	}
	log.Info("Downloading shared TKM shielded proving key", "url", keyURL, "path", path)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, keyURL, nil)
	if err != nil {
		return fmt.Errorf("create proving-key request: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download proving key: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download proving key: HTTP %s", response.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".proving-key-*")
	if err != nil {
		return fmt.Errorf("create temporary proving key: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(response.Body, maxSize+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return fmt.Errorf("download proving key: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close downloaded proving key: %w", closeErr)
	}
	if written > maxSize {
		return fmt.Errorf("downloaded proving key exceeds %d bytes", maxSize)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != expectedSHA256 {
		return fmt.Errorf("downloaded proving key has SHA-256 %s, want %s", got, expectedSHA256)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("cache proving key: %w", err)
	}
	log.Info("Verified and cached shared TKM shielded proving key", "path", path, "sha256", expectedSHA256)
	return nil
}

func (p *managedTkmProver) stop() {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if err := p.cmd.Process.Kill(); err != nil {
		log.Debug("TKM shielded prover already stopped", "err", err)
		return
	}
	_, _ = p.cmd.Process.Wait()
	log.Info("TKM shielded prover stopped")
}
