//go:build windows

package openvpn

import (
	"errors"
	"syscall"
)

var errSignalsUnsupported = errors.New("openvpn: signals are not supported on Windows")

// Windows has no signal(2). The stale-daemon reclaim path is disabled on
// Windows anyway (processLooksLikeOpenVPN always returns false there), so
// these are conservative stubs that keep the shared code compiling.
func signalProcess(pid int, sig syscall.Signal) error {
	return errSignalsUnsupported
}

// isProcessAlive cannot cheaply probe a foreign PID without cgo; report
// alive so callers stay on the safe side.
func isProcessAlive(pid int) bool {
	return true
}
