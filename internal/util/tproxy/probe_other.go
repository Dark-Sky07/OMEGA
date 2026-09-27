//go:build !linux

package tproxy

// Supported is always false off Linux: the divert path is built on iptables
// TPROXY, which has no equivalent elsewhere.
func Supported() bool { return false }
