//go:build shield3 && wasm

package shielded3

import (
	"fmt"
	"syscall/js"
)

// The browser proof worker supplies this function after loading the pinned
// Rust WASM module. The call is synchronous inside the worker, so the spending
// witness never leaves the browser or enters a network request.
func NativeAvailable() bool {
	return js.Global().Get("tkmShield3NativeCall").Type() == js.TypeFunction
}

func nativeCall(operation uint32, input []byte, limit int) ([]byte, error) {
	if len(input) == 0 || limit < 1 || limit > MaxProofSize || !NativeAvailable() {
		return nil, ErrBackendUnavailable
	}
	request := js.Global().Get("Uint8Array").New(len(input))
	js.CopyBytesToJS(request, input)
	result := js.Global().Call("tkmShield3NativeCall", operation, request)
	if result.Type() != js.TypeObject || !result.InstanceOf(js.Global().Get("Uint8Array")) {
		return nil, fmt.Errorf("%w: browser proof worker returned a non-byte result", ErrInvalidProof)
	}
	if result.Length() > limit {
		return nil, errOutputLimit
	}
	out := make([]byte, result.Length())
	js.CopyBytesToGo(out, result)
	return out, nil
}
