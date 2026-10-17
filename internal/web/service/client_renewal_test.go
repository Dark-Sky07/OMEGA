package service

import (
	"testing"
	"time"
)

// ts builds a millisecond timestamp in the local zone (the renewal math uses
// the local calendar via time.UnixMilli, so tests must construct in the same
// zone to stay deterministic across CI timezones).
func ts(year int, month time.Month, day, hour, min int) int64 {
	return time.Date(year, month, day, hour, min, 0, 0, time.Local).UnixMilli()
}

// The legacy rolling loop must stay identical to the historic
// `for newExpiry < now { newExpiry += Reset*86400000 }` (strictly <).
func TestNextRenewalExpiry_LegacyRollingParity(t *testing.T) {
	now := ts(2025, time.June, 15, 12, 0)

	// One renewal step: expiry 1h in the past, 30-day cycle.
	old := ts(2025, time.June, 15, 11, 0)
	got := nextRenewalExpiry(old, now, renewalSchedule{mode: RenewalModeRolling, rollingDays: 30})
	if want := old + 30*msPerDay; got != want {
		t.Fatalf("single step: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Exactly 10 whole days late on a 1-day cycle lands on now itself
	// (the legacy loop stops once newExpiry >= now).
	old = now - 10*msPerDay
	got = nextRenewalExpiry(old, now, renewalSchedule{mode: RenewalModeRolling, rollingDays: 1})
	if got != now {
		t.Fatalf("multi step: got %v want %v", time.UnixMilli(got), time.UnixMilli(now))
	}

	// A future expiry is returned unchanged — no step is taken.
	future := now + 5*msPerDay
	if got := nextRenewalExpiry(future, now, renewalSchedule{mode: RenewalModeRolling, rollingDays: 30}); got != future {
		t.Fatalf("future expiry: got %v want unchanged %v", time.UnixMilli(got), time.UnixMilli(future))
	}

	// rollingDays=0 disables the schedule entirely.
	if got := nextRenewalExpiry(now-msPerDay, now, renewalSchedule{mode: RenewalModeRolling, rollingDays: 0}); got != 0 {
		t.Fatalf("rollingDays=0 must skip, got %d", got)
	}
}

// Calendar-month renewal anchors on renewalDay (1-28) and steps whole
// calendar months; the first anchor strictly after now wins.
func TestNextRenewalExpiry_MonthlyAnchor(t *testing.T) {
	// This month's 15th is still ahead → it is the next anchor.
	now := ts(2025, time.January, 12, 9, 0)
	got := nextRenewalExpiry(ts(2025, time.January, 10, 8, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 15})
	if want := ts(2025, time.January, 15, 8, 0); got != want {
		t.Fatalf("anchor 15 this month: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Both this month's and last month's anchor passed → next month's 15th.
	now = ts(2025, time.February, 16, 9, 0)
	got = nextRenewalExpiry(ts(2025, time.January, 10, 8, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 15})
	if want := ts(2025, time.March, 15, 8, 0); got != want {
		t.Fatalf("roll past both anchors: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// renewalDay=0 auto-anchors on the old expiry's day-of-month (1st here).
	now = ts(2025, time.January, 31, 10, 0)
	got = nextRenewalExpiry(ts(2025, time.January, 1, 0, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 0})
	if want := ts(2025, time.February, 1, 0, 0); got != want {
		t.Fatalf("auto anchor: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Explicit day is limited to 1..28; anything else falls back to the old
	// expiry's day-of-month (day=31 is NOT a 31st anchor). Feb 15 has already
	// passed by Feb 20, so the anchor rolls to Mar 15.
	now = ts(2025, time.February, 20, 0, 0)
	got = nextRenewalExpiry(ts(2025, time.January, 15, 8, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 31})
	if want := ts(2025, time.March, 15, 8, 0); got != want {
		t.Fatalf("day=31 falls back to old day: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}
}

// Month-end anchors (old expiry on the 29th-31st) clamp to the last day of
// short months and return to the original day in long months.
func TestNextRenewalExpiry_MonthEndClamp(t *testing.T) {
	// Auto anchor 31 from a Jan 31 expiry; February 2025 clamps to the 28th.
	now := ts(2025, time.February, 20, 0, 0)
	got := nextRenewalExpiry(ts(2025, time.January, 31, 8, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 0})
	if want := ts(2025, time.February, 28, 8, 0); got != want {
		t.Fatalf("clamp to month end: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Once February's clamped anchor is gone, March restores the 31st.
	now = ts(2025, time.March, 2, 0, 0)
	got = nextRenewalExpiry(ts(2025, time.January, 31, 8, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 0})
	if want := ts(2025, time.March, 31, 8, 0); got != want {
		t.Fatalf("restore day after clamp: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Leap-year February clamps to the 29th.
	now = ts(2024, time.February, 1, 12, 0)
	got = nextRenewalExpiry(ts(2024, time.January, 31, 0, 0), now,
		renewalSchedule{mode: RenewalModeMonthly, day: 0})
	if want := ts(2024, time.February, 29, 0, 0); got != want {
		t.Fatalf("leap clamp: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}
}

// Weekly renewal lands on the next matching weekday strictly after now, at
// the old expiry's time-of-day. 2025-06-15 is a Sunday.
func TestNextRenewalExpiry_Weekly(t *testing.T) {
	now := ts(2025, time.June, 15, 12, 0)

	// Sunday 08:00 already passed 12:00 today → next Sunday.
	old := ts(2025, time.June, 15, 8, 0)
	got := nextRenewalExpiry(old, now, renewalSchedule{mode: RenewalModeWeekly, day: 0})
	if want := ts(2025, time.June, 22, 8, 0); got != want {
		t.Fatalf("weekly sunday rolled: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Sunday 18:00 is still ahead today → same Sunday.
	old = ts(2025, time.June, 15, 18, 0)
	got = nextRenewalExpiry(old, now, renewalSchedule{mode: RenewalModeWeekly, day: 0})
	if want := ts(2025, time.June, 15, 18, 0); got != want {
		t.Fatalf("weekly same-day: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Wednesday (3) of the same week.
	got = nextRenewalExpiry(old, now, renewalSchedule{mode: RenewalModeWeekly, day: 3})
	if want := ts(2025, time.June, 18, 18, 0); got != want {
		t.Fatalf("weekly wednesday: got %v want %v", time.UnixMilli(got), time.UnixMilli(want))
	}

	// Invalid weekday cannot produce a schedule.
	if got := nextRenewalExpiry(old, now, renewalSchedule{mode: RenewalModeWeekly, day: 7}); got != 0 {
		t.Fatalf("invalid weekday must skip, got %d", got)
	}
}
