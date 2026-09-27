//go:build linux

package tproxy

import "syscall"

// Supported reports whether this process may open transparent sockets, which
// is what Xray's "tproxy" listener needs (IP_TRANSPARENT requires
// CAP_NET_ADMIN). Xray runs as a child of the panel and inherits its
// privileges, so probing here is a faithful stand-in — and it lets the config
// generator leave the relay out entirely instead of handing Xray an inbound it
// could not start.
func Supported() bool {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)
	return syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1) == nil
}
