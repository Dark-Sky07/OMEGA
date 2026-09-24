package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// lookupClientRecord is shared by the per-inbound tunnel regression tests.
func lookupClientRecord(t *testing.T, email string) *model.ClientRecord {
	t.Helper()
	row := &model.ClientRecord{}
	if err := database.GetDB().Where("email = ?", email).First(row).Error; err != nil {
		t.Fatalf("lookup client %q: %v", email, err)
	}
	return row
}

// wgServerSettings is the smallest valid plain-WireGuard settings document
// used by the client CRUD tests. The production allocator treats the subnet
// fields as optional and falls back to 10.0.0.0/24, but spelling them out here
// keeps cross-inbound collision tests deterministic.
func wgServerSettings() string {
	return `{"subnetIp":"10.0.0.0","subnetCidr":24,"clients":[]}`
}
