// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.

// egypt-contract-test deploys small deterministic EVM fixtures with the Egypt
// chain configuration. It does not connect to or modify a live node.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/consensus/rotatingking"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tkmasset"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/pqcrypto"
	"github.com/ethereum/go-ethereum/params"
)

type assembler struct {
	code   []byte
	labels map[string]int
	fixups []struct {
		at    int
		label string
	}
}

func newAssembler() *assembler { return &assembler{labels: make(map[string]int)} }
func (a *assembler) op(ops ...vm.OpCode) {
	for _, op := range ops {
		a.code = append(a.code, byte(op))
	}
}
func (a *assembler) push1(v byte)   { a.code = append(a.code, byte(vm.PUSH1), v) }
func (a *assembler) push2(v uint16) { a.code = append(a.code, byte(vm.PUSH2), byte(v>>8), byte(v)) }
func (a *assembler) push4(v []byte) {
	a.code = append(a.code, byte(vm.PUSH4))
	a.code = append(a.code, v...)
}
func (a *assembler) label(name string) { a.labels[name] = len(a.code); a.op(vm.JUMPDEST) }
func (a *assembler) jump(name string) {
	a.code = append(a.code, byte(vm.PUSH2), 0, 0)
	a.fixups = append(a.fixups, struct {
		at    int
		label string
	}{len(a.code) - 2, name})
	a.op(vm.JUMPI)
}
func (a *assembler) resolve() []byte {
	for _, fixup := range a.fixups {
		pc, ok := a.labels[fixup.label]
		if !ok || pc > 0xffff {
			panic("invalid assembler label")
		}
		a.code[fixup.at] = byte(pc >> 8)
		a.code[fixup.at+1] = byte(pc)
	}
	return append([]byte(nil), a.code...)
}

func selector(signature string) []byte { return crypto.Keccak256([]byte(signature))[:4] }

func dispatch(a *assembler, entries ...struct{ sig, label string }) {
	a.push1(0)
	a.op(vm.CALLDATALOAD)
	a.push1(224)
	a.op(vm.SHR)
	for _, entry := range entries {
		a.op(vm.DUP1)
		a.push4(selector(entry.sig))
		a.op(vm.EQ)
		a.jump(entry.label)
	}
	a.op(vm.POP)
	a.push1(0)
	a.push1(0)
	a.op(vm.REVERT)
}

func returnWord(a *assembler) {
	a.push1(0)
	a.op(vm.MSTORE)
	a.push1(32)
	a.push1(0)
	a.op(vm.RETURN)
}

func tokenRuntime() []byte {
	a := newAssembler()
	dispatch(a,
		struct{ sig, label string }{"totalSupply()", "total"},
		struct{ sig, label string }{"balanceOf(address)", "balance"},
		struct{ sig, label string }{"mint(address,uint256)", "mint"},
		struct{ sig, label string }{"transfer(address,uint256)", "transfer"},
	)
	a.label("total")
	a.op(vm.POP)
	a.push1(0)
	a.op(vm.SLOAD)
	returnWord(a)
	a.label("balance")
	a.op(vm.POP)
	a.push1(4)
	a.op(vm.CALLDATALOAD)
	a.push1(0)
	a.op(vm.MSTORE)
	a.push1(1)
	a.push1(32)
	a.op(vm.MSTORE)
	a.push1(64)
	a.push1(0)
	a.op(vm.KECCAK256, vm.SLOAD)
	returnWord(a)
	a.label("mint")
	a.op(vm.POP)
	// balanceOf storage key = keccak256(padded recipient || uint256(1)).
	a.push1(4)
	a.op(vm.CALLDATALOAD)
	a.push1(0)
	a.op(vm.MSTORE)
	a.push1(1)
	a.push1(32)
	a.op(vm.MSTORE)
	a.push1(64)
	a.push1(0)
	a.op(vm.KECCAK256)
	a.push1(36)
	a.op(vm.CALLDATALOAD)
	a.op(vm.SWAP1, vm.SSTORE)
	// totalSupply += amount
	a.push1(36)
	a.op(vm.CALLDATALOAD)
	a.push1(0)
	a.op(vm.SLOAD)
	a.op(vm.ADD)
	a.push1(0)
	a.op(vm.SSTORE)
	a.op(vm.STOP)
	a.label("transfer")
	a.op(vm.POP)
	// Keep the recipient and amount in memory while deriving the same
	// balanceOf(address) storage key used by mint and balanceOf. The caller is
	// the sender for this direct runtime call.
	a.push1(4)
	a.op(vm.CALLDATALOAD)
	a.push1(0)
	a.op(vm.MSTORE)
	a.push1(36)
	a.op(vm.CALLDATALOAD)
	a.push1(32)
	a.op(vm.MSTORE)
	// sender balance -= amount. The fixture invokes the token directly, so
	// ORIGIN is the configured sender for this deterministic call.
	a.op(vm.ORIGIN)
	a.push1(64)
	a.op(vm.MSTORE)
	a.push1(1)
	a.push1(96)
	a.op(vm.MSTORE)
	a.push1(64)
	a.push1(64)
	a.op(vm.KECCAK256, vm.SLOAD)
	a.push1(32)
	a.op(vm.MLOAD)
	a.op(vm.SWAP1)
	a.op(vm.SUB)
	// SSTORE consumes [value, slot].
	a.op(vm.ORIGIN)
	a.push1(64)
	a.op(vm.MSTORE)
	a.push1(1)
	a.push1(96)
	a.op(vm.MSTORE)
	a.push1(64)
	a.push1(64)
	a.op(vm.KECCAK256)
	a.op(vm.SSTORE)
	// recipient balance += amount
	a.push1(0)
	a.op(vm.MLOAD)
	a.push1(64)
	a.op(vm.MSTORE)
	a.push1(1)
	a.push1(96)
	a.op(vm.MSTORE)
	a.push1(64)
	a.push1(64)
	a.op(vm.KECCAK256, vm.SLOAD)
	a.push1(32)
	a.op(vm.MLOAD)
	a.op(vm.ADD)
	a.push1(0)
	a.op(vm.MLOAD)
	a.push1(64)
	a.op(vm.MSTORE)
	a.push1(1)
	a.push1(96)
	a.op(vm.MSTORE)
	a.push1(64)
	a.push1(64)
	a.op(vm.KECCAK256)
	a.op(vm.SSTORE)
	// Return the ERC-20 success value.
	a.push1(1)
	a.push1(0)
	a.op(vm.MSTORE)
	a.push1(32)
	a.push1(0)
	a.op(vm.RETURN)
	return a.resolve()
}

