// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math/big"
        "github.com/ethereum/go-ethereum/common"
)

const (
	GasLimitBoundDivisor uint64 = 1024               // The bound divisor of the gas limit, used in update calculations.
	MinGasLimit          uint64 = 5000               // Minimum the gas limit may ever be.
	MaxGasLimit          uint64 = 0x7fffffffffffffff // Maximum the gas limit (2^63-1).
	GenesisGasLimit      uint64 = 134217728

	MaxTxGas uint64 = 1 << 24 // Maximum transaction gas limit after eip-7825 (16,777,216).

	MaximumExtraDataSize  uint64 = 32    // Maximum size extra data may be after Genesis.
	ExpByteGas            uint64 = 10    // Times ceil(log256(exponent)) for the EXP instruction.
	SloadGas              uint64 = 50    //
	CallValueTransferGas  uint64 = 9000  // Paid for CALL when the value transfer is non-zero.
	CallNewAccountGas     uint64 = 25000 // Paid for CALL when the destination address didn't exist prior.
	TxGas                 uint64 = 21000 // Per transaction not creating a contract.
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract.
	TxDataZeroGas         uint64 = 4     // Per byte of data attached to a transaction that equals zero.
	QuadCoeffDiv          uint64 = 512   // Divisor for the quadratic particle of the memory cost equation.
	LogDataGas            uint64 = 8     // Per byte in a LOG* operation's data.
	CallStipend           uint64 = 2300  // Free gas given at beginning of call.

	Keccak256Gas     uint64 = 30 // Once per KECCAK256 operation.
	Keccak256WordGas uint64 = 6  // Once per word of the KECCAK256 operation's data.
	InitCodeWordGas  uint64 = 2  // Once per word of the init code when creating a contract.

	SstoreSetGas    uint64 = 20000 // Once per SSTORE operation.
	SstoreResetGas  uint64 = 5000  // Once per SSTORE operation if the zeroness changes from zero.
	SstoreClearGas  uint64 = 5000  // Once per SSTORE operation if the zeroness doesn't change.
	SstoreRefundGas uint64 = 15000 // Once per SSTORE operation if the zeroness changes to zero.

	NetSstoreNoopGas  uint64 = 200   // Once per SSTORE operation if the value doesn't change.
	NetSstoreInitGas  uint64 = 20000 // Once per SSTORE operation from clean zero.
	NetSstoreCleanGas uint64 = 5000  // Once per SSTORE operation from clean non-zero.
	NetSstoreDirtyGas uint64 = 200   // Once per SSTORE operation from dirty.

	NetSstoreClearRefund      uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot
	NetSstoreResetRefund      uint64 = 4800  // Once per SSTORE operation for resetting to the original non-zero value
	NetSstoreResetClearRefund uint64 = 19800 // Once per SSTORE operation for resetting to the original zero value

	SstoreSentryGasEIP2200            uint64 = 2300  // Minimum gas required to be present for an SSTORE call, not consumed
	SstoreSetGasEIP2200               uint64 = 20000 // Once per SSTORE operation from clean zero to non-zero
	SstoreResetGasEIP2200             uint64 = 5000  // Once per SSTORE operation from clean non-zero to something else
	SstoreClearsScheduleRefundEIP2200 uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot

	ColdAccountAccessCostEIP2929 = uint64(2600) // COLD_ACCOUNT_ACCESS_COST
	ColdSloadCostEIP2929         = uint64(2100) // COLD_SLOAD_COST
	WarmStorageReadCostEIP2929   = uint64(100)  // WARM_STORAGE_READ_COST

	SstoreClearsScheduleRefundEIP3529 uint64 = SstoreResetGasEIP2200 - ColdSloadCostEIP2929 + TxAccessListStorageKeyGas

	JumpdestGas   uint64 = 1     // Once per JUMPDEST operation.
	EpochDuration uint64 = 30000 // Duration between proof-of-work epochs (RandomX epochs handled separately)

	CreateDataGas         uint64 = 200   //
	CallCreateDepth       uint64 = 1024  // Maximum depth of call/create stack.
	ExpGas                uint64 = 10    // Once per EXP instruction
	LogGas                uint64 = 375   // Per LOG* operation.
	CopyGas               uint64 = 3     // Multiplied by the number of 32-byte words that are copied (round up) for any *COPY operation and added.
	StackLimit            uint64 = 1024  // Maximum size of VM stack allowed.
	TierStepGas           uint64 = 0     // Once per operation, for a selection of them.
	LogTopicGas           uint64 = 375   // Multiplied by the * of the LOG*, per LOG transaction.
	CreateGas             uint64 = 32000 // Once per CREATE operation & contract-creation transaction.
	Create2Gas            uint64 = 32000 // Once per CREATE2 operation
	CreateNGasEip4762     uint64 = 1000  // Once per CREATEn operations post-verkle
	SelfdestructRefundGas uint64 = 24000 // Refunded following a selfdestruct operation.
	MemoryGas             uint64 = 3     // Times the address of the (highest referenced byte in memory + 1).

	TxDataNonZeroGasFrontier  uint64 = 68    // Per byte of data attached to a transaction that is not equal to zero.
	TxDataNonZeroGasEIP2028   uint64 = 16    // Per byte of non zero data attached to a transaction after EIP 2028
	TxTokenPerNonZeroByte     uint64 = 4     // Token cost per non-zero byte as specified by EIP-7623.
	TxCostFloorPerToken       uint64 = 10    // Cost floor per byte of data as specified by EIP-7623.
	TxCostFloorPerToken7976   uint64 = 16    // Cost floor per byte of data as specified by EIP-7976.
	TxAccessListAddressGas    uint64 = 2400  // Per address specified in EIP 2930 access list
	TxAccessListStorageKeyGas uint64 = 1900  // Per storage key specified in EIP 2930 access list
	TxAuthTupleGas            uint64 = 12500 // Per auth tuple code specified in EIP-7702

	// Call gas costs
	CallGasFrontier              uint64 = 40  // Once per CALL operation & message call transaction.
	CallGasEIP150                uint64 = 700 // Static portion of gas for CALL-derivates after EIP 150
	BalanceGasFrontier           uint64 = 20  // The cost of a BALANCE operation
	BalanceGasEIP150             uint64 = 400 // The cost of a BALANCE operation after Tangerine
	BalanceGasEIP1884            uint64 = 700 // The cost of a BALANCE operation after EIP 1884
	ExtcodeSizeGasFrontier       uint64 = 20  // Cost of EXTCODESIZE before EIP 150
	ExtcodeSizeGasEIP150         uint64 = 700 // Cost of EXTCODESIZE after EIP 150
	SloadGasFrontier             uint64 = 50
	SloadGasEIP150               uint64 = 200
	SloadGasEIP1884              uint64 = 800  // Cost of SLOAD after EIP 1884
	SloadGasEIP2200              uint64 = 800  // Cost of SLOAD after EIP 2200
	ExtcodeHashGasConstantinople uint64 = 400  // Cost of EXTCODEHASH
	ExtcodeHashGasEIP1884        uint64 = 700  // Cost of EXTCODEHASH after EIP 1884
	SelfdestructGasEIP150        uint64 = 5000 // Cost of SELFDESTRUCT post EIP 150

	ExpByteFrontier uint64 = 10 // was set to 10 in Frontier
	ExpByteEIP158   uint64 = 50 // was raised to 50 during Eip158

	ExtcodeCopyBaseFrontier uint64 = 20
	ExtcodeCopyBaseEIP150   uint64 = 700

	CreateBySelfdestructGas uint64 = 25000

	InitialBaseFee = 7

	MaxCodeSize              = 24576                    // Maximum bytecode to permit for a contract
	MaxInitCodeSize          = 2 * MaxCodeSize          // Maximum initcode to permit in a creation transaction
	MaxCodeSizeAmsterdam     = 32768                    // Maximum bytecode to permit for a contract post Amsterdam
	MaxInitCodeSizeAmsterdam = 2 * MaxCodeSizeAmsterdam // Maximum initcode to permit post Amsterdam

	// Precompiled contract gas prices
	EcrecoverGas        uint64 = 3000
	Sha256BaseGas       uint64 = 60
	Sha256PerWordGas    uint64 = 12
	Ripemd160BaseGas    uint64 = 600
	Ripemd160PerWordGas uint64 = 120
	IdentityBaseGas     uint64 = 15
	IdentityPerWordGas  uint64 = 3

	Bn256AddGasByzantium             uint64 = 500
	Bn256AddGasIstanbul              uint64 = 150
	Bn256ScalarMulGasByzantium       uint64 = 40000
	Bn256ScalarMulGasIstanbul        uint64 = 6000
	Bn256PairingBaseGasByzantium     uint64 = 100000
	Bn256PairingBaseGasIstanbul      uint64 = 45000
	Bn256PairingPerPointGasByzantium uint64 = 80000
	Bn256PairingPerPointGasIstanbul  uint64 = 34000

	Bls12381G1AddGas          uint64 = 375
	Bls12381G1MulGas          uint64 = 12000
	Bls12381G2AddGas          uint64 = 600
	Bls12381G2MulGas          uint64 = 22500
	Bls12381PairingBaseGas    uint64 = 37700
	Bls12381PairingPerPairGas uint64 = 32600
	Bls12381MapG1Gas          uint64 = 5500
	Bls12381MapG2Gas          uint64 = 23800

	P256VerifyGas uint64 = 6900

	RefundQuotient        uint64 = 2
	RefundQuotientEIP3529 uint64 = 5

	BlobTxBytesPerFieldElement         = 32
	BlobTxFieldElementsPerBlob         = 4096
	BlobTxBlobGasPerBlob               = 1 << 17
	BlobTxMinBlobGasprice              = 1
	BlobTxPointEvaluationPrecompileGas = 50000
	BlobTxMaxBlobs                     = 6
	BlobBaseCost                       = 1 << 13

	HistoryServeWindow = 8191

	MaxBlockSize = 8_388_608 // maximum size of an RLP-encoded block
)

