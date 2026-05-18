// Copyright 2016 The go-ethereum Authors
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
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Genesis hashes to enforce below configs on.
var (
	MainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	SepoliaGenesisHash = common.HexToHash("0x25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9")
	HoleskyGenesisHash = common.HexToHash("0xb5f7f912443c940f21fd611f12828d75b534364ed9e95ca4e307729a4661bde4")
	HoodiGenesisHash   = common.HexToHash("0xbbe312868b376a3001692a646dd2d7d1e4406380dfd86b98aa8a34d1557c971b")
)

func newUint64(val uint64) *uint64 { return &val }

// RandomXConfig is the consensus engine configs for RandomX proof-of-work based sealing.
type RandomXConfig struct {
	EpochLength   uint64 `json:"epochLength"`   // Blocks per epoch (default: 2048)
	CacheSizeMB   uint64 `json:"cacheSizeMB"`   // Cache size in MB (default: 256)
	DatasetSizeGB uint64 `json:"datasetSizeGB"` // Dataset size in GB (default: 2)
	MinMemory     uint64 `json:"minMemory"`     // Minimum memory required in bytes (default: 4GB)
}

// String implements the stringer interface, returning the consensus engine details.
func (c RandomXConfig) String() string {
	return fmt.Sprintf("randomx(epoch: %d, cache: %dMB, dataset: %dGB)",
		c.EpochLength, c.CacheSizeMB, c.DatasetSizeGB)
}

// DefaultRandomXConfig returns the default RandomX configuration.
func DefaultRandomXConfig() *RandomXConfig {
	return &RandomXConfig{
		EpochLength:   2048,
		CacheSizeMB:   256,
		DatasetSizeGB: 2,
		MinMemory:     4 * 1024 * 1024 * 1024, // 4GB
	}
}

// ChainConfig is the core config which determines the blockchain settings.
type ChainConfig struct {
	ChainID *big.Int `json:"chainId"`

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"`
	DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`
	DAOForkSupport bool     `json:"daoForkSupport,omitempty"`
	EIP150Block    *big.Int `json:"eip150Block,omitempty"`
	EIP155Block    *big.Int `json:"eip155Block,omitempty"`
	EIP158Block    *big.Int `json:"eip158Block,omitempty"`
	ByzantiumBlock *big.Int `json:"byzantiumBlock,omitempty"`

	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"`
	PetersburgBlock     *big.Int `json:"petersburgBlock,omitempty"`
	IstanbulBlock       *big.Int `json:"istanbulBlock,omitempty"`
	BerlinBlock         *big.Int `json:"berlinBlock,omitempty"`
	LondonBlock         *big.Int `json:"londonBlock,omitempty"`
	ArrowGlacierBlock   *big.Int `json:"arrowGlacierBlock,omitempty"`
	GrayGlacierBlock    *big.Int `json:"grayGlacierBlock,omitempty"`

	ShanghaiTime *uint64 `json:"shanghaiTime,omitempty"`
	CancunTime   *uint64 `json:"cancunTime,omitempty"`
	PragueTime   *uint64 `json:"pragueTime,omitempty"`
	OsakaTime    *uint64 `json:"osakaTime,omitempty"`
	BPO1Time     *uint64 `json:"bpo1Time,omitempty"`
	BPO2Time     *uint64 `json:"bpo2Time,omitempty"`
	BPO3Time     *uint64 `json:"bpo3Time,omitempty"`
	BPO4Time     *uint64 `json:"bpo4Time,omitempty"`
	BPO5Time     *uint64 `json:"bpo5Time,omitempty"`
	AmsterdamTime *uint64 `json:"amsterdamTime,omitempty"`
	UBTTime      *uint64 `json:"ubtTime,omitempty"`

	// RandomX consensus engine
	RandomX            *RandomXConfig       `json:"randomx,omitempty"`
	BlobScheduleConfig *BlobScheduleConfig `json:"blobSchedule,omitempty"`
}

// String implements the fmt.Stringer interface.
func (c *ChainConfig) String() string {
	return fmt.Sprintf("ChainConfig{ChainID: %v, RandomX: %v}", c.ChainID, c.RandomX)
}

