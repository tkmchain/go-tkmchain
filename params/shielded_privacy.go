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
	shieldedGroth16VKMagic               = "TKMG16VK1"
	shieldedGroth16PublicInputs          = 11
	mainnetShieldedGroth16VKHex          = "0x544b4d473136564b31f90276a0a0f9c50bd033530a1d773ce6ae0c7ddd4f76f6f9a5ea56751b5274afce87954ab8409270d71bd8bbb4a7174266c289bd9981c07bb036dd6873900fe2e96664b626a90d570cc5063bd9c31e7724db00b8827891ac8c09a8575f7e82883869757ebdb1b840998e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c21800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6edb8409652191c25536a49bb0e339cf6b1bb3b273d132a7b4293aa3a2acf207f154a2d11b1e9a7d3d32f5be3f9ab290f5a43a215d3dcb8eafa6296630beea9d0ab02b2f9018ca09706c094ad8dd66349f67e10811d96309ac685cf87adec881f8b44c601c4fbb1a0dc493b3d0f7db24b1294620064860bd6a403d52664fe14434623fa2c3a109f53a0cea72f67a3b4f597d7a3aa872f75ad3fffcb018291db9f8dfcbce0f03a2a82aaa0ad2a4dce3cf0bbf9bf514d303018685cd1f07cc7c10f86050859af3367519f1ba0949139d5f1269ec0af09f1989e37876fa06dce355d825b5215617a4263256d3da0965e1374605baf49ea8c2121ffc38dbad92b2f60fb9fb2f99f06c2af2a403976a0dee48e6917553a16673545aeed44d5dd50d459adb4c63cac23013aa2d274023ba0c5d4cba100ccebbb79f477555db85ee789eee5cb6b364bc9e201d17b1b5b042ba0e36044cc191ffd53eca50b51ea319fea6219905c40e3e8724adea50f45c5a0c6a0d8e809bf0098506101cb22cd72b106fcf57367edf021e0ef259f0b8b5e0f1efca0dc2d9592878e3571976c86dcd3937edc698c658ffa1c5833d2b8371008ab7344a0aa3e430e9851ebe00de001098b220bba2c55a0143f8067d365bc3ddf0f837e07"
	mainnetShieldedGroth16UpgradedVKHex  = "0x544b4d473136564b31f90276a0e396bcf2b1b6e1d5ee70facaffa8ec19be0b4c2383cee0bd416e537f49bec791b8409485f8b2fa048ee2eb5500d801f436338b17bd370067023c989a4474e5d68c242c39869fd7beea6f9570efc6e13f214d2dc39bebde00e0df2c49b3b55d9f6b61b840998e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c21800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6edb840ac376d7593b05cd26bfe43472ba0509ec8e96999451c128e7b63864014b9ef9e035912b552ebcdb0ea7f460cd03d489b44f4edde9ecd9bc8a01a121613c99c2bf9018ca0a9a73ffb7dd021174157f7b5276945ad58fba095072dc08e85af99c2fea57d30a09289e80fd8d3aac10bbce9394d3f21eec0d24bd855d9feeccd611e92b2f64b92a0dfc75fff4e9cc84e88c5a0530f5015f15716b5b5c0483ac8633b3136f138d601a0efbafbbbb76f88f058d61f64ee38763a00dbb75d757f1d54a88e96774e80f194a0d67ad8f37492a9435aa92a341e8a7a9c23f27d530c562d32bac7e9d3de3e0fdda0c7ef5bb22c8e9a1389e3f86b761ace2f95c51551900434e6342a8c53440032c1a08688765c30eba2327d752cd0824809213865859ef804186c798c7569838f57e1a0804eca3bc9f5cbb45d9cdb4d0497ddf89aa5f850977d49f067e7af0f6e78f00ba08d4822602d49d924b239a76b69e42896e2876a1c6590df9f7dc618f019646ab7a0a2cc754f361cfba81f8299cc5b0508d9f93e0c4dc4ab0d55145ec45c53ed37bfa0d7d751b94746c3c83f08d966acb6af90384eafd278a452e93b11381da7ecd797a0942da169d23313590bf19bcee90c3ae8ccff13aebe2d9be187186698e4ea44cf"
	mainnetShieldedGroth16RecoveryVKHex  = "0x544b4d473136564b31f90276a0860bac6864942c846a3b43e5f696db6b7e380087932d81db46fc815e21c04e2fb840e0f98d080fbafdc8008b18bab2c45cd455cf3e562facb9c5c5fb432c61b4f30b00648e9ff04c31ef3af9362f84ff375801c22b175c5a5a8cf56b2eea1b928e7ab840998e9393920d483a7260bfb731fb5d25f1aa493335a9e71297e485b7aef312c21800deef121f1e76426a00665e5c4479674322d4f75edadd46debd5cd992f6edb840e14e4a25c51cb8704dbf938cb8e3f4b7b7e054b5e2d64f7904d90b084790378405e470382cdcaefa8a5ba3a7c0bb661f487d26d932b2425e406f72a5c4c437adf9018ca0aa65a5b11381656d464be4fc84c246a113f537c37e7c0bcb848c01c6afe41a22a0efd84662c6e7f04914a014da6f574d38746e5a323f5c8615162edbcfd32cd934a0869f96c7120c8d8a67bb2f13ce8369f74537998ba9d90a55714aec72fb89bd49a08bc24355092d640b854d5005bd887eca23349431261a5dd62aa23788e2ec90dca0d6fa4495afe9e6ea36fff52a0eff9df8bd01255c943a002ea3f798dd7ab9a4a3a0a4b82b5c0c610e0b884c5d70faf2c768c97f242fcef55a9eb31cb1a6bd323dc8a0abdea2978e5a0d3fdb745f0ee9d656987aefc6e57e46c3aa11ccc87f85a6bf7aa0c267bea381461c5401b6d2174822f3d00873319f7bcd26e0081452ef1527982fa0975ee93dc44aad5a1653c56e7f27fed51a70bd91e2817a9e5e8efc932ccc9f85a0cc6430389ce76571d2f63cb6d6ecfa64cdbaaea4e48cce929efb38ff49fcee08a0ccc3b8e398ebf7d7e7fde8b00e8c16eea2dbd2d56a08283c814df3021c01179fa0939543b1f45bca8c6eabe749c06277b40967c375f9af4c7ea09f4a5275c39169"
	mainnetShieldedGroth16VKName         = "mainnet shielded Groth16 verifying key"
	mainnetShieldedGroth16UpgradedVKName = "mainnet upgraded shielded Groth16 verifying key"
)