// Bls12381G1MultiExpDiscountTable is the gas discount table for BLS12-381 G1 multi exponentiation operation
var Bls12381G1MultiExpDiscountTable = [128]uint64{1000, 949, 848, 797, 764, 750, 738, 728, 719, 712, 705, 698, 692, 687, 682, 677, 673, 669, 665, 661, 658, 654, 651, 648, 645, 642, 640, 637, 635, 632, 630, 627, 625, 623, 621, 619, 617, 615, 613, 611, 609, 608, 606, 604, 603, 601, 599, 598, 596, 595, 593, 592, 591, 589, 588, 586, 585, 584, 582, 581, 580, 579, 577, 576, 575, 574, 573, 572, 570, 569, 568, 567, 566, 565, 564, 563, 562, 561, 560, 559, 558, 557, 556, 555, 554, 553, 552, 551, 550, 549, 548, 547, 547, 546, 545, 544, 543, 542, 541, 540, 540, 539, 538, 537, 536, 536, 535, 534, 533, 532, 532, 531, 530, 529, 528, 528, 527, 526, 525, 525, 524, 523, 522, 522, 521, 520, 520, 519}

// Bls12381G2MultiExpDiscountTable is the gas discount table for BLS12-381 G2 multi exponentiation operation
var Bls12381G2MultiExpDiscountTable = [128]uint64{1000, 1000, 923, 884, 855, 832, 812, 796, 782, 770, 759, 749, 740, 732, 724, 717, 711, 704, 699, 693, 688, 683, 679, 674, 670, 666, 663, 659, 655, 652, 649, 646, 643, 640, 637, 634, 632, 629, 627, 624, 622, 620, 618, 615, 613, 611, 609, 607, 606, 604, 602, 600, 598, 597, 595, 593, 592, 590, 589, 587, 586, 584, 583, 582, 580, 579, 578, 576, 575, 574, 573, 571, 570, 569, 568, 567, 566, 565, 563, 562, 561, 560, 559, 558, 557, 556, 555, 554, 553, 552, 552, 551, 550, 549, 548, 547, 546, 545, 545, 544, 543, 542, 541, 541, 540, 539, 538, 537, 537, 536, 535, 535, 534, 533, 532, 532, 531, 530, 530, 529, 528, 528, 527, 526, 526, 525, 524, 524}

