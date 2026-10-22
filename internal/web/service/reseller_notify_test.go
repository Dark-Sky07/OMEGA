package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func notifyRowCount(t *testing.T, resellerId int) int64 {
	t.Helper()
	var count int64
	if err := database.GetDB().Model(model.ResellerNotification{}).
		Where("reseller_id = ?", resellerId).
		Count(&count).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	return count
}

func TestResellerNotifyEvaluateQuotaEpisodes(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "notify-quota", Password: "hash", Enable: true, TrafficLimit: 1000}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerNotifyService{}

	eval := func(allocated, used int64) []*model.ResellerNotification {
		return svc.Evaluate(&ResellerStat{Reseller: reseller, AllocatedTraffic: allocated, UsedTraffic: used})
	}

	// Far below the warning threshold: silent.
	if fresh := eval(500, 0); len(fresh) != 0 {
		t.Fatalf("below threshold fresh = %d, want 0", len(fresh))
	}
	if got := notifyRowCount(t, reseller.Id); got != 0 {
		t.Fatalf("below threshold rows = %d, want 0", got)
	}

	// Crossing 80% emits exactly one quota80 notification.
	fresh := eval(850, 0)
	if len(fresh) != 1 || fresh[0].Kind != ResellerNotifQuota80 {
		t.Fatalf("fresh rows = %+v, want one %q", fresh, ResellerNotifQuota80)
	}
	if got := notifyRowCount(t, reseller.Id); got != 1 {
		t.Fatalf("after 85%% rows = %d, want 1", got)
	}
	var first model.ResellerNotification
	if err := db.Where("reseller_id = ?", reseller.Id).First(&first).Error; err != nil {
		t.Fatalf("load notification: %v", err)
	}
	if first.Kind != ResellerNotifQuota80 {
		t.Fatalf("kind = %q, want %q", first.Kind, ResellerNotifQuota80)
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(first.Params), &params); err != nil {
		t.Fatalf("params not valid json: %v", err)
	}
	if params["pct"] != float64(85) {
		t.Fatalf("params[pct] = %v, want 85", params["pct"])
	}

	// Staying at 85% must not re-notify within the same episode.
	eval(860, 0)
	if got := notifyRowCount(t, reseller.Id); got != 1 {
		t.Fatalf("repeat 85%% rows = %d, want 1", got)
	}

	// Escalating past 95% adds the critical notification.
	eval(960, 0)
	if got := notifyRowCount(t, reseller.Id); got != 2 {
		t.Fatalf("after 96%% rows = %d, want 2", got)
	}

	// Exhausting the quota adds the exhaustion notification.
	eval(1200, 0)
	if got := notifyRowCount(t, reseller.Id); got != 3 {
		t.Fatalf("after over-quota rows = %d, want 3", got)
	}

	// Consumed traffic alone can exhaust the quota too.
	other := &model.Reseller{Username: "notify-quota-used", Password: "hash", Enable: true, TrafficLimit: 1000}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create second reseller: %v", err)
	}
	svc.Evaluate(&ResellerStat{Reseller: other, AllocatedTraffic: 100, UsedTraffic: 1000})
	var kinds []string
	db.Model(model.ResellerNotification{}).Where("reseller_id = ?", other.Id).Pluck("kind", &kinds)
	if len(kinds) != 1 || kinds[0] != ResellerNotifQuotaOut {
		t.Fatalf("used-traffic kinds = %v, want [%q]", kinds, ResellerNotifQuotaOut)
	}

	// Dropping back below the thresholds closes the episode...
	eval(500, 0)
	if got := notifyRowCount(t, reseller.Id); got != 3 {
		t.Fatalf("rows must persist after recovery, got %d", got)
	}
	// ...so approaching 80% again notifies again.
	eval(850, 0)
	if got := notifyRowCount(t, reseller.Id); got != 4 {
		t.Fatalf("new episode rows = %d, want 4", got)
	}
}

