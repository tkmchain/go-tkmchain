package vm

import "testing"

// Simple unit test that calls the precompile function directly.
func TestRKPrecompile(t *testing.T) {
	out, gas, err := rkPrecompile(nil, 100000)
	if err != nil {
		t.Fatalf("rkPrecompile returned error: %v", err)
	}
	if gas == 0 {
		t.Fatalf("expected gas > 0")
	}
	if len(out) == 0 {
		t.Fatalf("expected non-empty output")
	}
}
