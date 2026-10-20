package service

import (
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/xray"
	"gorm.io/gorm"
)

// Auto-renew schedule modes. The zero value preserves the legacy behavior of
// stepping whole Reset-day intervals, so clients created before these fields
// existed renew exactly as they always did.
const (
	RenewalModeRolling = 0 // step whole Reset-day intervals (legacy)
	RenewalModeMonthly = 1 // step whole calendar months
	RenewalModeWeekly  = 2 // renew on a fixed weekday (RenewalDay 0=Sun..6=Sat)
)

const msPerDay = int64(86400000)

// renewalSchedule is the resolved per-client auto-renew schedule used when a
// renewal fires. Mode is RenewalModeRolling unless the client record opts in
// to a calendar mode.
type renewalSchedule struct {
	mode int
	day  int
	// rollingDays mirrors the legacy traffic-row Reset value and is only
	// consulted for RenewalModeRolling.
	rollingDays int
}

// nextRenewalExpiry computes the expiry a renewal should set. It must never
// loop without bound and never return a value in the past: a non-positive
// result signals "cannot renew" and the caller leaves the client untouched.
//
//   - RenewalModeRolling: legacy semantics — add whole rollingDays-day steps
//     to oldExpiry until it lands at or after now (identical to the historic
//     `for newExpiry < now { newExpiry += Reset }` loop).
//   - RenewalModeMonthly: step whole calendar months from oldExpiry. When day
//     is 1..28 each step anchors on that day-of-month, otherwise the anchor is
//     oldExpiry's own day-of-month (clamped to the target month's length).
//     The time-of-day is preserved. The first anchor strictly after now wins.
//   - RenewalModeWeekly: the next occurrence of weekday `day` strictly after
//     now, at oldExpiry's time-of-day.
func nextRenewalExpiry(oldExpiry, now int64, sched renewalSchedule) int64 {
	switch sched.mode {
	case RenewalModeMonthly:
		return nextMonthlyRenewal(oldExpiry, now, sched.day)
	case RenewalModeWeekly:
		return nextWeeklyRenewal(oldExpiry, now, sched.day)
	default:
		if sched.rollingDays <= 0 {
			return 0
		}
		newExpiry := oldExpiry
		// Bounded: rollingDays >= 1 and now-oldExpiry is finite, so this loop
		// terminates. The explicit cap is a final safety net for corrupt input.
		for steps := 0; newExpiry < now && steps < 100000; steps++ {
			newExpiry += int64(sched.rollingDays) * msPerDay
		}
		if newExpiry < now {
			return 0
		}
		return newExpiry
	}
}

// loadCalendarRenewalSchedules resolves the calendar renewal opt-ins
// (renewal_mode > 0) for the supplied traffic rows, keyed by normalized email.
// Rows without an opt-in are absent from the map, which keeps the legacy
// rolling schedule (traffic-row reset) as the default.
func loadCalendarRenewalSchedules(tx *gorm.DB, traffics []*xray.ClientTraffic) map[string]renewalSchedule {
	out := make(map[string]renewalSchedule)
	if tx == nil || len(traffics) == 0 {
		return out
	}
	seen := make(map[string]struct{}, len(traffics))
	keys := make([]string, 0, len(traffics))
	for _, traffic := range traffics {
		if traffic == nil {
			continue
		}
		key := transferEmailKey(traffic.Email)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for start := 0; start < len(keys); start += sqliteMaxVars {
		end := min(start+sqliteMaxVars, len(keys))
		var rows []model.ClientRecord
		if err := tx.Where("LOWER(TRIM(email)) IN ? AND renewal_mode > 0", keys[start:end]).
			Find(&rows).Error; err != nil {
			continue
		}
		for i := range rows {
			key := strings.ToLower(strings.TrimSpace(rows[i].Email))
			if key == "" {
				continue
			}
			out[key] = renewalSchedule{
				mode: rows[i].RenewalMode,
				day:  rows[i].RenewalDay,
			}
		}
	}
	return out
}

// nextMonthlyRenewal steps whole calendar months from oldExpiry until the
// anchor lands strictly after now. The anchor's day-of-month is `day` when it
// is within 1..28, otherwise oldExpiry's day-of-month clamped to the target
// month's length. The time-of-day of oldExpiry is preserved.
func nextMonthlyRenewal(oldExpiry, now int64, day int) int64 {
	old := time.UnixMilli(oldExpiry)
	anchorDay := day
	if anchorDay < 1 || anchorDay > 28 {
		anchorDay = old.Day()
	}
	hh, mm, ss := old.Clock()
	// Hard cap (~100 years of monthly steps) so corrupt timestamps can never
	// spin the loop; every realistic input resolves within a few iterations.
	for k := 0; k < 1200; k++ {
		// time.Date normalizes month overflow (e.g. month 13 -> next January).
		cand := time.Date(old.Year(), old.Month()+time.Month(k), anchorDay, hh, mm, ss, 0, old.Location())
		if cand.Day() != anchorDay {
			// Target month is too short for anchorDay: use its last day by
			// taking day 0 of the following month.
			cand = time.Date(old.Year(), old.Month()+time.Month(k)+1, 0, hh, mm, ss, 0, old.Location())
		}
		if cand.UnixMilli() > now {
			return cand.UnixMilli()
		}
	}
	return 0
}

// nextWeeklyRenewal returns the next occurrence of weekday `day` strictly
// after now, at oldExpiry's time-of-day. Invalid weekdays return 0.
func nextWeeklyRenewal(oldExpiry, now int64, day int) int64 {
	if day < 0 || day > 6 {
		return 0
	}
	old := time.UnixMilli(oldExpiry)
	nowT := time.UnixMilli(now)
	hh, mm, ss := old.Clock()
	delta := (day - int(nowT.Weekday()) + 7) % 7
	cand := time.Date(nowT.Year(), nowT.Month(), nowT.Day()+delta, hh, mm, ss, 0, nowT.Location())
	if cand.UnixMilli() <= now {
		cand = cand.AddDate(0, 0, 7)
	}
	if cand.UnixMilli() <= now {
		return 0
	}
	return cand.UnixMilli()
}