func TestResellerNotifyEvaluateClientLimitAndExpiry(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "notify-mixed", Password: "hash", Enable: true, ClientLimit: 10}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerNotifyService{}

	// Client count below thresholds: silent.
	svc.Evaluate(&ResellerStat{Reseller: reseller, ClientCount: 5})
	if got := notifyRowCount(t, reseller.Id); got != 0 {
		t.Fatalf("client rows = %d, want 0", got)
	}

	// At the limit the reached notification fires once.
	svc.Evaluate(&ResellerStat{Reseller: reseller, ClientCount: 10})
	if got := notifyRowCount(t, reseller.Id); got != 1 {
		t.Fatalf("client rows = %d, want 1", got)
	}
	var kinds []string
	db.Model(model.ResellerNotification{}).Where("reseller_id = ?", reseller.Id).Pluck("kind", &kinds)
	if kinds[0] != ResellerNotifClientMax {
		t.Fatalf("kind = %v, want %q", kinds, ResellerNotifClientMax)
	}

	// A fresh reseller approaching the limit warns at 80%.
	warned := &model.Reseller{Username: "notify-climit", Password: "hash", Enable: true, ClientLimit: 10}
	if err := db.Create(warned).Error; err != nil {
		t.Fatalf("create warned reseller: %v", err)
	}
	svc.Evaluate(&ResellerStat{Reseller: warned, ClientCount: 8})
	if got := notifyRowCount(t, warned.Id); got != 1 {
		t.Fatalf("climit80 rows = %d, want 1", got)
	}

	// Expiry: soon, then expired, then recovered (admin extends the date).
	day := int64(24 * 60 * 60 * 1000)
	expiring := &model.Reseller{Username: "notify-expiry", Password: "hash", Enable: true, ExpiryTime: time.Now().UnixMilli() + 5*day + 10*60*1000}
	if err := db.Create(expiring).Error; err != nil {
		t.Fatalf("create expiring reseller: %v", err)
	}
	svc.Evaluate(&ResellerStat{Reseller: expiring})
	var expiryKinds []string
	db.Model(model.ResellerNotification{}).Where("reseller_id = ?", expiring.Id).Pluck("kind", &expiryKinds)
	if len(expiryKinds) != 1 || expiryKinds[0] != ResellerNotifExpirySoon {
		t.Fatalf("expiry kinds = %v, want [%q]", expiryKinds, ResellerNotifExpirySoon)
	}

	past := &model.Reseller{Username: "notify-expired", Password: "hash", Enable: true, ExpiryTime: time.Now().UnixMilli() - 1000}
	if err := db.Create(past).Error; err != nil {
		t.Fatalf("create expired reseller: %v", err)
	}
	svc.Evaluate(&ResellerStat{Reseller: past})
	db.Model(model.ResellerNotification{}).Where("reseller_id = ?", past.Id).Pluck("kind", &expiryKinds)
	if len(expiryKinds) != 1 || expiryKinds[0] != ResellerNotifExpired {
		t.Fatalf("expired kinds = %v, want [%q]", expiryKinds, ResellerNotifExpired)
	}

	// Unlimited resellers never produce quota notifications.
	unlimited := &model.Reseller{Username: "notify-unlimited", Password: "hash", Enable: true}
	if err := db.Create(unlimited).Error; err != nil {
		t.Fatalf("create unlimited reseller: %v", err)
	}
	svc.Evaluate(&ResellerStat{Reseller: unlimited, ClientCount: 1000, AllocatedTraffic: 1 << 40, UsedTraffic: 1 << 40})
	if got := notifyRowCount(t, unlimited.Id); got != 0 {
		t.Fatalf("unlimited rows = %d, want 0", got)
	}
}