// BlobConfig specifies the target and max blobs per block.
type BlobConfig struct {
	Target         int    `json:"target"`
	Max            int    `json:"max"`
	UpdateFraction uint64 `json:"baseFeeUpdateFraction"`
}

func (bc *BlobConfig) String() string {
	if bc == nil {
		return "nil"
	}
	return fmt.Sprintf("target: %d, max: %d, fraction: %d", bc.Target, bc.Max, bc.UpdateFraction)
}

// BlobScheduleConfig determines blob config per fork.
type BlobScheduleConfig struct {
	Cancun    *BlobConfig `json:"cancun,omitempty"`
	Prague    *BlobConfig `json:"prague,omitempty"`
	Osaka     *BlobConfig `json:"osaka,omitempty"`
	BPO1      *BlobConfig `json:"bpo1,omitempty"`
	BPO2      *BlobConfig `json:"bpo2,omitempty"`
	BPO3      *BlobConfig `json:"bpo3,omitempty"`
	BPO4      *BlobConfig `json:"bpo4,omitempty"`
	BPO5      *BlobConfig `json:"bpo5,omitempty"`
	Amsterdam *BlobConfig `json:"amsterdam,omitempty"`
	UBT       *BlobConfig `json:"ubt,omitempty"`
}

var (
	DefaultCancunBlobConfig = &BlobConfig{
		Target:         3,
		Max:            6,
		UpdateFraction: 3338477,
	}
	DefaultPragueBlobConfig = &BlobConfig{
		Target:         6,
		Max:            9,
		UpdateFraction: 5007716,
	}
	DefaultOsakaBlobConfig = &BlobConfig{
		Target:         6,
		Max:            9,
		UpdateFraction: 5007716,
	}
	DefaultBPO1BlobConfig = &BlobConfig{
		Target:         10,
		Max:            15,
		UpdateFraction: 8346193,
	}
	DefaultBPO2BlobConfig = &BlobConfig{
		Target:         14,
		Max:            21,
		UpdateFraction: 11684671,
	}
	DefaultBPO3BlobConfig = &BlobConfig{
		Target:         21,
		Max:            32,
		UpdateFraction: 20609697,
	}
	DefaultBPO4BlobConfig = &BlobConfig{
		Target:         14,
		Max:            21,
		UpdateFraction: 13739630,
	}
	DefaultBPO5BlobConfig = &BlobConfig{
		Target:         14,
		Max:            21,
		UpdateFraction: 13739630,
	}
)

// MainnetChainConfig is the chain parameters for RandomX mainnet.
var MainnetChainConfig = &ChainConfig{
	ChainID:                 big.NewInt(1),
	HomesteadBlock:          big.NewInt(0),
	DAOForkBlock:            nil,
	DAOForkSupport:          true,
	EIP150Block:             big.NewInt(0),
	EIP155Block:             big.NewInt(0),
	EIP158Block:             big.NewInt(0),
	ByzantiumBlock:          big.NewInt(0),
	ConstantinopleBlock:     big.NewInt(0),
	PetersburgBlock:         big.NewInt(0),
	IstanbulBlock:           big.NewInt(0),
	BerlinBlock:             big.NewInt(0),
	LondonBlock:             big.NewInt(0),
	ArrowGlacierBlock:       nil,
	GrayGlacierBlock:        nil,
	ShanghaiTime:            newUint64(0),
	CancunTime:              newUint64(0),
	PragueTime:              nil,
	OsakaTime:               nil,
	BPO1Time:                nil,
	BPO2Time:                nil,
	BPO3Time:                nil,
	BPO4Time:                nil,
	BPO5Time:                nil,
	AmsterdamTime:           nil,
	UBTTime:                 nil,
	RandomX:                 DefaultRandomXConfig(),
	BlobScheduleConfig: &BlobScheduleConfig{
		Cancun: DefaultCancunBlobConfig,
	},
}

