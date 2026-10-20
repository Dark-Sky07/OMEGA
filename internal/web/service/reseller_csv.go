package service

import (
	"strconv"
	"strings"
	"time"
)

// csvBOM lets Excel auto-detect UTF-8 so Persian/Arabic/CJK values stay
// readable when the file is opened by double click.
var csvBOM = []byte{0xEF, 0xBB, 0xBF}

// csvEscape quotes a field following RFC 4180 (comma, quote, CR or LF force
// quoting; embedded quotes are doubled).
func csvEscape(field string) string {
	if strings.ContainsAny(field, ",\"\r\n") {
		return `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
	}
	return field
}

// csvGB renders a byte count in the panel's binary GB unit, like the UI.
func csvGB(bytes int64) string {
	return strconv.FormatFloat(float64(bytes)/bytesPerGB, 'f', 3, 64)
}

// csvDate renders a unix-millisecond timestamp as a plain date, empty for 0.
func csvDate(unixMilli int64) string {
	if unixMilli <= 0 {
		return ""
	}
	return time.UnixMilli(unixMilli).Format("2006-01-02")
}

// csvRowBuilder writes CRLF-terminated rows with a UTF-8 BOM header.
type csvRowBuilder struct {
	b strings.Builder
}

func (w *csvRowBuilder) row(fields ...string) {
	w.b.WriteString(strings.Join(fields, ","))
	w.b.WriteString("\r\n")
}

func (w *csvRowBuilder) bytes() []byte {
	return append(csvBOM, []byte(w.b.String())...)
}

// RenderResellerClientsCSV renders one reseller's report as CSV: one row per
// client with quota/usage in GB, cost and expiry. Empty quota/expiry fields
// mean "unlimited"/"never".
func RenderResellerClientsCSV(report *ResellerReport) []byte {
	w := &csvRowBuilder{}
	w.row("email", "comment", "group", "enabled", "quota_gb", "used_gb", "up_gb", "down_gb",
		"cost", "expiry", "inbound_ids", "sub_id")
	if report == nil {
		return w.bytes()
	}
	for _, row := range report.Clients {
		quota := ""
		if row.TotalBytes > 0 {
			quota = csvGB(row.TotalBytes)
		}
		ids := make([]string, 0, len(row.InboundIds))
		for _, id := range row.InboundIds {
			ids = append(ids, strconv.Itoa(id))
		}
		w.row(
			csvEscape(row.Email),
			csvEscape(row.Comment),
			csvEscape(row.Group),
			strconv.FormatBool(row.Enable),
			quota,
			csvGB(row.Used),
			csvGB(row.Up),
			csvGB(row.Down),
			strconv.FormatFloat(row.Cost, 'f', 3, 64),
			csvDate(row.ExpiryTime),
			csvEscape(strings.Join(ids, ";")),
			csvEscape(row.SubID),
		)
	}
	return w.bytes()
}

// RenderResellerListCSV renders the admin resellers list as CSV: one row per
// reseller with live usage numbers. Empty limit fields mean "unlimited".
func RenderResellerListCSV(stats []*ResellerStat) []byte {
	w := &csvRowBuilder{}
	w.row("id", "username", "name", "comment", "enabled", "clients", "client_limit", "online",
		"used_gb", "allocated_gb", "traffic_limit_gb", "balance", "price_per_gb", "expiry")
	for _, stat := range stats {
		if stat == nil || stat.Reseller == nil {
			continue
		}
		r := stat.Reseller
		clientLimit := ""
		if r.ClientLimit > 0 {
			clientLimit = strconv.Itoa(r.ClientLimit)
		}
		trafficLimit := ""
		if r.TrafficLimit > 0 {
			trafficLimit = csvGB(r.TrafficLimit)
		}
		w.row(
			strconv.Itoa(r.Id),
			csvEscape(r.Username),
			csvEscape(r.Name),
			csvEscape(r.Comment),
			strconv.FormatBool(r.Enable),
			strconv.Itoa(stat.ClientCount),
			clientLimit,
			strconv.Itoa(stat.OnlineCount),
			csvGB(stat.UsedTraffic),
			csvGB(stat.AllocatedTraffic),
			trafficLimit,
			strconv.FormatFloat(r.Deposit-stat.Cost, 'f', 2, 64),
			strconv.FormatFloat(r.PricePerGB, 'f', 3, 64),
			csvDate(r.ExpiryTime),
		)
	}
	return w.bytes()
}

// ReportCSV returns one reseller's client report as a UTF-8 (BOM) CSV.
func (s *ResellerService) ReportCSV(resellerId int) ([]byte, error) {
	report, err := s.Report(resellerId)
	if err != nil {
		return nil, err
	}
	return RenderResellerClientsCSV(report), nil
}

// ListCSV returns every reseller's live usage row as a UTF-8 (BOM) CSV.
func (s *ResellerService) ListCSV() ([]byte, error) {
	stats, err := s.List()
	if err != nil {
		return nil, err
	}
	return RenderResellerListCSV(stats), nil
}
