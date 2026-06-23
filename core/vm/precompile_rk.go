package vm

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// Address chosen for this precompile: 0x00000000000000000000000000000000000000f1
var RKPrecompileAddr = common.HexToAddress("0x00000000000000000000000000000000000000f1")

type rkResult struct {
	CurrentKing string `json:"currentKing"`
	NextKing    string `json:"nextKing"`
	TotalKings  int    `json:"totalKings"`
}

// rkPrecompile is a small deterministic helper that produces the return bytes and
// an estimated gas cost. Replace the placeholders with a deterministic getter
// from your rotatingking subsystem.
func rkPrecompile(input []byte) ([]byte, uint64, error) {
	res := rkResult{
		CurrentKing: "0x0000000000000000000000000000000000000000", // TODO: wire to real value
		NextKing:    "0x0000000000000000000000000000000000000001", // TODO: wire to real value
		TotalKings:  0,                                         // TODO: wire to real value
	}
	out, err := json.Marshal(res)
	if err != nil {
		return nil, 0, fmt.Errorf("rk precompile marshal: %w", err)
	}
	const gasCost = 300 // tune later
	return out, gasCost, nil
}

// rkPrecompileContract implements the PrecompiledContract interface so it can be
// registered in the maps in contracts.go.
type rkPrecompileContract struct{}

func (r *rkPrecompileContract) RequiredGas(input []byte) uint64 {
	// Return a conservative fixed gas estimate; you may compute based on input if needed.
	_, gas, _ := rkPrecompile(input)
	return gas
}

func (r *rkPrecompileContract) Run(input []byte) ([]byte, error) {
	out, _, err := rkPrecompile(input)
	return out, err
}

func (r *rkPrecompileContract) Name() string {
	return "ROTATINGKING"
}