func counterRuntime() []byte {
	a := newAssembler()
	dispatch(a,
		struct{ sig, label string }{"get()", "get"},
		struct{ sig, label string }{"set(uint256)", "set"},
	)
	a.label("get")
	a.op(vm.POP)
	a.push1(0)
	a.op(vm.SLOAD)
	returnWord(a)
	a.label("set")
	a.op(vm.POP)
	a.push1(4)
	a.op(vm.CALLDATALOAD)
	a.push1(0)
	a.op(vm.SSTORE)
	a.op(vm.STOP)
	return a.resolve()
}

func constructor(runtimeCode []byte) []byte {
	return program.New().ReturnViaCodeCopy(runtimeCode).Bytes()
}

func wordAddress(address common.Address) []byte { return common.LeftPadBytes(address.Bytes(), 32) }
func wordUint(value *big.Int) []byte            { return common.LeftPadBytes(value.Bytes(), 32) }
func call(sig string, args ...[]byte) []byte {
	out := append([]byte(nil), selector(sig)...)
	for _, arg := range args {
		out = append(out, arg...)
	}
	return out
}

type result struct {
	Network                    string         `json:"network"`
	ChainID                    string         `json:"chainId"`
	TokenName                  string         `json:"tokenName"`
	TokenSymbol                string         `json:"tokenSymbol"`
	TokenDecimals              uint8          `json:"tokenDecimals"`
	TokenStandard              string         `json:"tokenStandard"`
	TokenAddress               string         `json:"tokenAddress"`
	TokenRuntimeHash           string         `json:"tokenRuntimeHash"`
	TokenManifestHash          string         `json:"tokenManifestHash"`
	TokenAssetID               string         `json:"tokenAssetId"`
	TokenPrecompileAssetID     string         `json:"tokenPrecompileAssetId"`
	TokenManifestVerified      bool           `json:"tokenManifestVerified"`
	TokenBalance               string         `json:"tokenBalance"`
	TokenBalanceAfterTransfer  string         `json:"tokenBalanceAfterTransfer"`
	TransferRecipient          string         `json:"transferRecipient"`
	TransferAmount             string         `json:"transferAmount"`
	RecipientBalance           string         `json:"recipientBalance"`
	TransferGasUsed            uint64         `json:"transferGasUsed"`
	TokenTotalSupply           string         `json:"tokenTotalSupply"`
	CounterAddress             string         `json:"counterAddress"`
	CounterRuntimeHash         string         `json:"counterRuntimeHash"`
	CounterValue               string         `json:"counterValue"`
	TokenDeployGas             uint64         `json:"tokenDeployGas"`
	CounterDeployGas           uint64         `json:"counterDeployGas"`
	FeatureChecks              []featureCheck `json:"featureChecks"`
	ProtocolChecks             []string       `json:"protocolChecks"`
	ProfileChecks              []string       `json:"profileChecks"`
	AssetRegistryRoot          string         `json:"assetRegistryRoot"`
	ShieldedAssetBinding       string         `json:"shieldedAssetBinding"`
	CanonicalWitnessCommitment string         `json:"canonicalWitnessCommitment"`
	LightClientBlockHash       string         `json:"lightClientBlockHash"`
	ConflictTranscript         string         `json:"conflictTranscript"`
	ReceiptTranscript          string         `json:"receiptTranscript"`
	PrivacyChecks              []string       `json:"privacyChecks"`
	Shield3Gas                 uint64         `json:"shield3Gas"`
	Shield4Gas                 uint64         `json:"shield4Gas"`
	ZKEVMClaim                 string         `json:"zkevmClaim"`
	StateWitnessCommit         string         `json:"stateWitnessCommitment"`
	RotatingKingChecks         []string       `json:"rotatingKingChecks"`
	RotatingKingRegistration   string         `json:"rotatingKingRegistration"`
	RotatingKingActivation     uint64         `json:"rotatingKingActivation"`
}

type profileOutput struct {
	Checks                     []string
	TypedDomainDigest          common.Hash
	AssetRegistryRoot          common.Hash
	ShieldedAssetBinding       common.Hash
	CanonicalWitnessCommitment common.Hash
	LightClientBlockHash       common.Hash
	ConflictTranscript         common.Hash
	ReceiptTranscript          common.Hash
}

