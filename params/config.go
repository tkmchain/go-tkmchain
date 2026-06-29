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
	"github.com/ethereum/go-ethereum/params/forks"
)

// Genesis hashes to enforce below configs on.
var (
	MainnetGenesisHash = common.HexToHash("0x6bdca03e891cd028a92355065c211ead725d3e3be9f4de1047c3c5faa464a55e")
	SepoliaGenesisHash = common.HexToHash("0x25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9")
	HoleskyGenesisHash = common.HexToHash("0xb5f7f912443c940f21fd611f12828d75b534364ed9e95ca4e307729a4661bde4")
	HoodiGenesisHash   = common.HexToHash("0xbbe312868b376a3001692a646dd2d7d1e4406380dfd86b98aa8a34d1557c971b")
        RandomXGenesisHash = common.HexToHash("0x6bdca03e891cd028a92355065c211ead725d3e3be9f4de1047c3c5faa464a55e")

)

func newUint64(val uint64) *uint64 { return &val }

// RandomXConfig is the consensus engine configs for RandomX proof-of-work based sealing.
type RandomXConfig struct {
	EpochLength    uint64 `json:"epochLength"`       // Blocks per epoch (default: 2048)
	CacheSizeMB    uint64 `json:"cacheSizeMB"`       // Cache size in MB (default: 256)
	DatasetSizeGB  uint64 `json:"datasetSizeGB"`     // Dataset size in GB (default: 2)
	MinMemory      uint64 `json:"minMemory"`         // Minimum memory required in bytes (default: 4GB)
        UseRAMCache    bool   `json:"useRAMCache"`       // Use RAM instead of disk
        PersistDataset bool   `json:"persistDataset"`    // Persist dataset to disk
}

// RandomXChainConfig
var RandomXChainConfig = &ChainConfig{
        ChainID:                 big.NewInt(8979),
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
        DepositContractAddress:  common.HexToAddress("0x00000000219ab540356cBB839Cbe05303d7705Fa"),
        MainKingAddress:         common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2"),
        RotatingKingAddresses: []common.Address{
                common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2"),
        },
        RotatingKingRotationInterval: 100,
        RandomX:                      DefaultRandomXConfig(),
        BlobScheduleConfig: &BlobScheduleConfig{
                Cancun: DefaultCancunBlobConfig,
        },
}
// String implements the stringer interface, returning the consensus engine details.
func (c RandomXConfig) String() string {
	return fmt.Sprintf("randomx(epoch: %d, cache: %dMB, dataset: %dGB)",
		c.EpochLength, c.CacheSizeMB, c.DatasetSizeGB)
}

// DefaultRandomXConfig returns the default RandomX configuration.
func DefaultRandomXConfig() *RandomXConfig {
	return &RandomXConfig{
		EpochLength:    2048,
		CacheSizeMB:    256,
		DatasetSizeGB:  2,
		MinMemory:      4 * 1024 * 1024 * 1024, // 4GB
                UseRAMCache:    false,
                PersistDataset: true,
	}
}

// CliqueConfig is the consensus engine configs for proof-of-authority based sealing.
type CliqueConfig struct {
	Period uint64 `json:"period"` // Number of seconds between blocks to enforce
	Epoch  uint64 `json:"epoch"`  // Epoch length to reset votes and checkpoint
}

