package reseller

import (
	"errors"
	"math"
	"testing"
)

func TestValidateChange(t *testing.T) {
	limits := Limits{Enabled: true, ExpiresAt: 2000, MaxBytes: 100, MaxClients: 2}
	valid := Allocation{Bytes: 60, ExpiresAt: 2000, InboundIDs: []int{1}}
	allowed := map[int]bool{1: true, 2: true}
	cases := []struct {
		name string
		limits Limits
		used Usage
		previous *Allocation
		next Allocation
		want error
	}{
		{"create exact boundary", limits, Usage{Bytes: 40, Clients: 1}, nil, valid, nil},
		{"byte cap", limits, Usage{Bytes: 41, Clients: 1}, nil, valid, ErrQuota},
		{"client cap", limits, Usage{Bytes: 40, Clients: 2}, nil, valid, ErrQuota},
		{"renew at client cap", limits, Usage{Bytes: 100, Clients: 2}, &valid, valid, nil},
		{"disabled", Limits{ExpiresAt: 2000, MaxBytes: 100, MaxClients: 2}, Usage{}, nil, valid, ErrInactive},
		{"expired at boundary", Limits{Enabled: true, ExpiresAt: 1000, MaxBytes: 100, MaxClients: 2}, Usage{}, nil, valid, ErrInactive},
		{"unlimited volume", limits, Usage{}, nil, Allocation{ExpiresAt: 2000, InboundIDs: []int{1}}, ErrInvalid},
		{"negative volume", limits, Usage{}, nil, Allocation{Bytes: -1, ExpiresAt: 2000, InboundIDs: []int{1}}, ErrInvalid},
		{"unlimited expiry", limits, Usage{}, nil, Allocation{Bytes: 1, InboundIDs: []int{1}}, ErrInvalid},
		{"relative expiry", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: -100, InboundIDs: []int{1}}, ErrInvalid},
		{"client already expired", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: 1000, InboundIDs: []int{1}}, ErrInvalid},
		{"beyond reseller expiry", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: 2001, InboundIDs: []int{1}}, ErrInvalid},
		{"forbidden inbound", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: 2000, InboundIDs: []int{3}}, ErrInbound},
		{"mixed permission", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: 2000, InboundIDs: []int{1, 3}}, ErrInbound},
		{"duplicate inbound", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: 2000, InboundIDs: []int{1, 1}}, ErrInbound},
		{"no inbound", limits, Usage{}, nil, Allocation{Bytes: 1, ExpiresAt: 2000}, ErrInbound},
		{"corrupt usage", limits, Usage{Bytes: -1}, nil, valid, ErrInvalid},
		{"missing previous allocation", limits, Usage{Bytes: 40, Clients: 1}, &valid, valid, ErrInvalid},
		{"overflow bytes", Limits{Enabled: true, ExpiresAt: 2000, MaxBytes: math.MaxInt64, MaxClients: 2}, Usage{Bytes: math.MaxInt64, Clients: 1}, nil, valid, ErrQuota},
		{"overflow count", Limits{Enabled: true, ExpiresAt: 2000, MaxBytes: 100, MaxClients: math.MaxInt64}, Usage{Clients: math.MaxInt64}, nil, valid, ErrQuota},
		{"shrink overallocated volume", limits, Usage{Bytes: 110, Clients: 2}, &Allocation{Bytes: 70}, valid, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateChange(tc.limits, tc.used, tc.previous, tc.next, allowed, 1000); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNoPermissionsFailsClosed(t *testing.T) {
	err := ValidateChange(Limits{true, 2000, 100, 2}, Usage{}, nil, Allocation{1, 2000, []int{1}}, nil, 1000)
	if !errors.Is(err, ErrInbound) { t.Fatalf("got %v", err) }
}
