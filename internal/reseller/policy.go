// Package reseller defines OMEGA's allocation policy independently of HTTP and
// Xray. Callers must validate and reserve under the same database transaction;
// validation alone is not a concurrency or authorization boundary.
package reseller

import "errors"

var (
	ErrInactive = errors.New("reseller is disabled or expired")
	ErrInvalid = errors.New("invalid reseller allocation")
	ErrQuota = errors.New("reseller allocation limit exceeded")
	ErrInbound = errors.New("inbound is not permitted for this reseller")
)

// Limits contains positive, finite limits. All timestamps are Unix milliseconds
// and all volume values are bytes, matching the upstream client database.
type Limits struct {
	Enabled bool
	ExpiresAt int64
	MaxBytes int64
	MaxClients int64
}

type Usage struct {
	Bytes int64
	Clients int64
}

type Allocation struct {
	Bytes int64
	ExpiresAt int64
	InboundIDs []int
}

// ValidateChange checks a create (previous == nil) or renewal/update. Usage must
// include ALL owned clients, including disabled/depleted clients and pending
// reservations. previous must be loaded by the caller from an ownership-scoped
// database query, never supplied by an untrusted API client.
//
// Count and byte arithmetic uses subtraction to avoid integer overflow. Zero
// and negative quotas/expiry (upstream's unlimited/relative-expiry conventions)
// are intentionally forbidden here.
func ValidateChange(l Limits, used Usage, previous *Allocation, next Allocation, allowed map[int]bool, now int64) error {
	if !l.Enabled || l.ExpiresAt <= now {
		return ErrInactive
	}
	if now < 0 || l.MaxBytes <= 0 || l.MaxClients <= 0 || used.Bytes < 0 || used.Clients < 0 || next.Bytes <= 0 || next.ExpiresAt <= now || next.ExpiresAt > l.ExpiresAt {
		return ErrInvalid
	}
	if len(next.InboundIDs) == 0 {
		return ErrInbound
	}
	seen := make(map[int]bool, len(next.InboundIDs))
	for _, id := range next.InboundIDs {
		if id <= 0 || !allowed[id] || seen[id] {
			return ErrInbound
		}
		seen[id] = true
	}
	baseBytes := used.Bytes
	if previous == nil {
		if used.Clients >= l.MaxClients {
			return ErrQuota
		}
	} else {
		if previous.Bytes <= 0 || previous.Bytes > used.Bytes || used.Clients < 1 {
			return ErrInvalid
		}
		if used.Clients > l.MaxClients {
			return ErrQuota
		}
		baseBytes -= previous.Bytes
	}
	if baseBytes > l.MaxBytes || next.Bytes > l.MaxBytes-baseBytes {
		return ErrQuota
	}
	return nil
}