func runProfileChecks(chainID *big.Int, assetID, manifestHash, runtimeHash common.Hash, tokenOwner common.Address, profileVersion uint8) profileOutput {
	result := profileOutput{}
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(fmt.Errorf("profile key: %w", err))
	}
	domain := antartical.TypedTransactionDomain{ChainID: new(big.Int).Set(chainID), Contract: common.HexToAddress("0x100"), Type: 1}
	digest, err := domain.Digest([]byte("EUSD transfer"))
	if err != nil {
		panic(fmt.Errorf("typed transaction domain: %w", err))
	}
	signature, err := crypto.Sign(digest.Bytes(), key)
	if err != nil {
		panic(fmt.Errorf("typed transaction signature: %w", err))
	}
	typed := antartical.TypedTransaction{Sender: crypto.PubkeyToAddress(key.PublicKey), Domain: domain, Payload: []byte("EUSD transfer"), Signature: signature}
	if err := typed.VerifyAtVersion(profileVersion); err != nil {
		panic(fmt.Errorf("typed transaction verification: %w", err))
	}
	result.TypedDomainDigest = digest
	result.Checks = append(result.Checks, "chain-and-contract-bound-typed-signature")

	seed := make([]byte, pqcrypto.MLDSA87SeedSize)
	seed[0] = 9
	pqKey, err := pqcrypto.NewMLDSA87FromSeed(seed)
	if err != nil {
		panic(fmt.Errorf("profile ML-DSA key: %w", err))
	}
	pqPublic := pqcrypto.PublicKeyBytes(pqKey)
	pqSender, err := pqcrypto.Address(pqcrypto.AlgorithmMLDSA87, pqPublic)
	if err != nil {
		panic(fmt.Errorf("profile ML-DSA address: %w", err))
	}
	pqDomain := antartical.TypedTransactionDomain{ChainID: new(big.Int).Set(chainID), Contract: common.HexToAddress("0x101"), Type: 2}
	pqDigest, err := pqDomain.Digest([]byte("EUSD shield"))
	if err != nil {
		panic(fmt.Errorf("PQ typed transaction domain: %w", err))
	}
	pqSignature, err := pqcrypto.SignMLDSA87(pqKey, pqDigest[:])
	if err != nil || (antartical.TypedTransaction{Sender: pqSender, Domain: pqDomain, Payload: []byte("EUSD shield"), Algorithm: pqcrypto.AlgorithmMLDSA87, PublicKey: pqPublic, Signature: pqSignature}).VerifyAtVersion(profileVersion) != nil {
		panic(fmt.Errorf("PQ typed transaction verification: %w", err))
	}
	result.Checks = append(result.Checks, "post-quantum-sender-policy")

	policy := antartical.TokenPolicy{AssetID: assetID, Admin: tokenOwner, Flags: tkmasset.FlagMintable | tkmasset.FlagBurnable | tkmasset.FlagPausable | tkmasset.FlagRoyalty | tkmasset.FlagShielded, MaxSupply: big.NewInt(1_000_000), RoyaltyBPS: 25, RoyaltyRecipient: common.HexToAddress("0x4")}
	policyHash, err := policy.Commitment()
	if err != nil {
		panic(fmt.Errorf("token policy commitment: %w", err))
	}
	policyManifest := tkmasset.Manifest{Kind: tkmasset.KindFungible, ChainID: new(big.Int).Set(chainID), Flags: policy.Flags, PolicyHash: policyHash, Name: "Egypt policy fixture", Symbol: "EPOL", MetadataURI: "ipfs://tkm/egypt/policy"}
	if err := policy.ValidateManifest(policyManifest); err != nil {
		panic(fmt.Errorf("token policy manifest binding: %w", err))
	}
	result.Checks = append(result.Checks, "token-policy-manifest-binding")
	state := antartical.TokenState{}
	if err := state.ApplyAtVersion(policy, antartical.TokenOperation{Kind: antartical.TokenMint, Actor: tokenOwner, To: tokenOwner, Amount: big.NewInt(1000)}, profileVersion); err != nil {
		panic(fmt.Errorf("token mint policy: %w", err))
	}
	if err := state.ApplyAtVersion(policy, antartical.TokenOperation{Kind: antartical.TokenTransfer, Actor: tokenOwner, From: tokenOwner, To: common.HexToAddress("0x3"), Amount: big.NewInt(100)}, profileVersion); err != nil {
		panic(fmt.Errorf("token royalty policy: %w", err))
	}
	if err := state.ApplyAtVersion(policy, antartical.TokenOperation{Kind: antartical.TokenPause, Actor: tokenOwner}, profileVersion); err != nil {
		panic(fmt.Errorf("token pause policy: %w", err))
	}
	if err := state.ApplyAtVersion(policy, antartical.TokenOperation{Kind: antartical.TokenUnpause, Actor: tokenOwner}, profileVersion); err != nil {
		panic(fmt.Errorf("token unpause policy: %w", err))
	}
	if err := state.ApplyAtVersion(policy, antartical.TokenOperation{Kind: antartical.TokenBurn, Actor: tokenOwner, From: tokenOwner, Amount: big.NewInt(10)}, profileVersion); err != nil {
		panic(fmt.Errorf("token burn policy: %w", err))
	}
	if err := state.ApplyAtVersion(policy, antartical.TokenOperation{Kind: antartical.TokenShield, Actor: tokenOwner, Amount: big.NewInt(1)}, profileVersion); err != nil {
		panic(fmt.Errorf("token shield policy: %w", err))
	}
	result.Checks = append(result.Checks, "mint-burn-pause-royalty-shield-policy")

	registryRoot, err := antartical.AssetRegistryCommitment([]antartical.AssetRegistryEntry{{AssetID: assetID, ManifestHash: manifestHash, RuntimeCodeHash: runtimeHash}})
	if err != nil {
		panic(fmt.Errorf("asset registry commitment: %w", err))
	}
	extra, err := antartical.AttachAssetRegistryCommitmentAtVersion([]byte("egypt-header"), registryRoot, profileVersion)
	if got, found, err := antartical.AssetRegistryCommitmentFromHeaderExtra(extra); err != nil || !found || got != registryRoot {
		panic(fmt.Errorf("asset registry header commitment: found=%t got=%s want=%s err=%v", found, got, registryRoot, err))
	}
	result.AssetRegistryRoot = registryRoot
	result.Checks = append(result.Checks, "canonical-asset-registry-header-commitment")

	tokenID := common.HexToHash("0x1234")
	nullifier := common.HexToHash("0x5678")
	result.ShieldedAssetBinding = antartical.ShieldedAssetNullifierBinding(chainID.Uint64(), assetID, tokenID, nullifier)
	if err := antartical.ValidateShieldedAssetNullifierBinding(chainID.Uint64(), assetID, tokenID, nullifier, result.ShieldedAssetBinding); err != nil {
		panic(fmt.Errorf("shielded asset binding: %w", err))
	}
	result.Checks = append(result.Checks, "shield3-shield4-asset-nullifier-binding")

	limit := antartical.ProtocolGasVector{EVM: 100, TVM: 100, Proof: 100, Blob: 100}
	used := antartical.ProtocolGasVector{}
	charge := antartical.ProtocolGasVector{EVM: 10, TVM: 20, Proof: 30, Blob: 4}
	if err := used.Charge(&limit, charge); err != nil || !used.Add(charge).Fits(limit) {
		panic(fmt.Errorf("protocol gas vector: %w", err))
	}
	result.Checks = append(result.Checks, "evm-tvm-proof-blob-gas-accounting")

	access := []antartical.AccessSet{{Reads: []common.Address{common.HexToAddress("0x1")}}, {Reads: []common.Address{common.HexToAddress("0x2")}}, {Unknown: true}}
	transcript := antartical.NewConflictTranscript(access)
	result.ConflictTranscript, err = transcript.Commitment()
	if err != nil {
		panic(fmt.Errorf("conflict transcript: %w", err))
	}
	receiptMetadata, err := transcript.ReceiptMetadataAtVersion(0, profileVersion)
	if err != nil || !receiptMetadata.Verify(transcript, 0) {
		panic(fmt.Errorf("receipt transcript: %w", err))
	}
	result.ReceiptTranscript = receiptMetadata.Commitment
	result.Checks = append(result.Checks, "deterministic-parallel-conflict-transcript")

	witness := antartical.StateWitness{Root: common.HexToHash("0x1"), Nodes: [][]byte{{3}, {1}, {2}}}
	result.CanonicalWitnessCommitment, err = witness.CommitmentAtVersion(profileVersion)
	if err != nil || !witness.VerifyCanonical(result.CanonicalWitnessCommitment) {
		panic(fmt.Errorf("canonical stateless witness: %w", err))
	}
	result.Checks = append(result.Checks, "canonical-verkle-stateless-witness")
	blockHash := crypto.Keccak256Hash([]byte("egypt-antartical-profile-block"), chainID.Bytes(), assetID[:])
	finalityDigest, err := antartical.LightClientFinalityDigest(chainID, blockHash)
	if err != nil {
		panic(fmt.Errorf("light-client finality digest: %w", err))
	}
	lightClientSignature, err := crypto.Sign(finalityDigest.Bytes(), key)
	if err != nil {
		panic(fmt.Errorf("light-client finality signature: %w", err))
	}
	proof := antartical.StatelessLightClientProof{
		ChainID: chainID, BlockNumber: 1, BlockHash: blockHash, StateRoot: witness.Root,
		WitnessCommitment: result.CanonicalWitnessCommitment, Witness: witness,
		Certificate: antartical.FinalityCertificate{Slot: 1, BlockHash: finalityDigest, CommitteeSize: 1, Signers: []common.Address{crypto.PubkeyToAddress(key.PublicKey)}, PublicKeys: [][]byte{crypto.FromECDSAPub(&key.PublicKey)}, Signatures: [][]byte{lightClientSignature}},
	}
	if err := proof.VerifyAtVersion(profileVersion, 1, 1); err != nil {
		panic(fmt.Errorf("stateless light-client proof: %w", err))
	}
	result.LightClientBlockHash = blockHash
	result.Checks = append(result.Checks, "stateless-light-client-finality-path")
	return result
}