// String implements the stringer interface, returning the consensus engine details.
func (c CliqueConfig) String() string {
	return "clique"
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

	ShanghaiTime  *uint64 `json:"shanghaiTime,omitempty"`
	CancunTime    *uint64 `json:"cancunTime,omitempty"`
	PragueTime    *uint64 `json:"pragueTime,omitempty"`
	OsakaTime     *uint64 `json:"osakaTime,omitempty"`
	BPO1Time      *uint64 `json:"bpo1Time,omitempty"`
	BPO2Time      *uint64 `json:"bpo2Time,omitempty"`
	BPO3Time      *uint64 `json:"bpo3Time,omitempty"`
	BPO4Time      *uint64 `json:"bpo4Time,omitempty"`
	BPO5Time      *uint64 `json:"bpo5Time,omitempty"`
	AmsterdamTime *uint64 `json:"amsterdamTime,omitempty"`
	UBTTime       *uint64 `json:"ubtTime,omitempty"`

	EnableUBTAtGenesis bool `json:"enableUBTAtGenesis,omitempty"`

	DepositContractAddress       common.Address   `json:"depositContractAddress,omitempty"`
	MainKingAddress              common.Address   `json:"mainKingAddress,omitempty"`
	RotatingKingAddresses        []common.Address `json:"rotatingKingAddresses,omitempty"`
	RotatingKingRotationInterval uint64           `json:"rotatingKingRotationInterval,omitempty"`

	// RandomX consensus engine
	RandomX            *RandomXConfig      `json:"randomx,omitempty"`
	Clique             *CliqueConfig       `json:"clique,omitempty"`
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
// Values are based on your actual genesis.json
var MainnetChainConfig = &ChainConfig{
	ChainID:                 big.NewInt(8979), // Your genesis chainId is 1
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
	DepositContractAddress:  common.HexToAddress("0x00000000219ab540356cBB839Cbe05303d7705Fa"),
	MainKingAddress:         common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2"),
	RotatingKingAddresses: []common.Address{
		common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2"), // Same address for now
	},
	RotatingKingRotationInterval: 100,
	RandomX:                      DefaultRandomXConfig(),
	BlobScheduleConfig: &BlobScheduleConfig{
		Cancun: DefaultCancunBlobConfig,
	},
}

// TestChainConfig is the configuration for testing RandomX networks.
var TestChainConfig = &ChainConfig{
	ChainID:             big.NewInt(1),
	HomesteadBlock:      big.NewInt(0),
	DAOForkBlock:        nil,
	DAOForkSupport:      false,
	EIP150Block:         big.NewInt(0),
	EIP155Block:         big.NewInt(0),
	EIP158Block:         big.NewInt(0),
	ByzantiumBlock:      big.NewInt(0),
	ConstantinopleBlock: big.NewInt(0),
	PetersburgBlock:     big.NewInt(0),
	IstanbulBlock:       big.NewInt(0),
	BerlinBlock:         big.NewInt(0),
	LondonBlock:         big.NewInt(0),
	ArrowGlacierBlock:   nil,
	GrayGlacierBlock:    nil,
	ShanghaiTime:        nil,
	CancunTime:          nil,
	PragueTime:          nil,
	OsakaTime:           nil,
	BPO1Time:            nil,
	BPO2Time:            nil,
	BPO3Time:            nil,
	BPO4Time:            nil,
	BPO5Time:            nil,
	AmsterdamTime:       nil,
	UBTTime:             nil,
	MainKingAddress:     common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2"),
	RotatingKingAddresses: []common.Address{
		common.HexToAddress("0xc40f4a0b4df81f8f67a88b179a8b2271107a9ac2"),
	},
	RotatingKingRotationInterval: 100,
	RandomX:                      DefaultRandomXConfig(),
	BlobScheduleConfig: &BlobScheduleConfig{
		Cancun: DefaultCancunBlobConfig,
	},
}

var (
        AllRandomXProtocolChanges  = TestChainConfig
        AllCliqueProtocolChanges   = &ChainConfig{
                ChainID:             big.NewInt(1337),
                HomesteadBlock:      big.NewInt(0),
                DAOForkBlock:        nil,
                DAOForkSupport:      false,
                EIP150Block:         big.NewInt(0),
                EIP155Block:         big.NewInt(0),
                EIP158Block:         big.NewInt(0),
                ByzantiumBlock:      big.NewInt(0),
                ConstantinopleBlock: big.NewInt(0),
                PetersburgBlock:     big.NewInt(0),
                IstanbulBlock:       big.NewInt(0),
                BerlinBlock:         big.NewInt(0),
                LondonBlock:         big.NewInt(0),
                Clique: &CliqueConfig{
                        Period: 0,
                        Epoch:  30000,
                },
                BlobScheduleConfig: &BlobScheduleConfig{
                        Cancun: DefaultCancunBlobConfig,
                },
        }
        AllDevChainProtocolChanges = TestChainConfig
        MergedTestChainConfig      = &ChainConfig{
                ChainID:             big.NewInt(1337),
                HomesteadBlock:      big.NewInt(0),
                DAOForkBlock:        nil,
                DAOForkSupport:      false,
                EIP150Block:         big.NewInt(0),
                EIP155Block:         big.NewInt(0),
                EIP158Block:         big.NewInt(0),
                ByzantiumBlock:      big.NewInt(0),
                ConstantinopleBlock: big.NewInt(0),
                PetersburgBlock:     big.NewInt(0),
                IstanbulBlock:       big.NewInt(0),
                BerlinBlock:         big.NewInt(0),
                LondonBlock:         big.NewInt(0),
                ShanghaiTime:        newUint64(0),
                CancunTime:          newUint64(0),
                PragueTime:          newUint64(0),
                OsakaTime:           newUint64(0),
                RandomX:             DefaultRandomXConfig(),
                BlobScheduleConfig: &BlobScheduleConfig{
                        Cancun: DefaultCancunBlobConfig,
                        Prague: DefaultPragueBlobConfig,
                        Osaka:  DefaultOsakaBlobConfig,
                },
        }
        SepoliaChainConfig = TestChainConfig
        HoleskyChainConfig = TestChainConfig
        HoodiChainConfig   = TestChainConfig
)

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
func (c *ChainConfig) Rules(num *big.Int, isMerge bool, timestamp uint64) Rules {
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
                IsMerge:          false, // RandomX chains always use proof-of-work consensus.
        }
}

