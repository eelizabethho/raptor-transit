// Package calendar resolves which GTFS services run on a given date.
//
// GTFS expresses a service's schedule in two files that must be combined:
// calendar.txt gives a weekday bitmap plus a start/end date range, and
// calendar_dates.txt lists exceptions that add or remove individual dates.
// An exception always wins over the weekly pattern, and a service may exist
// only as exceptions with no calendar.txt row at all (KCM's service 4557 is
// exactly that — see docs/FEED_NOTES.md).
//
// This logic lives in its own package because more than one caller needs
// it: the RAPTOR engine filters trips by service date on every query, and
// the HTTP API wants to answer "does anything run on this date?" without
// running a full query. The per-service predicate itself stays on
// timetable.Service.ActiveOn; this package is the set-level view over it.
package calendar

import (
	"time"

	"raptor-transit/internal/timetable"
)

// DateFormat is the GTFS service-date layout (YYYYMMDD).
const DateFormat = "20060102"

// ParseDate parses a GTFS service date. The returned time is midnight UTC
// on that date; only its calendar fields and weekday are ever used, so the
// zone is irrelevant.
func ParseDate(date string) (time.Time, error) {
	return time.Parse(DateFormat, date)
}

// ActiveServices returns the set of service_ids running on day.
//
// The result is a set rather than a slice because callers test membership
// once per candidate trip — on the full KCM feed that is millions of
// lookups per query.
func ActiveServices(services []timetable.Service, day time.Time) map[string]bool {
	date := day.Format(DateFormat)
	wd := day.Weekday()
	active := make(map[string]bool)
	for _, svc := range services {
		if svc.ActiveOn(date, wd) {
			active[svc.ID] = true
		}
	}
	return active
}

// InFeedWindow reports whether any service at all runs on day.
//
// A GTFS feed covers a bounded window (the current KCM feed ends
// 2027-03-26). Queries outside it are not errors — they correctly find
// nothing — but "no journey found" and "the feed does not cover that date"
// are very different messages to show a user, and this distinguishes them.
func InFeedWindow(services []timetable.Service, day time.Time) bool {
	return len(ActiveServices(services, day)) > 0
}