func TestResellerNotifyListAndMarkRead(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "notify-list", Password: "hash", Enable: true}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerNotifyService{}

	other := &model.Reseller{Username: "notify-list-other", Password: "hash", Enable: true}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other reseller: %v", err)
	}
	if _, err := svc.Notify(other.Id, ResellerNotifQuota80, map[string]any{"pct": 80}); err != nil {
		t.Fatalf("notify other: %v", err)
	}

	for _, kind := range []string{ResellerNotifQuota80, ResellerNotifQuota95, ResellerNotifExpired} {
		if _, err := svc.Notify(reseller.Id, kind, nil); err != nil {
			t.Fatalf("notify %s: %v", kind, err)
		}
	}

	rows, unread, err := svc.List(reseller.Id, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 || unread != 3 {
		t.Fatalf("List = %d rows, %d unread; want 3, 3", len(rows), unread)
	}
	if rows[0].Kind != ResellerNotifExpired {
		t.Fatalf("newest first = %q, want %q", rows[0].Kind, ResellerNotifExpired)
	}

	// Marking one row read leaves the other two unread.
	if err := svc.MarkRead(reseller.Id, rows[0].Id); err != nil {
		t.Fatalf("MarkRead one: %v", err)
	}
	if _, unread, err = svc.List(reseller.Id, 0); err != nil || unread != 2 {
		t.Fatalf("after one read unread = %d (err %v), want 2", unread, err)
	}

	// Marking all read also must not touch the other reseller's rows.
	if err := svc.MarkRead(reseller.Id, 0); err != nil {
		t.Fatalf("MarkRead all: %v", err)
	}
	if _, unread, err = svc.List(reseller.Id, 0); err != nil || unread != 0 {
		t.Fatalf("after mark-all unread = %d (err %v), want 0", unread, err)
	}
	if _, otherUnread, err := svc.List(other.Id, 0); err != nil || otherUnread != 1 {
		t.Fatalf("other reseller unread = %d (err %v), want 1", otherUnread, err)
	}

	// Read rows older than the retention window are purged.
	old := model.ResellerNotification{ResellerId: reseller.Id, Kind: ResellerNotifQuota80, IsRead: true, CreatedAt: time.Now().UnixMilli() - 40*24*60*60*1000}
	if err := db.Create(&old).Error; err != nil {
		t.Fatalf("create old notification: %v", err)
	}
	if err := svc.PurgeRead(30); err != nil {
		t.Fatalf("PurgeRead: %v", err)
	}
	rows, _, err = svc.List(reseller.Id, 0)
	if err != nil {
		t.Fatalf("List after purge: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows after purge = %d, want 3 (old row removed)", len(rows))
	}

	// Absurd limits are clamped server-side and never error.
	if _, _, err := svc.List(reseller.Id, 1<<30); err != nil {
		t.Fatalf("List with huge limit: %v", err)
	}
}