// IsUBTGenesis reports whether the UBT fork is enabled from genesis.
func (c *ChainConfig) IsUBTGenesis() bool {
        return c != nil && c.EnableUBTAtGenesis
}

// CheckConfigForkOrder validates that configured forks are ordered by activation.
func (c *ChainConfig) CheckConfigForkOrder() error {
        if c == nil {
                return nil
        }
        blockForks := []struct {
                name     string
                block    *big.Int
                optional bool
        }{
                {"homesteadBlock", c.HomesteadBlock, false},
                {"daoForkBlock", c.DAOForkBlock, true},
                {"eip150Block", c.EIP150Block, false},
                {"eip155Block", c.EIP155Block, false},
                {"eip158Block", c.EIP158Block, false},
                {"byzantiumBlock", c.ByzantiumBlock, false},
                {"constantinopleBlock", c.ConstantinopleBlock, false},
                {"petersburgBlock", c.PetersburgBlock, true},
                {"istanbulBlock", c.IstanbulBlock, false},
                {"berlinBlock", c.BerlinBlock, false},
                {"londonBlock", c.LondonBlock, false},
                {"arrowGlacierBlock", c.ArrowGlacierBlock, true},
                {"grayGlacierBlock", c.GrayGlacierBlock, true},
        }
        var lastName string
        var lastBlock *big.Int
        for _, fork := range blockForks {
                if fork.block == nil {
                        if !fork.optional {
                                lastName, lastBlock = fork.name, nil
                        }
                        continue
                }
                if lastBlock != nil && fork.block.Cmp(lastBlock) < 0 {
                        return fmt.Errorf("unsupported fork ordering: %s enabled at block %v, but %s enabled at block %v", lastName, lastBlock, fork.name, fork.block)
                }
                lastName, lastBlock = fork.name, fork.block
        }
        timeForks := []struct {
                name string
                time *uint64
        }{
                {"shanghaiTime", c.ShanghaiTime},
                {"cancunTime", c.CancunTime},
                {"pragueTime", c.PragueTime},
                {"osakaTime", c.OsakaTime},
                {"bpo1Time", c.BPO1Time},
                {"bpo2Time", c.BPO2Time},
                {"bpo3Time", c.BPO3Time},
                {"bpo4Time", c.BPO4Time},
                {"bpo5Time", c.BPO5Time},
                {"amsterdamTime", c.AmsterdamTime},
                {"ubtTime", c.UBTTime},
        }
        lastName = ""
        var lastTime *uint64
        for _, fork := range timeForks {
                if fork.time == nil {
                        continue
                }
                if lastTime != nil && *fork.time < *lastTime {
                        return fmt.Errorf("unsupported fork ordering: %s enabled at timestamp %v, but %s enabled at timestamp %v", lastName, *lastTime, fork.name, *fork.time)
                }
                lastName, lastTime = fork.name, fork.time
        }
        return nil
}

// ConfigCompatError is raised if a stored chain config is incompatible with a new one.
type ConfigCompatError struct {
        What          string
        StoredBlock   *big.Int
        NewBlock      *big.Int
        StoredTime    *uint64
        NewTime       *uint64
        RewindToBlock uint64
        RewindToTime  uint64
}

