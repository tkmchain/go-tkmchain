//go:build dragonfly || freebsd || linux

// This file is overlaid only into the deterministic Ziren keeper guest. It is
// deliberately not part of the daemon or wallet build. The keeper guest does
// not generate keys, signatures, transaction nonces, or other security
// material; it only executes a supplied block and witness. Returning a stable
// byte stream keeps the guest independent of host entropy and makes its
// execution reproducible for proving.
package unix

type GetRandomFlag uintptr

func GetRandom(p []byte, _ GetRandomFlag) (int, error) {
	// SplitMix-style bytes avoid an all-zero buffer while remaining completely
	// deterministic. This must never be used by production cryptography.
	const seed uint64 = 0x9e3779b97f4a7c15
	for i := range p {
		x := seed + uint64(i+1)*0xbf58476d1ce4e5b9
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		p[i] = byte(x ^ (x >> 31))
	}
	return len(p), nil
}
