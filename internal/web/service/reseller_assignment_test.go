package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestResellerInboundBatchAssignmentSupportsSharedInbounds(t *testing.T) {
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
	if err := svc.SetInbounds(second.Id, []int{inbounds[1].Id, inbounds[2].Id}); err != nil {
		t.Fatalf("shared inbound assignment should succeed: %v", err)
	}
	ownedSecond, err := svc.OwnedInboundIds(second.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownedSecond) != 2 || ownedSecond[0] != inbounds[1].Id || ownedSecond[1] != inbounds[2].Id {
		t.Fatalf("unexpected second reseller inbound ownership: %v", ownedSecond)
	}
	// Missing inbound id must still fail atomically without modifying mappings.
	if err := svc.SetInbounds(second.Id, []int{inbounds[0].Id, 999999}); err == nil {
		t.Fatal("missing inbound id was accepted")
	}
	if err := svc.SetInbounds(first.Id, nil); err != nil {
		t.Fatalf("clear first reseller assignment: %v", err)
	}
	ownedFirst, err := svc.OwnedInboundIds(first.Id)
	if err != nil || len(ownedFirst) != 0 {
		t.Fatalf("empty desired set did not clear mappings: %v, %v", ownedFirst, err)
	}
	// Clearing first reseller must leave second reseller's shared inbound intact.
	ownedSecond, err = svc.OwnedInboundIds(second.Id)
	if err != nil || len(ownedSecond) != 2 {
		t.Fatalf("clearing first reseller affected second reseller: %v, %v", ownedSecond, err)
	}
}

func TestResellerClientBatchAssignmentIsAtomicAndExclusive(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "client-batch", Password: "hash", Enable: true}
	other := &model.Reseller{Username: "client-other", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(other).Error; err != nil {
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
	if err := svc.AssignClients(other.Id, []string{"batch-a@example.com"}); err == nil {
		t.Fatal("client already assigned to another reseller was accepted")
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

func TestSharedInboundClientIsolationAcrossResellers(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	resellerA := &model.Reseller{Username: "reseller-a", Password: "hash", Enable: true}
	resellerB := &model.Reseller{Username: "reseller-b", Password: "hash", Enable: true}
	if err := db.Create(resellerA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(resellerB).Error; err != nil {
		t.Fatal(err)
	}
	sharedInbound := &model.Inbound{
		UserId:   1,
		Tag:      "shared-vless",
		Port:     22001,
		Protocol: model.VLESS,
		Up:       300,
		Down:     700,
		Settings: `{"clients":[{"id":"uuid-a","email":"alice@a.com"},{"id":"uuid-b","email":"bob@b.com"}]}`,
	}
	if err := db.Create(sharedInbound).Error; err != nil {
		t.Fatal(err)
	}
	for _, rec := range []*model.ClientRecord{
		{Email: "alice@a.com", UUID: "uuid-a", Enable: true},
		{Email: "bob@b.com", UUID: "uuid-b", Enable: true},
	} {
		if err := db.Create(rec).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: sharedInbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, stat := range []*xray.ClientTraffic{
		{InboundId: sharedInbound.Id, Email: "alice@a.com", Up: 100, Down: 200, Enable: true},
		{InboundId: sharedInbound.Id, Email: "bob@b.com", Up: 200, Down: 500, Enable: true},
	} {
		if err := db.Create(stat).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := &ResellerService{}
	if err := svc.AssignInbound(resellerA.Id, sharedInbound.Id); err != nil {
		t.Fatalf("assign shared inbound to A: %v", err)
	}
	if err := svc.AssignInbound(resellerB.Id, sharedInbound.Id); err != nil {
		t.Fatalf("assign shared inbound to B: %v", err)
	}
	if err := svc.AssignClient(resellerA.Id, "alice@a.com"); err != nil {
		t.Fatalf("assign alice to A: %v", err)
	}
	if err := svc.AssignClient(resellerB.Id, "bob@b.com"); err != nil {
		t.Fatalf("assign bob to B: %v", err)
	}

	// Reseller A owns alice but NOT bob, and vice versa.
	if ok, err := svc.OwnsClient(resellerA.Id, "alice@a.com"); err != nil || !ok {
		t.Fatalf("A should own alice: ok=%v err=%v", ok, err)
	}
	if ok, err := svc.OwnsClient(resellerA.Id, "bob@b.com"); err != nil || ok {
		t.Fatalf("A must NOT own bob: ok=%v err=%v", ok, err)
	}
	if ok, err := svc.OwnsClient(resellerB.Id, "bob@b.com"); err != nil || !ok {
		t.Fatalf("B should own bob: ok=%v err=%v", ok, err)
	}
	if ok, err := svc.OwnsClient(resellerB.Id, "alice@a.com"); err != nil || ok {
		t.Fatalf("B must NOT own alice: ok=%v err=%v", ok, err)
	}

	// InboundService.GetInboundsForReseller must strictly isolate clients and traffic per reseller.
	inboundSvc := &InboundService{}
	inboundsA, err := inboundSvc.GetInboundsForReseller(resellerA.Id)
	if err != nil || len(inboundsA) != 1 {
		t.Fatalf("GetInboundsForReseller(A): len=%d err=%v", len(inboundsA), err)
	}
	if len(inboundsA[0].ClientStats) != 1 || inboundsA[0].ClientStats[0].Email != "alice@a.com" {
		t.Fatalf("A saw wrong ClientStats: %+v", inboundsA[0].ClientStats)
	}
	if inboundsA[0].Up != 100 || inboundsA[0].Down != 200 {
		t.Fatalf("A saw leaked inbound traffic totals: up=%d down=%d", inboundsA[0].Up, inboundsA[0].Down)
	}

	inboundsB, err := inboundSvc.GetInboundsForReseller(resellerB.Id)
	if err != nil || len(inboundsB) != 1 {
		t.Fatalf("GetInboundsForReseller(B): len=%d err=%v", len(inboundsB), err)
	}
	if len(inboundsB[0].ClientStats) != 1 || inboundsB[0].ClientStats[0].Email != "bob@b.com" {
		t.Fatalf("B saw wrong ClientStats: %+v", inboundsB[0].ClientStats)
	}
	if inboundsB[0].Up != 200 || inboundsB[0].Down != 500 {
		t.Fatalf("B saw leaked inbound traffic totals: up=%d down=%d", inboundsB[0].Up, inboundsB[0].Down)
	}
}
