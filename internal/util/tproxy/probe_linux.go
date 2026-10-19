//go:build linux

package tproxy

import "syscall"

// Supported reports whether this process can bind loopback sockets on
// 127.0.0.1 for the nat PREROUTING DNAT relay (TCP SO_ORIGINAL_DST + UDP DNS).
func Supported() bool {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)
	sa := &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}
	return syscall.Bind(fd, sa) == nil
}