// TestChainConfig is the configuration for testing RandomX networks.
var TestChainConfig = &ChainConfig{
	ChainID:                 big.NewInt(1337),
	HomesteadBlock:          big.NewInt(0),
	DAOForkBlock:            nil,
	DAOForkSupport:          false,
	EIP150Block:             big.NewInt(0),
	EIP155Block:             big.NewInt(0),
	EIP158Block:             big.NewInt(0),
	ByzantiumBlock:          big.NewInt(0),
	ConstantinopleBlock:     big.NewInt(0),
	PetersburgBlock:         big.NewInt(0),
	IstanbulBlock:           big.NewInt(0),
	BerlinBlock:             big.NewInt(0),
	LondonBlock:             big.NewInt(0),
	ArrowGlacierBlock:       nil,
	GrayGlacierBlock:        nil,
	ShanghaiTime:            nil,
	CancunTime:              nil,
	PragueTime:              nil,
	OsakaTime:               nil,
	BPO1Time:                nil,
	BPO2Time:                nil,
	BPO3Time:                nil,
	BPO4Time:                nil,
	BPO5Time:                nil,
	AmsterdamTime:           nil,
	UBTTime:                 nil,
	RandomX:                 DefaultRandomXConfig(),
	BlobScheduleConfig: &BlobScheduleConfig{
		Cancun: DefaultCancunBlobConfig,
	},
}

// IsHomestead returns whether num is at or beyond Homestead.
func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return isBlockForked(c.HomesteadBlock, num)
}

// IsDAOFork returns whether num is at or beyond DAO fork.
func (c *ChainConfig) IsDAOFork(num *big.Int) bool {
	return isBlockForked(c.DAOForkBlock, num)
}

// IsEIP150 returns whether num is at or beyond EIP150.
func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return isBlockForked(c.EIP150Block, num)
}

// IsEIP155 returns whether num is at or beyond EIP155.
func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return isBlockForked(c.EIP155Block, num)
}

// IsEIP158 returns whether num is at or beyond EIP158.
func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return isBlockForked(c.EIP158Block, num)
}

// IsByzantium returns whether num is at or beyond Byzantium.
func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return isBlockForked(c.ByzantiumBlock, num)
}

// IsConstantinople returns whether num is at or beyond Constantinople.
func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isBlockForked(c.ConstantinopleBlock, num)
}

// IsPetersburg returns whether num is at or beyond Petersburg.
func (c *ChainConfig) IsPetersburg(num *big.Int) bool {
	return isBlockForked(c.PetersburgBlock, num) || (c.PetersburgBlock == nil && isBlockForked(c.ConstantinopleBlock, num))
}

// IsIstanbul returns whether num is at or beyond Istanbul.
func (c *ChainConfig) IsIstanbul(num *big.Int) bool {
	return isBlockForked(c.IstanbulBlock, num)
}

// IsBerlin returns whether num is at or beyond Berlin.
func (c *ChainConfig) IsBerlin(num *big.Int) bool {
	return isBlockForked(c.BerlinBlock, num)
}

// IsLondon returns whether num is at or beyond London.
func (c *ChainConfig) IsLondon(num *big.Int) bool {
	return isBlockForked(c.LondonBlock, num)
}

// IsArrowGlacier returns whether num is at or beyond Arrow Glacier.
func (c *ChainConfig) IsArrowGlacier(num *big.Int) bool {
	return isBlockForked(c.ArrowGlacierBlock, num)
}

// IsGrayGlacier returns whether num is at or beyond Gray Glacier.
func (c *ChainConfig) IsGrayGlacier(num *big.Int) bool {
	return isBlockForked(c.GrayGlacierBlock, num)
}

// Timestamp-based fork checks
func (c *ChainConfig) IsShanghai(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.ShanghaiTime, time)
}

func (c *ChainConfig) IsMuirGlacier(num *big.Int) bool {
	return false // Muir Glacier not activated in RandomX chain
}

// IsEip1559 returns true if London fork is active
func (c *ChainConfig) IsEip1559(num *big.Int) bool {
	return c.IsLondon(num)
}

func (c *ChainConfig) IsCancun(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.CancunTime, time)
}

func (c *ChainConfig) IsPrague(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.PragueTime, time)
}

