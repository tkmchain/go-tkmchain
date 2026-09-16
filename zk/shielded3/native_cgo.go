//go:build shield3 && cgo

package shielded3

/*
#include <stdint.h>
#include <stddef.h>
#cgo !windows,!android LDFLAGS: ${SRCDIR}/stark/target/release/libtkm_shield3_stark.a
#cgo windows,amd64 LDFLAGS: ${SRCDIR}/stark/target/x86_64-pc-windows-gnu/release/libtkm_shield3_stark.a
#cgo android,arm64 LDFLAGS: ${SRCDIR}/stark/target/aarch64-linux-android/release/libtkm_shield3_stark.a
#cgo linux,!android LDFLAGS: -ldl -lpthread -lm
#cgo android LDFLAGS: -ldl -lm -llog
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation -liconv -lm
#cgo windows LDFLAGS: -lws2_32 -luserenv -lbcrypt -lntdll
int tkm_shield3_call(uint32_t, const uint8_t*, size_t, uint8_t*, size_t, size_t*);
*/
import "C"
import "unsafe"

func NativeAvailable() bool { return true }
func nativeCall(operation uint32, input []byte, limit int) ([]byte, error) {
	if len(input) == 0 || limit < 1 || limit > MaxProofSize {
		return nil, ErrInvalidStatement
	}
	output := make([]byte, limit)
	var written C.size_t
	status := C.tkm_shield3_call(C.uint32_t(operation), (*C.uint8_t)(unsafe.Pointer(&input[0])), C.size_t(len(input)), (*C.uint8_t)(unsafe.Pointer(&output[0])), C.size_t(limit), &written)
	if status != 0 || uint64(written) > uint64(limit) {
		clear(output)
		return nil, ErrInvalidProof
	}
	return output[:int(written)], nil
}
