//go:build linux

// The deterministic keeper guest has no kernel hostname. Avoid the standard
// library's uname probe (Linux/MIPS syscall 4122) and return a fixed value.
package os

func hostname() (string, error) {
	return "localhost", nil
}
