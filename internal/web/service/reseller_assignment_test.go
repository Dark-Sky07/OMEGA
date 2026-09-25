package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestResellerInboundBatchAssignmentIsAtomicAndExclusive(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	first := &model.Reseller{Username: "batch-first", Password: "hash", Enable: true}
	second := &model.Reseller{Username: "batch-second", Password: "hash", Enable: true}
	if err := db.Create(first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(second).Error; err != nil {
		t.Fatal(err)
	}
	for _, inbound := range []*model.Inbound{
		{UserId: 1, Tag: "batch-1", Port: 21001, Protocol: model.VLESS, Settings: `{"clients":[]}`},
		{UserId: 1, Tag: "batch-2", Port: 21002, Protocol: model.VLESS, Settings: `{"clients":[]}`},
		{UserId: 1, Tag: "batch-3", Port: 21003, Protocol: model.VLESS, Settings: `{"clients":[]}`},
	} {
		if err := db.Create(inbound).Error; err != nil {
			t.Fatal(err)
		}
	}
	var inbounds []model.Inbound
	if err := db.Order("id ASC").Find(&inbounds).Error; err != nil {
		t.Fatal(err)
	}

	svc := &ResellerService{}
	if err := svc.SetInbounds(first.Id, []int{inbounds[0].Id, inbounds[1].Id}); err != nil {
		t.Fatalf("initial SetInbounds: %v", err)
	}
	if err := svc.SetInbounds(second.Id, []int{inbounds[1].Id, inbounds[2].Id}); err == nil {
		t.Fatal("ownership conflict was accepted")
	}
	owned, err := svc.OwnedInboundIds(second.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("failed conflicting batch partially changed ownership: %v", owned)
	}
	if err := svc.SetInbounds(second.Id, []int{inbounds[2].Id}); err != nil {
		t.Fatalf("assign unowned inbound: %v", err)
	}
	if err := svc.SetInbounds(first.Id, nil); err != nil {
		t.Fatalf("clear first reseller assignment: %v", err)
	}
	owned, err = svc.OwnedInboundIds(first.Id)
	if err != nil || len(owned) != 0 {
		t.Fatalf("empty desired set did not clear mappings: %v, %v", owned, err)
	}
}

func TestResellerClientBatchAssignmentIsAtomic(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "client-batch", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientRecord{Email: "batch-a@example.com", UUID: "batch-a"}).Error; err != nil {
		t.Fatal(err)
	}

	svc := &ResellerService{}
	if err := svc.AssignClients(reseller.Id, []string{"batch-a@example.com", "missing@example.com"}); err == nil {
		t.Fatal("missing client was accepted")
	}
	var count int64
	if err := db.Model(&model.ResellerClient{}).Where("reseller_id = ?", reseller.Id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed client batch partially created %d mapping(s)", count)
	}
	if err := svc.AssignClients(reseller.Id, []string{"batch-a@example.com"}); err != nil {
		t.Fatalf("valid client batch: %v", err)
	}
	if err := svc.UnassignClients(reseller.Id, []string{"batch-a@example.com"}); err != nil {
		t.Fatalf("unassign client batch: %v", err)
	}
	if err := db.Model(&model.ResellerClient{}).Where("reseller_id = ?", reseller.Id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unassign client batch left %d mapping(s)", count)
	}
}
