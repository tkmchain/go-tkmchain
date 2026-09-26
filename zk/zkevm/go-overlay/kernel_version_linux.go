//go:build linux

// This file replaces the standard library's uname-based kernel-version probe
// when the keeper is compiled as a deterministic Ziren guest. The guest has
// no host kernel and Ziren exposes only a small, deterministic syscall ABI.
// Returning the modern layout avoids emitting Linux syscall 4122 (uname).
package unix

func KernelVersion() (major, minor int) {
	return 5, 3
}
