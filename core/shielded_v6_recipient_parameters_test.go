package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/shield6test"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

// This cache is exclusively for opt-in integration tests. Its names and
// manifest domain deliberately differ from production wallet artifacts. Keys
// come from genuine random Groth16.Setup, never DummySetup or fixed scalars.
const shield6TestParametersEnv = "SHIELD6_RECIPIENT_TEST_PARAMETERS"

type shield6TestParameterManifest struct {
	Domain string                 `json:"domain"`
	Pins   recipient.ArtifactPins `json:"pins"`
}

func shield6RecipientTestParameters(t *testing.T, registration bool) (*recipient.PreparedProver, []byte, recipient.ArtifactPins) {
	t.Helper()
	name, fingerprint := "output", recipient.CircuitSHA256
	compile := recipient.Compile
	prepare, open := recipient.NewPreparedProver, recipient.OpenPreparedProver
	if registration {
		name, fingerprint = "registration", recipient.RegistrationCircuitSHA256
		compile = recipient.CompileRegistration
		prepare, open = recipient.NewRegistrationProver, recipient.OpenRegistrationProver
	}
	if production := os.Getenv("SHIELD6_RECIPIENT_PRODUCTION_PARAMETERS"); production != "" {
		prefix, manifestName := "recipient", "manifest.json"
		if registration {
			prefix, manifestName = "registration", "registration-manifest.json"
		}
		data, err := os.ReadFile(filepath.Join(production, manifestName))
		if err != nil {
			t.Fatal(err)
		}
		var pins recipient.ArtifactPins
		if err := json.Unmarshal(data, &pins); err != nil {
			t.Fatal(err)
		}
		config := params.MainnetChainConfig
		vkPin, pkPin := config.Shield6RecipientKeyHash, config.Shield6RecipientProvingKeyHash
		if registration {
			vkPin, pkPin = config.Shield6RegistrationKeyHash, config.Shield6RegistrationProvingKeyHash
		}
		if common.Hash(pins.VerifyingKey) != vkPin || common.Hash(pins.ProvingKey) != pkPin {
			t.Fatal("production parameters differ from mainnet consensus pins")
		}
		prepared, err := open(filepath.Join(production, prefix+".r1cs"), filepath.Join(production, prefix+".pk"), filepath.Join(production, prefix+".vk"), pins)
		if err != nil {
			t.Fatal(err)
		}
		vk, err := os.ReadFile(filepath.Join(production, prefix+".vk"))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: authenticated production setup matches mainnet consensus pins", name)
		return prepared, vk, pins
	}
	const domain = "SHIELD6_EPHEMERAL_TEST_SETUP_V2_NOT_FOR_PRODUCTION"
	var directory string
	if base := os.Getenv(shield6TestParametersEnv); base != "" {
		directory = filepath.Join(base, name+"-"+fingerprint)
		manifestPath := filepath.Join(directory, "test-only-manifest.json")
		if file, err := os.Open(manifestPath); err == nil {
			defer file.Close()
			decoder := json.NewDecoder(io.LimitReader(file, 8192))
			decoder.DisallowUnknownFields()
			var manifest shield6TestParameterManifest
			if err := decoder.Decode(&manifest); err != nil || manifest.Domain != domain || decoder.Decode(new(any)) != io.EOF {
				t.Fatal("invalid ephemeral test setup manifest", err)
			}
			started := time.Now()
			prepared, err := open(filepath.Join(directory, "test-circuit.r1cs"), filepath.Join(directory, "test-proving.pk"), filepath.Join(directory, "test-verifying.vk"), manifest.Pins)
			if err != nil {
				t.Fatal(err)
			}
			vk, err := os.ReadFile(filepath.Join(directory, "test-verifying.vk"))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: authenticated cached test setup loaded in %s", name, time.Since(started))
			return prepared, vk, manifest.Pins
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if _, err := os.Stat(directory); err == nil || !os.IsNotExist(err) {
			t.Fatal("refusing to overwrite incomplete test parameters", directory, err)
		}
	}
	started := time.Now()
	t.Logf("%s: compiling current relation; GOMAXPROCS=%d", name, runtime.GOMAXPROCS(0))
	cs, hash, err := compile()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: compiled %d constraints in %s; beginning genuine ephemeral Groth16.Setup", name, cs.GetNbConstraints(), time.Since(started))
	stop := make(chan struct{})
	done := make(chan struct{})
	setupStart := time.Now()
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				t.Logf("%s: setup elapsed=%s heap=%dMiB GC=%d", name, time.Since(setupStart).Round(time.Second), stats.HeapAlloc>>20, stats.NumGC)
			}
		}
	}()
	pk, vk, err := groth16.Setup(cs)
	close(stop)
	<-done
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: genuine setup completed in %s", name, time.Since(setupStart))
	var encoded bytes.Buffer
	if _, err := vk.WriteTo(&encoded); err != nil {
		t.Fatal(err)
	}
	pins := recipient.ArtifactPins{Version: recipient.RelationVersion, R1CS: hash, VerifyingKey: sha256.Sum256(encoded.Bytes())}
	h := sha256.New()
	if _, err := pk.WriteTo(h); err != nil {
		t.Fatal(err)
	}
	copy(pins.ProvingKey[:], h.Sum(nil))
	if directory != "" {
		shield6SaveTestParameters(t, directory, domain, cs, pk, encoded.Bytes(), pins)
	}
	prepared, err := prepare(cs, pk, encoded.Bytes(), pins)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, encoded.Bytes(), pins
}

