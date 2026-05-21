//go:build !cgo || !randomx
// +build !cgo !randomx

package randomx

import "fmt"

const (
	RANDOMX_FLAG_DEFAULT  = 0
	RANDOMX_FLAG_FULL_MEM = 1
	DatasetItemCount      = 0
)

type Cache struct{}
type Dataset struct{}
type VM struct{}

func NewCache(_ int) (*Cache, error)                             { return nil, fmt.Errorf("randomx requires cgo") }
func (c *Cache) Init(seed []byte)                                {}
func (c *Cache) Close()                                          {}
func NewDataset(_ int) (*Dataset, error)                         { return nil, fmt.Errorf("randomx requires cgo") }
func (d *Dataset) InitDataset(cache *Cache, start, count uint64) {}
func (d *Dataset) Close()                                        {}
func NewVM(_ int, cache *Cache, dataset *Dataset) (*VM, error) {
	return nil, fmt.Errorf("randomx requires cgo")
}
func (vm *VM) CalculateHash(input []byte, output []byte) {}
func (vm *VM) Close()                                    {}

func Available() bool { return false }