func TestResellerNotifyUsesConfiguredThresholdsAndChannels(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	reseller := &model.Reseller{Username: "notify-config", Password: "hash", Enable: true, TrafficLimit: 1000, ClientLimit: 10}
	if err := db.Create(reseller).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	svc := &ResellerNotifyService{}
	config := ResellerNotifyConfig{
		TrafficWarningPercent:  70,
		TrafficCriticalPercent: 90,
		ClientWarningPercent:   75,
		ExpiryWarningDays:      10,
		InPanelEnable:          false,
		TelegramEnable:         true,
	}

	// With both delivery channels disabled, the episode is not claimed; enabling
	// Telegram while the warning condition remains active can still deliver it.
	disabled := config
	disabled.TelegramEnable = false
	if fresh := svc.EvaluateWithConfig(&ResellerStat{Reseller: reseller, AllocatedTraffic: 750}, disabled); len(fresh) != 0 {
		t.Fatalf("disabled channels emitted %d notifications, want 0", len(fresh))
	}

	fresh := svc.EvaluateWithConfig(&ResellerStat{Reseller: reseller, AllocatedTraffic: 750}, config)
	if len(fresh) != 1 || fresh[0].Kind != ResellerNotifQuota80 {
		t.Fatalf("custom traffic warning rows = %+v, want one %q", fresh, ResellerNotifQuota80)
	}
	if got := notifyRowCount(t, reseller.Id); got != 0 {
		t.Fatalf("Telegram-only notification persisted %d in-panel rows, want 0", got)
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(fresh[0].Params), &params); err != nil || params["pct"] != float64(75) {
		t.Fatalf("transient Telegram parameters = %v (err %v), want pct=75", params, err)
	}

	// Thresholds are honored independently for the critical quota and client limit.
	critical := &model.Reseller{Username: "notify-critical-config", Password: "hash", Enable: true, TrafficLimit: 1000}
	if err := db.Create(critical).Error; err != nil {
		t.Fatalf("create critical reseller: %v", err)
	}
	fresh = svc.EvaluateWithConfig(&ResellerStat{Reseller: critical, AllocatedTraffic: 920}, config)
	if len(fresh) != 1 || fresh[0].Kind != ResellerNotifQuota95 {
		t.Fatalf("custom critical rows = %+v, want one %q", fresh, ResellerNotifQuota95)
	}

	clients := &model.Reseller{Username: "notify-client-config", Password: "hash", Enable: true, ClientLimit: 10}
	if err := db.Create(clients).Error; err != nil {
		t.Fatalf("create client-limit reseller: %v", err)
	}
	fresh = svc.EvaluateWithConfig(&ResellerStat{Reseller: clients, ClientCount: 8}, config)
	if len(fresh) != 1 || fresh[0].Kind != ResellerNotifClient80 {
		t.Fatalf("custom client threshold rows = %+v, want one %q", fresh, ResellerNotifClient80)
	}
	if got := notifyRowCount(t, clients.Id); got != 0 {
		t.Fatalf("client Telegram-only notification persisted %d rows, want 0", got)
	}

	expiring := &model.Reseller{Username: "notify-expiry-config", Password: "hash", Enable: true, ExpiryTime: time.Now().Add(9 * 24 * time.Hour).UnixMilli()}
	if err := db.Create(expiring).Error; err != nil {
		t.Fatalf("create expiring reseller: %v", err)
	}
	fresh = svc.EvaluateWithConfig(&ResellerStat{Reseller: expiring}, config)
	if len(fresh) != 1 || fresh[0].Kind != ResellerNotifExpirySoon {
		t.Fatalf("custom expiry threshold rows = %+v, want one %q", fresh, ResellerNotifExpirySoon)
	}
	if got := notifyRowCount(t, expiring.Id); got != 0 {
		t.Fatalf("expiry Telegram-only notification persisted %d rows, want 0", got)
	}

	// In-panel-only mode still stores a fresh alert even if Telegram is off.
	panelOnly := config
	panelOnly.InPanelEnable = true
	panelOnly.TelegramEnable = false
	panelReseller := &model.Reseller{Username: "notify-panel-only", Password: "hash", Enable: true, TrafficLimit: 1000}
	if err := db.Create(panelReseller).Error; err != nil {
		t.Fatalf("create panel-only reseller: %v", err)
	}
	fresh = svc.EvaluateWithConfig(&ResellerStat{Reseller: panelReseller, AllocatedTraffic: 750}, panelOnly)
	if len(fresh) != 1 || notifyRowCount(t, panelReseller.Id) != 1 {
		t.Fatalf("in-panel-only notification = %+v, stored rows = %d", fresh, notifyRowCount(t, panelReseller.Id))
	}
}

func TestResellerNotifyConfigDefaultsPreserveCurrentBehavior(t *testing.T) {
	setupConflictDB(t)
	svc := &ResellerNotifyService{}
	config := svc.Config()
	if config.TrafficWarningPercent != 80 || config.TrafficCriticalPercent != 95 ||
		config.ClientWarningPercent != 80 || config.ExpiryWarningDays != 7 ||
		!config.InPanelEnable || !config.TelegramEnable {
		t.Fatalf("default config = %+v, want legacy thresholds and both channels enabled", config)
	}

	settings := []model.Setting{
		{Key: "resellerNotifTrafficWarningPercent", Value: "72"},
		{Key: "resellerNotifTrafficCriticalPercent", Value: "91"},
		{Key: "resellerNotifClientWarningPercent", Value: "68"},
		{Key: "resellerNotifExpiryWarningDays", Value: "12"},
		{Key: "resellerNotifInPanelEnable", Value: "false"},
		{Key: "resellerNotifTelegramEnable", Value: "false"},
	}
	if err := database.GetDB().Create(&settings).Error; err != nil {
		t.Fatalf("save notification settings: %v", err)
	}
	config = svc.Config()
	if config.TrafficWarningPercent != 72 || config.TrafficCriticalPercent != 91 ||
		config.ClientWarningPercent != 68 || config.ExpiryWarningDays != 12 ||
		config.InPanelEnable || config.TelegramEnable {
		t.Fatalf("stored config = %+v, want configured thresholds and both channels disabled", config)
	}
}
