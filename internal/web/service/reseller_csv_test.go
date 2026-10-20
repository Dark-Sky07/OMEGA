package service

import (
	"strings"
	"testing"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func csvLines(t *testing.T, csv string) []string {
	t.Helper()
	body := strings.TrimPrefix(csv, "\xEF\xBB\xBF")
	body = strings.TrimSuffix(body, "\r\n")
	return strings.Split(body, "\r\n")
}

func TestRenderResellerClientsCSV(t *testing.T) {
	expiry := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	report := &ResellerReport{
		Clients: []ResellerClientRow{
			{
				Email:      `ali, "the reseller" @x`,
				Comment:    "مشتری ویژه\nline two",
				Group:      "gold",
				Enable:     true,
				TotalBytes: 5 * bytesPerGB,
				ExpiryTime: expiry.UnixMilli(),
				Up:         bytesPerGB,
				Down:       2 * bytesPerGB,
				Used:       3 * bytesPerGB,
				Cost:       1.5,
				InboundIds: []int{3, 7},
				SubID:      "sub-1",
			},
			{
				// unlimited quota, no expiry, no inbound ids
				Email: "plain@x",
				Used:  bytesPerGB / 2,
			},
		},
	}
	csv := string(RenderResellerClientsCSV(report))

	if !strings.HasPrefix(csv, "\xEF\xBB\xBFemail,comment,group,enabled,quota_gb") {
		t.Fatalf("missing BOM or header: %q", csv[:60])
	}

	lines := csvLines(t, csv)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want header + 2 rows", len(lines))
	}

	// Comma, quotes and the embedded newline force RFC 4180 quoting; the
	// semicolon-separated inbound ids and plain fields stay unquoted.
	first := lines[1]
	if !strings.HasPrefix(first, `"ali, ""the reseller"" @x","مشتری ویژه`+"\n"+`line two",gold,true,5.000,3.000,1.000,2.000,1.500,2026-12-01,3;7,sub-1`) {
		t.Fatalf("first row = %q", first)
	}

	// Unlimited quota and never-expiring client render as empty fields.
	if second := lines[2]; second != "plain@x,,,false,,0.500,0.000,0.000,0.000,,," {
		t.Fatalf("second row = %q", second)
	}
}

func TestRenderResellerClientsCSVEmpty(t *testing.T) {
	csv := string(RenderResellerClientsCSV(nil))
	if csv != "\xEF\xBB\xBFemail,comment,group,enabled,quota_gb,used_gb,up_gb,down_gb,cost,expiry,inbound_ids,sub_id\r\n" {
		t.Fatalf("nil report = %q", csv)
	}
	if csv := string(RenderResellerClientsCSV(&ResellerReport{})); strings.Count(csv, "\r\n") != 1 {
		t.Fatalf("empty report must be header only, got %q", csv)
	}
}

func TestRenderResellerListCSV(t *testing.T) {
	stats := []*ResellerStat{
		{
			Reseller: &model.Reseller{
				Id:          7,
				Username:    "reseller1",
				Name:        "Ali",
				Comment:     "note, with comma",
				Enable:      true,
				ClientLimit: 10,
				ExpiryTime:  time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
				PricePerGB:  0.5,
				Deposit:     20,
			},
			ClientCount:      4,
			OnlineCount:      2,
			UsedTraffic:      2 * bytesPerGB,
			AllocatedTraffic: 5 * bytesPerGB,
			Cost:             1,
		},
		{
			// unlimited everything
			Reseller: &model.Reseller{Id: 8, Username: "reseller2", Enable: false},
		},
		nil,
	}
	lines := csvLines(t, string(RenderResellerListCSV(stats)))
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want header + 2 rows (nil stat skipped)", len(lines))
	}
	if lines[0] != "id,username,name,comment,enabled,clients,client_limit,online,used_gb,allocated_gb,traffic_limit_gb,balance,price_per_gb,expiry" {
		t.Fatalf("header = %q", lines[0])
	}
	if lines[1] != `7,reseller1,Ali,"note, with comma",true,4,10,2,2.000,5.000,,19.00,0.500,2027-03-01` {
		t.Fatalf("first row = %q", lines[1])
	}
	if lines[2] != "8,reseller2,,,false,0,,0,0.000,0.000,,0.00,0.000," {
		t.Fatalf("second row = %q", lines[2])
	}
}
