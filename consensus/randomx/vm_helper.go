package randomx

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/_workspace/randomx/src
#cgo LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build -lrandomx -lstdc++ -lm

#include <stdlib.h>
#include "randomx.h"
*/
import "C"

// NewVMFromCache creates a new RandomX VM from a cache (for external miners)
func NewVMFromCache(cache *Cache) *VM {
        if cache == nil || cache.ptr == nil {
                return nil
        }
        flags := RANDOMX_FLAG_JIT | RANDOMX_FLAG_HARD_AES
        vm := C.randomx_create_vm(C.randomx_flags(flags), cache.ptr, nil)
        if vm == nil {
                return nil
        }
        return &VM{ptr: vm}
}