type egyptKingState struct {
	balances map[common.Address]*big.Int
	height   uint64
}

func (s egyptKingState) GetBalance(address common.Address) *big.Int {
	if balance := s.balances[address]; balance != nil {
		return new(big.Int).Set(balance)
	}
	return new(big.Int)
}

func (s egyptKingState) GetBlockNumber() uint64 { return s.height }

// runRotatingKingChecks exercises the registration path with Egypt's chain
// identity. The manager uses block heights only, so this rehearsal is
// deterministic and does not depend on wall-clock time or a live node.
func runRotatingKingChecks() (checks []string, registrationHash common.Hash, activationHeight uint64) {
	mainKing := common.HexToAddress("0x1001")
	initialKing := common.HexToAddress("0x1002")
	candidate := common.HexToAddress("0x1003")
	manager := rotatingking.NewRotatingKingManagerForChain(params.EgyptChainConfig.ChainID, mainKing, []common.Address{initialKing}, 10)
	if _, err := manager.RegisterKingAt(candidate, 8, new(big.Int).Sub(rotatingking.EligibilityThreshold, big.NewInt(1))); !errors.Is(err, rotatingking.ErrInsufficientStake) {
		panic(fmt.Errorf("Egypt underfunded rotating-king registration returned %v", err))
	}
	registration, err := manager.RegisterKingAt(candidate, 8, rotatingking.EligibilityThreshold)
	if err != nil {
		panic(fmt.Errorf("Egypt rotating-king registration: %w", err))
	}
	if registration.RegistrationHash == (common.Hash{}) || registration.ActivationHeight != 10 {
		panic(fmt.Sprintf("Egypt rotating-king registration record = %+v", registration))
	}
	if _, err := manager.RegisterKingAt(candidate, 9, rotatingking.EligibilityThreshold); !errors.Is(err, rotatingking.ErrDuplicateRegistration) {
		panic(fmt.Errorf("Egypt duplicate rotating-king registration returned %v", err))
	}
	if got := manager.GetKingAtHeight(9); got != initialKing {
		panic(fmt.Sprintf("Egypt king before activation = %s, want %s", got.Hex(), initialKing.Hex()))
	}
	if got := manager.GetKingAtHeight(10); got != candidate {
		panic(fmt.Sprintf("Egypt king at activation = %s, want %s", got.Hex(), candidate.Hex()))
	}
	state := egyptKingState{height: 10, balances: map[common.Address]*big.Int{
		initialKing: rotatingking.EligibilityThreshold,
		candidate:   rotatingking.EligibilityThreshold,
	}}
	if err := manager.RotateToNextKing(10, common.HexToHash("0x1004"), state); err != nil {
		panic(fmt.Errorf("Egypt eligible rotation: %w", err))
	}
	if got := manager.GetCurrentKing(); got != candidate {
		panic(fmt.Sprintf("Egypt current rotating king = %s, want %s", got.Hex(), candidate.Hex()))
	}
	return []string{
		"stake-required-registration",
		"deterministic-block-activation",
		"duplicate-registration-rejection",
		"eligible-rotation-selection",
	}, registration.RegistrationHash, registration.ActivationHeight
}