func (err *ConfigCompatError) Error() string {
        if err == nil || err.What == "" {
                return ""
        }
        if err.StoredTime != nil || err.NewTime != nil {
                return fmt.Sprintf("mismatching %s in database (have timestamp %v, want timestamp %v, rewindto timestamp %v)", err.What, configTimestamp(err.StoredTime), configTimestamp(err.NewTime), err.RewindToTime)
        }
        return fmt.Sprintf("mismatching %s in database (have block %v, want block %v, rewindto block %v)", err.What, err.StoredBlock, err.NewBlock, err.RewindToBlock)
}

func configTimestamp(timestamp *uint64) any {
        if timestamp == nil {
                return "nil"
        }
        return *timestamp
}

func newBlockCompatError(what string, storedBlock, newBlock *big.Int) *ConfigCompatError {
        rewind := uint64(0)
        var rew *big.Int
        if storedBlock == nil {
                rew = newBlock
        } else if newBlock == nil || storedBlock.Cmp(newBlock) < 0 {
                rew = storedBlock
        } else {
                rew = newBlock
        }
        if rew != nil && rew.Sign() > 0 {
                rewind = new(big.Int).Sub(rew, common.Big1).Uint64()
        }
        return &ConfigCompatError{What: what, StoredBlock: storedBlock, NewBlock: newBlock, RewindToBlock: rewind}
}

func newTimestampCompatError(what string, storedTime, newTime *uint64) *ConfigCompatError {
        rewind := uint64(0)
        switch {
        case storedTime == nil && newTime != nil:
                if *newTime > 0 {
                        rewind = *newTime - 1
                }
        case storedTime != nil && newTime == nil:
                if *storedTime > 0 {
                        rewind = *storedTime - 1
                }
        case storedTime != nil && newTime != nil:
                if *storedTime < *newTime {
                        if *storedTime > 0 {
                                rewind = *storedTime - 1
                        }
                } else if *newTime > 0 {
                        rewind = *newTime - 1
                }
        }
        return &ConfigCompatError{What: what, StoredTime: storedTime, NewTime: newTime, RewindToTime: rewind}
}

