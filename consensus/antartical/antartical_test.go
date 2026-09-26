package antartical

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestUserOperationSignature(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	op := &UserOperation{
		Sender:               crypto.PubkeyToAddress(key.PublicKey),
		Nonce:                big.NewInt(1),
		CallGasLimit:         big.NewInt(100000),
		VerificationGasLimit: big.NewInt(100000),
		PreVerificationGas:   big.NewInt(1000),
		MaxFeePerGas:         big.NewInt(10),
		MaxPriorityFeePerGas: big.NewInt(1),
	}
	hash, err := op.Hash(big.NewInt(8979), common.HexToAddress("0x100"))
	if err != nil {
		t.Fatal(err)
	}
	op.Signature, err = crypto.Sign(hash.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.VerifySignature(big.NewInt(8979), common.HexToAddress("0x100")); err != nil {
		t.Fatal(err)
	}
}

func TestBuildExecutionWaves(t *testing.T) {
	a, b := common.HexToAddress("0x1"), common.HexToAddress("0x2")
	waves := BuildExecutionWaves([]AccessSet{{Reads: []common.Address{a}}, {Reads: []common.Address{b}}, {Unknown: true}})
	if len(waves) != 2 || len(waves[0]) != 2 || len(waves[1]) != 1 {
		t.Fatalf("unexpected waves: %#v", waves)
	}
}

func TestGasWitnessAndEOF(t *testing.T) {
	var used GasVector
	limit := GasVector{Execution: 10, StateRead: 5, StateWrite: 5, Blob: 2}
	if err := used.Charge(&limit, GasVector{Execution: 3, StateRead: 2}); err != nil {
		t.Fatal(err)
	}
	witness := StateWitness{Root: common.HexToHash("0x1"), Nodes: [][]byte{{1}, {2}}}
	commitment, err := witness.Commitment()
	if err != nil || !witness.Verify(commitment) {
		t.Fatalf("witness commitment failed: %v", err)
	}
	if err := ValidateEOF([]byte{0xef, 0x00, 0x01, 0x01, 0x00, 0x01, 0xaa, 0x02, 0x00, 0x01, 0x60}); err != nil {
		t.Fatal(err)
	}
}

func TestOracleCrossChainAndFinality(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	obs := OracleObservation{FeedID: common.HexToHash("0x1"), Round: 1, Value: []byte("42"), Timestamp: 1, Signer: crypto.PubkeyToAddress(key.PublicKey)}
	hash, err := obs.Hash(big.NewInt(8979))
	if err != nil {
		t.Fatal(err)
	}
	obs.Signature, err = crypto.Sign(hash.Bytes(), key)
	if err != nil || obs.Verify(big.NewInt(8979)) != nil {
		t.Fatal("oracle signature rejected")
	}
	message := CrossChainMessage{SourceChainID: big.NewInt(8979), DestinationChainID: big.NewInt(8980), Nonce: 1, Sender: obs.Signer, Target: common.HexToAddress("0x2"), Payload: []byte("payload")}
	if _, err := message.ReplayKey(); err != nil {
		t.Fatal(err)
	}
	blockHash := common.HexToHash("0xabc")
	sig, err := crypto.Sign(blockHash.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert := FinalityCertificate{Slot: 1, BlockHash: blockHash, Signers: []common.Address{obs.Signer}, PublicKeys: [][]byte{crypto.FromECDSAPub(&key.PublicKey)}, Signatures: [][]byte{sig}}
	if err := cert.Verify(1, 1); err != nil {
		t.Fatal(err)
	}
}

type testEngine struct {
	name string
	out  ExecutionOutput
}

func (e testEngine) Name() string { return e.name }

func (e testEngine) Execute(ExecutionInput) (ExecutionOutput, error) { return e.out, nil }

func TestCompareEngines(t *testing.T) {
	out := ExecutionOutput{StateRoot: common.HexToHash("0x1"), ReceiptsRoot: common.HexToHash("0x2"), ProofDigest: common.HexToHash("0x3")}
	input := ExecutionInput{ParentStateRoot: common.HexToHash("0x4"), BlockHash: common.HexToHash("0x5")}
	registry, err := NewEngineRegistry(testEngine{"go", out})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(testEngine{"revm", out}); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Get("revm"); !ok {
		t.Fatal("registered engine missing")
	}
	if err := CompareEngines(testEngine{"go", out}, testEngine{"revm", out}, input); err != nil {
		t.Fatal(err)
	}
	if err := CompareEngines(testEngine{"go", out}, testEngine{"evmone", ExecutionOutput{}}, input); err != ErrExecutionEngineMismatch {
		t.Fatalf("mismatch error = %v", err)
	}
}
