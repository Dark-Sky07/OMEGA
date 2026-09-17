//go:build !windows

package openvpn

import "syscall"

// signalProcess sends a signal to an arbitrary PID. It is the low-level
// primitive the stale-daemon reclaim path (killStaleDaemon) uses.
func signalProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// isProcessAlive probes whether a PID still exists.
func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