// CheckCompatible checks whether scheduled forks differ after the given chain head.
func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, headBlock uint64, headTimestamp uint64) *ConfigCompatError {
        var lastErr *ConfigCompatError
        for {
                err := c.checkCompatible(newcfg, headBlock, headTimestamp)
                if err == nil {
                        return lastErr
                }
                if lastErr != nil && err.RewindToBlock == lastErr.RewindToBlock && err.RewindToTime == lastErr.RewindToTime {
                        return err
                }
                lastErr = err
                if err.RewindToBlock != 0 {
                        headBlock = err.RewindToBlock
                }
                if err.RewindToTime != 0 {
                        headTimestamp = err.RewindToTime
                }
        }
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, headBlock uint64, headTimestamp uint64) *ConfigCompatError {
        if c == nil || newcfg == nil {
                return nil
        }
        if isForkBlockIncompatible(c.HomesteadBlock, newcfg.HomesteadBlock, headBlock) {
                return newBlockCompatError("Homestead fork block", c.HomesteadBlock, newcfg.HomesteadBlock)
        }
        if isForkBlockIncompatible(c.DAOForkBlock, newcfg.DAOForkBlock, headBlock) {
                return newBlockCompatError("DAO fork block", c.DAOForkBlock, newcfg.DAOForkBlock)
        }
        if isForkBlockIncompatible(c.EIP150Block, newcfg.EIP150Block, headBlock) {
                return newBlockCompatError("EIP150 fork block", c.EIP150Block, newcfg.EIP150Block)
        }
        if isForkBlockIncompatible(c.EIP155Block, newcfg.EIP155Block, headBlock) {
                return newBlockCompatError("EIP155 fork block", c.EIP155Block, newcfg.EIP155Block)
        }
        if isForkBlockIncompatible(c.EIP158Block, newcfg.EIP158Block, headBlock) {
                return newBlockCompatError("EIP158 fork block", c.EIP158Block, newcfg.EIP158Block)
        }
        if isForkBlockIncompatible(c.ByzantiumBlock, newcfg.ByzantiumBlock, headBlock) {
                return newBlockCompatError("Byzantium fork block", c.ByzantiumBlock, newcfg.ByzantiumBlock)
        }
        if isForkBlockIncompatible(c.ConstantinopleBlock, newcfg.ConstantinopleBlock, headBlock) {
                return newBlockCompatError("Constantinople fork block", c.ConstantinopleBlock, newcfg.ConstantinopleBlock)
        }
        if isForkBlockIncompatible(c.PetersburgBlock, newcfg.PetersburgBlock, headBlock) && isForkBlockIncompatible(c.ConstantinopleBlock, newcfg.PetersburgBlock, headBlock) {
                return newBlockCompatError("Petersburg fork block", c.PetersburgBlock, newcfg.PetersburgBlock)
        }
        if isForkBlockIncompatible(c.IstanbulBlock, newcfg.IstanbulBlock, headBlock) {
                return newBlockCompatError("Istanbul fork block", c.IstanbulBlock, newcfg.IstanbulBlock)
        }
        if isForkBlockIncompatible(c.BerlinBlock, newcfg.BerlinBlock, headBlock) {
                return newBlockCompatError("Berlin fork block", c.BerlinBlock, newcfg.BerlinBlock)
        }
        if isForkBlockIncompatible(c.LondonBlock, newcfg.LondonBlock, headBlock) {
                return newBlockCompatError("London fork block", c.LondonBlock, newcfg.LondonBlock)
        }
        if isForkTimestampIncompatible(c.ShanghaiTime, newcfg.ShanghaiTime, headTimestamp) {
                return newTimestampCompatError("Shanghai fork timestamp", c.ShanghaiTime, newcfg.ShanghaiTime)
        }
        if isForkTimestampIncompatible(c.CancunTime, newcfg.CancunTime, headTimestamp) {
                return newTimestampCompatError("Cancun fork timestamp", c.CancunTime, newcfg.CancunTime)
        }
        if isForkTimestampIncompatible(c.PragueTime, newcfg.PragueTime, headTimestamp) {
                return newTimestampCompatError("Prague fork timestamp", c.PragueTime, newcfg.PragueTime)
        }
        if isForkTimestampIncompatible(c.OsakaTime, newcfg.OsakaTime, headTimestamp) {
                return newTimestampCompatError("Osaka fork timestamp", c.OsakaTime, newcfg.OsakaTime)
        }
        if isForkTimestampIncompatible(c.BPO1Time, newcfg.BPO1Time, headTimestamp) {
                return newTimestampCompatError("BPO1 fork timestamp", c.BPO1Time, newcfg.BPO1Time)
        }
        if isForkTimestampIncompatible(c.BPO2Time, newcfg.BPO2Time, headTimestamp) {
                return newTimestampCompatError("BPO2 fork timestamp", c.BPO2Time, newcfg.BPO2Time)
        }
        if isForkTimestampIncompatible(c.BPO3Time, newcfg.BPO3Time, headTimestamp) {
                return newTimestampCompatError("BPO3 fork timestamp", c.BPO3Time, newcfg.BPO3Time)
        }
        if isForkTimestampIncompatible(c.BPO4Time, newcfg.BPO4Time, headTimestamp) {
                return newTimestampCompatError("BPO4 fork timestamp", c.BPO4Time, newcfg.BPO4Time)
        }
        if isForkTimestampIncompatible(c.BPO5Time, newcfg.BPO5Time, headTimestamp) {
                return newTimestampCompatError("BPO5 fork timestamp", c.BPO5Time, newcfg.BPO5Time)
        }
        if isForkTimestampIncompatible(c.AmsterdamTime, newcfg.AmsterdamTime, headTimestamp) {
                return newTimestampCompatError("Amsterdam fork timestamp", c.AmsterdamTime, newcfg.AmsterdamTime)
        }
        if isForkTimestampIncompatible(c.UBTTime, newcfg.UBTTime, headTimestamp) {
                return newTimestampCompatError("UBT fork timestamp", c.UBTTime, newcfg.UBTTime)
        }
        if c.MainKingAddress != newcfg.MainKingAddress {
                return &ConfigCompatError{What: "main king address", RewindToBlock: 0}
        }
        return nil
}

