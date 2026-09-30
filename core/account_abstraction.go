package core

// Native account-abstraction execution.  Antartical user operations are
// submitted through a reserved, consensus-owned entry point and are executed
// by the canonical Go EVM. The relaying transaction pays the outer gas, while
// factory, paymaster, verification, and call gas are charged from the same
// block gas pool. No contract at the reserved address can replace these rules.

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/antartical"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

const AccountAbstractionMagic = "TKM-AA-ENTRYPOINT-V1"

var (
	ErrAccountAbstractionEnvelope  = errors.New("invalid TKM account-abstraction envelope")
	ErrAccountAbstractionNonce     = errors.New("invalid TKM account-abstraction nonce")
	ErrAccountAbstractionTarget    = errors.New("invalid TKM account-abstraction target")
	ErrAccountAbstractionPaymaster = errors.New("unsupported or invalid TKM account-abstraction paymaster")
)

func HasAccountAbstractionPrefix(data []byte) bool {
	return bytes.HasPrefix(data, []byte(AccountAbstractionMagic))
}

// EncodeAccountAbstraction wraps the canonical UserOperation encoding in a
// distinct envelope so legacy transaction decoders cannot reinterpret it.
func EncodeAccountAbstraction(op *antartical.UserOperation) ([]byte, error) {
	if op == nil {
		return nil, ErrAccountAbstractionEnvelope
	}
	encoded, err := op.Encode()
	if err != nil {
		return nil, err
	}
	return append([]byte(AccountAbstractionMagic), encoded...), nil
}

func DecodeAccountAbstraction(data []byte) (*antartical.UserOperation, error) {
	if !HasAccountAbstractionPrefix(data) || uint64(len(data)) > ShieldedV3MaxTxSize {
		return nil, ErrAccountAbstractionEnvelope
	}
	op, err := antartical.DecodeUserOperation(data[len(AccountAbstractionMagic):])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAccountAbstractionEnvelope, err)
	}
	return op, nil
}

func accountAbstractionNonceSlot(sender common.Address) common.Hash {
	return ShieldedV3StateSlot("account-abstraction/nonce", sender.Bytes())
}

func accountAbstractionNonce(st *state.StateDB, sender common.Address) uint64 {
	return uint64FromHash(st.GetState(params.ShieldedPoolAddress, accountAbstractionNonceSlot(sender)))
}

func setAccountAbstractionNonce(st *state.StateDB, sender common.Address, nonce uint64) {
	st.SetState(params.ShieldedPoolAddress, accountAbstractionNonceSlot(sender), uint64Hash(nonce))
}

// accountAbstractionCallData uses a fixed 20-byte target prefix followed by
// the target calldata.  This keeps the consensus envelope independent of an
// ABI implementation while still providing the target required by EIP-4337.
func accountAbstractionCallData(data []byte) (common.Address, []byte, error) {
	if len(data) < common.AddressLength {
		return common.Address{}, nil, ErrAccountAbstractionTarget
	}
	target := common.BytesToAddress(data[:common.AddressLength])
	if target == (common.Address{}) || target == params.TKMEntryPointAddress {
		return common.Address{}, nil, ErrAccountAbstractionTarget
	}
	return target, data[common.AddressLength:], nil
}

