//go:build cgo && randomx
// +build cgo,randomx

package randomx

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -L/usr/local/lib -lrandomx -lstdc++ -lm

#include <stdlib.h>
#include <randomx.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	RANDOMX_FLAG_DEFAULT  = 0
	RANDOMX_FLAG_FULL_MEM = 1
	DatasetItemCount      = 0
)

type Cache struct{ ptr *C.randomx_cache }
type Dataset struct{ ptr *C.randomx_dataset }
type VM struct{ ptr *C.randomx_vm }

func NewCache(_ int) (*Cache, error) {
	ptr := C.randomx_alloc_cache(C.randomx_get_flags())
	if ptr == nil {
		return nil, fmt.Errorf("failed to allocate RandomX cache")
	}
	return &Cache{ptr: ptr}, nil
}

func (c *Cache) Init(seed []byte) {
	if len(seed) == 0 || c == nil || c.ptr == nil {
		return
	}
	C.randomx_init_cache(c.ptr, unsafe.Pointer(&seed[0]), C.size_t(len(seed)))
}

func (c *Cache) Close() {
	if c != nil && c.ptr != nil {
		C.randomx_release_cache(c.ptr)
		c.ptr = nil
	}
}

func NewDataset(_ int) (*Dataset, error) {
	ptr := C.randomx_alloc_dataset(C.randomx_get_flags())
	if ptr == nil {
		return nil, fmt.Errorf("failed to allocate RandomX dataset")
	}
	return &Dataset{ptr: ptr}, nil
}

func (d *Dataset) InitDataset(cache *Cache, start, count uint64) {
	if d == nil || d.ptr == nil || cache == nil || cache.ptr == nil {
		return
	}
	C.randomx_init_dataset(d.ptr, cache.ptr, C.ulong(start), C.ulong(count))
}

func (d *Dataset) Close() {
	if d != nil && d.ptr != nil {
		C.randomx_release_dataset(d.ptr)
		d.ptr = nil
	}
}

func NewVM(_ int, cache *Cache, dataset *Dataset) (*VM, error) {
	var cptr *C.randomx_cache
	var dptr *C.randomx_dataset
	if cache != nil {
		cptr = cache.ptr
	}
	if dataset != nil {
		dptr = dataset.ptr
	}
	ptr := C.randomx_create_vm(C.randomx_get_flags(), cptr, dptr)
	if ptr == nil {
		return nil, fmt.Errorf("failed to create RandomX VM")
	}
	return &VM{ptr: ptr}, nil
}

func (vm *VM) CalculateHash(input []byte, output []byte) {
	if vm == nil || vm.ptr == nil || len(input) == 0 || len(output) < 32 {
		return
	}
	C.randomx_calculate_hash(vm.ptr, unsafe.Pointer(&input[0]), C.size_t(len(input)), unsafe.Pointer(&output[0]))
}

func (vm *VM) Close() {
	if vm != nil && vm.ptr != nil {
		C.randomx_destroy_vm(vm.ptr)
		vm.ptr = nil
	}
}

func Available() bool { return true }