// Description returns a human-readable chain configuration summary.
func (c *ChainConfig) Description() string {
        return fmt.Sprintf("Chain ID: %v\nConsensus: RandomX\nRandomX: %v", c.ChainID, c.RandomX)
}

// LatestFork returns the latest timestamp-based fork active at the given timestamp.
func (c *ChainConfig) LatestFork(timestamp uint64) forks.Fork {
        latest := forks.London
        for _, fork := range []forks.Fork{forks.Shanghai, forks.Cancun, forks.Prague, forks.Osaka, forks.BPO1, forks.BPO2, forks.BPO3, forks.BPO4, forks.BPO5, forks.Amsterdam} {
                if t := c.Timestamp(fork); t != nil && *t <= timestamp {
                        latest = fork
                }
        }
        return latest
}

// Timestamp returns the activation timestamp of a timestamp-based fork.
func (c *ChainConfig) Timestamp(fork forks.Fork) *uint64 {
        switch fork {
        case forks.Shanghai:
                return c.ShanghaiTime
        case forks.Cancun:
                return c.CancunTime
        case forks.Prague:
                return c.PragueTime
        case forks.Osaka:
                return c.OsakaTime
        case forks.BPO1:
                return c.BPO1Time
        case forks.BPO2:
                return c.BPO2Time
        case forks.BPO3:
                return c.BPO3Time
        case forks.BPO4:
                return c.BPO4Time
        case forks.BPO5:
                return c.BPO5Time
        case forks.Amsterdam:
                return c.AmsterdamTime
        }
        return nil
}

// BlobConfig returns the blob schedule configured for fork.
func (c *ChainConfig) BlobConfig(fork forks.Fork) *BlobConfig {
        if c == nil || c.BlobScheduleConfig == nil {
                return nil
        }
        switch fork {
        case forks.Cancun:
                return c.BlobScheduleConfig.Cancun
        case forks.Prague:
                return c.BlobScheduleConfig.Prague
        case forks.Osaka:
                return c.BlobScheduleConfig.Osaka
        case forks.BPO1:
                return c.BlobScheduleConfig.BPO1
        case forks.BPO2:
                return c.BlobScheduleConfig.BPO2
        case forks.BPO3:
                return c.BlobScheduleConfig.BPO3
        case forks.BPO4:
                return c.BlobScheduleConfig.BPO4
        case forks.BPO5:
                return c.BlobScheduleConfig.BPO5
        case forks.Amsterdam:
                return c.BlobScheduleConfig.Amsterdam
        }
        return nil
}

// ActiveSystemContracts returns the currently active system contracts at the given timestamp.
func (c *ChainConfig) ActiveSystemContracts(time uint64) map[string]common.Address {
        fork := c.LatestFork(time)
        active := make(map[string]common.Address)
        if fork >= forks.Prague {
               // active["CONSOLIDATION_REQUEST_PREDEPLOY_ADDRESS"] = ConsolidationQueueAddress
               // active["DEPOSIT_CONTRACT_ADDRESS"] = c.DepositContractAddress
               // active["HISTORY_STORAGE_ADDRESS"] = HistoryStorageAddress
               // active["WITHDRAWAL_REQUEST_PREDEPLOY_ADDRESS"] = WithdrawalQueueAddress
        }
        return active
}

func isForkBlockIncompatible(storedBlock, newBlock *big.Int, head uint64) bool {
        return isBlockForked(storedBlock, new(big.Int).SetUint64(head)) != isBlockForked(newBlock, new(big.Int).SetUint64(head)) || (isBlockForked(storedBlock, new(big.Int).SetUint64(head)) && !configBlockEqual(storedBlock, newBlock))
}

func isForkTimestampIncompatible(storedTime, newTime *uint64, head uint64) bool {
        return isTimestampForked(storedTime, head) != isTimestampForked(newTime, head) || (isTimestampForked(storedTime, head) && !configTimestampEqual(storedTime, newTime))
}

func configBlockEqual(x, y *big.Int) bool {
        if x == nil {
                return y == nil
        }
        if y == nil {
                return false
        }
        return x.Cmp(y) == 0
}

func configTimestampEqual(x, y *uint64) bool {
        if x == nil {
                return y == nil
        }
        if y == nil {
                return false
        }
        return *x == *y
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
