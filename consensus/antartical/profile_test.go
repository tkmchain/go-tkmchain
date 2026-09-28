package antartical

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tkmasset"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
)

func TestTypedTransactionDomainBindsChainAndContract(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	domain := TypedTransactionDomain{ChainID: big.NewInt(8980), Contract: common.HexToAddress("0x100"), Type: 1}
	digest, err := domain.Digest([]byte("mint"))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	tx := TypedTransaction{Sender: crypto.PubkeyToAddress(key.PublicKey), Domain: domain, Payload: []byte("mint"), Signature: sig}
	if err := tx.Verify(); err != nil {
		t.Fatalf("secp typed transaction rejected: %v", err)
	}
	if err := tx.VerifyAtVersion(LegacyProfileVersion); err != ErrProfileInactive {
		t.Fatalf("pre-fork typed transaction error = %v", err)
	}
	if err := tx.VerifyAtVersion(AntarticalProfileVersion); err != nil {
		t.Fatalf("active typed transaction rejected: %v", err)
	}
	tx.Domain.Contract = common.HexToAddress("0x101")
	if err := tx.Verify(); err == nil {
		t.Fatal("contract-bound signature accepted for a different contract")
	}
}

func TestTypedTransactionDomainMLDSASenderPolicy(t *testing.T) {
	seed := make([]byte, pqcrypto.MLDSA87SeedSize)
	seed[0] = 7
	key, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := pqcrypto.PublicKeyBytes(key)
	sender, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	domain := TypedTransactionDomain{ChainID: big.NewInt(8980), Contract: common.HexToAddress("0x100"), Type: 2}
	digest, err := domain.Digest([]byte("shield"))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := pqcrypto.SignMLDSA87(key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	tx := TypedTransaction{Sender: sender, Domain: domain, Payload: []byte("shield"), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: publicKey, Signature: signature}
	if err := tx.Verify(); err != nil {
		t.Fatalf("ML-DSA typed transaction rejected: %v", err)
	}
	tx.Payload = []byte("changed")
	if err := tx.Verify(); err == nil {
		t.Fatal("ML-DSA signature accepted for changed payload")
	}
}

func TestTokenPolicyEnforcesCapabilitiesAndRoyalty(t *testing.T) {
	admin := common.HexToAddress("0x1")
	issuer := common.HexToAddress("0x2")
	recipient := common.HexToAddress("0x3")
	royalty := common.HexToAddress("0x4")
	policy := TokenPolicy{AssetID: common.HexToHash("0x11"), Admin: admin, Flags: tkmasset.FlagMintable | tkmasset.FlagBurnable | tkmasset.FlagPausable | tkmasset.FlagRoyalty | tkmasset.FlagShielded, MaxSupply: big.NewInt(1000), RoyaltyBPS: 500, RoyaltyRecipient: royalty}
	policyHash, err := policy.Commitment()
	if err != nil {
		t.Fatal(err)
	}
	manifest := tkmasset.Manifest{Kind: tkmasset.KindFungible, ChainID: big.NewInt(8980), Flags: policy.Flags, PolicyHash: policyHash, Name: "Policy test", Symbol: "POL", MetadataURI: "ipfs://tkm/policy"}
	if err := policy.ValidateManifest(manifest); err != nil {
		t.Fatalf("policy commitment did not bind manifest: %v", err)
	}
	manifest.Flags ^= tkmasset.FlagShielded
	if err := policy.ValidateManifest(manifest); err == nil {
		t.Fatal("manifest capability substitution accepted")
	}
	manifest.Flags = policy.Flags
	state := TokenState{}
	if err := state.ApplyAtVersion(policy, TokenOperation{Kind: TokenMint, Actor: admin, To: issuer, Amount: big.NewInt(1)}, LegacyProfileVersion); err != ErrProfileInactive {
		t.Fatalf("pre-fork token policy error = %v", err)
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenMint, Actor: admin, To: issuer, Amount: big.NewInt(1000)}); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenTransfer, Actor: issuer, From: issuer, To: recipient, Amount: big.NewInt(100)}); err != nil {
		t.Fatal(err)
	}
	if state.Balances[recipient].Cmp(big.NewInt(95)) != 0 || state.Balances[royalty].Cmp(big.NewInt(5)) != 0 {
		t.Fatalf("royalty not enforced: recipient=%v royalty=%v", state.Balances[recipient], state.Balances[royalty])
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenPause, Actor: admin}); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenTransfer, Actor: issuer, From: issuer, To: recipient, Amount: big.NewInt(1)}); err == nil {
		t.Fatal("transfer succeeded while token was paused")
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenUnpause, Actor: admin}); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenShield, Actor: issuer, Amount: big.NewInt(1)}); err != nil {
		t.Fatal(err)
	}
	if err := state.Apply(policy, TokenOperation{Kind: TokenMint, Actor: issuer, To: issuer, Amount: big.NewInt(1)}); err == nil {
		t.Fatal("non-admin mint succeeded")
	}
}

