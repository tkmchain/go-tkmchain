package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

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
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init-phase1":
		err = initPhase1(os.Args[2:])
	case "contribute-phase1":
		err = contributePhase1(os.Args[2:])
	case "verify-phase1":
		err = verifyPhase1(os.Args[2:])
	case "init-phase2":
		err = initPhase2(os.Args[2:])
	case "contribute-phase2":
		err = contributePhase2(os.Args[2:])
	case "finalize":
		err = finalize(os.Args[2:])
	case "encode-vk":
		err = encodeVK(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: shielded-ceremony <command> [flags]

commands:
  init-phase1         -out phase1-0.bin
  contribute-phase1   -in phase1-N.bin -out phase1-N+1.bin
  verify-phase1       -beacon <hex-or-text> -out-commons commons.bin phase1-1.bin ...
  init-phase2         -commons commons.bin -out phase2-0.bin
  contribute-phase2   -in phase2-N.bin -out phase2-N+1.bin
  finalize            -commons commons.bin -beacon <hex-or-text> -pk proving.key -vk verifying.key -vk-hex verifying.hex phase2-1.bin ...
  encode-vk           -vk verifying.key -out verifying.hex
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
	vkHexPath := fs.String("vk-hex", "", "output TKMG16VK1 verifying key hex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *commonsPath == "" || *beacon == "" || *pkPath == "" || *vkPath == "" || *vkHexPath == "" {
		return fmt.Errorf("-commons, -beacon, -pk, -vk and -vk-hex are required")
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
	return writeBytes(*vkHexPath, []byte(hexVK+"\n"))
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

func compileCircuit() (*cs.R1CS, error) {
	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1csbuilder.NewBuilder, &shielded.SpendCircuit{})
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
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	n, err := object.ReadFrom(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if int(n) != len(data) {
		return fmt.Errorf("%s: read %d bytes, file has %d", path, n, len(data))
	}
	return nil
}

func writeObject(path string, object io.WriterTo) error {
	var buf bytes.Buffer
	if _, err := object.WriteTo(&buf); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return err
	}
	sum := sha256.Sum256(buf.Bytes())
	fmt.Printf("%s  %s\n", hex.EncodeToString(sum[:]), path)
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
