package recipient

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/ethereum/go-ethereum/zk/shielded6"
)

func TestRecipientProofAllocationGuard(t *testing.T) {
	var encoded bytes.Buffer
	if _, err := groth16.NewProof(ecc.BN254).WriteTo(&encoded); err != nil {
		t.Fatal(err)
	}
	proof := encoded.Bytes()
	if !validProofEncoding(proof) {
		t.Fatal("guard does not match the pinned gnark compressed encoding")
	}
	forged := append([]byte(nil), proof...)
	binary.BigEndian.PutUint32(forged[128:132], ^uint32(0))
	if validProofEncoding(forged) {
		t.Fatal("forged allocation prefix accepted")
	}
	for _, bad := range [][]byte{nil, proof[:127], proof[:len(proof)-1], append(append([]byte(nil), proof...), 0), make([]byte, MaxProofBytes+1)} {
		if validProofEncoding(bad) {
			t.Fatal("invalid proof shape accepted")
		}
	}
	uncompressed := append([]byte(nil), proof...)
	uncompressed[0] &= 0x3f
	if validProofEncoding(uncompressed) {
		t.Fatal("uncompressed proof point accepted")
	}
}
func FuzzRecipientProofAllocationGuard(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, 164))
	f.Add(bytes.Repeat([]byte{0xff}, MaxProofBytes))
	f.Fuzz(func(t *testing.T, data []byte) {
		if validProofEncoding(data) && (len(data) < 164 || len(data) > MaxProofBytes) {
			t.Fatal("invalid bounded shape accepted")
		}
	})
}

// This tiny circuit tests backend serialization/commitment metadata only;
// it is not the recipient relation and its ephemeral key is never deployed.
type verifierMetadataCircuit struct {
	Public [32]frontend.Variable `gnark:",public"`
	Secret frontend.Variable
}

func (c *verifierMetadataCircuit) Define(api frontend.API) error {
	var sum frontend.Variable = 0
	for _, v := range c.Public {
		api.ToBinary(v, 8)
		sum = api.Add(sum, v)
	}
	api.AssertIsEqual(c.Secret, sum)
	commitment, err := api.Compiler().(frontend.Committer).Commit(c.Secret, c.Public[0])
	if err != nil {
		return err
	}
	api.AssertIsDifferent(commitment, 0)
	return nil
}
func TestRecipientVerifierCommitmentMetadata(t *testing.T) {
	s := Statement{ChainID: 18979, AssetID: 1, Output: shielded6.OutputRecord{EncryptedNote: []byte{1}}}
	digest, err := s.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, &verifierMetadataCircuit{})
	if err != nil {
		t.Fatal(err)
	}
	pk, vk, err := groth16.Setup(cs)
	if err != nil {
		t.Fatal(err)
	}
	c := &verifierMetadataCircuit{}
	var sum uint64
	for i, v := range digest {
		c.Public[i] = v
		sum += uint64(v)
	}
	c.Secret = sum
	witness, err := frontend.NewWitness(c, ecc.BN254.ScalarField())
	if err != nil {
		t.Fatal(err)
	}
	proof, err := groth16.Prove(cs, pk, witness)
	if err != nil {
		t.Fatal(err)
	}
	var keyBytes, proofBytes bytes.Buffer
	if _, err := vk.WriteTo(&keyBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := proof.WriteTo(&proofBytes); err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(keyBytes.Bytes(), sha256.Sum256(keyBytes.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if verifier.commitments == 0 {
		t.Fatal("fixture has no internal commitment")
	}
	if err := verifier.Verify(s, proofBytes.Bytes()); err != nil {
		t.Fatal(err)
	}
	var concurrent sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		concurrent.Add(1)
		go func() { defer concurrent.Done(); failures <- verifier.Verify(s, proofBytes.Bytes()) }()
	}
	concurrent.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !validProofEncoding(proofBytes.Bytes()) {
		t.Fatal("encoding guard rejected real gnark proof")
	}
	// A shorter commitment list used to cause a verifier indexing panic in
	// the dependency. Reject it before decoding or calling the dependency.
	short := append([]byte(nil), proofBytes.Bytes()[:128]...)
	short = binary.BigEndian.AppendUint32(short, 0)
	short = append(short, proofBytes.Bytes()[len(proofBytes.Bytes())-32:]...)
	if err := verifier.Verify(s, short); err == nil {
		t.Fatal("accepted wrong commitment count")
	}
}