// Difficulty parameters based on your genesis
var (
	DifficultyBoundDivisor = big.NewInt(2048)    // The bound divisor of the difficulty
	GenesisDifficulty      = big.NewInt(4194304) // YOUR genesis difficulty (0x400000)
	MinimumDifficulty      = big.NewInt(131072)  // The minimum that the difficulty may ever be
	DurationLimit          = big.NewInt(13)      // The decision boundary on the blocktime duration
)

var (
	SystemAddress             = common.HexToAddress("0xfffffffffffffffffffffffffffffffffffffffe")
	BeaconRootsAddress        = common.HexToAddress("0x000F3df6D732807Ef1319fB7B8bB8522d0Beac02")
	BeaconRootsCode           = common.FromHex("")
	HistoryStorageAddress     = common.HexToAddress("0x0000F90827F1C53a10cb7A02335B175320002935")
	HistoryStorageCode        = common.FromHex("")
	WithdrawalQueueAddress    = common.HexToAddress("0x00000961Ef480Eb55e80D19ad83579A64c007002")
	WithdrawalQueueCode       = common.FromHex("")
	ConsolidationQueueAddress = common.HexToAddress("0x0000BBdDc7CE488642fb579F8B00f3a590007251")
	ConsolidationQueueCode    = common.FromHex("")
)

var (
	EthTransferLogEvent = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	EthBurnLogEvent     = common.HexToHash("0xcc16f5dbb4873280815c1ee09dbd06736cffcc184412cf7a71a0fdb75d397ca5")
)