func TestAssetRegistryAndHeaderCommitment(t *testing.T) {
	entries := []AssetRegistryEntry{
		{AssetID: common.HexToHash("0x02"), ManifestHash: common.HexToHash("0x12"), RuntimeCodeHash: common.HexToHash("0x22")},
		{AssetID: common.HexToHash("0x01"), ManifestHash: common.HexToHash("0x11"), RuntimeCodeHash: common.HexToHash("0x21")},
	}
	first, err := AssetRegistryCommitment(entries)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AssetRegistryCommitment([]AssetRegistryEntry{entries[1], entries[0]})
	if err != nil || first != second {
		t.Fatalf("registry root is order-dependent: %s %s", first, second)
	}
	extra := AttachAssetRegistryCommitment([]byte("header-extra"), first)
	got, found, err := AssetRegistryCommitmentFromHeaderExtra(extra)
	if err != nil || !found || got != first {
		t.Fatalf("header registry root mismatch: found=%t got=%s want=%s err=%v", found, got, first, err)
	}
	extra[len(extra)-1] ^= 1
	if got, _, _ := AssetRegistryCommitmentFromHeaderExtra(extra); got == first {
		t.Fatal("tampered registry root was accepted")
	}
	legacyExtra, err := AttachAssetRegistryCommitmentAtVersion([]byte("header-extra"), first, LegacyProfileVersion)
	if err != nil || string(legacyExtra) != "header-extra" {
		t.Fatalf("legacy header metadata changed: %v", err)
	}
	activeExtra, err := AttachAssetRegistryCommitmentAtVersion([]byte("header-extra"), first, AntarticalProfileVersion)
	if err != nil || len(activeExtra) <= len(legacyExtra) {
		t.Fatalf("active header metadata was not appended: %v", err)
	}
}

func TestShieldedAssetBindingAndProtocolGas(t *testing.T) {
	asset := common.HexToHash("0x100")
	token := common.HexToHash("0x200")
	nullifier := common.HexToHash("0x300")
	binding := ShieldedAssetNullifierBinding(8980, asset, token, nullifier)
	if err := ValidateShieldedAssetNullifierBinding(8980, asset, token, nullifier, binding); err != nil {
		t.Fatal(err)
	}
	if err := ValidateShieldedAssetNullifierBinding(8980, asset, common.HexToHash("0x201"), nullifier, binding); err == nil {
		t.Fatal("token ID substitution accepted by shielded binding")
	}
	limit := ProtocolGasVector{EVM: 10, TVM: 10, Proof: 10, Blob: 10}
	used := ProtocolGasVector{}
	firstCharge := ProtocolGasVector{EVM: 2, TVM: 3, Proof: 4, Blob: 1}
	if err := used.Charge(&limit, firstCharge); err != nil {
		t.Fatal(err)
	}
	used = used.Add(firstCharge)
	if err := used.Charge(&limit, ProtocolGasVector{Proof: 7}); err == nil {
		t.Fatal("proof dimension over-consumption accepted")
	}
	if err := (ProtocolGasVector{EVM: ^uint64(0)}).Charge(&ProtocolGasVector{EVM: ^uint64(0)}, ProtocolGasVector{EVM: 1}); err == nil {
		t.Fatal("gas counter overflow accepted")
	}
}