// MainnetShieldedGroth16VerifyingKey is the audited ceremony output for the
// mainnet shielded spend circuit, encoded with core.EncodeShieldedGroth16VerifyingKey.
//
// It must remain empty until the final circuit has completed independent audit
// and key generation. Mainnet startup refuses an active privacy fork while this
// artifact is missing.
var MainnetShieldedGroth16VerifyingKey = mustDecodeOptionalHex(mainnetShieldedGroth16VKHex)

// MainnetShieldedGroth16UpgradedVerifyingKey is the upgraded verifier artifact
// used by nodes that support transparent-to-shielded deposits and Merkle-root
// checked shielded spends. It is kept outside the stored chain config so nodes
// with the original post-privacy config can restart without a config mismatch.
var MainnetShieldedGroth16UpgradedVerifyingKey = mustDecodeOptionalHex(mainnetShieldedGroth16UpgradedVKHex)

// MainnetShieldedGroth16RecoveryTime activates the recovery ceremony verifier.
const MainnetShieldedGroth16RecoveryTime uint64 = 1787220000

// MainnetShieldedGroth16RecoveryVerifyingKey matches the publicly distributed
// recovery proving key and applies only at and after the recovery timestamp.
var MainnetShieldedGroth16RecoveryVerifyingKey = mustDecodeOptionalHex(mainnetShieldedGroth16RecoveryVKHex)

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
	if err := validateShieldedGroth16VK(c.ShieldedGroth16VerifyingKey, mainnetShieldedGroth16VKName); err != nil {
		return err
	}
	if err := validateShieldedGroth16VK(MainnetShieldedGroth16UpgradedVerifyingKey, mainnetShieldedGroth16UpgradedVKName); err != nil {
		return err
	}
	return validateShieldedGroth16VK(MainnetShieldedGroth16RecoveryVerifyingKey, "mainnet recovery shielded Groth16 verifying key")
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
