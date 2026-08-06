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
	mainnetShieldedGroth16VKHex  = "0x544b4d473136564b31f90276a0a0f9c50bd033530a1d773ce6ae0c7ddd4f76f6f9a5ea56751b5274afce87954ab8409270d71bd8bbb4a7174266c289bd9981c07bb036dd6873900fe2e96664b626a90d570cc5063bd9c31e7724db00b8827891ac8c09a8575f7e82883869757ebdb1b840998e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c21800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6edb8409652191c25536a49bb0e339cf6b1bb3b273d132a7b4293aa3a2acf207f154a2d11b1e9a7d3d32f5be3f9ab290f5a43a215d3dcb8eafa6296630beea9d0ab02b2f9018ca09706c094ad8dd66349f67e10811d96309ac685cf87adec881f8b44c601c4fbb1a0dc493b3d0f7db24b1294620064860bd6a403d52664fe14434623fa2c3a109f53a0cea72f67a3b4f597d7a3aa872f75ad3fffcb018291db9f8dfcbce0f03a2a82aaa0ad2a4dce3cf0bbf9bf514d303018685cd1f07cc7c10f86050859af3367519f1ba0949139d5f1269ec0af09f1989e37876fa06dce355d825b5215617a4263256d3da0965e1374605baf49ea8c2121ffc38dbad92b2f60fb9fb2f99f06c2af2a403976a0dee48e6917553a16673545aeed44d5dd50d459adb4c63cac23013aa2d274023ba0c5d4cba100ccebbb79f477555db85ee789eee5cb6b364bc9e201d17b1b5b042ba0e36044cc191ffd53eca50b51ea319fea6219905c40e3e8724adea50f45c5a0c6a0d8e809bf0098506101cb22cd72b106fcf57367edf021e0ef259f0b8b5e0f1efca0dc2d9592878e3571976c86dcd3937edc698c658ffa1c5833d2b8371008ab7344a0aa3e430e9851ebe00de001098b220bba2c55a0143f8067d365bc3ddf0f837e07"
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