func TestConflictTranscriptIsDeterministic(t *testing.T) {
	access := []AccessSet{{Reads: []common.Address{common.HexToAddress("0x1")}}, {Reads: []common.Address{common.HexToAddress("0x2")}}, {Unknown: true}}
	transcript := NewConflictTranscript(access)
	commitment, err := transcript.Commitment()
	if err != nil || !transcript.Verify(access, commitment) {
		t.Fatalf("conflict transcript verification failed: %v", err)
	}
	access[0].Reads[0] = common.HexToAddress("0x3")
	if transcript.Verify(access, commitment) {
		t.Fatal("changed access set retained the old conflict commitment")
	}
}

func TestReceiptTranscriptBindsReceiptIndex(t *testing.T) {
	access := []AccessSet{{Reads: []common.Address{common.HexToAddress("0x1")}}, {Reads: []common.Address{common.HexToAddress("0x2")}}}
	transcript := NewConflictTranscript(access)
	metadata, err := transcript.ReceiptMetadata(4)
	if err != nil || !metadata.Verify(transcript, 4) {
		t.Fatalf("receipt transcript metadata failed: %v", err)
	}
	if metadata.Verify(transcript, 5) {
		t.Fatal("receipt transcript accepted for a different receipt index")
	}
	if _, err := transcript.ReceiptMetadataAtVersion(4, LegacyProfileVersion); err != ErrProfileInactive {
		t.Fatalf("pre-fork receipt metadata error = %v", err)
	}
	if _, err := transcript.ReceiptMetadataAtVersion(4, AntarticalProfileVersion); err != nil {
		t.Fatal(err)
	}
	transcript.Waves[0][0] = 2
	if _, err := transcript.Commitment(); err == nil {
		t.Fatal("non-canonical conflict wave was committed")
	}
}

func TestCanonicalStateWitnessCommitmentIsOrderIndependent(t *testing.T) {
	a := StateWitness{Root: common.HexToHash("0x1"), Nodes: [][]byte{{3}, {1}, {2}}}
	b := StateWitness{Root: a.Root, Nodes: [][]byte{{2}, {3}, {1}}}
	first, err := a.CanonicalCommitment()
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.CanonicalCommitment()
	if err != nil || first != second || !a.VerifyCanonical(first) {
		t.Fatalf("canonical witness mismatch: %s %s %v", first, second, err)
	}
	legacy, err := a.CommitmentAtVersion(LegacyProfileVersion)
	if err != nil || legacy == first {
		t.Fatal("legacy witness did not retain its historical encoding")
	}
	active, err := a.CommitmentAtVersion(AntarticalProfileVersion)
	if err != nil || active != first {
		t.Fatal("active witness did not select canonical encoding")
	}
}

func TestStatelessLightClientProof(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	blockHash := common.HexToHash("0x1234")
	witness := StateWitness{Root: common.HexToHash("0x5678"), Nodes: [][]byte{{3}, {1}, {2}}}
	commitment, err := witness.CanonicalCommitment()
	if err != nil {
		t.Fatal(err)
	}
	finalityDigest, err := LightClientFinalityDigest(big.NewInt(8980), blockHash)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := crypto.Sign(finalityDigest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	proof := StatelessLightClientProof{
		ChainID: big.NewInt(8980), BlockNumber: 42, BlockHash: blockHash, StateRoot: witness.Root,
		WitnessCommitment: commitment, Witness: witness,
		Certificate: FinalityCertificate{Slot: 42, BlockHash: finalityDigest, CommitteeSize: 1, Signers: []common.Address{crypto.PubkeyToAddress(key.PublicKey)}, PublicKeys: [][]byte{crypto.FromECDSAPub(&key.PublicKey)}, Signatures: [][]byte{signature}},
	}
	if err := proof.Verify(1, 1); err != nil {
		t.Fatal(err)
	}
	if err := proof.VerifyAtVersion(LegacyProfileVersion, 1, 1); err != ErrProfileInactive {
		t.Fatalf("pre-fork light-client error = %v", err)
	}
	if err := proof.VerifyAtVersion(AntarticalProfileVersion, 1, 1); err != nil {
		t.Fatal(err)
	}
	proof.Witness.Nodes[0][0] = 4
	if err := proof.Verify(1, 1); err == nil {
		t.Fatal("mutated stateless witness accepted")
	}
}