func (c *ChainConfig) IsOsaka(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.OsakaTime, time)
}

func (c *ChainConfig) IsBPO1(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.BPO1Time, time)
}

func (c *ChainConfig) IsBPO2(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.BPO2Time, time)
}

func (c *ChainConfig) IsBPO3(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.BPO3Time, time)
}

func (c *ChainConfig) IsBPO4(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.BPO4Time, time)
}

func (c *ChainConfig) IsBPO5(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.BPO5Time, time)
}

func (c *ChainConfig) IsAmsterdam(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.AmsterdamTime, time)
}

func (c *ChainConfig) IsUBT(num *big.Int, time uint64) bool {
	return c.IsLondon(num) && isTimestampForked(c.UBTTime, time)
}

// BaseFeeChangeDenominator bounds the amount the base fee can change between blocks.
func (c *ChainConfig) BaseFeeChangeDenominator() uint64 {
	return DefaultBaseFeeChangeDenominator
}

// ElasticityMultiplier bounds the maximum gas limit an EIP-1559 block may have.
func (c *ChainConfig) ElasticityMultiplier() uint64 {
	return DefaultElasticityMultiplier
}

// Rules wraps ChainConfig for fork-specific rules.
type Rules struct {
	IsHomestead, IsEIP150, IsEIP155, IsEIP158               bool
	IsByzantium, IsConstantinople, IsPetersburg, IsIstanbul bool
	IsBerlin, IsLondon                                      bool
	IsArrowGlacier, IsGrayGlacier                           bool
	IsShanghai, IsCancun, IsPrague, IsOsaka                 bool
	IsBPO1, IsBPO2, IsBPO3, IsBPO4, IsBPO5                  bool
	IsAmsterdam, IsUBT                                      bool
	IsEIP2929, IsEIP4762                                    bool
	IsMerge                                                 bool // Always false for RandomX
}

// Rules returns the rules for the given block number and timestamp.
func (c *ChainConfig) Rules(num *big.Int, timestamp uint64) Rules {
	// EIP2929 is active from Berlin fork, but disabled during UBT/VERKLE
	isEIP2929 := c.IsBerlin(num) && !c.IsUBT(num, timestamp)
	isEIP4762 := c.IsUBT(num, timestamp)

	return Rules{
		IsHomestead:      c.IsHomestead(num),
		IsEIP150:         c.IsEIP150(num),
		IsEIP155:         c.IsEIP155(num),
		IsEIP158:         c.IsEIP158(num),
		IsByzantium:      c.IsByzantium(num),
		IsConstantinople: c.IsConstantinople(num),
		IsPetersburg:     c.IsPetersburg(num),
		IsIstanbul:       c.IsIstanbul(num),
		IsBerlin:         c.IsBerlin(num),
		IsLondon:         c.IsLondon(num),
		IsArrowGlacier:   c.IsArrowGlacier(num),
		IsGrayGlacier:    c.IsGrayGlacier(num),
		IsShanghai:       c.IsShanghai(num, timestamp),
		IsCancun:         c.IsCancun(num, timestamp),
		IsPrague:         c.IsPrague(num, timestamp),
		IsOsaka:          c.IsOsaka(num, timestamp),
		IsBPO1:           c.IsBPO1(num, timestamp),
		IsBPO2:           c.IsBPO2(num, timestamp),
		IsBPO3:           c.IsBPO3(num, timestamp),
		IsBPO4:           c.IsBPO4(num, timestamp),
		IsBPO5:           c.IsBPO5(num, timestamp),
		IsAmsterdam:      c.IsAmsterdam(num, timestamp),
		IsUBT:            c.IsUBT(num, timestamp),
		IsEIP2929:        isEIP2929,
		IsEIP4762:        isEIP4762,
		IsMerge:          false, // RandomX chain never merged to PoS
	}
}

// Helper functions
func isBlockForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func isTimestampForked(s *uint64, head uint64) bool {
	if s == nil {
		return false
	}
	return *s <= head
}

// Constants
const (
	DefaultBaseFeeChangeDenominator = 8
	DefaultElasticityMultiplier     = 2

)