// ProcessAccountAbstraction validates and executes one operation. The
// operation is deliberately atomic: an invalid call reverts the operation and
// the enclosing block, exactly like a failed consensus transaction. InitCode
// and paymaster payloads are executed only through code-bearing EVM contracts;
// an absent factory or paymaster is rejected rather than treated as a no-op.
func ProcessAccountAbstraction(config *params.ChainConfig, number *big.Int, timestamp uint64, st *state.StateDB, evm *vm.EVM, gp *GasPool, tx *types.Transaction, sender common.Address) (uint64, error) {
	if config == nil || !config.IsAntartical(number, timestamp) {
		return 0, antartical.ErrProfileInactive
	}
	if tx == nil || tx.Type() != types.PQTkmTxType || tx.To() == nil || *tx.To() != params.TKMEntryPointAddress || tx.Value().Sign() != 0 || tx.ChainId().Cmp(config.ChainID) != 0 {
		return 0, ErrAccountAbstractionEnvelope
	}
	op, err := DecodeAccountAbstraction(tx.Data())
	if err != nil {
		return 0, err
	}
	if err := op.VerifySignature(config.ChainID, params.TKMEntryPointAddress); err != nil {
		return 0, err
	}
	// The outer transaction sender is the bundler/relayer. UserOperation
	// authorization is carried by the operation signature and may belong to a
	// different account, as required by EIP-4337.
	_ = sender
	if op.Nonce.BitLen() > 64 {
		return 0, ErrAccountAbstractionNonce
	}
	want := accountAbstractionNonce(st, op.Sender)
	if op.Nonce.Uint64() != want {
		return 0, fmt.Errorf("%w: have %d want %d", ErrAccountAbstractionNonce, op.Nonce.Uint64(), want)
	}
	target, input, err := accountAbstractionCallData(op.CallData)
	if err != nil {
		return 0, err
	}
	if op.CallGasLimit.BitLen() > 64 || op.VerificationGasLimit.BitLen() > 64 || op.PreVerificationGas.BitLen() > 64 {
		return 0, ErrAccountAbstractionEnvelope
	}
	callGas := op.CallGasLimit.Uint64()
	verificationGas := op.VerificationGasLimit.Uint64()
	preVerificationGas := op.PreVerificationGas.Uint64()
	if callGas == 0 || verificationGas == 0 {
		return 0, ErrAccountAbstractionEnvelope
	}
	if preVerificationGas > ^uint64(0)-verificationGas || preVerificationGas+verificationGas > callGas {
		return 0, ErrAccountAbstractionEnvelope
	}
	reserved := callGas
	if reserved > ^uint64(0)-verificationGas {
		return 0, ErrAccountAbstractionEnvelope
	}
	reserved += verificationGas
	if err := gp.SubGas(reserved); err != nil {
		return 0, err
	}
	if verificationGas > callGas {
		return 0, ErrAccountAbstractionEnvelope
	}
	// Factory deployment and paymaster validation are ordinary EVM calls made
	// by the reserved entry point. Their code and return status are consensus
	// checked; an absent paymaster is rejected instead of being treated as a
	// successful no-op.
	verificationBudget := vm.NewGasBudget(verificationGas)
	if len(op.InitCode) != 0 {
		factory, factoryInput, err := accountAbstractionCallData(op.InitCode)
		if err != nil {
			return 0, err
		}
		if st.GetCodeSize(factory) == 0 {
			return 0, fmt.Errorf("%w: factory has no code", ErrAccountAbstractionEnvelope)
		}
		_, next, callErr := evm.Call(params.TKMEntryPointAddress, factory, factoryInput, verificationBudget, new(uint256.Int))
		verificationBudget = next
		if callErr != nil {
			return 0, fmt.Errorf("account-abstraction factory validation: %w", callErr)
		}
	}
	if len(op.PaymasterAndData) != 0 {
		if len(op.PaymasterAndData) < common.AddressLength {
			return 0, ErrAccountAbstractionPaymaster
		}
		paymaster := common.BytesToAddress(op.PaymasterAndData[:common.AddressLength])
		if paymaster == (common.Address{}) || st.GetCodeSize(paymaster) == 0 {
			return 0, ErrAccountAbstractionPaymaster
		}
		_, next, callErr := evm.Call(params.TKMEntryPointAddress, paymaster, op.PaymasterAndData[common.AddressLength:], verificationBudget, new(uint256.Int))
		verificationBudget = next
		if callErr != nil {
			return 0, fmt.Errorf("account-abstraction paymaster validation: %w", callErr)
		}
	}
	verificationUsed := verificationGas - verificationBudget.RegularGas
	budget := vm.NewGasBudget(callGas)
	_, left, callErr := evm.Call(op.Sender, target, input, budget, new(uint256.Int))
	usedCall := callGas - left.RegularGas
	used := verificationUsed + usedCall
	if used < preVerificationGas {
		used = preVerificationGas
	}
	// The reservation was removed before execution. Return the unused portion,
	// while retaining the actual operation cost in the block gas accounting.
	if err := gp.ReturnGas(reserved-used, used); err != nil {
		return 0, err
	}
	if callErr != nil {
		return used, callErr
	}
	setAccountAbstractionNonce(st, op.Sender, want+1)
	return used, nil
}
