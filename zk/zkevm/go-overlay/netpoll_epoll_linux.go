//go:build linux

// Ziren guests do not have a host file descriptor table or an epoll kernel.
// The keeper is a pure stateless execution guest and never opens a network
// connection, so Go's Linux epoll backend must be deterministic and inert.
package runtime

var epfd int32 = -1

func netpollinit() {}

func netpollIsPollDescriptor(fd uintptr) bool {
	return false
}

func netpollopen(fd uintptr, pd *pollDesc) uintptr {
	return 0
}

func netpollclose(fd uintptr) uintptr {
	return 0
}

func netpollarm(pd *pollDesc, mode int) {
	throw("runtime: unused")
}

func netpollBreak() {}

func netpoll(delay int64) (gList, int32) {
	return gList{}, 0
}
