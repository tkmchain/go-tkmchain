//go:build cgo && randomx
// +build cgo,randomx

package randomx

/*
#cgo CFLAGS: -I../../build/_workspace/randomx/src
#cgo LDFLAGS: -L../../build/_workspace/randomx/build -lrandomx -lstdc++ -lm

#include <stdlib.h>
#include <randomx.h>

// Helper function to get flags
int get_flags() {
    return randomx_get_flags();
}
*/
import "C"

import (
    "fmt"
    "unsafe"
)

// Constants matching the C++ library
const (
    RANDOMX_FLAG_DEFAULT  = 0
    RANDOMX_FLAG_FULL_MEM = 1  // For mining with dataset
    RANDOMX_FLAG_JIT      = 2  // For JIT compilation
    DatasetItemCount      = 0  // Use all items
)

type Cache struct{ ptr *C.randomx_cache }
type Dataset struct{ ptr *C.randomx_dataset }
type VM struct{ ptr *C.randomx_vm }

// NewCache creates a RandomX cache
func NewCache(flags int) (*Cache, error) {
    cflags := C.get_flags()
    ptr := C.randomx_alloc_cache(cflags)
    if ptr == nil {
        return nil, fmt.Errorf("failed to allocate RandomX cache")
    }
    return &Cache{ptr: ptr}, nil
}

// Init initializes the cache with seed
func (c *Cache) Init(seed []byte) {
    if len(seed) == 0 || c == nil || c.ptr == nil {
        return
    }
    C.randomx_init_cache(c.ptr, unsafe.Pointer(&seed[0]), C.size_t(len(seed)))
}

// Close releases the cache
func (c *Cache) Close() {
    if c != nil && c.ptr != nil {
        C.randomx_release_cache(c.ptr)
        c.ptr = nil
    }
}

// NewDataset creates a RandomX dataset
func NewDataset(flags int) (*Dataset, error) {
    cflags := C.get_flags()
    ptr := C.randomx_alloc_dataset(cflags)
    if ptr == nil {
        return nil, fmt.Errorf("failed to allocate RandomX dataset")
    }
    return &Dataset{ptr: ptr}, nil
}

// InitDataset initializes the dataset from cache
func (d *Dataset) InitDataset(cache *Cache, start, count uint64) {
    if d == nil || d.ptr == nil || cache == nil || cache.ptr == nil {
        return
    }
    C.randomx_init_dataset(d.ptr, cache.ptr, C.ulong(start), C.ulong(count))
}

// Close releases the dataset
func (d *Dataset) Close() {
    if d != nil && d.ptr != nil {
        C.randomx_release_dataset(d.ptr)
        d.ptr = nil
    }
}

// NewVM creates a RandomX virtual machine
func NewVM(flags int, cache *Cache, dataset *Dataset) (*VM, error) {
    var cptr *C.randomx_cache
    var dptr *C.randomx_dataset
    if cache != nil {
        cptr = cache.ptr
    }
    if dataset != nil {
        dptr = dataset.ptr
    }
    
    // Use the flags passed in, or default
    cflags := C.get_flags()
    ptr := C.randomx_create_vm(cflags, cptr, dptr)
    if ptr == nil {
        return nil, fmt.Errorf("failed to create RandomX VM")
    }
    return &VM{ptr: ptr}, nil
}

// CalculateHash computes RandomX hash
func (vm *VM) CalculateHash(input []byte, output []byte) {
    if vm == nil || vm.ptr == nil || len(input) == 0 || len(output) < 32 {
        return
    }
    C.randomx_calculate_hash(vm.ptr, unsafe.Pointer(&input[0]), C.size_t(len(input)), unsafe.Pointer(&output[0]))
}

// Close destroys the VM
func (vm *VM) Close() {
    if vm != nil && vm.ptr != nil {
        C.randomx_destroy_vm(vm.ptr)
        vm.ptr = nil
    }
}

// Available returns true if RandomX is available
func Available() bool { return true }
