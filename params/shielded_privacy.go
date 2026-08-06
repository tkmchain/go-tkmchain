package params

import (
	"bytes"
	"fmt"
	"io"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	shieldedGroth16VKMagic       = "TKMG16VK1"
	shieldedGroth16PublicInputs  = 11
	mainnetShieldedGroth16VKHex  = ""
	mainnetShieldedGroth16VKName = "mainnet shielded Groth16 verifying key"
)

// MainnetShieldedGroth16VerifyingKey is the audited ceremony output for the
// mainnet shielded spend circuit, encoded with core.EncodeShieldedGroth16VerifyingKey.
//
// It must remain empty until the final circuit has completed independent audit
// and key generation. Mainnet startup refuses an active privacy fork while this
// artifact is missing.
var MainnetShieldedGroth16VerifyingKey = mustDecodeOptionalHex(mainnetShieldedGroth16VKHex)

type shieldedGroth16VerifyingKeyEnvelope struct {
	AlphaG1 []byte
	BetaG2  []byte
	GammaG2 []byte
	DeltaG2 []byte
	IC      [][]byte
}

func mustDecodeOptionalHex(input string) hexutil.Bytes {
	if input == "" {
		return nil
	}
	out, err := hexutil.Decode(input)
	if err != nil {
		panic(err)
	}
	return out
}

// CheckMainnetShieldedPrivacyReady verifies that a mainnet config cannot enter
// the shielded privacy fork without the audited Groth16 verifying key artifact.
func (c *ChainConfig) CheckMainnetShieldedPrivacyReady() error {
	if c == nil || c.ChainID == nil || c.ChainID.Uint64() != MainnetChainConfig.ChainID.Uint64() {
		return nil
	}
	if c.PrivacyCommitmentTime == nil {
		return nil
	}
	return validateShieldedGroth16VK(c.ShieldedGroth16VerifyingKey, mainnetShieldedGroth16VKName)
}

func validateShieldedGroth16VK(encoded []byte, name string) error {
	if len(encoded) == 0 {
		return fmt.Errorf("%s is missing: embed the audited ceremony output in shieldedGroth16VerifyingKey before activating mainnet privacy", name)
	}
	if !bytes.HasPrefix(encoded, []byte(shieldedGroth16VKMagic)) {
		return fmt.Errorf("%s has invalid magic", name)
	}
	var envelope shieldedGroth16VerifyingKeyEnvelope
	if err := rlp.DecodeBytes(encoded[len(shieldedGroth16VKMagic):], &envelope); err != nil {
		return fmt.Errorf("%s is malformed: %w", name, err)
	}
	if len(envelope.IC) != shieldedGroth16PublicInputs+1 {
		return fmt.Errorf("%s has %d IC points, want %d", name, len(envelope.IC), shieldedGroth16PublicInputs+1)
	}
	if err := validateShieldedG1(envelope.AlphaG1); err != nil {
		return fmt.Errorf("%s alpha_g1 is invalid: %w", name, err)
	}
	if err := validateShieldedG2(envelope.BetaG2); err != nil {
		return fmt.Errorf("%s beta_g2 is invalid: %w", name, err)
	}
	if err := validateShieldedG2(envelope.GammaG2); err != nil {
		return fmt.Errorf("%s gamma_g2 is invalid: %w", name, err)
	}
	if err := validateShieldedG2(envelope.DeltaG2); err != nil {
		return fmt.Errorf("%s delta_g2 is invalid: %w", name, err)
	}
	for i, point := range envelope.IC {
		if err := validateShieldedG1(point); err != nil {
			return fmt.Errorf("%s ic[%d] is invalid: %w", name, i, err)
		}
	}
	return nil
}

func validateShieldedG1(encoded []byte) error {
	var point bn254.G1Affine
	if len(encoded) == 0 {
		return io.ErrUnexpectedEOF
	}
	reader := bytes.NewReader(encoded)
	dec := bn254.NewDecoder(reader)
	if err := dec.Decode(&point); err != nil {
		return err
	}
	if reader.Len() != 0 {
		return fmt.Errorf("trailing G1 bytes: %d", reader.Len())
	}
	if point.IsInfinity() {
		return fmt.Errorf("point is infinity")
	}
	return nil
}

func validateShieldedG2(encoded []byte) error {
	var point bn254.G2Affine
	if len(encoded) == 0 {
		return io.ErrUnexpectedEOF
	}
	reader := bytes.NewReader(encoded)
	dec := bn254.NewDecoder(reader)
	if err := dec.Decode(&point); err != nil {
		return err
	}
	if reader.Len() != 0 {
		return fmt.Errorf("trailing G2 bytes: %d", reader.Len())
	}
	if point.IsInfinity() {
		return fmt.Errorf("point is infinity")
	}
	return nil
}
