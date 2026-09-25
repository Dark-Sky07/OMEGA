package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestResolveTransferIdentityRequiresOnePortableMatch(t *testing.T) {
	inbounds := []model.Inbound{
		{Id: 7, Tag: "vless-main", Remark: "Main", Protocol: model.VLESS, Port: 443, Listen: "0.0.0.0"},
		{Id: 8, Tag: "vless-other", Remark: "Other", Protocol: model.VLESS, Port: 8443, Listen: "0.0.0.0"},
	}

	id, err := resolveTransferIdentity(ClientTransferInboundRef{Tag: "vless-main", Protocol: model.VLESS, Port: 443}, inbounds)
	if err != nil || id != 7 {
		t.Fatalf("portable identity resolved to (%d, %v), want (7, nil)", id, err)
	}
	id, err = resolveTransferIdentity(ClientTransferInboundRef{Tag: "missing", Protocol: model.VLESS, Port: 443}, inbounds)
	if err != nil || id != 0 {
		t.Fatalf("missing identity resolved to (%d, %v), want (0, nil)", id, err)
	}

	ambiguous := []model.Inbound{
		{Id: 9, Protocol: model.VLESS, Port: 443},
		{Id: 10, Protocol: model.VLESS, Port: 443},
	}
	if _, err := resolveTransferIdentity(ClientTransferInboundRef{Protocol: model.VLESS, Port: 443}, ambiguous); err == nil {
		t.Fatal("ambiguous portable identity was accepted")
	}
}

func TestResolveTransferFallbackUsesPortableMasterAndPath(t *testing.T) {
	inbounds := []model.Inbound{
		{Id: 21, Tag: "master", Remark: "Master", Protocol: model.VLESS, Port: 443, Listen: "0.0.0.0"},
		{Id: 22, Tag: "child", Remark: "Child", Protocol: model.VLESS, Port: 443, Listen: "0.0.0.0"},
	}
	fallbacks := []model.InboundFallback{{MasterId: 21, ChildId: 22, Path: "/portable"}}
	ref := ClientTransferInboundRef{FallbackParent: &ClientTransferFallbackRef{
		MasterTag: "master", MasterProtocol: model.VLESS, MasterPort: 443, MasterListen: "0.0.0.0", Path: "/portable",
	}}
	id, err := resolveTransferFallbackParent(ref, inbounds, fallbacks)
	if err != nil || id != 22 {
		t.Fatalf("fallback resolved to (%d, %v), want (22, nil)", id, err)
	}

	ref.FallbackParent.Path = "/not-on-master"
	if _, err := resolveTransferFallbackParent(ref, inbounds, fallbacks); err == nil {
		t.Fatal("missing fallback path was accepted")
	}
}

func TestFilterTransferInboundIDsKeepsOnlyResellerOwnedTargets(t *testing.T) {
	scope := &ClientTransferScope{Reseller: true, InboundIDs: map[int]struct{}{2: {}}}
	got := filterTransferInboundIDs([]int{1, 2, 3}, scope)
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("filtered IDs = %v, want [2]", got)
	}
}
