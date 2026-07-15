//go:build cgo && randomx
// +build cgo,randomx

package randomx

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/_workspace/randomx/src
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-host -lrandomx -lstdc++ -lm
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-linux-arm64 -lrandomx -lstdc++ -lm
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-windows-amd64 -lrandomx -lstdc++ -lwinpthread
#cgo darwin LDFLAGS: -L${SRCDIR}/../../build/_workspace/randomx/build-darwin -lrandomx -lc++ -lm -framework CoreFoundation -framework Security

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