func shield6SaveTestParameters(t *testing.T, directory, domain string, cs constraint.ConstraintSystem, pk groth16.ProvingKey, vk []byte, pins recipient.ArtifactPins) {
	t.Helper()
	base := filepath.Dir(directory)
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := os.MkdirTemp(base, ".pending-test-setup-") //nolint:usetesting // Cache publication requires staging on the destination filesystem.
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stage)
	for _, item := range []struct {
		name string
		data io.WriterTo
	}{{"test-circuit.r1cs", cs}, {"test-proving.pk", pk}} {
		file, err := os.OpenFile(filepath.Join(stage, item.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := item.data.WriteTo(file)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal(writeErr, closeErr)
		}
	}
	if err := os.WriteFile(filepath.Join(stage, "test-verifying.vk"), vk, 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(shield6TestParameterManifest{Domain: domain, Pins: pins})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "test-only-manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, directory); err != nil {
		t.Fatal(err)
	}
	t.Log("checkpointed genuine ephemeral test setup; this material is not a production setup")
}

// Run after the full canonical rehearsal. This exercises the same bounded
// on-disk parameter loader used by the wallet, against genuine saved setup
// artifacts and the exact payment proofs already validated canonically.
func TestShield6RecipientCachedSetupArtifacts(t *testing.T) {
	base, path := os.Getenv(shield6TestParametersEnv), os.Getenv(shield6test.FixtureEnvironment)
	if base == "" || path == "" {
		t.Skip("requires completed real-proof setup cache and network fixture")
	}
	fixture, err := shield6test.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(fixture.Payment); err != nil {
		t.Fatal(err)
	}
	envelope, recognized, err := DecodeShieldedV6Transaction(tx.Data())
	if err != nil || !recognized || envelope.Recipients == nil {
		t.Fatal("missing genuine proof-bearing payment", err)
	}
	for _, registration := range []bool{false, true} {
		name, fingerprint := "output", recipient.CircuitSHA256
		if registration {
			name, fingerprint = "registration", recipient.RegistrationCircuitSHA256
		}
		t.Run(name, func(t *testing.T) {
			// A missing cache is a failure here, never a request to run setup.
			if _, err := os.Stat(filepath.Join(base, name+"-"+fingerprint, "test-only-manifest.json")); err != nil {
				t.Fatal(err)
			}
			prepared, _, pins := shield6RecipientTestParameters(t, registration)
			if registration {
				if common.Hash(pins.ProvingKey) != fixture.Config.Shield6RegistrationProvingKeyHash || common.Hash(pins.VerifyingKey) != fixture.Config.Shield6RegistrationKeyHash {
					t.Fatal("cached registration keys differ from canonical rehearsal")
				}
			} else {
				if common.Hash(pins.ProvingKey) != fixture.Config.Shield6RecipientProvingKeyHash || common.Hash(pins.VerifyingKey) != fixture.Config.Shield6RecipientKeyHash {
					t.Fatal("cached output keys differ from canonical rehearsal")
				}
				for i, proof := range envelope.Recipients.Proofs {
					statement, err := ShieldedV6RecipientStatement(&tx, envelope, i)
					if err != nil {
						t.Fatal(err)
					}
					if err := prepared.Verifier().Verify(statement, proof); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
		runtime.GC()
		debug.FreeOSMemory()
	}
}
