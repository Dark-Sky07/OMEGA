package panel

import (
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestAdminRBACPermissionsAndCRUD(t *testing.T) {
	dir := t.TempDir()
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	var userSvc UserService

	// Seed primary superadmin (id=1) via UpdateFirstUser
	if err := userSvc.UpdateFirstUser("rootadmin", "rootpass"); err != nil {
		t.Fatalf("UpdateFirstUser failed: %v", err)
	}

	// Create support admin
	supportAdmin, err := userSvc.CreateAdmin("support1", "pass1234", AdminRoleSupport, nil, "Shift 1 support")
	if err != nil {
		t.Fatalf("CreateAdmin support failed: %v", err)
	}
	if supportAdmin.Password != "" {
		t.Fatal("expected password to be redacted in CreateAdmin response")
	}

	// Verify support role permissions
	if !HasAdminPermission(supportAdmin, "/panel/api/clients/add", "POST") {
		t.Fatal("support should be allowed to mutate clients")
	}
	if !HasAdminPermission(supportAdmin, "/panel/api/inbounds/list", "POST") {
		t.Fatal("support should be allowed to read inbounds list")
	}
	if HasAdminPermission(supportAdmin, "/panel/api/inbounds/add", "POST") {
		t.Fatal("support should NOT be allowed to add inbounds")
	}
	if HasAdminPermission(supportAdmin, "/panel/api/setting/update", "POST") {
		t.Fatal("support should NOT be allowed to update panel settings")
	}

	// Create viewer admin
	viewerAdmin, err := userSvc.CreateAdmin("auditor", "pass1234", AdminRoleViewer, nil, "Read-only auditor")
	if err != nil {
		t.Fatalf("CreateAdmin viewer failed: %v", err)
	}
	if !HasAdminPermission(viewerAdmin, "/panel/api/inbounds/list", "POST") {
		t.Fatal("viewer should be allowed to read inbounds list")
	}
	if !HasAdminPermission(viewerAdmin, "/panel/api/clients/list", "GET") {
		t.Fatal("viewer should be allowed to GET clients list")
	}
	if HasAdminPermission(viewerAdmin, "/panel/api/clients/add", "POST") {
		t.Fatal("viewer should NOT be allowed to create clients")
	}

	// Create custom module-scoped admin
	customAdmin, err := userSvc.CreateAdmin("netops", "pass1234", AdminRoleAdmin, []string{"inbounds", "nodes"}, "Network ops")
	if err != nil {
		t.Fatalf("CreateAdmin custom failed: %v", err)
	}
	if !HasAdminPermission(customAdmin, "/panel/api/inbounds/add", "POST") {
		t.Fatal("custom admin with inbounds scope should be allowed /panel/api/inbounds/add")
	}
	if !HasAdminPermission(customAdmin, "/panel/api/nodes/add", "POST") {
		t.Fatal("custom admin with nodes scope should be allowed /panel/api/nodes/add")
	}
	if HasAdminPermission(customAdmin, "/panel/api/resellers/add", "POST") {
		t.Fatal("custom admin without resellers scope should NOT be allowed /panel/api/resellers/add")
	}
	if HasAdminPermission(customAdmin, "/panel/api/setting/admins/create", "POST") {
		t.Fatal("non-superadmin should never be allowed /panel/api/setting/admins/*")
	}

	// Primary superadmin (id=1) cannot be demoted, disabled, or deleted
	updatedRoot, err := userSvc.UpdateAdmin(1, "rootadmin", "", AdminRoleViewer, nil, "", true)
	if err != nil {
		t.Fatalf("UpdateAdmin(1) failed: %v", err)
	}
	if updatedRoot.Role != AdminRoleSuperAdmin || updatedRoot.Disabled {
		t.Fatalf("primary superadmin was demoted or disabled: %+v", updatedRoot)
	}
	if err := userSvc.DeleteAdmin(1, 2); err == nil {
		t.Fatal("expected deleting primary superadmin (id=1) to fail")
	}

	// Disabling secondary admin blocks login and permissions
	disabledSupport, err := userSvc.UpdateAdmin(supportAdmin.Id, "support1", "", AdminRoleSupport, nil, "Suspended", true)
	if err != nil {
		t.Fatalf("UpdateAdmin support failed: %v", err)
	}
	if HasAdminPermission(disabledSupport, "/panel/api/clients/list", "GET") {
		t.Fatal("disabled admin should have no permissions")
	}
	if _, err := userSvc.CheckUser("support1", "pass1234", ""); err == nil {
		t.Fatal("expected CheckUser to reject disabled admin")
	}

	// Legacy empty Role row is treated as superadmin
	legacy := &model.User{Id: 99, Username: "legacy", Role: ""}
	if !HasAdminPermission(legacy, "/panel/api/setting/admins/create", "POST") {
		t.Fatal("legacy admin with empty Role must be treated as superadmin")
	}
}
