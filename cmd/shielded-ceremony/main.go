package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/consensys/gnark-crypto/ecc"
	curve "github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark/backend/groth16"
	bn254groth16 "github.com/consensys/gnark/backend/groth16/bn254"
	ceremony "github.com/consensys/gnark/backend/groth16/bn254/mpcsetup"
	cs "github.com/consensys/gnark/constraint/bn254"
	"github.com/consensys/gnark/frontend"
	r1csbuilder "github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/zk/shielded"
	"github.com/ethereum/go-ethereum/zk/shielded6/recipient"
)

var selectedCircuit = "shielded-v2"

func main() {
	log.SetFlags(0)
	global := flag.NewFlagSet("shielded-ceremony", flag.ExitOnError)
	global.StringVar(&selectedCircuit, "circuit", "shielded-v2", "shielded-v2, shield6-recipient-v2 or shield6-registration-v2")
	if err := global.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	args := global.Args()
	if selectedCircuit != "shielded-v2" && selectedCircuit != "shield6-recipient-v2" && selectedCircuit != "shield6-registration-v2" {
		log.Fatal("unsupported circuit")
	}
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	var err error
	switch args[0] {
	case "generate-local":
		err = generateLocal(args[1:])
	case "compile":
		err = compileOutput(args[1:])
	case "init-phase1":
		err = initPhase1(args[1:])
	case "contribute-phase1":
		err = contributePhase1(args[1:])
	case "verify-phase1":
		err = verifyPhase1(args[1:])
	case "init-phase2":
		err = initPhase2(args[1:])
	case "contribute-phase2":
		err = contributePhase2(args[1:])
	case "finalize":
		err = finalize(args[1:])
	case "encode-vk":
		err = encodeVK(args[1:])
	case "verify-artifacts":
		err = verifyArtifacts(args[1:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// generateLocal is an explicitly single-operator setup. It never reuses test
// parameters and never claims the independent-contributor guarantees of MPC.
func generateLocal(args []string) error {
	fs := flag.NewFlagSet("generate-local", flag.ExitOnError)
	out := fs.String("out-dir", "", "directory for fresh setup artifacts")
	single := fs.Bool("single-operator", false, "acknowledge this is a single-operator trusted setup, not MPC")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*single || *out == "" || selectedCircuit == "shielded-v2" {
		return fmt.Errorf("recipient generate-local requires -out-dir and explicit -single-operator")
	}
	prefix, manifest := "recipient", "manifest.json"
	if selectedCircuit == "shield6-registration-v2" {
		prefix, manifest = "registration", "registration-manifest.json"
	}
	if err := os.MkdirAll(*out, 0700); err != nil {
		return err
	}
	names := []string{prefix + ".r1cs", prefix + ".pk", prefix + ".vk", prefix + "-provenance.json", manifest}
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(*out, name)); err == nil || !os.IsNotExist(err) {
			return fmt.Errorf("refusing to overwrite setup artifact %s", name)
		}
	}
	stage, err := os.MkdirTemp(*out, ".fresh-setup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	started := time.Now()
	compiled, err := compileCircuit()
	if err != nil {
		return err
	}
	expected := recipient.CircuitSHA256
	if selectedCircuit == "shield6-registration-v2" {
		expected = recipient.RegistrationCircuitSHA256
	}
	fingerprint := sha256.New()
	if _, err := compiled.WriteTo(fingerprint); err != nil {
		return err
	}
	if hex.EncodeToString(fingerprint.Sum(nil)) != expected {
		return fmt.Errorf("compiled circuit does not match its pinned production candidate")
	}
	fmt.Printf("fresh %s single-operator setup: %d constraints\n", selectedCircuit, compiled.GetNbConstraints())
	pk, vk, err := groth16.Setup(compiled)
	if err != nil {
		return err
	}
	if _, err := chainVerifyingKeyHex(vk); err != nil {
		return err
	}
	for _, artifact := range []struct {
		name string
		data io.WriterTo
	}{{prefix + ".r1cs", compiled}, {prefix + ".pk", pk}, {prefix + ".vk", vk}} {
		if err := writeObject(filepath.Join(stage, artifact.name), artifact.data); err != nil {
			return err
		}
	}
	pins := recipient.ArtifactPins{Version: recipient.RelationVersion}
	if pins.R1CS, err = hashFile(filepath.Join(stage, prefix+".r1cs")); err != nil {
		return err
	}
	if pins.ProvingKey, err = hashFile(filepath.Join(stage, prefix+".pk")); err != nil {
		return err
	}
	if pins.VerifyingKey, err = hashFile(filepath.Join(stage, prefix+".vk")); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pins, "", "  ")
	if err != nil {
		return err
	}
	if err := writeBytes(filepath.Join(stage, manifest), append(data, '\n')); err != nil {
		return err
	}
	provenance, err := json.MarshalIndent(map[string]any{
		"circuit": selectedCircuit, "method": "single-operator-groth16-setup",
		"independentContributors": false, "independentAudit": false,
		"entropy": "operating-system crypto/rand", "testParametersReused": false,
		"createdUTC":     time.Now().UTC().Format(time.RFC3339),
		"elapsedSeconds": time.Since(started).Seconds(), "pins": pins,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := writeBytes(filepath.Join(stage, prefix+"-provenance.json"), append(provenance, '\n')); err != nil {
		return err
	}
	// Publish the manifest last; a partial copy is never a complete artifact set.
	for _, name := range names {
		if err := os.Link(filepath.Join(stage, name), filepath.Join(*out, name)); err != nil {
			return err
		}
	}
	fmt.Printf("fresh %s setup completed in %s; single operator, not independently audited\n", selectedCircuit, time.Since(started))
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: shielded-ceremony [-circuit shielded-v2|shield6-recipient-v2|shield6-registration-v2] <command> [flags]

commands:
  generate-local      -single-operator -out-dir parameters (fresh Shield6 setup, not MPC)
  compile             -out recipient.r1cs
  init-phase1         -out phase1-0.bin
  contribute-phase1   -in phase1-N.bin -out phase1-N+1.bin
  verify-phase1       -beacon <hex-or-text> -out-commons commons.bin phase1-1.bin ...
  init-phase2         -commons commons.bin -out phase2-0.bin
  contribute-phase2   -in phase2-N.bin -out phase2-N+1.bin
  finalize            -commons commons.bin -beacon <hex-or-text> -pk proving.key -vk verifying.key -vk-hex verifying.hex phase2-1.bin ...
  encode-vk           -vk verifying.key -out verifying.hex
  verify-artifacts    -pk proving.key -vk verifying.key
`)
}

func initPhase1(args []string) error {
	fs := flag.NewFlagSet("init-phase1", flag.ExitOnError)
	out := fs.String("out", "", "output phase1 transcript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	r1cs, err := compileCircuit()
	if err != nil {
		return err
	}
	domainSize := ecc.NextPowerOfTwo(uint64(r1cs.GetNbConstraints()))
	phase1 := ceremony.NewPhase1(domainSize)
	return writeObject(*out, phase1)
}

func contributePhase1(args []string) error {
	fs := flag.NewFlagSet("contribute-phase1", flag.ExitOnError)
	in := fs.String("in", "", "input phase1 transcript")
	out := fs.String("out", "", "output phase1 transcript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" {
		return fmt.Errorf("-in and -out are required")
	}
	var phase1 ceremony.Phase1
	if err := readObject(*in, &phase1); err != nil {
		return err
	}
	phase1.Contribute()
	return writeObject(*out, &phase1)
}

func verifyPhase1(args []string) error {
	fs := flag.NewFlagSet("verify-phase1", flag.ExitOnError)
	beacon := fs.String("beacon", "", "final phase1 beacon as 0x hex or text")
	outCommons := fs.String("out-commons", "", "output sealed commons file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *beacon == "" || *outCommons == "" {
		return fmt.Errorf("-beacon and -out-commons are required")
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("at least one phase1 contribution file is required")
	}
	r1cs, err := compileCircuit()
	if err != nil {
		return err
	}
	domainSize := ecc.NextPowerOfTwo(uint64(r1cs.GetNbConstraints()))
	contribs, err := readPhase1Contribs(fs.Args())
	if err != nil {
		return err
	}
	commons, err := ceremony.VerifyPhase1(domainSize, beaconBytes(*beacon), contribs...)
	if err != nil {
		return err
	}
	return writeObject(*outCommons, &commons)
}

func initPhase2(args []string) error {
	fs := flag.NewFlagSet("init-phase2", flag.ExitOnError)
	commonsPath := fs.String("commons", "", "sealed phase1 commons file")
	out := fs.String("out", "", "output phase2 transcript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *commonsPath == "" || *out == "" {
		return fmt.Errorf("-commons and -out are required")
	}
	r1cs, err := compileCircuit()
	if err != nil {
		return err
	}
	commons, err := readCommons(*commonsPath)
	if err != nil {
		return err
	}
	var phase2 ceremony.Phase2
	phase2.Initialize(r1cs, commons)
	return writeObject(*out, &phase2)
}

func contributePhase2(args []string) error {
	fs := flag.NewFlagSet("contribute-phase2", flag.ExitOnError)
	in := fs.String("in", "", "input phase2 transcript")
	out := fs.String("out", "", "output phase2 transcript")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" {
		return fmt.Errorf("-in and -out are required")
	}
	var phase2 ceremony.Phase2
	if err := readObject(*in, &phase2); err != nil {
		return err
	}
	phase2.Contribute()
	return writeObject(*out, &phase2)
}

func finalize(args []string) error {
	fs := flag.NewFlagSet("finalize", flag.ExitOnError)
	commonsPath := fs.String("commons", "", "sealed phase1 commons file")
	beacon := fs.String("beacon", "", "final phase2 beacon as 0x hex or text")
	pkPath := fs.String("pk", "", "output gnark proving key")
	vkPath := fs.String("vk", "", "output gnark verifying key")
	vkHexPath := fs.String("vk-hex", "", "output chain verifying key hex")
	manifestPath := fs.String("manifest", "", "authenticated-release manifest output (required for Shield6 recipient)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *commonsPath == "" || *beacon == "" || *pkPath == "" || *vkPath == "" || *vkHexPath == "" {
		return fmt.Errorf("-commons, -beacon, -pk, -vk and -vk-hex are required")
	}
	if selectedCircuit != "shielded-v2" && *manifestPath == "" {
		return fmt.Errorf("-manifest is required for recipient artifacts")
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("at least one phase2 contribution file is required")
	}
	r1cs, err := compileCircuit()
	if err != nil {
		return err
	}
	commons, err := readCommons(*commonsPath)
	if err != nil {
		return err
	}
	contribs, err := readPhase2Contribs(fs.Args())
	if err != nil {
		return err
	}
	pk, vk, err := ceremony.VerifyPhase2(r1cs, commons, beaconBytes(*beacon), contribs...)
	if err != nil {
		return err
	}
	if err := writeObject(*pkPath, pk); err != nil {
		return err
	}
	if err := writeObject(*vkPath, vk); err != nil {
		return err
	}
	hexVK, err := chainVerifyingKeyHex(vk)
	if err != nil {
		return err
	}
	if err := writeBytes(*vkHexPath, []byte(hexVK+"\n")); err != nil {
		return err
	}
	if selectedCircuit != "shielded-v2" {
		csHash := sha256.New()
		if _, err := r1cs.WriteTo(csHash); err != nil {
			return err
		}
		pins := recipient.ArtifactPins{Version: recipient.RelationVersion}
		copy(pins.R1CS[:], csHash.Sum(nil))
		if pins.ProvingKey, err = hashFile(*pkPath); err != nil {
			return err
		}
		if pins.VerifyingKey, err = hashFile(*vkPath); err != nil {
			return err
		}
		data, err := json.MarshalIndent(pins, "", "  ")
		if err != nil {
			return err
		}
		return writeBytes(*manifestPath, append(data, '\n'))
	}
	return nil
}

func encodeVK(args []string) error {
	fs := flag.NewFlagSet("encode-vk", flag.ExitOnError)
	vkPath := fs.String("vk", "", "input gnark verifying key")
	out := fs.String("out", "", "output TKMG16VK1 verifying key hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *vkPath == "" || *out == "" {
		return fmt.Errorf("-vk and -out are required")
	}
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if err := readObject(*vkPath, vk); err != nil {
		return err
	}
	hexVK, err := chainVerifyingKeyHex(vk)
	if err != nil {
		return err
	}
	return writeBytes(*out, []byte(hexVK+"\n"))
}

func verifyArtifacts(args []string) error {
	fs := flag.NewFlagSet("verify-artifacts", flag.ExitOnError)
	pkPath := fs.String("pk", "", "input gnark proving key")
	vkPath := fs.String("vk", "", "input gnark verifying key")
	statementPath := fs.String("statement", "", "recipient statement JSON")
	witnessPath := fs.String("witness", "", "private recipient witness JSON; never publish")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pkPath == "" || *vkPath == "" {
		return fmt.Errorf("-pk and -vk are required")
	}
	r1cs, err := compileCircuit()
	if err != nil {
		return err
	}
	pk := groth16.NewProvingKey(ecc.BN254)
	if err := readObject(*pkPath, pk); err != nil {
		return err
	}
	vk := groth16.NewVerifyingKey(ecc.BN254)
	if err := readObject(*vkPath, vk); err != nil {
		return err
	}
	if selectedCircuit == "shield6-registration-v2" {
		if *statementPath == "" || *witnessPath == "" {
			return fmt.Errorf("registration verification requires -statement and private -witness files")
		}
		var statement recipient.RegistrationStatement
		var private recipient.RegistrationWitness
		defer private.Clear()
		if err := readJSON(*statementPath, &statement); err != nil {
			return err
		}
		if err := readJSON(*witnessPath, &private); err != nil {
			return err
		}
		proof, err := recipient.ProveRegistration(r1cs, pk, statement, private)
		if err != nil {
			return err
		}
		var encoded bytes.Buffer
		if _, err := vk.WriteTo(&encoded); err != nil {
			return err
		}
		verifier, err := recipient.NewVerifier(encoded.Bytes(), sha256.Sum256(encoded.Bytes()))
		if err != nil {
			return err
		}
		if err := verifier.VerifyRegistration(statement, proof); err != nil {
			return err
		}
		fmt.Println("registration artifact pair passed a real key-control proof; this does not establish production ceremony provenance")
		return nil
	}
	if selectedCircuit == "shield6-recipient-v2" {
		if *statementPath == "" || *witnessPath == "" {
			return fmt.Errorf("recipient artifact verification requires -statement and private -witness files")
		}
		var statement recipient.Statement
		var private recipient.Witness
		defer private.Clear()
		if err := readJSON(*statementPath, &statement); err != nil {
			return err
		}
		if err := readJSON(*witnessPath, &private); err != nil {
			return err
		}
		proof, err := recipient.Prove(r1cs, pk, statement, private)
		if err != nil {
			return err
		}
		var encoded bytes.Buffer
		if _, err := vk.WriteTo(&encoded); err != nil {
			return err
		}
		verifier, err := recipient.NewVerifier(encoded.Bytes(), sha256.Sum256(encoded.Bytes()))
		if err != nil {
			return err
		}
		if err := verifier.Verify(statement, proof); err != nil {
			return err
		}
		fmt.Println("recipient artifact pair passed a full proof roundtrip; this does not establish production ceremony provenance")
		return nil
	}
	assignment := shielded.DeterministicV2Assignment()
	witness, err := frontend.NewWitness(assignment, ecc.BN254.ScalarField())
	if err != nil {
		return fmt.Errorf("build full witness: %w", err)
	}
	proof, err := groth16.Prove(r1cs, pk, witness)
	if err != nil {
		return fmt.Errorf("prove deterministic vector: %w", err)
	}
	publicWitness, err := witness.Public()
	if err != nil {
		return fmt.Errorf("build public witness: %w", err)
	}
	if err := groth16.Verify(proof, vk, publicWitness); err != nil {
		return fmt.Errorf("verify deterministic proof: %w", err)
	}
	fmt.Println("proving key and verifying key passed a full Groth16 prove/verify round trip")
	return nil
}

func compileCircuit() (*cs.R1CS, error) {
	if selectedCircuit == "shield6-registration-v2" {
		compiled, hash, err := recipient.CompileRegistration()
		if err != nil {
			return nil, err
		}
		fmt.Printf("registration R1CS SHA256: %x\n", hash)
		return compiled.(*cs.R1CS), nil
	}
	if selectedCircuit == "shield6-recipient-v2" {
		compiled, hash, err := recipient.Compile()
		if err != nil {
			return nil, err
		}
		fmt.Printf("recipient R1CS SHA256: %x\n", hash)
		return compiled.(*cs.R1CS), nil
	}
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1csbuilder.NewBuilder, &shielded.SpendCircuitV2{})
	if err != nil {
		return nil, err
	}
	r1cs, ok := ccs.(*cs.R1CS)
	if !ok {
		return nil, fmt.Errorf("unexpected constraint system type %T", ccs)
	}
	return r1cs, nil
}

func chainVerifyingKeyHex(vk groth16.VerifyingKey) (string, error) {
	if selectedCircuit != "shielded-v2" {
		var encoded bytes.Buffer
		if _, err := vk.WriteTo(&encoded); err != nil {
			return "", err
		}
		if _, err := recipient.NewVerifier(encoded.Bytes(), sha256.Sum256(encoded.Bytes())); err != nil {
			return "", err
		}
		return "0x" + hex.EncodeToString(encoded.Bytes()), nil
	}
	vkBN254, ok := vk.(*bn254groth16.VerifyingKey)
	if !ok {
		return "", fmt.Errorf("unexpected verifying key type %T", vk)
	}
	encoded, err := core.EncodeShieldedGroth16VerifyingKey(core.ShieldedGroth16VerifyingKey{
		AlphaG1: encodeG1(vkBN254.G1.Alpha),
		BetaG2:  encodeG2(vkBN254.G2.Beta),
		GammaG2: encodeG2(vkBN254.G2.Gamma),
		DeltaG2: encodeG2(vkBN254.G2.Delta),
		IC:      encodeIC(vkBN254),
	})
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(encoded), nil
}

func encodeG1(point curve.G1Affine) []byte {
	encoded := point.Bytes()
	return append([]byte(nil), encoded[:]...)
}

func encodeG2(point curve.G2Affine) []byte {
	encoded := point.Bytes()
	return append([]byte(nil), encoded[:]...)
}

func encodeIC(vk *bn254groth16.VerifyingKey) [][]byte {
	out := make([][]byte, len(vk.G1.K))
	for i := range vk.G1.K {
		out[i] = encodeG1(vk.G1.K[i])
	}
	return out
}

func readPhase1Contribs(paths []string) ([]*ceremony.Phase1, error) {
	out := make([]*ceremony.Phase1, len(paths))
	for i, path := range paths {
		out[i] = new(ceremony.Phase1)
		if err := readObject(path, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readPhase2Contribs(paths []string) ([]*ceremony.Phase2, error) {
	out := make([]*ceremony.Phase2, len(paths))
	for i, path := range paths {
		out[i] = new(ceremony.Phase2)
		if err := readObject(path, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readCommons(path string) (*ceremony.SrsCommons, error) {
	commons := new(ceremony.SrsCommons)
	if err := readObject(path, commons); err != nil {
		return nil, err
	}
	return commons, nil
}

func readObject(path string, object io.ReaderFrom) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	n, err := object.ReadFrom(file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if n != stat.Size() {
		return fmt.Errorf("%s: trailing or truncated artifact", path)
	}
	return nil
}

func writeObject(path string, object io.WriterTo) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := object.WriteTo(io.MultiWriter(file, h)); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return err
	}
	fmt.Printf("%x  %s\n", h.Sum(nil), path)
	return nil
}

func compileOutput(args []string) error {
	fs := flag.NewFlagSet("compile", flag.ExitOnError)
	out := fs.String("out", "", "compiled R1CS output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	compiled, err := compileCircuit()
	if err != nil {
		return err
	}
	return writeObject(*out, compiled)
}

func hashFile(path string) ([32]byte, error) {
	var sum [32]byte
	file, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(io.LimitReader(file, 32<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("invalid trailing JSON")
	}
	return nil
}

func writeBytes(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	fmt.Printf("%s  %s\n", hex.EncodeToString(sum[:]), path)
	return nil
}

func beaconBytes(input string) []byte {
	if len(input) >= 2 && input[:2] == "0x" {
		decoded, err := hex.DecodeString(input[2:])
		if err != nil {
			log.Fatalf("invalid beacon hex: %v", err)
		}
		return decoded
	}
	return []byte(input)
}
