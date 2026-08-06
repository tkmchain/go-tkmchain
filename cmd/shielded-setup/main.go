package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/consensys/gnark-crypto/ecc"
	curve "github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark/backend/groth16"
	bn254groth16 "github.com/consensys/gnark/backend/groth16/bn254"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/zk/shielded"
)

func main() {
	out := flag.String("out", "", "write the hex-encoded TKMG16VK1 verifying key to this file")
	flag.Parse()

	ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &shielded.SpendCircuit{})
	if err != nil {
		log.Fatal(err)
	}
	_, vk, err := groth16.Setup(ccs)
	if err != nil {
		log.Fatal(err)
	}
	vkBN254, ok := vk.(*bn254groth16.VerifyingKey)
	if !ok {
		log.Fatalf("unexpected verifying key type %T", vk)
	}

	encoded, err := core.EncodeShieldedGroth16VerifyingKey(core.ShieldedGroth16VerifyingKey{
		AlphaG1: encodeG1(vkBN254.G1.Alpha),
		BetaG2:  encodeG2(vkBN254.G2.Beta),
		GammaG2: encodeG2(vkBN254.G2.Gamma),
		DeltaG2: encodeG2(vkBN254.G2.Delta),
		IC:      encodeIC(vkBN254),
	})
	if err != nil {
		log.Fatal(err)
	}
	hexVK := "0x" + hex.EncodeToString(encoded)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(hexVK+"\n"), 0600); err != nil {
			log.Fatal(err)
		}
		return
	}
	fmt.Println(hexVK)
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
