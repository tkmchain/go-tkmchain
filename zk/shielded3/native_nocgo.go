//go:build !shield3 || !cgo

package shielded3

func NativeAvailable() bool                          { return false }
func nativeCall(uint32, []byte, int) ([]byte, error) { return nil, ErrBackendUnavailable }
