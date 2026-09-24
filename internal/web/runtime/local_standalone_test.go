package runtime

import (
	"context"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestLocalOpenVPNRuntimeNeverFallsThroughToXray(t *testing.T) {
	apiCalled := false
	local := NewLocal(LocalDeps{
		APIPort: func() int {
			apiCalled = true
			return 1
		},
	})
	oldInbound := &model.Inbound{Id: 41, Tag: "openvpn-old", Protocol: model.OpenVPN, Enable: true}
	newInbound := &model.Inbound{Id: 41, Tag: "openvpn-new", Protocol: model.OpenVPN, Enable: true}
	client := model.Client{Email: "alice@example.test", ID: "client-id", Enable: true}
	ctx := context.Background()

	if err := local.AddInbound(ctx, oldInbound); err != nil {
		t.Fatalf("AddInbound: %v", err)
	}
	if err := local.UpdateInbound(ctx, oldInbound, newInbound); err != nil {
		t.Fatalf("UpdateInbound: %v", err)
	}
	if err := local.AddUser(ctx, newInbound, map[string]any{"email": client.Email}); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if err := local.RemoveUser(ctx, newInbound, client.Email); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
	if err := local.AddClient(ctx, newInbound, client); err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	if err := local.DelInbound(ctx, newInbound); err != nil {
		t.Fatalf("DelInbound: %v", err)
	}
	if apiCalled {
		t.Fatal("OpenVPN runtime operation unexpectedly initialized the Xray API")
	}
}
