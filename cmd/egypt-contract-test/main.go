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
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/crypto"
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
	Network                   string         `json:"network"`
	ChainID                   string         `json:"chainId"`
	TokenAddress              string         `json:"tokenAddress"`
	TokenRuntimeHash          string         `json:"tokenRuntimeHash"`
	TokenBalance              string         `json:"tokenBalance"`
	TokenBalanceAfterTransfer string         `json:"tokenBalanceAfterTransfer"`
	TransferRecipient         string         `json:"transferRecipient"`
	TransferAmount            string         `json:"transferAmount"`
	RecipientBalance          string         `json:"recipientBalance"`
	TransferGasUsed           uint64         `json:"transferGasUsed"`
	TokenTotalSupply          string         `json:"tokenTotalSupply"`
	CounterAddress            string         `json:"counterAddress"`
	CounterRuntimeHash        string         `json:"counterRuntimeHash"`
	CounterValue              string         `json:"counterValue"`
	TokenDeployGas            uint64         `json:"tokenDeployGas"`
	CounterDeployGas          uint64         `json:"counterDeployGas"`
	FeatureChecks             []featureCheck `json:"featureChecks"`
	ProtocolChecks            []string       `json:"protocolChecks"`
	PrivacyChecks             []string       `json:"privacyChecks"`
	Shield3Gas                uint64         `json:"shield3Gas"`
	Shield4Gas                uint64         `json:"shield4Gas"`
	ZKEVMClaim                string         `json:"zkevmClaim"`
	StateWitnessCommit        string         `json:"stateWitnessCommitment"`
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
	if err != nil || registry.Register(secondary) != nil || antartical.CompareEngines(primary, secondary, antartical.ExecutionInput{}) != nil {
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
	tokenCode := tokenRuntime()
	deployedToken, tokenAddress, tokenGas, err := runtime.Create(constructor(tokenCode), cfg)
	if err != nil {
		panic(fmt.Errorf("deploy token: %w", err))
	}
	amount := new(big.Int).Mul(big.NewInt(1000), big.NewInt(params.Ether))
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
	transferAmount := new(big.Int).Mul(big.NewInt(250), big.NewInt(params.Ether))
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
	expectedSender := new(big.Int).Mul(big.NewInt(750), big.NewInt(params.Ether))
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
		TokenAddress: tokenAddress.Hex(), TokenRuntimeHash: crypto.Keccak256Hash(deployedToken).Hex(), TokenBalance: new(big.Int).SetBytes(balance).String(), TokenBalanceAfterTransfer: new(big.Int).SetBytes(balanceAfterTransfer).String(), TransferRecipient: transferRecipient.Hex(), TransferAmount: transferAmount.String(), RecipientBalance: new(big.Int).SetBytes(recipientBalance).String(), TransferGasUsed: cfg.GasLimit - transferGasLeft, TokenTotalSupply: new(big.Int).SetBytes(total).String(),
		CounterAddress: counterAddress.Hex(), CounterRuntimeHash: crypto.Keccak256Hash(deployedCounter).Hex(), CounterValue: new(big.Int).SetBytes(counterValue).String(), TokenDeployGas: tokenGas, CounterDeployGas: counterGas,
		FeatureChecks: featureChecks, ProtocolChecks: protocolChecks, PrivacyChecks: privacyChecks, Shield3Gas: shield3Gas, Shield4Gas: shield4Gas,
		ZKEVMClaim: claimCommitment.Hex(), StateWitnessCommit: witnessCommitment.Hex(),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