type featureCheck struct {
	ID             string `json:"id"`
	ActiveBefore   bool   `json:"activeBefore"`
	ActiveAtFork   bool   `json:"activeAtFork"`
	ConsensusReady bool   `json:"consensusReady"`
}

type fixtureEngine struct {
	name string
	out  antartical.ExecutionOutput
}

func (e fixtureEngine) Name() string { return e.name }

func (e fixtureEngine) Execute(antartical.ExecutionInput) (antartical.ExecutionOutput, error) {
	return e.out, nil
}

func runPrivacyChecks(forkConfig *params.ChainConfig) ([]string, uint64, uint64) {
	to := params.ShieldedPoolAddress
	legacy := types.NewTx(&types.LegacyTx{Nonce: 1, To: &to, Value: big.NewInt(1), Gas: 100_000, GasPrice: big.NewInt(1)})
	pqTransparent := types.NewTx(&types.PQTkmTx{ChainID: forkConfig.ChainID, Nonce: 1, To: &to, Value: new(big.Int), Gas: 100_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Data: []byte("transparent")})
	if err := core.ValidatePrivateExecutionPolicy(forkConfig, big.NewInt(1), params.MainnetAntarticalTime-1, legacy); err != nil {
		panic(fmt.Errorf("pre-fork transparent compatibility: %w", err))
	}
	for name, tx := range map[string]*types.Transaction{"legacy": legacy, "pq-transparent": pqTransparent} {
		if err := core.ValidatePrivateExecutionPolicy(forkConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); !errors.Is(err, core.ErrPublicExecutionDisabled) {
			panic(fmt.Sprintf("post-fork %s transaction was not rejected: %v", name, err))
		}
	}
	v3Data, err := core.EncodeShieldedV3Transaction(&core.ShieldedV3Transaction{Version: 3, Deposit: true, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
	if err != nil {
		panic(fmt.Errorf("encode Shield3 gas fixture: %w", err))
	}
	v4Data, err := core.EncodeShieldedV4Transaction(&core.ShieldedV4Transaction{Version: 4, Deposit: true, WithdrawalValue: new(big.Int), GasSponsorValue: new(big.Int)})
	if err != nil {
		panic(fmt.Errorf("encode Shield4 gas fixture: %w", err))
	}
	for name, data := range map[string][]byte{"Shield3": v3Data, "Shield4": v4Data} {
		tx := types.NewTx(&types.PQTkmTx{ChainID: forkConfig.ChainID, Nonce: 1, To: &to, Value: big.NewInt(1), Gas: 8_000_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Data: data})
		if err := core.ValidatePrivateExecutionPolicy(forkConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err != nil {
			panic(fmt.Errorf("%s private transaction rejected: %w", name, err))
		}
	}
	for name, data := range map[string][]byte{"stamp": []byte(core.AntarticalStampMagic), "private-tvm": []byte(core.PrivateTVMMagic)} {
		tx := types.NewTx(&types.PQTkmTx{ChainID: forkConfig.ChainID, Nonce: 1, To: &to, Value: new(big.Int), Gas: 8_000_000, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(1), Data: data})
		if err := core.ValidatePrivateExecutionPolicy(forkConfig, big.NewInt(1), params.MainnetAntarticalTime, tx); err == nil {
			// A bare protocol prefix must never be admitted. Stamp envelopes
			// are checked for their full stateless shape before registration,
			// so malformed stamps may return a specific validation error rather
			// than the generic transparent-execution error.
			panic(fmt.Sprintf("unwrapped %s protocol envelope was accepted", name))
		}
	}
	v3Gas, err := core.IntrinsicGasWithShield3(v3Data, nil, nil, false, true, true, true, true, true)
	if err != nil {
		panic(fmt.Errorf("Shield3 gas calculation: %w", err))
	}
	v4Gas, err := core.IntrinsicGasWithShield4(v4Data, nil, nil, false, true, true, true, true, true)
	if err != nil {
		panic(fmt.Errorf("Shield4 gas calculation: %w", err))
	}
	if v3Gas.RegularGas < core.ShieldedV3VerifyGas || v4Gas.RegularGas < core.ShieldedV4VerifyGas {
		panic(fmt.Sprintf("shielded proof gas undercharged: Shield3=%d Shield4=%d", v3Gas.RegularGas, v4Gas.RegularGas))
	}
	return []string{"pre-fork-transparent-compatibility", "post-fork-transparent-rejection", "shield3-only-private-send", "shield4-only-private-send", "unwrapped-protocol-envelope-rejection", "shielded-proof-gas-accounting"}, v3Gas.RegularGas, v4Gas.RegularGas
}

func runProtocolChecks() ([]featureCheck, []string, []string, uint64, uint64, common.Hash, common.Hash) {
	// Egypt activates Antartical at genesis. Clone the config and move the
	// activation into the future only inside this unit-style rehearsal so both
	// pre-fork and post-fork gate transitions remain covered.
	forkConfig := *params.EgyptChainConfig
	forkConfig.AntarticalTime = new(uint64)
	*forkConfig.AntarticalTime = params.MainnetAntarticalTime
	privacyChecks, shield3Gas, shield4Gas := runPrivacyChecks(&forkConfig)
	features := forkConfig.AntarticalFeatureCatalog()
	checks := make([]featureCheck, 0, len(features))
	for _, feature := range features {
		before := forkConfig.IsAntarticalFeatureActive(feature.ID, big.NewInt(1), params.MainnetAntarticalTime-1)
		atFork := forkConfig.IsAntarticalFeatureActive(feature.ID, big.NewInt(1), params.MainnetAntarticalTime)
		if before || !atFork {
			panic(fmt.Sprintf("feature gate %s did not transition at Antartical", feature.ID))
		}
		checks = append(checks, featureCheck{ID: string(feature.ID), ActiveBefore: before, ActiveAtFork: atFork, ConsensusReady: feature.ConsensusReady})
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		panic(fmt.Errorf("generate protocol test key: %w", err))
	}
	sender := crypto.PubkeyToAddress(key.PublicKey)
	entryPoint := common.HexToAddress("0x100")
	op := &antartical.UserOperation{
		Sender: sender, Nonce: big.NewInt(1), CallGasLimit: big.NewInt(100_000),
		VerificationGasLimit: big.NewInt(100_000), PreVerificationGas: big.NewInt(1_000),
		MaxFeePerGas: big.NewInt(10), MaxPriorityFeePerGas: big.NewInt(1),
	}
	opHash, err := op.Hash(params.EgyptChainConfig.ChainID, entryPoint)
	if err != nil {
		panic(fmt.Errorf("account-abstraction hash: %w", err))
	}
	op.Signature, err = crypto.Sign(opHash.Bytes(), key)
	if err != nil || op.VerifySignature(params.EgyptChainConfig.ChainID, entryPoint) != nil {
		panic("account-abstraction signature verification failed")
	}

	accessWaves := antartical.BuildExecutionWaves([]antartical.AccessSet{
		{Reads: []common.Address{common.HexToAddress("0x1")}},
		{Reads: []common.Address{common.HexToAddress("0x2")}},
		{Unknown: true},
	})
	if len(accessWaves) != 2 || len(accessWaves[0]) != 2 || len(accessWaves[1]) != 1 {
		panic(fmt.Sprintf("parallel execution waves: %#v", accessWaves))
	}
	var used antartical.GasVector
	limit := antartical.GasVector{Execution: 10, StateRead: 5, StateWrite: 5, Blob: 2}
	if err := used.Charge(&limit, antartical.GasVector{Execution: 3, StateRead: 2}); err != nil {
		panic(fmt.Errorf("multidimensional gas: %w", err))
	}

	witness := antartical.StateWitness{Root: common.HexToHash("0x1"), Nodes: [][]byte{{1}, {2}}}
	witnessCommitment, err := witness.Commitment()
	if err != nil || !witness.Verify(witnessCommitment) {
		panic(fmt.Errorf("stateless witness commitment: %w", err))
	}
	blockHash := common.HexToHash("0xabc")
	randomness := antartical.DeriveRandomness(common.HexToHash("0x123"), blockHash, 1)
	if randomness == (common.Hash{}) {
		panic("native randomness returned zero")
	}

	observation := antartical.OracleObservation{FeedID: common.HexToHash("0x1"), Round: 1, Value: []byte("42"), Timestamp: 1, Signer: sender}
	observationHash, err := observation.Hash(params.EgyptChainConfig.ChainID)
	if err != nil {
		panic(fmt.Errorf("oracle hash: %w", err))
	}
	observation.Signature, err = crypto.Sign(observationHash.Bytes(), key)
	if err != nil || observation.Verify(params.EgyptChainConfig.ChainID) != nil {
		panic("oracle verification failed")
	}

	crossChain := antartical.CrossChainMessage{SourceChainID: params.EgyptChainConfig.ChainID, DestinationChainID: params.RandomXChainConfig.ChainID, Nonce: 1, Sender: sender, Target: common.HexToAddress("0x2"), Payload: []byte("payload")}
	if _, err := crossChain.ReplayKey(); err != nil {
		panic(fmt.Errorf("cross-chain replay key: %w", err))
	}
	if err := antartical.ValidateEOF([]byte{0xef, 0x00, 0x01, 0x01, 0x00, 0x01, 0xaa, 0x02, 0x00, 0x01, 0x60}); err != nil {
		panic(fmt.Errorf("EOF validation: %w", err))
	}
	precompiles := antartical.NewPrecompileRegistry()
	if err := precompiles.Register(antartical.PrecompileModule{Address: common.HexToAddress("0x100"), Name: "egypt-test", Gas: 1, Run: func(input []byte, _ *big.Int) ([]byte, error) { return input, nil }}); err != nil {
		panic(fmt.Errorf("precompile registration: %w", err))
	}
	if _, ok := precompiles.Lookup(common.HexToAddress("0x100")); !ok {
		panic("precompile lookup failed")
	}
	certificate := antartical.FinalityCertificate{Slot: 1, BlockHash: blockHash, Signers: []common.Address{sender}, PublicKeys: [][]byte{crypto.FromECDSAPub(&key.PublicKey)}, Signatures: nil}
	certificate.Signatures = [][]byte{nil}
	certificate.Signatures[0], err = crypto.Sign(blockHash.Bytes(), key)
	if err != nil || certificate.Verify(1, 1) != nil {
		panic("single-slot finality certificate failed")
	}

	engineOutput := antartical.ExecutionOutput{StateRoot: common.HexToHash("0x1"), ReceiptsRoot: common.HexToHash("0x2"), ProofDigest: common.HexToHash("0x3")}
	primary := fixtureEngine{name: "go-evm", out: engineOutput}
	secondary := fixtureEngine{name: "revm", out: engineOutput}
	registry, err := antartical.NewEngineRegistry(primary)
	if err != nil || registry.RegisterConformant(secondary, []antartical.ExecutionInput{{}}) != nil {
		panic("alternative EVM differential execution failed")
	}
	claim := antartical.ExecutionClaim{ParentStateRoot: common.HexToHash("0x10"), StateRoot: engineOutput.StateRoot, Transactions: common.HexToHash("0x11"), Receipts: engineOutput.ReceiptsRoot, ProofDigest: engineOutput.ProofDigest}
	claimCommitment, err := claim.Commitment()
	if err != nil {
		panic(fmt.Errorf("zkEVM execution claim: %w", err))
	}

	checksPassed := []string{
		"account-abstraction", "parallel-execution", "multidimensional-gas", "stateless-witness", "native-randomness",
		"oracles", "cross-chain-replay-protection", "eof", "modular-precompiles", "single-slot-finality",
		"alternative-evm-differential", "zkevm-execution-claim",
	}
	return checks, checksPassed, privacyChecks, shield3Gas, shield4Gas, claimCommitment, witnessCommitment
}

func main() {
	featureChecks, protocolChecks, privacyChecks, shield3Gas, shield4Gas, claimCommitment, witnessCommitment := runProtocolChecks()
	tokenOwner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	counterOwner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	cfg := &runtime.Config{ChainConfig: params.EgyptChainConfig, Origin: tokenOwner, BlockNumber: big.NewInt(1), Time: 1, GasLimit: 12_000_000, GasPrice: big.NewInt(1), Value: new(big.Int)}
	// EUSD is a six-decimal Egypt testnet stablecoin fixture. The manifest is
	// appended to the runtime code exactly as a deployment tool would append
	// the result of tkmasset_buildManifest.
	eusdManifest := tkmasset.Manifest{
		Kind:        tkmasset.KindFungible,
		ChainID:     new(big.Int).Set(params.EgyptChainConfig.ChainID),
		Decimals:    6,
		Flags:       tkmasset.FlagMintable,
		PolicyHash:  crypto.Keccak256Hash([]byte("EUSD Egypt policy v1: issuer-minted, six-decimal test asset")),
		Name:        "Egypt United Dollar",
		Symbol:      "EUSD",
		MetadataURI: "ipfs://tkm/eusd/manifest-v1.json",
	}
	manifestHash, err := eusdManifest.ManifestHash()
	if err != nil {
		panic(fmt.Errorf("hash EUSD manifest: %w", err))
	}
	tokenCode, err := tkmasset.AppendTrailer(tokenRuntime(), eusdManifest)
	if err != nil {
		panic(fmt.Errorf("append EUSD manifest trailer: %w", err))
	}
	deployedToken, tokenAddress, tokenGas, err := runtime.Create(constructor(tokenCode), cfg)
	if err != nil {
		panic(fmt.Errorf("deploy token: %w", err))
	}
	parsedManifest, found, err := tkmasset.ParseRuntimeCode(deployedToken)
	if err != nil || !found || parsedManifest.Symbol != eusdManifest.Symbol || parsedManifest.ChainID.Cmp(eusdManifest.ChainID) != 0 {
		panic(fmt.Errorf("verify EUSD runtime manifest: found=%t err=%v", found, err))
	}
	assetID, err := tkmasset.AssetID(eusdManifest.ChainID, tokenAddress, eusdManifest.Kind, manifestHash)
	if err != nil {
		panic(fmt.Errorf("compute EUSD asset ID: %w", err))
	}
	precompileInput, err := tkmasset.PrecompileInput(eusdManifest.ChainID, tokenAddress, eusdManifest.Kind, manifestHash)
	if err != nil {
		panic(fmt.Errorf("build EUSD precompile input: %w", err))
	}
	precompile, ok := vm.ActivePrecompiledContracts(params.Rules{IsCancun: true})[vm.TKMAssetIDPrecompileAddr]
	if !ok {
		panic("TKM asset identity precompile is not active in the Antartical EUSD rehearsal")
	}
	precompileAssetID, err := precompile.Run(precompileInput)
	if err != nil || common.BytesToHash(precompileAssetID) != assetID {
		panic(fmt.Errorf("EUSD precompile identity mismatch: got=%s want=%s err=%v", common.BytesToHash(precompileAssetID), assetID, err))
	}
	profileVersion := params.EgyptChainConfig.Rules(big.NewInt(1), false, 1).TKMProfileVersion
	if profileVersion != params.TKMProfileAntarticalVersion {
		panic(fmt.Sprintf("Egypt TKM profile version is %d, want %d", profileVersion, params.TKMProfileAntarticalVersion))
	}
	profile := runProfileChecks(params.EgyptChainConfig.ChainID, assetID, manifestHash, crypto.Keccak256Hash(deployedToken), tokenOwner, profileVersion)
	rotatingKingChecks, rotatingKingRegistration, rotatingKingActivation := runRotatingKingChecks()
	amount := new(big.Int).Mul(big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(eusdManifest.Decimals)), nil))
	if _, _, err := runtime.Call(tokenAddress, call("mint(address,uint256)", wordAddress(tokenOwner), wordUint(amount)), cfg); err != nil {
		panic(fmt.Errorf("mint token: %w", err))
	}
	balance, _, err := runtime.Call(tokenAddress, call("balanceOf(address)", wordAddress(tokenOwner)), cfg)
	if err != nil {
		panic(fmt.Errorf("read token balance: %w", err))
	}
	if new(big.Int).SetBytes(balance).Cmp(amount) != 0 {
		panic(fmt.Sprintf("mint balance mismatch: got=%s want=%s", new(big.Int).SetBytes(balance), amount))
	}
	transferRecipient := common.HexToAddress("0x0000000000000000000000000000000000000003")
	transferAmount := new(big.Int).Mul(big.NewInt(250), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(eusdManifest.Decimals)), nil))
	transferOutput, transferGasLeft, err := runtime.Call(tokenAddress, call("transfer(address,uint256)", wordAddress(transferRecipient), wordUint(transferAmount)), cfg)
	if err != nil {
		panic(fmt.Errorf("transfer token: %w", err))
	}
	if got := new(big.Int).SetBytes(transferOutput); got.Cmp(big.NewInt(1)) != 0 {
		panic(fmt.Sprintf("transfer did not return success: %s", got))
	}
	balanceAfterTransfer, _, err := runtime.Call(tokenAddress, call("balanceOf(address)", wordAddress(tokenOwner)), cfg)
	if err != nil {
		panic(fmt.Errorf("read sender balance after transfer: %w", err))
	}
	recipientBalance, _, err := runtime.Call(tokenAddress, call("balanceOf(address)", wordAddress(transferRecipient)), cfg)
	if err != nil {
		panic(fmt.Errorf("read recipient balance after transfer: %w", err))
	}
	totalAfterTransfer, _, err := runtime.Call(tokenAddress, call("totalSupply()"), cfg)
	if err != nil {
		panic(fmt.Errorf("read total supply after transfer: %w", err))
	}
	expectedSender := new(big.Int).Mul(big.NewInt(750), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(eusdManifest.Decimals)), nil))
	expectedRecipient := transferAmount
	if new(big.Int).SetBytes(balanceAfterTransfer).Cmp(expectedSender) != 0 || new(big.Int).SetBytes(recipientBalance).Cmp(expectedRecipient) != 0 || new(big.Int).SetBytes(totalAfterTransfer).Cmp(amount) != 0 {
		panic(fmt.Sprintf("transfer state mismatch: sender=%s recipient=%s total=%s", new(big.Int).SetBytes(balanceAfterTransfer), new(big.Int).SetBytes(recipientBalance), new(big.Int).SetBytes(totalAfterTransfer)))
	}
	total, _, err := runtime.Call(tokenAddress, call("totalSupply()"), cfg)
	if err != nil {
		panic(fmt.Errorf("read total supply: %w", err))
	}
	cfg.Origin = counterOwner
	counterCode := counterRuntime()
	deployedCounter, counterAddress, counterGas, err := runtime.Create(constructor(counterCode), cfg)
	if err != nil {
		panic(fmt.Errorf("deploy counter: %w", err))
	}
	if _, _, err := runtime.Call(counterAddress, call("set(uint256)", wordUint(big.NewInt(42))), cfg); err != nil {
		panic(fmt.Errorf("set counter: %w", err))
	}
	counterValue, _, err := runtime.Call(counterAddress, call("get()"), cfg)
	if err != nil {
		panic(fmt.Errorf("read counter: %w", err))
	}
	out := result{
		Network: "egypt", ChainID: params.EgyptChainConfig.ChainID.String(),
		TokenName: eusdManifest.Name, TokenSymbol: eusdManifest.Symbol, TokenDecimals: eusdManifest.Decimals, TokenStandard: eusdManifest.Kind.Standard(),
		TokenAddress: tokenAddress.Hex(), TokenRuntimeHash: crypto.Keccak256Hash(deployedToken).Hex(), TokenManifestHash: manifestHash.Hex(), TokenAssetID: assetID.Hex(), TokenPrecompileAssetID: common.BytesToHash(precompileAssetID).Hex(), TokenManifestVerified: true,
		TokenBalance: new(big.Int).SetBytes(balance).String(), TokenBalanceAfterTransfer: new(big.Int).SetBytes(balanceAfterTransfer).String(), TransferRecipient: transferRecipient.Hex(), TransferAmount: transferAmount.String(), RecipientBalance: new(big.Int).SetBytes(recipientBalance).String(), TransferGasUsed: cfg.GasLimit - transferGasLeft, TokenTotalSupply: new(big.Int).SetBytes(total).String(),
		CounterAddress: counterAddress.Hex(), CounterRuntimeHash: crypto.Keccak256Hash(deployedCounter).Hex(), CounterValue: new(big.Int).SetBytes(counterValue).String(), TokenDeployGas: tokenGas, CounterDeployGas: counterGas,
		FeatureChecks: featureChecks, ProtocolChecks: protocolChecks, ProfileChecks: profile.Checks, AssetRegistryRoot: profile.AssetRegistryRoot.Hex(), ShieldedAssetBinding: profile.ShieldedAssetBinding.Hex(), CanonicalWitnessCommitment: profile.CanonicalWitnessCommitment.Hex(), LightClientBlockHash: profile.LightClientBlockHash.Hex(), ConflictTranscript: profile.ConflictTranscript.Hex(), ReceiptTranscript: profile.ReceiptTranscript.Hex(), PrivacyChecks: privacyChecks, Shield3Gas: shield3Gas, Shield4Gas: shield4Gas,
		ZKEVMClaim: claimCommitment.Hex(), StateWitnessCommit: witnessCommitment.Hex(),
		RotatingKingChecks: rotatingKingChecks, RotatingKingRegistration: rotatingKingRegistration.Hex(), RotatingKingActivation: rotatingKingActivation,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
